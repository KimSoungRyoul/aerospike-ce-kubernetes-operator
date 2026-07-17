package controller

import (
	"testing"

	ackov1alpha1 "github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/api/v1alpha1"
	"github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/internal/podutil"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
