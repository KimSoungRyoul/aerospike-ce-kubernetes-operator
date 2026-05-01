package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// ClusterPhase reports the current phase of each AerospikeCluster as a
	// numeric gauge. The full string-to-float mapping is defined by
	// PhaseToFloat in this package (see below) — keep that function as the
	// single source of truth instead of enumerating phases in the Help text,
	// which goes stale every time a phase is added.
	ClusterPhase = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "acko_cluster_phase",
			Help: "Current phase of the AerospikeCluster as a numeric code; see metrics.PhaseToFloat for the full string-to-value mapping.",
		},
		[]string{"namespace", "name"},
	)

	// ClusterReadyPods reports the number of ready pods per cluster.
	ClusterReadyPods = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "acko_cluster_ready_pods",
			Help: "Number of ready pods in the AerospikeCluster",
		},
		[]string{"namespace", "name"},
	)

	// ReconcileDuration tracks the duration of reconciliation loops.
	ReconcileDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "acko_reconcile_duration_seconds",
			Help:    "Duration of AerospikeCluster reconciliation in seconds",
			Buckets: prometheus.ExponentialBuckets(0.1, 2, 10), // 0.1s to ~51.2s
		},
		[]string{"namespace", "name"},
	)

	// WarmRestartsTotal counts the number of warm restarts (SIGUSR1) performed.
	WarmRestartsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "acko_warm_restarts_total",
			Help: "Total number of warm restarts (SIGUSR1) performed",
		},
		[]string{"namespace", "name"},
	)

	// ColdRestartsTotal counts the number of cold restarts (pod delete) performed.
	ColdRestartsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "acko_cold_restarts_total",
			Help: "Total number of cold restarts (pod delete) performed",
		},
		[]string{"namespace", "name"},
	)

	// DynamicConfigUpdatesTotal counts successful dynamic config updates.
	DynamicConfigUpdatesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "acko_dynamic_config_updates_total",
			Help: "Total number of successful dynamic config updates via set-config",
		},
		[]string{"namespace", "name"},
	)

	// ACLSyncTotal counts the number of ACL synchronizations performed.
	ACLSyncTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "acko_acl_sync_total",
			Help: "Total number of ACL synchronization operations performed",
		},
		[]string{"namespace", "name", "result"},
	)

	// ReconcileErrorsTotal counts reconciliation errors by reason.
	// Reason labels are a bounded set of constants (see ReconcileErrorReason* consts).
	ReconcileErrorsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "acko_reconcile_errors_total",
			Help: "Total number of reconciliation errors by reason",
		},
		[]string{"namespace", "name", "reason"},
	)

	// LastReconcileTimestamp reports the Unix timestamp of the last successful reconciliation.
	// Use with time() to detect staleness: time() - acko_last_reconcile_timestamp_seconds > 300
	LastReconcileTimestamp = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "acko_last_reconcile_timestamp_seconds",
			Help: "Unix timestamp of the last successful reconciliation for each AerospikeCluster",
		},
		[]string{"namespace", "name"},
	)

	// ClusterASSize reports the Aerospike cluster-size as reported by asinfo.
	// This may differ from acko_cluster_ready_pods during split-brain or rolling restarts.
	ClusterASSize = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "acko_cluster_as_size",
			Help: "Aerospike cluster-size reported by asinfo (may differ from K8s pod count)",
		},
		[]string{"namespace", "name"},
	)

	// ScaleDownDeferralsTotal counts the number of scale-down operations
	// deferred due to in-progress data migrations.
	ScaleDownDeferralsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "acko_scaledown_deferrals_total",
			Help: "Total number of scale-down operations deferred due to data migration in progress",
		},
		[]string{"namespace", "name"},
	)

	// ClusterMigratingPartitions reports the total number of partitions
	// remaining to be migrated across all nodes in the cluster.
	ClusterMigratingPartitions = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "acko_cluster_migrating_partitions",
			Help: "Total partitions remaining to be migrated across all cluster nodes",
		},
		[]string{"namespace", "name"},
	)

	// CircuitBreakerActive reports whether the circuit breaker is active (1) or inactive (0)
	// for each AerospikeCluster. The circuit breaker activates after consecutive reconcile failures
	// exceed the threshold (default 10).
	CircuitBreakerActive = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "acko_circuit_breaker_active",
			Help: "Whether the reconcile circuit breaker is active (1=active, 0=inactive)",
		},
		[]string{"namespace", "name"},
	)

	// ClusterPausedTimestamp records the Unix timestamp when reconciliation was paused.
	// Used to compute pause duration when the cluster is resumed. Reset to 0 on resume.
	ClusterPausedTimestamp = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "acko_cluster_paused_timestamp_seconds",
			Help: "Unix timestamp when reconciliation was paused (0 = not paused)",
		},
		[]string{"namespace", "name"},
	)

	// ClusterPausedDuration observes the total duration (in seconds) of each pause/resume cycle.
	ClusterPausedDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "acko_cluster_paused_duration_seconds",
			Help:    "Duration of reconciliation pause cycles in seconds",
			Buckets: prometheus.ExponentialBuckets(1, 2, 15), // 1s to ~16384s (~4.5h)
		},
		[]string{"namespace", "name"},
	)
)

// ReconcileErrorReason constants define bounded labels for ReconcileErrorsTotal.
const (
	ReasonService     = "Service"
	ReasonConfigMap   = "ConfigMap"
	ReasonStatefulSet = "StatefulSet"
	ReasonPDB         = "PDB"
	ReasonRestart     = "Restart"
	ReasonACL         = "ACL"
	ReasonMonitoring  = "Monitoring"
	ReasonNetPolicy   = "NetworkPolicy"
	ReasonStatus      = "Status"
	ReasonOperations  = "Operations"
	ReasonOther       = "Other"
)

// PhaseToFloat converts a phase string to a float64 for the gauge.
func PhaseToFloat(phase string) float64 {
	switch phase {
	case "InProgress":
		return 1
	case "Completed":
		return 2
	case "Error":
		return 3
	case "ScalingUp":
		return 4
	case "ScalingDown":
		return 5
	case "WaitingForMigration":
		return 6
	case "RollingRestart":
		return 7
	case "ACLSync":
		return 8
	case "Paused":
		return 9
	case "Deleting":
		return 10
	case "BackoffActive":
		return 11
	default:
		return 0
	}
}

// CleanupClusterMetrics removes all metrics for a deleted cluster.
func CleanupClusterMetrics(namespace, name string) {
	labels := prometheus.Labels{"namespace": namespace, "name": name}
	ClusterPhase.Delete(labels)
	ClusterReadyPods.Delete(labels)
	ReconcileDuration.Delete(labels)
	WarmRestartsTotal.Delete(labels)
	ColdRestartsTotal.Delete(labels)
	DynamicConfigUpdatesTotal.Delete(labels)
	LastReconcileTimestamp.Delete(labels)
	ClusterASSize.Delete(labels)
	ClusterMigratingPartitions.Delete(labels)
	ScaleDownDeferralsTotal.Delete(labels)
	CircuitBreakerActive.Delete(labels)
	ClusterPausedTimestamp.Delete(labels)
	ClusterPausedDuration.Delete(labels)

	for _, result := range []string{"success", "error"} {
		ACLSyncTotal.Delete(prometheus.Labels{"namespace": namespace, "name": name, "result": result})
	}

	for _, reason := range []string{
		ReasonService, ReasonConfigMap, ReasonStatefulSet, ReasonPDB,
		ReasonRestart, ReasonACL, ReasonMonitoring, ReasonNetPolicy,
		ReasonStatus, ReasonOperations, ReasonOther,
	} {
		ReconcileErrorsTotal.Delete(prometheus.Labels{"namespace": namespace, "name": name, "reason": reason})
	}
}

func init() {
	metrics.Registry.MustRegister(
		ClusterPhase,
		ClusterReadyPods,
		ReconcileDuration,
		WarmRestartsTotal,
		ColdRestartsTotal,
		DynamicConfigUpdatesTotal,
		ACLSyncTotal,
		ReconcileErrorsTotal,
		LastReconcileTimestamp,
		ClusterASSize,
		ClusterMigratingPartitions,
		ScaleDownDeferralsTotal,
		CircuitBreakerActive,
		ClusterPausedTimestamp,
		ClusterPausedDuration,
	)
}
