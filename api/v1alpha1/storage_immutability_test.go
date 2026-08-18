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

	// The rack-ADDITION branch. The loop in validateStorageImmutability iterates
	// the NEW racks, so a rack present only in the new spec has no old
	// counterpart and is skipped by `if !ok { continue }`. The removal case above
	// never visits that branch — the removed rack is absent from the loop
	// entirely — so it passed with or without the guard.
	//
	// Without the guard, a newly added rack is compared against an empty old
	// storage spec and every volume it carries is reported as "cannot add
	// volume". That would refuse a legitimate and ordinary operation: scaling out
	// to a new zone with its own storage class. Adding a rack creates a fresh
	// StatefulSet, so there is no existing VolumeClaimTemplate for the
	// immutability rule to protect.
	t.Run("adding a whole rack that carries storage is allowed", func(t *testing.T) {
		before := storageCluster(&AerospikeStorageSpec{Volumes: []VolumeSpec{pvVolume(storageDataVol, "10Gi", "fast")}})
		before.Spec.RackConfig = &RackConfig{Racks: []Rack{{ID: 1, Storage: rackVolume("10Gi")}}}

		after := before.DeepCopy()
		after.Spec.RackConfig.Racks = append(after.Spec.RackConfig.Racks, Rack{
			ID: 2,
			// A different size AND a different storage class, so the comparison
			// would definitely fire if the new rack were measured against an
			// empty old spec.
			Storage: &AerospikeStorageSpec{Volumes: []VolumeSpec{pvVolume(storageDataVol, "50Gi", "slow")}},
		})

		if _, err := v.ValidateUpdate(context.Background(), before, after); err != nil {
			t.Fatalf("ValidateUpdate() error = %v; adding a rack creates a new StatefulSet, "+
				"so there is no existing volumeClaimTemplate for immutability to protect", err)
		}
	})

	// And the rack that already existed is still frozen while another is added,
	// so the guard cannot be mistaken for "skip the check whenever racks change".
	t.Run("adding a rack does not unfreeze the racks that already existed", func(t *testing.T) {
		before := storageCluster(&AerospikeStorageSpec{Volumes: []VolumeSpec{pvVolume(storageDataVol, "10Gi", "fast")}})
		before.Spec.RackConfig = &RackConfig{Racks: []Rack{{ID: 1, Storage: rackVolume("10Gi")}}}

		after := before.DeepCopy()
		after.Spec.RackConfig.Racks[0].Storage = rackVolume("20Gi")
		after.Spec.RackConfig.Racks = append(after.Spec.RackConfig.Racks, Rack{ID: 2})

		_, err := v.ValidateUpdate(context.Background(), before, after)
		if err == nil {
			t.Fatal("ValidateUpdate() = nil, want an error: rack 1's persistentVolume was resized")
		}
		if !strings.Contains(err.Error(), "spec.rackConfig.racks[id=1].storage") {
			t.Errorf("ValidateUpdate() error = %q, want it to name rack 1's storage", err.Error())
		}
	})
}
