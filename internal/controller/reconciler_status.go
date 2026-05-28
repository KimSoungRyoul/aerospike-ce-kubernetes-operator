package controller

import (
	"context"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"

	aero "github.com/aerospike/aerospike-client-go/v8"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	ackov1alpha1 "github.com/ksr/aerospike-ce-kubernetes-operator/api/v1alpha1"
	"github.com/ksr/aerospike-ce-kubernetes-operator/internal/metrics"
	"github.com/ksr/aerospike-ce-kubernetes-operator/internal/podutil"
	"github.com/ksr/aerospike-ce-kubernetes-operator/internal/utils"
	"github.com/ksr/aerospike-ce-kubernetes-operator/internal/version"
)

// StatusUpdateOpts carries optional annotations for updateStatusAndPhase.
type StatusUpdateOpts struct {
	// ACLErr, if non-nil, sets the ACLSynced condition to False.
	// If nil and ACL spec is present, ACLSynced is set to True.
	ACLErr error
	// ACLSynced indicates whether ACL was actually applied (not skipped).
	// When false and ACLErr is nil, ACL sync was skipped (e.g., no ready pods).
	ACLSynced bool
	// RestartInProgress indicates a rolling restart is active (MigrationComplete=False).
	RestartInProgress bool
	// Paused indicates reconciliation is paused (ReconciliationPaused=True).
	Paused bool
}

// updateStatusAndPhase re-fetches the latest cluster object from the API server,
// populates status fields, sets the desired phase and reason, and performs a status update.
// This pattern avoids "object has been modified" conflict errors that occur when
// updating status on a stale object.
// If the status already matches the desired state, the update is skipped to avoid
// triggering unnecessary reconciliation loops.
func (r *AerospikeClusterReconciler) updateStatusAndPhase(
	ctx context.Context,
	namespacedName types.NamespacedName,
	phase ackov1alpha1.AerospikePhase,
	phaseReason string,
	opts StatusUpdateOpts,
) error {
	log := logf.FromContext(ctx)

	// Re-fetch the latest version from the API server.
	latest, err := r.refetchCluster(ctx, namespacedName)
	if err != nil {
		return err
	}

	// Capture the previous state for comparison (before populateStatus modifies it).
	prev := statusSnapshot{
		Phase:       latest.Status.Phase,
		PhaseReason: latest.Status.PhaseReason,
		Size:        latest.Status.Size,
		Health:      latest.Status.Health,
		Generation:  latest.Status.ObservedGeneration,
		Selector:    latest.Status.Selector,
		Pods:        maps.Clone(latest.Status.Pods),
		Conditions:  conditionsSnapshot(latest.Status.Conditions),
	}

	readyCount, err := r.populateStatus(ctx, latest)
	if err != nil {
		return err
	}
	latest.Status.Phase = phase
	latest.Status.PhaseReason = phaseReason

	// Apply fine-grained conditions.
	setFineGrainedConditions(latest, opts)

	// Skip the update if nothing meaningful changed to avoid
	// triggering a reconciliation feedback loop via the watch.
	if statusUnchanged(prev, latest, readyCount, phase, phaseReason) {
		log.V(1).Info("Status unchanged, skipping update",
			"readyPods", readyCount, "desiredSize", latest.Spec.Size, "phase", phase)
		return nil
	}

	log.Info("Updating status", "readyPods", readyCount, "desiredSize", latest.Spec.Size, "phase", phase, "phaseReason", phaseReason)

	// On successful completion: record the full applied spec and refresh per-node info.
	if phase == ackov1alpha1.AerospikePhaseCompleted {
		// AppliedSpec records the last successfully reconciled spec for drift detection.
		latest.Status.AppliedSpec = latest.Spec.DeepCopy()

		// Record operator version and reconcile timestamp.
		latest.Status.OperatorVersion = version.Version
		now := metav1.Now()
		latest.Status.LastReconcileTime = &now

		// Clear pending restart pods on successful completion.
		latest.Status.PendingRestartPods = nil

		// Enrich status with Aerospike cluster info using a single client connection.
		r.enrichStatusWithAerospikeInfo(ctx, latest)

		// Populate external endpoints from per-pod and seeds finder LB services.
		r.populateExternalEndpoints(ctx, latest)
	}

	// Update Prometheus metrics
	metrics.ClusterPhase.WithLabelValues(latest.Namespace, latest.Name).Set(metrics.PhaseToFloat(string(phase)))
	metrics.ClusterReadyPods.WithLabelValues(latest.Namespace, latest.Name).Set(float64(readyCount))
	if latest.Status.LastReconcileTime != nil {
		metrics.LastReconcileTimestamp.WithLabelValues(latest.Namespace, latest.Name).Set(float64(latest.Status.LastReconcileTime.Unix()))
	}
	metrics.ClusterASSize.WithLabelValues(latest.Namespace, latest.Name).Set(float64(latest.Status.AerospikeClusterSize))
	if latest.Status.MigrationStatus != nil {
		metrics.ClusterMigratingPartitions.WithLabelValues(latest.Namespace, latest.Name).Set(float64(latest.Status.MigrationStatus.RemainingPartitions))
	}

	// Use RetryOnConflict for the final status write. On conflict, re-fetch the
	// object and recompute the status to avoid a full requeue cycle.
	// Non-conflict errors are returned immediately without refetching, since
	// RetryOnConflict won't retry them anyway and refetching would clobber
	// concurrent writes by other reconcilers.
	//
	// computedObservedGeneration is the ObservedGeneration this reconcile
	// computed (against the spec it actually processed). It is preserved across
	// the conflict-retry path so a concurrent spec bump does not get falsely
	// marked as observed (see the IMPORTANT note below).
	computedObservedGeneration := latest.Status.ObservedGeneration
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if err := r.Status().Update(ctx, latest); err != nil {
			// Only refetch on conflict — RetryOnConflict will not retry on
			// other error types, so a refetch in those paths would be wasted
			// work and could overwrite legitimate concurrent state changes.
			if !apierrors.IsConflict(err) {
				return err
			}
			refetched, fetchErr := r.refetchCluster(ctx, namespacedName)
			if fetchErr != nil {
				return fetchErr
			}

			// Recompute status against the refetched object instead of
			// stamping a pre-conflict snapshot onto it. The snapshot's
			// Status.Pods could be stale (a concurrent reconcile may have
			// just published newer pod-readiness), and blindly re-applying it
			// would regress that state. populateStatus re-lists pods, so the
			// retry writes fresh pod-readiness.
			if _, fetchErr := r.populateStatus(ctx, refetched); fetchErr != nil {
				return fetchErr
			}
			refetched.Status.Phase = phase
			refetched.Status.PhaseReason = phaseReason
			setFineGrainedConditions(refetched, opts)
			if phase == ackov1alpha1.AerospikePhaseCompleted {
				refetched.Status.AppliedSpec = refetched.Spec.DeepCopy()
				refetched.Status.OperatorVersion = version.Version
				now := metav1.Now()
				refetched.Status.LastReconcileTime = &now
				refetched.Status.PendingRestartPods = nil
				r.enrichStatusWithAerospikeInfo(ctx, refetched)
				r.populateExternalEndpoints(ctx, refetched)
			}

			// IMPORTANT: do NOT advance ObservedGeneration to refetched.Generation.
			// The conflict often comes from a concurrent spec update (Generation bump).
			// populateStatus above set ObservedGeneration to the refetched (new)
			// Generation; restore the value this reconcile actually computed so we
			// do not falsely advertise that the new spec has been observed. Keeping
			// the computed value lets the controller-runtime watch detect the gap
			// (Generation > ObservedGeneration) and trigger a fresh reconcile that
			// actually processes the new spec.
			refetched.Status.ObservedGeneration = computedObservedGeneration

			latest = refetched
			return err
		}
		return nil
	})
}

// enrichStatusWithAerospikeInfo creates a single Aerospike client connection and uses it
// to collect per-node info, cluster size, and migration stats. All queries are best-effort:
// failures are logged at V(1) and never block the status update.
func (r *AerospikeClusterReconciler) enrichStatusWithAerospikeInfo(
	ctx context.Context,
	cluster *ackov1alpha1.AerospikeCluster,
) {
	log := logf.FromContext(ctx)

	aeroClient, err := r.getAerospikeClient(ctx, cluster)
	if err != nil {
		log.V(1).Info("Could not connect to Aerospike for status enrichment (non-fatal)", "err", err)
		return
	}
	defer closeAerospikeClient(aeroClient)

	// 1. Enrich pod status with per-node Aerospike info (NodeID, ClusterName, endpoints).
	if aeroInfoMap := collectAerospikeInfo(ctx, aeroClient, cluster); aeroInfoMap != nil {
		for podName, info := range aeroInfoMap {
			if ps, ok := cluster.Status.Pods[podName]; ok {
				ps.NodeID = info.NodeID
				ps.ClusterName = info.ClusterName
				ps.AccessEndpoints = info.AccessEndpoints
				cluster.Status.Pods[podName] = ps
			}
		}
	}

	// 2. Update AerospikeClusterSize (best-effort).
	if size, err := clusterSize(aeroClient); err != nil {
		log.V(1).Info("cluster-size query failed (non-fatal)", "err", err)
	} else {
		cluster.Status.AerospikeClusterSize = int32(size)
	}

	// 3. Update migration status (best-effort).
	if perNode, err := migrateStatsPerNode(log, aeroClient); err != nil {
		log.V(1).Info("Migration stats query failed (non-fatal)", "err", err)
	} else {
		applyMigrationStats(cluster, perNode)
	}
}

// applyMigrationStats applies per-node migration statistics to the cluster status.
// It sets MigratingPartitions on pods whose IP matches a key in perNode, clears
// MigratingPartitions on pods not present in perNode, and updates the cluster-level
// MigrationStatus and MigrationComplete condition.
//
// An empty perNode map means no node reported a usable migration stat (every
// node had a nil host, or the stat was absent everywhere). Treating that as
// "0 partitions remaining" would falsely flip MigrationComplete to True, which
// could let a deferred scale-down or rolling restart proceed before migration
// actually finished. In that case the previous MigrationStatus/condition is
// left untouched so a stale-but-safe value is kept instead of a false positive.
func applyMigrationStats(cluster *ackov1alpha1.AerospikeCluster, perNode map[string]int64) {
	if len(perNode) == 0 {
		return
	}

	// Build pod-IP → pod-name lookup.
	podIPToPodName := make(map[string]string, len(cluster.Status.Pods))
	for podName, ps := range cluster.Status.Pods {
		if ps.PodIP != "" {
			podIPToPodName[ps.PodIP] = podName
		}
	}

	var totalRemaining int64
	anyMigrating := false

	for nodeIP, remaining := range perNode {
		totalRemaining += remaining
		if remaining > 0 {
			anyMigrating = true
		}

		// Update per-pod migration info.
		if podName, ok := podIPToPodName[nodeIP]; ok {
			if ps, exists := cluster.Status.Pods[podName]; exists {
				remainingVal := remaining // copy: &remaining would alias the loop variable
				ps.MigratingPartitions = &remainingVal
				cluster.Status.Pods[podName] = ps
			}
		}
	}

	// Clear MigratingPartitions for pods whose IP was not found in the migration stats
	// (e.g., node was unreachable or not yet part of the cluster).
	for podName, ps := range cluster.Status.Pods {
		if _, found := perNode[ps.PodIP]; !found {
			if ps.MigratingPartitions != nil {
				ps.MigratingPartitions = nil
				cluster.Status.Pods[podName] = ps
			}
		}
	}

	now := metav1.Now()
	cluster.Status.MigrationStatus = &ackov1alpha1.MigrationStatus{
		InProgress:          anyMigrating,
		RemainingPartitions: totalRemaining,
		LastChecked:         now,
	}

	// Update the MigrationComplete condition based on actual migration state.
	setCondition(cluster, ackov1alpha1.ConditionMigrationComplete, !anyMigrating,
		"MigrationComplete", fmt.Sprintf("Remaining partitions: %d", totalRemaining))
}

// populateStatus fills in the cluster's status fields and returns the ready pod count.
func (r *AerospikeClusterReconciler) populateStatus(
	ctx context.Context,
	cluster *ackov1alpha1.AerospikeCluster,
) (int32, error) {
	log := logf.FromContext(ctx)

	// List all pods for this cluster
	podList, err := r.listClusterPods(ctx, cluster)
	if err != nil {
		return 0, err
	}

	servicePort := int32(getServicePort(cluster))
	podStatuses := make(map[string]ackov1alpha1.AerospikePodStatus, len(podList.Items))
	readyCount := int32(0)

	for i := range podList.Items {
		pod := &podList.Items[i]

		rackID := 0
		if rackStr, ok := pod.Labels[utils.RackLabel]; ok {
			if id, err := strconv.Atoi(rackStr); err != nil {
				log.V(1).Info("Failed to parse rack ID label", "pod", pod.Name, "label", rackStr, "error", err)
			} else {
				rackID = id
			}
		}

		prev := cluster.Status.Pods[pod.Name]
		ps := buildPodStatus(pod, prev, cluster.Spec.Image, servicePort, rackID)
		podStatuses[pod.Name] = ps

		if ps.IsRunningAndReady {
			readyCount++
		}
	}

	cluster.Status.Pods = podStatuses
	cluster.Status.Size = readyCount
	cluster.Status.Health = fmt.Sprintf("%d/%d", readyCount, cluster.Spec.Size)
	cluster.Status.ObservedGeneration = cluster.Generation
	// DeepCopy to give Status an independent snapshot. A shallow alias would
	// allow later mutations of cluster.Spec.AerospikeConfig (e.g.
	// InjectAccessAddressPlaceholders) to leak into Status.
	cluster.Status.AerospikeConfig = cluster.Spec.AerospikeConfig.DeepCopy()

	// Build a deterministic selector string for HPA.
	cluster.Status.Selector = buildSelectorString(utils.SelectorLabelsForCluster(cluster.Name))

	// Update base conditions (Available, Ready).
	setCondition(cluster, ackov1alpha1.ConditionAvailable, readyCount > 0, "ClusterAvailable", "At least one pod is ready")
	setCondition(cluster, ackov1alpha1.ConditionReady, readyCount == cluster.Spec.Size, "AllPodsReady", fmt.Sprintf("%d/%d pods ready", readyCount, cluster.Spec.Size))

	return readyCount, nil
}

// buildPodStatus constructs an AerospikePodStatus for a single pod.
// It merges live pod state with preserved fields from the previous status
// (Aerospike node info, dirty volumes, unstable timestamps, restart history).
func buildPodStatus(
	pod *corev1.Pod,
	prev ackov1alpha1.AerospikePodStatus,
	specImage string,
	servicePort int32,
	rackID int,
) ackov1alpha1.AerospikePodStatus {
	isReady := isPodReady(pod)

	// Read hashes from pod annotations
	var configHash, podSpecHash string
	if pod.Annotations != nil {
		configHash = pod.Annotations[utils.ConfigHashAnnotation]
		podSpecHash = pod.Annotations[utils.PodSpecHashAnnotation]
	}

	// Use the actual running image from the pod, not the desired spec image.
	// During rolling updates the pod may still run the old image.
	podImage := specImage
	for _, c := range pod.Spec.Containers {
		if c.Name == podutil.AerospikeContainerName {
			podImage = c.Image
			break
		}
	}

	// Preserve dirty volumes from previous status; clear them once the pod is ready
	// (meaning the init container has already wiped them during restart).
	var dirtyVolumes []string
	if len(prev.DirtyVolumes) > 0 && !isReady {
		dirtyVolumes = prev.DirtyVolumes
	}

	// Preserve Aerospike node info from previous status.
	// These fields are refreshed via collectAerospikeInfo only when phase == Completed.
	nodeID := prev.NodeID
	clusterName := prev.ClusterName
	accessEndpoints := prev.AccessEndpoints
	lastRestartReason := prev.LastRestartReason
	lastRestartTime := prev.LastRestartTime
	migratingPartitions := prev.MigratingPartitions
	dynamicConfigStatus := prev.DynamicConfigStatus
	dynamicConfigChanges := prev.DynamicConfigChanges
	initializedVolumes := prev.InitializedVolumes

	// Track pod instability: set UnstableSince on first NotReady, preserve it
	// while still NotReady, clear it when the pod becomes Ready.
	var unstableSince *metav1.Time
	if !isReady {
		if prev.UnstableSince != nil {
			unstableSince = prev.UnstableSince // preserve original timestamp
		} else {
			now := metav1.Now()
			unstableSince = &now
		}
	}

	gateSatisfied, _ := findPodReadinessCondition(pod)

	return ackov1alpha1.AerospikePodStatus{
		PodIP:                  pod.Status.PodIP,
		HostIP:                 pod.Status.HostIP,
		Image:                  podImage,
		PodPort:                servicePort,
		ServicePort:            servicePort,
		Rack:                   rackID,
		IsRunningAndReady:      isReady,
		ConfigHash:             configHash,
		PodSpecHash:            podSpecHash,
		DirtyVolumes:           dirtyVolumes,
		NodeID:                 nodeID,
		ClusterName:            clusterName,
		AccessEndpoints:        accessEndpoints,
		ReadinessGateSatisfied: gateSatisfied,
		LastRestartReason:      lastRestartReason,
		LastRestartTime:        lastRestartTime,
		UnstableSince:          unstableSince,
		MigratingPartitions:    migratingPartitions,
		DynamicConfigStatus:    dynamicConfigStatus,
		DynamicConfigChanges:   dynamicConfigChanges,
		InitializedVolumes:     initializedVolumes,
	}
}

func isPodReady(pod *corev1.Pod) bool {
	if pod.Status.Phase != corev1.PodRunning {
		return false
	}
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func buildSelectorString(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}

	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	slices.Sort(keys)

	selectorParts := make([]string, 0, len(keys))
	for _, k := range keys {
		selectorParts = append(selectorParts, fmt.Sprintf("%s=%s", k, labels[k]))
	}

	return strings.Join(selectorParts, ",")
}

func setCondition(cluster *ackov1alpha1.AerospikeCluster, condType string, status bool, reason, message string) {
	condStatus := metav1.ConditionFalse
	if status {
		condStatus = metav1.ConditionTrue
	}

	newCond := metav1.Condition{
		Type:               condType,
		Status:             condStatus,
		ObservedGeneration: cluster.Generation,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
	}

	for i, existing := range cluster.Status.Conditions {
		if existing.Type == condType {
			// Preserve transition time when status itself has not changed.
			if existing.Status == condStatus {
				newCond.LastTransitionTime = existing.LastTransitionTime
			}
			if existing.Status != condStatus ||
				existing.ObservedGeneration != cluster.Generation ||
				existing.Reason != reason ||
				existing.Message != message {
				cluster.Status.Conditions[i] = newCond
			}
			return
		}
	}

	cluster.Status.Conditions = append(cluster.Status.Conditions, newCond)
}

// removeCondition removes a condition from the cluster's status conditions slice.
// No-op if the condition is not present.
func removeCondition(cluster *ackov1alpha1.AerospikeCluster, condType string) {
	conditions := cluster.Status.Conditions[:0]
	for _, c := range cluster.Status.Conditions {
		if c.Type != condType {
			conditions = append(conditions, c)
		}
	}
	cluster.Status.Conditions = conditions
}

// setFineGrainedConditions sets all fine-grained status conditions:
// ConfigApplied, ReconciliationPaused, ACLSynced, MigrationComplete.
// Called from updateStatusAndPhase after populateStatus.
func setFineGrainedConditions(cluster *ackov1alpha1.AerospikeCluster, o StatusUpdateOpts) {
	// ConfigApplied: true when all pods carry the same config hash as the desired config.
	desiredHash := configHash(cluster.Spec.AerospikeConfig)
	allConfigApplied := len(cluster.Status.Pods) > 0
	for _, ps := range cluster.Status.Pods {
		if ps.ConfigHash != desiredHash {
			allConfigApplied = false
			break
		}
	}
	if allConfigApplied {
		setCondition(cluster, ackov1alpha1.ConditionConfigApplied, true,
			"ConfigApplied", "All pods have the desired Aerospike configuration")
	} else {
		setCondition(cluster, ackov1alpha1.ConditionConfigApplied, false,
			"ConfigPending", "One or more pods do not yet have the desired configuration")
	}

	// ReconciliationPaused
	reason := "ReconciliationActive"
	message := "Reconciliation is active"
	if o.Paused {
		reason = "ReconciliationPaused"
		message = "Reconciliation is paused by user (spec.paused=true)"
	}
	setCondition(cluster, ackov1alpha1.ConditionReconciliationPaused, o.Paused, reason, message)

	// ACLSynced — only set if ACL is configured; cleared when ACL is removed.
	if cluster.Spec.AerospikeAccessControl != nil {
		if o.ACLErr != nil {
			setCondition(cluster, ackov1alpha1.ConditionACLSynced, false,
				"ACLSyncFailed", o.ACLErr.Error())
		} else if o.ACLSynced {
			setCondition(cluster, ackov1alpha1.ConditionACLSynced, true,
				"ACLSyncSucceeded", "ACL roles and users are synchronized")
		} else {
			setCondition(cluster, ackov1alpha1.ConditionACLSynced, false,
				"ACLSyncPending", "ACL sync skipped: no ready pods available")
		}
	} else {
		// ACL was removed: clear any stale ACLSynced condition to avoid confusion.
		removeCondition(cluster, ackov1alpha1.ConditionACLSynced)
	}

	// DynamicConfigDegraded — clear when the cluster reaches a healthy phase.
	// The condition is set by setConfigDegraded() when rollback fails; once the
	// operator recovers (e.g., via cold restart), it is removed.
	if cluster.Status.Phase != ackov1alpha1.AerospikePhaseConfigDegraded {
		removeCondition(cluster, ackov1alpha1.ConditionDynamicConfigDegraded)
	}

	// MigrationComplete — set to False while rolling restart is in progress,
	// True otherwise as a default. When phase == Completed, enrichStatusWithAerospikeInfo
	// → applyMigrationStats will overwrite this with the actual cluster migration state.
	if o.RestartInProgress {
		setCondition(cluster, ackov1alpha1.ConditionMigrationComplete, false,
			"MigrationComplete", "Rolling restart in progress")
	} else {
		setCondition(cluster, ackov1alpha1.ConditionMigrationComplete, true,
			"MigrationComplete", "No rolling restart in progress")
	}
}

type conditionSnapshot struct {
	Status             metav1.ConditionStatus
	ObservedGeneration int64
	Reason             string
	Message            string
}

// conditionsSnapshot returns a map of condition Type → stable fields for skip-check comparison.
func conditionsSnapshot(conds []metav1.Condition) map[string]conditionSnapshot {
	m := make(map[string]conditionSnapshot, len(conds))
	for _, c := range conds {
		m[c.Type] = conditionSnapshot{
			Status:             c.Status,
			ObservedGeneration: c.ObservedGeneration,
			Reason:             c.Reason,
			Message:            c.Message,
		}
	}
	return m
}

// conditionsChanged returns true if any condition type or status differs between
// the snapshot taken before populateStatus and the current slice after all updates.
func conditionsChanged(prev map[string]conditionSnapshot, cur []metav1.Condition) bool {
	if len(prev) != len(cur) {
		return true
	}
	for _, c := range cur {
		s, ok := prev[c.Type]
		if !ok {
			return true
		}
		if s.Status != c.Status ||
			s.ObservedGeneration != c.ObservedGeneration ||
			s.Reason != c.Reason ||
			s.Message != c.Message {
			return true
		}
	}
	return false
}

// statusSnapshot captures the relevant status fields before populateStatus
// modifies them, so we can compare and skip no-op status updates.
type statusSnapshot struct {
	Phase       ackov1alpha1.AerospikePhase
	PhaseReason string
	Size        int32
	Health      string
	Generation  int64
	Selector    string
	Pods        map[string]ackov1alpha1.AerospikePodStatus
	Conditions  map[string]conditionSnapshot
}

func statusUnchanged(prev statusSnapshot, latest *ackov1alpha1.AerospikeCluster, readyCount int32, phase ackov1alpha1.AerospikePhase, phaseReason string) bool {
	return prev.Phase == phase &&
		prev.PhaseReason == phaseReason &&
		prev.Size == readyCount &&
		prev.Health == latest.Status.Health &&
		prev.Generation == latest.Generation &&
		prev.Selector == latest.Status.Selector &&
		!conditionsChanged(prev.Conditions, latest.Status.Conditions) &&
		reflect.DeepEqual(prev.Pods, latest.Status.Pods)
}

// aeroPodInfo holds per-node Aerospike information collected via asinfo commands.
type aeroPodInfo struct {
	NodeID          string
	ClusterName     string
	AccessEndpoints []string
}

// maxParallelInfoQueries limits the number of concurrent per-node info queries
// to prevent connection storms on large clusters.
const maxParallelInfoQueries = 8

// collectAerospikeInfo collects per-node information (NodeID, ClusterName,
// AccessEndpoints) keyed by pod name using the provided Aerospike client.
// Queries are executed concurrently (bounded by maxParallelInfoQueries).
// Errors are logged at V(1) and the function returns nil rather than failing
// so that status updates are never blocked by an unreachable cluster.
func collectAerospikeInfo(
	ctx context.Context,
	aeroClient *aero.Client,
	cluster *ackov1alpha1.AerospikeCluster,
) map[string]aeroPodInfo {
	log := logf.FromContext(ctx)
	// Build a pod-IP → pod-name lookup from the current status pods.
	podIPToPodName := make(map[string]string, len(cluster.Status.Pods))
	for podName, ps := range cluster.Status.Pods {
		if ps.PodIP != "" {
			podIPToPodName[ps.PodIP] = podName
		}
	}

	nodes := aeroClient.GetNodes()
	if len(nodes) == 0 {
		return nil
	}

	var (
		mu     sync.Mutex
		wg     sync.WaitGroup
		sem    = make(chan struct{}, maxParallelInfoQueries)
		result = make(map[string]aeroPodInfo, len(nodes))
	)

	for _, node := range nodes {
		nodeHost := node.GetHost()
		if nodeHost == nil {
			log.V(1).Info("Skipping Aerospike node with nil host info")
			continue
		}
		podName, ok := podIPToPodName[nodeHost.Name]
		if !ok {
			log.V(1).Info("Aerospike node IP not matched to any pod", "nodeIP", nodeHost.Name)
			continue
		}

		wg.Add(1)
		go func(node *aero.Node, podName string) {
			defer wg.Done()
			// Acquire a semaphore slot or bail out on cancellation.
			// The release defer MUST be co-located with the acquire branch
			// so that:
			//   1. The cancellation branch never tries to release a slot
			//      it did not take (a stray `<-sem` would steal a slot
			//      from a peer worker).
			//   2. A panic between acquire and release cannot leak the
			//      slot — the defer is registered the instant we own it.
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }() // release semaphore slot
			case <-ctx.Done():
				return
			}

			info := aeroPodInfo{}

			if nodeID, err := asinfoCommandOnNode(node, "node"); err == nil {
				info.NodeID = strings.TrimSpace(nodeID)
			} else {
				log.V(1).Info("Failed to get nodeID", "pod", podName, "error", err)
			}

			if clusterName, err := asinfoCommandOnNode(node, "cluster-name"); err == nil {
				info.ClusterName = strings.TrimSpace(clusterName)
			} else {
				log.V(1).Info("Failed to get cluster-name", "pod", podName, "error", err)
			}

			if serviceStr, err := asinfoCommandOnNode(node, "service"); err == nil {
				info.AccessEndpoints = parseServiceEndpoints(serviceStr)
			} else {
				log.V(1).Info("Failed to get service endpoints", "pod", podName, "error", err)
			}

			mu.Lock()
			result[podName] = info
			mu.Unlock()
		}(node, podName)
	}

	wg.Wait()

	return result
}

// setConfigDegraded transitions the cluster to ConfigDegraded phase and sets
// the DynamicConfigDegraded condition with details about which pods have
// inconsistent configuration. This is called when a dynamic config rollback fails.
func (r *AerospikeClusterReconciler) setConfigDegraded(
	ctx context.Context,
	cluster *ackov1alpha1.AerospikeCluster,
	failedPods []string,
) {
	log := logf.FromContext(ctx)

	latest, err := r.refetchCluster(ctx, types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace})
	if err != nil {
		log.Error(err, "Failed to re-fetch cluster for ConfigDegraded status update")
		return
	}

	base := latest.DeepCopy()
	latest.Status.Phase = ackov1alpha1.AerospikePhaseConfigDegraded
	latest.Status.PhaseReason = fmt.Sprintf("Dynamic config rollback failed on pods: %s", strings.Join(failedPods, ", "))

	setCondition(latest, ackov1alpha1.ConditionDynamicConfigDegraded, true,
		"RollbackFailed",
		fmt.Sprintf("Dynamic config rollback failed on %d pod(s): %s. Cold restart required.",
			len(failedPods), strings.Join(failedPods, ", ")))

	if err := r.Status().Patch(ctx, latest, client.MergeFrom(base)); err != nil {
		log.Error(err, "Failed to set ConfigDegraded status")
		return
	}
	metrics.ClusterPhase.WithLabelValues(latest.Namespace, latest.Name).
		Set(metrics.PhaseToFloat(string(ackov1alpha1.AerospikePhaseConfigDegraded)))

	r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventDynamicConfigDegraded,
		"Cluster entered ConfigDegraded phase: rollback failed on pods %s", strings.Join(failedPods, ", "))
}

// populateExternalEndpoints reads per-pod and seeds finder LoadBalancer/NodePort
// services to populate status.Endpoints and status.SeedsEndpoint.
func (r *AerospikeClusterReconciler) populateExternalEndpoints(
	ctx context.Context,
	cluster *ackov1alpha1.AerospikeCluster,
) {
	log := logf.FromContext(ctx)

	// Collect per-pod external endpoints.
	var endpoints []string
	svcList := &corev1.ServiceList{}
	matchLabels := utils.SelectorLabelsForCluster(cluster.Name)
	if err := r.List(ctx, svcList,
		client.InNamespace(cluster.Namespace),
		client.MatchingLabels(matchLabels),
		client.HasLabels{podServiceLabel},
	); err != nil {
		log.V(1).Info("Could not list pod services for endpoint status", "err", err)
	} else {
		for i := range svcList.Items {
			svc := &svcList.Items[i]
			if ep := externalEndpoint(svc); ep != "" {
				endpoints = append(endpoints, ep)
			}
		}
	}
	slices.Sort(endpoints)
	cluster.Status.Endpoints = strings.Join(endpoints, ",")

	// Seeds finder LB endpoint.
	seedsSvcName := seedsFinderServiceName(cluster.Name)
	seedsSvc := &corev1.Service{}
	if err := r.Get(ctx, types.NamespacedName{Name: seedsSvcName, Namespace: cluster.Namespace}, seedsSvc); err == nil {
		cluster.Status.SeedsEndpoint = externalEndpoint(seedsSvc)
	} else {
		cluster.Status.SeedsEndpoint = ""
	}
}

// externalEndpoint returns "host:port" from a LoadBalancer or NodePort service.
// Returns empty string if no external endpoint is available.
func externalEndpoint(svc *corev1.Service) string {
	if len(svc.Spec.Ports) == 0 {
		return ""
	}
	port := svc.Spec.Ports[0].Port

	if svc.Spec.Type == corev1.ServiceTypeLoadBalancer {
		for _, ing := range svc.Status.LoadBalancer.Ingress {
			host := ing.IP
			if host == "" {
				host = ing.Hostname
			}
			if host != "" {
				return fmt.Sprintf("%s:%d", host, port)
			}
		}
	}

	if svc.Spec.Type == corev1.ServiceTypeNodePort && len(svc.Spec.Ports) > 0 {
		np := svc.Spec.Ports[0].NodePort
		if np > 0 {
			return fmt.Sprintf("<node-ip>:%d", np)
		}
	}

	return ""
}

// parseServiceEndpoints splits the asinfo "service" response (semicolon-separated
// "host:port" entries) into a string slice.
func parseServiceEndpoints(serviceStr string) []string {
	serviceStr = strings.TrimSpace(serviceStr)
	if serviceStr == "" {
		return nil
	}
	parts := strings.Split(serviceStr, ";")
	endpoints := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			endpoints = append(endpoints, p)
		}
	}
	return endpoints
}
