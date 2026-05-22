package controller

import (
	"context"
	"errors"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	ackov1alpha1 "github.com/ksr/aerospike-ce-kubernetes-operator/api/v1alpha1"
)

// errMonitoringGetFailure is a sentinel non-NotFound error used to simulate an
// API failure during monitoring-resource cleanup.
var errMonitoringGetFailure = errors.New("simulated API server failure")

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
	cluster.Name = "demo"
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
