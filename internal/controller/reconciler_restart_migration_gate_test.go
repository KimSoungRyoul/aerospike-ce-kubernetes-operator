package controller

import (
	"context"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	ackov1alpha1 "github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/api/v1alpha1"
	"github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/internal/utils"
)

// --- the rolling restart's migration gate fails closed ---
//
// Both destructive paths gate on isMigrationInProgress, and the sensor beneath
// is deliberately fail-closed (aero_info.go: an errored, absent or unparseable
// migrate_partitions_remaining counts as migrating; an unreachable node is
// recorded with a positive sentinel rather than dropped). The scale-down path
// preserves that and treats a check error as "migrating". The rolling restart
// discarded it and deleted the next batch anyway — the same signal producing
// opposite postures in the two paths that destroy pods, with the fail-open
// branch firing exactly when the cluster is least reachable (#341).
//
// TestIsBatchBlocked_MigrationCheckError_FailsClosed is the regression test: it
// asserts true (blocked). It fails on the pre-fix code, which returned false.
//
// The remaining tests pin the bounded escape hatch, which is what keeps failing
// closed from becoming a deadlock when the restart is itself the remedy.

const (
	migrationGateRack    = 0
	migrationGateCluster = "demo"
	migrationGateSecret  = "missing-admin-secret"
)

// migrationGateReconciler builds a reconciler whose migration check errors
// deterministically and without touching the network: ACL is enabled, so
// getAerospikeClient looks up the admin password Secret first, and that Secret
// does not exist.
func migrationGateReconciler(t *testing.T) (*AerospikeClusterReconciler, *ackov1alpha1.AerospikeCluster, *record.FakeRecorder) {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := ackov1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("acko AddToScheme() error = %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("corev1 AddToScheme() error = %v", err)
	}

	cluster := &ackov1alpha1.AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      migrationGateCluster,
			Namespace: ctrlTestNamespace,
			UID:       "cluster-uid-demo",
		},
		Spec: ackov1alpha1.AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeAccessControl: &ackov1alpha1.AerospikeAccessControlSpec{
				Users: []ackov1alpha1.AerospikeUserSpec{{
					Name:       "admin",
					SecretName: migrationGateSecret,
					Roles:      []string{"sys-admin", "user-admin"},
				}},
			},
		},
	}

	recorder := record.NewFakeRecorder(16)
	r := &AerospikeClusterReconciler{
		Client:   fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster).Build(),
		Scheme:   scheme,
		Recorder: recorder,
	}
	return r, cluster, recorder
}

func TestIsBatchBlocked_MigrationCheckError_FailsClosed(t *testing.T) {
	r, cluster, recorder := migrationGateReconciler(t)

	// Sanity: the check really does error, so the assertion below is exercising
	// the error branch and not a "not migrating" answer.
	if _, err := r.isMigrationInProgress(context.Background(), cluster); err == nil {
		t.Fatal("isMigrationInProgress() returned no error; this test needs the check to fail")
	}

	if blocked := r.isBatchBlocked(context.Background(), cluster, migrationGateRack, nil); !blocked {
		t.Fatal("isBatchBlocked() = false on a migration-check error, want true (blocked); " +
			"deleting the next batch while partitions may still be moving is how records are lost")
	}

	events := drainRecorderEvents(recorder)
	if !containsEvent(events, EventMigrationCheckFailed) {
		t.Errorf("expected a %s event, got %v", EventMigrationCheckFailed, events)
	}
	if containsEvent(events, EventMigrationCheckUnavailable) {
		t.Errorf("did not expect the escape-hatch event on the first failure, got %v", events)
	}
}

func TestIsBatchBlocked_MigrationCheckError_EscapeHatchIsBounded(t *testing.T) {
	tests := []struct {
		name string
		// seeded failure state before the call; zero value means no prior state.
		priorFailures int
		firstSeenAgo  time.Duration
		wantBlocked   bool
	}{
		{
			name:        "first failure blocks",
			wantBlocked: true,
		},
		{
			name:          "count reached but no time elapsed still blocks",
			priorFailures: maxMigrationCheckFailures,
			firstSeenAgo:  time.Second,
			wantBlocked:   true,
		},
		{
			name:          "time elapsed but count not reached still blocks",
			priorFailures: 1,
			firstSeenAgo:  2 * migrationCheckFailureGrace,
			wantBlocked:   true,
		},
		{
			// Both bounds satisfied: the restart proceeds without a migration
			// answer, because the restart may be the thing that makes the cluster
			// reachable again.
			name:          "count and time both reached releases the batch",
			priorFailures: maxMigrationCheckFailures - 1,
			firstSeenAgo:  2 * migrationCheckFailureGrace,
			wantBlocked:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, cluster, recorder := migrationGateReconciler(t)
			if tt.priorFailures > 0 {
				r.migrationCheckFailures = map[string]*migrationCheckState{
					migrationCheckKey(cluster, migrationGateRack): {
						failures:  tt.priorFailures,
						firstSeen: time.Now().Add(-tt.firstSeenAgo),
						// A live entry is touched on every failing reconcile, so
						// lastSeen is recent even when firstSeen is old. Leaving it
						// zero would make the TTL sweep drop the entry and silently
						// reset the budget the case is trying to exercise.
						lastSeen: time.Now(),
					},
				}
			}

			blocked := r.isBatchBlocked(context.Background(), cluster, migrationGateRack, nil)
			if blocked != tt.wantBlocked {
				t.Fatalf("isBatchBlocked() = %v, want %v", blocked, tt.wantBlocked)
			}

			events := drainRecorderEvents(recorder)
			if tt.wantBlocked {
				if containsEvent(events, EventMigrationCheckUnavailable) {
					t.Errorf("batch was held but the escape-hatch event was emitted: %v", events)
				}
				return
			}
			// Proceeding without a migration answer must be loud and distinct
			// from the "held" event, or it is invisible after the fact.
			if !containsEvent(events, EventMigrationCheckUnavailable) {
				t.Errorf("expected a %s event when the escape hatch opens, got %v",
					EventMigrationCheckUnavailable, events)
			}
		})
	}
}

// TestMigrationCheckFailures_ClearedByAnAnswer pins that a check which returns
// an answer resets the budget, so an outage later in the same restart starts
// from a full count rather than inheriting a nearly-spent one.
func TestMigrationCheckFailures_ClearedByAnAnswer(t *testing.T) {
	r, cluster, _ := migrationGateReconciler(t)
	key := migrationCheckKey(cluster, migrationGateRack)

	for range 3 {
		r.recordMigrationCheckFailure(cluster, migrationGateRack)
	}
	if got := r.migrationCheckFailures[key].failures; got != 3 {
		t.Fatalf("failures = %d, want 3", got)
	}

	r.clearMigrationCheckFailures(cluster, migrationGateRack)
	if _, still := r.migrationCheckFailures[key]; still {
		t.Fatal("failure state survived clearMigrationCheckFailures")
	}

	failures, _, hatchOpen := r.recordMigrationCheckFailure(cluster, migrationGateRack)
	if failures != 1 {
		t.Errorf("failures after clear = %d, want 1", failures)
	}
	if hatchOpen {
		t.Error("escape hatch opened on the first failure after a reset")
	}
}

// TestMigrationCheckFailures_KeyedPerRackAndCluster pins that one rack burning
// its budget does not release another rack's gate, and that a delete-and-recreate
// of the same cluster name starts fresh.
func TestMigrationCheckFailures_KeyedPerRackAndCluster(t *testing.T) {
	r, cluster, _ := migrationGateReconciler(t)

	for range maxMigrationCheckFailures {
		r.recordMigrationCheckFailure(cluster, 0)
	}

	if failures, _, _ := r.recordMigrationCheckFailure(cluster, 1); failures != 1 {
		t.Errorf("rack 1 failures = %d, want 1; rack 0's budget must not carry over", failures)
	}

	recreated := cluster.DeepCopy()
	recreated.UID = "cluster-uid-demo-recreated"
	if failures, _, _ := r.recordMigrationCheckFailure(recreated, 0); failures != 1 {
		t.Errorf("recreated cluster failures = %d, want 1; state must be keyed on UID", failures)
	}
}

// TestMigrationCheckKey_OperationsPathHasItsOwnBudget pins that the on-demand
// operations path does not share an escape-hatch budget with a real rack.
//
// operationBatchBlocked used to pass rackID 0, and getRacks returns a default
// rack with ID 0 for a cluster without rackConfig — so with the rack id half of
// migrationCheckKey, a rolling restart that had already spent four of its five
// failures let the operations path open the hatch on its first failure, deleting
// pods with migration state unknown.
func TestMigrationCheckKey_OperationsPathHasItsOwnBudget(t *testing.T) {
	r, cluster, _ := migrationGateReconciler(t)

	// The default rack burns its whole budget.
	for range maxMigrationCheckFailures {
		r.recordMigrationCheckFailure(cluster, 0)
	}
	if got := r.migrationCheckFailures[migrationCheckKey(cluster, 0)].failures; got != maxMigrationCheckFailures {
		t.Fatalf("setup: default rack failures = %d, want %d", got, maxMigrationCheckFailures)
	}

	failures, _, hatchOpen := r.recordMigrationCheckFailure(cluster, onDemandOperationRackID)
	if failures != 1 {
		t.Errorf("operations-path failures = %d, want 1; it must not inherit the default rack's budget", failures)
	}
	if hatchOpen {
		t.Error("escape hatch opened on the operations path's first failure")
	}

	// And the sentinel must not be a rack id the operator can actually produce.
	if onDemandOperationRackID >= 0 {
		t.Errorf("onDemandOperationRackID = %d; must be negative so it cannot collide with a real rack",
			onDemandOperationRackID)
	}
}

// TestCleanupRemovedRacks_EvictsMigrationBudgetOnRackRemoval closes the rack-ID
// reuse hole: migrationCheckKey is UID + rackID, so it names a SLOT, not an
// incarnation. Clearing only happened on an answering check, so a rack removed
// with a spent budget bequeathed it to any future rack that reused the ID — which
// could then open the escape hatch on its FIRST failure and delete pods with
// migration state unknown.
func TestCleanupRemovedRacks_EvictsMigrationBudgetOnRackRemoval(t *testing.T) {
	scheme := runtime.NewScheme()
	for _, add := range []func(*runtime.Scheme) error{
		ackov1alpha1.AddToScheme, corev1.AddToScheme, appsv1.AddToScheme,
	} {
		if err := add(scheme); err != nil {
			t.Fatalf("AddToScheme() error = %v", err)
		}
	}

	const goneRack = 2
	cluster := &ackov1alpha1.AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "demo", Namespace: ctrlTestNamespace, UID: "cluster-uid-evict",
		},
		Spec: ackov1alpha1.AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			// Rack 2 is gone from the spec; only rack 1 remains.
			RackConfig: &ackov1alpha1.RackConfig{Racks: []ackov1alpha1.Rack{{ID: 1}}},
		},
	}
	zero := int32(0)
	goneSts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.StatefulSetName(cluster.Name, goneRack),
			Namespace: ctrlTestNamespace,
			Labels:    utils.LabelsForCluster(cluster.Name),
		},
		Spec: appsv1.StatefulSetSpec{Replicas: &zero},
	}

	r := &AerospikeClusterReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&ackov1alpha1.AerospikeCluster{}).
			WithObjects(cluster, goneSts).Build(),
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(16),
	}

	// The departing rack spends its entire budget.
	for range maxMigrationCheckFailures {
		r.recordMigrationCheckFailure(cluster, goneRack)
	}
	if _, ok := r.migrationCheckFailures[migrationCheckKey(cluster, goneRack)]; !ok {
		t.Fatal("setup: no budget recorded for the departing rack")
	}

	if _, err := r.cleanupRemovedRacks(context.Background(), cluster, cluster.Spec.RackConfig.Racks); err != nil {
		t.Fatalf("cleanupRemovedRacks() error = %v", err)
	}

	if _, ok := r.migrationCheckFailures[migrationCheckKey(cluster, goneRack)]; ok {
		t.Fatal("removed rack's migration budget survived; a rack re-created with the same ID inherits it")
	}

	// A rack re-created with the same ID must start from a full budget.
	failures, _, hatchOpen := r.recordMigrationCheckFailure(cluster, goneRack)
	if failures != 1 {
		t.Errorf("re-created rack failures = %d, want 1", failures)
	}
	if hatchOpen {
		t.Error("re-created rack opened the escape hatch on its first failure")
	}
}

// TestMigrationCheckState_AgesOutUntouchedEntries pins the leak fix: entries are
// only ever removed by an answering check, so a deleted cluster's entries would
// live for the operator's lifetime. An entry nothing has touched for the TTL is
// swept; an actively failing one — touched every reconcile — is not, even though
// its firstSeen is deliberately older than the grace period.
func TestMigrationCheckState_AgesOutUntouchedEntries(t *testing.T) {
	r, cluster, _ := migrationGateReconciler(t)
	stale := migrationCheckKey(cluster, 7)

	r.migrationCheckFailures = map[string]*migrationCheckState{
		stale: {
			failures:  3,
			firstSeen: time.Now().Add(-2 * migrationCheckStateTTL),
			lastSeen:  time.Now().Add(-2 * migrationCheckStateTTL),
		},
	}

	// Recording against a DIFFERENT rack triggers the sweep.
	r.recordMigrationCheckFailure(cluster, 0)

	if _, ok := r.migrationCheckFailures[stale]; ok {
		t.Error("an entry untouched for longer than the TTL was not swept")
	}

	// A long-running failure that is still being touched must survive, or the
	// sweep would silently reset a budget that is at the hatch threshold.
	live := migrationCheckKey(cluster, 0)
	r.migrationCheckFailures[live].firstSeen = time.Now().Add(-2 * migrationCheckStateTTL)
	r.recordMigrationCheckFailure(cluster, 0)
	if _, ok := r.migrationCheckFailures[live]; !ok {
		t.Error("an actively-failing entry was swept because its firstSeen was old")
	}
}
