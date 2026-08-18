package v1alpha1

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// --- persistentVolume-backed storage is immutable ---
//
// PV-backed volumes become the StatefulSet's volumeClaimTemplates, and VCTs
// cannot be changed on a live StatefulSet — reconcileStatefulSet deliberately
// never patches them. The pod template IS replaced whenever anything else makes
// needsUpdate true, and BuildVolumes emits a volumeMount for every volume with an
// `aerospike` attachment whatever its source. So a storage edit adding a
// PV-backed volume, landing together with an aerospikeConfig change, wrote a pod
// template referencing a volume no VCT provided: the kubelet rejected every pod,
// and under OnDelete the operator kept deleting pods that could not come back
// (#340).
//
// Every "rejects" case below is accepted without validateStorageImmutability.
// The "allows" cases are the ones that keep this from being a blunt freeze on
// spec.storage: only the PV-backed volumes are frozen.

const (
	storageTestImage = "aerospike:ce-8.1.1.1"
	storageDataVol   = "data"
	storageExtraVol  = "extra"
)

func storageCluster(storage *AerospikeStorageSpec) *AerospikeCluster {
	return &AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "aerospike"},
		Spec: AerospikeClusterSpec{
			Size:    3,
			Image:   storageTestImage,
			Storage: storage,
		},
	}
}

func pvVolume(name, size, storageClass string) VolumeSpec {
	return VolumeSpec{
		Name: name,
		Source: VolumeSource{
			PersistentVolume: &PersistentVolumeSpec{
				Size:         size,
				StorageClass: storageClass,
				VolumeMode:   corev1.PersistentVolumeFilesystem,
			},
		},
		Aerospike: &AerospikeVolumeAttachment{Path: "/opt/aerospike/" + name},
	}
}

func emptyDirVolume(name string) VolumeSpec {
	return VolumeSpec{
		Name:      name,
		Source:    VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
		Aerospike: &AerospikeVolumeAttachment{Path: "/opt/aerospike/" + name},
	}
}

func TestValidateUpdate_StorageImmutability(t *testing.T) {
	oneVolume := func() *AerospikeStorageSpec {
		return &AerospikeStorageSpec{Volumes: []VolumeSpec{pvVolume(storageDataVol, "10Gi", "fast")}}
	}

	tests := []struct {
		name    string
		old     *AerospikeStorageSpec
		updated *AerospikeStorageSpec
		wantErr string // substring; empty means the update must be accepted
	}{
		{
			// The severe case from the issue: adding the device that backs a new
			// namespace, which is exactly what an operator does alongside an
			// aerospikeConfig edit.
			name: "rejects adding a persistentVolume-backed volume",
			old:  oneVolume(),
			updated: &AerospikeStorageSpec{Volumes: []VolumeSpec{
				pvVolume(storageDataVol, "10Gi", "fast"),
				pvVolume(storageExtraVol, "20Gi", "fast"),
			}},
			wantErr: `cannot add volume "extra"`,
		},
		{
			name:    "rejects removing a persistentVolume-backed volume",
			old:     &AerospikeStorageSpec{Volumes: []VolumeSpec{pvVolume(storageDataVol, "10Gi", "fast"), pvVolume(storageExtraVol, "20Gi", "fast")}},
			updated: oneVolume(),
			wantErr: `cannot remove volume "extra"`,
		},
		{
			name:    "rejects resizing a persistentVolume-backed volume",
			old:     oneVolume(),
			updated: &AerospikeStorageSpec{Volumes: []VolumeSpec{pvVolume(storageDataVol, "20Gi", "fast")}},
			wantErr: `cannot change volume "data"`,
		},
		{
			name:    "rejects changing the storage class",
			old:     oneVolume(),
			updated: &AerospikeStorageSpec{Volumes: []VolumeSpec{pvVolume(storageDataVol, "10Gi", "slow")}},
			wantErr: `cannot change volume "data"`,
		},
		{
			name: "rejects changing the volume mode",
			old:  oneVolume(),
			updated: &AerospikeStorageSpec{Volumes: []VolumeSpec{{
				Name: storageDataVol,
				Source: VolumeSource{PersistentVolume: &PersistentVolumeSpec{
					Size: "10Gi", StorageClass: "fast", VolumeMode: corev1.PersistentVolumeBlock,
				}},
				Aerospike: &AerospikeVolumeAttachment{Path: "/opt/aerospike/data"},
			}}},
			wantErr: `cannot change volume "data"`,
		},
		{
			name:    "rejects dropping spec.storage entirely",
			old:     oneVolume(),
			updated: nil,
			wantErr: `cannot remove volume "data"`,
		},
		{
			name:    "rejects introducing storage on a cluster that had none",
			old:     nil,
			updated: oneVolume(),
			wantErr: `cannot add volume "data"`,
		},
		// --- accepted: not VolumeClaimTemplate-affecting ---
		{
			name:    "allows an unchanged storage spec",
			old:     oneVolume(),
			updated: oneVolume(),
		},
		{
			name:    "allows a cosmetic size rewrite of the same quantity",
			old:     oneVolume(),
			updated: &AerospikeStorageSpec{Volumes: []VolumeSpec{pvVolume(storageDataVol, "10240Mi", "fast")}},
		},
		{
			name: "allows adding an emptyDir volume",
			old:  oneVolume(),
			updated: &AerospikeStorageSpec{Volumes: []VolumeSpec{
				pvVolume(storageDataVol, "10Gi", "fast"),
				emptyDirVolume(storageExtraVol),
			}},
		},
		{
			name: "allows changing a mount path",
			old:  oneVolume(),
			updated: &AerospikeStorageSpec{Volumes: []VolumeSpec{{
				Name:      storageDataVol,
				Source:    VolumeSource{PersistentVolume: &PersistentVolumeSpec{Size: "10Gi", StorageClass: "fast", VolumeMode: corev1.PersistentVolumeFilesystem}},
				Aerospike: &AerospikeVolumeAttachment{Path: "/opt/aerospike/moved"},
			}}},
		},
		{
			name: "allows changing cleanupThreads",
			old:  oneVolume(),
			updated: &AerospikeStorageSpec{
				Volumes:        []VolumeSpec{pvVolume(storageDataVol, "10Gi", "fast")},
				CleanupThreads: 4,
			},
		},
		{
			name:    "allows both sides having no storage",
			old:     nil,
			updated: nil,
		},
	}

	v := &AerospikeClusterValidator{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := v.ValidateUpdate(context.Background(), storageCluster(tt.old), storageCluster(tt.updated))

			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateUpdate() error = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("ValidateUpdate() = nil, want an error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("ValidateUpdate() error = %q, want it to contain %q", err.Error(), tt.wantErr)
			}
			// The message has to say why, or an operator will just retry the edit.
			if !strings.Contains(err.Error(), "volumeClaimTemplates") {
				t.Errorf("ValidateUpdate() error = %q, want it to mention volumeClaimTemplates", err.Error())
			}
		})
	}
}

// TestValidateUpdate_RackStorageImmutability covers the per-rack override, which
// feeds the same BuildVolumeClaimTemplates call for that rack's StatefulSet.
func TestValidateUpdate_RackStorageImmutability(t *testing.T) {
	withRack := func(storage *AerospikeStorageSpec) *AerospikeCluster {
		cluster := storageCluster(&AerospikeStorageSpec{Volumes: []VolumeSpec{pvVolume(storageDataVol, "10Gi", "fast")}})
		cluster.Spec.RackConfig = &RackConfig{Racks: []Rack{{ID: 1, Storage: storage}, {ID: 2}}}
		return cluster
	}
	rackVolume := func(size string) *AerospikeStorageSpec {
		return &AerospikeStorageSpec{Volumes: []VolumeSpec{pvVolume(storageDataVol, size, "fast")}}
	}

	v := &AerospikeClusterValidator{}

	t.Run("rejects resizing a rack's persistentVolume", func(t *testing.T) {
		_, err := v.ValidateUpdate(context.Background(), withRack(rackVolume("10Gi")), withRack(rackVolume("20Gi")))
		if err == nil {
			t.Fatal("ValidateUpdate() = nil, want an error")
		}
		if !strings.Contains(err.Error(), "spec.rackConfig.racks[id=1].storage") {
			t.Errorf("ValidateUpdate() error = %q, want it to name the rack's storage field", err.Error())
		}
	})

	t.Run("allows an unchanged rack storage override", func(t *testing.T) {
		if _, err := v.ValidateUpdate(context.Background(), withRack(rackVolume("10Gi")), withRack(rackVolume("10Gi"))); err != nil {
			t.Fatalf("ValidateUpdate() error = %v, want nil", err)
		}
	})

	t.Run("does not report removing a whole rack as a storage change", func(t *testing.T) {
		before := withRack(rackVolume("10Gi"))
		after := storageCluster(&AerospikeStorageSpec{Volumes: []VolumeSpec{pvVolume(storageDataVol, "10Gi", "fast")}})
		after.Spec.RackConfig = &RackConfig{Racks: []Rack{{ID: 2}}}

		if _, err := v.ValidateUpdate(context.Background(), before, after); err != nil {
			t.Fatalf("ValidateUpdate() error = %v; removing a rack is cleanupRemovedRacks' job, not a storage change", err)
		}
	})
}

// TestValidateUpdate_AddingARackWithStorageIsAllowed pins the rack-*addition*
// branch of the immutability guard, which nothing else covered.
//
// The existing cases exercise rack removal, and the comparison loop runs over
// the racks in the *new* spec — so a removed rack is never visited and those
// cases pass with or without the `if !ok { continue }` skip. Adding a rack is
// the direction that actually reaches it.
//
// The regression this guards: without that skip, adding a rack that carries its
// own persistentVolume storage is rejected at admission, because the new rack
// has no counterpart in the old spec and reads as a PV change. Adding a rack
// creates a *new* StatefulSet with fresh volumeClaimTemplates, so there is no
// in-place VCT change to refuse.
func TestValidateUpdate_AddingARackWithStorageIsAllowed(t *testing.T) {
	base := func() *AerospikeCluster {
		return storageCluster(&AerospikeStorageSpec{Volumes: []VolumeSpec{pvVolume(storageDataVol, "10Gi", "fast")}})
	}
	rackStorage := &AerospikeStorageSpec{Volumes: []VolumeSpec{pvVolume(storageDataVol, "10Gi", "fast")}}

	before := base()
	before.Spec.RackConfig = &RackConfig{Racks: []Rack{{ID: 1, Storage: rackStorage}}}

	after := base()
	after.Spec.RackConfig = &RackConfig{Racks: []Rack{
		{ID: 1, Storage: rackStorage},
		{ID: 2, Storage: rackStorage}, // new rack, new StatefulSet, fresh VCTs
	}}

	v := &AerospikeClusterValidator{}
	if _, err := v.ValidateUpdate(context.Background(), before, after); err != nil {
		t.Fatalf("ValidateUpdate() error = %v; adding a rack creates a new StatefulSet, so its storage is not an in-place VCT change", err)
	}
}
