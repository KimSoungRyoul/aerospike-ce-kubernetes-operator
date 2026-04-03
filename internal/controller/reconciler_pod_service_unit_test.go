package controller

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestPodServiceNeedsUpdate(t *testing.T) {
	desiredLabels := map[string]string{
		"app.kubernetes.io/name": "aerospike",
		"acko.io/pod-service":    "aerospike-0",
	}
	desiredAnnotations := map[string]string{"example.com/env": "dev"}
	desiredSelector := map[string]string{"statefulset.kubernetes.io/pod-name": "aerospike-0"}
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
			Type:     corev1.ServiceTypeClusterIP,
			Selector: desiredSelector,
			Ports:    desiredPorts,
		},
	}

	tests := []struct {
		name     string
		mutate   func(svc *corev1.Service)
		expected bool
	}{
		{
			name:     "no-change",
			mutate:   func(_ *corev1.Service) {},
			expected: false,
		},
		{
			name: "selector-changed",
			mutate: func(svc *corev1.Service) {
				svc.Spec.Selector = map[string]string{"statefulset.kubernetes.io/pod-name": "aerospike-1"}
			},
			expected: true,
		},
		{
			name: "type-changed",
			mutate: func(svc *corev1.Service) {
				svc.Spec.Type = corev1.ServiceTypeNodePort
			},
			expected: true,
		},
		{
			name: "ports-changed",
			mutate: func(svc *corev1.Service) {
				svc.Spec.Ports[0].Port = 4000
			},
			expected: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := base.DeepCopy()
			tc.mutate(svc)
			got := podServiceNeedsUpdate(svc, desiredLabels, desiredAnnotations, desiredSelector, desiredPorts, corev1.ServiceTypeClusterIP)
			if got != tc.expected {
				t.Fatalf("podServiceNeedsUpdate()=%v, want %v", got, tc.expected)
			}
		})
	}
}
