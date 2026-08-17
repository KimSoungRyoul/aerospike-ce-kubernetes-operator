package controller

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	ackov1alpha1 "github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/api/v1alpha1"
)

// errMonitoringGetFailure is a sentinel non-NotFound error used to simulate an
// API failure during monitoring-resource cleanup.
var errMonitoringGetFailure = errors.New("simulated API server failure")

// monitoringTestCluster is the cluster name shared by the monitoring unit tests.
// Named rather than repeated inline so goconst does not flag the literal, and so
// the fixtures and the expected resource names cannot drift apart.
const monitoringTestCluster = "demo"

func TestToStringMap(t *testing.T) {
	tests := []struct {
		name string
		in   map[string]string
		want map[string]any
	}{
		{
			name: "nil map returns empty map (not nil)",
			in:   nil,
			want: map[string]any{},
		},
		{
			name: "empty map returns empty map",
			in:   map[string]string{},
			want: map[string]any{},
		},
		{
			name: "single entry is converted",
			in:   map[string]string{"app": "aerospike"},
			want: map[string]any{"app": "aerospike"},
		},
		{
			name: "multiple entries are all converted",
			in: map[string]string{
				"app":  "aerospike",
				"team": "platform",
				"env":  "prod",
			},
			want: map[string]any{
				"app":  "aerospike",
				"team": "platform",
				"env":  "prod",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := toStringMap(tc.in)
			if got == nil {
				t.Fatal("toStringMap() returned nil, want non-nil map")
			}
			if len(got) != len(tc.want) {
				t.Fatalf("toStringMap() returned %d entries, want %d", len(got), len(tc.want))
			}
			for k, wantVal := range tc.want {
				gotVal, ok := got[k]
				if !ok {
					t.Errorf("toStringMap() missing key %q", k)
					continue
				}
				if gotVal != wantVal {
					t.Errorf("toStringMap()[%q] = %v, want %v", k, gotVal, wantVal)
				}
			}
		})
	}
}

func TestDefaultAlertRules(t *testing.T) {
	rules := defaultAlertRules("my-cluster", "aerospike")
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule group, got %d", len(rules))
	}

	group, ok := rules[0].(map[string]any)
	if !ok {
		t.Fatal("rule group is not map[string]any")
	}

	groupName, ok := group["name"].(string)
	if !ok || groupName != "my-cluster.rules" {
		t.Errorf("group name = %q, want %q", groupName, "my-cluster.rules")
	}

	ruleList, ok := group["rules"].([]any)
	if !ok {
		t.Fatal("rules is not []any")
	}

	expectedAlerts := []string{
		"AerospikeNodeDown",
		"AerospikeNamespaceStopWrites",
		"AerospikeHighDiskUsage",
		"AerospikeHighMemoryUsage",
		"AerospikeReconcileStale",
		"AerospikeClusterSizeMismatch",
		"AerospikeOperatorCircuitBreakerActive",
	}

	if len(ruleList) != len(expectedAlerts) {
		t.Fatalf("expected %d alert rules, got %d", len(expectedAlerts), len(ruleList))
	}

	for i, expected := range expectedAlerts {
		rule, ok := ruleList[i].(map[string]any)
		if !ok {
			t.Fatalf("rule[%d] is not map[string]any", i)
		}
		alertName, ok := rule["alert"].(string)
		if !ok || alertName != expected {
			t.Errorf("rule[%d].alert = %q, want %q", i, alertName, expected)
		}

		// Verify expressions reference the cluster/namespace context
		expr, ok := rule["expr"].(string)
		if !ok {
			t.Errorf("rule[%d].expr is not string", i)
			continue
		}
		if !strings.Contains(expr, "aerospike") {
			t.Errorf("rule[%d].expr = %q, expected to contain namespace reference", i, expr)
		}
		if !strings.Contains(expr, "my-cluster") {
			t.Errorf("rule[%d].expr = %q, expected to contain cluster name reference", i, expr)
		}
	}
}

func TestDefaultAlertRules_LabelSeverity(t *testing.T) {
	rules := defaultAlertRules("test", "ns")
	group := rules[0].(map[string]any)
	ruleList := group["rules"].([]any)

	criticalCount := 0
	warningCount := 0
	for _, r := range ruleList {
		rule := r.(map[string]any)
		labels := rule["labels"].(map[string]any)
		switch labels["severity"].(string) {
		case "critical":
			criticalCount++
		case "warning":
			warningCount++
		}
	}

	if criticalCount != 3 {
		t.Errorf("expected 3 critical alerts, got %d", criticalCount)
	}
	if warningCount != 4 {
		t.Errorf("expected 4 warning alerts, got %d", warningCount)
	}
}

func TestUnstructuredResourceChanged(t *testing.T) {
	t.Run("returns false when spec and labels are unchanged", func(t *testing.T) {
		existing := &unstructured.Unstructured{
			Object: map[string]any{
				"spec": map[string]any{
					"groups": []any{map[string]any{"name": "test.rules"}},
				},
			},
		}
		existing.SetLabels(map[string]string{"app": "aerospike", "instance": "test"})

		changed := unstructuredResourceChanged(
			existing,
			map[string]any{
				"groups": []any{map[string]any{"name": "test.rules"}},
			},
			map[string]string{"app": "aerospike", "instance": "test"},
		)
		if changed {
			t.Fatal("unstructuredResourceChanged() = true, want false")
		}
	})

	t.Run("returns true when labels change", func(t *testing.T) {
		existing := &unstructured.Unstructured{
			Object: map[string]any{
				"spec": map[string]any{
					"selector": map[string]any{"matchLabels": map[string]any{"app": "aerospike"}},
				},
			},
		}
		existing.SetLabels(map[string]string{"app": "aerospike", "instance": "test"})

		changed := unstructuredResourceChanged(
			existing,
			map[string]any{
				"selector": map[string]any{"matchLabels": map[string]any{"app": "aerospike"}},
			},
			map[string]string{"app": "aerospike", "instance": "test2"},
		)
		if !changed {
			t.Fatal("unstructuredResourceChanged() = false, want true when labels differ")
		}
	})

	t.Run("returns true when spec changes", func(t *testing.T) {
		existing := &unstructured.Unstructured{
			Object: map[string]any{
				"spec": map[string]any{
					"groups": []any{map[string]any{"name": "test.rules"}},
				},
			},
		}
		existing.SetLabels(map[string]string{"app": "aerospike"})

		changed := unstructuredResourceChanged(
			existing,
			map[string]any{
				"groups": []any{map[string]any{"name": "test-v2.rules"}},
			},
			map[string]string{"app": "aerospike"},
		)
		if !changed {
			t.Fatal("unstructuredResourceChanged() = false, want true when spec differs")
		}
	})

	t.Run("returns true when existing spec is missing", func(t *testing.T) {
		existing := &unstructured.Unstructured{Object: map[string]any{}}
		existing.SetLabels(map[string]string{"app": "aerospike"})

		changed := unstructuredResourceChanged(
			existing,
			map[string]any{"groups": []any{}},
			map[string]string{"app": "aerospike"},
		)
		if !changed {
			t.Fatal("unstructuredResourceChanged() = false, want true when existing spec is missing")
		}
	})
}

func TestMetricsServiceNeedsUpdate(t *testing.T) {
	desired := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"app.kubernetes.io/name":     "aerospike-cluster",
				"app.kubernetes.io/instance": "demo",
			},
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: map[string]string{"app.kubernetes.io/instance": "demo"},
			Ports: []corev1.ServicePort{
				{
					Name:       "metrics",
					Port:       9145,
					TargetPort: intstr.FromInt32(9145),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}

	tests := []struct {
		name   string
		mutate func(existing *corev1.Service)
		want   bool
	}{
		{
			name:   "unchanged",
			mutate: func(_ *corev1.Service) {},
			want:   false,
		},
		{
			name: "type drift",
			mutate: func(existing *corev1.Service) {
				existing.Spec.Type = corev1.ServiceTypeNodePort
			},
			want: true,
		},
		{
			name: "selector drift",
			mutate: func(existing *corev1.Service) {
				existing.Spec.Selector = map[string]string{"app.kubernetes.io/instance": "other"}
			},
			want: true,
		},
		{
			name: "port drift",
			mutate: func(existing *corev1.Service) {
				existing.Spec.Ports[0].Port = 9200
			},
			want: true,
		},
		{
			name: "labels drift",
			mutate: func(existing *corev1.Service) {
				existing.Labels["custom"] = "stale"
			},
			want: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			existing := desired.DeepCopy()
			tc.mutate(existing)
			if got := metricsServiceNeedsUpdate(existing, desired); got != tc.want {
				t.Fatalf("metricsServiceNeedsUpdate() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestReconcileMetricsService_CleanupGetErrorNotSwallowed is the regression test
// for the monitoring cleanup error-masking fix. When monitoring is disabled and
// the Get for the existing metrics Service fails with a non-NotFound error, the
// reconcile must surface that error instead of silently returning nil — a
// swallowed error would leak the Service while reporting success.
func TestReconcileMetricsService_CleanupGetErrorNotSwallowed(t *testing.T) {
	scheme := rollingRestartScheme(t)

	cluster := &ackov1alpha1.AerospikeCluster{}
	cluster.Name = monitoringTestCluster
	cluster.Namespace = ctrlTestNamespace

	base := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	wrapped := interceptor.NewClient(base, interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey,
			obj client.Object, opts ...client.GetOption) error {
			if _, ok := obj.(*corev1.Service); ok {
				return apierrors.NewInternalError(errMonitoringGetFailure)
			}
			return c.Get(ctx, key, obj, opts...)
		},
	})

	r := &AerospikeClusterReconciler{Client: wrapped, Scheme: scheme}

	err := r.reconcileMetricsService(context.Background(), cluster, false)
	if err == nil {
		t.Fatal("expected error when cleanup Get fails with a non-NotFound error, got nil")
	}
	if !strings.Contains(err.Error(), "getting metrics service") {
		t.Errorf("expected wrapped cleanup Get error, got: %v", err)
	}
}

// monitoringResourceFor returns the monitoring.coreos.com resource name for an
// unstructured object, or "" if the object is not one of the two optional
// Prometheus-Operator kinds.
func monitoringResourceFor(obj client.Object) string {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return ""
	}
	gvk := u.GroupVersionKind()
	if gvk.Group != "monitoring.coreos.com" {
		return ""
	}
	switch gvk.Kind {
	case "ServiceMonitor":
		return "servicemonitors"
	case "PrometheusRule":
		return "prometheusrules"
	}
	return ""
}

// monitoringGetErrClient builds a fake client whose Get fails for every
// monitoring.coreos.com object with the error errFor produces for that
// resource, while serving every other Get normally.
func monitoringGetErrClient(scheme *runtime.Scheme, errFor func(resource string) error) client.WithWatch {
	base := fake.NewClientBuilder().WithScheme(scheme).Build()

	return interceptor.NewClient(base, interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey,
			obj client.Object, opts ...client.GetOption) error {
			if resource := monitoringResourceFor(obj); resource != "" {
				return errFor(resource)
			}
			return c.Get(ctx, key, obj, opts...)
		},
	})
}

func forbiddenMonitoringErr(resource string) error {
	return apierrors.NewForbidden(
		schema.GroupResource{Group: "monitoring.coreos.com", Resource: resource},
		monitoringTestCluster,
		errors.New(`clusterrole "acko-manager" does not grant this resource`),
	)
}

// TestReconcileMonitoring_ForbiddenDegradesFeature is the regression test for
// the RBAC-403 cluster freeze. reconcilePrometheusRule issues a Get on *every*
// reconcile — including when the feature is disabled, where the Get exists only
// to clean up a stale object. A chart install whose manager ClusterRole omits
// monitoring.coreos.com/prometheusrules therefore got a 403 on every pass, and
// the error funnelled into handleReconcileError until the circuit breaker
// wedged the cluster in BackoffActive: no scale, no rolling restart, no config
// change, no ACL sync. A 403 on an optional resource must degrade that one
// feature and leave the reconcile successful, exactly as a missing CRD does.
func TestReconcileMonitoring_ForbiddenDegradesFeature(t *testing.T) {
	tests := []struct {
		name       string
		monitoring *ackov1alpha1.AerospikeMonitoringSpec
	}{
		{
			// The reported failure mode: the cluster asks for no monitoring at
			// all, yet the stale-object cleanup Get still 403s.
			name:       "monitoring not configured",
			monitoring: nil,
		},
		{
			name: "monitoring enabled, optional resources disabled",
			monitoring: &ackov1alpha1.AerospikeMonitoringSpec{
				Enabled:       true,
				ExporterImage: "exporter:v1",
				Port:          9145,
			},
		},
		{
			name: "optional resources explicitly enabled",
			monitoring: &ackov1alpha1.AerospikeMonitoringSpec{
				Enabled:       true,
				ExporterImage: "exporter:v1",
				Port:          9145,
				ServiceMonitor: &ackov1alpha1.ServiceMonitorSpec{
					Enabled:  true,
					Interval: "30s",
				},
				PrometheusRule: &ackov1alpha1.PrometheusRuleSpec{
					Enabled: true,
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			scheme := rollingRestartScheme(t)

			cluster := &ackov1alpha1.AerospikeCluster{}
			cluster.Name = monitoringTestCluster
			cluster.Namespace = ctrlTestNamespace
			cluster.Spec.Monitoring = tc.monitoring

			r := &AerospikeClusterReconciler{
				Client: monitoringGetErrClient(scheme, forbiddenMonitoringErr),
				Scheme: scheme,
			}

			if err := r.reconcileMonitoring(context.Background(), cluster); err != nil {
				t.Fatalf("reconcileMonitoring() = %v, want nil: a 403 on an optional "+
					"monitoring resource must not fail the reconcile", err)
			}
		})
	}
}

// TestReconcileMonitoring_UnexpectedErrorStillFailsReconcile guards the other
// side of the fix: only "this resource is unreachable by design" errors — a
// missing CRD or a 403 — are allowed to degrade the feature. A transient API
// failure must still surface, or a real outage would be silently swallowed.
func TestReconcileMonitoring_UnexpectedErrorStillFailsReconcile(t *testing.T) {
	scheme := rollingRestartScheme(t)

	cluster := &ackov1alpha1.AerospikeCluster{}
	cluster.Name = monitoringTestCluster
	cluster.Namespace = ctrlTestNamespace

	r := &AerospikeClusterReconciler{
		Client: monitoringGetErrClient(scheme, func(string) error {
			return apierrors.NewInternalError(errMonitoringGetFailure)
		}),
		Scheme: scheme,
	}

	err := r.reconcileMonitoring(context.Background(), cluster)
	if err == nil {
		t.Fatal("reconcileMonitoring() = nil, want error for a non-403 API failure")
	}
	if !strings.Contains(err.Error(), "reconciling ServiceMonitor") {
		t.Errorf("expected the ServiceMonitor error to propagate, got: %v", err)
	}
}

// recordingSink captures which level a log line was emitted at, so the
// enabled/disabled split in logForbiddenMonitoringResource is asserted rather
// than assumed. WithValues/WithName intentionally return the same sink: the test
// only cares about messages and levels.
type recordingSink struct {
	errorMsgs []string
	infoMsgs  []string
}

func (s *recordingSink) Init(logr.RuntimeInfo)               {}
func (s *recordingSink) Enabled(int) bool                    { return true }
func (s *recordingSink) WithValues(...any) logr.LogSink      { return s }
func (s *recordingSink) WithName(string) logr.LogSink        { return s }
func (s *recordingSink) Info(_ int, msg string, _ ...any)    { s.infoMsgs = append(s.infoMsgs, msg) }
func (s *recordingSink) Error(_ error, msg string, _ ...any) { s.errorMsgs = append(s.errorMsgs, msg) }

func (s *recordingSink) logged(msgs []string, want string) bool {
	for _, m := range msgs {
		if strings.Contains(m, want) {
			return true
		}
	}
	return false
}

// TestReconcileMonitoring_ForbiddenLogLevelMatchesEnabled pins the Error-vs-Info
// split. A 403 on a resource the user explicitly enabled means the operator is
// silently not delivering something that was asked for, and with no
// MonitoringReady condition to read the log is the only signal — so it must be
// Error. A 403 on a disabled resource only blocks the stale-object cleanup Get,
// where an Error on every reconcile would train operators to ignore the
// operator's Error logs.
func TestReconcileMonitoring_ForbiddenLogLevelMatchesEnabled(t *testing.T) {
	const (
		errorMsg = "Access denied for an enabled monitoring resource"
		infoMsg  = "Access denied for monitoring resource, skipping stale-object cleanup"
	)

	tests := []struct {
		name         string
		monitoring   *ackov1alpha1.AerospikeMonitoringSpec
		wantErrorLog bool
	}{
		{
			name:         "disabled resource logs at Info",
			monitoring:   nil,
			wantErrorLog: false,
		},
		{
			name: "enabled resource logs at Error",
			monitoring: &ackov1alpha1.AerospikeMonitoringSpec{
				Enabled:       true,
				ExporterImage: "exporter:v1",
				Port:          9145,
				ServiceMonitor: &ackov1alpha1.ServiceMonitorSpec{
					Enabled:  true,
					Interval: "30s",
				},
				PrometheusRule: &ackov1alpha1.PrometheusRuleSpec{Enabled: true},
			},
			wantErrorLog: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			scheme := rollingRestartScheme(t)

			cluster := &ackov1alpha1.AerospikeCluster{}
			cluster.Name = monitoringTestCluster
			cluster.Namespace = ctrlTestNamespace
			cluster.Spec.Monitoring = tc.monitoring

			r := &AerospikeClusterReconciler{
				Client: monitoringGetErrClient(scheme, forbiddenMonitoringErr),
				Scheme: scheme,
			}

			sink := &recordingSink{}
			ctx := logf.IntoContext(context.Background(), logr.New(sink))

			if err := r.reconcileMonitoring(ctx, cluster); err != nil {
				t.Fatalf("reconcileMonitoring() = %v, want nil", err)
			}

			gotError := sink.logged(sink.errorMsgs, errorMsg)
			gotInfo := sink.logged(sink.infoMsgs, infoMsg)

			if tc.wantErrorLog {
				if !gotError {
					t.Errorf("expected an Error-level log for an enabled resource, got errors=%v infos=%v",
						sink.errorMsgs, sink.infoMsgs)
				}
				if gotInfo {
					t.Errorf("an enabled resource must not be reported at Info: %v", sink.infoMsgs)
				}
				return
			}
			if !gotInfo {
				t.Errorf("expected an Info-level log for a disabled resource, got errors=%v infos=%v",
					sink.errorMsgs, sink.infoMsgs)
			}
			if gotError {
				t.Errorf("a disabled resource must not be reported at Error: %v", sink.errorMsgs)
			}
		})
	}
}

// TestMonitoringCleanupForbiddenSurvivesErrorWrap pins the coupling between the
// cleanup error wrap and the IsForbidden classification.
//
// The disabled path wraps the Get failure as
// "getting <Kind> %s for cleanup: %w", and reconcileMonitoring's
// errors.IsForbidden case only matches because apierrors.IsForbidden resolves
// through errors.As. If a refactor drops the %w — switching to %v, or building a
// fresh error — the 403 silently stops being recognised and the reconcile starts
// failing again, which is the original cluster-freeze bug. Nothing else in the
// suite would catch that, because reconcileMonitoring would still return an
// error for a reason the tests do not distinguish.
func TestMonitoringCleanupForbiddenSurvivesErrorWrap(t *testing.T) {
	tests := []struct {
		name    string
		call    func(r *AerospikeClusterReconciler, cluster *ackov1alpha1.AerospikeCluster) error
		wantMsg string
	}{
		{
			name: "ServiceMonitor cleanup",
			call: func(r *AerospikeClusterReconciler, c *ackov1alpha1.AerospikeCluster) error {
				return r.reconcileServiceMonitor(context.Background(), c, false)
			},
			wantMsg: "getting ServiceMonitor",
		},
		{
			name: "PrometheusRule cleanup",
			call: func(r *AerospikeClusterReconciler, c *ackov1alpha1.AerospikeCluster) error {
				return r.reconcilePrometheusRule(context.Background(), c, false)
			},
			wantMsg: "getting PrometheusRule",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			scheme := rollingRestartScheme(t)

			cluster := &ackov1alpha1.AerospikeCluster{}
			cluster.Name = monitoringTestCluster
			cluster.Namespace = ctrlTestNamespace

			r := &AerospikeClusterReconciler{
				Client: monitoringGetErrClient(scheme, forbiddenMonitoringErr),
				Scheme: scheme,
			}

			err := tc.call(r, cluster)
			if err == nil {
				t.Fatal("expected the cleanup Get error to be returned to the caller")
			}
			// The wrap must still be there ...
			if !strings.Contains(err.Error(), tc.wantMsg) || !strings.Contains(err.Error(), "for cleanup") {
				t.Errorf("expected the wrapped cleanup error, got: %v", err)
			}
			// ... and IsForbidden must still see through it.
			if !apierrors.IsForbidden(err) {
				t.Errorf("apierrors.IsForbidden must match through the %%w wrap, got: %v", err)
			}
		})
	}
}
