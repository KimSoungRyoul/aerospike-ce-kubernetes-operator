package storage

import (
	"context"
	"fmt"
	"slices"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/ksr/aerospike-ce-kubernetes-operator/api/v1alpha1"
)

// IsLocalStorageClass returns true if the given storage class name is in the list of local storage classes.
func IsLocalStorageClass(storageClassName string, localClasses []string) bool {
	return slices.Contains(localClasses, storageClassName)
}

// GetLocalPVCsForPod returns PVCs for the given pod that use local storage classes.
func GetLocalPVCsForPod(
	ctx context.Context,
	c client.Client,
	namespace string,
	clusterName string,
	stsName string,
	ordinal int32,
	storageSpec *v1alpha1.AerospikeStorageSpec,
) ([]corev1.PersistentVolumeClaim, error) {
	if storageSpec == nil || len(storageSpec.LocalStorageClasses) == 0 {
		return nil, nil
	}

	// Get all PVCs for this StatefulSet
	allPVCs, err := GetPVCsForStatefulSet(ctx, c, namespace, clusterName, stsName)
	if err != nil {
		return nil, err
	}

	// Filter PVCs that belong to this pod ordinal and use local storage
	var localPVCs []corev1.PersistentVolumeClaim
	for i := range allPVCs {
		pvc := &allPVCs[i]
		pvcOrdinal, ok := extractOrdinal(pvc.Name, stsName)
		if !ok || pvcOrdinal != ordinal {
			continue
		}

		// Check if the PVC's storage class is a local storage class
		scName := ""
		if pvc.Spec.StorageClassName != nil {
			scName = *pvc.Spec.StorageClassName
		}
		if IsLocalStorageClass(scName, storageSpec.LocalStorageClasses) {
			localPVCs = append(localPVCs, *pvc)
		}
	}

	return localPVCs, nil
}

// DeleteLocalPVCsForPod deletes PVCs backed by local storage for the given pod.
// This is called before pod deletion during cold restart when deleteLocalStorageOnRestart is true.
func DeleteLocalPVCsForPod(
	ctx context.Context,
	c client.Client,
	namespace string,
	clusterName string,
	stsName string,
	ordinal int32,
	storageSpec *v1alpha1.AerospikeStorageSpec,
) error {
	localPVCs, err := GetLocalPVCsForPod(ctx, c, namespace, clusterName, stsName, ordinal, storageSpec)
	if err != nil {
		return fmt.Errorf("getting local PVCs for pod %s-%d: %w", stsName, ordinal, err)
	}

	for i := range localPVCs {
		if err := c.Delete(ctx, &localPVCs[i]); err != nil {
			if errors.IsNotFound(err) {
				continue
			}
			return fmt.Errorf("deleting local PVC %s: %w", localPVCs[i].Name, err)
		}
	}

	return nil
}

// ParsePodName extracts the StatefulSet name and ordinal index from a pod name.
// StatefulSet pod names follow the pattern: <stsName>-<ordinal>
func ParsePodName(podName string) (stsName string, ordinal int32, ok bool) {
	// Find the last dash followed by digits
	lastDash := -1
	for i := len(podName) - 1; i >= 0; i-- {
		if podName[i] == '-' {
			lastDash = i
			break
		}
		if podName[i] < '0' || podName[i] > '9' {
			return "", 0, false
		}
	}
	if lastDash < 0 || lastDash == len(podName)-1 {
		return "", 0, false
	}

	stsName = podName[:lastDash]
	parsed, err := strconv.ParseInt(podName[lastDash+1:], 10, 32)
	if err != nil {
		return "", 0, false
	}
	return stsName, int32(parsed), true
}
