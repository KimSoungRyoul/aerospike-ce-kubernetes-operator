package controller

import (
	"context"
	"fmt"
	"maps"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	ackov1alpha1 "github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/api/v1alpha1"
	"github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/internal/utils"
)

// pdbPolicy is the disruption budget the operator wants for one selector. Exactly
// one of MaxUnavailable / MinAvailable is set, mirroring PodDisruptionBudgetSpec.
//
// Both shapes are needed because the two sources express the budget differently:
// an operator writes spec.maxUnavailable or rack.maxUnavailable, while the
// quorum-aware default is naturally a minAvailable ("keep a majority up").
// Converting the default to a maxUnavailable would lose meaning when the rack is
// scaled, since minAvailable is relative to the live pod count.
type pdbPolicy struct {
	MaxUnavailable *intstr.IntOrString
	MinAvailable   *intstr.IntOrString
}

// quorumMinAvailable returns the majority of rackSize: the number of pods that
// must stay available for the rack to keep a majority.
//
//	rackSize=1 -> 1   rackSize=2 -> 2   rackSize=3 -> 2
//	rackSize=4 -> 3   rackSize=5 -> 3   rackSize=6 -> 4
//
// Note that for rackSize 1 and 2 this permits ZERO voluntary disruption, so a
// `kubectl drain` of a node hosting one of those pods blocks until the operator
// raises spec.maxUnavailable or rack.maxUnavailable. That follows directly from
// the formula and is called out in the PR and the docs rather than smoothed over.
func quorumMinAvailable(rackSize int32) int32 {
	if rackSize < 1 {
		return 0
	}
	return rackSize/2 + 1
}

// effectivePDBPolicy resolves the budget for one rack.
//
// Precedence: rack.maxUnavailable > spec.maxUnavailable > quorum-aware default.
// An explicit value is clamped so the budget can never permit every pod in the
// rack to be evicted at once — a PDB that allows full disruption is not a budget,
// and the webhook check for this was warning-only, which "appears once in
// kubectl apply output and is not a control".
func effectivePDBPolicy(cluster *ackov1alpha1.AerospikeCluster, rack *ackov1alpha1.Rack, rackSize int32) pdbPolicy {
	configured := cluster.Spec.MaxUnavailable
	if rack != nil && rack.MaxUnavailable != nil {
		configured = rack.MaxUnavailable
	}

	if configured == nil {
		if rackSize < 1 {
			// Nothing to protect yet — spec.size is unresolved (a templateRef whose
			// snapshot has not been applied). Emit the historical default rather
			// than a degenerate minAvailable: 0, which would read as a budget while
			// permitting everything.
			mu := intstr.FromInt32(1)
			return pdbPolicy{MaxUnavailable: &mu}
		}
		mi := intstr.FromInt32(quorumMinAvailable(rackSize))
		return pdbPolicy{MinAvailable: &mi}
	}

	// Percentages are handed to Kubernetes untouched: it resolves them against
	// the live pod count, which is the semantics a user writing "50%" wants, and
	// the webhook rejects >= 100%.
	if configured.Type != intstr.Int {
		value := *configured
		return pdbPolicy{MaxUnavailable: &value}
	}

	capped := configured.IntVal
	if rackSize > 0 && capped > rackSize-1 {
		capped = rackSize - 1
	}
	if capped < 0 {
		capped = 0
	}
	value := intstr.FromInt32(capped)
	return pdbPolicy{MaxUnavailable: &value}
}

// reconcilePDB reconciles the cluster's PodDisruptionBudget(s).
//
// A multi-rack cluster gets ONE PDB PER RACK. A single cluster-wide PDB counts
// disruptions across every rack, so with 3 racks of 2 pods and maxUnavailable=1
// Kubernetes permits one eviction cluster-wide — but nothing stops a drain from
// taking both pods of the SAME rack in sequence as each eviction is allowed in
// turn, leaving that rack with nothing. Per-rack budgets make the constraint
// per-rack, which is what rack-awareness is for (#94).
//
// A single-rack cluster keeps the cluster-wide PDB it has today, same name and
// same selector, so nothing changes for the common case.
func (r *AerospikeClusterReconciler) reconcilePDB(
	ctx context.Context,
	cluster *ackov1alpha1.AerospikeCluster,
) error {
	racks := r.getRacks(cluster)
	perRack := len(racks) > 1

	if cluster.Spec.DisablePDB != nil && *cluster.Spec.DisablePDB {
		return r.deleteAllPDBs(ctx, cluster, racks)
	}

	if !perRack {
		// Single rack: cluster-wide PDB, unchanged in name and selector. Any
		// per-rack PDBs left over from a multi-rack topology are removed.
		if err := r.deleteRackPDBs(ctx, cluster, nil); err != nil {
			return err
		}
		rackSize := r.getRackSize(cluster, racks, 0)
		return r.reconcileOnePDB(ctx, cluster,
			utils.PDBName(cluster.Name),
			utils.SelectorLabelsForCluster(cluster.Name),
			effectivePDBPolicy(cluster, &racks[0], rackSize))
	}

	// Multi-rack: one PDB per rack. The cluster-wide PDB is removed so the two
	// do not both constrain the same pods with different budgets.
	if err := r.deleteClusterPDB(ctx, cluster); err != nil {
		return err
	}

	keep := make(map[string]bool, len(racks))
	for i := range racks {
		rack := &racks[i]
		name := utils.RackPDBName(cluster.Name, rack.ID)
		keep[name] = true

		rackSize := r.getRackSize(cluster, racks, i)
		// The rack label is what makes the selector per-rack; it is the same
		// label listRackPods selects on throughout the operator.
		selector := utils.LabelsForRack(cluster.Name, rack.ID)
		if err := r.reconcileOnePDB(ctx, cluster, name, selector,
			effectivePDBPolicy(cluster, rack, rackSize)); err != nil {
			return err
		}
	}

	// Racks dropped from the spec leave their PDB behind; it would keep
	// constraining evictions for pods that no longer exist.
	return r.deleteRackPDBs(ctx, cluster, keep)
}

// reconcileOnePDB creates or updates a single PodDisruptionBudget.
func (r *AerospikeClusterReconciler) reconcileOnePDB(
	ctx context.Context,
	cluster *ackov1alpha1.AerospikeCluster,
	pdbName string,
	selectorLabels map[string]string,
	policy pdbPolicy,
) error {
	log := logf.FromContext(ctx)
	labels := utils.LabelsForCluster(cluster.Name)

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
				MaxUnavailable: policy.MaxUnavailable,
				MinAvailable:   policy.MinAvailable,
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

	if !pdbNeedsUpdate(existing, labels, selectorLabels, policy) {
		return nil
	}

	if existing.Labels == nil {
		existing.Labels = make(map[string]string, len(labels))
	}
	maps.Copy(existing.Labels, labels)
	existing.Spec.MinAvailable = policy.MinAvailable
	existing.Spec.MaxUnavailable = policy.MaxUnavailable
	existing.Spec.Selector = &metav1.LabelSelector{MatchLabels: selectorLabels}
	log.Info("Updating PDB", "name", pdbName)
	if err := r.Update(ctx, existing); err != nil {
		return fmt.Errorf("updating PDB %s: %w", pdbName, err)
	}
	r.Recorder.Eventf(cluster, corev1.EventTypeNormal, EventPDBUpdated, "Updated PodDisruptionBudget %s", pdbName)
	return nil
}

// deletePDBByName removes a PDB if it exists.
func (r *AerospikeClusterReconciler) deletePDBByName(
	ctx context.Context,
	cluster *ackov1alpha1.AerospikeCluster,
	pdbName string,
) error {
	existing := &policyv1.PodDisruptionBudget{}
	err := r.Get(ctx, types.NamespacedName{Name: pdbName, Namespace: cluster.Namespace}, existing)
	if errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("getting PDB %s for deletion: %w", pdbName, err)
	}
	if err := r.Delete(ctx, existing); err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("deleting PDB %s: %w", pdbName, err)
	}
	logf.FromContext(ctx).Info("Deleted PDB", "name", pdbName)
	return nil
}

// deleteClusterPDB removes the cluster-wide PDB.
func (r *AerospikeClusterReconciler) deleteClusterPDB(
	ctx context.Context,
	cluster *ackov1alpha1.AerospikeCluster,
) error {
	return r.deletePDBByName(ctx, cluster, utils.PDBName(cluster.Name))
}

// deleteRackPDBs removes per-rack PDBs whose name is not in keep.
//
// Candidate names are derived from the racks the operator knows about plus the
// racks that currently have a PDB, found by listing on the cluster's own labels.
// Listing rather than guessing is what lets a rack removed from the spec have its
// PDB cleaned up: its ID is no longer in the spec to derive a name from.
func (r *AerospikeClusterReconciler) deleteRackPDBs(
	ctx context.Context,
	cluster *ackov1alpha1.AerospikeCluster,
	keep map[string]bool,
) error {
	pdbList := &policyv1.PodDisruptionBudgetList{}
	if err := r.List(ctx, pdbList,
		client.InNamespace(cluster.Namespace),
		client.MatchingLabels(utils.LabelsForCluster(cluster.Name)),
	); err != nil {
		return fmt.Errorf("listing PDBs for cluster %s: %w", cluster.Name, err)
	}

	clusterPDB := utils.PDBName(cluster.Name)
	for i := range pdbList.Items {
		name := pdbList.Items[i].Name
		if name == clusterPDB || keep[name] {
			continue
		}
		// Only touch names this operator would have produced, so a PDB a user
		// created and happened to label with the cluster's labels is left alone.
		if !isRackPDBName(name, cluster.Name) {
			continue
		}
		if err := r.deletePDBByName(ctx, cluster, name); err != nil {
			return err
		}
	}
	return nil
}

// deleteAllPDBs removes every PDB the operator manages for this cluster, used
// when spec.disablePDB is true.
func (r *AerospikeClusterReconciler) deleteAllPDBs(
	ctx context.Context,
	cluster *ackov1alpha1.AerospikeCluster,
	racks []ackov1alpha1.Rack,
) error {
	if err := r.deleteClusterPDB(ctx, cluster); err != nil {
		return err
	}
	return r.deleteRackPDBs(ctx, cluster, nil)
}

func pdbNeedsUpdate(
	existing *policyv1.PodDisruptionBudget,
	desiredLabels map[string]string,
	desiredSelectorLabels map[string]string,
	desired pdbPolicy,
) bool {
	desiredSelector := &metav1.LabelSelector{MatchLabels: desiredSelectorLabels}
	if !intOrStringPtrEqual(existing.Spec.MaxUnavailable, desired.MaxUnavailable) {
		return true
	}
	if !intOrStringPtrEqual(existing.Spec.MinAvailable, desired.MinAvailable) {
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

// intOrStringPtrEqual compares two optional IntOrString values, treating nil and
// a set value as different.
func intOrStringPtrEqual(a, b *intstr.IntOrString) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return intOrStringEqual(*a, *b)
}

// intOrStringEqual compares two IntOrString values by type and value,
// avoiding the ambiguity of String()-based comparison where int(1) and
// string("1") would appear equal.
func intOrStringEqual(a, b intstr.IntOrString) bool {
	return a.Type == b.Type && a.IntVal == b.IntVal && a.StrVal == b.StrVal
}

// isRackPDBName reports whether name matches the "<cluster>-<rackID>-pdb" shape
// this operator produces for per-rack PDBs.
func isRackPDBName(name, clusterName string) bool {
	rest, ok := strings.CutPrefix(name, clusterName+"-")
	if !ok {
		return false
	}
	idStr, ok := strings.CutSuffix(rest, "-pdb")
	if !ok {
		return false
	}
	_, err := strconv.Atoi(idStr)
	return err == nil
}
