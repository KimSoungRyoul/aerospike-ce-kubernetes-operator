package controller

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ackov1alpha1 "github.com/ksr/aerospike-ce-kubernetes-operator/api/v1alpha1"
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
			name:      "9 failures: capped at 2^8 = 256s (exponent capped at 8)",
			failCount: 9,
			want:      256 * time.Second,
		},
		{
			name:      "10 failures: capped at 2^8 = 256s",
			failCount: 10,
			want:      256 * time.Second,
		},
		{
			name:      "100 failures: capped at 2^8 = 256s",
			failCount: 100,
			want:      256 * time.Second,
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
