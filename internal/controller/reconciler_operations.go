package controller

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	ackov1alpha1 "github.com/ksr/aerospike-ce-kubernetes-operator/api/v1alpha1"
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

	// Track completed pods from previous status
	completedSet := make(map[string]bool)
	if cluster.Status.OperationStatus != nil && cluster.Status.OperationStatus.ID == op.ID {
		for _, p := range cluster.Status.OperationStatus.CompletedPods {
			completedSet[p] = true
		}
		opStatus.CompletedPods = cluster.Status.OperationStatus.CompletedPods
		opStatus.FailedPods = cluster.Status.OperationStatus.FailedPods
	}

	processed := int32(0)

	for _, pod := range pods {
		if completedSet[pod.Name] {
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
		}

		if opErr == nil && restartReason != "" {
			r.recordPodRestartStatus(ctx, cluster, pod.Name, restartReason)
		}

		if opErr != nil {
			log.Error(opErr, "Operation failed on pod", "pod", pod.Name, "kind", op.Kind)
			opStatus.FailedPods = append(opStatus.FailedPods, pod.Name)
		} else {
			opStatus.CompletedPods = append(opStatus.CompletedPods, pod.Name)
			completedSet[pod.Name] = true
		}
		processed++
	}

	// Determine completion by directly inspecting how many target pods are still
	// outstanding. The previous implementation flipped a bool to false on the
	// first incomplete pod and never restored it, leaving the phase stuck on
	// InProgress forever once any pod fell out of completedSet — a permanent
	// regression for any operation that needed more than one reconcile loop.
	allDone := finalizeOperationPhase(opStatus, pods, completedSet)

	// Update operation status using Patch to avoid overwriting concurrent status changes.
	latest, err := r.refetchCluster(ctx, types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace})
	if err != nil {
		return !allDone, err
	}
	base := latest.DeepCopy()
	latest.Status.OperationStatus = opStatus
	if err := r.Status().Patch(ctx, latest, client.MergeFrom(base)); err != nil {
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

// finalizeOperationPhase inspects the target pod list against the completed set
// and, when no pods remain outstanding, sets opStatus.Phase to Completed (or
// Error if any pods failed). It returns true when the operation has reached a
// terminal state, false when more reconciles are needed.
//
// Splitting this out lets us unit-test the completion logic without exercising
// the full reconciler — the historical bug (allDone bool flipped to false and
// never restored) hid here for several releases because the reconcile loop is
// hard to test.
func finalizeOperationPhase(
	opStatus *ackov1alpha1.OperationStatus,
	pods []*corev1.Pod,
	completedSet map[string]bool,
) bool {
	remainingPods := 0
	for _, pod := range pods {
		if !completedSet[pod.Name] {
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
