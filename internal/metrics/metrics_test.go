package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestPhaseToFloat(t *testing.T) {
	tests := []struct {
		phase    string
		expected float64
	}{
		{"InProgress", 1},
		{"Completed", 2},
		{"Error", 3},
		{"ScalingUp", 4},
		{"ScalingDown", 5},
		{"WaitingForMigration", 6},
		{"RollingRestart", 7},
		{"ACLSync", 8},
		{"Paused", 9},
		{"Deleting", 10},
		{"BackoffActive", 11},
		{"", 0},
		{"Unknown", 0},
	}

	for _, tc := range tests {
		if got := PhaseToFloat(tc.phase); got != tc.expected {
			t.Errorf("PhaseToFloat(%q) = %v, expected %v", tc.phase, got, tc.expected)
		}
	}
}

func TestClusterPhaseMetric(t *testing.T) {
	ClusterPhase.WithLabelValues("ns1", "cluster1").Set(2)

	val := testutil.ToFloat64(ClusterPhase.WithLabelValues("ns1", "cluster1"))
	if val != 2 {
		t.Errorf("expected phase=2, got %v", val)
	}

	// Cleanup
	ClusterPhase.Delete(prometheus.Labels{"namespace": "ns1", "name": "cluster1"})
}

func TestClusterReadyPodsMetric(t *testing.T) {
	ClusterReadyPods.WithLabelValues("ns1", "cluster1").Set(3)

	val := testutil.ToFloat64(ClusterReadyPods.WithLabelValues("ns1", "cluster1"))
	if val != 3 {
		t.Errorf("expected ready_pods=3, got %v", val)
	}

	ClusterReadyPods.Delete(prometheus.Labels{"namespace": "ns1", "name": "cluster1"})
}

func TestRestartCounters(t *testing.T) {
	WarmRestartsTotal.WithLabelValues("ns1", "cluster1").Inc()
	WarmRestartsTotal.WithLabelValues("ns1", "cluster1").Inc()

	val := testutil.ToFloat64(WarmRestartsTotal.WithLabelValues("ns1", "cluster1"))
	if val != 2 {
		t.Errorf("expected warm_restarts=2, got %v", val)
	}

	ColdRestartsTotal.WithLabelValues("ns1", "cluster1").Inc()
	val = testutil.ToFloat64(ColdRestartsTotal.WithLabelValues("ns1", "cluster1"))
	if val != 1 {
		t.Errorf("expected cold_restarts=1, got %v", val)
	}

	// Cleanup
	WarmRestartsTotal.Delete(prometheus.Labels{"namespace": "ns1", "name": "cluster1"})
	ColdRestartsTotal.Delete(prometheus.Labels{"namespace": "ns1", "name": "cluster1"})
}

func TestDynamicConfigUpdatesTotal(t *testing.T) {
	DynamicConfigUpdatesTotal.WithLabelValues("ns1", "cluster1").Inc()
	DynamicConfigUpdatesTotal.WithLabelValues("ns1", "cluster1").Inc()
	DynamicConfigUpdatesTotal.WithLabelValues("ns1", "cluster1").Inc()

	val := testutil.ToFloat64(DynamicConfigUpdatesTotal.WithLabelValues("ns1", "cluster1"))
	if val != 3 {
		t.Errorf("expected dynamic_config_updates=3, got %v", val)
	}

	DynamicConfigUpdatesTotal.Delete(prometheus.Labels{"namespace": "ns1", "name": "cluster1"})
}

func TestACLSyncTotal(t *testing.T) {
	ACLSyncTotal.WithLabelValues("ns1", "cluster1", "success").Inc()
	ACLSyncTotal.WithLabelValues("ns1", "cluster1", "success").Inc()
	ACLSyncTotal.WithLabelValues("ns1", "cluster1", "error").Inc()

	successVal := testutil.ToFloat64(ACLSyncTotal.WithLabelValues("ns1", "cluster1", "success"))
	if successVal != 2 {
		t.Errorf("expected acl_sync success=2, got %v", successVal)
	}

	errorVal := testutil.ToFloat64(ACLSyncTotal.WithLabelValues("ns1", "cluster1", "error"))
	if errorVal != 1 {
		t.Errorf("expected acl_sync error=1, got %v", errorVal)
	}

	ACLSyncTotal.Delete(prometheus.Labels{"namespace": "ns1", "name": "cluster1", "result": "success"})
	ACLSyncTotal.Delete(prometheus.Labels{"namespace": "ns1", "name": "cluster1", "result": "error"})
}

func TestReconcileDuration(t *testing.T) {
	ReconcileDuration.WithLabelValues("ns1", "cluster1").Observe(0.5)
	ReconcileDuration.WithLabelValues("ns1", "cluster1").Observe(1.5)

	// Histogram doesn't have a simple ToFloat64 for count, just verify it doesn't panic
	ReconcileDuration.Delete(prometheus.Labels{"namespace": "ns1", "name": "cluster1"})
}

func TestCleanupClusterMetrics(t *testing.T) {
	// Set some metrics
	ClusterPhase.WithLabelValues("ns-cleanup", "test-cluster").Set(2)
	ClusterReadyPods.WithLabelValues("ns-cleanup", "test-cluster").Set(3)
	WarmRestartsTotal.WithLabelValues("ns-cleanup", "test-cluster").Inc()
	ColdRestartsTotal.WithLabelValues("ns-cleanup", "test-cluster").Inc()
	DynamicConfigUpdatesTotal.WithLabelValues("ns-cleanup", "test-cluster").Inc()
	ACLSyncTotal.WithLabelValues("ns-cleanup", "test-cluster", "success").Inc()
	ACLSyncTotal.WithLabelValues("ns-cleanup", "test-cluster", "error").Inc()

	// Cleanup
	CleanupClusterMetrics("ns-cleanup", "test-cluster")

	// After cleanup, the metric should be gone (returns 0 for a non-existent metric)
	val := testutil.ToFloat64(ClusterPhase.WithLabelValues("ns-cleanup", "test-cluster"))
	if val != 0 {
		t.Errorf("expected cleaned-up phase=0, got %v", val)
	}

	val = testutil.ToFloat64(ClusterReadyPods.WithLabelValues("ns-cleanup", "test-cluster"))
	if val != 0 {
		t.Errorf("expected cleaned-up ready_pods=0, got %v", val)
	}

	// Re-cleanup to remove the metric labels re-created by ToFloat64
	ClusterPhase.Delete(prometheus.Labels{"namespace": "ns-cleanup", "name": "test-cluster"})
	ClusterReadyPods.Delete(prometheus.Labels{"namespace": "ns-cleanup", "name": "test-cluster"})
}

func TestCleanupClusterMetrics_NoPanic(t *testing.T) {
	// Cleaning up metrics for a non-existent cluster should not panic
	CleanupClusterMetrics("nonexistent-ns", "nonexistent-cluster")
}

func TestMultipleClusters_IndependentMetrics(t *testing.T) {
	// Verify different clusters have independent metrics
	ClusterPhase.WithLabelValues("ns1", "cluster-a").Set(1)
	ClusterPhase.WithLabelValues("ns1", "cluster-b").Set(2)
	ClusterPhase.WithLabelValues("ns2", "cluster-a").Set(3)

	val1 := testutil.ToFloat64(ClusterPhase.WithLabelValues("ns1", "cluster-a"))
	val2 := testutil.ToFloat64(ClusterPhase.WithLabelValues("ns1", "cluster-b"))
	val3 := testutil.ToFloat64(ClusterPhase.WithLabelValues("ns2", "cluster-a"))

	if val1 != 1 || val2 != 2 || val3 != 3 {
		t.Errorf("metrics should be independent: got %v, %v, %v", val1, val2, val3)
	}

	ClusterPhase.Delete(prometheus.Labels{"namespace": "ns1", "name": "cluster-a"})
	ClusterPhase.Delete(prometheus.Labels{"namespace": "ns1", "name": "cluster-b"})
	ClusterPhase.Delete(prometheus.Labels{"namespace": "ns2", "name": "cluster-a"})
}
