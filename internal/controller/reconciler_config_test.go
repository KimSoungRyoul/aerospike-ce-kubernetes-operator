package controller

import (
	"context"
	"testing"

	ackov1alpha1 "github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/api/v1alpha1"
	"github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/internal/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestGetEffectiveConfig(t *testing.T) {
	r := &AerospikeClusterReconciler{}

	tests := []struct {
		name          string
		clusterConfig *ackov1alpha1.AerospikeConfigSpec
		rackConfig    *ackov1alpha1.AerospikeConfigSpec
		wantNil       bool
		// checkKey and checkVal are used for non-nil results to verify a specific top-level key.
		checkKey string
		checkVal any
	}{
		{
			name:          "both nil returns nil",
			clusterConfig: nil,
			rackConfig:    nil,
			wantNil:       true,
		},
		{
			name: "cluster config set, rack nil returns cluster config",
			clusterConfig: &ackov1alpha1.AerospikeConfigSpec{
				Value: map[string]any{
					"service": map[string]any{
						"cluster-name": "test-cluster",
					},
				},
			},
			rackConfig: nil,
			wantNil:    false,
			checkKey:   "service",
		},
		{
			name:          "cluster config nil, rack config set returns rack config",
			clusterConfig: nil,
			rackConfig: &ackov1alpha1.AerospikeConfigSpec{
				Value: map[string]any{
					"service": map[string]any{
						"proto-fd-max": 10000,
					},
				},
			},
			wantNil:  false,
			checkKey: "service",
		},
		{
			name: "both set returns deep merge with rack overriding cluster",
			clusterConfig: &ackov1alpha1.AerospikeConfigSpec{
				Value: map[string]any{
					"service": map[string]any{
						"cluster-name": "test-cluster",
						"proto-fd-max": 15000,
					},
					"network": map[string]any{
						"service": map[string]any{
							"port": 3000,
						},
					},
				},
			},
			rackConfig: &ackov1alpha1.AerospikeConfigSpec{
				Value: map[string]any{
					"service": map[string]any{
						"proto-fd-max": 20000,
					},
				},
			},
			wantNil:  false,
			checkKey: "service",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cluster := &ackov1alpha1.AerospikeCluster{
				Spec: ackov1alpha1.AerospikeClusterSpec{
					AerospikeConfig: tc.clusterConfig,
				},
			}
			rack := &ackov1alpha1.Rack{
				ID:              0,
				AerospikeConfig: tc.rackConfig,
			}

			got := r.getEffectiveConfig(cluster, rack)

			if tc.wantNil {
				if got != nil {
					t.Fatalf("getEffectiveConfig() = %v, want nil", got)
				}
				return
			}

			if got == nil {
				t.Fatal("getEffectiveConfig() = nil, want non-nil")
			}

			if tc.checkKey != "" {
				if _, ok := got.Value[tc.checkKey]; !ok {
					t.Errorf("getEffectiveConfig() result missing key %q", tc.checkKey)
				}
			}
		})
	}
}

func TestGetEffectiveConfig_MergeOverride(t *testing.T) {
	r := &AerospikeClusterReconciler{}

	cluster := &ackov1alpha1.AerospikeCluster{
		Spec: ackov1alpha1.AerospikeClusterSpec{
			AerospikeConfig: &ackov1alpha1.AerospikeConfigSpec{
				Value: map[string]any{
					"service": map[string]any{
						"cluster-name": "test-cluster",
						"proto-fd-max": 15000,
					},
					"network": map[string]any{
						"service": map[string]any{
							"port": 3000,
						},
					},
				},
			},
		},
	}
	rack := &ackov1alpha1.Rack{
		ID: 1,
		AerospikeConfig: &ackov1alpha1.AerospikeConfigSpec{
			Value: map[string]any{
				"service": map[string]any{
					"proto-fd-max": 20000,
				},
			},
		},
	}

	got := r.getEffectiveConfig(cluster, rack)
	if got == nil {
		t.Fatal("getEffectiveConfig() = nil, want non-nil")
	}

	// Rack override should win for proto-fd-max
	svc, ok := got.Value["service"].(map[string]any)
	if !ok {
		t.Fatal("expected 'service' to be map[string]any")
	}
	if svc["proto-fd-max"] != 20000 {
		t.Errorf("proto-fd-max = %v, want 20000", svc["proto-fd-max"])
	}

	// Cluster-level cluster-name should be preserved
	if svc["cluster-name"] != "test-cluster" {
		t.Errorf("cluster-name = %v, want 'test-cluster'", svc["cluster-name"])
	}

	// Cluster-level network section should be preserved
	net, ok := got.Value["network"].(map[string]any)
	if !ok {
		t.Fatal("expected 'network' to be map[string]any")
	}
	netSvc, ok := net["service"].(map[string]any)
	if !ok {
		t.Fatal("expected 'network.service' to be map[string]any")
	}
	if netSvc["port"] != 3000 {
		t.Errorf("network.service.port = %v, want 3000", netSvc["port"])
	}
}

func TestGetEffectiveConfig_ClusterConfigOnly(t *testing.T) {
	r := &AerospikeClusterReconciler{}

	clusterConfig := &ackov1alpha1.AerospikeConfigSpec{
		Value: map[string]any{
			"service": map[string]any{
				"cluster-name": "my-cluster",
			},
		},
	}
	cluster := &ackov1alpha1.AerospikeCluster{
		Spec: ackov1alpha1.AerospikeClusterSpec{
			AerospikeConfig: clusterConfig,
		},
	}
	rack := &ackov1alpha1.Rack{ID: 0}

	got := r.getEffectiveConfig(cluster, rack)
	if got != clusterConfig {
		t.Errorf("when rack config is nil, getEffectiveConfig should return cluster config pointer directly")
	}
}

func TestGetEffectiveConfig_RackConfigOnly(t *testing.T) {
	r := &AerospikeClusterReconciler{}

	rackConfig := &ackov1alpha1.AerospikeConfigSpec{
		Value: map[string]any{
			"service": map[string]any{
				"proto-fd-max": 10000,
			},
		},
	}
	cluster := &ackov1alpha1.AerospikeCluster{
		Spec: ackov1alpha1.AerospikeClusterSpec{
			AerospikeConfig: nil,
		},
	}
	rack := &ackov1alpha1.Rack{
		ID:              0,
		AerospikeConfig: rackConfig,
	}

	got := r.getEffectiveConfig(cluster, rack)
	if got != rackConfig {
		t.Errorf("when cluster config is nil, getEffectiveConfig should return rack config pointer directly")
	}
}

// TestReconcileConfigMapDoesNotMutateClusterSpec verifies that reconcileConfigMap
// does not leak access-address placeholders into the shared
// cluster.Spec.AerospikeConfig when spec.aerospikeNetworkPolicy is set.
//
// getEffectiveConfig returns cluster.Spec.AerospikeConfig directly when there is
// no rack-level config to merge, so InjectAccessAddressPlaceholders must operate
// on a deep copy. Otherwise the placeholders pollute the dynamic-config diff
// (forcing a cold restart for every dynamic change) and cluster.Status.AerospikeConfig.
func TestReconcileConfigMapDoesNotMutateClusterSpec(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme(client-go) error = %v", err)
	}
	if err := ackov1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme(acko) error = %v", err)
	}

	cluster := &ackov1alpha1.AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo",
			Namespace: "default",
		},
		Spec: ackov1alpha1.AerospikeClusterSpec{
			Size: 1,
			// aerospikeNetworkPolicy set => InjectAccessAddressPlaceholders is active.
			AerospikeNetworkPolicy: &ackov1alpha1.AerospikeNetworkPolicy{
				AccessType: ackov1alpha1.AerospikeNetworkTypePod,
			},
			AerospikeConfig: &ackov1alpha1.AerospikeConfigSpec{
				Value: map[string]any{
					"service": map[string]any{
						"cluster-name": "demo",
					},
					"network": map[string]any{
						"service": map[string]any{
							"port": 3000,
						},
						"heartbeat": map[string]any{
							"mode": "mesh",
							"port": 3002,
						},
						"fabric": map[string]any{
							"port": 3001,
						},
					},
				},
			},
		},
	}

	reconciler := &AerospikeClusterReconciler{
		Client: fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(cluster).
			Build(),
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(8),
	}

	rack := &ackov1alpha1.Rack{ID: 0}
	// getEffectiveConfig returns cluster.Spec.AerospikeConfig directly here
	// (no rack override), so this is the pointer that must not be mutated.
	effective := reconciler.getEffectiveConfig(cluster, rack)

	if err := reconciler.reconcileConfigMap(context.Background(), cluster, rack, effective); err != nil {
		t.Fatalf("reconcileConfigMap() error = %v", err)
	}

	// The placeholder must NOT have leaked into the shared cluster spec.
	netSection := cluster.Spec.AerospikeConfig.Value["network"].(map[string]any)
	svcSection := netSection["service"].(map[string]any)
	if _, leaked := svcSection["access-address"]; leaked {
		t.Errorf("access-address placeholder leaked into cluster.Spec.AerospikeConfig: %v", svcSection)
	}

	// Sanity check: the ConfigMap itself was still generated with the placeholder.
	cm := &corev1.ConfigMap{}
	cmName := utils.ConfigMapName(cluster.Name, rack.ID)
	if err := reconciler.Get(context.Background(),
		types.NamespacedName{Name: cmName, Namespace: cluster.Namespace}, cm); err != nil {
		t.Fatalf("Get(ConfigMap) error = %v", err)
	}
	if cm.Data["aerospike.conf"] == "" {
		t.Fatal("generated ConfigMap has no aerospike.conf data")
	}
}
