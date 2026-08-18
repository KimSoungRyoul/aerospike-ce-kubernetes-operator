package controller

import (
	"context"
	"fmt"
	"maps"
	"strconv"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	ackov1alpha1 "github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/api/v1alpha1"
	"github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/internal/metrics"
	"github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/internal/podutil"
	"github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/internal/storage"
	"github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/internal/utils"
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
	storageHash := computeStorageHash(cluster, rack)
	podTemplate.Annotations[utils.StorageHashAnnotation] = storageHash

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

	// Reclaim PVCs orphaned above the StatefulSet's *observed* replica count.
	// Runs on every pass, before any of the mutation logic below, so it is
	// reached whichever way this function returns — see reclaimOrphanedRackPVCs
	// for why keying it on the replica delta of a single reconcile leaked every
	// scale-down PVC permanently.
	//
	// rackSize is passed so reclamation can tell an operator-driven scale-down
	// from an external write that lowered spec.replicas behind the operator's
	// back — see reclaimOrphanedRackPVCs.
	r.reclaimOrphanedRackPVCs(ctx, cluster, rack.ID, existing, rackSize, storageSpec)

	needsUpdate := oldReplicas != rackSize
	var existingHash, existingPodSpecHash, existingStorageHash string
	if existing.Spec.Template.Annotations != nil {
		existingHash = existing.Spec.Template.Annotations[utils.ConfigHashAnnotation]
		existingPodSpecHash = existing.Spec.Template.Annotations[utils.PodSpecHashAnnotation]
		existingStorageHash = existing.Spec.Template.Annotations[utils.StorageHashAnnotation]
	}
	if existingHash != hash {
		needsUpdate = true
	}
	if existingPodSpecHash != podSpecHash {
		needsUpdate = true
	}
	// A StatefulSet templated before this annotation existed carries no storage
	// hash, so the first reconcile after an upgrade patches the template to add
	// it. That is not disruptive on its own: the StatefulSet uses OnDelete, so a
	// template change restarts nothing by itself, and selectPodsToRestart treats a
	// pod with no storage annotation as matching.
	if existingStorageHash != storageHash {
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

	// PVC reclamation for the ordinals this patch just removed happens on the
	// NEXT reconcile, from the call at the top of this function: the pods are
	// still terminating right now (deletion is asynchronous and they carry a
	// preStop sleep), so any attempt here would defer anyway. Patching
	// spec.replicas bumps the StatefulSet generation, and the ReadyReplicas
	// change as the pods go away is a second trigger, so the follow-up reconcile
	// is guaranteed rather than left to the resync period.
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

// reclaimOrphanedRackPVCs deletes cascade-delete PVCs left above the rack's
// current replica count. It is idempotent and safe to call on every reconcile.
//
// Keyed on OBSERVED state, not on the replica delta of one reconcile. The
// previous call site sat inside the `needsUpdate` branch and so ran only on the
// pass that performed the scale-down — the single pass on which it can never
// succeed, because pod deletion is asynchronous and the pods carry a preStop
// sleep, so it always hit the "still terminating" deferral. On the next
// reconcile the StatefulSet already read the desired size, both hashes matched,
// and reconcileStatefulSet returned before the cleanup was reachable. Since the
// default getScaleDownBatchSize returns the whole delta, the deferred pass was
// the ONLY pass, so every cascadeDelete PVC leaked permanently.
//
// The leak is not merely wasted storage. StatefulSet PVC names are
// ordinal-derived, so a later scale-up remounts the exact device the removed
// node wrote, and the init container only wipes volumes explicitly marked
// dirty — an Aerospike node can rejoin the cluster carrying records the
// operator was told to destroy.
//
// Safety, in the order the guards run:
//
//   - The bound is sts.Spec.Replicas — the count the StatefulSet is actually
//     running — NEVER the desired rack size. During a batched scale-down the
//     ordinals between the two still have live pods, and deleting their PVCs
//     would pull the volume out from under a running Aerospike node. A nil or
//     non-positive value means the observed replica count is unknown or the rack
//     is being torn down entirely, and reclamation is skipped rather than
//     guessed at.
//   - currentReplicas must not be below rackSize. Only an operator-driven
//     scale-down may trigger reclamation; an external write that lowered
//     spec.replicas must not.
//   - The StatefulSet must report a settled status matching its spec
//     (ObservedGeneration == Generation, Status.Replicas == Spec.Replicas).
//     This is the LOAD-BEARING pod gate: the ordinal bound alone is not
//     sufficient, because during a scale-down the replica count drops before
//     the removed pods finish terminating, so a PVC at an ordinal >=
//     spec.replicas can still be mounted. status.replicas comes from
//     kube-controller-manager using the StatefulSet's own selector, so unlike a
//     rack-label query it cannot be defeated by pod labels or a stale informer.
//   - The rack-label pod list is kept as a secondary check.
//   - Selection then requires ALL of: ordinal >= spec.replicas, the operator's
//     cluster labels (with no unfiltered namespace-wide fallback), a volume name
//     that is one of the StatefulSet's own VolumeClaimTemplates, no foreign
//     ownerReference, and cascadeDelete on that volume. See
//     storage.ownedOrphanCandidates.
//
// Cost on the steady-state path: nothing at all for clusters with no
// cascadeDelete PV volume (the common case — cascadeDelete defaults to false),
// which is checked before either List is issued.
func (r *AerospikeClusterReconciler) reclaimOrphanedRackPVCs(
	ctx context.Context,
	cluster *ackov1alpha1.AerospikeCluster,
	rackID int,
	sts *appsv1.StatefulSet,
	rackSize int32,
	storageSpec *ackov1alpha1.AerospikeStorageSpec,
) {
	log := logf.FromContext(ctx)

	// Cheapest gate first: with no cascade-delete PV volume there is nothing to
	// reclaim, so skip the pod List and the PVC List entirely.
	if !storage.HasCascadeDeletePVCs(storageSpec) {
		return
	}

	if sts == nil {
		return
	}
	stsName := sts.Name

	// Without an observed replica count there is no safe lower bound on which
	// ordinals are orphaned. Treating nil as 0 would make every PVC in the rack
	// a candidate.
	if sts.Spec.Replicas == nil {
		log.V(1).Info("Skipping PVC reclamation: StatefulSet has no observed replica count",
			"statefulset", stsName)
		return
	}
	currentReplicas := *sts.Spec.Replicas

	// A rack at zero replicas is being torn down, not scaled down. Whole-rack
	// PVC deletion belongs to the rack-removal / cluster-deletion path, which
	// deletes by ownership rather than by ordinal; reclaiming from ordinal 0
	// here would duplicate that decision without its guards.
	if currentReplicas < 1 {
		log.V(1).Info("Skipping PVC reclamation: rack is at zero replicas",
			"statefulset", stsName)
		return
	}

	// Only reclaim when the operator itself put the StatefulSet at this replica
	// count. A count BELOW the rack size the operator wants did not come from an
	// operator scale-down — `kubectl scale sts`, an HPA aimed at the StatefulSet
	// instead of the CR, GitOps drift or a backup restore can all lower it — and
	// this function sits above the scale-down branch, so isMigrationInProgress
	// and the quiesce path never run for it. Reclaiming there would destroy the
	// volumes of every ordinal the operator is about to bring back, on the same
	// pass that scales the rack up onto blank devices.
	//
	// Skipping is safe because reclamation also runs on every converged pass: a
	// stale volume left by a real scale-down is caught while the rack sits at the
	// smaller size, not only on the way back up. The residual gap is a scale-down
	// immediately followed by a scale-up with no reconcile converging in between,
	// which is documented in the PR rather than papered over.
	if currentReplicas < rackSize {
		log.V(1).Info("Skipping PVC reclamation: StatefulSet is below the desired rack size",
			"statefulset", stsName, "replicas", currentReplicas, "rackSize", rackSize)
		return
	}

	// The StatefulSet's own status is the authoritative pod count, and it is the
	// only one that cannot be defeated by labels. kube-controller-manager
	// computes status.replicas from the StatefulSet's OWN selector
	// (SelectorLabelsForCluster, which carries no rack label) and counts
	// terminating pods, and it arrives on the same object read as spec.replicas,
	// so there is no cross-informer skew.
	//
	// The label-scoped list below cannot do this job alone: the rack label is not
	// selector-enforced, and podutil.BuildPodTemplateSpec lets
	// spec.podSpec.metadata.labels overwrite acko.io/rack, so a pod can be live
	// and invisible to listRackPods. A stale pod informer has the same effect.
	if sts.Status.ObservedGeneration != sts.Generation {
		log.V(1).Info("Deferring PVC reclamation: StatefulSet status has not caught up with its spec",
			"statefulset", stsName, "generation", sts.Generation,
			"observedGeneration", sts.Status.ObservedGeneration)
		return
	}
	if sts.Status.Replicas != currentReplicas {
		log.V(1).Info("Deferring PVC reclamation: StatefulSet still reports pods it has not released",
			"statefulset", stsName, "statusReplicas", sts.Status.Replicas, "replicas", currentReplicas)
		return
	}

	rackPods, listErr := r.listRackPods(ctx, cluster, rackID)
	if listErr != nil {
		log.Error(listErr, "Failed to list rack pods for PVC reclamation, deferring",
			"statefulset", stsName)
		return
	}

	// Secondary check. Weaker than the status gate above — it can be defeated by
	// label manipulation or a stale informer — but it costs nothing and catches
	// the case where the rack label genuinely disagrees with the StatefulSet.
	if len(rackPods) != int(currentReplicas) {
		log.V(1).Info("Deferring PVC reclamation: observed pod count does not match the replica count",
			"statefulset", stsName, "pods", len(rackPods), "replicas", currentReplicas)
		return
	}

	for i := range rackPods {
		ordinal, ok := rackPodOrdinal(rackPods[i].Name)
		if !ok || ordinal >= int(currentReplicas) {
			// Fail closed on an unparseable name: a pod whose ordinal we cannot
			// read must never be assumed to be below the replica count.
			log.V(1).Info("Deferring PVC reclamation: a pod at or above the replica count is still present",
				"statefulset", stsName, "pod", rackPods[i].Name, "replicas", currentReplicas)
			return
		}
	}

	log.V(1).Info("Checking for orphaned cascade-delete PVCs",
		"statefulset", stsName, "replicas", currentReplicas)
	deleted, err := storage.DeleteOrphanedCascadeDeletePVCs(
		ctx, r.Client, sts, cluster.Name, currentReplicas, storageSpec)
	if err != nil {
		log.Error(err, "Failed to delete orphaned cascade PVCs", "statefulset", stsName)
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventPVCCleanupFailed,
			"Failed to delete orphaned cascade PVCs for %s: %v", stsName, err)
	} else if deleted > 0 {
		log.Info("Deleted orphaned cascade-delete PVCs",
			"statefulset", stsName, "count", deleted, "replicas", currentReplicas)
		r.Recorder.Eventf(cluster, corev1.EventTypeNormal, EventPVCCleanedUp,
			"Deleted %d orphaned PVC(s) for %s above replica count %d", deleted, stsName, currentReplicas)
	}
}

// rackPodOrdinal parses the StatefulSet ordinal from a pod name, reporting
// whether the parse succeeded.
//
// podOrdinal returns 0 for a name it cannot parse, which is fine for restart
// *ordering* but not for a destructive decision: "ordinal 0" would read as
// "below the replica count, therefore not blocking PVC deletion". Reclamation
// uses this instead and fails closed.
func rackPodOrdinal(podName string) (int, bool) {
	idx := strings.LastIndex(podName, "-")
	if idx < 0 {
		return 0, false
	}
	ordinal, err := strconv.Atoi(podName[idx+1:])
	if err != nil {
		return 0, false
	}
	// The ordinal is the segment after the last dash, so a leading '-' is always
	// consumed as the separator and Atoi can never return a negative value here.
	// The bound is asserted rather than assumed because the reclamation guard's
	// "ordinal >= replicas" comparison depends on it.
	if ordinal < 0 {
		return 0, false
	}
	return ordinal, true
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

		// Step 1: Delete the StatefulSet with Foreground propagation so the
		// StatefulSet object remains visible (with deletionTimestamp) until
		// all of its pods have been terminated. This guarantees that the next
		// reconcile pass still observes this sts in stsList and re-enters
		// this branch for PVC/ConfigMap cleanup. With the default Background
		// propagation, the sts disappears immediately and orphan PVCs would
		// never be revisited, leaking storage.
		if sts.DeletionTimestamp.IsZero() {
			fg := metav1.DeletePropagationForeground
			if err := r.Delete(ctx, sts, &client.DeleteOptions{PropagationPolicy: &fg}); err != nil && !errors.IsNotFound(err) {
				return err
			}
		}

		// Step 2: Defer PVC/ConfigMap cleanup until pods are confirmed gone.
		// We deliberately do NOT block the reconcile loop polling for pod
		// termination — a long blocking poll holds the controller worker
		// hostage and, if it times out, the StatefulSet is already gone so
		// the next reconcile will not see this stsName again, leaking PVCs.
		//
		// Instead, we check once: if pods are still terminating, return nil
		// without deleting PVCs/ConfigMap. The list-Pods on the next
		// reconcile will re-enter this loop (StatefulSet may already be
		// fully gone but the orphan PVCs/ConfigMap survive and are handled
		// by the cleanup path keyed off rackID below).
		//
		// If the rackID cannot be parsed from the STS name we skip this
		// StatefulSet rather than failing: a non-numeric suffix means this is
		// not an operator-managed rack StatefulSet, so it is not ours to clean
		// up. Returning a hard error here would abort the entire reconcile for
		// every other rack over a single unrecognized name.
		rackIDStr := strings.TrimPrefix(stsName, cluster.Name+"-")
		rackID, convErr := strconv.Atoi(rackIDStr)
		if convErr != nil {
			log.V(1).Info("Skipping StatefulSet with unparseable rackID suffix; not an operator-managed rack",
				"statefulset", stsName, "err", convErr)
			continue
		}

		pods, listErr := r.listRackPods(ctx, cluster, rackID)
		if listErr != nil {
			log.V(1).Info("listRackPods failed for removed rack; deferring PVC/ConfigMap cleanup to next reconcile",
				"statefulset", stsName, "err", listErr)
			continue
		}
		if len(pods) > 0 {
			log.V(1).Info("Pods for removed rack still terminating; deferring PVC and ConfigMap cleanup to next reconcile",
				"statefulset", stsName, "remainingPods", len(pods))
			continue
		}

		// Step 3a: Delete cascade-delete PVCs now that pods are gone.
		storageSpec := cluster.Spec.Storage
		if err := storage.DeleteCascadeDeletePVCs(ctx, r.Client, cluster.Namespace, cluster.Name, stsName, storageSpec); err != nil {
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

// computeStorageHash returns a short SHA256 hash of the rack's EFFECTIVE storage
// spec — the rack override when present, else the cluster-level spec, resolved
// the same way reconcileStatefulSet resolves it, so the hash describes the
// storage this rack's pod template was actually built from.
//
// Storage renders into the pod template: BuildVolumes produces the inline
// volumes and every aerospike volumeMount from it. Without hashing it, a
// storage-only edit left needsUpdate false and was discarded silently while the
// cluster reported phase=Completed with storage that did not match the spec
// (#340). The VolumeClaimTemplate half of a storage edit cannot be applied at
// all — VCTs are immutable on a live StatefulSet — so ValidateUpdate rejects
// VCT-affecting changes outright, and what remains here is the mount-only half,
// which hashing makes take effect.
//
// It is a SEPARATE annotation from the pod-spec hash so that introducing it did
// not change any existing pod's pod-spec hash. See utils.StorageHashAnnotation
// for why that mattered.
func computeStorageHash(cluster *ackov1alpha1.AerospikeCluster, rack *ackov1alpha1.Rack) string {
	storageSpec := cluster.Spec.Storage
	if rack.Storage != nil {
		storageSpec = rack.Storage
	}
	return utils.ShortSHA256(struct {
		Storage *ackov1alpha1.AerospikeStorageSpec `json:"storage,omitempty"`
	}{Storage: storageSpec})
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
//
// PodService and AerospikeNetworkPolicy are included because they change what
// the operator renders into the pod template: AerospikeNetworkPolicy drives the
// ConfigMap's access-address placeholders and PodService injects a per-pod
// service env var into the container. Without hashing them, editing
// spec.podService or spec.aerospikeNetworkPolicy would update the ConfigMap but
// leave needsUpdate false, so the StatefulSet template would never be patched
// and pods would keep stale config. Both are JSON-serializable pointers; a nil
// value is omitted and hashes stably.
//
// Storage is deliberately NOT hashed here — it has its own annotation, see
// computeStorageHash. Folding it in changed the pod-spec hash of every cluster
// carrying any spec.storage on operator upgrade, queueing entire fleets for a
// cold restart with no user edit at all.
//
// The hash is written to the pod template as utils.PodSpecHashAnnotation.
// reconcileRollingRestart compares it against each pod's annotation and rolls
// (cold-restarts) any pod whose pod-spec hash is stale, so a change to the
// hashed fields actually propagates to running pods rather than only updating
// the StatefulSet template.
func computePodSpecHash(cluster *ackov1alpha1.AerospikeCluster, rack *ackov1alpha1.Rack) string {
	input := struct {
		Image                  string                                `json:"image"`
		PodSpec                *ackov1alpha1.AerospikePodSpec        `json:"podSpec,omitempty"`
		Monitoring             *ackov1alpha1.AerospikeMonitoringSpec `json:"monitoring,omitempty"`
		PodService             *ackov1alpha1.AerospikeServiceSpec    `json:"podService,omitempty"`
		AerospikeNetworkPolicy *ackov1alpha1.AerospikeNetworkPolicy  `json:"aerospikeNetworkPolicy,omitempty"`
		RackID                 int                                   `json:"rackID"`
		PreStopSleepSec        int                                   `json:"preStopSleepSec"`
	}{
		Image:                  cluster.Spec.Image,
		PodSpec:                cluster.Spec.PodSpec,
		Monitoring:             cluster.Spec.Monitoring,
		PodService:             cluster.Spec.PodService,
		AerospikeNetworkPolicy: cluster.Spec.AerospikeNetworkPolicy,
		RackID:                 rack.ID,
		PreStopSleepSec:        podutil.PreStopSleepSeconds,
	}
	return utils.ShortSHA256(input)
}
