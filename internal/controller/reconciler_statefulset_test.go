package controller

import (
	"maps"
	"testing"

	ackov1alpha1 "github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/api/v1alpha1"
)

// --- computePodSpecHash tests ---

func TestComputePodSpecHash_Deterministic(t *testing.T) {
	cluster := &ackov1alpha1.AerospikeCluster{
		Spec: ackov1alpha1.AerospikeClusterSpec{
			Image: "aerospike:ce-8.1.1.1",
		},
	}
	rack := &ackov1alpha1.Rack{ID: 0}

	hash1 := computePodSpecHash(cluster, rack)
	hash2 := computePodSpecHash(cluster, rack)

	if hash1 != hash2 {
		t.Errorf("hash should be deterministic: %q != %q", hash1, hash2)
	}
}

func TestComputePodSpecHash_ChangesWithImage(t *testing.T) {
	cluster1 := &ackov1alpha1.AerospikeCluster{
		Spec: ackov1alpha1.AerospikeClusterSpec{
			Image: "aerospike:ce-8.1.1.1",
		},
	}
	cluster2 := &ackov1alpha1.AerospikeCluster{
		Spec: ackov1alpha1.AerospikeClusterSpec{
			Image: "aerospike:ce-8.2.0.0",
		},
	}
	rack := &ackov1alpha1.Rack{ID: 0}

	hash1 := computePodSpecHash(cluster1, rack)
	hash2 := computePodSpecHash(cluster2, rack)

	if hash1 == hash2 {
		t.Error("hash should change when image changes")
	}
}

func TestComputePodSpecHash_ChangesWithRackID(t *testing.T) {
	cluster := &ackov1alpha1.AerospikeCluster{
		Spec: ackov1alpha1.AerospikeClusterSpec{
			Image: "aerospike:ce-8.1.1.1",
		},
	}

	hash1 := computePodSpecHash(cluster, &ackov1alpha1.Rack{ID: 0})
	hash2 := computePodSpecHash(cluster, &ackov1alpha1.Rack{ID: 1})

	if hash1 == hash2 {
		t.Error("hash should change with different rack IDs")
	}
}

func TestComputePodSpecHash_ChangesWithPodSpec(t *testing.T) {
	cluster1 := &ackov1alpha1.AerospikeCluster{
		Spec: ackov1alpha1.AerospikeClusterSpec{
			Image: "aerospike:ce-8.1.1.1",
		},
	}
	cluster2 := &ackov1alpha1.AerospikeCluster{
		Spec: ackov1alpha1.AerospikeClusterSpec{
			Image: "aerospike:ce-8.1.1.1",
			PodSpec: &ackov1alpha1.AerospikePodSpec{
				HostNetwork: true,
			},
		},
	}
	rack := &ackov1alpha1.Rack{ID: 0}

	hash1 := computePodSpecHash(cluster1, rack)
	hash2 := computePodSpecHash(cluster2, rack)

	if hash1 == hash2 {
		t.Error("hash should change when podSpec changes")
	}
}

func TestComputePodSpecHash_ChangesWithMonitoring(t *testing.T) {
	cluster1 := &ackov1alpha1.AerospikeCluster{
		Spec: ackov1alpha1.AerospikeClusterSpec{
			Image: "aerospike:ce-8.1.1.1",
		},
	}
	cluster2 := &ackov1alpha1.AerospikeCluster{
		Spec: ackov1alpha1.AerospikeClusterSpec{
			Image: "aerospike:ce-8.1.1.1",
			Monitoring: &ackov1alpha1.AerospikeMonitoringSpec{
				Enabled:       true,
				ExporterImage: "exporter:v1",
				Port:          9145,
			},
		},
	}
	rack := &ackov1alpha1.Rack{ID: 0}

	hash1 := computePodSpecHash(cluster1, rack)
	hash2 := computePodSpecHash(cluster2, rack)

	if hash1 == hash2 {
		t.Error("hash should change when monitoring config changes")
	}
}

func TestComputePodSpecHash_ChangesWithAerospikeNetworkPolicy(t *testing.T) {
	cluster1 := &ackov1alpha1.AerospikeCluster{
		Spec: ackov1alpha1.AerospikeClusterSpec{
			Image: "aerospike:ce-8.1.1.1",
		},
	}
	cluster2 := &ackov1alpha1.AerospikeCluster{
		Spec: ackov1alpha1.AerospikeClusterSpec{
			Image: "aerospike:ce-8.1.1.1",
			AerospikeNetworkPolicy: &ackov1alpha1.AerospikeNetworkPolicy{
				AccessType: ackov1alpha1.AerospikeNetworkTypeHostExternal,
			},
		},
	}
	rack := &ackov1alpha1.Rack{ID: 0}

	hash1 := computePodSpecHash(cluster1, rack)
	hash2 := computePodSpecHash(cluster2, rack)

	if hash1 == hash2 {
		t.Error("hash should change when aerospikeNetworkPolicy changes")
	}
}

func TestComputePodSpecHash_ChangesWithNetworkPolicyAccessType(t *testing.T) {
	cluster1 := &ackov1alpha1.AerospikeCluster{
		Spec: ackov1alpha1.AerospikeClusterSpec{
			Image: "aerospike:ce-8.1.1.1",
			AerospikeNetworkPolicy: &ackov1alpha1.AerospikeNetworkPolicy{
				AccessType: ackov1alpha1.AerospikeNetworkTypePod,
			},
		},
	}
	cluster2 := &ackov1alpha1.AerospikeCluster{
		Spec: ackov1alpha1.AerospikeClusterSpec{
			Image: "aerospike:ce-8.1.1.1",
			AerospikeNetworkPolicy: &ackov1alpha1.AerospikeNetworkPolicy{
				AccessType: ackov1alpha1.AerospikeNetworkTypeHostExternal,
			},
		},
	}
	rack := &ackov1alpha1.Rack{ID: 0}

	hash1 := computePodSpecHash(cluster1, rack)
	hash2 := computePodSpecHash(cluster2, rack)

	if hash1 == hash2 {
		t.Error("hash should change when aerospikeNetworkPolicy.accessType changes")
	}
}

func TestComputePodSpecHash_ChangesWithPodService(t *testing.T) {
	cluster1 := &ackov1alpha1.AerospikeCluster{
		Spec: ackov1alpha1.AerospikeClusterSpec{
			Image: "aerospike:ce-8.1.1.1",
		},
	}
	cluster2 := &ackov1alpha1.AerospikeCluster{
		Spec: ackov1alpha1.AerospikeClusterSpec{
			Image: "aerospike:ce-8.1.1.1",
			PodService: &ackov1alpha1.AerospikeServiceSpec{
				ServiceType: "NodePort",
			},
		},
	}
	rack := &ackov1alpha1.Rack{ID: 0}

	hash1 := computePodSpecHash(cluster1, rack)
	hash2 := computePodSpecHash(cluster2, rack)

	if hash1 == hash2 {
		t.Error("hash should change when podService changes")
	}
}

func TestComputePodSpecHash_ChangesWithPodServiceType(t *testing.T) {
	cluster1 := &ackov1alpha1.AerospikeCluster{
		Spec: ackov1alpha1.AerospikeClusterSpec{
			Image: "aerospike:ce-8.1.1.1",
			PodService: &ackov1alpha1.AerospikeServiceSpec{
				ServiceType: "ClusterIP",
			},
		},
	}
	cluster2 := &ackov1alpha1.AerospikeCluster{
		Spec: ackov1alpha1.AerospikeClusterSpec{
			Image: "aerospike:ce-8.1.1.1",
			PodService: &ackov1alpha1.AerospikeServiceSpec{
				ServiceType: "LoadBalancer",
			},
		},
	}
	rack := &ackov1alpha1.Rack{ID: 0}

	hash1 := computePodSpecHash(cluster1, rack)
	hash2 := computePodSpecHash(cluster2, rack)

	if hash1 == hash2 {
		t.Error("hash should change when podService.serviceType changes")
	}
}

func TestComputePodSpecHash_SameWithIdenticalNetworkAndPodService(t *testing.T) {
	mk := func() *ackov1alpha1.AerospikeCluster {
		return &ackov1alpha1.AerospikeCluster{
			Spec: ackov1alpha1.AerospikeClusterSpec{
				Image: "aerospike:ce-8.1.1.1",
				AerospikeNetworkPolicy: &ackov1alpha1.AerospikeNetworkPolicy{
					AccessType: ackov1alpha1.AerospikeNetworkTypeHostExternal,
				},
				PodService: &ackov1alpha1.AerospikeServiceSpec{
					ServiceType: "NodePort",
				},
			},
		}
	}
	rack := &ackov1alpha1.Rack{ID: 0}

	hash1 := computePodSpecHash(mk(), rack)
	hash2 := computePodSpecHash(mk(), rack)

	if hash1 != hash2 {
		t.Errorf("hash should be identical for identical podService/aerospikeNetworkPolicy: %q != %q", hash1, hash2)
	}
}

func TestComputePodSpecHash_NilNetworkAndPodServiceStable(t *testing.T) {
	// A cluster with nil PodService / AerospikeNetworkPolicy must hash stably
	// and identically across calls (nil pointers are omitted from the JSON).
	cluster := &ackov1alpha1.AerospikeCluster{
		Spec: ackov1alpha1.AerospikeClusterSpec{
			Image: "aerospike:ce-8.1.1.1",
		},
	}
	rack := &ackov1alpha1.Rack{ID: 0}

	hash1 := computePodSpecHash(cluster, rack)
	hash2 := computePodSpecHash(cluster, rack)

	if hash1 != hash2 {
		t.Errorf("nil podService/aerospikeNetworkPolicy should hash stably: %q != %q", hash1, hash2)
	}
}

func TestComputePodSpecHash_SameWithDifferentConfig(t *testing.T) {
	// PodSpecHash should NOT change when only aerospikeConfig changes
	// (that's what configHash is for)
	cluster1 := &ackov1alpha1.AerospikeCluster{
		Spec: ackov1alpha1.AerospikeClusterSpec{
			Image: "aerospike:ce-8.1.1.1",
			AerospikeConfig: &ackov1alpha1.AerospikeConfigSpec{
				Value: map[string]any{"service": map[string]any{"proto-fd-max": 15000}},
			},
		},
	}
	cluster2 := &ackov1alpha1.AerospikeCluster{
		Spec: ackov1alpha1.AerospikeClusterSpec{
			Image: "aerospike:ce-8.1.1.1",
			AerospikeConfig: &ackov1alpha1.AerospikeConfigSpec{
				Value: map[string]any{"service": map[string]any{"proto-fd-max": 20000}},
			},
		},
	}
	rack := &ackov1alpha1.Rack{ID: 0}

	hash1 := computePodSpecHash(cluster1, rack)
	hash2 := computePodSpecHash(cluster2, rack)

	if hash1 != hash2 {
		t.Error("hash should NOT change when only aerospikeConfig changes (config-only change)")
	}
}

func TestComputePodSpecHash_Format(t *testing.T) {
	cluster := &ackov1alpha1.AerospikeCluster{
		Spec: ackov1alpha1.AerospikeClusterSpec{
			Image: "aerospike:ce-8.1.1.1",
		},
	}
	rack := &ackov1alpha1.Rack{ID: 0}

	hash := computePodSpecHash(cluster, rack)

	// Hash is first 8 bytes of SHA256, formatted as hex = 16 chars
	if len(hash) != 16 {
		t.Errorf("hash length = %d, want 16 (hex of 8 bytes)", len(hash))
	}
}

// --- configHash tests ---

func TestConfigHash_Deterministic(t *testing.T) {
	config := &ackov1alpha1.AerospikeConfigSpec{
		Value: map[string]any{"service": map[string]any{"proto-fd-max": 15000}},
	}

	h1 := configHash(config)
	h2 := configHash(config)
	if h1 != h2 {
		t.Errorf("configHash should be deterministic: %q != %q", h1, h2)
	}
}

func TestConfigHash_NilReturnsEmpty(t *testing.T) {
	h := configHash(nil)
	if h != "" {
		t.Errorf("configHash(nil) = %q, want empty string", h)
	}
}

func TestConfigHash_DifferentConfigs(t *testing.T) {
	config1 := &ackov1alpha1.AerospikeConfigSpec{
		Value: map[string]any{"service": map[string]any{"proto-fd-max": 15000}},
	}
	config2 := &ackov1alpha1.AerospikeConfigSpec{
		Value: map[string]any{"service": map[string]any{"proto-fd-max": 20000}},
	}

	h1 := configHash(config1)
	h2 := configHash(config2)
	if h1 == h2 {
		t.Error("different configs should produce different hashes")
	}
}

// --- mapsEqual tests ---

func TestMapsEqual_BothEmpty(t *testing.T) {
	if !maps.Equal(map[string]string{}, map[string]string{}) {
		t.Error("empty maps should be equal")
	}
}

func TestMapsEqual_Same(t *testing.T) {
	a := map[string]string{"k1": "v1", "k2": "v2"}
	b := map[string]string{"k1": "v1", "k2": "v2"}
	if !maps.Equal(a, b) {
		t.Error("identical maps should be equal")
	}
}

func TestMapsEqual_DifferentValues(t *testing.T) {
	a := map[string]string{"k1": "v1"}
	b := map[string]string{"k1": "v2"}
	if maps.Equal(a, b) {
		t.Error("maps with different values should not be equal")
	}
}

func TestMapsEqual_DifferentKeys(t *testing.T) {
	a := map[string]string{"k1": "v1"}
	b := map[string]string{"k2": "v1"}
	if maps.Equal(a, b) {
		t.Error("maps with different keys should not be equal")
	}
}

func TestMapsEqual_DifferentLengths(t *testing.T) {
	a := map[string]string{"k1": "v1"}
	b := map[string]string{"k1": "v1", "k2": "v2"}
	if maps.Equal(a, b) {
		t.Error("maps with different lengths should not be equal")
	}
}

// TestComputePodSpecHash_ChangesWithK8sNodeBlockList pins that editing
// spec.k8sNodeBlockList rolls the rack.
//
// The block list renders into the pod template's node affinity, but node
// affinity is IgnoredDuringExecution: it only decides where a pod is PLACED.
// Unhashed, adding a node to the list would leave needsUpdate false in
// reconcileStatefulSet, the template would never be patched, no pod would be
// recreated, and the evacuation would silently do nothing — which is how #344
// started. Fails without K8sNodeBlockList in computePodSpecHash.
func TestComputePodSpecHash_ChangesWithK8sNodeBlockList(t *testing.T) {
	rack := &ackov1alpha1.Rack{ID: 0}
	base := func(blockList []string) *ackov1alpha1.AerospikeCluster {
		return &ackov1alpha1.AerospikeCluster{
			Spec: ackov1alpha1.AerospikeClusterSpec{
				Image:            "aerospike:ce-8.1.1.1",
				K8sNodeBlockList: blockList,
			},
		}
	}

	empty := computePodSpecHash(base(nil), rack)
	oneNode := computePodSpecHash(base([]string{"node-a"}), rack)
	twoNodes := computePodSpecHash(base([]string{"node-a", "node-b"}), rack)
	differentNode := computePodSpecHash(base([]string{"node-b"}), rack)

	if empty == oneNode {
		t.Error("hash should change when a node is added to an empty k8sNodeBlockList")
	}
	if oneNode == twoNodes {
		t.Error("hash should change when a second node is added to k8sNodeBlockList")
	}
	if oneNode == differentNode {
		t.Error("hash should change when the blocked node changes")
	}
	if got := computePodSpecHash(base([]string{"node-a"}), rack); got != oneNode {
		t.Errorf("hash should stay stable for an unchanged block list: %q != %q", got, oneNode)
	}
}
