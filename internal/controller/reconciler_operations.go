package controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	ackov1alpha1 "github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/api/v1alpha1"
)

// reconcileOperations handles on-demand operations (WarmRestart, PodRestart).
// Returns true if an operation is in progress and caller should requeue.
func (r *AerospikeClusterReconciler) reconcileOperations(
	ctx context.Context,
	cluster *ackov1alpha1.AerospikeCluster,
) (bool, error) {
	// If no operations specified, clear status and return
	if len(cluster.Spec.Operations) == 0 {
		return false, nil
	}

	op := cluster.Spec.Operations[0]

	// Check if this operation was already completed
	if cluster.Status.OperationStatus != nil &&
		cluster.Status.OperationStatus.ID == op.ID &&
		cluster.Status.OperationStatus.Phase == ackov1alpha1.AerospikePhaseCompleted {
		return false, nil
	}

	// Check if this operation already errored out
	if cluster.Status.OperationStatus != nil &&
		cluster.Status.OperationStatus.ID == op.ID &&
		cluster.Status.OperationStatus.Phase == ackov1alpha1.AerospikePhaseError {
		return false, nil
	}

	log := logf.FromContext(ctx)
	log.Info("Processing on-demand operation", "kind", op.Kind, "id", op.ID)

	// Get target pods
	pods, err := r.getOperationTargetPods(ctx, cluster, op.PodList)
	if err != nil {
		return false, err
	}

	// Guard against an operation that explicitly targets pods by name but
	// resolves to none of them (typo, stale name, or pods deleted/renamed).
	// Without this, finalizeOperationPhase would see zero outstanding pods and
	// spuriously mark the operation Completed having done nothing — a silent
	// no-op that misleads the operator. An explicit-but-empty target set is a
	// terminal error: surface it as the Error phase plus a Warning event so the
	// next reconcile short-circuits instead of re-reporting a false success.
	if len(op.PodList) > 0 && len(pods) == 0 {
		log.Info("Operation targets named pods but none matched; marking operation as failed",
			"id", op.ID, "kind", op.Kind, "podList", op.PodList)
		errStatus := &ackov1alpha1.OperationStatus{
			ID:    op.ID,
			Kind:  op.Kind,
			Phase: ackov1alpha1.AerospikePhaseError,
		}
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventOperation,
			"Operation %s (%s): none of the targeted pods %v exist in this cluster",
			op.ID, op.Kind, op.PodList)
		if err := r.persistOperationStatus(ctx, cluster, errStatus); err != nil {
			// A conflict means a concurrent writer updated status first; requeue
			// to retry. Other errors propagate to the caller.
			if errors.IsConflict(err) {
				return true, nil
			}
			return false, err
		}
		return false, nil
	}

	// Initialize or update operation status
	opStatus := &ackov1alpha1.OperationStatus{
		ID:    op.ID,
		Kind:  op.Kind,
		Phase: ackov1alpha1.AerospikePhaseInProgress,
	}

	// Get batch size — honor RackConfig.RollingUpdateBatchSize precedence over
	// the legacy spec.rollingUpdateBatchSize field by reusing the same helper
	// used by the rolling restart path.
	batchSize := r.getRollingUpdateBatchSize(cluster, int32(len(pods)))

	// Track completed and previously-failed pods from prior status.
	// A pod that has already been attempted (success OR failure) must not be
	// retried on every reconcile: a failed warm/cold restart would otherwise be
	// re-run every 5s forever with no backoff, and FailedPods would grow
	// unbounded with duplicates.
	completedSet := make(map[string]bool)
	failedSet := make(map[string]bool)
	if cluster.Status.OperationStatus != nil && cluster.Status.OperationStatus.ID == op.ID {
		for _, p := range cluster.Status.OperationStatus.CompletedPods {
			completedSet[p] = true
		}
		for _, p := range cluster.Status.OperationStatus.FailedPods {
			failedSet[p] = true
		}
		opStatus.CompletedPods = cluster.Status.OperationStatus.CompletedPods
		opStatus.FailedPods = cluster.Status.OperationStatus.FailedPods
	}

	// Before restarting any more pods, honor the same readiness/migration guard
	// the rolling-restart path uses (reconciler_restart.go isBatchBlocked). The
	// operations path only fires SIGUSR1 (warm) or issues a Delete (cold) and
	// marks the pod "completed" immediately — neither waits for the node to
	// rejoin the mesh or for migrations to drain. Without this guard, a batched
	// PodRestart on a replication-factor-2 cluster can take a second node down
	// while the first is still cold/migrating, risking data unavailability. If
	// the batch is blocked we requeue (inProgress=true) WITHOUT advancing to the
	// next pod or persisting further "completed" pods. We only gate when work
	// actually remains, so a finished operation can still reach its terminal
	// phase below. The migration check is cluster-wide and the readiness-gate
	// check inspects these pods; see operationBatchBlocked for why this path uses
	// a sentinel rack id rather than 0.
	if r.operationBatchBlocked(ctx, cluster, pods, completedSet, failedSet) {
		return true, nil
	}

	processed := int32(0)

	for _, pod := range pods {
		// Skip pods already resolved (succeeded or failed) in a prior reconcile.
		if completedSet[pod.Name] || failedSet[pod.Name] {
			continue
		}

		if processed >= batchSize {
			break
		}

		var opErr error
		var restartReason ackov1alpha1.RestartReason
		switch op.Kind {
		case ackov1alpha1.OperationWarmRestart:
			opErr = r.warmRestartPod(ctx, pod)
			restartReason = ackov1alpha1.RestartReasonWarmRestart
		case ackov1alpha1.OperationPodRestart:
			opErr = r.coldRestartPod(ctx, cluster, pod)
			restartReason = ackov1alpha1.RestartReasonManualRestart
		default:
			// An unrecognized operation kind must never be reported as a silent
			// success. Today the CRD enum guards op.Kind at the API server, so
			// this is defense-in-depth: if validation is ever bypassed (older
			// CRD, direct status writer, future kind added without a handler),
			// route the pod into the FailedPods path so finalizeOperationPhase
			// drives the operation to the terminal Error phase instead of
			// appending it to CompletedPods having restarted nothing.
			opErr = fmt.Errorf("unsupported operation kind %q", op.Kind)
		}

		if opErr == nil && restartReason != "" {
			r.recordPodRestartStatus(ctx, cluster, pod.Name, restartReason)
		}

		if opErr != nil {
			log.Error(opErr, "Operation failed on pod", "pod", pod.Name, "kind", op.Kind)
			// Dedupe: only record a failed pod once across reconciles.
			if !failedSet[pod.Name] {
				opStatus.FailedPods = append(opStatus.FailedPods, pod.Name)
				failedSet[pod.Name] = true
			}
		} else {
			opStatus.CompletedPods = append(opStatus.CompletedPods, pod.Name)
			completedSet[pod.Name] = true
		}
		processed++
	}

	// Determine completion by directly inspecting how many target pods are still
	// outstanding. A pod counts as resolved once attempted — success OR failure —
	// so an operation with a permanently failing pod still reaches the terminal
	// Error phase instead of requeueing every 5s forever.
	allDone := finalizeOperationPhase(opStatus, pods, completedSet, failedSet)

	// Update operation status using Patch to avoid overwriting concurrent status changes.
	if err := r.persistOperationStatus(ctx, cluster, opStatus); err != nil {
		if errors.IsConflict(err) {
			log.V(1).Info("Conflict patching operation status, will requeue", "operation", op.ID)
			return true, nil
		}
		return !allDone, err
	}

	r.Recorder.Eventf(cluster, corev1.EventTypeNormal, EventOperation,
		"Operation %s (%s): %d/%d pods processed", op.ID, op.Kind, len(opStatus.CompletedPods), len(pods))

	return !allDone, nil
}

// persistOperationStatus writes opStatus to cluster.Status.OperationStatus via a
// MergeFrom patch against a freshly-refetched object, so a concurrent status
// writer is not clobbered. A conflict error is returned to the caller, which
// decides whether to requeue.
func (r *AerospikeClusterReconciler) persistOperationStatus(
	ctx context.Context,
	cluster *ackov1alpha1.AerospikeCluster,
	opStatus *ackov1alpha1.OperationStatus,
) error {
	latest, err := r.refetchCluster(ctx, types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace})
	if err != nil {
		return err
	}
	base := latest.DeepCopy()
	latest.Status.OperationStatus = opStatus
	return r.Status().Patch(ctx, latest, client.MergeFrom(base))
}

// getOperationTargetPods returns the pods targeted by an operation.
func (r *AerospikeClusterReconciler) getOperationTargetPods(
	ctx context.Context,
	cluster *ackov1alpha1.AerospikeCluster,
	podList []string,
) ([]*corev1.Pod, error) {
	allPods, err := r.listClusterPods(ctx, cluster)
	if err != nil {
		return nil, err
	}
	return filterPodsByNames(allPods.Items, podList), nil
}

// finalizeOperationPhase inspects the target pod list against the set of
// resolved pods (completed OR failed) and, when no pods remain outstanding,
// sets opStatus.Phase to Completed (or Error if any pods failed). It returns
// true when the operation has reached a terminal state, false when more
// reconciles are needed.
//
// A failed pod counts as resolved: otherwise a pod whose warm/cold restart
// permanently fails would keep remainingPods > 0 forever, the operation would
// never reach the terminal Error phase, and the cluster would requeue every 5s
// indefinitely while re-restarting the failed pod with no backoff.
//
// Splitting this out lets us unit-test the completion logic without exercising
// the full reconciler — the historical bug (allDone bool flipped to false and
// never restored) hid here for several releases because the reconcile loop is
// hard to test.
func finalizeOperationPhase(
	opStatus *ackov1alpha1.OperationStatus,
	pods []*corev1.Pod,
	completedSet map[string]bool,
	failedSet map[string]bool,
) bool {
	remainingPods := 0
	for _, pod := range pods {
		if !completedSet[pod.Name] && !failedSet[pod.Name] {
			remainingPods++
		}
	}
	if remainingPods > 0 {
		return false
	}
	opStatus.Phase = ackov1alpha1.AerospikePhaseCompleted
	if len(opStatus.FailedPods) > 0 {
		opStatus.Phase = ackov1alpha1.AerospikePhaseError
	}
	return true
}

// operationBatchBlocked reports whether reconcileOperations should hold the
// current batch and requeue without restarting any further pod. It reuses the
// rolling-restart path's isBatchBlocked guard (readiness gate when enabled,
// otherwise a cluster-wide migration check) so an on-demand PodRestart cannot
// take a node down while a previously restarted node is still cold/migrating —
// the same data-availability protection the rolling-restart path already has.
//
// The guard is only consulted when work actually remains (some target pod is
// neither completed nor failed); a finished operation can therefore still reach
// its terminal phase instead of being held behind a blocked-batch requeue.
// The rack id passed to isBatchBlocked is onDemandOperationRackID, a sentinel
// rather than a real rack: this path targets an explicit pod list that can span
// racks, so no single rack owns it. It is NOT logging-only — isBatchBlocked keys
// its migration-check escape-hatch budget on it, and passing 0 made this path
// share a budget with the default rack that getRacks returns for a cluster
// without rackConfig.
func (r *AerospikeClusterReconciler) operationBatchBlocked(
	ctx context.Context,
	cluster *ackov1alpha1.AerospikeCluster,
	pods []*corev1.Pod,
	completedSet, failedSet map[string]bool,
) bool {
	outstanding := false
	for _, pod := range pods {
		if !completedSet[pod.Name] && !failedSet[pod.Name] {
			outstanding = true
			break
		}
	}
	if !outstanding {
		return false
	}
	return r.isBatchBlocked(ctx, cluster, onDemandOperationRackID, derefPods(pods))
}

// derefPods converts a slice of pod pointers into a slice of pod values, as
// required by isBatchBlocked (which the rolling-restart path calls with the
// []corev1.Pod returned by listRackPods). nil pointers are skipped defensively.
func derefPods(pods []*corev1.Pod) []corev1.Pod {
	out := make([]corev1.Pod, 0, len(pods))
	for _, p := range pods {
		if p != nil {
			out = append(out, *p)
		}
	}
	return out
}

// filterPodsByNames returns pointers to the pods matching the given names.
// If names is empty, all pods are returned.
func filterPodsByNames(allPods []corev1.Pod, names []string) []*corev1.Pod {
	if len(names) == 0 {
		result := make([]*corev1.Pod, len(allPods))
		for i := range allPods {
			result[i] = &allPods[i]
		}
		return result
	}

	podMap := make(map[string]*corev1.Pod, len(allPods))
	for i := range allPods {
		podMap[allPods[i].Name] = &allPods[i]
	}

	result := make([]*corev1.Pod, 0, len(names))
	for _, name := range names {
		if pod, ok := podMap[name]; ok {
			result = append(result, pod)
		}
	}
	return result
}
