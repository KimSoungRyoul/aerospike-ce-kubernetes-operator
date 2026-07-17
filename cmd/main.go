/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"os"
	"time"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	uberzap "go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	ackov1alpha1 "github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/api/v1alpha1"
	"github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/internal/controller"
	"github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/internal/telemetry"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
	// version is stamped onto telemetry as the service.version resource
	// attribute. Override at build time with -ldflags "-X main.version=...".
	version = "dev"
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(ackov1alpha1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

func main() {
	// run holds the real startup logic so that its deferred telemetry
	// Shutdown is guaranteed to execute — os.Exit (used here for a non-zero
	// exit code) skips deferred calls, which would drop the final batch of
	// buffered traces/metrics/logs on every error path.
	if err := run(); err != nil {
		setupLog.Error(err, "Operator exited with error")
		os.Exit(1)
	}
}

// nolint:gocyclo
func run() error {
	var metricsAddr string
	var metricsCertPath, metricsCertName, metricsCertKey string
	var webhookCertPath, webhookCertName, webhookCertKey string
	var enableLeaderElection bool
	var probeAddr string
	var secureMetrics bool
	var enableHTTP2 bool
	var tlsOpts []func(*tls.Config)
	flag.StringVar(&metricsAddr, "metrics-bind-address", "0", "The address the metrics endpoint binds to. "+
		"Use :8443 for HTTPS or :8080 for HTTP, or leave as 0 to disable the metrics service.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	// Leader election defaults to true so that a bare in-cluster deployment
	// (one that omits the flag) can never end up with two active managers
	// reconciling — and racing pod deletions on — the same AerospikeCluster.
	// The Helm chart and config/manager/manager.yaml pass --leader-elect
	// explicitly; local out-of-cluster runs must pass --leader-elect=false
	// (make run does) because there is no in-cluster namespace to host the
	// election Lease.
	flag.BoolVar(&enableLeaderElection, "leader-elect", true,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager. "+
			"Set --leader-elect=false for local out-of-cluster runs (e.g. make run).")
	flag.BoolVar(&secureMetrics, "metrics-secure", true,
		"If set, the metrics endpoint is served securely via HTTPS. Use --metrics-secure=false to use HTTP instead.")
	flag.StringVar(&webhookCertPath, "webhook-cert-path", "", "The directory that contains the webhook certificate.")
	flag.StringVar(&webhookCertName, "webhook-cert-name", "tls.crt", "The name of the webhook certificate file.")
	flag.StringVar(&webhookCertKey, "webhook-cert-key", "tls.key", "The name of the webhook key file.")
	flag.StringVar(&metricsCertPath, "metrics-cert-path", "",
		"The directory that contains the metrics server certificate.")
	flag.StringVar(&metricsCertName, "metrics-cert-name", "tls.crt", "The name of the metrics server certificate file.")
	flag.StringVar(&metricsCertKey, "metrics-cert-key", "tls.key", "The name of the metrics server key file.")
	flag.BoolVar(&enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics and webhook servers")
	var devMode bool
	flag.BoolVar(&devMode, "dev", false, "Enable development mode logging (human-readable, DPanic panics)")
	opts := zap.Options{}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()
	opts.Development = devMode

	// Initialize the OpenTelemetry pipeline before building the logger so the
	// OTLP log core can be teed into the controller-runtime zap logger. Setup
	// is a no-op unless OTEL_SDK_DISABLED is explicitly falsy.
	tel, telErr := telemetry.Setup(context.Background(), version)
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := tel.Shutdown(shutdownCtx); err != nil {
			setupLog.Error(err, "Failed to shut down telemetry")
		}
	}()
	if otelCore := tel.ZapCore(); otelCore != nil {
		opts.ZapOpts = append(opts.ZapOpts, uberzap.WrapCore(func(c zapcore.Core) zapcore.Core {
			return zapcore.NewTee(c, otelCore)
		}))
	}

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	if telErr != nil {
		setupLog.Error(telErr, "OpenTelemetry setup failed; continuing with telemetry export disabled")
	} else if tel.Enabled() {
		setupLog.Info("OpenTelemetry export enabled", "service", telemetry.ServiceName)
	}

	// if the enable-http2 flag is false (the default), http/2 should be disabled
	// due to its vulnerabilities. More specifically, disabling http/2 will
	// prevent from being vulnerable to the HTTP/2 Stream Cancellation and
	// Rapid Reset CVEs. For more information see:
	// - https://github.com/advisories/GHSA-qppj-fm5r-hxr3
	// - https://github.com/advisories/GHSA-4374-p667-p6c8
	disableHTTP2 := func(c *tls.Config) {
		setupLog.Info("Disabling HTTP/2")
		c.NextProtos = []string{"http/1.1"}
	}

	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	// Initial webhook TLS options
	webhookTLSOpts := tlsOpts
	webhookServerOptions := webhook.Options{
		TLSOpts: webhookTLSOpts,
	}

	if len(webhookCertPath) > 0 {
		setupLog.Info("Initializing webhook certificate watcher using provided certificates",
			"webhook-cert-path", webhookCertPath, "webhook-cert-name", webhookCertName, "webhook-cert-key", webhookCertKey)

		webhookServerOptions.CertDir = webhookCertPath
		webhookServerOptions.CertName = webhookCertName
		webhookServerOptions.KeyName = webhookCertKey
	}

	webhookServer := webhook.NewServer(webhookServerOptions)

	// Metrics endpoint is enabled in 'config/default/kustomization.yaml'. The Metrics options configure the server.
	// More info:
	// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.23.1/pkg/metrics/server
	// - https://book.kubebuilder.io/reference/metrics.html
	metricsServerOptions := metricsserver.Options{
		BindAddress:   metricsAddr,
		SecureServing: secureMetrics,
		TLSOpts:       tlsOpts,
	}

	if secureMetrics {
		// FilterProvider is used to protect the metrics endpoint with authn/authz.
		// These configurations ensure that only authorized users and service accounts
		// can access the metrics endpoint. The RBAC are configured in 'config/rbac/kustomization.yaml'. More info:
		// https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.23.1/pkg/metrics/filters#WithAuthenticationAndAuthorization
		metricsServerOptions.FilterProvider = filters.WithAuthenticationAndAuthorization
	}

	if len(metricsCertPath) > 0 {
		setupLog.Info("Initializing metrics certificate watcher using provided certificates",
			"metrics-cert-path", metricsCertPath, "metrics-cert-name", metricsCertName, "metrics-cert-key", metricsCertKey)

		metricsServerOptions.CertDir = metricsCertPath
		metricsServerOptions.CertName = metricsCertName
		metricsServerOptions.KeyName = metricsCertKey
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsServerOptions,
		WebhookServer:          webhookServer,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "ed3f4371.aerospike.com",
	})
	if err != nil {
		return fmt.Errorf("unable to start manager: %w", err)
	}

	if err := (&controller.AerospikeClusterReconciler{
		Client:     mgr.GetClient(),
		Scheme:     mgr.GetScheme(),
		RestConfig: mgr.GetConfig(),
		//nolint:staticcheck // GetEventRecorder returns incompatible interface
		Recorder: mgr.GetEventRecorderFor("aerospikecluster-controller"),
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to create controller AerospikeCluster: %w", err)
	}
	if err := (&ackov1alpha1.AerospikeCluster{}).SetupWebhookWithManager(mgr); err != nil {
		return fmt.Errorf("unable to create webhook AerospikeCluster: %w", err)
	}
	if err := (&ackov1alpha1.AerospikeClusterTemplate{}).SetupWebhookWithManager(mgr); err != nil {
		return fmt.Errorf("unable to create webhook AerospikeClusterTemplate: %w", err)
	}
	// +kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return fmt.Errorf("unable to set up health check: %w", err)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return fmt.Errorf("unable to set up ready check: %w", err)
	}

	setupLog.Info("Starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		return fmt.Errorf("problem running manager: %w", err)
	}

	return nil
}
