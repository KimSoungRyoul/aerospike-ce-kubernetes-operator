package controller

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	ackov1alpha1 "github.com/ksr/aerospike-ce-kubernetes-operator/api/v1alpha1"
	"github.com/ksr/aerospike-ce-kubernetes-operator/internal/metrics"
)

// HandlePause handles the transition into the paused state. It sets Phase=Paused,
// the ReconciliationPaused condition to True, records a Kubernetes event, and
// sets the pause timestamp metric. Uses RetryOnConflict for robustness.
func (r *AerospikeClusterReconciler) HandlePause(
	ctx context.Context,
	cluster *ackov1alpha1.AerospikeCluster,
) error {
	log := logf.FromContext(ctx)
	nn := types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}

	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		latest, fetchErr := r.refetchCluster(ctx, nn)
		if fetchErr != nil {
			return fetchErr
		}

		latest.Status.Phase = ackov1alpha1.AerospikePhasePaused
		latest.Status.PhaseReason = "Reconciliation paused by user"

		setCondition(latest, ackov1alpha1.ConditionReconciliationPaused, true,
			"ReconciliationPaused", "Reconciliation is paused by user (spec.paused=true)")

		return r.Status().Update(ctx, latest)
	})
	if err != nil {
		return err
	}

	// Record event only on successful update.
	r.Recorder.Event(cluster, corev1.EventTypeNormal, EventPaused,
		"Reconciliation paused by user (spec.paused=true)")

	// Record pause start time for duration tracking.
	metrics.ClusterPausedTimestamp.WithLabelValues(cluster.Namespace, cluster.Name).
		Set(float64(time.Now().Unix()))
	metrics.ClusterPhase.WithLabelValues(cluster.Namespace, cluster.Name).
		Set(metrics.PhaseToFloat(string(ackov1alpha1.AerospikePhasePaused)))

	// Propagate to caller's object.
	cluster.Status.Phase = ackov1alpha1.AerospikePhasePaused
	cluster.Status.PhaseReason = "Reconciliation paused by user"

	log.Info("Reconciliation paused")
	return nil
}

// HandleResume handles the transition from paused to active state. It clears the
// ReconciliationPaused condition, resets stale error state if present, records a
// Kubernetes event, and observes the pause duration metric.
func (r *AerospikeClusterReconciler) HandleResume(
	ctx context.Context,
	cluster *ackov1alpha1.AerospikeCluster,
) error {
	log := logf.FromContext(ctx)
	nn := types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}

	// Capture pause start time from the condition before the retry loop clears it.
	var pauseStartTime time.Time
	for _, c := range cluster.Status.Conditions {
		if c.Type == ackov1alpha1.ConditionReconciliationPaused && c.Status == metav1.ConditionTrue {
			pauseStartTime = c.LastTransitionTime.Time
			break
		}
	}

	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		latest, fetchErr := r.refetchCluster(ctx, nn)
		if fetchErr != nil {
			return fetchErr
		}

		// Clear the ReconciliationPaused condition.
		setCondition(latest, ackov1alpha1.ConditionReconciliationPaused, false,
			"ReconciliationResumed", "Reconciliation resumed")

		// Clear stale error state that was set before pause.
		if latest.Status.Phase == ackov1alpha1.AerospikePhasePaused ||
			latest.Status.Phase == ackov1alpha1.AerospikePhaseError {
			latest.Status.FailedReconcileCount = 0
			latest.Status.LastReconcileError = ""
		}

		return r.Status().Update(ctx, latest)
	})
	if err != nil {
		return err
	}

	// Record event only on successful update.
	r.Recorder.Event(cluster, corev1.EventTypeNormal, EventResumed,
		"Reconciliation resumed")

	// Observe pause duration using the ReconciliationPaused condition's LastTransitionTime
	// (captured before the retry loop cleared it), then reset the timestamp gauge.
	if !pauseStartTime.IsZero() {
		duration := time.Since(pauseStartTime).Seconds()
		metrics.ClusterPausedDuration.WithLabelValues(cluster.Namespace, cluster.Name).Observe(duration)
	}
	metrics.ClusterPausedTimestamp.WithLabelValues(cluster.Namespace, cluster.Name).Set(0)

	// Propagate cleared state back to caller's object so subsequent checks
	// in the same reconcile loop (e.g. circuit breaker) use fresh data.
	cluster.Status.FailedReconcileCount = 0
	cluster.Status.LastReconcileError = ""

	log.Info("Reconciliation resumed from paused state")
	return nil
}
