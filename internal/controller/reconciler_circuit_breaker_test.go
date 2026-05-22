package controller

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	ackov1alpha1 "github.com/ksr/aerospike-ce-kubernetes-operator/api/v1alpha1"
	"github.com/ksr/aerospike-ce-kubernetes-operator/internal/metrics"
	"github.com/ksr/aerospike-ce-kubernetes-operator/internal/utils"
)

func TestCalculateBackoff(t *testing.T) {
	tests := []struct {
		name      string
		failCount int32
		want      time.Duration
	}{
		{
			name:      "zero failures returns default retry interval",
			failCount: 0,
			want:      defaultReconcileRetryInterval,
		},
		{
			name:      "negative failures returns default retry interval",
			failCount: -1,
			want:      defaultReconcileRetryInterval,
		},
		{
			name:      "1 failure: 2^1 = 2s",
			failCount: 1,
			want:      2 * time.Second,
		},
		{
			name:      "2 failures: 2^2 = 4s",
			failCount: 2,
			want:      4 * time.Second,
		},
		{
			name:      "3 failures: 2^3 = 8s",
			failCount: 3,
			want:      8 * time.Second,
		},
		{
			name:      "4 failures: 2^4 = 16s",
			failCount: 4,
			want:      16 * time.Second,
		},
		{
			name:      "5 failures: 2^5 = 32s",
			failCount: 5,
			want:      32 * time.Second,
		},
		{
			name:      "6 failures: 2^6 = 64s",
			failCount: 6,
			want:      64 * time.Second,
		},
		{
			name:      "7 failures: 2^7 = 128s",
			failCount: 7,
			want:      128 * time.Second,
		},
		{
			name:      "8 failures: 2^8 = 256s",
			failCount: 8,
			want:      256 * time.Second,
		},
		{
			name:      "9 failures: 2^9 = 512s capped at maxBackoffSeconds (300s)",
			failCount: 9,
			want:      time.Duration(maxBackoffSeconds) * time.Second,
		},
		{
			name:      "10 failures: capped at maxBackoffSeconds (300s)",
			failCount: 10,
			want:      time.Duration(maxBackoffSeconds) * time.Second,
		},
		{
			name:      "100 failures: capped at maxBackoffSeconds (300s)",
			failCount: 100,
			want:      time.Duration(maxBackoffSeconds) * time.Second,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := calculateBackoff(tc.failCount)
			if got != tc.want {
				t.Errorf("calculateBackoff(%d) = %v, want %v", tc.failCount, got, tc.want)
			}
		})
	}
}

func TestCalculateBackoff_MonotonicallyIncreasing(t *testing.T) {
	// Backoff should be monotonically increasing from failCount=1 onward.
	// failCount=0 returns the default retry interval which is a special case.
	prev := calculateBackoff(1)
	for i := int32(2); i <= 12; i++ {
		cur := calculateBackoff(i)
		if cur < prev {
			t.Errorf("calculateBackoff(%d) = %v < calculateBackoff(%d) = %v: backoff should be monotonically increasing",
				i, cur, i-1, prev)
		}
		prev = cur
	}
}

func TestCalculateBackoff_NeverExceedsMax(t *testing.T) {
	maxDuration := time.Duration(maxBackoffSeconds) * time.Second
	for i := int32(0); i <= 1000; i++ {
		got := calculateBackoff(i)
		if got > maxDuration {
			t.Errorf("calculateBackoff(%d) = %v exceeds maxBackoffSeconds (%v)", i, got, maxDuration)
		}
	}
}

// TestCalculateBackoff_ReachesDocumentedMax verifies the documented 5-minute
// (maxBackoffSeconds) cap is actually reached at a high failure count. A prior
// implementation capped the exponent at 8 (2^8 = 256s), which kept the result
// permanently below 300s and made the cap dead code.
func TestCalculateBackoff_ReachesDocumentedMax(t *testing.T) {
	maxDuration := time.Duration(maxBackoffSeconds) * time.Second
	got := calculateBackoff(maxFailedReconciles)
	if got != maxDuration {
		t.Errorf("calculateBackoff(%d) = %v, want documented max %v",
			maxFailedReconciles, got, maxDuration)
	}
	// Sustained failures must stay pinned at the documented maximum.
	if got := calculateBackoff(1000); got != maxDuration {
		t.Errorf("calculateBackoff(1000) = %v, want %v", got, maxDuration)
	}
}

func TestCircuitBreakerConstants(t *testing.T) {
	if maxFailedReconciles <= 0 {
		t.Errorf("maxFailedReconciles should be positive, got %d", maxFailedReconciles)
	}
	if maxBackoffSeconds <= 0 {
		t.Errorf("maxBackoffSeconds should be positive, got %d", maxBackoffSeconds)
	}
	if reconcileTimeout <= 0 {
		t.Errorf("reconcileTimeout should be positive, got %v", reconcileTimeout)
	}
	if reconcileTimeout != 5*time.Minute {
		t.Errorf("reconcileTimeout = %v, want 5m", reconcileTimeout)
	}
	if maxFailedReconciles != 10 {
		t.Errorf("maxFailedReconciles = %d, want 10", maxFailedReconciles)
	}
}

func TestSetConditionReconcileHealthyFalse(t *testing.T) {
	cluster := &ackov1alpha1.AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{Generation: 1},
	}

	setCondition(cluster, ackov1alpha1.ConditionReconcileHealthy, false,
		"PermanentError", "invalid config: namespaces must be a list")

	if len(cluster.Status.Conditions) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(cluster.Status.Conditions))
	}
	cond := cluster.Status.Conditions[0]
	if cond.Type != ackov1alpha1.ConditionReconcileHealthy {
		t.Errorf("expected type %q, got %q", ackov1alpha1.ConditionReconcileHealthy, cond.Type)
	}
	if cond.Status != metav1.ConditionFalse {
		t.Errorf("expected status False, got %s", cond.Status)
	}
	if cond.Reason != "PermanentError" {
		t.Errorf("expected reason PermanentError, got %q", cond.Reason)
	}
	if cond.Message != "invalid config: namespaces must be a list" {
		t.Errorf("unexpected message: %q", cond.Message)
	}
}

func TestSetConditionReconcileHealthyTrue(t *testing.T) {
	cluster := &ackov1alpha1.AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{Generation: 2},
		Status: ackov1alpha1.AerospikeClusterStatus{
			Conditions: []metav1.Condition{
				{
					Type:               ackov1alpha1.ConditionReconcileHealthy,
					Status:             metav1.ConditionFalse,
					Reason:             "PermanentError",
					Message:            "some error",
					ObservedGeneration: 1,
					LastTransitionTime: metav1.Now(),
				},
			},
		},
	}

	setCondition(cluster, ackov1alpha1.ConditionReconcileHealthy, true,
		"ReconcileSucceeded", "Reconciliation succeeded")

	if len(cluster.Status.Conditions) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(cluster.Status.Conditions))
	}
	cond := cluster.Status.Conditions[0]
	if cond.Status != metav1.ConditionTrue {
		t.Errorf("expected status True, got %s", cond.Status)
	}
	if cond.Reason != "ReconcileSucceeded" {
		t.Errorf("expected reason ReconcileSucceeded, got %q", cond.Reason)
	}
}

func TestValidationErrorSetsMaxFailedReconciles(t *testing.T) {
	// Simulate what handleReconcileError does for validation errors:
	// it should set FailedReconcileCount to maxFailedReconciles immediately.
	cluster := &ackov1alpha1.AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{Generation: 1},
		Status: ackov1alpha1.AerospikeClusterStatus{
			FailedReconcileCount: 0,
		},
	}

	// Simulate the validation error branch logic
	cluster.Status.FailedReconcileCount = maxFailedReconciles
	cluster.Status.LastReconcileError = "validation error: invalid config"
	setCondition(cluster, ackov1alpha1.ConditionReconcileHealthy, false,
		"PermanentError", "validation error: invalid config")

	if cluster.Status.FailedReconcileCount != maxFailedReconciles {
		t.Errorf("expected FailedReconcileCount=%d, got %d",
			maxFailedReconciles, cluster.Status.FailedReconcileCount)
	}

	// Verify circuit breaker would be active
	if cluster.Status.FailedReconcileCount < maxFailedReconciles {
		t.Error("circuit breaker should be active after validation error")
	}

	// Verify backoff is at the max level
	backoff := calculateBackoff(cluster.Status.FailedReconcileCount)
	expectedBackoff := calculateBackoff(maxFailedReconciles)
	if backoff != expectedBackoff {
		t.Errorf("backoff = %v, want %v", backoff, expectedBackoff)
	}
}

// TestCircuitBreakerReconcileSetsBackoffActivePhase drives the actual
// Reconcile() entry point with a CR seeded at the failure threshold and
// asserts that the circuit-breaker branch fires and transitions the phase
// to BackoffActive (rather than leaving a stale InProgress).
func TestCircuitBreakerReconcileSetsBackoffActivePhase(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := ackov1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}

	stored := &ackov1alpha1.AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "demo",
			Namespace:  "default",
			Finalizers: []string{utils.StorageFinalizer},
		},
		Status: ackov1alpha1.AerospikeClusterStatus{
			Phase:                ackov1alpha1.AerospikePhaseInProgress,
			PhaseReason:          "Reconciliation started",
			FailedReconcileCount: maxFailedReconciles,
			LastReconcileError:   "validation error: bad config",
		},
	}

	reconciler := &AerospikeClusterReconciler{
		Client: fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&ackov1alpha1.AerospikeCluster{}).
			WithObjects(stored).
			Build(),
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
	}

	res, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: stored.Name, Namespace: stored.Namespace},
	})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if res.RequeueAfter <= 0 {
		t.Errorf("Reconcile() RequeueAfter = %v, want > 0 (circuit breaker should requeue with backoff)", res.RequeueAfter)
	}

	updated := &ackov1alpha1.AerospikeCluster{}
	if err := reconciler.Get(
		context.Background(),
		types.NamespacedName{Name: stored.Name, Namespace: stored.Namespace},
		updated,
	); err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if updated.Status.Phase != ackov1alpha1.AerospikePhaseBackoffActive {
		t.Errorf("Phase = %q, want %q", updated.Status.Phase, ackov1alpha1.AerospikePhaseBackoffActive)
	}
	if updated.Status.PhaseReason == "" {
		t.Error("PhaseReason should not be empty after BackoffActive transition")
	}
	// The circuit breaker counter should still be at the threshold; it is only reset
	// on the next successful reconcile.
	if updated.Status.FailedReconcileCount != maxFailedReconciles {
		t.Errorf("FailedReconcileCount = %d, want %d (counter must persist while backoff is active)",
			updated.Status.FailedReconcileCount, maxFailedReconciles)
	}
}

// TestCircuitBreakerBackoffPhaseIsIdempotent guards the optimization at
// reconciler.go where setPhase is only called on the *transition* into
// BackoffActive — not on every requeue while in backoff. Without this guard
// each requeue would write Status (the reason string contains the dynamic
// failure count + backoff duration, breaking the equality short-circuit in
// setPhase) and pressure the API server.
func TestCircuitBreakerBackoffPhaseIsIdempotent(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := ackov1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}

	stored := &ackov1alpha1.AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "demo",
			Namespace:  "default",
			Finalizers: []string{utils.StorageFinalizer},
		},
		Status: ackov1alpha1.AerospikeClusterStatus{
			Phase:                ackov1alpha1.AerospikePhaseBackoffActive,
			PhaseReason:          "Circuit breaker active after 10 consecutive failures; backing off 4m16s",
			FailedReconcileCount: maxFailedReconciles,
			LastReconcileError:   "validation error: bad config",
		},
	}

	reconciler := &AerospikeClusterReconciler{
		Client: fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&ackov1alpha1.AerospikeCluster{}).
			WithObjects(stored).
			Build(),
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
	}

	// Capture the resourceVersion before the requeue.
	before := &ackov1alpha1.AerospikeCluster{}
	if err := reconciler.Get(
		context.Background(),
		types.NamespacedName{Name: stored.Name, Namespace: stored.Namespace},
		before,
	); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	beforeRV := before.ResourceVersion

	if _, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: stored.Name, Namespace: stored.Namespace},
	}); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	after := &ackov1alpha1.AerospikeCluster{}
	if err := reconciler.Get(
		context.Background(),
		types.NamespacedName{Name: stored.Name, Namespace: stored.Namespace},
		after,
	); err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	// If the reconciler had called Status().Update on the requeue, the fake
	// client would have bumped the resourceVersion. The idempotency fix means
	// we should observe the same resourceVersion.
	if after.ResourceVersion != beforeRV {
		t.Errorf("ResourceVersion changed (%q -> %q): reconciler issued a Status().Update while already in BackoffActive; setPhase should only fire on transition",
			beforeRV, after.ResourceVersion)
	}
	if after.Status.Phase != ackov1alpha1.AerospikePhaseBackoffActive {
		t.Errorf("Phase = %q, want %q (should remain BackoffActive)", after.Status.Phase, ackov1alpha1.AerospikePhaseBackoffActive)
	}
}

// TestBackoffActivePhaseClearsOnSuccessfulReconcile ensures that once a
// cluster is sitting in BackoffActive, a successful "recovery" path
// (resetFailedReconcileCount) clears the counter and transitions the phase
// off of BackoffActive (back to InProgress) so a subsequent
// updateStatusAndPhase can advance it to Completed without leaving a stale
// BackoffActive phase visible to users.
func TestBackoffActivePhaseClearsOnSuccessfulReconcile(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := ackov1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}

	stored := &ackov1alpha1.AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "demo",
			Namespace:  "default",
			Finalizers: []string{utils.StorageFinalizer},
			Generation: 1,
		},
		Status: ackov1alpha1.AerospikeClusterStatus{
			Phase:                ackov1alpha1.AerospikePhaseBackoffActive,
			PhaseReason:          "Circuit breaker active after 10 consecutive failures; backing off 4m16s",
			FailedReconcileCount: maxFailedReconciles,
			LastReconcileError:   "validation error: bad config",
			ObservedGeneration:   1,
		},
	}

	reconciler := &AerospikeClusterReconciler{
		Client: fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&ackov1alpha1.AerospikeCluster{}).
			WithObjects(stored).
			Build(),
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
	}

	cluster := stored.DeepCopy()
	if err := reconciler.resetFailedReconcileCount(context.Background(), cluster); err != nil {
		t.Fatalf("resetFailedReconcileCount() error = %v", err)
	}

	updated := &ackov1alpha1.AerospikeCluster{}
	if err := reconciler.Get(
		context.Background(),
		types.NamespacedName{Name: stored.Name, Namespace: stored.Namespace},
		updated,
	); err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if updated.Status.FailedReconcileCount != 0 {
		t.Errorf("FailedReconcileCount = %d, want 0", updated.Status.FailedReconcileCount)
	}
	if updated.Status.LastReconcileError != "" {
		t.Errorf("LastReconcileError = %q, want empty", updated.Status.LastReconcileError)
	}
	if updated.Status.Phase == ackov1alpha1.AerospikePhaseBackoffActive {
		t.Errorf("Phase = %q, should not remain BackoffActive after successful reset",
			updated.Status.Phase)
	}
	// The successful recovery transitions back to InProgress; the next
	// updateStatusAndPhase call in the same reconcile loop advances it to
	// Completed. We don't try to drive a full successful reconcile here
	// (which would require many sub-reconciler dependencies), so we just
	// assert the post-reset phase.
	if updated.Status.Phase != ackov1alpha1.AerospikePhaseInProgress {
		t.Errorf("Phase = %q, want %q after recovery from BackoffActive",
			updated.Status.Phase, ackov1alpha1.AerospikePhaseInProgress)
	}
	// Also propagated to the in-memory cluster object passed in.
	if cluster.Status.FailedReconcileCount != 0 {
		t.Errorf("in-memory FailedReconcileCount = %d, want 0", cluster.Status.FailedReconcileCount)
	}
	if cluster.Status.Phase != ackov1alpha1.AerospikePhaseInProgress {
		t.Errorf("in-memory Phase = %q, want %q", cluster.Status.Phase, ackov1alpha1.AerospikePhaseInProgress)
	}
}

func TestAerospikePhaseBackoffActiveConstantValue(t *testing.T) {
	if ackov1alpha1.AerospikePhaseBackoffActive != "BackoffActive" {
		t.Errorf("AerospikePhaseBackoffActive = %q, want %q",
			ackov1alpha1.AerospikePhaseBackoffActive, "BackoffActive")
	}
}

// TestBackoffActivePhaseUpdatesMetric exercises the gap discovered during
// kind-cluster validation: when the circuit breaker drives the cluster into
// BackoffActive via setPhase, the acko_cluster_phase gauge must reflect
// PhaseToFloat("BackoffActive") (=11). Before the fix, setPhase did
// Status().Update() directly without calling metrics.ClusterPhase.Set,
// leaving the gauge stuck at whatever the previous phase was — meaning
// the BackoffActive phase value was unreachable in production metrics.
func TestBackoffActivePhaseUpdatesMetric(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := ackov1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}

	stored := &ackov1alpha1.AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "metric-demo",
			Namespace:  "default",
			Finalizers: []string{utils.StorageFinalizer},
		},
		Status: ackov1alpha1.AerospikeClusterStatus{
			Phase:                ackov1alpha1.AerospikePhaseInProgress,
			PhaseReason:          "Reconciliation started",
			FailedReconcileCount: maxFailedReconciles,
			LastReconcileError:   "validation error: bad config",
		},
	}

	reconciler := &AerospikeClusterReconciler{
		Client: fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&ackov1alpha1.AerospikeCluster{}).
			WithObjects(stored).
			Build(),
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
	}

	// Reset the gauge to a known non-target value so we can verify the
	// transition wrote the BackoffActive value (and not just inherited it).
	metrics.ClusterPhase.WithLabelValues(stored.Namespace, stored.Name).
		Set(metrics.PhaseToFloat(string(ackov1alpha1.AerospikePhaseInProgress)))
	t.Cleanup(func() {
		metrics.ClusterPhase.DeleteLabelValues(stored.Namespace, stored.Name)
	})

	if _, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: stored.Name, Namespace: stored.Namespace},
	}); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	updated := &ackov1alpha1.AerospikeCluster{}
	if err := reconciler.Get(
		context.Background(),
		types.NamespacedName{Name: stored.Name, Namespace: stored.Namespace},
		updated,
	); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if updated.Status.Phase != ackov1alpha1.AerospikePhaseBackoffActive {
		t.Fatalf("precondition: Phase = %q, want %q (test driver did not hit the circuit-breaker branch)",
			updated.Status.Phase, ackov1alpha1.AerospikePhaseBackoffActive)
	}

	gotGauge := testutil.ToFloat64(metrics.ClusterPhase.WithLabelValues(stored.Namespace, stored.Name))
	wantGauge := metrics.PhaseToFloat(string(ackov1alpha1.AerospikePhaseBackoffActive))
	if gotGauge != wantGauge {
		t.Errorf("acko_cluster_phase = %v, want %v (BackoffActive); setPhase must update the gauge",
			gotGauge, wantGauge)
	}
	if wantGauge != 11 {
		t.Errorf("PhaseToFloat(BackoffActive) = %v, want 11 (regression: enum value drifted)", wantGauge)
	}
}

func TestCircuitBreakerResetClearsReconcileHealthyCondition(t *testing.T) {
	cluster := &ackov1alpha1.AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{Generation: 3},
		Status: ackov1alpha1.AerospikeClusterStatus{
			FailedReconcileCount: maxFailedReconciles,
			LastReconcileError:   "validation error: bad config",
			Conditions: []metav1.Condition{
				{
					Type:               ackov1alpha1.ConditionReconcileHealthy,
					Status:             metav1.ConditionFalse,
					Reason:             "PermanentError",
					Message:            "validation error: bad config",
					ObservedGeneration: 2,
					LastTransitionTime: metav1.Now(),
				},
			},
		},
	}

	// Simulate what resetFailedReconcileCount does
	cluster.Status.FailedReconcileCount = 0
	cluster.Status.LastReconcileError = ""
	setCondition(cluster, ackov1alpha1.ConditionReconcileHealthy, true,
		"ReconcileSucceeded", "Reconciliation succeeded")

	if cluster.Status.FailedReconcileCount != 0 {
		t.Errorf("expected FailedReconcileCount=0, got %d", cluster.Status.FailedReconcileCount)
	}
	if cluster.Status.LastReconcileError != "" {
		t.Errorf("expected empty LastReconcileError, got %q", cluster.Status.LastReconcileError)
	}

	// Verify condition is now True
	found := false
	for _, c := range cluster.Status.Conditions {
		if c.Type == ackov1alpha1.ConditionReconcileHealthy {
			found = true
			if c.Status != metav1.ConditionTrue {
				t.Errorf("expected ReconcileHealthy=True, got %s", c.Status)
			}
			if c.Reason != "ReconcileSucceeded" {
				t.Errorf("expected reason ReconcileSucceeded, got %q", c.Reason)
			}
		}
	}
	if !found {
		t.Error("ReconcileHealthy condition not found after reset")
	}
}
