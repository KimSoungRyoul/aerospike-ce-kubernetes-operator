package v1alpha1

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// --- a rack's storage override is validated like the cluster's ---
//
// reconcileStatefulSet resolves rack.Storage OVER cluster.Spec.Storage before
// calling BuildVolumes and BuildVolumeClaimTemplates, so for that rack it IS the
// storage spec. It was never passed through validateStorage, on create or update,
// so a rack could carry input the cluster level rejects.
//
// The sharpest case is a volume with NO source: every VolumeSource field is
// +optional, so it is schema-valid, and it is exactly the input that produces an
// unbacked mount — BuildVolumes emits the mount for any volume with an
// `aerospike` attachment, volumeForSpec returns nil, BuildVolumeClaimTemplates
// skips it, and the kubelet rejects every pod of that rack.
//
// validateStorageImmutability cannot catch it either: pvBackedVolumes indexes
// only volumes whose Source.PersistentVolume is non-nil, so a sourceless volume
// is invisible to it.

func rackStorageCluster(clusterStorage, rackStorage *AerospikeStorageSpec) *AerospikeCluster {
	cluster := &AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "aerospike"},
		Spec: AerospikeClusterSpec{
			Size:    3,
			Image:   "aerospike:ce-8.1.1.1",
			Storage: clusterStorage,
		},
	}
	if rackStorage != nil {
		cluster.Spec.RackConfig = &RackConfig{
			Racks: []Rack{{ID: 1, Storage: rackStorage}, {ID: 2}},
		}
	}
	return cluster
}

func sourcelessVolume(name string) VolumeSpec {
	return VolumeSpec{
		Name:      name,
		Source:    VolumeSource{}, // schema-valid: every source field is +optional
		Aerospike: &AerospikeVolumeAttachment{Path: "/opt/aerospike/" + name},
	}
}

func TestValidate_RackStorageOverrideIsValidated(t *testing.T) {
	tests := []struct {
		name    string
		storage *AerospikeStorageSpec
		wantErr string // substring; empty means accepted
	}{
		{
			name:    "a volume with no source is rejected",
			storage: &AerospikeStorageSpec{Volumes: []VolumeSpec{sourcelessVolume("ghost")}},
			wantErr: `spec.rackConfig.racks[id=1].storage.volumes[0] "ghost": exactly one volume source must be specified`,
		},
		{
			name: "duplicate volume names are rejected",
			storage: &AerospikeStorageSpec{Volumes: []VolumeSpec{
				{Name: "data", Source: VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
				{Name: "data", Source: VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
			}},
			wantErr: "spec.rackConfig.racks[id=1].storage.volumes: duplicate volume name",
		},
		{
			name: "a valid rack override is accepted",
			storage: &AerospikeStorageSpec{Volumes: []VolumeSpec{{
				Name:      "data",
				Source:    VolumeSource{PersistentVolume: &PersistentVolumeSpec{Size: "10Gi"}},
				Aerospike: &AerospikeVolumeAttachment{Path: "/opt/aerospike/data"},
			}}},
		},
		{
			name:    "a rack with no storage override is accepted",
			storage: nil,
		},
	}

	v := &AerospikeClusterValidator{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := rackStorageCluster(nil, tt.storage)

			// Both entry points, because the gap was open on each.
			for _, tc := range []struct {
				entry string
				run   func() error
			}{
				{"ValidateCreate", func() error {
					_, err := v.ValidateCreate(context.Background(), cluster)
					return err
				}},
				{"ValidateUpdate", func() error {
					_, err := v.ValidateUpdate(context.Background(), cluster.DeepCopy(), cluster)
					return err
				}},
			} {
				err := tc.run()
				if tt.wantErr == "" {
					if err != nil {
						t.Errorf("%s() error = %v, want nil", tc.entry, err)
					}
					continue
				}
				if err == nil {
					t.Errorf("%s() = nil, want an error containing %q", tc.entry, tt.wantErr)
					continue
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("%s() error = %q, want it to contain %q", tc.entry, err.Error(), tt.wantErr)
				}
			}
		})
	}
}

// TestValidate_ClusterAndRackStorageUseTheSameRules pins that the two paths
// share one implementation: the same bad input must be rejected wherever it sits,
// differing only in which field the message names.
func TestValidate_ClusterAndRackStorageUseTheSameRules(t *testing.T) {
	v := &AerospikeClusterValidator{}
	bad := &AerospikeStorageSpec{Volumes: []VolumeSpec{sourcelessVolume("ghost")}}

	_, clusterErr := v.ValidateCreate(context.Background(), rackStorageCluster(bad, nil))
	_, rackErr := v.ValidateCreate(context.Background(), rackStorageCluster(nil, bad))

	if clusterErr == nil || rackErr == nil {
		t.Fatalf("both must be rejected; cluster=%v rack=%v", clusterErr, rackErr)
	}
	const shared = `"ghost": exactly one volume source must be specified`
	if !strings.Contains(clusterErr.Error(), shared) || !strings.Contains(rackErr.Error(), shared) {
		t.Errorf("messages diverged:\n  cluster: %v\n  rack:    %v", clusterErr, rackErr)
	}
	if !strings.Contains(rackErr.Error(), "spec.rackConfig.racks[id=1].") {
		t.Errorf("rack error does not name the rack: %v", rackErr)
	}
}

// TestValidateUpdate_RackStorageAdditionIsRejected covers the rack-ADDITION
// branch of the immutability guard, which had no test — only the rack-resize
// case did.
func TestValidateUpdate_RackStorageAdditionIsRejected(t *testing.T) {
	pv := func(name string) VolumeSpec {
		return VolumeSpec{
			Name:      name,
			Source:    VolumeSource{PersistentVolume: &PersistentVolumeSpec{Size: "10Gi", VolumeMode: corev1.PersistentVolumeFilesystem}},
			Aerospike: &AerospikeVolumeAttachment{Path: "/opt/aerospike/" + name},
		}
	}
	oldCluster := rackStorageCluster(nil, &AerospikeStorageSpec{Volumes: []VolumeSpec{pv("data")}})
	newCluster := rackStorageCluster(nil, &AerospikeStorageSpec{Volumes: []VolumeSpec{pv("data"), pv("extra")}})

	v := &AerospikeClusterValidator{}
	_, err := v.ValidateUpdate(context.Background(), oldCluster, newCluster)
	if err == nil {
		t.Fatal("ValidateUpdate() = nil, want an error for a persistentVolume added to a rack override")
	}
	if !strings.Contains(err.Error(), `spec.rackConfig.racks[id=1].storage is immutable`) {
		t.Errorf("ValidateUpdate() error = %q, want it to name the rack's storage field", err.Error())
	}
	if !strings.Contains(err.Error(), `cannot add volume "extra"`) {
		t.Errorf("ValidateUpdate() error = %q, want it to name the added volume", err.Error())
	}
}
