package controller

import (
	"context"
	"errors"
	"strings"
	"testing"

	ackov1alpha1 "github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/api/v1alpha1"
	"github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/internal/podutil"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestBuildK8sNetworkPolicy_BasicPorts(t *testing.T) {
	r := &AerospikeClusterReconciler{}
	cluster := &ackov1alpha1.AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: ackov1alpha1.AerospikeClusterSpec{
			Size: 3,
		},
	}

	np := r.buildK8sNetworkPolicy(cluster, "test-cluster-np")

	if np.Name != "test-cluster-np" {
		t.Errorf("expected name test-cluster-np, got %s", np.Name)
	}
	if np.Namespace != "default" {
		t.Errorf("expected namespace default, got %s", np.Namespace)
	}

	// Should have 2 ingress rules: intra-cluster (fabric+heartbeat) + client (service)
	if len(np.Spec.Ingress) != 2 {
		t.Fatalf("expected 2 ingress rules, got %d", len(np.Spec.Ingress))
	}

	// First rule: fabric + heartbeat (intra-cluster)
	intraCluster := np.Spec.Ingress[0]
	if len(intraCluster.Ports) != 2 {
		t.Fatalf("expected 2 intra-cluster ports, got %d", len(intraCluster.Ports))
	}
	if intraCluster.Ports[0].Port.IntVal != podutil.FabricPort {
		t.Errorf("expected fabric port %d, got %d", podutil.FabricPort, intraCluster.Ports[0].Port.IntVal)
	}
	if intraCluster.Ports[1].Port.IntVal != podutil.HeartbeatPort {
		t.Errorf("expected heartbeat port %d, got %d", podutil.HeartbeatPort, intraCluster.Ports[1].Port.IntVal)
	}

	// First rule should restrict to same-cluster pods
	if len(intraCluster.From) != 1 {
		t.Fatalf("expected 1 from selector, got %d", len(intraCluster.From))
	}
	if intraCluster.From[0].PodSelector == nil {
		t.Fatal("expected pod selector in from")
	}

	// Second rule: service port (client access)
	clientRule := np.Spec.Ingress[1]
	if len(clientRule.Ports) != 1 {
		t.Fatalf("expected 1 client port, got %d", len(clientRule.Ports))
	}
	if clientRule.Ports[0].Port.IntVal != podutil.ServicePort {
		t.Errorf("expected service port %d, got %d", podutil.ServicePort, clientRule.Ports[0].Port.IntVal)
	}
	// Client rule should be open (no From restriction)
	if len(clientRule.From) != 0 {
		t.Errorf("expected open client rule, got %d from selectors", len(clientRule.From))
	}
}

func TestBuildK8sNetworkPolicy_WithMonitoring(t *testing.T) {
	r := &AerospikeClusterReconciler{}
	cluster := &ackov1alpha1.AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: ackov1alpha1.AerospikeClusterSpec{
			Size: 3,
			Monitoring: &ackov1alpha1.AerospikeMonitoringSpec{
				Enabled: true,
				Port:    9145,
			},
		},
	}

	np := r.buildK8sNetworkPolicy(cluster, "test-cluster-np")

	// Should have 3 ingress rules: intra-cluster + client + metrics
	if len(np.Spec.Ingress) != 3 {
		t.Fatalf("expected 3 ingress rules with monitoring, got %d", len(np.Spec.Ingress))
	}

	metricsRule := np.Spec.Ingress[2]
	if len(metricsRule.Ports) != 1 {
		t.Fatalf("expected 1 metrics port, got %d", len(metricsRule.Ports))
	}
	if metricsRule.Ports[0].Port.IntVal != 9145 {
		t.Errorf("expected metrics port 9145, got %d", metricsRule.Ports[0].Port.IntVal)
	}
}

func TestBuildK8sNetworkPolicy_WithoutMonitoring(t *testing.T) {
	r := &AerospikeClusterReconciler{}
	cluster := &ackov1alpha1.AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: ackov1alpha1.AerospikeClusterSpec{
			Size: 3,
			Monitoring: &ackov1alpha1.AerospikeMonitoringSpec{
				Enabled: false,
				Port:    9145,
			},
		},
	}

	np := r.buildK8sNetworkPolicy(cluster, "test-cluster-np")

	// Should have only 2 ingress rules when monitoring is disabled
	if len(np.Spec.Ingress) != 2 {
		t.Fatalf("expected 2 ingress rules without monitoring, got %d", len(np.Spec.Ingress))
	}
}

func TestBuildK8sNetworkPolicy_Labels(t *testing.T) {
	r := &AerospikeClusterReconciler{}
	cluster := &ackov1alpha1.AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-cluster",
			Namespace: "ns1",
		},
		Spec: ackov1alpha1.AerospikeClusterSpec{
			Size: 1,
		},
	}

	np := r.buildK8sNetworkPolicy(cluster, "my-cluster-np")

	if np.Labels["app.kubernetes.io/instance"] != "my-cluster" {
		t.Errorf("expected instance label my-cluster, got %s", np.Labels["app.kubernetes.io/instance"])
	}

	// PodSelector should use selector labels
	if np.Spec.PodSelector.MatchLabels["app.kubernetes.io/instance"] != "my-cluster" {
		t.Error("expected pod selector to match cluster name")
	}
}

func TestBuildK8sNetworkPolicy_PolicyTypes(t *testing.T) {
	r := &AerospikeClusterReconciler{}
	cluster := &ackov1alpha1.AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: ackov1alpha1.AerospikeClusterSpec{
			Size: 1,
		},
	}

	np := r.buildK8sNetworkPolicy(cluster, "test-cluster-np")

	if len(np.Spec.PolicyTypes) != 1 {
		t.Fatalf("expected 1 policy type, got %d", len(np.Spec.PolicyTypes))
	}
	if np.Spec.PolicyTypes[0] != "Ingress" {
		t.Errorf("expected Ingress policy type, got %s", np.Spec.PolicyTypes[0])
	}
}

func TestK8sNetworkPolicyChanged_Unchanged(t *testing.T) {
	r := &AerospikeClusterReconciler{}
	cluster := &ackov1alpha1.AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: ackov1alpha1.AerospikeClusterSpec{
			Size: 1,
		},
	}

	desired := r.buildK8sNetworkPolicy(cluster, "test-cluster-np")
	existing := desired.DeepCopy()

	if changed := k8sNetworkPolicyChanged(existing, desired); changed {
		t.Fatal("k8sNetworkPolicyChanged() = true, want false for identical policy")
	}
}

func TestK8sNetworkPolicyChanged_SpecChanged(t *testing.T) {
	r := &AerospikeClusterReconciler{}
	cluster := &ackov1alpha1.AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: ackov1alpha1.AerospikeClusterSpec{
			Size: 1,
		},
	}

	desired := r.buildK8sNetworkPolicy(cluster, "test-cluster-np")
	existing := desired.DeepCopy()
	existing.Spec.PolicyTypes = append(existing.Spec.PolicyTypes, networkingv1.PolicyTypeEgress)

	if changed := k8sNetworkPolicyChanged(existing, desired); !changed {
		t.Fatal("k8sNetworkPolicyChanged() = false, want true when spec differs")
	}
}

func TestK8sNetworkPolicyChanged_LabelsChanged(t *testing.T) {
	r := &AerospikeClusterReconciler{}
	cluster := &ackov1alpha1.AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: ackov1alpha1.AerospikeClusterSpec{
			Size: 1,
		},
	}

	desired := r.buildK8sNetworkPolicy(cluster, "test-cluster-np")
	existing := desired.DeepCopy()
	existing.Labels["custom"] = "different"

	if changed := k8sNetworkPolicyChanged(existing, desired); !changed {
		t.Fatal("k8sNetworkPolicyChanged() = false, want true when labels differ")
	}
}

// TestReconcileNetworkPolicy_CiliumForbiddenDegrades covers the same 403 funnel
// as the monitoring resources. reconcileNetworkPolicy's error reaches
// reconcileCluster -> handleReconcileError -> the circuit breaker
// (reconciler.go:609), so a denied CiliumNetworkPolicy Get would freeze scale,
// rolling restart, config and ACL reconciliation for the whole cluster over an
// optional network-policy integration. The chart does grant cilium.io, so the
// exposure is narrower than the missing prometheusrules grant — but an RBAC-only
// install, a policy-engine denial or a ResourceQuota all reach it.
func TestReconcileNetworkPolicy_CiliumForbiddenDegrades(t *testing.T) {
	scheme := rollingRestartScheme(t)

	cluster := &ackov1alpha1.AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: ctrlTestNamespace},
		Spec: ackov1alpha1.AerospikeClusterSpec{
			Size:  1,
			Image: "aerospike:ce-8.1.1.1",
			NetworkPolicyConfig: &ackov1alpha1.NetworkPolicyConfig{
				Enabled: true,
				Type:    ackov1alpha1.NetworkPolicyTypeCilium,
			},
		},
	}

	base := fake.NewClientBuilder().WithScheme(scheme).Build()
	denying := interceptor.NewClient(base, interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey,
			obj client.Object, opts ...client.GetOption) error {
			if u, ok := obj.(*unstructured.Unstructured); ok &&
				u.GroupVersionKind() == ciliumNetworkPolicyGVK {
				return apierrors.NewForbidden(
					schema.GroupResource{Group: "cilium.io", Resource: "ciliumnetworkpolicies"},
					key.Name, errors.New("access denied"))
			}
			return c.Get(ctx, key, obj, opts...)
		},
	})

	r := &AerospikeClusterReconciler{
		Client:   denying,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(8),
	}

	if err := r.reconcileNetworkPolicy(context.Background(), cluster); err != nil {
		t.Fatalf("reconcileNetworkPolicy() = %v, want nil: a 403 on an optional "+
			"CiliumNetworkPolicy must not fail the reconcile", err)
	}
}

// TestReconcileNetworkPolicy_CiliumOtherErrorStillFails guards the other side —
// only a 403 or an absent CRD may degrade; a transient API failure must surface.
func TestReconcileNetworkPolicy_CiliumOtherErrorStillFails(t *testing.T) {
	scheme := rollingRestartScheme(t)

	cluster := &ackov1alpha1.AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: ctrlTestNamespace},
		Spec: ackov1alpha1.AerospikeClusterSpec{
			Size:  1,
			Image: "aerospike:ce-8.1.1.1",
			NetworkPolicyConfig: &ackov1alpha1.NetworkPolicyConfig{
				Enabled: true,
				Type:    ackov1alpha1.NetworkPolicyTypeCilium,
			},
		},
	}

	base := fake.NewClientBuilder().WithScheme(scheme).Build()
	failing := interceptor.NewClient(base, interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey,
			obj client.Object, opts ...client.GetOption) error {
			if u, ok := obj.(*unstructured.Unstructured); ok &&
				u.GroupVersionKind() == ciliumNetworkPolicyGVK {
				return apierrors.NewInternalError(errors.New("etcd unavailable"))
			}
			return c.Get(ctx, key, obj, opts...)
		},
	})

	r := &AerospikeClusterReconciler{
		Client:   failing,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(8),
	}

	err := r.reconcileNetworkPolicy(context.Background(), cluster)
	if err == nil {
		t.Fatal("reconcileNetworkPolicy() = nil, want error for a non-403 API failure")
	}
	if !strings.Contains(err.Error(), "getting CiliumNetworkPolicy") {
		t.Errorf("expected the wrapped Get error, got: %v", err)
	}
}
