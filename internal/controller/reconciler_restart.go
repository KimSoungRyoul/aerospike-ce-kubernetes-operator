package controller

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	aero "github.com/aerospike/aerospike-client-go/v8"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	ackov1alpha1 "github.com/ksr/aerospike-ce-kubernetes-operator/api/v1alpha1"
	"github.com/ksr/aerospike-ce-kubernetes-operator/internal/metrics"
	"github.com/ksr/aerospike-ce-kubernetes-operator/internal/podutil"
	"github.com/ksr/aerospike-ce-kubernetes-operator/internal/storage"
	"github.com/ksr/aerospike-ce-kubernetes-operator/internal/utils"
)

// maxPodUnstableDuration is the threshold after which a pod stuck in a non-ready
// state is skipped from the rolling restart to avoid blocking healthy pods.
const maxPodUnstableDuration = 10 * time.Minute

// reconcileRollingRestart checks if pods need restart due to config changes.
// Returns true if a restart was triggered (caller should requeue).
// Supports batch restart via spec.rollingUpdateBatchSize.
//
// Precedence: dynamic config update > warm restart (SIGUSR1) > cold restart (pod delete).
func (r *AerospikeClusterReconciler) reconcileRollingRestart(
	ctx context.Context,
	cluster *ackov1alpha1.AerospikeCluster,
	rack *ackov1alpha1.Rack,
) (bool, error) {
	log := logf.FromContext(ctx)

	stsName := utils.StatefulSetName(cluster.Name, rack.ID)
	log = log.WithValues("rack", rack.ID, "statefulset", stsName)

	sts := &appsv1.StatefulSet{}
	if err := r.Get(ctx, types.NamespacedName{Name: stsName, Namespace: cluster.Namespace}, sts); err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	// Get desired config hash from the StatefulSet template
	desiredHash := ""
	if sts.Spec.Template.Annotations != nil {
		desiredHash = sts.Spec.Template.Annotations[utils.ConfigHashAnnotation]
	}

	if desiredHash == "" {
		return false, nil
	}

	// Compute the old and new config for dynamic config comparison.
	// Old config comes from the CR's last-applied status; new config from the spec.
	var oldConfig, newConfig map[string]any
	if cluster.Status.AerospikeConfig != nil {
		oldConfig = cluster.Status.AerospikeConfig.Value
	}
	if cluster.Spec.AerospikeConfig != nil {
		newConfig = cluster.Spec.AerospikeConfig.Value
	}

	// Collect pods that need restart (reverse order = highest ordinal first)
	replicas := int32(0)
	if sts.Spec.Replicas != nil {
		replicas = *sts.Spec.Replicas
	}

	batchSize := r.getRollingUpdateBatchSize(cluster, replicas)
	maxIgnorablePods := r.getMaxIgnorablePods(cluster, replicas)

	// Fetch all pods for this rack in a single List call instead of N individual Get calls.
	rackPods, err := r.listRackPods(ctx, cluster, rack.ID)
	if err != nil {
		return false, fmt.Errorf("listing rack pods for rolling restart: %w", err)
	}

	var podsToRestart []*corev1.Pod
	ignoredCount := int32(0)
	for i := range rackPods {
		pod := &rackPods[i]

		// Skip pending/failed pods if within ignorable limit
		if pod.Status.Phase == corev1.PodPending || pod.Status.Phase == corev1.PodFailed {
			if ignoredCount < maxIgnorablePods {
				ignoredCount++
				log.V(1).Info("Ignoring pending/failed pod", "pod", pod.Name)
				continue
			}
		}

		// Skip pods that have been in a non-ready state longer than the threshold.
		// This prevents a stuck pod from blocking healthy pods in the same rack.
		if ps, ok := cluster.Status.Pods[pod.Name]; ok && ps.UnstableSince != nil {
			if time.Since(ps.UnstableSince.Time) > maxPodUnstableDuration {
				log.Info("Pod stuck in non-ready state beyond threshold, skipping restart",
					"pod", pod.Name, "unstableSince", ps.UnstableSince.Time,
					"threshold", maxPodUnstableDuration)
				r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventRestartFailed,
					"Pod %s stuck in non-ready state since %v (>%v), skipping restart",
					pod.Name, ps.UnstableSince.Time, maxPodUnstableDuration)
				continue
			}
		}

		currentHash := ""
		if pod.Annotations != nil {
			currentHash = pod.Annotations[utils.ConfigHashAnnotation]
		}

		if currentHash != desiredHash {
			podsToRestart = append(podsToRestart, pod)
		}
	}

	if len(podsToRestart) == 0 {
		cluster.Status.PendingRestartPods = nil
		return false, nil
	}

	// Track pods pending restart
	pendingNames := make([]string, 0, len(podsToRestart))
	for _, pod := range podsToRestart {
		pendingNames = append(pendingNames, pod.Name)
	}

	r.Recorder.Eventf(cluster, corev1.EventTypeNormal, EventRollingRestartStarted,
		"Rolling restart started for rack %d: %d pods to restart", rack.ID, len(podsToRestart))

	// Create Aerospike client once for all pods (lazy, only if dynamic config is attempted).
	var aeroClient *aero.Client
	defer func() {
		if aeroClient != nil {
			closeAerospikeClient(aeroClient)
		}
	}()

	// Publish the truly-pending pods before any early return so observers see
	// them even while the batch is blocked on migration or readiness gates.
	cluster.Status.PendingRestartPods = pendingNames

	// Hold the next batch when migration or readiness gates are blocking.
	if r.isBatchBlocked(ctx, cluster, rack.ID, rackPods) {
		return true, nil
	}

	// Restart up to batchSize pods, continuing on individual pod failures.
	restarted, failedPods, batchPods := r.restartPodBatch(ctx, cluster, podsToRestart, sts, desiredHash,
		batchSize, oldConfig, newConfig, &aeroClient)

	// Update PendingRestartPods to only include pods that were NOT successfully restarted.
	if len(failedPods) > 0 || restarted > 0 {
		remaining := filterUnrestarted(pendingNames, failedPods, restarted, batchPods)
		cluster.Status.PendingRestartPods = remaining
	}

	// If all attempted pods failed, return error to signal a full batch failure.
	if len(failedPods) > 0 && restarted == 0 {
		return false, fmt.Errorf("all %d pod restart(s) in batch failed: %v",
			len(failedPods), strings.Join(failedPods, ", "))
	}

	if len(failedPods) > 0 {
		log.Info("Partial batch restart failure, some pods restarted successfully",
			"restarted", restarted, "failed", len(failedPods), "failedPods", strings.Join(failedPods, ", "))
	}

	if restarted >= int32(len(podsToRestart)) {
		r.Recorder.Eventf(cluster, corev1.EventTypeNormal, EventRollingRestartCompleted,
			"Rolling restart completed for rack %d: all %d pods restarted", rack.ID, restarted)
	}

	return restarted > 0, nil
}

// podDynamicUpdate tracks a pod that received a successful dynamic config update,
// along with the node and applied changes needed for cross-pod rollback.
type podDynamicUpdate struct {
	podName string
	node    *aero.Node
	applied []appliedChange
}

// restartPodBatch attempts to restart up to batchSize pods, trying dynamic config first.
// Dynamic config updates count against the batch size limit. If any pod fails a cold/warm
// restart after other pods were dynamically updated in the same batch, those dynamic
// updates are rolled back for consistency.
// Returns the count of successfully processed pods, names of failed pods, and the
// actual batch slice considered (subset of podsToRestart up to batchSize). The caller
// passes the batch back to filterUnrestarted so that pods which were never attempted
// in this reconcile pass remain in the pending list rather than being mis-classified
// as restarted.
func (r *AerospikeClusterReconciler) restartPodBatch(
	ctx context.Context,
	cluster *ackov1alpha1.AerospikeCluster,
	podsToRestart []*corev1.Pod,
	sts *appsv1.StatefulSet,
	desiredHash string,
	batchSize int32,
	oldConfig, newConfig map[string]any,
	aeroClient **aero.Client,
) (int32, []string, []*corev1.Pod) {
	log := logf.FromContext(ctx)

	restarted := int32(0)
	var failedPods []string
	attempted := int32(0)
	var dynamicUpdated []podDynamicUpdate

	// Determine the batch of pods to process
	var batchPods []*corev1.Pod
	for _, pod := range podsToRestart {
		if int32(len(batchPods)) >= batchSize {
			break
		}
		batchPods = append(batchPods, pod)
	}

	// 1. Try 2PC batch dynamic config update for all pods in the batch
	if oldConfig != nil && newConfig != nil {
		if *aeroClient == nil {
			var clientErr error
			*aeroClient, clientErr = r.getAerospikeClient(ctx, cluster)
			if clientErr != nil {
				log.V(1).Info("Could not create Aerospike client for dynamic config, will fall back to restart", "error", clientErr)
			}
		}
		if *aeroClient != nil {
			allOk, _, rbResult := r.tryDynamicConfigUpdateBatch(ctx, cluster, batchPods, oldConfig, newConfig, *aeroClient)
			if allOk {
				log.Info("2PC batch dynamic config update succeeded for all pods", "podCount", len(batchPods))
				return int32(len(batchPods)), nil, batchPods
			}

			// If rollback failed, set ConfigDegraded phase
			if rbResult != nil && rbResult.HasFailures() {
				log.Info("2PC batch rollback had failures, setting ConfigDegraded",
					"failedPods", rbResult.FailedPods, "successCount", rbResult.SuccessCount)
				r.setConfigDegraded(ctx, cluster, rbResult.FailedPods)
			}

			// On any 2PC failure (validation abort, apply abort, or successful
			// rollback) every pod returned in `updates` is either not-applied
			// or already-rolled-back. Adding them to dynamicUpdated would
			// inflate `restarted` below, and filterUnrestarted would then
			// drop them from the pending queue — the pods would silently
			// never be cold-restarted, leaving config-hash mismatch forever.
			// Keep dynamicUpdated at its zero value (nil) so the per-pod
			// fallback restart loop covers every batch member.
			dynamicUpdated = nil
		}
	}

	// 2. Fall back to per-pod restart (warm or cold) for pods not dynamically updated
	dynamicSet := make(map[string]bool, len(dynamicUpdated))
	for _, du := range dynamicUpdated {
		dynamicSet[du.podName] = true
	}
	restarted += int32(len(dynamicUpdated))

	for _, pod := range batchPods {
		if dynamicSet[pod.Name] {
			continue // Already handled by dynamic update
		}
		attempted++

		if err := r.restartPod(ctx, cluster, pod, sts, desiredHash); err != nil {
			r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventRestartFailed,
				"Failed to restart pod %s: %v", pod.Name, err)
			failedPods = append(failedPods, pod.Name)
			log.Error(err, "Pod restart failed, continuing with next pod", "pod", pod.Name)
			continue
		}
		restarted++
	}

	// Cross-pod rollback: if any cold/warm restart failed AND dynamic updates were
	// applied in this batch, roll back those dynamic changes for consistency.
	if len(failedPods) > 0 && len(dynamicUpdated) > 0 {
		log.Info("Rolling back dynamic config updates due to batch restart failures",
			"dynamicUpdated", len(dynamicUpdated), "failedPods", len(failedPods))
		rbResult := r.rollbackDynamicChangesBatch(log, cluster, dynamicUpdated)
		if rbResult.HasFailures() {
			r.setConfigDegraded(ctx, cluster, rbResult.FailedPods)
		}
	}

	return restarted, failedPods, batchPods
}

// restartPod attempts a warm restart first, falling back to cold restart.
func (r *AerospikeClusterReconciler) restartPod(
	ctx context.Context,
	cluster *ackov1alpha1.AerospikeCluster,
	pod *corev1.Pod,
	sts *appsv1.StatefulSet,
	desiredHash string,
) error {
	log := logf.FromContext(ctx)

	isWarm := r.shouldWarmRestart(cluster, pod, sts)

	// Determine desired image and hashes for restart reason
	desiredImage := cluster.Spec.Image
	desiredPodSpecHash := ""
	if sts.Spec.Template.Annotations != nil {
		desiredPodSpecHash = sts.Spec.Template.Annotations[utils.PodSpecHashAnnotation]
	}
	reason := determineRestartReason(pod, desiredImage, desiredHash, desiredPodSpecHash, isWarm)
	r.recordPodRestartStatus(ctx, cluster, pod.Name, reason)

	if !isWarm {
		log.Info("Pod config/spec hash mismatch, deleting for restart", "pod", pod.Name)
		return r.coldRestartPod(ctx, cluster, pod)
	}

	log.Info("Attempting warm restart (SIGUSR1)", "pod", pod.Name)
	if err := r.warmRestartPod(ctx, pod); err != nil {
		log.Info("Warm restart failed, falling back to cold restart", "pod", pod.Name, "error", err)
		r.recordPodRestartStatus(ctx, cluster, pod.Name, ackov1alpha1.RestartReasonConfigChanged)
		return r.coldRestartPod(ctx, cluster, pod)
	}

	// Update config hash annotation so next reconcile won't re-restart this pod.
	// If this fails, return the error so the reconciler requeues rather than
	// looping back through warm restart on every reconcile (hash mismatch).
	if err := r.updatePodConfigHash(ctx, pod, desiredHash); err != nil {
		log.Error(err, "Failed to update pod config hash after warm restart", "pod", pod.Name)
		return fmt.Errorf("warm restart succeeded but config hash update failed for pod %s: %w", pod.Name, err)
	}
	metrics.WarmRestartsTotal.WithLabelValues(cluster.Namespace, cluster.Name).Inc()
	r.Recorder.Eventf(cluster, corev1.EventTypeNormal, EventPodWarmRestarted,
		"Pod %s warm-restarted (SIGUSR1)", pod.Name)
	return nil
}

// determineRestartReason returns the reason a pod needs to be restarted.
// Priority: image change > config change > pod spec change.
func determineRestartReason(
	pod *corev1.Pod,
	desiredImage string,
	desiredConfigHash string,
	desiredPodSpecHash string,
	isWarm bool,
) ackov1alpha1.RestartReason {
	// Check image
	for _, c := range pod.Spec.Containers {
		if c.Name == podutil.AerospikeContainerName {
			if c.Image != desiredImage {
				return ackov1alpha1.RestartReasonImageChanged
			}
			break
		}
	}
	// Check config hash
	currentConfigHash := ""
	if pod.Annotations != nil {
		currentConfigHash = pod.Annotations[utils.ConfigHashAnnotation]
	}
	if currentConfigHash != desiredConfigHash {
		if isWarm {
			return ackov1alpha1.RestartReasonWarmRestart
		}
		return ackov1alpha1.RestartReasonConfigChanged
	}
	// Check pod spec hash
	currentPodSpecHash := ""
	if pod.Annotations != nil {
		currentPodSpecHash = pod.Annotations[utils.PodSpecHashAnnotation]
	}
	if desiredPodSpecHash != "" && currentPodSpecHash != desiredPodSpecHash {
		return ackov1alpha1.RestartReasonPodSpecChanged
	}
	return ackov1alpha1.RestartReasonConfigChanged
}

// recordPodRestartStatus records the restart reason/time for the pod via a status patch.
// Uses MergePatch instead of full Update to reduce conflict risk during concurrent operations.
//
// The cluster is re-fetched fresh before constructing the MergePatch base so that
// the patch ResourceVersion is current. Using the in-flight `cluster` object would
// risk a stale ResourceVersion when other reconcilers have written status during
// the in-progress reconcile (mirrors the pattern in markDirtyVolumes /
// updateDynamicConfigStatus).
func (r *AerospikeClusterReconciler) recordPodRestartStatus(
	ctx context.Context,
	cluster *ackov1alpha1.AerospikeCluster,
	podName string,
	reason ackov1alpha1.RestartReason,
) {
	log := logf.FromContext(ctx)
	now := metav1.Now()

	latest, err := r.refetchCluster(ctx, types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace})
	if err != nil {
		log.V(1).Info("Failed to refetch cluster for pod restart status (non-fatal)", "pod", podName, "err", err)
		return
	}

	patch := client.MergeFrom(latest.DeepCopy())
	if latest.Status.Pods == nil {
		latest.Status.Pods = make(map[string]ackov1alpha1.AerospikePodStatus)
	}
	podStatus := latest.Status.Pods[podName]
	podStatus.LastRestartReason = &reason
	podStatus.LastRestartTime = &now
	latest.Status.Pods[podName] = podStatus

	if err := r.Status().Patch(ctx, latest, patch); err != nil {
		log.V(1).Info("Failed to record pod restart status (non-fatal)", "pod", podName, "err", err)
	}
}

// updatePodConfigHash updates the config hash annotation on a pod after a warm restart.
func (r *AerospikeClusterReconciler) updatePodConfigHash(ctx context.Context, pod *corev1.Pod, hash string) error {
	podCopy := pod.DeepCopy()
	if podCopy.Annotations == nil {
		podCopy.Annotations = make(map[string]string)
	}
	podCopy.Annotations[utils.ConfigHashAnnotation] = hash
	return r.Patch(ctx, podCopy, client.MergeFrom(pod))
}

// coldRestartPod deletes the pod to trigger a cold restart via StatefulSet.
// It marks volumes that have a wipe method as dirty so the init container
// can wipe them when the pod is recreated.
// Before deletion, it attempts to quiesce the Aerospike node so it can
// gracefully stop accepting new transactions and complete in-flight ones.
func (r *AerospikeClusterReconciler) coldRestartPod(
	ctx context.Context,
	cluster *ackov1alpha1.AerospikeCluster,
	pod *corev1.Pod,
) error {
	log := logf.FromContext(ctx)

	// Quiesce the Aerospike node before deletion (best-effort).
	// This tells the node to stop accepting new client connections and
	// complete in-flight transactions, allowing clients to smoothly
	// failover to other nodes.
	r.quiesceNodeBeforeDeletion(ctx, cluster, pod)

	// Mark dirty volumes in pod status before deletion.
	// The init container will read these via WIPE_VOLUMES env and wipe them on restart.
	dirtyVols := getDirtyVolumes(cluster.Spec.Storage)
	if len(dirtyVols) > 0 {
		if err := r.markDirtyVolumes(ctx, cluster, pod.Name, dirtyVols); err != nil {
			log.Error(err, "Failed to mark dirty volumes", "pod", pod.Name)
			// Non-fatal: continue with pod deletion
		}
	}

	// Delete local storage PVCs before pod deletion if configured
	if cluster.Spec.Storage != nil &&
		cluster.Spec.Storage.DeleteLocalStorageOnRestart != nil &&
		*cluster.Spec.Storage.DeleteLocalStorageOnRestart {
		stsName, ordinal, ok := storage.ParsePodName(pod.Name)
		if !ok {
			log.V(1).Info("Failed to parse pod name for PVC cleanup, skipping local PVC deletion", "pod", pod.Name)
		} else {
			if err := storage.DeleteLocalPVCsForPod(ctx, r.Client, cluster.Namespace, cluster.Name, stsName, ordinal, cluster.Spec.Storage); err != nil {
				log.Error(err, "Failed to delete local PVCs before restart", "pod", pod.Name)
				r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventLocalPVCDeleteFailed,
					"Failed to delete local PVCs for pod %s before restart: %v", pod.Name, err)
				// Non-fatal: continue with pod deletion
			}
		}
	}

	if err := r.Delete(ctx, pod); err != nil && !errors.IsNotFound(err) {
		return err
	}
	metrics.ColdRestartsTotal.WithLabelValues(cluster.Namespace, cluster.Name).Inc()
	r.Recorder.Eventf(cluster, corev1.EventTypeNormal, EventPodColdRestarted,
		"Pod %s deleted for cold restart", pod.Name)
	return nil
}

// quiesceNodeBeforeDeletion attempts to quiesce an Aerospike node before
// deleting its pod. This is best-effort: if quiesce fails, pod deletion
// still proceeds. The function emits Kubernetes events to track the
// quiesce lifecycle.
func (r *AerospikeClusterReconciler) quiesceNodeBeforeDeletion(
	ctx context.Context,
	cluster *ackov1alpha1.AerospikeCluster,
	pod *corev1.Pod,
) {
	log := logf.FromContext(ctx)

	if !isPodReady(pod) {
		log.V(1).Info("Pod not ready, skipping quiesce", "pod", pod.Name)
		return
	}

	r.Recorder.Eventf(cluster, corev1.EventTypeNormal, EventNodeQuiesceStarted,
		"Quiescing Aerospike node on pod %s before deletion", pod.Name)

	if err := r.quiesceNode(ctx, pod, cluster); err != nil {
		log.Error(err, "Failed to quiesce node, proceeding with deletion", "pod", pod.Name)
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventNodeQuiesceFailed,
			"Failed to quiesce node on pod %s: %v", pod.Name, err)
		return
	}

	log.Info("Node quiesced successfully before deletion", "pod", pod.Name)
	r.Recorder.Eventf(cluster, corev1.EventTypeNormal, EventNodeQuiesced,
		"Node on pod %s quiesced successfully", pod.Name)
}

// getDirtyVolumes returns the names of volumes that have a non-"none" wipe method.
func getDirtyVolumes(storageSpec *ackov1alpha1.AerospikeStorageSpec) []string {
	if storageSpec == nil {
		return nil
	}
	var dirty []string
	for i := range storageSpec.Volumes {
		vol := &storageSpec.Volumes[i]
		wm := storage.ResolveWipeMethod(vol, storageSpec)
		if wm != "" && wm != ackov1alpha1.VolumeWipeMethodNone {
			dirty = append(dirty, vol.Name)
		}
	}
	return dirty
}

// markDirtyVolumes records dirty volumes in the cluster status for the given pod.
func (r *AerospikeClusterReconciler) markDirtyVolumes(
	ctx context.Context,
	cluster *ackov1alpha1.AerospikeCluster,
	podName string,
	dirtyVols []string,
) error {
	latest, err := r.refetchCluster(ctx, types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace})
	if err != nil {
		return err
	}

	if latest.Status.Pods == nil {
		latest.Status.Pods = make(map[string]ackov1alpha1.AerospikePodStatus)
	}

	podStatus := latest.Status.Pods[podName]
	podStatus.DirtyVolumes = dirtyVols
	latest.Status.Pods[podName] = podStatus
	return r.Status().Update(ctx, latest)
}

// getRollingUpdateBatchSize returns the effective rolling update batch size.
// RackConfig-level setting takes precedence over spec-level setting.
func (r *AerospikeClusterReconciler) getRollingUpdateBatchSize(cluster *ackov1alpha1.AerospikeCluster, totalPods int32) int32 {
	// RackConfig-level takes precedence
	if cluster.Spec.RackConfig != nil && cluster.Spec.RackConfig.RollingUpdateBatchSize != nil {
		return resolveIntOrPercent(cluster.Spec.RackConfig.RollingUpdateBatchSize, totalPods)
	}
	// Fall back to spec-level (legacy int32 field)
	if cluster.Spec.RollingUpdateBatchSize != nil && *cluster.Spec.RollingUpdateBatchSize > 0 {
		return *cluster.Spec.RollingUpdateBatchSize
	}
	return 1
}

// getMaxIgnorablePods returns the number of pods that can be ignored.
func (r *AerospikeClusterReconciler) getMaxIgnorablePods(cluster *ackov1alpha1.AerospikeCluster, totalPods int32) int32 {
	if cluster.Spec.RackConfig != nil && cluster.Spec.RackConfig.MaxIgnorablePods != nil {
		return resolveIntOrPercent(cluster.Spec.RackConfig.MaxIgnorablePods, totalPods)
	}
	return 0
}

// listRackPods fetches all pods for a specific rack in a single API call,
// sorted by ordinal descending (highest ordinal first) to preserve the
// rolling restart ordering semantics.
func (r *AerospikeClusterReconciler) listRackPods(
	ctx context.Context,
	cluster *ackov1alpha1.AerospikeCluster,
	rackID int,
) ([]corev1.Pod, error) {
	podList := &corev1.PodList{}
	if err := r.List(ctx, podList,
		client.InNamespace(cluster.Namespace),
		client.MatchingLabels(utils.LabelsForRack(cluster.Name, rackID)),
	); err != nil {
		return nil, err
	}

	// Sort by ordinal descending (highest first) for rolling restart ordering.
	sort.Slice(podList.Items, func(i, j int) bool {
		return podOrdinal(podList.Items[i].Name) > podOrdinal(podList.Items[j].Name)
	})

	return podList.Items, nil
}

// filterUnrestarted returns the pod names that were not successfully restarted.
// This includes failed pods and pods that were pending but not attempted in the
// current batch. Callers MUST pass the actual batch slice (not the full pending
// queue) as batchPods — otherwise pods beyond the batch boundary may be
// mis-classified as restarted.
func filterUnrestarted(allPending []string, failedPods []string, restarted int32, batchPods []*corev1.Pod) []string {
	// Build a set of successfully restarted pod names.
	// Successfully restarted = attempted in batch AND not in failedPods.
	failedSet := make(map[string]bool, len(failedPods))
	for _, name := range failedPods {
		failedSet[name] = true
	}

	restartedSet := make(map[string]bool)
	for _, pod := range batchPods {
		if !failedSet[pod.Name] {
			restartedSet[pod.Name] = true
		}
		// Only count up to 'restarted' successes (the rest were not attempted)
		if int32(len(restartedSet)) >= restarted {
			break
		}
	}

	var remaining []string
	for _, name := range allPending {
		if !restartedSet[name] {
			remaining = append(remaining, name)
		}
	}
	return remaining
}

// isBatchBlocked returns true when the next restart batch should wait:
//   - readiness gates are enabled and a previously restarted pod has not yet satisfied its gate, OR
//   - readiness gates are disabled and a migration check confirms migration is active.
//
// Connection failures from migration checks are treated as non-blocking (logged as warnings)
// to avoid deadlocking the restart when the cluster is temporarily unreachable.
func (r *AerospikeClusterReconciler) isBatchBlocked(
	ctx context.Context,
	cluster *ackov1alpha1.AerospikeCluster,
	rackID int,
	rackPods []corev1.Pod,
) bool {
	log := logf.FromContext(ctx)

	if isReadinessGateEnabled(cluster) {
		if blocked, blockedPod := anyPodGateUnsatisfied(cluster, rackPods); blocked {
			log.Info("Readiness gate not yet satisfied, delaying next restart", "pod", blockedPod, "rack", rackID)
			r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReadinessGateBlocking,
				"Rolling restart paused: pod %s readiness gate not yet satisfied (rack %d)", blockedPod, rackID)
			return true
		}
		return false
	}

	// Direct migration check when readiness gates are not enabled.
	migrating, err := r.isMigrationInProgress(ctx, cluster)
	if err != nil {
		// Connection failure: log a warning but proceed. The cluster may be
		// unreachable during the early phase of a rolling restart (e.g. first pod
		// is still coming up). Blocking here could deadlock the restart.
		log.V(1).Info("Migration check failed during rolling restart, proceeding with caution",
			"error", err, "rack", rackID)
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventMigrationCheckFailed,
			"Rolling restart rack %d: migration check failed (%v), proceeding", rackID, err)
		return false
	}
	if migrating {
		log.Info("Data migration in progress, delaying next restart batch", "rack", rackID)
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventRollingRestartStarted,
			"Rolling restart paused for rack %d: data migration in progress", rackID)
		return true
	}
	return false
}

// podOrdinal extracts the ordinal index from a StatefulSet pod name (e.g., "sts-0" → 0).
func podOrdinal(podName string) int {
	idx := strings.LastIndex(podName, "-")
	if idx < 0 {
		return 0
	}
	ordinal, err := strconv.Atoi(podName[idx+1:])
	if err != nil {
		return 0
	}
	return ordinal
}
