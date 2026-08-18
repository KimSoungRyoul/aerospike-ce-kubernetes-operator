package controller

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	ackov1alpha1 "github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/api/v1alpha1"
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
