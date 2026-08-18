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

// TestEffectivePDBPolicy covers budget resolution: precedence, the
// replication-factor default, and the clamp that stops a PDB permitting full
// disruption.
func TestEffectivePDBPolicy(t *testing.T) {
	ptr := func(v intstr.IntOrString) *intstr.IntOrString { return &v }

	tests := []struct {
		name       string
		clusterMax *intstr.IntOrString
		rackMax    *intstr.IntOrString
		rackRF     int // 0 = no aerospikeConfig at all
		rackSize   int32
		want       intstr.IntOrString
	}{
		{
			// The default RF. One node can go without a partition becoming
			// unavailable, which is also today's flat default — no regression.
			name: "replication-factor 2 allows one", rackRF: 2, rackSize: 3,
			want: intstr.FromInt32(1),
		},
		{
			name: "replication-factor 3 allows two", rackRF: 3, rackSize: 6,
			want: intstr.FromInt32(2),
		},
		{
			// Floored at 1. RF=1 has no redundancy so the honest budget is 0, but
			// a default of 0 is a deadlock on a configuration CE users run for
			// dev. Nothing warns about this today — the docs state it, and the
			// budget is deliberately permissive rather than correct.
			name: "replication-factor 1 is floored at one", rackRF: 1, rackSize: 3,
			want: intstr.FromInt32(1),
		},
		{
			name: "no aerospikeConfig falls back to Aerospike's own default RF", rackRF: 0, rackSize: 3,
			want: intstr.FromInt32(1),
		},
		{
			// The layout that made the majority rule unusable: 3 racks at CE's
			// 8-node cap gives a rack of 2. Must still allow one.
			name: "a rack of two still allows one", rackRF: 2, rackSize: 2,
			want: intstr.FromInt32(1),
		},
		{
			// The clamp degenerates at one pod, so it is skipped there rather
			// than blocking drains on the single-pod minimal template.
			name: "a rack of one is not clamped to zero", rackRF: 2, rackSize: 1,
			want: intstr.FromInt32(1),
		},
		{
			name:       "cluster-level maxUnavailable wins over the default",
			clusterMax: ptr(intstr.FromInt32(1)), rackRF: 3, rackSize: 6,
			want: intstr.FromInt32(1),
		},
		{
			name:       "rack maxUnavailable wins over the cluster level",
			clusterMax: ptr(intstr.FromInt32(1)), rackMax: ptr(intstr.FromInt32(2)),
			rackRF: 2, rackSize: 6,
			want: intstr.FromInt32(2),
		},
		{
			// Defence in depth behind the webhook rejection.
			name:       "a budget permitting the whole rack is clamped",
			clusterMax: ptr(intstr.FromInt32(3)), rackRF: 2, rackSize: 3,
			want: intstr.FromInt32(2),
		},
		{
			name:       "percentages are passed through for Kubernetes to resolve",
			clusterMax: ptr(intstr.FromString("50%")), rackRF: 2, rackSize: 6,
			want: intstr.FromString("50%"),
		},
	}

	r := &AerospikeClusterReconciler{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := pdbTestCR(6)
			cluster.Spec.MaxUnavailable = tt.clusterMax
			if tt.rackRF > 0 {
				cluster.Spec.AerospikeConfig = &ackov1alpha1.AerospikeConfigSpec{Value: map[string]any{
					"namespaces": []any{map[string]any{"name": "test", "replication-factor": tt.rackRF}},
				}}
			}
			rack := &ackov1alpha1.Rack{ID: 1, MaxUnavailable: tt.rackMax}

			got := r.effectivePDBPolicy(cluster, rack, tt.rackSize)
			if got.MaxUnavailable == nil {
				t.Fatalf("MaxUnavailable = nil, want %v", tt.want)
			}
			if !intOrStringEqual(*got.MaxUnavailable, tt.want) {
				t.Errorf("MaxUnavailable = %v, want %v", got.MaxUnavailable, tt.want)
			}
		})
	}
}

// TestDefaultMaxUnavailable_NoLayoutBlocksDisruption is the regression for the
// HIGH review found: the majority rule (minAvailable = rackSize/2 + 1) blocked
// ALL voluntary disruption on 12 of 21 CE-legal layouts, including EVERY 3-rack
// layout at every size, because getRackSize divides spec.size across racks so a
// large cluster still has small racks. Three racks, one per zone, is the
// canonical rack-aware topology — exactly what per-rack PDBs are for.
func TestDefaultMaxUnavailable_NoLayoutBlocksDisruption(t *testing.T) {
	r := &AerospikeClusterReconciler{}
	for rf := 1; rf <= 3; rf++ {
		for size := int32(1); size <= 8; size++ {
			for numRacks := 1; int32(numRacks) <= size; numRacks++ {
				racks := make([]ackov1alpha1.Rack, numRacks)
				for i := range racks {
					racks[i] = ackov1alpha1.Rack{ID: i + 1}
				}
				cluster := pdbTestCR(size)
				cluster.Spec.RackConfig = &ackov1alpha1.RackConfig{Racks: racks}
				cluster.Spec.AerospikeConfig = &ackov1alpha1.AerospikeConfigSpec{Value: map[string]any{
					"namespaces": []any{map[string]any{"name": "test", "replication-factor": rf}},
				}}

				for i := range racks {
					rackSize := r.getRackSize(cluster, racks, i)
					policy := r.effectivePDBPolicy(cluster, &racks[i], rackSize)
					if policy.MaxUnavailable.IntVal < 1 {
						t.Errorf("rf=%d size=%d racks=%d rack[%d] size=%d: maxUnavailable=%d — "+
							"blocks kubectl drain, autoscaler node recycling and node-pool upgrades",
							rf, size, numRacks, i, rackSize, policy.MaxUnavailable.IntVal)
					}
				}
			}
		}
	}
}

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

// TestReconcilePDB_DoesNotHijackAnotherClustersPDB covers the name collision:
// RackPDBName("demo", 1) and PDBName("demo-1") both produce "demo-1-pdb". The
// update path was a plain Get-then-Update by name, so cluster "demo" silently
// rewrote cluster "demo-1"'s PDB — repointing the selector at demo's pods,
// overwriting the cluster label so demo-1 could not find its own PDB to repair,
// and leaving the victim's ownerRef so deleting demo-1 would garbage-collect a
// PDB demo believed it owned.
func TestReconcilePDB_DoesNotHijackAnotherClustersPDB(t *testing.T) {
	const victimName = "demo-1"
	collidingName := utils.RackPDBName(pdbTestCluster, 1)
	if collidingName != utils.PDBName(victimName) {
		t.Fatalf("setup: expected a name collision, got %q vs %q",
			collidingName, utils.PDBName(victimName))
	}

	victimMax := intstr.FromInt32(1)
	victimPDB := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      collidingName,
			Namespace: pdbTestNS,
			Labels:    utils.LabelsForCluster(victimName),
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MaxUnavailable: &victimMax,
			Selector:       &metav1.LabelSelector{MatchLabels: utils.SelectorLabelsForCluster(victimName)},
		},
	}

	// "demo" is multi-rack, so it wants a per-rack PDB named demo-1-pdb.
	cluster := pdbTestCR(6, 1, 2)
	scheme := pdbTestScheme(t)
	recorder := record.NewFakeRecorder(16)
	r := &AerospikeClusterReconciler{
		Client:   fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster, victimPDB).Build(),
		Scheme:   scheme,
		Recorder: recorder,
	}

	if err := r.reconcilePDB(context.Background(), cluster); err != nil {
		t.Fatalf("reconcilePDB() error = %v", err)
	}

	after, ok := getPDB(t, r, collidingName)
	if !ok {
		t.Fatal("the other cluster's PDB was deleted")
	}
	if got := after.Labels[utils.InstanceLabel]; got != victimName {
		t.Errorf("cluster label = %q, want %q; the victim can no longer find its own PDB by label", got, victimName)
	}
	if got := after.Spec.Selector.MatchLabels[utils.InstanceLabel]; got != victimName {
		t.Errorf("selector instance = %q, want %q; the victim's pods lost disruption protection", got, victimName)
	}

	if events := drainRecorderEvents(recorder); !containsEvent(events, EventPDBNameConflict) {
		t.Errorf("expected a %s event, got %v", EventPDBNameConflict, events)
	}

	// The other rack, whose name does not collide, must still get its PDB.
	if _, ok := getPDB(t, r, utils.RackPDBName(pdbTestCluster, 2)); !ok {
		t.Error("a name conflict on one rack stopped the other rack's PDB being created")
	}
}

// TestIsRackPDBName covers the ownership guard in deleteRackPDBs. The digit check
// is the only thing preventing over-deletion: the cluster label list alone does
// not protect a user's own PDB, since anyone writing a PDB for this cluster would
// naturally label it with the cluster's labels.
func TestIsRackPDBName(t *testing.T) {
	tests := []struct {
		name        string
		pdbName     string
		clusterName string
		want        bool
	}{
		{name: "operator per-rack name", pdbName: "demo-1-pdb", clusterName: "demo", want: true},
		{name: "multi-digit rack id", pdbName: "demo-42-pdb", clusterName: "demo", want: true},
		{name: "cluster-wide name is not a rack name", pdbName: "demo-pdb", clusterName: "demo", want: false},
		{
			// The case the digit check exists for: a user's own PDB, labelled with
			// the cluster's labels because it is for this cluster, must survive.
			name: "user PDB with a non-numeric suffix", pdbName: "demo-critical-pdb", clusterName: "demo", want: false,
		},
		{name: "another cluster's rack PDB", pdbName: "other-1-pdb", clusterName: "demo", want: false},
		{name: "missing the -pdb suffix", pdbName: "demo-1", clusterName: "demo", want: false},
		{name: "empty rack id", pdbName: "demo--pdb", clusterName: "demo", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isRackPDBName(tt.pdbName, tt.clusterName); got != tt.want {
				t.Errorf("isRackPDBName(%q, %q) = %v, want %v", tt.pdbName, tt.clusterName, got, tt.want)
			}
		})
	}
}
