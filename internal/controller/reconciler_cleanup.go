package controller

import (
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	ackov1alpha1 "github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/api/v1alpha1"
	"github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/internal/storage"
	"github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/internal/utils"
)

func (r *AerospikeClusterReconciler) handleDeletion(
	ctx context.Context,
	cluster *ackov1alpha1.AerospikeCluster,
) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(cluster, utils.StorageFinalizer) {
		return ctrl.Result{}, nil
	}

	log.Info("Handling cluster deletion")
	r.Recorder.Eventf(cluster, corev1.EventTypeNormal, EventClusterDeletionStarted,
		"Cluster deletion started, cleaning up resources")

	// Set Deleting phase so observers know the cluster is being removed.
	if err := r.setPhase(ctx, cluster, ackov1alpha1.AerospikePhaseDeleting, "Cluster is being deleted"); err != nil {
		if !k8serrors.IsConflict(err) {
			return ctrl.Result{}, err
		}
		// Conflict is non-fatal here; the deletion will proceed regardless.
	}

	// Selectively delete PVCs for volumes that have cascadeDelete enabled.
	// Continue through all StatefulSets even if some fail, to clean up as much as possible.
	if cluster.Spec.Storage != nil {
		stsList, err := r.listClusterStatefulSets(ctx, cluster)
		if err != nil {
			return ctrl.Result{}, err
		}
		var pvcErrors []error
		for _, sts := range stsList.Items {
			if err := storage.DeleteCascadeDeletePVCs(ctx, r.Client, cluster.Namespace, cluster.Name, sts.Name, cluster.Spec.Storage); err != nil {
				if !k8serrors.IsNotFound(err) {
					log.Error(err, "Failed to delete cascade PVCs for StatefulSet", "statefulset", sts.Name)
					r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventPVCCleanupFailed,
						"Failed to delete cascade PVCs for StatefulSet %s: %v", sts.Name, err)
					pvcErrors = append(pvcErrors, fmt.Errorf("statefulset %s: %w", sts.Name, err))
				}
			}
		}
		if len(pvcErrors) > 0 {
			return ctrl.Result{}, fmt.Errorf("failed to delete cascade PVCs for %d StatefulSet(s): %w",
				len(pvcErrors), errors.Join(pvcErrors...))
		}
		log.Info("Cascade-delete PVCs cleaned up for cluster deletion")
	}

	// Re-fetch before removing finalizer to avoid conflict on stale object.
	latest, err := r.refetchCluster(ctx, types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace})
	if err != nil {
		return ctrl.Result{}, err
	}

	controllerutil.RemoveFinalizer(latest, utils.StorageFinalizer)
	// Emit event before Update: once the finalizer is removed the object is
	// immediately eligible for garbage collection, so the event must be
	// recorded while the object still exists in the API server.
	r.Recorder.Eventf(latest, corev1.EventTypeNormal, EventFinalizerRemoved,
		"Storage finalizer removed, cluster deletion proceeding")
	if err := r.Update(ctx, latest); err != nil {
		if k8serrors.IsConflict(err) {
			log.V(1).Info("Conflict removing finalizer, will requeue")
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, err
	}

	log.Info("Cluster deletion handled successfully")
	return ctrl.Result{}, nil
}
