package storage

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/api/v1alpha1"
)

func TestBuildVolumes_NilSpec(t *testing.T) {
	vols, mounts := BuildVolumes(nil)
	if vols != nil || mounts != nil {
		t.Error("nil spec should return nil volumes and mounts")
	}
}

func TestBuildVolumes_EmptySpec(t *testing.T) {
	spec := &v1alpha1.AerospikeStorageSpec{}
	vols, mounts := BuildVolumes(spec)
	if len(vols) != 0 || len(mounts) != 0 {
		t.Error("empty spec should return empty volumes and mounts")
	}
}

func TestBuildVolumes_EmptyDir(t *testing.T) {
	spec := &v1alpha1.AerospikeStorageSpec{
		Volumes: []v1alpha1.VolumeSpec{
			{
				Name: "data",
				Source: v1alpha1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
				Aerospike: &v1alpha1.AerospikeVolumeAttachment{Path: "/opt/aerospike/data"},
			},
		},
	}

	vols, mounts := BuildVolumes(spec)
	if len(vols) != 1 {
		t.Fatalf("expected 1 volume, got %d", len(vols))
	}
	if vols[0].Name != "data" {
		t.Errorf("volume name = %q, want %q", vols[0].Name, "data")
	}
	if vols[0].EmptyDir == nil {
		t.Error("expected EmptyDir source")
	}

	if len(mounts) != 1 {
		t.Fatalf("expected 1 mount, got %d", len(mounts))
	}
	if mounts[0].MountPath != "/opt/aerospike/data" {
		t.Errorf("mount path = %q, want %q", mounts[0].MountPath, "/opt/aerospike/data")
	}
}

func TestBuildVolumes_Secret(t *testing.T) {
	spec := &v1alpha1.AerospikeStorageSpec{
		Volumes: []v1alpha1.VolumeSpec{
			{
				Name: "tls-certs",
				Source: v1alpha1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{SecretName: "my-secret"},
				},
				Aerospike: &v1alpha1.AerospikeVolumeAttachment{Path: "/etc/aerospike/certs"},
			},
		},
	}

	vols, mounts := BuildVolumes(spec)
	if len(vols) != 1 {
		t.Fatalf("expected 1 volume, got %d", len(vols))
	}
	if vols[0].Secret == nil {
		t.Error("expected Secret source")
	}
	if vols[0].Secret.SecretName != "my-secret" {
		t.Errorf("secret name = %q, want %q", vols[0].Secret.SecretName, "my-secret")
	}
	if len(mounts) != 1 {
		t.Fatalf("expected 1 mount, got %d", len(mounts))
	}
}

func TestBuildVolumes_ConfigMap(t *testing.T) {
	spec := &v1alpha1.AerospikeStorageSpec{
		Volumes: []v1alpha1.VolumeSpec{
			{
				Name: "config",
				Source: v1alpha1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{Name: "aero-config"},
					},
				},
				Aerospike: &v1alpha1.AerospikeVolumeAttachment{Path: "/etc/aerospike"},
			},
		},
	}

	vols, mounts := BuildVolumes(spec)
	if len(vols) != 1 {
		t.Fatalf("expected 1 volume, got %d", len(vols))
	}
	if vols[0].ConfigMap == nil {
		t.Error("expected ConfigMap source")
	}
	if len(mounts) != 1 {
		t.Fatalf("expected 1 mount, got %d", len(mounts))
	}
}

func TestBuildVolumes_PersistentVolume_NoVolume(t *testing.T) {
	spec := &v1alpha1.AerospikeStorageSpec{
		Volumes: []v1alpha1.VolumeSpec{
			{
				Name: "data",
				Source: v1alpha1.VolumeSource{
					PersistentVolume: &v1alpha1.PersistentVolumeSpec{
						Size: "10Gi",
					},
				},
				Aerospike: &v1alpha1.AerospikeVolumeAttachment{Path: "/opt/aerospike/data"},
			},
		},
	}

	vols, mounts := BuildVolumes(spec)
	// PersistentVolume sources should NOT generate inline volumes
	if len(vols) != 0 {
		t.Errorf("expected 0 volumes for PV source, got %d", len(vols))
	}
	// But mount should still be generated
	if len(mounts) != 1 {
		t.Fatalf("expected 1 mount for PV source, got %d", len(mounts))
	}
}

func TestBuildVolumes_NoAerospikeAttachment(t *testing.T) {
	spec := &v1alpha1.AerospikeStorageSpec{
		Volumes: []v1alpha1.VolumeSpec{
			{
				Name: "logs",
				Source: v1alpha1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
				// No Aerospike attachment
			},
		},
	}

	vols, mounts := BuildVolumes(spec)
	if len(vols) != 1 {
		t.Fatalf("expected 1 volume, got %d", len(vols))
	}
	if len(mounts) != 0 {
		t.Errorf("expected 0 mounts without Aerospike attachment, got %d", len(mounts))
	}
}

func TestBuildVolumes_MultipleVolumes(t *testing.T) {
	spec := &v1alpha1.AerospikeStorageSpec{
		Volumes: []v1alpha1.VolumeSpec{
			{
				Name: "data",
				Source: v1alpha1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
				Aerospike: &v1alpha1.AerospikeVolumeAttachment{Path: "/opt/aerospike/data"},
			},
			{
				Name: "sindex",
				Source: v1alpha1.VolumeSource{
					PersistentVolume: &v1alpha1.PersistentVolumeSpec{Size: "5Gi"},
				},
				Aerospike: &v1alpha1.AerospikeVolumeAttachment{Path: "/opt/aerospike/sindex"},
			},
		},
	}

	vols, mounts := BuildVolumes(spec)
	// Only emptyDir should produce an inline volume
	if len(vols) != 1 {
		t.Errorf("expected 1 inline volume, got %d", len(vols))
	}
	// Both should produce mounts
	if len(mounts) != 2 {
		t.Errorf("expected 2 mounts, got %d", len(mounts))
	}
}

// --- BuildVolumeClaimTemplates tests ---

func TestBuildVolumeClaimTemplates_NilSpec(t *testing.T) {
	claims := BuildVolumeClaimTemplates(nil)
	if claims != nil {
		t.Error("nil spec should return nil")
	}
}

func TestBuildVolumeClaimTemplates_NoPersistentVolumes(t *testing.T) {
	spec := &v1alpha1.AerospikeStorageSpec{
		Volumes: []v1alpha1.VolumeSpec{
			{
				Name:   "data",
				Source: v1alpha1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
			},
		},
	}

	claims := BuildVolumeClaimTemplates(spec)
	if len(claims) != 0 {
		t.Errorf("expected 0 claims for non-PV volumes, got %d", len(claims))
	}
}

func TestBuildVolumeClaimTemplates_DefaultAccessMode(t *testing.T) {
	spec := &v1alpha1.AerospikeStorageSpec{
		Volumes: []v1alpha1.VolumeSpec{
			{
				Name: "data",
				Source: v1alpha1.VolumeSource{
					PersistentVolume: &v1alpha1.PersistentVolumeSpec{
						Size: "10Gi",
					},
				},
			},
		},
	}

	claims := BuildVolumeClaimTemplates(spec)
	if len(claims) != 1 {
		t.Fatalf("expected 1 claim, got %d", len(claims))
	}

	if len(claims[0].Spec.AccessModes) != 1 || claims[0].Spec.AccessModes[0] != corev1.ReadWriteOnce {
		t.Errorf("default access mode should be ReadWriteOnce, got %v", claims[0].Spec.AccessModes)
	}
}

func TestBuildVolumeClaimTemplates_DefaultVolumeMode(t *testing.T) {
	spec := &v1alpha1.AerospikeStorageSpec{
		Volumes: []v1alpha1.VolumeSpec{
			{
				Name: "data",
				Source: v1alpha1.VolumeSource{
					PersistentVolume: &v1alpha1.PersistentVolumeSpec{
						Size: "10Gi",
					},
				},
			},
		},
	}

	claims := BuildVolumeClaimTemplates(spec)
	if len(claims) != 1 {
		t.Fatalf("expected 1 claim, got %d", len(claims))
	}

	if claims[0].Spec.VolumeMode == nil || *claims[0].Spec.VolumeMode != corev1.PersistentVolumeFilesystem {
		t.Error("default volume mode should be Filesystem")
	}
}

func TestBuildVolumeClaimTemplates_CustomAccessModes(t *testing.T) {
	spec := &v1alpha1.AerospikeStorageSpec{
		Volumes: []v1alpha1.VolumeSpec{
			{
				Name: "data",
				Source: v1alpha1.VolumeSource{
					PersistentVolume: &v1alpha1.PersistentVolumeSpec{
						Size:        "10Gi",
						AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
					},
				},
			},
		},
	}

	claims := BuildVolumeClaimTemplates(spec)
	if len(claims) != 1 {
		t.Fatalf("expected 1 claim, got %d", len(claims))
	}

	if claims[0].Spec.AccessModes[0] != corev1.ReadWriteMany {
		t.Errorf("access mode = %v, want ReadWriteMany", claims[0].Spec.AccessModes)
	}
}

func TestBuildVolumeClaimTemplates_StorageClass(t *testing.T) {
	spec := &v1alpha1.AerospikeStorageSpec{
		Volumes: []v1alpha1.VolumeSpec{
			{
				Name: "data",
				Source: v1alpha1.VolumeSource{
					PersistentVolume: &v1alpha1.PersistentVolumeSpec{
						Size:         "10Gi",
						StorageClass: "fast-ssd",
					},
				},
			},
		},
	}

	claims := BuildVolumeClaimTemplates(spec)
	if len(claims) != 1 {
		t.Fatalf("expected 1 claim, got %d", len(claims))
	}

	if claims[0].Spec.StorageClassName == nil || *claims[0].Spec.StorageClassName != "fast-ssd" {
		t.Error("storage class should be set to fast-ssd")
	}
}

func TestBuildVolumeClaimTemplates_NoStorageClass(t *testing.T) {
	spec := &v1alpha1.AerospikeStorageSpec{
		Volumes: []v1alpha1.VolumeSpec{
			{
				Name: "data",
				Source: v1alpha1.VolumeSource{
					PersistentVolume: &v1alpha1.PersistentVolumeSpec{
						Size: "10Gi",
					},
				},
			},
		},
	}

	claims := BuildVolumeClaimTemplates(spec)
	if claims[0].Spec.StorageClassName != nil {
		t.Error("storage class should be nil when not specified")
	}
}

func TestBuildVolumeClaimTemplates_WithSelector(t *testing.T) {
	spec := &v1alpha1.AerospikeStorageSpec{
		Volumes: []v1alpha1.VolumeSpec{
			{
				Name: "data",
				Source: v1alpha1.VolumeSource{
					PersistentVolume: &v1alpha1.PersistentVolumeSpec{
						Size: "10Gi",
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"type": "ssd"},
						},
					},
				},
			},
		},
	}

	claims := BuildVolumeClaimTemplates(spec)
	if claims[0].Spec.Selector == nil {
		t.Fatal("selector should be set")
	}
	if claims[0].Spec.Selector.MatchLabels["type"] != "ssd" {
		t.Error("selector should contain type=ssd label")
	}
}

func TestBuildVolumeClaimTemplates_StorageSize(t *testing.T) {
	spec := &v1alpha1.AerospikeStorageSpec{
		Volumes: []v1alpha1.VolumeSpec{
			{
				Name: "data",
				Source: v1alpha1.VolumeSource{
					PersistentVolume: &v1alpha1.PersistentVolumeSpec{
						Size: "50Gi",
					},
				},
			},
		},
	}

	claims := BuildVolumeClaimTemplates(spec)
	storageReq := claims[0].Spec.Resources.Requests[corev1.ResourceStorage]
	if storageReq.String() != "50Gi" {
		t.Errorf("storage size = %q, want %q", storageReq.String(), "50Gi")
	}
}

func TestBuildVolumeClaimTemplates_MultiplePVs(t *testing.T) {
	spec := &v1alpha1.AerospikeStorageSpec{
		Volumes: []v1alpha1.VolumeSpec{
			{
				Name: "data",
				Source: v1alpha1.VolumeSource{
					PersistentVolume: &v1alpha1.PersistentVolumeSpec{Size: "10Gi"},
				},
			},
			{
				Name:   "logs",
				Source: v1alpha1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
			},
			{
				Name: "sindex",
				Source: v1alpha1.VolumeSource{
					PersistentVolume: &v1alpha1.PersistentVolumeSpec{Size: "5Gi"},
				},
			},
		},
	}

	claims := BuildVolumeClaimTemplates(spec)
	if len(claims) != 2 {
		t.Errorf("expected 2 PVC claims, got %d", len(claims))
	}
}

// --- VolumeMountsForContainer tests ---

func TestVolumeMountsForContainer_NilSpec(t *testing.T) {
	mounts := VolumeMountsForContainer(nil, "my-sidecar", true)
	if mounts != nil {
		t.Error("nil spec should return nil")
	}
}

func TestVolumeMountsForContainer_Sidecar(t *testing.T) {
	spec := &v1alpha1.AerospikeStorageSpec{
		Volumes: []v1alpha1.VolumeSpec{
			{
				Name: "data",
				Source: v1alpha1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
				Sidecars: []v1alpha1.VolumeAttachment{
					{ContainerName: "exporter", Path: "/data"},
					{ContainerName: "other", Path: "/other"},
				},
			},
		},
	}

	mounts := VolumeMountsForContainer(spec, "exporter", true)
	if len(mounts) != 1 {
		t.Fatalf("expected 1 mount for exporter, got %d", len(mounts))
	}
	if mounts[0].Name != "data" || mounts[0].MountPath != "/data" {
		t.Errorf("mount = {%q, %q}, want {data, /data}", mounts[0].Name, mounts[0].MountPath)
	}
}

func TestVolumeMountsForContainer_InitContainer(t *testing.T) {
	spec := &v1alpha1.AerospikeStorageSpec{
		Volumes: []v1alpha1.VolumeSpec{
			{
				Name: "config",
				Source: v1alpha1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
				InitContainers: []v1alpha1.VolumeAttachment{
					{ContainerName: "init-data", Path: "/init"},
				},
			},
		},
	}

	mounts := VolumeMountsForContainer(spec, "init-data", false)
	if len(mounts) != 1 {
		t.Fatalf("expected 1 mount for init-data, got %d", len(mounts))
	}
	if mounts[0].MountPath != "/init" {
		t.Errorf("mount path = %q, want %q", mounts[0].MountPath, "/init")
	}
}

func TestVolumeMountsForContainer_NoMatch(t *testing.T) {
	spec := &v1alpha1.AerospikeStorageSpec{
		Volumes: []v1alpha1.VolumeSpec{
			{
				Name: "data",
				Source: v1alpha1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
				Sidecars: []v1alpha1.VolumeAttachment{
					{ContainerName: "other", Path: "/data"},
				},
			},
		},
	}

	mounts := VolumeMountsForContainer(spec, "nonexistent", true)
	if len(mounts) != 0 {
		t.Errorf("expected 0 mounts for nonexistent container, got %d", len(mounts))
	}
}

func TestVolumeMountsForContainer_MultipleVolumes(t *testing.T) {
	spec := &v1alpha1.AerospikeStorageSpec{
		Volumes: []v1alpha1.VolumeSpec{
			{
				Name: "data",
				Source: v1alpha1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
				Sidecars: []v1alpha1.VolumeAttachment{
					{ContainerName: "exporter", Path: "/data"},
				},
			},
			{
				Name: "logs",
				Source: v1alpha1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
				Sidecars: []v1alpha1.VolumeAttachment{
					{ContainerName: "exporter", Path: "/logs"},
				},
			},
		},
	}

	mounts := VolumeMountsForContainer(spec, "exporter", true)
	if len(mounts) != 2 {
		t.Errorf("expected 2 mounts for exporter across volumes, got %d", len(mounts))
	}
}

// --- PVC Metadata tests ---

func TestBuildVolumeClaimTemplates_WithMetadata(t *testing.T) {
	spec := &v1alpha1.AerospikeStorageSpec{
		Volumes: []v1alpha1.VolumeSpec{
			{
				Name: "data",
				Source: v1alpha1.VolumeSource{
					PersistentVolume: &v1alpha1.PersistentVolumeSpec{
						Size: "10Gi",
						Metadata: &v1alpha1.AerospikeObjectMeta{
							Labels:      map[string]string{"app": "aerospike", "tier": "storage"},
							Annotations: map[string]string{"backup.io/enabled": "true"},
						},
					},
				},
			},
		},
	}

	claims := BuildVolumeClaimTemplates(spec)
	if len(claims) != 1 {
		t.Fatalf("expected 1 claim, got %d", len(claims))
	}

	pvc := claims[0]
	if pvc.Labels == nil {
		t.Fatal("expected labels to be set")
	}
	if pvc.Labels["app"] != "aerospike" {
		t.Errorf("label app = %q, want %q", pvc.Labels["app"], "aerospike")
	}
	if pvc.Labels["tier"] != "storage" {
		t.Errorf("label tier = %q, want %q", pvc.Labels["tier"], "storage")
	}
	if pvc.Annotations == nil {
		t.Fatal("expected annotations to be set")
	}
	if pvc.Annotations["backup.io/enabled"] != "true" {
		t.Errorf("annotation backup.io/enabled = %q, want %q", pvc.Annotations["backup.io/enabled"], "true")
	}
}

func TestBuildVolumeClaimTemplates_MetadataNil(t *testing.T) {
	spec := &v1alpha1.AerospikeStorageSpec{
		Volumes: []v1alpha1.VolumeSpec{
			{
				Name: "data",
				Source: v1alpha1.VolumeSource{
					PersistentVolume: &v1alpha1.PersistentVolumeSpec{
						Size: "10Gi",
					},
				},
			},
		},
	}

	claims := BuildVolumeClaimTemplates(spec)
	if len(claims) != 1 {
		t.Fatalf("expected 1 claim, got %d", len(claims))
	}

	pvc := claims[0]
	if pvc.Labels != nil {
		t.Errorf("expected nil labels when metadata is nil, got %v", pvc.Labels)
	}
	if pvc.Annotations != nil {
		t.Errorf("expected nil annotations when metadata is nil, got %v", pvc.Annotations)
	}
}

// --- Block Volume Mode tests ---

func TestBuildVolumeClaimTemplates_BlockVolumeMode(t *testing.T) {
	spec := &v1alpha1.AerospikeStorageSpec{
		Volumes: []v1alpha1.VolumeSpec{
			{
				Name: "block-data",
				Source: v1alpha1.VolumeSource{
					PersistentVolume: &v1alpha1.PersistentVolumeSpec{
						Size:       "10Gi",
						VolumeMode: corev1.PersistentVolumeBlock,
					},
				},
			},
		},
	}

	claims := BuildVolumeClaimTemplates(spec)
	if len(claims) != 1 {
		t.Fatalf("expected 1 claim, got %d", len(claims))
	}

	if claims[0].Spec.VolumeMode == nil || *claims[0].Spec.VolumeMode != corev1.PersistentVolumeBlock {
		t.Error("volume mode should be Block")
	}
}

// --- PVC Metadata edge case tests ---

func TestBuildVolumeClaimTemplates_MetadataLabelsOnly(t *testing.T) {
	spec := &v1alpha1.AerospikeStorageSpec{
		Volumes: []v1alpha1.VolumeSpec{
			{
				Name: "data",
				Source: v1alpha1.VolumeSource{
					PersistentVolume: &v1alpha1.PersistentVolumeSpec{
						Size: "10Gi",
						Metadata: &v1alpha1.AerospikeObjectMeta{
							Labels: map[string]string{"app": "aerospike"},
						},
					},
				},
			},
		},
	}

	claims := BuildVolumeClaimTemplates(spec)
	if len(claims) != 1 {
		t.Fatalf("expected 1 claim, got %d", len(claims))
	}

	pvc := claims[0]
	if pvc.Labels == nil || pvc.Labels["app"] != "aerospike" {
		t.Errorf("expected label app=aerospike, got labels: %v", pvc.Labels)
	}
	if pvc.Annotations != nil {
		t.Errorf("expected nil annotations when only labels set, got: %v", pvc.Annotations)
	}
}

func TestBuildVolumeClaimTemplates_MetadataAnnotationsOnly(t *testing.T) {
	spec := &v1alpha1.AerospikeStorageSpec{
		Volumes: []v1alpha1.VolumeSpec{
			{
				Name: "data",
				Source: v1alpha1.VolumeSource{
					PersistentVolume: &v1alpha1.PersistentVolumeSpec{
						Size: "10Gi",
						Metadata: &v1alpha1.AerospikeObjectMeta{
							Annotations: map[string]string{"backup.io/enabled": "true"},
						},
					},
				},
			},
		},
	}

	claims := BuildVolumeClaimTemplates(spec)
	if len(claims) != 1 {
		t.Fatalf("expected 1 claim, got %d", len(claims))
	}

	pvc := claims[0]
	if pvc.Annotations == nil || pvc.Annotations["backup.io/enabled"] != "true" {
		t.Errorf("expected annotation backup.io/enabled=true, got annotations: %v", pvc.Annotations)
	}
	if pvc.Labels != nil {
		t.Errorf("expected nil labels when only annotations set, got: %v", pvc.Labels)
	}
}

func TestBuildVolumeClaimTemplates_MetadataDoesNotMutateSrc(t *testing.T) {
	origLabels := map[string]string{"app": "aerospike"}
	spec := &v1alpha1.AerospikeStorageSpec{
		Volumes: []v1alpha1.VolumeSpec{
			{
				Name: "data",
				Source: v1alpha1.VolumeSource{
					PersistentVolume: &v1alpha1.PersistentVolumeSpec{
						Size: "10Gi",
						Metadata: &v1alpha1.AerospikeObjectMeta{
							Labels: origLabels,
						},
					},
				},
			},
		},
	}

	claims := BuildVolumeClaimTemplates(spec)
	// Mutating the PVC labels should not affect the original
	claims[0].Labels["extra"] = "value"

	if _, exists := origLabels["extra"]; exists {
		t.Error("mutating PVC labels should not affect source metadata (should be cloned)")
	}
}

// --- HostPath tests ---

func TestBuildVolumes_HostPath(t *testing.T) {
	hostPathType := corev1.HostPathDirectory
	spec := &v1alpha1.AerospikeStorageSpec{
		Volumes: []v1alpha1.VolumeSpec{
			{
				Name: "host-data",
				Source: v1alpha1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: "/mnt/ssd/aerospike",
						Type: &hostPathType,
					},
				},
			},
		},
	}

	vols, mounts := BuildVolumes(spec)
	if len(vols) != 1 {
		t.Fatalf("expected 1 volume, got %d", len(vols))
	}
	if vols[0].HostPath == nil {
		t.Fatal("expected HostPath source")
	}
	if vols[0].HostPath.Path != "/mnt/ssd/aerospike" {
		t.Errorf("hostPath path = %q, want %q", vols[0].HostPath.Path, "/mnt/ssd/aerospike")
	}
	if *vols[0].HostPath.Type != corev1.HostPathDirectory {
		t.Errorf("hostPath type = %v, want Directory", *vols[0].HostPath.Type)
	}
	if len(mounts) != 0 {
		t.Errorf("expected 0 mounts without Aerospike attachment, got %d", len(mounts))
	}
}

func TestBuildVolumes_HostPath_WithAerospikeMount(t *testing.T) {
	spec := &v1alpha1.AerospikeStorageSpec{
		Volumes: []v1alpha1.VolumeSpec{
			{
				Name: "host-data",
				Source: v1alpha1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: "/mnt/ssd/aerospike",
					},
				},
				Aerospike: &v1alpha1.AerospikeVolumeAttachment{Path: "/opt/aerospike/data"},
			},
		},
	}

	vols, mounts := BuildVolumes(spec)
	if len(vols) != 1 {
		t.Fatalf("expected 1 volume, got %d", len(vols))
	}
	if vols[0].HostPath == nil {
		t.Fatal("expected HostPath source")
	}
	if len(mounts) != 1 {
		t.Fatalf("expected 1 mount, got %d", len(mounts))
	}
	if mounts[0].Name != "host-data" {
		t.Errorf("mount name = %q, want %q", mounts[0].Name, "host-data")
	}
	if mounts[0].MountPath != "/opt/aerospike/data" {
		t.Errorf("mount path = %q, want %q", mounts[0].MountPath, "/opt/aerospike/data")
	}
}

// --- Volume Mount Advanced Options tests ---

func TestBuildVolumes_ReadOnlyMount(t *testing.T) {
	spec := &v1alpha1.AerospikeStorageSpec{
		Volumes: []v1alpha1.VolumeSpec{
			{
				Name: "config",
				Source: v1alpha1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
				Aerospike: &v1alpha1.AerospikeVolumeAttachment{
					Path:     "/etc/aerospike",
					ReadOnly: true,
				},
			},
		},
	}

	_, mounts := BuildVolumes(spec)
	if len(mounts) != 1 {
		t.Fatalf("expected 1 mount, got %d", len(mounts))
	}
	if !mounts[0].ReadOnly {
		t.Error("mount should be read-only")
	}
}

func TestBuildVolumes_SubPath(t *testing.T) {
	spec := &v1alpha1.AerospikeStorageSpec{
		Volumes: []v1alpha1.VolumeSpec{
			{
				Name: "shared",
				Source: v1alpha1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
				Aerospike: &v1alpha1.AerospikeVolumeAttachment{
					Path:    "/opt/aerospike/data",
					SubPath: "aerospike-data",
				},
			},
		},
	}

	_, mounts := BuildVolumes(spec)
	if len(mounts) != 1 {
		t.Fatalf("expected 1 mount, got %d", len(mounts))
	}
	if mounts[0].SubPath != "aerospike-data" {
		t.Errorf("subPath = %q, want %q", mounts[0].SubPath, "aerospike-data")
	}
}

func TestBuildVolumes_MountPropagation(t *testing.T) {
	bidirectional := corev1.MountPropagationBidirectional
	spec := &v1alpha1.AerospikeStorageSpec{
		Volumes: []v1alpha1.VolumeSpec{
			{
				Name: "data",
				Source: v1alpha1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
				Aerospike: &v1alpha1.AerospikeVolumeAttachment{
					Path:             "/opt/aerospike/data",
					MountPropagation: &bidirectional,
				},
			},
		},
	}

	_, mounts := BuildVolumes(spec)
	if len(mounts) != 1 {
		t.Fatalf("expected 1 mount, got %d", len(mounts))
	}
	if mounts[0].MountPropagation == nil {
		t.Fatal("mount propagation should be set")
	}
	if *mounts[0].MountPropagation != corev1.MountPropagationBidirectional {
		t.Errorf("mount propagation = %v, want Bidirectional", *mounts[0].MountPropagation)
	}
}

func TestBuildVolumes_SubPathExpr(t *testing.T) {
	spec := &v1alpha1.AerospikeStorageSpec{
		Volumes: []v1alpha1.VolumeSpec{
			{
				Name: "shared",
				Source: v1alpha1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
				Aerospike: &v1alpha1.AerospikeVolumeAttachment{
					Path:        "/opt/aerospike/data",
					SubPathExpr: "$(POD_NAME)",
				},
			},
		},
	}

	_, mounts := BuildVolumes(spec)
	if len(mounts) != 1 {
		t.Fatalf("expected 1 mount, got %d", len(mounts))
	}
	if mounts[0].SubPathExpr != "$(POD_NAME)" {
		t.Errorf("subPathExpr = %q, want %q", mounts[0].SubPathExpr, "$(POD_NAME)")
	}
	if mounts[0].SubPath != "" {
		t.Error("subPath should be empty when subPathExpr is set")
	}
}

func TestVolumeMountsForContainer_AdvancedOptions(t *testing.T) {
	hostToContainer := corev1.MountPropagationHostToContainer
	spec := &v1alpha1.AerospikeStorageSpec{
		Volumes: []v1alpha1.VolumeSpec{
			{
				Name: "data",
				Source: v1alpha1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
				Sidecars: []v1alpha1.VolumeAttachment{
					{
						ContainerName:    "exporter",
						Path:             "/data",
						ReadOnly:         true,
						SubPath:          "metrics",
						MountPropagation: &hostToContainer,
					},
				},
			},
		},
	}

	mounts := VolumeMountsForContainer(spec, "exporter", true)
	if len(mounts) != 1 {
		t.Fatalf("expected 1 mount, got %d", len(mounts))
	}

	m := mounts[0]
	if !m.ReadOnly {
		t.Error("sidecar mount should be read-only")
	}
	if m.SubPath != "metrics" {
		t.Errorf("subPath = %q, want %q", m.SubPath, "metrics")
	}
	if m.MountPropagation == nil {
		t.Fatal("mount propagation should be set")
	}
	if *m.MountPropagation != corev1.MountPropagationHostToContainer {
		t.Errorf("mount propagation = %v, want HostToContainer", *m.MountPropagation)
	}
}
