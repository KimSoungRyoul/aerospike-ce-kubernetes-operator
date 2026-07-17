package storage

import (
	corev1 "k8s.io/api/core/v1"

	v1alpha1 "github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/api/v1alpha1"
)

// ResolveInitMethod returns the effective init method for a volume.
// Precedence: per-volume (any non-empty value including "none") > global policy > "none".
func ResolveInitMethod(vol *v1alpha1.VolumeSpec, storageSpec *v1alpha1.AerospikeStorageSpec) v1alpha1.VolumeInitMethod {
	// Per-volume override: any explicit value (including "none") takes precedence
	if vol.InitMethod != "" {
		return vol.InitMethod
	}

	// Global policy fallback
	if policy := getVolumePolicy(vol, storageSpec); policy != nil {
		if policy.InitMethod != "" {
			return policy.InitMethod
		}
	}

	return v1alpha1.VolumeInitMethodNone
}

// ResolveWipeMethod returns the effective wipe method for a volume.
// Precedence: per-volume (any non-empty value including "none") > global policy > "none".
func ResolveWipeMethod(vol *v1alpha1.VolumeSpec, storageSpec *v1alpha1.AerospikeStorageSpec) v1alpha1.VolumeWipeMethod {
	// Per-volume override: any explicit value (including "none") takes precedence
	if vol.WipeMethod != "" {
		return vol.WipeMethod
	}

	// Global policy fallback
	if policy := getVolumePolicy(vol, storageSpec); policy != nil {
		if policy.WipeMethod != "" {
			return policy.WipeMethod
		}
	}

	return v1alpha1.VolumeWipeMethodNone
}

// ResolveCascadeDelete returns the effective cascade delete setting for a volume.
// Precedence: per-volume (explicit true/false) > global policy > false.
func ResolveCascadeDelete(vol *v1alpha1.VolumeSpec, storageSpec *v1alpha1.AerospikeStorageSpec) bool {
	// Per-volume override (nil means "not set", defer to policy)
	if vol.CascadeDelete != nil {
		return *vol.CascadeDelete
	}

	// Global policy fallback
	if policy := getVolumePolicy(vol, storageSpec); policy != nil {
		if policy.CascadeDelete != nil {
			return *policy.CascadeDelete
		}
	}

	return false
}

// getVolumePolicy returns the applicable global volume policy for the given volume.
// Returns nil for non-persistent volumes or when no policy is defined.
func getVolumePolicy(vol *v1alpha1.VolumeSpec, storageSpec *v1alpha1.AerospikeStorageSpec) *v1alpha1.AerospikeVolumePolicy {
	if storageSpec == nil || vol.Source.PersistentVolume == nil {
		return nil
	}

	pv := vol.Source.PersistentVolume

	volumeMode := pv.VolumeMode
	if volumeMode == "" {
		volumeMode = corev1.PersistentVolumeFilesystem
	}

	if volumeMode == corev1.PersistentVolumeBlock {
		return storageSpec.BlockVolumePolicy
	}

	return storageSpec.FilesystemVolumePolicy
}
