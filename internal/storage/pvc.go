package storage

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/api/v1alpha1"
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

// DeleteOrphanedCascadeDeletePVCs removes PVCs for pod ordinals >= desiredReplicas,
// but only for volumes that have cascadeDelete enabled. Non-cascade PVCs are preserved.
// This is the correct function to use during scale-down.
func DeleteOrphanedCascadeDeletePVCs(
	ctx context.Context,
	c client.Client,
	namespace, clusterName, stsName string,
	desiredReplicas int32,
	storageSpec *v1alpha1.AerospikeStorageSpec,
) (int, error) {
	if storageSpec == nil {
		return 0, nil
	}

	// Build a set of volume names that have cascadeDelete enabled
	cascadeVolumes := make(map[string]bool)
	for i := range storageSpec.Volumes {
		vol := &storageSpec.Volumes[i]
		if vol.Source.PersistentVolume != nil && ResolveCascadeDelete(vol, storageSpec) {
			cascadeVolumes[vol.Name] = true
		}
	}

	if len(cascadeVolumes) == 0 {
		return 0, nil
	}

	pvcs, err := GetPVCsForStatefulSet(ctx, c, namespace, clusterName, stsName)
	if err != nil {
		return 0, err
	}

	deleted := 0
	var deleteErrs []error
	for i := range pvcs {
		pvc := &pvcs[i]
		ordinal, ok := extractOrdinal(pvc.Name, stsName)
		if !ok {
			continue
		}

		if ordinal < desiredReplicas {
			continue
		}

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
	if storageSpec == nil {
		return nil
	}

	// Build a set of volume names that have cascadeDelete enabled
	cascadeVolumes := make(map[string]bool)
	for i := range storageSpec.Volumes {
		vol := &storageSpec.Volumes[i]
		if vol.Source.PersistentVolume != nil && ResolveCascadeDelete(vol, storageSpec) {
			cascadeVolumes[vol.Name] = true
		}
	}

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
