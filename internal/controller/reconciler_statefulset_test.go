package controller

import (
	"maps"
	"testing"

	corev1 "k8s.io/api/core/v1"

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

// TestComputeStorageHash_ChangesWithStorage pins that a storage edit rolls pods.
//
// spec.storage renders into the pod template via BuildVolumes — the inline
// volumes and every aerospike volumeMount come from it. Unhashed, a storage-only
// edit left needsUpdate false in reconcileStatefulSet, so the StatefulSet
// template was never patched and the edit was discarded silently while the
// cluster reported phase=Completed (#340). Fails without Storage in
// computeStorageHash.
func TestComputeStorageHash_ChangesWithStorage(t *testing.T) {
	rack := &ackov1alpha1.Rack{ID: 0}
	withStorage := func(s *ackov1alpha1.AerospikeStorageSpec) *ackov1alpha1.AerospikeCluster {
		return &ackov1alpha1.AerospikeCluster{
			Spec: ackov1alpha1.AerospikeClusterSpec{
				Image:   "aerospike:ce-8.1.1.1",
				Storage: s,
			},
		}
	}
	emptyDirAt := func(name, path string) *ackov1alpha1.AerospikeStorageSpec {
		return &ackov1alpha1.AerospikeStorageSpec{
			Volumes: []ackov1alpha1.VolumeSpec{{
				Name:      name,
				Source:    ackov1alpha1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
				Aerospike: &ackov1alpha1.AerospikeVolumeAttachment{Path: path},
			}},
		}
	}

	none := computeStorageHash(withStorage(nil), rack)
	oneVolume := computeStorageHash(withStorage(emptyDirAt("data", "/opt/aerospike/data")), rack)
	movedMount := computeStorageHash(withStorage(emptyDirAt("data", "/opt/aerospike/moved")), rack)
	renamed := computeStorageHash(withStorage(emptyDirAt("other", "/opt/aerospike/data")), rack)

	if none == oneVolume {
		t.Error("hash should change when a volume is added to an empty storage spec")
	}
	if oneVolume == movedMount {
		t.Error("hash should change when a volume's mount path changes")
	}
	if oneVolume == renamed {
		t.Error("hash should change when a volume is renamed")
	}
	if got := computeStorageHash(withStorage(emptyDirAt("data", "/opt/aerospike/data")), rack); got != oneVolume {
		t.Errorf("hash should stay stable for unchanged storage: %q != %q", got, oneVolume)
	}
}

// TestComputeStorageHash_UsesRackStorageOverride pins that the hash describes the
// storage the rack's pod template was actually built from. reconcileStatefulSet
// resolves rack.Storage over cluster.Spec.Storage; hashing the cluster-level spec
// instead would leave a rack-override edit undetected.
func TestComputeStorageHash_UsesRackStorageOverride(t *testing.T) {
	clusterStorage := &ackov1alpha1.AerospikeStorageSpec{
		Volumes: []ackov1alpha1.VolumeSpec{{
			Name:      "data",
			Source:    ackov1alpha1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
			Aerospike: &ackov1alpha1.AerospikeVolumeAttachment{Path: "/opt/aerospike/data"},
		}},
	}
	cluster := &ackov1alpha1.AerospikeCluster{
		Spec: ackov1alpha1.AerospikeClusterSpec{
			Image:   "aerospike:ce-8.1.1.1",
			Storage: clusterStorage,
		},
	}

	rackOverride := func(path string) *ackov1alpha1.Rack {
		return &ackov1alpha1.Rack{ID: 1, Storage: &ackov1alpha1.AerospikeStorageSpec{
			Volumes: []ackov1alpha1.VolumeSpec{{
				Name:      "data",
				Source:    ackov1alpha1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
				Aerospike: &ackov1alpha1.AerospikeVolumeAttachment{Path: path},
			}},
		}}
	}

	before := computeStorageHash(cluster, rackOverride("/opt/aerospike/data"))
	after := computeStorageHash(cluster, rackOverride("/opt/aerospike/moved"))
	if before == after {
		t.Error("hash should change when a rack's storage override changes")
	}
}
