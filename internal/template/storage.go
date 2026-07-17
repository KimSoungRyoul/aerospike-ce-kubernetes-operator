/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package template

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ackov1alpha1 "github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/api/v1alpha1"
)

const (
	// defaultDataVolumeName is the name used for the template-derived data volume.
	defaultDataVolumeName = "data"
	// defaultDataMountPath is the default mount path for the data volume.
	defaultDataMountPath = "/opt/aerospike/data"
)

// applyStorage merges template storage defaults into the cluster's storage spec.
// If the cluster already has storage volumes configured, the template storage is not applied.
func applyStorage(tmplStorage *ackov1alpha1.TemplateStorage, cluster *ackov1alpha1.AerospikeCluster) {
	if tmplStorage == nil {
		return
	}

	// Only apply template storage if cluster has no volumes configured.
	if cluster.Spec.Storage != nil && len(cluster.Spec.Storage.Volumes) > 0 {
		return
	}

	// Determine storage size from template resources.
	size := "1Gi" // fallback default
	if tmplStorage.Resources.Requests != nil {
		if qty, ok := tmplStorage.Resources.Requests[corev1.ResourceStorage]; ok {
			size = qty.String()
		}
	}

	// Build a PVC-backed volume from template storage settings using the operator's PersistentVolumeSpec.
	pvSpec := &ackov1alpha1.PersistentVolumeSpec{
		StorageClass: tmplStorage.StorageClassName,
		VolumeMode:   tmplStorage.VolumeMode,
		Size:         size,
		AccessModes:  tmplStorage.AccessModes,
	}

	if pvSpec.VolumeMode == "" {
		pvSpec.VolumeMode = corev1.PersistentVolumeFilesystem
	}
	if len(pvSpec.AccessModes) == 0 {
		pvSpec.AccessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}
	}

	vol := ackov1alpha1.VolumeSpec{
		Name: defaultDataVolumeName,
		Source: ackov1alpha1.VolumeSource{
			PersistentVolume: pvSpec,
		},
		Aerospike: &ackov1alpha1.AerospikeVolumeAttachment{
			Path: defaultDataMountPath,
		},
	}

	if cluster.Spec.Storage == nil {
		cluster.Spec.Storage = &ackov1alpha1.AerospikeStorageSpec{}
	}
	cluster.Spec.Storage.Volumes = append(cluster.Spec.Storage.Volumes, vol)
}

// ValidateLocalPV checks that a StorageClass has WaitForFirstConsumer binding mode,
// which is required for proper local PV scheduling.
func ValidateLocalPV(ctx context.Context, reader client.Reader, storageClassName string) error {
	if storageClassName == "" {
		return fmt.Errorf("storageClassName must be specified when localPVRequired is true")
	}

	sc := &storagev1.StorageClass{}
	if err := reader.Get(ctx, types.NamespacedName{Name: storageClassName}, sc); err != nil {
		return fmt.Errorf("getting StorageClass %q: %w", storageClassName, err)
	}

	if sc.VolumeBindingMode == nil || *sc.VolumeBindingMode != storagev1.VolumeBindingWaitForFirstConsumer {
		return fmt.Errorf(
			"StorageClass %q must have volumeBindingMode: WaitForFirstConsumer for localPVRequired=true (got %v)",
			storageClassName, sc.VolumeBindingMode,
		)
	}

	return nil
}
