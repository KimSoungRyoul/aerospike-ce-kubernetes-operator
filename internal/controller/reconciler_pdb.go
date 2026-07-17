package controller

import (
	"context"
	"fmt"
	"maps"

	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	ackov1alpha1 "github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/api/v1alpha1"
	"github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/internal/utils"
)

func (r *AerospikeClusterReconciler) reconcilePDB(
	ctx context.Context,
	cluster *ackov1alpha1.AerospikeCluster,
) error {
	log := logf.FromContext(ctx)

	// Check if PDB is disabled
	if cluster.Spec.DisablePDB != nil && *cluster.Spec.DisablePDB {
		// Delete PDB if it exists
		pdbName := utils.PDBName(cluster.Name)
		existing := &policyv1.PodDisruptionBudget{}
		if err := r.Get(ctx, types.NamespacedName{Name: pdbName, Namespace: cluster.Namespace}, existing); err == nil {
			if err := r.Delete(ctx, existing); err != nil && !errors.IsNotFound(err) {
				return fmt.Errorf("deleting PDB %s: %w", pdbName, err)
			}
		} else if !errors.IsNotFound(err) {
			return fmt.Errorf("getting PDB %s for deletion: %w", pdbName, err)
		}
		return nil
	}

	pdbName := utils.PDBName(cluster.Name)
	labels := utils.LabelsForCluster(cluster.Name)
	selectorLabels := utils.SelectorLabelsForCluster(cluster.Name)

	maxUnavailable := intstr.FromInt32(1)
	if cluster.Spec.MaxUnavailable != nil {
		maxUnavailable = *cluster.Spec.MaxUnavailable
	}

	existing := &policyv1.PodDisruptionBudget{}
	err := r.Get(ctx, types.NamespacedName{Name: pdbName, Namespace: cluster.Namespace}, existing)

	if errors.IsNotFound(err) {
		pdb := &policyv1.PodDisruptionBudget{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pdbName,
				Namespace: cluster.Namespace,
				Labels:    labels,
			},
			Spec: policyv1.PodDisruptionBudgetSpec{
				MaxUnavailable: &maxUnavailable,
				Selector: &metav1.LabelSelector{
					MatchLabels: selectorLabels,
				},
			},
		}
		if err := r.setOwnerRef(cluster, pdb); err != nil {
			return err
		}
		log.Info("Creating PDB", "name", pdbName)
		if err := r.Create(ctx, pdb); err != nil {
			return fmt.Errorf("creating PDB %s: %w", pdbName, err)
		}
		r.Recorder.Eventf(cluster, corev1.EventTypeNormal, EventPDBCreated, "Created PodDisruptionBudget %s", pdbName)
		return nil
	} else if err != nil {
		return fmt.Errorf("getting PDB %s: %w", pdbName, err)
	}

	if !pdbNeedsUpdate(existing, labels, selectorLabels, maxUnavailable) {
		return nil
	}

	if existing.Labels == nil {
		existing.Labels = make(map[string]string, len(labels))
	}
	maps.Copy(existing.Labels, labels)
	existing.Spec.MinAvailable = nil
	existing.Spec.MaxUnavailable = &maxUnavailable
	existing.Spec.Selector = &metav1.LabelSelector{MatchLabels: selectorLabels}
	log.Info("Updating PDB", "name", pdbName)
	if err := r.Update(ctx, existing); err != nil {
		return fmt.Errorf("updating PDB %s: %w", pdbName, err)
	}
	r.Recorder.Eventf(cluster, corev1.EventTypeNormal, EventPDBUpdated, "Updated PodDisruptionBudget %s", pdbName)
	return nil
}

func pdbNeedsUpdate(
	existing *policyv1.PodDisruptionBudget,
	desiredLabels map[string]string,
	desiredSelectorLabels map[string]string,
	desiredMaxUnavailable intstr.IntOrString,
) bool {
	desiredSelector := &metav1.LabelSelector{MatchLabels: desiredSelectorLabels}
	if existing.Spec.MinAvailable != nil || existing.Spec.MaxUnavailable == nil {
		return true
	}
	if !intOrStringEqual(*existing.Spec.MaxUnavailable, desiredMaxUnavailable) {
		return true
	}
	if !equality.Semantic.DeepEqual(existing.Spec.Selector, desiredSelector) {
		return true
	}
	for k, v := range desiredLabels {
		if existing.Labels[k] != v {
			return true
		}
	}
	return false
}

// intOrStringEqual compares two IntOrString values by type and value,
// avoiding the ambiguity of String()-based comparison where int(1) and
// string("1") would appear equal.
func intOrStringEqual(a, b intstr.IntOrString) bool {
	return a.Type == b.Type && a.IntVal == b.IntVal && a.StrVal == b.StrVal
}
