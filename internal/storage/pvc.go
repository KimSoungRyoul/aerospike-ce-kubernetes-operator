package storage

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	v1alpha1 "github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/api/v1alpha1"
	"github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/internal/utils"
)

// GetPVCsForStatefulSet lists PVCs belonging to the given StatefulSet of the
// named cluster.
//
// The primary filter matches both app.kubernetes.io/name and
// app.kubernetes.io/instance (the cluster name). Scoping by instance prevents a
// PVC from one cluster being attributed to a sibling cluster in the same
// namespace when one cluster name is a prefix of the other (e.g. "foo" vs
// "foo-0"), where the "-<stsName>-" substring check alone can collide.
// Operator-created PVCs carry the instance label because PVC templates get
// LabelsForCluster applied in reconciler_statefulset.go.
//
// If the label-scoped query returns nothing, it falls back to listing all PVCs
// in the namespace so legacy/pre-label PVCs are still found; in that path the
// name-substring check is the only ownership signal.
func GetPVCsForStatefulSet(ctx context.Context, c client.Client, namespace, clusterName, stsName string) ([]corev1.PersistentVolumeClaim, error) {
	pvcList := &corev1.PersistentVolumeClaimList{}
	if err := c.List(ctx, pvcList,
		client.InNamespace(namespace),
		client.MatchingLabels{
			"app.kubernetes.io/name":     "aerospike-cluster",
			"app.kubernetes.io/instance": clusterName,
		},
	); err != nil {
		return nil, fmt.Errorf("listing PVCs in namespace %s: %w", namespace, err)
	}

	// Fallback: if label-based query returned no results, re-list without labels
	// to find PVCs created before labels were added to VolumeClaimTemplates.
	if len(pvcList.Items) == 0 {
		if err := c.List(ctx, pvcList, client.InNamespace(namespace)); err != nil {
			return nil, fmt.Errorf("listing all PVCs in namespace %s: %w", namespace, err)
		}
	}

	var matched []corev1.PersistentVolumeClaim
	for i := range pvcList.Items {
		pvc := &pvcList.Items[i]
		// StatefulSet PVC names follow the pattern: <volumeName>-<stsName>-<ordinal>
		if isOwnedByStatefulSet(pvc, stsName) {
			matched = append(matched, *pvc)
		}
	}

	return matched, nil
}

// cascadeDeleteVolumeNames returns the set of PV-backed volume names that have
// cascadeDelete enabled. Volumes without a PersistentVolume source have no PVC
// to reclaim, and cascadeDelete defaults to false, so the set is empty for any
// cluster that has not opted in.
func cascadeDeleteVolumeNames(storageSpec *v1alpha1.AerospikeStorageSpec) map[string]bool {
	if storageSpec == nil {
		return nil
	}
	cascadeVolumes := make(map[string]bool)
	for i := range storageSpec.Volumes {
		vol := &storageSpec.Volumes[i]
		if vol.Source.PersistentVolume != nil && ResolveCascadeDelete(vol, storageSpec) {
			cascadeVolumes[vol.Name] = true
		}
	}
	return cascadeVolumes
}

// HasCascadeDeletePVCs reports whether the spec declares any PV-backed volume
// with cascadeDelete enabled — i.e. whether there is anything for
// DeleteOrphanedCascadeDeletePVCs to reclaim at all.
//
// Callers use it as a cheap pre-gate so the steady-state reconcile path skips
// the PVC List entirely for clusters that never opt into cascadeDelete. It
// shares cascadeDeleteVolumeNames with the deleter deliberately: a gate that
// could ever be looser than the deleter's own predicate would let a caller
// believe it had reclaimed PVCs it never looked at.
func HasCascadeDeletePVCs(storageSpec *v1alpha1.AerospikeStorageSpec) bool {
	return len(cascadeDeleteVolumeNames(storageSpec)) > 0
}

// ownedByStatefulSetUID reports whether pvc may be attributed to the given
// StatefulSet UID.
//
// A PVC with NO ownerReferences passes, because that is the normal shape rather
// than an anomaly: the StatefulSet controller only stamps an ownerReference onto
// a volumeClaimTemplate PVC when persistentVolumeClaimRetentionPolicy is set to
// Delete, and this operator never sets that field — it defaults to
// Retain/Retain, documented upstream as "retained until manually deleted".
// Requiring an ownerReference would reclaim nothing at all on a default install.
//
// What this check does buy is rejecting a PVC that names some *other* owner: a
// sibling StatefulSet, a foreign cluster, or a hand-created claim that happens
// to collide with the ordinal naming pattern.
func ownedByStatefulSetUID(pvc *corev1.PersistentVolumeClaim, stsUID types.UID) bool {
	refs := pvc.GetOwnerReferences()
	if len(refs) == 0 {
		return true
	}
	for i := range refs {
		if refs[i].UID == stsUID {
			return true
		}
	}
	return false
}

// ownerRefSummary renders a PVC's ownerReferences compactly for a log field.
func ownerRefSummary(pvc *corev1.PersistentVolumeClaim) string {
	refs := pvc.GetOwnerReferences()
	parts := make([]string, 0, len(refs))
	for i := range refs {
		parts = append(parts, fmt.Sprintf("%s/%s(%s)", refs[i].Kind, refs[i].Name, refs[i].UID))
	}
	return strings.Join(parts, ",")
}

// ownedOrphanCandidates returns the PVCs that belong to sts and sit at an
// ordinal at or above desiredReplicas.
//
// Ownership is established against the StatefulSet object the caller just read,
// and every available signal must agree:
//
//   - The operator's cluster labels must be present. Unlike
//     GetPVCsForStatefulSet this deliberately does NOT fall back to an
//     unfiltered namespace-wide List. That fallback exists so cluster
//     *deletion* can still find legacy pre-label PVCs, and in that path a name
//     substring is the only ownership signal. Reclamation runs on every
//     reconcile of every rack, so it must not inherit a name-only test — an
//     unlabelled foreign claim called data-<sts>-9 would be selected.
//   - The PVC name must decompose as <volume>-<stsName>-<ordinal> where
//     <volume> is one of the StatefulSet's own VolumeClaimTemplate names, taken
//     from the live object rather than from the spec.
//   - Any ownerReferences present must include this StatefulSet's UID.
//
// desiredReplicas must be the replica count the StatefulSet is actually running
// (its own spec.replicas), never a lower target the operator is still working
// towards: ordinals between the two still have live pods holding their volumes.
func ownedOrphanCandidates(
	ctx context.Context,
	c client.Client,
	sts *appsv1.StatefulSet,
	clusterName string,
	desiredReplicas int32,
) ([]corev1.PersistentVolumeClaim, error) {
	vctNames := make(map[string]bool, len(sts.Spec.VolumeClaimTemplates))
	for i := range sts.Spec.VolumeClaimTemplates {
		vctNames[sts.Spec.VolumeClaimTemplates[i].Name] = true
	}
	if len(vctNames) == 0 {
		return nil, nil
	}

	// All four labels LabelsForCluster stamps, not just the two the legacy
	// getter checks. component and managed-by cost nothing to match and narrow
	// the surface against hand-created or restored claims that happen to carry
	// the name/instance pair.
	pvcList := &corev1.PersistentVolumeClaimList{}
	if err := c.List(ctx, pvcList,
		client.InNamespace(sts.Namespace),
		client.MatchingLabels(utils.LabelsForCluster(clusterName)),
	); err != nil {
		return nil, fmt.Errorf("listing PVCs in namespace %s: %w", sts.Namespace, err)
	}

	var matched []corev1.PersistentVolumeClaim
	for i := range pvcList.Items {
		pvc := &pvcList.Items[i]

		ordinal, ok := extractOrdinal(pvc.Name, sts.Name)
		if !ok || ordinal < desiredReplicas {
			continue
		}
		volName, ok := extractVolumeName(pvc.Name, sts.Name)
		if !ok || !vctNames[volName] {
			continue
		}
		if !ownedByStatefulSetUID(pvc, sts.UID) {
			// Log rather than drop silently. Under
			// persistentVolumeClaimRetentionPolicy.whenScaled=Delete the
			// StatefulSet controller stamps the *Pod* as owner, which reads as
			// foreign here — correct (kube-controller-manager reclaims those
			// itself, so this path has nothing to do) but worth seeing, because
			// otherwise reclamation appearing to do nothing is indistinguishable
			// from reclamation being broken.
			logf.FromContext(ctx).V(1).Info(
				"Skipping PVC: ownerReferences name a different owner",
				"pvc", pvc.Name, "statefulset", sts.Name, "owners", ownerRefSummary(pvc))
			continue
		}
		matched = append(matched, *pvc)
	}

	return matched, nil
}

// DeleteOrphanedCascadeDeletePVCs removes PVCs for pod ordinals >= desiredReplicas,
// but only for volumes that have cascadeDelete enabled. Non-cascade PVCs are preserved.
// This is the correct function to use during scale-down.
//
// sts must be the live StatefulSet object; it supplies the namespace, the name
// prefix, the VolumeClaimTemplate names and the UID that together establish
// ownership. See ownedOrphanCandidates for the full predicate.
func DeleteOrphanedCascadeDeletePVCs(
	ctx context.Context,
	c client.Client,
	sts *appsv1.StatefulSet,
	clusterName string,
	desiredReplicas int32,
	storageSpec *v1alpha1.AerospikeStorageSpec,
) (int, error) {
	cascadeVolumes := cascadeDeleteVolumeNames(storageSpec)
	if len(cascadeVolumes) == 0 || sts == nil {
		return 0, nil
	}

	pvcs, err := ownedOrphanCandidates(ctx, c, sts, clusterName, desiredReplicas)
	if err != nil {
		return 0, err
	}

	deleted := 0
	var deleteErrs []error
	for i := range pvcs {
		pvc := &pvcs[i]

		volName, ok := extractVolumeName(pvc.Name, sts.Name)
		if !ok || !cascadeVolumes[volName] {
			continue
		}

		if err := c.Delete(ctx, pvc); err != nil {
			if kerrors.IsNotFound(err) {
				continue
			}
			deleteErrs = append(deleteErrs, fmt.Errorf("deleting orphaned cascade PVC %s: %w", pvc.Name, err))
			continue
		}
		deleted++
	}

	if len(deleteErrs) > 0 {
		return deleted, errors.Join(deleteErrs...)
	}
	return deleted, nil
}

// DeletePVCsForStatefulSet deletes all PVCs associated with the given StatefulSet.
// Used when the cluster CR is deleted with cascadeDelete.
func DeletePVCsForStatefulSet(ctx context.Context, c client.Client, namespace, clusterName, stsName string) error {
	pvcs, err := GetPVCsForStatefulSet(ctx, c, namespace, clusterName, stsName)
	if err != nil {
		return err
	}

	for i := range pvcs {
		if err := c.Delete(ctx, &pvcs[i]); err != nil {
			if kerrors.IsNotFound(err) {
				continue // already deleted (concurrent deletion)
			}
			return fmt.Errorf("deleting PVC %s: %w", pvcs[i].Name, err)
		}
	}

	return nil
}

// isOwnedByStatefulSet checks if a PVC name contains the StatefulSet name as
// part of the standard naming convention: <volumeName>-<stsName>-<ordinal>.
func isOwnedByStatefulSet(pvc *corev1.PersistentVolumeClaim, stsName string) bool {
	_, ok := extractOrdinal(pvc.Name, stsName)
	return ok
}

// extractOrdinal parses the ordinal index from a PVC name that follows the
// StatefulSet naming pattern: <volumeName>-<stsName>-<ordinal>.
func extractOrdinal(pvcName, stsName string) (int32, bool) {
	// PVC names follow: <volumeClaimName>-<stsName>-<ordinal>
	// We search for "-<stsName>-" and then parse the trailing ordinal.
	pattern := "-" + stsName + "-"
	idx := len(pvcName) - 1

	// Find the last digit sequence (the ordinal).
	for idx >= 0 && pvcName[idx] >= '0' && pvcName[idx] <= '9' {
		idx--
	}

	if idx < 0 || idx == len(pvcName)-1 {
		return 0, false
	}

	// Check that the text before the ordinal ends with "-<stsName>-"
	prefix := pvcName[:idx+1]
	if len(prefix) < len(pattern) {
		return 0, false
	}

	if prefix[len(prefix)-len(pattern):] != pattern {
		return 0, false
	}

	// Parse ordinal using strconv for proper overflow/error handling.
	ordinal, err := strconv.ParseInt(pvcName[idx+1:], 10, 32)
	if err != nil {
		return 0, false
	}

	return int32(ordinal), true
}

// extractVolumeName extracts the volume claim template name from a PVC name
// that follows the StatefulSet naming pattern: <volumeName>-<stsName>-<ordinal>.
func extractVolumeName(pvcName, stsName string) (string, bool) {
	pattern := "-" + stsName + "-"
	idx := strings.LastIndex(pvcName, pattern)
	if idx <= 0 {
		return "", false
	}

	// Verify the suffix after the pattern is a valid ordinal
	suffix := pvcName[idx+len(pattern):]
	if suffix == "" {
		return "", false
	}
	if _, err := strconv.ParseInt(suffix, 10, 32); err != nil {
		return "", false
	}

	return pvcName[:idx], true
}

// DeleteCascadeDeletePVCs deletes only PVCs for volumes that have cascadeDelete=true.
// This ensures non-cascade volumes are preserved when the CR is deleted.
func DeleteCascadeDeletePVCs(
	ctx context.Context,
	c client.Client,
	namespace, clusterName, stsName string,
	storageSpec *v1alpha1.AerospikeStorageSpec,
) error {
	cascadeVolumes := cascadeDeleteVolumeNames(storageSpec)
	if len(cascadeVolumes) == 0 {
		return nil
	}

	pvcs, err := GetPVCsForStatefulSet(ctx, c, namespace, clusterName, stsName)
	if err != nil {
		return err
	}

	for i := range pvcs {
		pvc := &pvcs[i]
		volName, ok := extractVolumeName(pvc.Name, stsName)
		if !ok {
			continue
		}

		if !cascadeVolumes[volName] {
			continue
		}

		if err := c.Delete(ctx, pvc); err != nil {
			if kerrors.IsNotFound(err) {
				continue
			}
			return fmt.Errorf("deleting cascade PVC %s: %w", pvc.Name, err)
		}
	}

	return nil
}
