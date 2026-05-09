package controller

import (
	"context"
	"fmt"
	"maps"
	"strconv"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	ackov1alpha1 "github.com/ksr/aerospike-ce-kubernetes-operator/api/v1alpha1"
	"github.com/ksr/aerospike-ce-kubernetes-operator/internal/metrics"
	"github.com/ksr/aerospike-ce-kubernetes-operator/internal/podutil"
	"github.com/ksr/aerospike-ce-kubernetes-operator/internal/storage"
	"github.com/ksr/aerospike-ce-kubernetes-operator/internal/utils"
)

// reconcileStatefulSet creates or updates the StatefulSet for a rack.
// Returns (deferred, error). deferred is true when a scale-down was blocked
// because data migration is still in progress; the caller should requeue.
func (r *AerospikeClusterReconciler) reconcileStatefulSet(
	ctx context.Context,
	cluster *ackov1alpha1.AerospikeCluster,
	rack *ackov1alpha1.Rack,
	_ *ackov1alpha1.AerospikeConfigSpec, // effectiveConfig (pre-computed, hash passed separately)
	hash string,
	rackSize int32,
) (bool, error) {
	log := logf.FromContext(ctx)

	stsName := utils.StatefulSetName(cluster.Name, rack.ID)
	configMapName := utils.ConfigMapName(cluster.Name, rack.ID)

	// Build pod template
	podTemplate := podutil.BuildPodTemplateSpec(cluster, rack, rack.ID, configMapName, hash)

	// Compute and add PodSpec hash for change detection
	podSpecHash := computePodSpecHash(cluster, rack)
	if podTemplate.Annotations == nil {
		podTemplate.Annotations = make(map[string]string)
	}
	podTemplate.Annotations[utils.PodSpecHashAnnotation] = podSpecHash

	// Build storage
	storageSpec := cluster.Spec.Storage
	if rack.Storage != nil {
		storageSpec = rack.Storage
	}
	pvcTemplates := storage.BuildVolumeClaimTemplates(storageSpec)
	// Add cluster labels to PVC templates so PVCs can be efficiently queried by label.
	pvcLabels := utils.LabelsForCluster(cluster.Name)
	for i := range pvcTemplates {
		if pvcTemplates[i].Labels == nil {
			pvcTemplates[i].Labels = make(map[string]string)
		}
		maps.Copy(pvcTemplates[i].Labels, pvcLabels)
	}

	// Check if StatefulSet exists
	existing := &appsv1.StatefulSet{}
	err := r.Get(ctx, types.NamespacedName{Name: stsName, Namespace: cluster.Namespace}, existing)

	if errors.IsNotFound(err) {
		// Create new StatefulSet
		sts := r.buildStatefulSet(cluster, stsName, rackSize, podTemplate, pvcTemplates)
		if err := r.setOwnerRef(cluster, sts); err != nil {
			return false, err
		}
		log.Info("Creating StatefulSet", "name", stsName, "replicas", rackSize)
		if err := r.Create(ctx, sts); err != nil {
			return false, fmt.Errorf("creating StatefulSet %s: %w", stsName, err)
		}
		r.Recorder.Eventf(cluster, corev1.EventTypeNormal, EventStatefulSetCreated,
			"StatefulSet %s created: replicas=%d", stsName, rackSize)
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("getting StatefulSet %s: %w", stsName, err)
	}

	// Snapshot the StatefulSet immediately after Get so we can compute a
	// MergeFrom patch later. Using Update (full object write) on `existing`
	// after intervening network I/O (migration check, scale-down readiness)
	// can race against external StatefulSet status updates and 409 — with
	// MaxConcurrentReconciles=2 those 409s also trip the reconcile circuit
	// breaker. The MergeFrom patch sends only the fields we changed and is
	// resilient to concurrent status writes from kube-controller-manager.
	stsSnapshot := existing.DeepCopy()

	// Update only if replicas or config hash changed
	oldReplicas := int32(0)
	if existing.Spec.Replicas != nil {
		oldReplicas = *existing.Spec.Replicas
	}
	needsUpdate := oldReplicas != rackSize
	var existingHash, existingPodSpecHash string
	if existing.Spec.Template.Annotations != nil {
		existingHash = existing.Spec.Template.Annotations[utils.ConfigHashAnnotation]
		existingPodSpecHash = existing.Spec.Template.Annotations[utils.PodSpecHashAnnotation]
	}
	if existingHash != hash {
		needsUpdate = true
	}
	if existingPodSpecHash != podSpecHash {
		needsUpdate = true
	}

	if !needsUpdate {
		return false, nil
	}

	scaleDown := rackSize < oldReplicas

	// Safety check: block scale-down while data migration is in progress.
	// This prevents data loss when pods are removed before their partitions
	// have been fully migrated to remaining nodes.
	if scaleDown {
		migrating, err := r.isMigrationInProgress(ctx, cluster)
		if err != nil {
			// Connection failure: treat as migrating to avoid scale-down during
			// an unreachable cluster state (network blip, DNS delay, etc.).
			// The operator will requeue and retry.
			log.V(1).Info("Could not check migration status before scale-down, deferring scale-down",
				"error", err, "rack", rack.ID)
			r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventScaleDownDeferred,
				"Scale-down deferred for rack %d: migration check failed (%v)", rack.ID, err)
			return true, nil
		}
		if migrating {
			log.Info("Data migration in progress, deferring scale-down",
				"rack", rack.ID, "currentReplicas", oldReplicas, "desiredReplicas", rackSize)
			r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventScaleDownDeferred,
				"Scale-down deferred for rack %d: data migration in progress (current=%d, desired=%d)",
				rack.ID, oldReplicas, rackSize)
			metrics.ScaleDownDeferralsTotal.WithLabelValues(cluster.Namespace, cluster.Name).Inc()

			if phaseErr := r.setPhase(ctx, cluster, ackov1alpha1.AerospikePhaseWaitingForMigration,
				fmt.Sprintf("Scale-down deferred for rack %d: data migration in progress", rack.ID)); phaseErr != nil {
				if !errors.IsConflict(phaseErr) {
					return false, phaseErr
				}
				log.V(1).Info("Conflict setting WaitingForMigration phase, continuing reconcile")
			}

			return true, nil
		}
	}

	targetReplicas := rackSize
	if scaleDown {
		// Apply scale-down batch size: only scale down a batch at a time.
		batchSize := r.getScaleDownBatchSize(cluster, oldReplicas-rackSize)
		targetReplicas = max(oldReplicas-batchSize, rackSize)

		deferred, err := r.checkScaleDownReadiness(ctx, cluster, rack.ID, targetReplicas)
		if err != nil {
			return false, err
		}
		if deferred {
			return true, nil
		}
	}

	existing.Spec.Replicas = &targetReplicas
	existing.Spec.Template = podTemplate
	// VolumeClaimTemplates are immutable after StatefulSet creation.
	// Always preserve the existing VCTs; they can only be set during buildStatefulSet.
	log.Info("Updating StatefulSet", "name", stsName, "targetReplicas", targetReplicas)
	// Patch with MergeFrom against the snapshot taken immediately after Get,
	// so concurrent StatefulSet status updates from kube-controller-manager do
	// not cause 409 (which would otherwise trip the reconcile circuit breaker
	// under MaxConcurrentReconciles=2).
	if err := r.Patch(ctx, existing, client.MergeFrom(stsSnapshot)); err != nil {
		return false, fmt.Errorf("patching StatefulSet %s: %w", stsName, err)
	}
	r.Recorder.Eventf(cluster, corev1.EventTypeNormal, EventStatefulSetUpdated,
		"StatefulSet %s updated: replicas=%d", stsName, targetReplicas)
	if oldReplicas != targetReplicas {
		r.Recorder.Eventf(cluster, corev1.EventTypeNormal, EventRackScaled,
			"Rack %d scaled from %d to %d replicas", rack.ID, oldReplicas, targetReplicas)
	}

	// Cleanup orphaned PVCs after scale-down.
	if scaleDown {
		r.cleanupOrphanedPVCsAfterScaleDown(ctx, cluster, rack.ID, stsName, targetReplicas, oldReplicas, storageSpec)
	}

	return false, nil
}

// checkScaleDownReadiness verifies that pods which will remain after scale-down are ready.
// Returns (deferred, error). deferred=true means the scale-down should be retried later.
func (r *AerospikeClusterReconciler) checkScaleDownReadiness(
	ctx context.Context,
	cluster *ackov1alpha1.AerospikeCluster,
	rackID int,
	targetReplicas int32,
) (bool, error) {
	log := logf.FromContext(ctx)

	rackPods, err := r.listRackPods(ctx, cluster, rackID)
	if err != nil {
		return false, fmt.Errorf("listing rack pods for scale-down readiness check: %w", err)
	}
	readyCount := int32(0)
	for i := range rackPods {
		// Only count pods that will remain after scale-down (ordinal < targetReplicas).
		// Pods being removed (ordinal >= targetReplicas) should not inflate the ready count.
		if podOrdinal(rackPods[i].Name) < int(targetReplicas) && isPodReady(&rackPods[i]) {
			readyCount++
		}
	}
	if readyCount < targetReplicas {
		log.Info("Not enough ready pods for safe scale-down, deferring",
			"rack", rackID, "readyPods", readyCount, "targetReplicas", targetReplicas)
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventScaleDownDeferred,
			"Scale-down deferred for rack %d: only %d/%d target pods are ready",
			rackID, readyCount, targetReplicas)
		return true, nil
	}
	log.V(1).Info("Scale-down readiness check passed",
		"rack", rackID, "readyPods", readyCount, "targetReplicas", targetReplicas)
	return false, nil
}

// cleanupOrphanedPVCsAfterScaleDown verifies pods have terminated and then deletes orphaned PVCs.
// Pod termination is asynchronous — PVCs are only deleted once all scaled-down pods are gone.
func (r *AerospikeClusterReconciler) cleanupOrphanedPVCsAfterScaleDown(
	ctx context.Context,
	cluster *ackov1alpha1.AerospikeCluster,
	rackID int,
	stsName string,
	targetReplicas, oldReplicas int32,
	storageSpec *ackov1alpha1.AerospikeStorageSpec,
) {
	log := logf.FromContext(ctx)

	rackPods, listErr := r.listRackPods(ctx, cluster, rackID)
	if listErr != nil {
		log.Error(listErr, "Failed to list rack pods for PVC cleanup check, deferring PVC cleanup",
			"statefulset", stsName)
		return
	}

	for i := range rackPods {
		if podOrdinal(rackPods[i].Name) >= int(targetReplicas) {
			log.Info("Deferring PVC cleanup: scaled-down pods still terminating",
				"statefulset", stsName, "targetReplicas", targetReplicas)
			return
		}
	}

	log.Info("All scaled-down pods terminated, cleaning up orphaned cascade-delete PVCs",
		"name", stsName, "old", oldReplicas, "new", targetReplicas)
	deleted, err := storage.DeleteOrphanedCascadeDeletePVCs(
		ctx, r.Client, cluster.Namespace, stsName, targetReplicas, storageSpec)
	if err != nil {
		log.Error(err, "Failed to delete orphaned cascade PVCs", "statefulset", stsName)
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventPVCCleanupFailed,
			"Failed to delete orphaned cascade PVCs for %s: %v", stsName, err)
	} else if deleted > 0 {
		log.Info("Deleted orphaned cascade-delete PVCs", "statefulset", stsName, "count", deleted)
		r.Recorder.Eventf(cluster, corev1.EventTypeNormal, EventPVCCleanedUp,
			"Deleted %d orphaned PVC(s) for %s after scale-down", deleted, stsName)
	}
}

func (r *AerospikeClusterReconciler) buildStatefulSet(
	cluster *ackov1alpha1.AerospikeCluster,
	name string,
	replicas int32,
	podTemplate corev1.PodTemplateSpec,
	pvcTemplates []corev1.PersistentVolumeClaim,
) *appsv1.StatefulSet {
	labels := utils.LabelsForCluster(cluster.Name)
	selectorLabels := utils.SelectorLabelsForCluster(cluster.Name)
	serviceName := utils.HeadlessServiceName(cluster.Name)

	podManagementPolicy := appsv1.ParallelPodManagement
	if cluster.Spec.PodSpec != nil && cluster.Spec.PodSpec.PodManagementPolicy != "" {
		podManagementPolicy = cluster.Spec.PodSpec.PodManagementPolicy
	}

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: cluster.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.StatefulSetSpec{
			ServiceName:         serviceName,
			Replicas:            &replicas,
			PodManagementPolicy: podManagementPolicy,
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
				Type: appsv1.OnDeleteStatefulSetStrategyType,
			},
			Selector: &metav1.LabelSelector{
				MatchLabels: selectorLabels,
			},
			Template:             podTemplate,
			VolumeClaimTemplates: pvcTemplates,
		},
	}

	return sts
}

// cleanupRemovedRacks deletes StatefulSets for racks that no longer exist in the spec.
func (r *AerospikeClusterReconciler) cleanupRemovedRacks(
	ctx context.Context,
	cluster *ackov1alpha1.AerospikeCluster,
	currentRacks []ackov1alpha1.Rack,
) error {
	log := logf.FromContext(ctx)

	stsList, err := r.listClusterStatefulSets(ctx, cluster)
	if err != nil {
		return err
	}

	currentRackNames := make(map[string]bool)
	for _, rack := range currentRacks {
		currentRackNames[utils.StatefulSetName(cluster.Name, rack.ID)] = true
	}

	// Note: when a rack is removed, its per-rack Storage spec is no longer in the CR.
	// We fall back to the cluster-level storage spec for cascadeDelete resolution.
	//
	// Ordering is critical for safety:
	//   1. Delete the StatefulSet first so pods begin graceful termination.
	//   2. Wait for all rack pods to terminate. Deleting PVCs while pods are
	//      still running risks data loss (Aerospike may flush to a backing
	//      store that is being unmounted) and crashes pods that are still
	//      accepting transactions.
	//   3. Only then delete cascade-delete PVCs and the rack ConfigMap.
	for i := range stsList.Items {
		sts := &stsList.Items[i]
		if currentRackNames[sts.Name] {
			continue
		}

		log.Info("Deleting removed rack StatefulSet", "name", sts.Name)
		stsName := sts.Name

		// Step 1: Delete the StatefulSet first to start pod termination.
		if err := r.Delete(ctx, sts); err != nil && !errors.IsNotFound(err) {
			return err
		}

		// Step 2: Wait for pods to terminate. Bounded to 30 retries × 1s with
		// context cancellation honored. If pods are still running after the
		// timeout we fall through and let the next reconcile retry — better
		// than indefinitely blocking the reconcile loop.
		//
		// If the rackID cannot be parsed from the STS name we refuse to
		// proceed: a partial cleanup that deletes PVCs without first
		// confirming pods are gone re-introduces the "PVC deleted while pods
		// alive" hazard. Returning an error lets the reconcile loop retry on
		// the next pass; a human can also intervene via logs/events.
		rackIDStr := strings.TrimPrefix(stsName, cluster.Name+"-")
		rackID, convErr := strconv.Atoi(rackIDStr)
		if convErr != nil {
			return fmt.Errorf("cannot derive rackID from StatefulSet name %q (cluster %q): %w; "+
				"refusing rack cleanup to avoid deleting PVCs while pods may still be running",
				stsName, cluster.Name, convErr)
		}

		waitErr := r.waitForRackPodsTerminated(ctx, cluster, rackID, stsName)
		if waitErr != nil {
			log.V(1).Info("Pods for removed rack still terminating; deferring PVC and ConfigMap cleanup to next reconcile",
				"statefulset", stsName, "err", waitErr)
			continue
		}

		// Step 3a: Delete cascade-delete PVCs now that pods are gone.
		storageSpec := cluster.Spec.Storage
		if err := storage.DeleteCascadeDeletePVCs(ctx, r.Client, cluster.Namespace, stsName, storageSpec); err != nil {
			log.Error(err, "Failed to delete cascade PVCs for removed rack", "statefulset", stsName)
			r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventPVCCleanupFailed,
				"Failed to delete cascade PVCs for removed rack %s: %v", stsName, err)
		}

		// Step 3b: Delete the associated ConfigMap for the removed rack.
		// The ConfigMap name is derived from the StatefulSet name suffix (rackID).
		cmName := utils.ConfigMapName(cluster.Name, rackID)
		cm := &corev1.ConfigMap{}
		if getErr := r.Get(ctx, types.NamespacedName{Name: cmName, Namespace: cluster.Namespace}, cm); getErr == nil {
			if delErr := r.Delete(ctx, cm); delErr != nil && !errors.IsNotFound(delErr) {
				log.Error(delErr, "Failed to delete ConfigMap for removed rack", "configmap", cmName)
			} else {
				log.Info("Deleted ConfigMap for removed rack", "configmap", cmName)
			}
		}
	}

	return nil
}

// waitForRackPodsTerminated polls until all pods for the given rack are gone,
// or until the bounded timeout (30 attempts × 1s) elapses, or until the
// context is cancelled. Returns nil when all pods have terminated, or an
// error describing why the wait ended early.
func (r *AerospikeClusterReconciler) waitForRackPodsTerminated(
	ctx context.Context,
	cluster *ackov1alpha1.AerospikeCluster,
	rackID int,
	stsName string,
) error {
	log := logf.FromContext(ctx)

	const (
		maxAttempts = 30
		pollEvery   = 1 * time.Second
	)

	for attempt := 0; attempt < maxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}

		pods, err := r.listRackPods(ctx, cluster, rackID)
		if err != nil {
			// Transient list error: log and keep polling rather than giving up.
			log.V(1).Info("listRackPods failed while waiting for pod termination",
				"statefulset", stsName, "attempt", attempt, "err", err)
		} else if len(pods) == 0 {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollEvery):
		}
	}

	return fmt.Errorf("timed out waiting for pods of removed rack %s to terminate", stsName)
}

// getScaleDownBatchSize returns the effective scale-down batch size.
func (r *AerospikeClusterReconciler) getScaleDownBatchSize(cluster *ackov1alpha1.AerospikeCluster, totalToScaleDown int32) int32 {
	if cluster.Spec.RackConfig != nil && cluster.Spec.RackConfig.ScaleDownBatchSize != nil {
		return resolveIntOrPercent(cluster.Spec.RackConfig.ScaleDownBatchSize, totalToScaleDown)
	}
	return totalToScaleDown // default: scale down all at once
}

// resolveIntOrPercent resolves an IntOrString to an absolute int32 value.
func resolveIntOrPercent(val *intstr.IntOrString, total int32) int32 {
	if val == nil {
		return 1
	}
	if val.Type == intstr.Int {
		v := val.IntVal
		if v < 1 {
			return 1
		}
		return v
	}
	// Percentage
	pct, err := intstr.GetScaledValueFromIntOrPercent(val, int(total), true)
	if err != nil || pct < 1 {
		return 1
	}
	return int32(pct)
}

// detectScaling checks each rack's current StatefulSet replicas against the
// desired rack size and returns whether a scale-up or scale-down is needed.
// Returns (scalingUp, scalingDown, error). Both can be false if no scaling is needed.
// If racks are simultaneously scaling in opposite directions (one rack up, another
// down), both flags can be true. The caller uses else-if so ScalingUp takes
// precedence in the phase display when both are true.
func (r *AerospikeClusterReconciler) detectScaling(
	ctx context.Context,
	cluster *ackov1alpha1.AerospikeCluster,
	racks []ackov1alpha1.Rack,
	rackSizes []int32,
) (scalingUp bool, scalingDown bool, err error) {
	for i, rack := range racks {
		stsName := utils.StatefulSetName(cluster.Name, rack.ID)
		existing := &appsv1.StatefulSet{}
		if err := r.Get(ctx, types.NamespacedName{Name: stsName, Namespace: cluster.Namespace}, existing); err != nil {
			if errors.IsNotFound(err) {
				// New rack — treated as scale-up (StatefulSet will be created).
				scalingUp = true
				continue
			}
			return false, false, err
		}
		oldReplicas := int32(0)
		if existing.Spec.Replicas != nil {
			oldReplicas = *existing.Spec.Replicas
		}
		desired := rackSizes[i]
		if desired > oldReplicas {
			scalingUp = true
		} else if desired < oldReplicas {
			scalingDown = true
		}
	}
	return scalingUp, scalingDown, nil
}

// computePodSpecHash returns a short SHA256 hash derived from the cluster image
// and pod-level spec settings so that changes to the pod template (aside from
// config) are captured.
func computePodSpecHash(cluster *ackov1alpha1.AerospikeCluster, rack *ackov1alpha1.Rack) string {
	input := struct {
		Image           string                                `json:"image"`
		PodSpec         *ackov1alpha1.AerospikePodSpec        `json:"podSpec,omitempty"`
		Monitoring      *ackov1alpha1.AerospikeMonitoringSpec `json:"monitoring,omitempty"`
		RackID          int                                   `json:"rackID"`
		PreStopSleepSec int                                   `json:"preStopSleepSec"`
	}{
		Image:           cluster.Spec.Image,
		PodSpec:         cluster.Spec.PodSpec,
		Monitoring:      cluster.Spec.Monitoring,
		RackID:          rack.ID,
		PreStopSleepSec: podutil.PreStopSleepSeconds,
	}
	return utils.ShortSHA256(input)
}
