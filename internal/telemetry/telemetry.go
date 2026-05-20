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

// Package telemetry wires the operator's OpenTelemetry pipeline — traces,
// metrics, and logs exported to an OTLP/gRPC collector.
//
// It is OFF by default. Setup is a no-op unless OTEL_SDK_DISABLED is
// explicitly set to a falsy value (e.g. "false"); while off, the global OTel
// providers stay NoOp and instrumentation costs effectively nothing. The
// unset-means-disabled default matches the aerospike-cluster-manager API so
// both halves of the stack behave the same way.
//
// When on, every exporter / sampler / resource detail is taken from the
// OpenTelemetry SDK standard environment variables — OTEL_EXPORTER_OTLP_ENDPOINT,
// OTEL_EXPORTER_OTLP_HEADERS, OTEL_TRACES_SAMPLER, OTEL_RESOURCE_ATTRIBUTES,
// OTEL_SERVICE_NAME, … — so no operator-specific wrapper flags are introduced.
// Signals exported:
//
//   - traces  : reconcile spans (see internal/controller).
//   - metrics : the whole controller-runtime + ACKO Prometheus registry,
//     bridged to OTLP — so the exact same series stay available both via the
//     /metrics scrape endpoint and as an OTLP push, with no metric-definition
//     code changes.
//   - logs    : the operator's zap log stream, teed into the OTLP log pipeline
//     by the otelzap bridge (wired in cmd/main.go via the core from ZapCore).
package telemetry

import (
	"context"
	"errors"
	"os"
	"strings"

	"go.opentelemetry.io/contrib/bridges/otelzap"
	prombridge "go.opentelemetry.io/contrib/bridges/prometheus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otlplog "go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	otlpmetric "go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	otlptrace "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	logglobal "go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.uber.org/zap/zapcore"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

// ServiceName is the OTel service.name stamped on every signal the operator
// emits, and the instrumentation-scope name used for its tracer.
const ServiceName = "aerospike-ce-kubernetes-operator"

// Provider holds the operator's configured OTel SDK providers together with
// the hooks needed to flush and shut them down. A disabled Provider (the
// default) is fully inert: Enabled() is false, ZapCore() is nil, and
// Shutdown() is a no-op.
type Provider struct {
	enabled       bool
	zapCore       zapcore.Core
	shutdownFuncs []func(context.Context) error
}

// Enabled reports whether the OTel pipeline was activated.
func (p *Provider) Enabled() bool { return p != nil && p.enabled }

// ZapCore returns a zapcore.Core that mirrors the operator's log stream into
// the OTLP log pipeline, or nil when telemetry is disabled. cmd/main.go tees
// this into the controller-runtime zap logger via zap.WrapCore.
func (p *Provider) ZapCore() zapcore.Core {
	if p == nil {
		return nil
	}
	return p.zapCore
}

// Shutdown flushes and stops every provider, giving the batch processors a
// final chance to export. Safe to call on a nil or disabled Provider, and
// safe to call after Setup already returned an error (it is then a no-op).
func (p *Provider) Shutdown(ctx context.Context) error {
	if p == nil {
		return nil
	}
	var errs []error
	// Shut down in reverse registration order (logs, then metrics, then traces).
	for i := len(p.shutdownFuncs) - 1; i >= 0; i-- {
		if err := p.shutdownFuncs[i](ctx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// activated reports whether OTEL_SDK_DISABLED opts the operator INTO
// telemetry. The operator ships with telemetry off, so an unset variable
// means disabled.
func activated() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("OTEL_SDK_DISABLED")))
	if v == "" {
		return false
	}
	return v != "true" && v != "1" && v != "yes"
}

// Setup builds the OTLP trace/metric/log providers, registers them as the
// global OpenTelemetry providers, and installs the W3C trace-context + baggage
// propagator. serviceVersion is stamped onto the resource as service.version.
//
// When OTEL_SDK_DISABLED keeps telemetry off, Setup returns a disabled
// Provider and no error. When a provider fails to build, any partially
// constructed providers are flushed and stopped, an inert Provider is
// returned, and the error is surfaced so the caller can keep running the
// operator with telemetry off.
func Setup(ctx context.Context, serviceVersion string) (*Provider, error) {
	if !activated() {
		return &Provider{}, nil
	}

	p := &Provider{}
	// fail flushes whatever was built so far and returns an inert Provider,
	// so the caller's deferred Shutdown becomes a clean no-op.
	fail := func(err error) (*Provider, error) {
		_ = p.Shutdown(ctx)
		return &Provider{}, err
	}

	// Defaults first, then WithFromEnv so OTEL_SERVICE_NAME /
	// OTEL_RESOURCE_ATTRIBUTES can override service.name / extra attributes.
	res, err := resource.New(ctx,
		resource.WithTelemetrySDK(),
		resource.WithAttributes(
			attribute.String("service.name", ServiceName),
			attribute.String("service.version", serviceVersion),
		),
		resource.WithFromEnv(),
	)
	if err != nil {
		return fail(err)
	}

	// Build all three exporters/providers BEFORE touching any global state.
	// If a later exporter fails, fail() shuts down whatever was built and the
	// global OTel providers are never mutated — they stay NoOp, so the
	// operator continues cleanly with telemetry off.

	// --- Traces ---------------------------------------------------------
	traceExp, err := otlptrace.New(ctx)
	if err != nil {
		return fail(err)
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExp),
		sdktrace.WithResource(res),
	)
	p.shutdownFuncs = append(p.shutdownFuncs, tp.Shutdown)

	// --- Metrics --------------------------------------------------------
	// The Prometheus bridge republishes the controller-runtime metrics
	// registry — the built-in reconcile/workqueue metrics plus every ACKO
	// custom metric from internal/metrics — through OTLP. No metric
	// definition changes: the same series stay scrapable at /metrics and
	// are additionally pushed to the collector.
	metricExp, err := otlpmetric.New(ctx)
	if err != nil {
		return fail(err)
	}
	reader := sdkmetric.NewPeriodicReader(metricExp,
		sdkmetric.WithProducer(prombridge.NewMetricProducer(
			prombridge.WithGatherer(ctrlmetrics.Registry),
		)),
	)
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(reader),
		sdkmetric.WithResource(res),
	)
	p.shutdownFuncs = append(p.shutdownFuncs, mp.Shutdown)

	// --- Logs -----------------------------------------------------------
	logExp, err := otlplog.New(ctx)
	if err != nil {
		return fail(err)
	}
	lp := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewBatchProcessor(logExp)),
		sdklog.WithResource(res),
	)
	p.shutdownFuncs = append(p.shutdownFuncs, lp.Shutdown)

	// All three providers built successfully — now publish them globally.
	otel.SetTracerProvider(tp)
	otel.SetMeterProvider(mp)
	logglobal.SetLoggerProvider(lp)
	// W3C TraceContext + Baggage so operator spans stitch with the
	// cluster-manager API and any other OTel-instrumented service.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	// otelzap core — cmd/main.go tees this into the zap logger so operator
	// log lines are exported as OTLP log records alongside traces.
	p.zapCore = otelzap.NewCore(ServiceName, otelzap.WithLoggerProvider(lp))

	p.enabled = true
	return p, nil
}
