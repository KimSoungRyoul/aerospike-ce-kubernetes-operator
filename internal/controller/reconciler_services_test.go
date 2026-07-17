package controller

import (
	"maps"
	"testing"

	"github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/internal/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestServicePortsChanged(t *testing.T) {
	tests := []struct {
		name     string
		existing []corev1.ServicePort
		desired  []corev1.ServicePort
		want     bool
	}{
		{
			name:     "both empty",
			existing: nil,
			desired:  nil,
			want:     false,
		},
		{
			name:     "same single port",
			existing: []corev1.ServicePort{{Name: "service", Port: 3000}},
			desired:  []corev1.ServicePort{{Name: "service", Port: 3000}},
			want:     false,
		},
		{
			name:     "different lengths",
			existing: []corev1.ServicePort{{Name: "service", Port: 3000}},
			desired: []corev1.ServicePort{
				{Name: "service", Port: 3000},
				{Name: "fabric", Port: 3001},
			},
			want: true,
		},
		{
			name:     "different port name",
			existing: []corev1.ServicePort{{Name: "service", Port: 3000}},
			desired:  []corev1.ServicePort{{Name: "fabric", Port: 3000}},
			want:     true,
		},
		{
			name:     "different port number",
			existing: []corev1.ServicePort{{Name: "service", Port: 3000}},
			desired:  []corev1.ServicePort{{Name: "service", Port: 4000}},
			want:     true,
		},
		{
			name: "same multiple ports",
			existing: []corev1.ServicePort{
				{Name: "service", Port: 3000},
				{Name: "fabric", Port: 3001},
			},
			desired: []corev1.ServicePort{
				{Name: "service", Port: 3000},
				{Name: "fabric", Port: 3001},
			},
			want: false,
		},
		{
			name: "different TargetPort",
			existing: []corev1.ServicePort{
				{Name: "service", Port: 3000, TargetPort: intstr.FromInt32(3000), Protocol: corev1.ProtocolTCP},
			},
			desired: []corev1.ServicePort{
				{Name: "service", Port: 3000, TargetPort: intstr.FromInt32(4000), Protocol: corev1.ProtocolTCP},
			},
			want: true,
		},
		{
			name: "different Protocol",
			existing: []corev1.ServicePort{
				{Name: "service", Port: 3000, TargetPort: intstr.FromInt32(3000), Protocol: corev1.ProtocolTCP},
			},
			desired: []corev1.ServicePort{
				{Name: "service", Port: 3000, TargetPort: intstr.FromInt32(3000), Protocol: corev1.ProtocolUDP},
			},
			want: true,
		},
		{
			name: "same ports with all fields",
			existing: []corev1.ServicePort{
				{Name: "service", Port: 3000, TargetPort: intstr.FromInt32(3000), Protocol: corev1.ProtocolTCP},
			},
			desired: []corev1.ServicePort{
				{Name: "service", Port: 3000, TargetPort: intstr.FromInt32(3000), Protocol: corev1.ProtocolTCP},
			},
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := servicePortsChanged(tc.existing, tc.desired)
			if got != tc.want {
				t.Errorf("servicePortsChanged() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestEqualAnnotations(t *testing.T) {
	tests := []struct {
		name     string
		actual   map[string]string
		desired  map[string]string
		expected bool
	}{
		{
			name:     "both nil",
			actual:   nil,
			desired:  nil,
			expected: true,
		},
		{
			name:     "desired nil, actual empty",
			actual:   map[string]string{},
			desired:  nil,
			expected: true,
		},
		{
			name:     "actual nil, desired empty map",
			actual:   nil,
			desired:  map[string]string{},
			expected: true,
		},
		{
			name:     "desired nil, actual has system annotation only",
			actual:   map[string]string{"kubectl.kubernetes.io/last-applied-configuration": "{}"},
			desired:  nil,
			expected: true,
		},
		{
			name:     "desired matches actual",
			actual:   map[string]string{"foo": "bar"},
			desired:  map[string]string{"foo": "bar"},
			expected: true,
		},
		{
			name:     "desired matches actual with system annotation preserved",
			actual:   map[string]string{"foo": "bar", "kubectl.kubernetes.io/last-applied-configuration": "{}"},
			desired:  map[string]string{"foo": "bar"},
			expected: true,
		},
		{
			name:     "desired adds new annotation",
			actual:   map[string]string{"foo": "bar"},
			desired:  map[string]string{"foo": "bar", "baz": "qux"},
			expected: false,
		},
		{
			name:     "desired updates value",
			actual:   map[string]string{"foo": "bar"},
			desired:  map[string]string{"foo": "new-value"},
			expected: false,
		},
		{
			name:     "desired removes annotation",
			actual:   map[string]string{"foo": "bar", "old": "stale"},
			desired:  map[string]string{"foo": "bar"},
			expected: false,
		},
		{
			name:     "desired removes all annotations",
			actual:   map[string]string{"foo": "bar"},
			desired:  nil,
			expected: false,
		},
		{
			name:     "desired nil, actual has system + operator annotation",
			actual:   map[string]string{"foo": "bar", "kubectl.kubernetes.io/last-applied-configuration": "{}"},
			desired:  nil,
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := equalAnnotations(tc.actual, tc.desired)
			if got != tc.expected {
				t.Errorf("equalAnnotations(%v, %v) = %v, want %v", tc.actual, tc.desired, got, tc.expected)
			}
		})
	}
}

func TestReconcileAnnotations(t *testing.T) {
	tests := []struct {
		name     string
		existing map[string]string
		desired  map[string]string
		expected map[string]string
	}{
		{
			name:     "both nil",
			existing: nil,
			desired:  nil,
			expected: nil,
		},
		{
			name:     "desired only",
			existing: nil,
			desired:  map[string]string{"foo": "bar"},
			expected: map[string]string{"foo": "bar"},
		},
		{
			name:     "existing only (non-system) — cleaned up",
			existing: map[string]string{"foo": "bar"},
			desired:  nil,
			expected: nil,
		},
		{
			name:     "existing empty map, desired nil",
			existing: map[string]string{},
			desired:  nil,
			expected: nil,
		},
		{
			name:     "existing system annotation preserved when desired is nil",
			existing: map[string]string{"kubectl.kubernetes.io/last-applied-configuration": "{}"},
			desired:  nil,
			expected: map[string]string{"kubectl.kubernetes.io/last-applied-configuration": "{}"},
		},
		{
			name:     "merge preserves system, applies desired",
			existing: map[string]string{"old": "stale", "kubectl.kubernetes.io/last-applied-configuration": "{}"},
			desired:  map[string]string{"new": "value"},
			expected: map[string]string{"new": "value", "kubectl.kubernetes.io/last-applied-configuration": "{}"},
		},
		{
			name:     "desired updates existing value",
			existing: map[string]string{"foo": "old"},
			desired:  map[string]string{"foo": "new"},
			expected: map[string]string{"foo": "new"},
		},
		{
			name:     "k8s.io system annotations preserved",
			existing: map[string]string{"app.k8s.io/managed-by": "helm", "user-key": "val"},
			desired:  map[string]string{"new-key": "new-val"},
			expected: map[string]string{"app.k8s.io/managed-by": "helm", "new-key": "new-val"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := reconcileAnnotations(tc.existing, tc.desired)
			if !maps.Equal(got, tc.expected) {
				t.Errorf("reconcileAnnotations(%v, %v) = %v, want %v", tc.existing, tc.desired, got, tc.expected)
			}
		})
	}
}

func TestIsSystemAnnotation(t *testing.T) {
	tests := []struct {
		key      string
		expected bool
	}{
		{"kubectl.kubernetes.io/last-applied-configuration", true},
		{"kubernetes.io/ingress-class", true},
		{"app.kubernetes.io/managed-by", true},
		{"app.k8s.io/managed-by", true},
		{"k8s.io/some-key", true},
		{"example.com/env", false},
		{"prometheus.io/scrape", false},
		{"acko.io/managed", false},
		{"foo", false},
		// Edge cases: substring matching should NOT match these
		{"bypass-kubernetes.io/hack", false},
		{"notkubernetes.io/key", false},
		{"notk8s.io/key", false},
		{"fakek8s.io/something", false},
		// No domain prefix (bare key)
		{"kubernetes.io", false},
	}

	for _, tc := range tests {
		t.Run(tc.key, func(t *testing.T) {
			got := isSystemAnnotation(tc.key)
			if got != tc.expected {
				t.Errorf("isSystemAnnotation(%q) = %v, want %v", tc.key, got, tc.expected)
			}
		})
	}
}

func TestMergeAdditionalLabels(t *testing.T) {
	base := map[string]string{
		utils.AppLabel:       "aerospike-cluster",
		utils.InstanceLabel:  "demo",
		utils.ManagedByLabel: "aerospike-ce-kubernetes-operator",
		podServiceLabel:      "demo-0",
	}

	additional := map[string]string{
		utils.AppLabel:      "user-value",
		utils.InstanceLabel: "other",
		podServiceLabel:     "other-pod",
		"custom":            "value",
	}

	got := mergeAdditionalLabels(maps.Clone(base), additional)

	if got[utils.AppLabel] != base[utils.AppLabel] {
		t.Fatalf("AppLabel overwritten: got %q, want %q", got[utils.AppLabel], base[utils.AppLabel])
	}
	if got[utils.InstanceLabel] != base[utils.InstanceLabel] {
		t.Fatalf("InstanceLabel overwritten: got %q, want %q", got[utils.InstanceLabel], base[utils.InstanceLabel])
	}
	if got[podServiceLabel] != base[podServiceLabel] {
		t.Fatalf("podServiceLabel overwritten: got %q, want %q", got[podServiceLabel], base[podServiceLabel])
	}
	if got["custom"] != "value" {
		t.Fatalf("custom label missing: got %q", got["custom"])
	}
}

func TestHeadlessServiceNeedsUpdate(t *testing.T) {
	desiredLabels := map[string]string{
		"app.kubernetes.io/name":     "aerospike-cluster",
		"app.kubernetes.io/instance": "demo",
	}
	desiredAnnotations := map[string]string{
		"example.com/env": "dev",
	}
	desiredSelector := map[string]string{
		"app.kubernetes.io/name":     "aerospike-cluster",
		"app.kubernetes.io/instance": "demo",
	}
	desiredPorts := []corev1.ServicePort{
		{
			Name:       "service",
			Port:       3000,
			TargetPort: intstr.FromInt32(3000),
			Protocol:   corev1.ProtocolTCP,
		},
	}

	base := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      desiredLabels,
			Annotations: desiredAnnotations,
		},
		Spec: corev1.ServiceSpec{
			Selector:                 desiredSelector,
			PublishNotReadyAddresses: true,
			Ports:                    desiredPorts,
		},
	}

	tests := []struct {
		name     string
		mutate   func(*corev1.Service)
		expected bool
	}{
		{
			name:     "unchanged",
			mutate:   func(_ *corev1.Service) {},
			expected: false,
		},
		{
			name: "selector drift",
			mutate: func(svc *corev1.Service) {
				svc.Spec.Selector = map[string]string{"app.kubernetes.io/instance": "other"}
			},
			expected: true,
		},
		{
			name: "publish not ready drift",
			mutate: func(svc *corev1.Service) {
				svc.Spec.PublishNotReadyAddresses = false
			},
			expected: true,
		},
		{
			name: "port drift",
			mutate: func(svc *corev1.Service) {
				svc.Spec.Ports[0].Port = 4000
			},
			expected: true,
		},
		{
			name: "label drift",
			mutate: func(svc *corev1.Service) {
				svc.Labels["custom"] = "stale"
			},
			expected: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := base.DeepCopy()
			tc.mutate(svc)
			if got := headlessServiceNeedsUpdate(svc, desiredLabels, desiredAnnotations, desiredSelector, desiredPorts); got != tc.expected {
				t.Fatalf("headlessServiceNeedsUpdate() = %v, want %v", got, tc.expected)
			}
		})
	}
}
