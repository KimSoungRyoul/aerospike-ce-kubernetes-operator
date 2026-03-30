package controller

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"slices"
	"sync"
	"time"
	"unicode/utf8"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	ackov1alpha1 "github.com/ksr/aerospike-ce-kubernetes-operator/api/v1alpha1"
	ackoerrors "github.com/ksr/aerospike-ce-kubernetes-operator/internal/errors"
	"github.com/ksr/aerospike-ce-kubernetes-operator/internal/metrics"
	aerotmpl "github.com/ksr/aerospike-ce-kubernetes-operator/internal/template"
	"github.com/ksr/aerospike-ce-kubernetes-operator/internal/utils"
)

const (
	defaultReconcileRetryInterval = 5 * time.Second

	// restartRequeueInterval is the requeue interval used after a rolling restart
	// batch. Pod restart (delete + recreate + readiness) typically takes 30-60s,
	// so polling every 5s is wasteful.
	restartRequeueInterval = 15 * time.Second

	// migrationRequeueInterval is the requeue interval used when scale-down or
	// rolling restart is deferred due to active data migrations. Migrations can
	// take minutes to hours, so polling every 5s creates unnecessary API server load.
	migrationRequeueInterval = 30 * time.Second

	// podReadyPollInterval is the requeue interval used when reconciliation
	// completes successfully but not all pods are ready yet. The controller
	// does not watch pod readiness events directly, so periodic polling is
	// required to detect when pods transition to Ready and update status
	// conditions (Available, Ready).
	podReadyPollInterval = 10 * time.Second

	// reconcileTimeout is the maximum duration for a single reconciliation loop.
	// If the context deadline is exceeded, the reconcile will be retried with backoff.
	reconcileTimeout = 5 * time.Minute

	// maxFailedReconciles is the circuit breaker threshold. After this many
	// consecutive failures, the operator backs off exponentially to prevent
	// excessive retries on persistently failing clusters.
	maxFailedReconciles int32 = 10

	// maxBackoffSeconds is the maximum backoff duration (5 minutes) for the
	// exponential backoff used by the circuit breaker.
	maxBackoffSeconds = 300
)

// AerospikeClusterReconciler reconciles an AerospikeCluster object.
type AerospikeClusterReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	Recorder   record.EventRecorder
	RestConfig *rest.Config
	// KubeClientset is a cached kubernetes.Clientset for pod exec operations.
	KubeClientset kubernetes.Interface
	kubeClientMu  sync.Mutex
}

// RBAC markers
// +kubebuilder:rbac:groups=acko.io,resources=aerospikeclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=acko.io,resources=aerospikeclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=acko.io,resources=aerospikeclusters/finalizers,verbs=update
// +kubebuilder:rbac:groups=acko.io,resources=aerospikeclustertemplates,verbs=get;list;watch
// +kubebuilder:rbac:groups=acko.io,resources=aerospikeclustertemplates/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;delete
// +kubebuilder:rbac:groups="",resources=pods/status,verbs=patch
// +kubebuilder:rbac:groups="",resources=pods/exec,verbs=create
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=policy,resources=poddisruptionbudgets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=prometheusrules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cilium.io,resources=ciliumnetworkpolicies,verbs=get;list;watch;create;update;patch;delete

func (r *AerospikeClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Apply reconcile timeout to prevent infinite execution.
	ctx, cancel := context.WithTimeout(ctx, reconcileTimeout)
	defer cancel()

	log := logf.FromContext(ctx)
	reconcileStart := time.Now()

	// 1. Fetch CR
	cluster := &ackov1alpha1.AerospikeCluster{}
	if err := r.Get(ctx, req.NamespacedName, cluster); err != nil {
		if errors.IsNotFound(err) {
			// Cluster deleted — clean up metrics
			metrics.CleanupClusterMetrics(req.Namespace, req.Name)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Record reconcile duration on exit
	defer func() {
		metrics.ReconcileDuration.WithLabelValues(cluster.Namespace, cluster.Name).
			Observe(time.Since(reconcileStart).Seconds())
	}()

	// 2. Handle deletion
	if !cluster.DeletionTimestamp.IsZero() {
		result, err := r.handleDeletion(ctx, cluster)
		if err == nil {
			metrics.CleanupClusterMetrics(cluster.Namespace, cluster.Name)
		}
		return result, err
	}

	// 3. Add finalizer
	if !controllerutil.ContainsFinalizer(cluster, utils.StorageFinalizer) {
		controllerutil.AddFinalizer(cluster, utils.StorageFinalizer)
		if err := r.Update(ctx, cluster); err != nil {
			if errors.IsConflict(err) {
				return ctrl.Result{Requeue: true}, nil
			}
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// 4. Check if paused
	wasPaused := cluster.Status.Phase == ackov1alpha1.AerospikePhasePaused
	isPaused := cluster.Spec.Paused != nil && *cluster.Spec.Paused

	if isPaused {
		if err := r.HandlePause(ctx, cluster); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	if wasPaused && !isPaused {
		if err := r.HandleResume(ctx, cluster); err != nil {
			log.Error(err, "Failed to handle resume from paused state")
			return ctrl.Result{Requeue: true}, nil
		}
	}

	// Circuit breaker: if consecutive failures exceed threshold, back off exponentially.
	if cluster.Status.FailedReconcileCount >= maxFailedReconciles {
		backoff := calculateBackoff(cluster.Status.FailedReconcileCount)
		log.Info("Circuit breaker active, backing off",
			"failedCount", cluster.Status.FailedReconcileCount,
			"backoff", backoff,
			"lastError", cluster.Status.LastReconcileError)
		metrics.CircuitBreakerActive.WithLabelValues(cluster.Namespace, cluster.Name).Set(1)
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventCircuitBreakerActive,
			"Circuit breaker active after %d consecutive failures, backing off %v. Last error: %s",
			cluster.Status.FailedReconcileCount, backoff, cluster.Status.LastReconcileError)
		return ctrl.Result{RequeueAfter: backoff}, nil
	}
	metrics.CircuitBreakerActive.WithLabelValues(cluster.Namespace, cluster.Name).Set(0)

	// 4.5 Template resolution: fetch/snapshot template and apply to in-memory spec.
	if cluster.Spec.TemplateRef != nil {
		if result, err := r.resolveTemplate(ctx, cluster); err != nil {
			return r.handleReconcileError(ctx, cluster, err)
		} else if result != nil {
			return *result, nil
		}
	}

	// 5. Set phase to InProgress only when the spec has actually changed
	// (i.e., observedGeneration is behind the current generation).
	// This prevents a Completed->InProgress->Completed feedback loop
	// where each status update triggers a new reconcile.
	if cluster.Status.ObservedGeneration != cluster.Generation ||
		cluster.Status.Phase == "" {
		if err := r.setPhase(ctx, cluster, ackov1alpha1.AerospikePhaseInProgress, "Reconciliation started"); err != nil {
			if errors.IsConflict(err) {
				return ctrl.Result{Requeue: true}, nil
			}
			return r.handleReconcileError(ctx, cluster, err)
		}
	}

	// 6. Reconcile headless service
	if err := r.reconcileHeadlessService(ctx, cluster); err != nil {
		log.Error(err, "Failed to reconcile headless service", "cluster", cluster.Name)
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReconcileError, "Headless service: %v", err)
		metrics.ReconcileErrorsTotal.WithLabelValues(cluster.Namespace, cluster.Name, metrics.ReasonService).Inc()
		return r.handleReconcileError(ctx, cluster, err)
	}

	// 6b. Reconcile per-pod services
	if err := r.reconcilePodServices(ctx, cluster); err != nil {
		log.Error(err, "Failed to reconcile per-pod services", "cluster", cluster.Name)
		metrics.ReconcileErrorsTotal.WithLabelValues(cluster.Namespace, cluster.Name, metrics.ReasonService).Inc()
		return r.handleReconcileError(ctx, cluster, err)
	}

	// 7-17. Reconcile cluster resources
	result, err := r.reconcileCluster(ctx, req.NamespacedName, cluster)
	if err != nil {
		return r.handleReconcileError(ctx, cluster, err)
	}

	// Reconcile succeeded — reset circuit breaker counter if previously non-zero.
	if cluster.Status.FailedReconcileCount > 0 {
		if resetErr := r.resetFailedReconcileCount(ctx, cluster); resetErr != nil {
			log.Error(resetErr, "Failed to reset circuit breaker counter")
			// Non-fatal: the counter will be reset on the next successful reconcile.
		}
	}

	return result, nil
}

// resolveTemplate handles template resolution, snapshot persistence, and annotation cleanup.
// Returns (nil, nil) on success, (*result, nil) if a requeue is needed, or (nil, err) on failure.
func (r *AerospikeClusterReconciler) resolveTemplate(
	ctx context.Context,
	cluster *ackov1alpha1.AerospikeCluster,
) (*ctrl.Result, error) {
	log := logf.FromContext(ctx)

	resolveResult, err := aerotmpl.Resolve(ctx, r.Client, cluster)
	if err != nil {
		log.Error(err, "Failed to resolve template", "template", cluster.Spec.TemplateRef.Name)
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventTemplateResolutionError,
			"Failed to resolve template %q: %v", cluster.Spec.TemplateRef.Name, err)
		return nil, err
	}
	if resolveResult.SnapshotUpdated {
		// Persist the new snapshot to the API server immediately so that
		// subsequent setPhase/updateStatusAndPhase calls (which re-fetch
		// the object) do not overwrite it with the stale version.
		// Status must be persisted BEFORE the Annotation Patch: the Patch
		// response refreshes the full cluster object (incl. Status) from the
		// server, which would nil-out the in-memory snapshot if Status has
		// not been saved yet.
		if err := r.Status().Update(ctx, cluster); err != nil {
			if errors.IsConflict(err) {
				return &ctrl.Result{Requeue: true}, nil
			}
			return nil, err
		}
		// Re-apply the template to the in-memory spec. Status().Update refreshes
		// the cluster object from the API server response, which resets the Spec
		// to the server-side version and discards in-memory changes made by
		// ApplyTemplate (e.g., PodAntiAffinity, Resources from the template).
		aerotmpl.ApplyTemplate(aerotmpl.MergeTemplateSpec(
			cluster.Status.TemplateSnapshot.Spec, cluster.Spec.Overrides), cluster)
	}
	// Remove the resync annotation from the API server now that the snapshot is persisted.
	// This Patch runs after Status.Update so it does not overwrite the snapshot.
	if resolveResult.AnnotationNeedsCleanup {
		patch := client.MergeFrom(cluster.DeepCopy())
		delete(cluster.Annotations, aerotmpl.AnnotationResyncTemplate)
		if err := r.Patch(ctx, cluster, patch); err != nil {
			if errors.IsConflict(err) {
				return &ctrl.Result{Requeue: true}, nil
			}
			return nil, err
		}
		// Re-apply after Patch too, which also refreshes the cluster object.
		aerotmpl.ApplyTemplate(aerotmpl.MergeTemplateSpec(
			cluster.Status.TemplateSnapshot.Spec, cluster.Spec.Overrides), cluster)
	}
	if resolveResult.SnapshotUpdated && cluster.Status.TemplateSnapshot != nil {
		r.Recorder.Eventf(cluster, corev1.EventTypeNormal, EventTemplateApplied,
			"Applied template %q (rv: %s)",
			cluster.Spec.TemplateRef.Name,
			cluster.Status.TemplateSnapshot.ResourceVersion)
	}
	for _, w := range resolveResult.Warnings {
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventValidationWarning, "%s", w)
	}
	return nil, nil
}

// handleReconcileError increments the failed reconcile count in the cluster status
// and returns the appropriate result with exponential backoff.
func (r *AerospikeClusterReconciler) handleReconcileError(
	ctx context.Context,
	cluster *ackov1alpha1.AerospikeCluster,
	reconcileErr error,
) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Use a detached context so status writes succeed even if the reconcile ctx timed out.
	updateCtx, updateCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer updateCancel()

	// Re-fetch to avoid conflict on a stale object.
	latest, err := r.refetchCluster(updateCtx, types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace})
	if err != nil {
		// Cannot update status — return original error and let controller-runtime retry.
		log.Error(err, "Failed to re-fetch cluster for error tracking")
		return ctrl.Result{}, reconcileErr
	}

	// Truncate error message to avoid bloating the status object.
	errMsg := truncateUTF8(reconcileErr.Error(), 256)

	// Validation errors are permanent and will never self-heal.
	// Immediately activate the circuit breaker to prevent wasteful retries.
	if ackoerrors.IsValidation(reconcileErr) {
		log.Info("Permanent validation error detected, activating circuit breaker immediately",
			"error", reconcileErr)
		latest.Status.FailedReconcileCount = maxFailedReconciles
		latest.Status.LastReconcileError = errMsg
		setCondition(latest, ackov1alpha1.ConditionReconcileHealthy, false,
			"PermanentError", errMsg)
		if err := r.Status().Update(updateCtx, latest); err != nil {
			log.Error(err, "Failed to update validation error in status")
			return ctrl.Result{}, reconcileErr
		}
		cluster.Status.FailedReconcileCount = latest.Status.FailedReconcileCount
		cluster.Status.LastReconcileError = latest.Status.LastReconcileError
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventPermanentError,
			"Permanent validation error, automatic retries halted: %s", errMsg)
		backoff := calculateBackoff(maxFailedReconciles)
		return ctrl.Result{RequeueAfter: backoff}, nil
	}

	latest.Status.FailedReconcileCount++
	latest.Status.LastReconcileError = errMsg

	if err := r.Status().Update(updateCtx, latest); err != nil {
		log.Error(err, "Failed to update failed reconcile count in status")
		// Return original error; the counter will be incremented on the next attempt.
		return ctrl.Result{}, reconcileErr
	}

	// Propagate updated fields back to the caller's object.
	cluster.Status.FailedReconcileCount = latest.Status.FailedReconcileCount
	cluster.Status.LastReconcileError = latest.Status.LastReconcileError

	backoff := calculateBackoff(latest.Status.FailedReconcileCount)
	log.Error(reconcileErr, "Reconcile failed, scheduling retry with backoff",
		"failedCount", latest.Status.FailedReconcileCount,
		"backoff", backoff)

	return ctrl.Result{RequeueAfter: backoff}, nil
}

// resetFailedReconcileCount resets the circuit breaker counter after a successful reconcile.
func (r *AerospikeClusterReconciler) resetFailedReconcileCount(
	ctx context.Context,
	cluster *ackov1alpha1.AerospikeCluster,
) error {
	log := logf.FromContext(ctx)

	latest, err := r.refetchCluster(ctx, types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace})
	if err != nil {
		return err
	}

	if latest.Status.FailedReconcileCount == 0 && latest.Status.LastReconcileError == "" {
		return nil
	}

	prevCount := latest.Status.FailedReconcileCount
	latest.Status.FailedReconcileCount = 0
	latest.Status.LastReconcileError = ""
	setCondition(latest, ackov1alpha1.ConditionReconcileHealthy, true,
		"ReconcileSucceeded", "Reconciliation succeeded")

	if err := r.Status().Update(ctx, latest); err != nil {
		return err
	}

	cluster.Status.FailedReconcileCount = 0
	cluster.Status.LastReconcileError = ""

	log.Info("Circuit breaker counter reset after successful reconcile", "previousFailedCount", prevCount)
	if prevCount >= maxFailedReconciles {
		r.Recorder.Eventf(cluster, corev1.EventTypeNormal, EventCircuitBreakerReset,
			"Circuit breaker reset after successful reconcile (was %d consecutive failures)", prevCount)
	}
	return nil
}

// calculateBackoff computes the exponential backoff duration for the given
// consecutive failure count. Uses 2^n seconds, capped at maxBackoffSeconds (5 min).
func calculateBackoff(failCount int32) time.Duration {
	if failCount <= 0 {
		return defaultReconcileRetryInterval
	}
	// Cap the exponent to avoid overflow: 2^8 = 256s which is < maxBackoffSeconds (300).
	exponent := min(failCount, 8)
	seconds := math.Pow(2, float64(exponent))
	if seconds > float64(maxBackoffSeconds) {
		seconds = float64(maxBackoffSeconds)
	}
	return time.Duration(seconds) * time.Second
}

// reconcileCluster reconciles all cluster resources (racks, services, operations, ACL, status).
func (r *AerospikeClusterReconciler) reconcileCluster(
	ctx context.Context,
	namespacedName types.NamespacedName,
	cluster *ackov1alpha1.AerospikeCluster,
) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.V(1).Info("Starting cluster reconciliation")
	racks := r.getRacks(cluster)

	// Pre-compute effective config and hash per rack.
	rackInfos := make([]rackInfo, len(racks))
	rackSizes := make([]int32, len(racks))
	for i, rack := range racks {
		ec := r.getEffectiveConfig(cluster, &rack)
		rackSizes[i] = r.getRackSize(cluster, racks, i)
		rackInfos[i] = rackInfo{
			effectiveConfig: ec,
			hash:            configHash(ec),
			rackSize:        rackSizes[i],
		}
	}

	// Detect scaling and update phase accordingly.
	scalingUp, scalingDown, err := r.detectScaling(ctx, cluster, racks, rackSizes)
	if err != nil {
		return ctrl.Result{}, err
	}
	if scalingUp {
		if err := r.setPhase(ctx, cluster, ackov1alpha1.AerospikePhaseScalingUp, "Scaling up cluster"); err != nil {
			if !errors.IsConflict(err) {
				return ctrl.Result{}, err
			}
			log.V(1).Info("Conflict setting ScalingUp phase, continuing reconcile")
		}
	} else if scalingDown {
		if err := r.setPhase(ctx, cluster, ackov1alpha1.AerospikePhaseScalingDown, "Scaling down cluster"); err != nil {
			if !errors.IsConflict(err) {
				return ctrl.Result{}, err
			}
			log.V(1).Info("Conflict setting ScalingDown phase, continuing reconcile")
		}
	}

	// Reconcile each rack's ConfigMap + StatefulSet.
	// reconcileRacks returns true if any scale-down was deferred due to migration.
	if deferred, err := r.reconcileRacks(ctx, cluster, racks, rackInfos); err != nil {
		return ctrl.Result{}, err
	} else if deferred {
		log.Info("Scale-down deferred due to data migration, requeuing")
		return ctrl.Result{RequeueAfter: migrationRequeueInterval}, nil
	}

	// Clean up removed racks
	if err := r.cleanupRemovedRacks(ctx, cluster, racks); err != nil {
		metrics.ReconcileErrorsTotal.WithLabelValues(cluster.Namespace, cluster.Name, metrics.ReasonStatefulSet).Inc()
		return ctrl.Result{}, err
	}

	// Reconcile auxiliary resources: PDB, Monitoring, NetworkPolicy
	auxReasons := []string{metrics.ReasonPDB, metrics.ReasonMonitoring, metrics.ReasonNetPolicy}
	for idx, fn := range []func(context.Context, *ackov1alpha1.AerospikeCluster) error{
		r.reconcilePDB,
		r.reconcileMonitoring,
		r.reconcileNetworkPolicy,
	} {
		if err := fn(ctx, cluster); err != nil {
			metrics.ReconcileErrorsTotal.WithLabelValues(cluster.Namespace, cluster.Name, auxReasons[idx]).Inc()
			return ctrl.Result{}, err
		}
	}

	// Handle on-demand operations
	if inProgress, err := r.reconcileOperations(ctx, cluster); err != nil {
		metrics.ReconcileErrorsTotal.WithLabelValues(cluster.Namespace, cluster.Name, metrics.ReasonOperations).Inc()
		return ctrl.Result{}, err
	} else if inProgress {
		return ctrl.Result{RequeueAfter: defaultReconcileRetryInterval}, nil
	}

	// Sync pod readiness gates (no-op when feature is disabled).
	// Must run before the rolling restart loop so the gate state is up-to-date
	// when anyPodGateUnsatisfied() is checked inside reconcileRollingRestart.
	if err := r.syncAllPodsReadinessGates(ctx, cluster); err != nil {
		log.Error(err, "Failed to sync pod readiness gates")
		// Non-fatal: gate sync errors leave gates as-is.
		// anyPodGateUnsatisfied() will safely hold the rolling restart.
	}

	// Rolling restart if needed
	for _, rack := range racks {
		restarted, err := r.reconcileRollingRestart(ctx, cluster, &rack)
		if err != nil {
			log.Error(err, "Failed rolling restart", "rack", rack.ID, "statefulset", utils.StatefulSetName(cluster.Name, rack.ID))
			metrics.ReconcileErrorsTotal.WithLabelValues(cluster.Namespace, cluster.Name, metrics.ReasonRestart).Inc()
			return ctrl.Result{}, err
		}
		if restarted {
			if err := r.setPhase(ctx, cluster, ackov1alpha1.AerospikePhaseRollingRestart,
				fmt.Sprintf("Rolling restart in progress for rack %d", rack.ID)); err != nil {
				if !errors.IsConflict(err) {
					return ctrl.Result{}, err
				}
				log.V(1).Info("Conflict setting RollingRestart phase, continuing reconcile", "rack", rack.ID)
			}
			return ctrl.Result{RequeueAfter: restartRequeueInterval}, nil
		}
	}

	// Reconcile ACL (non-fatal); capture error and skip flag for ACLSynced condition.
	var aclErr error
	aclSynced := false
	if cluster.Spec.AerospikeAccessControl != nil {
		if err := r.setPhase(ctx, cluster, ackov1alpha1.AerospikePhaseACLSync, "Synchronizing ACL roles and users"); err != nil {
			if errors.IsConflict(err) {
				log.V(1).Info("Conflict setting ACLSync phase, continuing reconcile")
			} else {
				return ctrl.Result{}, err
			}
		}
	}
	if synced, err := r.reconcileACL(ctx, cluster); err != nil {
		log.Error(err, "Failed to reconcile ACL", "cluster", cluster.Name)
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventACLSyncError, "ACL sync failed: %v", err)
		metrics.ReconcileErrorsTotal.WithLabelValues(cluster.Namespace, cluster.Name, metrics.ReasonACL).Inc()
		aclErr = err
	} else {
		aclSynced = synced
	}

	// Update status and set phase to Completed.
	statusOpts := StatusUpdateOpts{ACLErr: aclErr, ACLSynced: aclSynced}
	if err := r.updateStatusAndPhase(ctx, namespacedName, ackov1alpha1.AerospikePhaseCompleted, "Cluster is healthy and stable", statusOpts); err != nil {
		if errors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		metrics.ReconcileErrorsTotal.WithLabelValues(cluster.Namespace, cluster.Name, metrics.ReasonStatus).Inc()
		return ctrl.Result{}, err
	}

	log.Info("Reconciliation completed successfully")

	// The controller does not watch pod readiness events directly (StatefulSet
	// Owns() uses GenerationChangedPredicate which ignores status-only updates).
	// If not all pods are ready yet, poll periodically so that the Available and
	// Ready conditions are updated once the pods finish starting up.
	latest, err := r.refetchCluster(ctx, namespacedName)
	if err == nil && latest.Status.Size < latest.Spec.Size {
		log.Info("Not all pods ready yet, requeuing for status update",
			"readyPods", latest.Status.Size, "desiredSize", latest.Spec.Size)
		return ctrl.Result{RequeueAfter: podReadyPollInterval}, nil
	}

	return ctrl.Result{}, nil
}

// rackInfo holds pre-computed per-rack configuration used during reconciliation.
type rackInfo struct {
	effectiveConfig *ackov1alpha1.AerospikeConfigSpec
	hash            string
	rackSize        int32
}

// reconcileRacks reconciles each rack's ConfigMap and StatefulSet.
// Returns (deferred, error). deferred is true when at least one rack's
// scale-down was blocked because data migration is still in progress.
func (r *AerospikeClusterReconciler) reconcileRacks(
	ctx context.Context,
	cluster *ackov1alpha1.AerospikeCluster,
	racks []ackov1alpha1.Rack,
	rackInfos []rackInfo,
) (bool, error) {
	log := logf.FromContext(ctx)
	log.V(1).Info("Reconciling racks", "count", len(racks))

	scaleDownDeferred := false
	for i, rack := range racks {
		ri := rackInfos[i]
		if err := r.reconcileConfigMap(ctx, cluster, &rack, ri.effectiveConfig); err != nil {
			metrics.ReconcileErrorsTotal.WithLabelValues(cluster.Namespace, cluster.Name, metrics.ReasonConfigMap).Inc()
			return false, err
		}
		deferred, err := r.reconcileStatefulSet(ctx, cluster, &rack, ri.effectiveConfig, ri.hash, ri.rackSize)
		if err != nil {
			metrics.ReconcileErrorsTotal.WithLabelValues(cluster.Namespace, cluster.Name, metrics.ReasonStatefulSet).Inc()
			return false, err
		}
		if deferred {
			scaleDownDeferred = true
		}
	}
	return scaleDownDeferred, nil
}

// getRacks returns the list of racks. If no rack config, returns a default rack.
func (r *AerospikeClusterReconciler) getRacks(cluster *ackov1alpha1.AerospikeCluster) []ackov1alpha1.Rack {
	if cluster.Spec.RackConfig != nil && len(cluster.Spec.RackConfig.Racks) > 0 {
		return cluster.Spec.RackConfig.Racks
	}
	return []ackov1alpha1.Rack{{ID: 0}}
}

// getRackSize returns the number of pods for a given rack.
func (r *AerospikeClusterReconciler) getRackSize(cluster *ackov1alpha1.AerospikeCluster, racks []ackov1alpha1.Rack, rackIndex int) int32 {
	totalSize := cluster.Spec.Size
	numRacks := int32(len(racks))
	baseSize := totalSize / numRacks
	remainder := totalSize % numRacks

	if int32(rackIndex) < remainder {
		return baseSize + 1
	}
	return baseSize
}

// setPhase re-fetches the latest cluster object and updates its phase and reason.
// Uses RetryOnConflict to handle transient conflict errors without requiring
// a full requeue cycle.
func (r *AerospikeClusterReconciler) setPhase(ctx context.Context, cluster *ackov1alpha1.AerospikeCluster, phase ackov1alpha1.AerospikePhase, reason string) error {
	desiredPendingRestartPods := slices.Clone(cluster.Status.PendingRestartPods)
	nn := types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}

	var latestRV string
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		latest, fetchErr := r.refetchCluster(ctx, nn)
		if fetchErr != nil {
			return fetchErr
		}

		if latest.Status.Phase == phase &&
			latest.Status.PhaseReason == reason &&
			slices.Equal(latest.Status.PendingRestartPods, desiredPendingRestartPods) {
			latestRV = latest.ResourceVersion
			return nil
		}

		latest.Status.Phase = phase
		latest.Status.PhaseReason = reason
		latest.Status.PendingRestartPods = desiredPendingRestartPods
		if err := r.Status().Update(ctx, latest); err != nil {
			return err
		}
		latestRV = latest.ResourceVersion
		return nil
	})
	if err != nil {
		return err
	}

	// Propagate the updated resource version back to the caller's object
	// so subsequent operations in the same reconcile loop use fresh data.
	cluster.ResourceVersion = latestRV
	cluster.Status.Phase = phase
	cluster.Status.PhaseReason = reason
	cluster.Status.PendingRestartPods = slices.Clone(desiredPendingRestartPods)
	return nil
}

// configHash computes a deterministic SHA256 hash of the aerospike config for
// change detection. Uses json.Marshal which sorts map keys, unlike fmt.Sprintf
// which iterates maps in non-deterministic order.
func configHash(config *ackov1alpha1.AerospikeConfigSpec) string {
	if config == nil {
		return ""
	}
	return utils.ShortSHA256(config.Value)
}

// SetupWithManager sets up the controller with the Manager.
func (r *AerospikeClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ackov1alpha1.AerospikeCluster{},
			// AnnotationChangedPredicate allows the resync annotation
			// (acko.io/resync-template=true) to trigger reconciliation,
			// since annotation-only changes do not increment generation.
			builder.WithPredicates(predicate.Or(
				predicate.GenerationChangedPredicate{},
				predicate.AnnotationChangedPredicate{},
			)),
		).
		// For StatefulSets, trigger reconciliation on both spec changes (generation)
		// and ReadyReplicas status changes so that Available/Ready conditions on the
		// AerospikeCluster are updated as soon as pods transition to Ready.
		// Service/ConfigMap/PDB still use GenerationChangedPredicate since their
		// status changes are irrelevant to cluster readiness.
		Owns(&appsv1.StatefulSet{}, builder.WithPredicates(predicate.Or(
			predicate.GenerationChangedPredicate{},
			statefulSetReadyReplicasPredicate{},
		))).
		Owns(&corev1.Service{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Owns(&corev1.ConfigMap{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Owns(&policyv1.PodDisruptionBudget{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		// Watch AerospikeClusterTemplate changes and mark referencing clusters as out-of-sync.
		Watches(
			&ackov1alpha1.AerospikeClusterTemplate{},
			handler.EnqueueRequestsFromMapFunc(r.mapTemplateToCluster),
			builder.WithPredicates(predicate.GenerationChangedPredicate{}),
		).
		// Watch Secrets referenced by AerospikeCluster ACL users.
		// Secret data changes (e.g., password rotation) don't increment the CR's
		// generation, so an explicit watch is needed to trigger ACL re-sync.
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.mapSecretToCluster),
			builder.WithPredicates(secretDataChangedPredicate{}),
		).
		Named("aerospikecluster").
		WithOptions(controller.Options{MaxConcurrentReconciles: 2}).
		Complete(r)
}

// mapTemplateToCluster maps an AerospikeClusterTemplate change to all clusters
// that reference it, so the controller can mark them as out-of-sync.
func (r *AerospikeClusterReconciler) mapTemplateToCluster(ctx context.Context, obj client.Object) []reconcile.Request {
	log := logf.FromContext(ctx)

	// List all AerospikeClusters across all namespaces since templates are cluster-scoped.
	clusterList := &ackov1alpha1.AerospikeClusterList{}
	if err := r.List(ctx, clusterList); err != nil {
		log.Error(err, "Failed to list clusters for template watch", "template", obj.GetName())
		return nil
	}

	var requests []reconcile.Request
	for i := range clusterList.Items {
		cl := &clusterList.Items[i]
		if cl.Spec.TemplateRef == nil || cl.Spec.TemplateRef.Name != obj.GetName() {
			continue
		}

		// Mark the cluster as out-of-sync by updating its snapshot status.
		if cl.Status.TemplateSnapshot != nil && cl.Status.TemplateSnapshot.Synced {
			latest := cl.DeepCopy()
			latest.Status.TemplateSnapshot.Synced = false
			if err := r.Status().Update(ctx, latest); err != nil {
				// Conflict errors are expected when multiple reconciles run concurrently;
				// the enqueued reconcile request below will handle the drift on next loop.
				if !errors.IsConflict(err) {
					log.Error(err, "Failed to mark cluster template as drifted", "cluster", cl.Name)
				}
			} else {
				r.Recorder.Eventf(cl, corev1.EventTypeWarning, EventTemplateDrifted,
					"Template %q changed (rv: %s → %s); cluster using snapshot. Set annotation acko.io/resync-template=true to resync.",
					obj.GetName(),
					cl.Status.TemplateSnapshot.ResourceVersion,
					obj.GetResourceVersion(),
				)
			}
		}

		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: cl.Namespace,
				Name:      cl.Name,
			},
		})
	}
	return requests
}

// statefulSetReadyReplicasPredicate fires a reconcile when a StatefulSet's
// ReadyReplicas count changes. This is needed because GenerationChangedPredicate
// only reacts to spec changes (generation increments) and ignores status-only
// updates. Without this predicate, the AerospikeCluster's Available/Ready
// conditions would not be updated when pods finish starting up.
type statefulSetReadyReplicasPredicate struct {
	predicate.Funcs
}

func (statefulSetReadyReplicasPredicate) Update(e event.UpdateEvent) bool {
	oldSTS, ok := e.ObjectOld.(*appsv1.StatefulSet)
	if !ok {
		return false
	}
	newSTS, ok := e.ObjectNew.(*appsv1.StatefulSet)
	if !ok {
		return false
	}
	return oldSTS.Status.ReadyReplicas != newSTS.Status.ReadyReplicas
}

// mapSecretToCluster maps a Secret change to the AerospikeCluster(s) that
// reference it via aerospikeAccessControl.users[*].secretName.
func (r *AerospikeClusterReconciler) mapSecretToCluster(ctx context.Context, obj client.Object) []reconcile.Request {
	log := logf.FromContext(ctx)
	secretName := obj.GetName()
	secretNamespace := obj.GetNamespace()

	// List AerospikeClusters in the same namespace as the Secret.
	clusterList := &ackov1alpha1.AerospikeClusterList{}
	if err := r.List(ctx, clusterList, client.InNamespace(secretNamespace)); err != nil {
		log.Error(err, "Failed to list clusters for secret watch",
			"secret", secretName, "namespace", secretNamespace)
		return nil
	}

	var requests []reconcile.Request
	for i := range clusterList.Items {
		cl := &clusterList.Items[i]
		if cl.Spec.AerospikeAccessControl == nil {
			continue
		}
		for _, user := range cl.Spec.AerospikeAccessControl.Users {
			if user.SecretName == secretName {
				log.V(1).Info("Secret referenced by AerospikeCluster ACL, enqueuing reconcile",
					"secret", secretName, "cluster", cl.Name)
				requests = append(requests, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: cl.Namespace,
						Name:      cl.Name,
					},
				})
				break // one match per cluster is enough
			}
		}
	}
	return requests
}

// secretDataChangedPredicate triggers only on Update events where the Secret's
// Data or StringData has changed. Create and Delete events are ignored since
// new Secrets don't need immediate ACL sync and deleted Secrets will cause
// errors that are surfaced during the next scheduled reconcile.
type secretDataChangedPredicate struct {
	predicate.Funcs
}

func (secretDataChangedPredicate) Create(_ event.CreateEvent) bool   { return false }
func (secretDataChangedPredicate) Delete(_ event.DeleteEvent) bool   { return false }
func (secretDataChangedPredicate) Generic(_ event.GenericEvent) bool { return false }

func (secretDataChangedPredicate) Update(e event.UpdateEvent) bool {
	oldSecret, ok := e.ObjectOld.(*corev1.Secret)
	if !ok {
		return false
	}
	newSecret, ok := e.ObjectNew.(*corev1.Secret)
	if !ok {
		return false
	}
	// Compare actual Data content to avoid unnecessary reconciliation on
	// metadata-only changes (e.g., label updates that bump ResourceVersion).
	return !reflect.DeepEqual(oldSecret.Data, newSecret.Data)
}

// truncateUTF8 truncates s to at most maxBytes bytes without splitting
// multi-byte UTF-8 characters. If truncation occurs, "..." is appended
// within the maxBytes budget.
func truncateUTF8(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	const suffix = "..."
	limit := max(maxBytes-len(suffix), 0)
	truncated := s[:limit]
	for len(truncated) > 0 && !utf8.ValidString(truncated) {
		truncated = truncated[:len(truncated)-1]
	}
	return truncated + suffix
}
