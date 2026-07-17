package storage

import (
	"testing"

	corev1 "k8s.io/api/core/v1"

	v1alpha1 "github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/api/v1alpha1"
)

func boolPtr(b bool) *bool { return &b }

// --- ResolveInitMethod tests ---

func TestResolveInitMethod_PerVolumeOverridesPolicy(t *testing.T) {
	vol := &v1alpha1.VolumeSpec{
		InitMethod: v1alpha1.VolumeInitMethodDD,
		Source: v1alpha1.VolumeSource{
			PersistentVolume: &v1alpha1.PersistentVolumeSpec{Size: "10Gi"},
		},
	}
	spec := &v1alpha1.AerospikeStorageSpec{
		FilesystemVolumePolicy: &v1alpha1.AerospikeVolumePolicy{
			InitMethod: v1alpha1.VolumeInitMethodDeleteFiles,
		},
	}

	result := ResolveInitMethod(vol, spec)
	if result != v1alpha1.VolumeInitMethodDD {
		t.Errorf("expected %q, got %q", v1alpha1.VolumeInitMethodDD, result)
	}
}

func TestResolveInitMethod_FilesystemPolicyFallback(t *testing.T) {
	vol := &v1alpha1.VolumeSpec{
		Source: v1alpha1.VolumeSource{
			PersistentVolume: &v1alpha1.PersistentVolumeSpec{
				Size:       "10Gi",
				VolumeMode: corev1.PersistentVolumeFilesystem,
			},
		},
	}
	spec := &v1alpha1.AerospikeStorageSpec{
		FilesystemVolumePolicy: &v1alpha1.AerospikeVolumePolicy{
			InitMethod: v1alpha1.VolumeInitMethodDeleteFiles,
		},
	}

	result := ResolveInitMethod(vol, spec)
	if result != v1alpha1.VolumeInitMethodDeleteFiles {
		t.Errorf("expected %q, got %q", v1alpha1.VolumeInitMethodDeleteFiles, result)
	}
}

func TestResolveInitMethod_BlockPolicyFallback(t *testing.T) {
	vol := &v1alpha1.VolumeSpec{
		Source: v1alpha1.VolumeSource{
			PersistentVolume: &v1alpha1.PersistentVolumeSpec{
				Size:       "10Gi",
				VolumeMode: corev1.PersistentVolumeBlock,
			},
		},
	}
	spec := &v1alpha1.AerospikeStorageSpec{
		BlockVolumePolicy: &v1alpha1.AerospikeVolumePolicy{
			InitMethod: v1alpha1.VolumeInitMethodBlkdiscard,
		},
		FilesystemVolumePolicy: &v1alpha1.AerospikeVolumePolicy{
			InitMethod: v1alpha1.VolumeInitMethodDeleteFiles,
		},
	}

	result := ResolveInitMethod(vol, spec)
	if result != v1alpha1.VolumeInitMethodBlkdiscard {
		t.Errorf("expected %q, got %q", v1alpha1.VolumeInitMethodBlkdiscard, result)
	}
}

func TestResolveInitMethod_NoPolicyDefaultNone(t *testing.T) {
	vol := &v1alpha1.VolumeSpec{
		Source: v1alpha1.VolumeSource{
			PersistentVolume: &v1alpha1.PersistentVolumeSpec{Size: "10Gi"},
		},
	}
	spec := &v1alpha1.AerospikeStorageSpec{}

	result := ResolveInitMethod(vol, spec)
	if result != v1alpha1.VolumeInitMethodNone {
		t.Errorf("expected %q, got %q", v1alpha1.VolumeInitMethodNone, result)
	}
}

func TestResolveInitMethod_DefaultVolumeModeIsFilesystem(t *testing.T) {
	vol := &v1alpha1.VolumeSpec{
		Source: v1alpha1.VolumeSource{
			PersistentVolume: &v1alpha1.PersistentVolumeSpec{
				Size: "10Gi",
				// VolumeMode empty → defaults to Filesystem
			},
		},
	}
	spec := &v1alpha1.AerospikeStorageSpec{
		FilesystemVolumePolicy: &v1alpha1.AerospikeVolumePolicy{
			InitMethod: v1alpha1.VolumeInitMethodHeaderCleanup,
		},
	}

	result := ResolveInitMethod(vol, spec)
	if result != v1alpha1.VolumeInitMethodHeaderCleanup {
		t.Errorf("expected %q, got %q", v1alpha1.VolumeInitMethodHeaderCleanup, result)
	}
}

// --- ResolveWipeMethod tests ---

func TestResolveWipeMethod_PerVolumeOverridesPolicy(t *testing.T) {
	vol := &v1alpha1.VolumeSpec{
		WipeMethod: v1alpha1.VolumeWipeMethodDD,
		Source: v1alpha1.VolumeSource{
			PersistentVolume: &v1alpha1.PersistentVolumeSpec{Size: "10Gi"},
		},
	}
	spec := &v1alpha1.AerospikeStorageSpec{
		FilesystemVolumePolicy: &v1alpha1.AerospikeVolumePolicy{
			WipeMethod: v1alpha1.VolumeWipeMethodDeleteFiles,
		},
	}

	result := ResolveWipeMethod(vol, spec)
	if result != v1alpha1.VolumeWipeMethodDD {
		t.Errorf("expected %q, got %q", v1alpha1.VolumeWipeMethodDD, result)
	}
}

func TestResolveWipeMethod_PolicyFallback(t *testing.T) {
	vol := &v1alpha1.VolumeSpec{
		Source: v1alpha1.VolumeSource{
			PersistentVolume: &v1alpha1.PersistentVolumeSpec{Size: "10Gi"},
		},
	}
	spec := &v1alpha1.AerospikeStorageSpec{
		FilesystemVolumePolicy: &v1alpha1.AerospikeVolumePolicy{
			WipeMethod: v1alpha1.VolumeWipeMethodBlkdiscardWithHeaderCleanup,
		},
	}

	result := ResolveWipeMethod(vol, spec)
	if result != v1alpha1.VolumeWipeMethodBlkdiscardWithHeaderCleanup {
		t.Errorf("expected %q, got %q", v1alpha1.VolumeWipeMethodBlkdiscardWithHeaderCleanup, result)
	}
}

func TestResolveWipeMethod_NoPolicyDefaultNone(t *testing.T) {
	vol := &v1alpha1.VolumeSpec{
		Source: v1alpha1.VolumeSource{
			PersistentVolume: &v1alpha1.PersistentVolumeSpec{Size: "10Gi"},
		},
	}
	spec := &v1alpha1.AerospikeStorageSpec{}

	result := ResolveWipeMethod(vol, spec)
	if result != v1alpha1.VolumeWipeMethodNone {
		t.Errorf("expected %q, got %q", v1alpha1.VolumeWipeMethodNone, result)
	}
}

// --- ResolveCascadeDelete tests ---

func TestResolveCascadeDelete_PerVolumeTrueOverridesPolicy(t *testing.T) {
	vol := &v1alpha1.VolumeSpec{
		CascadeDelete: boolPtr(true),
		Source: v1alpha1.VolumeSource{
			PersistentVolume: &v1alpha1.PersistentVolumeSpec{Size: "10Gi"},
		},
	}
	spec := &v1alpha1.AerospikeStorageSpec{
		FilesystemVolumePolicy: &v1alpha1.AerospikeVolumePolicy{
			CascadeDelete: boolPtr(false),
		},
	}

	if !ResolveCascadeDelete(vol, spec) {
		t.Error("expected true when per-volume cascadeDelete is true")
	}
}

func TestResolveCascadeDelete_PerVolumeFalseOverridesPolicy(t *testing.T) {
	vol := &v1alpha1.VolumeSpec{
		CascadeDelete: boolPtr(false),
		Source: v1alpha1.VolumeSource{
			PersistentVolume: &v1alpha1.PersistentVolumeSpec{Size: "10Gi"},
		},
	}
	spec := &v1alpha1.AerospikeStorageSpec{
		FilesystemVolumePolicy: &v1alpha1.AerospikeVolumePolicy{
			CascadeDelete: boolPtr(true),
		},
	}

	if ResolveCascadeDelete(vol, spec) {
		t.Error("expected false when per-volume cascadeDelete is explicitly false")
	}
}

func TestResolveCascadeDelete_PolicyFallback(t *testing.T) {
	vol := &v1alpha1.VolumeSpec{
		// CascadeDelete is nil → falls back to policy
		Source: v1alpha1.VolumeSource{
			PersistentVolume: &v1alpha1.PersistentVolumeSpec{Size: "10Gi"},
		},
	}
	spec := &v1alpha1.AerospikeStorageSpec{
		FilesystemVolumePolicy: &v1alpha1.AerospikeVolumePolicy{
			CascadeDelete: boolPtr(true),
		},
	}

	if !ResolveCascadeDelete(vol, spec) {
		t.Error("expected true when policy cascadeDelete is true")
	}
}

func TestResolveCascadeDelete_NoPolicyDefaultFalse(t *testing.T) {
	vol := &v1alpha1.VolumeSpec{
		Source: v1alpha1.VolumeSource{
			PersistentVolume: &v1alpha1.PersistentVolumeSpec{Size: "10Gi"},
		},
	}
	spec := &v1alpha1.AerospikeStorageSpec{}

	if ResolveCascadeDelete(vol, spec) {
		t.Error("expected false when no policy and no per-volume cascadeDelete")
	}
}

// --- ResolveInitMethod edge cases ---

func TestResolveInitMethod_ExplicitNoneOverridesPolicy(t *testing.T) {
	// When per-volume is explicitly "none", it should override the global policy
	vol := &v1alpha1.VolumeSpec{
		InitMethod: v1alpha1.VolumeInitMethodNone,
		Source: v1alpha1.VolumeSource{
			PersistentVolume: &v1alpha1.PersistentVolumeSpec{Size: "10Gi"},
		},
	}
	spec := &v1alpha1.AerospikeStorageSpec{
		FilesystemVolumePolicy: &v1alpha1.AerospikeVolumePolicy{
			InitMethod: v1alpha1.VolumeInitMethodDeleteFiles,
		},
	}

	result := ResolveInitMethod(vol, spec)
	if result != v1alpha1.VolumeInitMethodNone {
		t.Errorf("explicit 'none' should override policy, expected %q, got %q", v1alpha1.VolumeInitMethodNone, result)
	}
}

func TestResolveInitMethod_UnsetFallsToPolicy(t *testing.T) {
	// When per-volume initMethod is empty (unset), it should fall back to global policy
	vol := &v1alpha1.VolumeSpec{
		// InitMethod is zero value ""
		Source: v1alpha1.VolumeSource{
			PersistentVolume: &v1alpha1.PersistentVolumeSpec{Size: "10Gi"},
		},
	}
	spec := &v1alpha1.AerospikeStorageSpec{
		FilesystemVolumePolicy: &v1alpha1.AerospikeVolumePolicy{
			InitMethod: v1alpha1.VolumeInitMethodDeleteFiles,
		},
	}

	result := ResolveInitMethod(vol, spec)
	if result != v1alpha1.VolumeInitMethodDeleteFiles {
		t.Errorf("unset initMethod should fall to policy, expected %q, got %q", v1alpha1.VolumeInitMethodDeleteFiles, result)
	}
}

func TestResolveInitMethod_EmptyStringFallsToPolicy(t *testing.T) {
	vol := &v1alpha1.VolumeSpec{
		// InitMethod is zero value ""
		Source: v1alpha1.VolumeSource{
			PersistentVolume: &v1alpha1.PersistentVolumeSpec{
				Size:       "10Gi",
				VolumeMode: corev1.PersistentVolumeBlock,
			},
		},
	}
	spec := &v1alpha1.AerospikeStorageSpec{
		BlockVolumePolicy: &v1alpha1.AerospikeVolumePolicy{
			InitMethod: v1alpha1.VolumeInitMethodBlkdiscard,
		},
	}

	result := ResolveInitMethod(vol, spec)
	if result != v1alpha1.VolumeInitMethodBlkdiscard {
		t.Errorf("expected block policy fallback %q, got %q", v1alpha1.VolumeInitMethodBlkdiscard, result)
	}
}

func TestResolveInitMethod_NonPersistentVolume_NoPolicy(t *testing.T) {
	vol := &v1alpha1.VolumeSpec{
		Source: v1alpha1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
	spec := &v1alpha1.AerospikeStorageSpec{
		FilesystemVolumePolicy: &v1alpha1.AerospikeVolumePolicy{
			InitMethod: v1alpha1.VolumeInitMethodDeleteFiles,
		},
	}

	result := ResolveInitMethod(vol, spec)
	if result != v1alpha1.VolumeInitMethodNone {
		t.Errorf("non-persistent volume should return none, got %q", result)
	}
}

func TestResolveInitMethod_NilStorageSpec(t *testing.T) {
	vol := &v1alpha1.VolumeSpec{
		Source: v1alpha1.VolumeSource{
			PersistentVolume: &v1alpha1.PersistentVolumeSpec{Size: "10Gi"},
		},
	}

	result := ResolveInitMethod(vol, nil)
	if result != v1alpha1.VolumeInitMethodNone {
		t.Errorf("nil storage spec should return none, got %q", result)
	}
}

func TestResolveInitMethod_PolicyAlsoNone(t *testing.T) {
	vol := &v1alpha1.VolumeSpec{
		Source: v1alpha1.VolumeSource{
			PersistentVolume: &v1alpha1.PersistentVolumeSpec{Size: "10Gi"},
		},
	}
	spec := &v1alpha1.AerospikeStorageSpec{
		FilesystemVolumePolicy: &v1alpha1.AerospikeVolumePolicy{
			InitMethod: v1alpha1.VolumeInitMethodNone,
		},
	}

	result := ResolveInitMethod(vol, spec)
	if result != v1alpha1.VolumeInitMethodNone {
		t.Errorf("policy none should return none, got %q", result)
	}
}

// --- ResolveWipeMethod edge cases ---

func TestResolveWipeMethod_BlockPolicyFallback(t *testing.T) {
	vol := &v1alpha1.VolumeSpec{
		Source: v1alpha1.VolumeSource{
			PersistentVolume: &v1alpha1.PersistentVolumeSpec{
				Size:       "10Gi",
				VolumeMode: corev1.PersistentVolumeBlock,
			},
		},
	}
	spec := &v1alpha1.AerospikeStorageSpec{
		BlockVolumePolicy: &v1alpha1.AerospikeVolumePolicy{
			WipeMethod: v1alpha1.VolumeWipeMethodBlkdiscard,
		},
		FilesystemVolumePolicy: &v1alpha1.AerospikeVolumePolicy{
			WipeMethod: v1alpha1.VolumeWipeMethodDeleteFiles,
		},
	}

	result := ResolveWipeMethod(vol, spec)
	if result != v1alpha1.VolumeWipeMethodBlkdiscard {
		t.Errorf("expected block policy wipe %q, got %q", v1alpha1.VolumeWipeMethodBlkdiscard, result)
	}
}

func TestResolveWipeMethod_ExplicitNoneOverridesPolicy(t *testing.T) {
	// When per-volume is explicitly "none", it should override the global policy
	vol := &v1alpha1.VolumeSpec{
		WipeMethod: v1alpha1.VolumeWipeMethodNone,
		Source: v1alpha1.VolumeSource{
			PersistentVolume: &v1alpha1.PersistentVolumeSpec{Size: "10Gi"},
		},
	}
	spec := &v1alpha1.AerospikeStorageSpec{
		FilesystemVolumePolicy: &v1alpha1.AerospikeVolumePolicy{
			WipeMethod: v1alpha1.VolumeWipeMethodDeleteFiles,
		},
	}

	result := ResolveWipeMethod(vol, spec)
	if result != v1alpha1.VolumeWipeMethodNone {
		t.Errorf("explicit 'none' should override policy, expected %q, got %q", v1alpha1.VolumeWipeMethodNone, result)
	}
}

func TestResolveWipeMethod_UnsetFallsToPolicy(t *testing.T) {
	// When per-volume wipeMethod is empty (unset), it should fall back to global policy
	vol := &v1alpha1.VolumeSpec{
		// WipeMethod is zero value ""
		Source: v1alpha1.VolumeSource{
			PersistentVolume: &v1alpha1.PersistentVolumeSpec{Size: "10Gi"},
		},
	}
	spec := &v1alpha1.AerospikeStorageSpec{
		FilesystemVolumePolicy: &v1alpha1.AerospikeVolumePolicy{
			WipeMethod: v1alpha1.VolumeWipeMethodDeleteFiles,
		},
	}

	result := ResolveWipeMethod(vol, spec)
	if result != v1alpha1.VolumeWipeMethodDeleteFiles {
		t.Errorf("unset wipeMethod should fall to policy, expected %q, got %q", v1alpha1.VolumeWipeMethodDeleteFiles, result)
	}
}

func TestResolveWipeMethod_NonPersistentVolume(t *testing.T) {
	vol := &v1alpha1.VolumeSpec{
		Source: v1alpha1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
	spec := &v1alpha1.AerospikeStorageSpec{
		FilesystemVolumePolicy: &v1alpha1.AerospikeVolumePolicy{
			WipeMethod: v1alpha1.VolumeWipeMethodDeleteFiles,
		},
	}

	result := ResolveWipeMethod(vol, spec)
	if result != v1alpha1.VolumeWipeMethodNone {
		t.Errorf("non-persistent volume should return none, got %q", result)
	}
}

func TestResolveWipeMethod_BlkdiscardWithHeaderCleanup(t *testing.T) {
	vol := &v1alpha1.VolumeSpec{
		WipeMethod: v1alpha1.VolumeWipeMethodBlkdiscardWithHeaderCleanup,
		Source: v1alpha1.VolumeSource{
			PersistentVolume: &v1alpha1.PersistentVolumeSpec{Size: "10Gi"},
		},
	}
	spec := &v1alpha1.AerospikeStorageSpec{}

	result := ResolveWipeMethod(vol, spec)
	if result != v1alpha1.VolumeWipeMethodBlkdiscardWithHeaderCleanup {
		t.Errorf("expected %q, got %q", v1alpha1.VolumeWipeMethodBlkdiscardWithHeaderCleanup, result)
	}
}

// --- ResolveCascadeDelete edge cases ---

func TestResolveCascadeDelete_BlockPolicyFallback(t *testing.T) {
	vol := &v1alpha1.VolumeSpec{
		Source: v1alpha1.VolumeSource{
			PersistentVolume: &v1alpha1.PersistentVolumeSpec{
				Size:       "10Gi",
				VolumeMode: corev1.PersistentVolumeBlock,
			},
		},
	}
	spec := &v1alpha1.AerospikeStorageSpec{
		BlockVolumePolicy: &v1alpha1.AerospikeVolumePolicy{
			CascadeDelete: boolPtr(true),
		},
		FilesystemVolumePolicy: &v1alpha1.AerospikeVolumePolicy{
			CascadeDelete: boolPtr(false),
		},
	}

	if !ResolveCascadeDelete(vol, spec) {
		t.Error("block volume should use block policy (true)")
	}
}

func TestResolveCascadeDelete_NonPersistentVolume(t *testing.T) {
	vol := &v1alpha1.VolumeSpec{
		Source: v1alpha1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
	spec := &v1alpha1.AerospikeStorageSpec{
		FilesystemVolumePolicy: &v1alpha1.AerospikeVolumePolicy{
			CascadeDelete: boolPtr(true),
		},
	}

	if ResolveCascadeDelete(vol, spec) {
		t.Error("non-persistent volume should return false regardless of policy")
	}
}

func TestResolveCascadeDelete_NilStorageSpec(t *testing.T) {
	vol := &v1alpha1.VolumeSpec{
		Source: v1alpha1.VolumeSource{
			PersistentVolume: &v1alpha1.PersistentVolumeSpec{Size: "10Gi"},
		},
	}

	if ResolveCascadeDelete(vol, nil) {
		t.Error("nil storage spec should return false")
	}
}

func TestResolveCascadeDelete_PerVolumeOverridesBlockPolicy(t *testing.T) {
	vol := &v1alpha1.VolumeSpec{
		CascadeDelete: boolPtr(false),
		Source: v1alpha1.VolumeSource{
			PersistentVolume: &v1alpha1.PersistentVolumeSpec{
				Size:       "10Gi",
				VolumeMode: corev1.PersistentVolumeBlock,
			},
		},
	}
	spec := &v1alpha1.AerospikeStorageSpec{
		BlockVolumePolicy: &v1alpha1.AerospikeVolumePolicy{
			CascadeDelete: boolPtr(true),
		},
	}

	if ResolveCascadeDelete(vol, spec) {
		t.Error("per-volume false should override block policy true")
	}
}

// --- getVolumePolicy tests ---

func TestGetVolumePolicy_NonPersistent_ReturnsNil(t *testing.T) {
	vol := &v1alpha1.VolumeSpec{
		Source: v1alpha1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
	spec := &v1alpha1.AerospikeStorageSpec{
		FilesystemVolumePolicy: &v1alpha1.AerospikeVolumePolicy{
			InitMethod: v1alpha1.VolumeInitMethodDeleteFiles,
		},
	}

	if getVolumePolicy(vol, spec) != nil {
		t.Error("expected nil for non-persistent volume")
	}
}

func TestGetVolumePolicy_NilStorageSpec(t *testing.T) {
	vol := &v1alpha1.VolumeSpec{
		Source: v1alpha1.VolumeSource{
			PersistentVolume: &v1alpha1.PersistentVolumeSpec{Size: "10Gi"},
		},
	}

	if getVolumePolicy(vol, nil) != nil {
		t.Error("expected nil for nil storageSpec")
	}
}

func TestGetVolumePolicy_BlockMode(t *testing.T) {
	blockPolicy := &v1alpha1.AerospikeVolumePolicy{
		InitMethod: v1alpha1.VolumeInitMethodBlkdiscard,
	}
	fsPolicy := &v1alpha1.AerospikeVolumePolicy{
		InitMethod: v1alpha1.VolumeInitMethodDeleteFiles,
	}

	vol := &v1alpha1.VolumeSpec{
		Source: v1alpha1.VolumeSource{
			PersistentVolume: &v1alpha1.PersistentVolumeSpec{
				Size:       "10Gi",
				VolumeMode: corev1.PersistentVolumeBlock,
			},
		},
	}
	spec := &v1alpha1.AerospikeStorageSpec{
		BlockVolumePolicy:      blockPolicy,
		FilesystemVolumePolicy: fsPolicy,
	}

	result := getVolumePolicy(vol, spec)
	if result != blockPolicy {
		t.Error("expected block policy for Block volume mode")
	}
}
