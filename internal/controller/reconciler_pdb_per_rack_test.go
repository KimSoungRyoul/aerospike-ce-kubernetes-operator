package controller

import (
	"context"
	"testing"

	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	ackov1alpha1 "github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/api/v1alpha1"
	"github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/internal/utils"
)

// --- per-rack PodDisruptionBudgets ---
//
// The PDB was cluster-wide, selecting on SelectorLabelsForCluster, which carries
// no rack label. With 3 racks of 2 pods and maxUnavailable: 1, Kubernetes permits
// one eviction cluster-wide at a time — but nothing constrains WHICH pods, so a
// drain can take both pods of the same rack in sequence and leave that rack with
// nothing (#94).
//
// Multi-rack clusters now get one PDB per rack, selecting on the rack label.
// Single-rack clusters keep the cluster-wide PDB unchanged.

const (
	pdbTestCluster = "demo"
	pdbTestNS      = "default"
)

func pdbTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme(client-go) error = %v", err)
	}
	if err := ackov1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme(acko) error = %v", err)
	}
	return scheme
}

func pdbTestCR(size int32, rackIDs ...int) *ackov1alpha1.AerospikeCluster {
	cluster := &ackov1alpha1.AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{Name: pdbTestCluster, Namespace: pdbTestNS},
		Spec: ackov1alpha1.AerospikeClusterSpec{
			Size:  size,
			Image: "aerospike:ce-8.1.1.1",
		},
	}
	if len(rackIDs) > 0 {
		racks := make([]ackov1alpha1.Rack, 0, len(rackIDs))
		for _, id := range rackIDs {
			racks = append(racks, ackov1alpha1.Rack{ID: id})
		}
		cluster.Spec.RackConfig = &ackov1alpha1.RackConfig{Racks: racks}
	}
	return cluster
}

func pdbTestReconciler(t *testing.T, cluster *ackov1alpha1.AerospikeCluster) *AerospikeClusterReconciler {
	t.Helper()
	scheme := pdbTestScheme(t)
	return &AerospikeClusterReconciler{
		Client:   fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster).Build(),
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(16),
	}
}

func getPDB(t *testing.T, r *AerospikeClusterReconciler, name string) (*policyv1.PodDisruptionBudget, bool) {
	t.Helper()
	pdb := &policyv1.PodDisruptionBudget{}
	err := r.Get(context.Background(), types.NamespacedName{Name: name, Namespace: pdbTestNS}, pdb)
	switch {
	case err == nil:
		return pdb, true
	case apierrors.IsNotFound(err):
		return nil, false
	default:
		t.Fatalf("Get PDB %s: %v", name, err)
		return nil, false
	}
}

// TestReconcilePDB_MultiRackCreatesOnePDBPerRack is the regression test: pre-fix
// only the cluster-wide PDB existed, so both per-rack lookups came back missing.
func TestReconcilePDB_MultiRackCreatesOnePDBPerRack(t *testing.T) {
	cluster := pdbTestCR(6, 1, 2, 3)
	r := pdbTestReconciler(t, cluster)

	if err := r.reconcilePDB(context.Background(), cluster); err != nil {
		t.Fatalf("reconcilePDB() error = %v", err)
	}

	for _, rackID := range []int{1, 2, 3} {
		name := utils.RackPDBName(pdbTestCluster, rackID)
		pdb, ok := getPDB(t, r, name)
		if !ok {
			t.Fatalf("PDB %s was not created; a cluster-wide PDB cannot stop a drain "+
				"from taking every pod of one rack", name)
		}
		// The rack label is what makes the budget per-rack.
		want := utils.LabelsForRack(pdbTestCluster, rackID)
		got := pdb.Spec.Selector.MatchLabels
		for k, v := range want {
			if got[k] != v {
				t.Errorf("PDB %s selector[%q] = %q, want %q (selector: %v)", name, k, got[k], v, got)
			}
		}
		if got[utils.RackLabel] == "" {
			t.Errorf("PDB %s selector has no rack label; it would constrain the whole cluster", name)
		}
	}

	// The cluster-wide PDB must be gone, or two budgets constrain the same pods.
	if _, ok := getPDB(t, r, utils.PDBName(pdbTestCluster)); ok {
		t.Error("cluster-wide PDB still present alongside per-rack PDBs")
	}
}

func TestReconcilePDB_SingleRackKeepsClusterWidePDB(t *testing.T) {
	tests := []struct {
		name    string
		cluster *ackov1alpha1.AerospikeCluster
	}{
		{name: "no rackConfig", cluster: pdbTestCR(3)},
		{name: "one explicit rack", cluster: pdbTestCR(3, 1)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := pdbTestReconciler(t, tt.cluster)
			if err := r.reconcilePDB(context.Background(), tt.cluster); err != nil {
				t.Fatalf("reconcilePDB() error = %v", err)
			}

			pdb, ok := getPDB(t, r, utils.PDBName(pdbTestCluster))
			if !ok {
				t.Fatal("cluster-wide PDB missing for a single-rack cluster")
			}
			if pdb.Spec.Selector.MatchLabels[utils.RackLabel] != "" {
				t.Error("single-rack cluster-wide PDB selector gained a rack label")
			}
		})
	}
}

// TestEffectivePDBPolicy covers the budget resolution: precedence, the
// quorum-aware default, and the clamp that stops a PDB permitting full
// disruption.
func TestEffectivePDBPolicy(t *testing.T) {
	ptr := func(v intstr.IntOrString) *intstr.IntOrString { return &v }

	tests := []struct {
		name             string
		clusterMax       *intstr.IntOrString
		rackMax          *intstr.IntOrString
		rackSize         int32
		wantMinAvailable *int32
		wantMaxUnavail   *intstr.IntOrString
	}{
		{
			name:             "default is quorum-aware for an odd rack",
			rackSize:         3,
			wantMinAvailable: ptrInt32(2),
		},
		{
			name:             "default is quorum-aware for an even rack",
			rackSize:         6,
			wantMinAvailable: ptrInt32(4),
		},
		{
			// Called out explicitly: for racks of 1 or 2 the formula permits no
			// voluntary disruption at all, so a node drain blocks.
			name:             "rack of two keeps both pods",
			rackSize:         2,
			wantMinAvailable: ptrInt32(2),
		},
		{
			name:             "rack of one keeps its only pod",
			rackSize:         1,
			wantMinAvailable: ptrInt32(1),
		},
		{
			name:           "unresolved size falls back to the historical default",
			rackSize:       0,
			wantMaxUnavail: ptr(intstr.FromInt32(1)),
		},
		{
			name:           "cluster-level maxUnavailable wins over the default",
			clusterMax:     ptr(intstr.FromInt32(1)),
			rackSize:       6,
			wantMaxUnavail: ptr(intstr.FromInt32(1)),
		},
		{
			name:           "rack maxUnavailable wins over the cluster level",
			clusterMax:     ptr(intstr.FromInt32(1)),
			rackMax:        ptr(intstr.FromInt32(2)),
			rackSize:       6,
			wantMaxUnavail: ptr(intstr.FromInt32(2)),
		},
		{
			// Defence in depth behind the webhook rejection: a value that would
			// let the whole rack go is clamped rather than honoured.
			name:           "a budget permitting the whole rack is clamped",
			clusterMax:     ptr(intstr.FromInt32(3)),
			rackSize:       3,
			wantMaxUnavail: ptr(intstr.FromInt32(2)),
		},
		{
			name:           "percentages are passed through for Kubernetes to resolve",
			clusterMax:     ptr(intstr.FromString("50%")),
			rackSize:       6,
			wantMaxUnavail: ptr(intstr.FromString("50%")),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := pdbTestCR(6)
			cluster.Spec.MaxUnavailable = tt.clusterMax
			rack := &ackov1alpha1.Rack{ID: 1, MaxUnavailable: tt.rackMax}

			got := effectivePDBPolicy(cluster, rack, tt.rackSize)

			switch {
			case tt.wantMinAvailable != nil:
				if got.MinAvailable == nil {
					t.Fatalf("MinAvailable = nil, want %d", *tt.wantMinAvailable)
				}
				if got.MinAvailable.IntVal != *tt.wantMinAvailable {
					t.Errorf("MinAvailable = %d, want %d", got.MinAvailable.IntVal, *tt.wantMinAvailable)
				}
				if got.MaxUnavailable != nil {
					t.Errorf("MaxUnavailable = %v, want nil when MinAvailable is set", got.MaxUnavailable)
				}
			default:
				if got.MaxUnavailable == nil {
					t.Fatalf("MaxUnavailable = nil, want %v", tt.wantMaxUnavail)
				}
				if !intOrStringEqual(*got.MaxUnavailable, *tt.wantMaxUnavail) {
					t.Errorf("MaxUnavailable = %v, want %v", got.MaxUnavailable, tt.wantMaxUnavail)
				}
				if got.MinAvailable != nil {
					t.Errorf("MinAvailable = %v, want nil when MaxUnavailable is set", got.MinAvailable)
				}
			}
		})
	}
}

func ptrInt32(v int32) *int32 { return &v }

// TestReconcilePDB_RemovedRackPDBIsCleanedUp pins that a rack dropped from the
// spec does not leave a budget behind constraining evictions for pods that no
// longer exist.
func TestReconcilePDB_RemovedRackPDBIsCleanedUp(t *testing.T) {
	cluster := pdbTestCR(6, 1, 2, 3)
	r := pdbTestReconciler(t, cluster)
	if err := r.reconcilePDB(context.Background(), cluster); err != nil {
		t.Fatalf("reconcilePDB() error = %v", err)
	}
	goneName := utils.RackPDBName(pdbTestCluster, 3)
	if _, ok := getPDB(t, r, goneName); !ok {
		t.Fatalf("setup: PDB %s was not created", goneName)
	}

	// Drop rack 3.
	cluster.Spec.RackConfig.Racks = cluster.Spec.RackConfig.Racks[:2]
	if err := r.reconcilePDB(context.Background(), cluster); err != nil {
		t.Fatalf("reconcilePDB() after rack removal error = %v", err)
	}

	if _, ok := getPDB(t, r, goneName); ok {
		t.Error("PDB for the removed rack was left behind")
	}
	for _, rackID := range []int{1, 2} {
		if _, ok := getPDB(t, r, utils.RackPDBName(pdbTestCluster, rackID)); !ok {
			t.Errorf("PDB for surviving rack %d was removed", rackID)
		}
	}
}

// TestReconcilePDB_SwitchingToSingleRackRemovesRackPDBs covers the reverse
// topology change, so a shrink to one rack does not leave stale per-rack budgets.
func TestReconcilePDB_SwitchingToSingleRackRemovesRackPDBs(t *testing.T) {
	cluster := pdbTestCR(6, 1, 2)
	r := pdbTestReconciler(t, cluster)
	if err := r.reconcilePDB(context.Background(), cluster); err != nil {
		t.Fatalf("reconcilePDB() error = %v", err)
	}

	cluster.Spec.RackConfig = nil
	if err := r.reconcilePDB(context.Background(), cluster); err != nil {
		t.Fatalf("reconcilePDB() after topology change error = %v", err)
	}

	if _, ok := getPDB(t, r, utils.PDBName(pdbTestCluster)); !ok {
		t.Error("cluster-wide PDB was not created after shrinking to a single rack")
	}
	for _, rackID := range []int{1, 2} {
		if _, ok := getPDB(t, r, utils.RackPDBName(pdbTestCluster, rackID)); ok {
			t.Errorf("per-rack PDB for rack %d survived the switch to a single rack", rackID)
		}
	}
}

// TestReconcilePDB_DisablePDBRemovesEveryBudget pins that spec.disablePDB still
// clears everything now that there can be more than one PDB.
func TestReconcilePDB_DisablePDBRemovesEveryBudget(t *testing.T) {
	cluster := pdbTestCR(6, 1, 2)
	r := pdbTestReconciler(t, cluster)
	if err := r.reconcilePDB(context.Background(), cluster); err != nil {
		t.Fatalf("reconcilePDB() error = %v", err)
	}

	disable := true
	cluster.Spec.DisablePDB = &disable
	if err := r.reconcilePDB(context.Background(), cluster); err != nil {
		t.Fatalf("reconcilePDB() with disablePDB error = %v", err)
	}

	for _, rackID := range []int{1, 2} {
		if _, ok := getPDB(t, r, utils.RackPDBName(pdbTestCluster, rackID)); ok {
			t.Errorf("per-rack PDB for rack %d survived disablePDB", rackID)
		}
	}
	if _, ok := getPDB(t, r, utils.PDBName(pdbTestCluster)); ok {
		t.Error("cluster-wide PDB survived disablePDB")
	}
}
