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

// pdbPolicy is the disruption budget the operator wants for one selector.
//
// Only MaxUnavailable is used. The knob users write is spec.maxUnavailable /
// rack.maxUnavailable, and the default below is naturally expressed the same way,
// so there is no reason to emit the other shape.
type pdbPolicy struct {
	MaxUnavailable *intstr.IntOrString
}

// defaultReplicationFactor is Aerospike's own default when a namespace does not
// state one.
const defaultReplicationFactor int32 = 2

// minReplicationFactor returns the smallest replication-factor across the config's
// namespaces, which is the binding constraint: losing N nodes costs availability
// as soon as N reaches the LEAST-replicated namespace's factor.
//
// Every integer type a decoder can plausibly produce is accepted, mirroring the
// webhook's own parsing. Anything unparseable falls back to Aerospike's default
// rather than guessing, because guessing high would hand out a larger disruption
// budget than the data can survive.
func minReplicationFactor(config *ackov1alpha1.AerospikeConfigSpec) int32 {
	if config == nil || config.Value == nil {
		return defaultReplicationFactor
	}
	namespaces, ok := config.Value["namespaces"].([]any)
	if !ok || len(namespaces) == 0 {
		return defaultReplicationFactor
	}

	minRF := int32(0)
	for _, ns := range namespaces {
		nsMap, ok := ns.(map[string]any)
		if !ok {
			continue
		}
		rf := defaultReplicationFactor
		switch v := nsMap["replication-factor"].(type) {
		case int:
			rf = int32(v)
		case int32:
			rf = v
		case int64:
			rf = int32(v)
		case float64:
			rf = int32(v)
		}
		if rf < 1 {
			rf = defaultReplicationFactor
		}
		if minRF == 0 || rf < minRF {
			minRF = rf
		}
	}
	if minRF == 0 {
		return defaultReplicationFactor
	}
	return minRF
}

// defaultMaxUnavailable returns the disruption budget for a rack that sets none.
//
// It is replication-factor - 1: the number of nodes Aerospike can lose at once
// without a partition becoming unavailable. That is the real constraint in CE.
//
// The obvious-looking alternative — a Raft-style majority, minAvailable =
// rackSize/2 + 1, borrowed from the Percona/Redis operators — does not map onto
// Aerospike CE, which has no quorum (strong consistency is Enterprise-only), and
// it deadlocks in practice. getRackSize divides spec.size across racks, so a
// large cluster still has SMALL racks: with CE's 8-node cap a 3-rack cluster tops
// out at 3/3/2, and the rack of 2 gets minAvailable 2 — zero voluntary
// disruption. Enumerated over every CE-legal layout, that rule blocks 12 of 21,
// including EVERY 3-rack layout at every size. Three racks, one per zone, is the
// canonical rack-aware topology and precisely the configuration per-rack PDBs
// exist to serve, so the default would hang kubectl drain, cluster-autoscaler
// node recycling and managed node-pool upgrades on the clusters most likely to
// adopt this feature.
//
// The floor of 1 is deliberate. replication-factor 1 means every partition has a
// single copy, so the honest budget is 0 — but a default of 0 is the same
// deadlock, on a configuration CE users legitimately run for dev. Returning 1
// keeps today's behaviour there; the webhook warns separately that a
// single-copy cluster has no redundancy for a PDB to protect.
func (r *AerospikeClusterReconciler) defaultMaxUnavailable(
	cluster *ackov1alpha1.AerospikeCluster,
	rack *ackov1alpha1.Rack,
) int32 {
	// The rack's EFFECTIVE config: a rack may override replication-factor, and
	// the budget has to describe the namespaces that rack actually runs.
	config := cluster.Spec.AerospikeConfig
	if rack != nil {
		config = r.getEffectiveConfig(cluster, rack)
	}
	if mu := minReplicationFactor(config) - 1; mu > 1 {
		return mu
	}
	return 1
}

// effectivePDBPolicy resolves the budget for one rack.
//
// Precedence: rack.maxUnavailable > spec.maxUnavailable > the
// replication-factor default.
//
// An explicit value is clamped so the budget can never permit every pod in the
// rack to be evicted at once — a PDB that allows full disruption is not a budget,
// and the webhook check for this was warning-only, which "appears once in
// kubectl apply output and is not a control".
//
// The clamp is skipped for a rack of one pod, where the concept degenerates:
// there is no value that both permits maintenance and prevents full disruption,
// and clamping to 0 would block drains on the single-pod `minimal` template that
// works today. An operator who explicitly asks for maxUnavailable >= size is
// still rejected at admission; this only governs the derived default.
func (r *AerospikeClusterReconciler) effectivePDBPolicy(
	cluster *ackov1alpha1.AerospikeCluster,
	rack *ackov1alpha1.Rack,
	rackSize int32,
) pdbPolicy {
	configured := cluster.Spec.MaxUnavailable
	if rack != nil && rack.MaxUnavailable != nil {
		configured = rack.MaxUnavailable
	}

	if configured == nil {
		mu := intstr.FromInt32(r.defaultMaxUnavailable(cluster, rack))
		return pdbPolicy{MaxUnavailable: &mu}
	}

	// Percentages are handed to Kubernetes untouched: it resolves them against
	// the live pod count, which is the semantics a user writing "50%" wants, and
	// the webhook rejects >= 100%.
	if configured.Type != intstr.Int {
		value := *configured
		return pdbPolicy{MaxUnavailable: &value}
	}

	capped := configured.IntVal
	if rackSize > 1 && capped > rackSize-1 {
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
		return r.deleteAllPDBs(ctx, cluster)
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
			r.effectivePDBPolicy(cluster, &racks[0], rackSize))
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
			r.effectivePDBPolicy(cluster, rack, rackSize)); err != nil {
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

	// Ownership check before ANY write. PDB names can collide across clusters in
	// one namespace: RackPDBName("demo", 1) and PDBName("demo-1") both produce
	// "demo-1-pdb". Without this, cluster "demo" would silently rewrite cluster
	// "demo-1"'s PodDisruptionBudget — pointing the selector at demo's pods, so
	// demo-1 loses all disruption protection; overwriting the cluster label via
	// maps.Copy, so demo-1 cannot even find its own PDB by label to repair it;
	// and leaving the victim's ownerRef in place (setOwnerRef runs only on the
	// create path), so deleting demo-1 garbage-collects a PDB demo believes it
	// owns.
	//
	// Skipping rather than erroring is deliberate. Returning an error here would
	// fail this cluster's reconcile on every pass and trip the circuit breaker,
	// stopping rolling restarts and scaling over a PDB. Skipping costs this rack
	// its disruption budget — which the event says plainly — while the cluster
	// stays managed. Renaming one of the two clusters is the only real fix and
	// that is a human decision.
	if owner := existing.Labels[utils.InstanceLabel]; owner != "" && owner != cluster.Name {
		log.Error(nil, "PodDisruptionBudget name is already taken by another cluster; skipping",
			"pdb", pdbName, "ownedBy", owner, "cluster", cluster.Name)
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventPDBNameConflict,
			"PodDisruptionBudget %s already belongs to cluster %q; this cluster's pods are NOT "+
				"disruption-protected by it. Rename one of the two clusters to resolve.",
			pdbName, owner)
		return nil
	}

	if !pdbNeedsUpdate(existing, labels, selectorLabels, policy) {
		return nil
	}

	if existing.Labels == nil {
		existing.Labels = make(map[string]string, len(labels))
	}
	maps.Copy(existing.Labels, labels)
	// Clear any MinAvailable an earlier operator (or a hand edit) left behind:
	// the two fields are mutually exclusive in a PodDisruptionBudgetSpec.
	existing.Spec.MinAvailable = nil
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
	if existing.Spec.MinAvailable != nil {
		return true
	}
	if !intOrStringPtrEqual(existing.Spec.MaxUnavailable, desired.MaxUnavailable) {
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
