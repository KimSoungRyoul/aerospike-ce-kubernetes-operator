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

package v1alpha1

import (
	"context"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// --- Defaulter tests ---

func TestAerospikeClusterTemplateDefault(t *testing.T) {
	tests := []struct {
		name   string
		tmpl   *AerospikeClusterTemplate
		verify func(t *testing.T, tmpl *AerospikeClusterTemplate)
	}{
		{
			name: "empty podAntiAffinityLevel defaults to preferred",
			tmpl: &AerospikeClusterTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
				Spec: AerospikeClusterTemplateSpec{
					Scheduling: &TemplateScheduling{
						PodAntiAffinityLevel: "",
					},
				},
			},
			verify: func(t *testing.T, tmpl *AerospikeClusterTemplate) {
				if tmpl.Spec.Scheduling.PodAntiAffinityLevel != PodAntiAffinityPreferred {
					t.Errorf("PodAntiAffinityLevel = %q, want %q",
						tmpl.Spec.Scheduling.PodAntiAffinityLevel, PodAntiAffinityPreferred)
				}
			},
		},
		{
			name: "empty volumeMode defaults to Filesystem",
			tmpl: &AerospikeClusterTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
				Spec: AerospikeClusterTemplateSpec{
					Storage: &TemplateStorage{
						StorageClassName: "standard",
					},
				},
			},
			verify: func(t *testing.T, tmpl *AerospikeClusterTemplate) {
				if tmpl.Spec.Storage.VolumeMode != corev1.PersistentVolumeFilesystem {
					t.Errorf("VolumeMode = %q, want %q",
						tmpl.Spec.Storage.VolumeMode, corev1.PersistentVolumeFilesystem)
				}
			},
		},
		{
			name: "empty accessModes defaults to ReadWriteOnce",
			tmpl: &AerospikeClusterTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
				Spec: AerospikeClusterTemplateSpec{
					Storage: &TemplateStorage{
						StorageClassName: "standard",
					},
				},
			},
			verify: func(t *testing.T, tmpl *AerospikeClusterTemplate) {
				if len(tmpl.Spec.Storage.AccessModes) != 1 {
					t.Fatalf("AccessModes length = %d, want 1", len(tmpl.Spec.Storage.AccessModes))
				}
				if tmpl.Spec.Storage.AccessModes[0] != corev1.ReadWriteOnce {
					t.Errorf("AccessModes[0] = %q, want %q",
						tmpl.Spec.Storage.AccessModes[0], corev1.ReadWriteOnce)
				}
			},
		},
		{
			name: "nil scheduling and nil storage does not panic",
			tmpl: &AerospikeClusterTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
				Spec:       AerospikeClusterTemplateSpec{},
			},
			verify: func(t *testing.T, tmpl *AerospikeClusterTemplate) {
				if tmpl.Spec.Scheduling != nil {
					t.Error("Scheduling should remain nil")
				}
				if tmpl.Spec.Storage != nil {
					t.Error("Storage should remain nil")
				}
			},
		},
		{
			name: "pre-set podAntiAffinityLevel is not overwritten",
			tmpl: &AerospikeClusterTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
				Spec: AerospikeClusterTemplateSpec{
					Scheduling: &TemplateScheduling{
						PodAntiAffinityLevel: PodAntiAffinityRequired,
					},
				},
			},
			verify: func(t *testing.T, tmpl *AerospikeClusterTemplate) {
				if tmpl.Spec.Scheduling.PodAntiAffinityLevel != PodAntiAffinityRequired {
					t.Errorf("PodAntiAffinityLevel = %q, want %q (should not be overwritten)",
						tmpl.Spec.Scheduling.PodAntiAffinityLevel, PodAntiAffinityRequired)
				}
			},
		},
		{
			name: "pre-set volumeMode is not overwritten",
			tmpl: &AerospikeClusterTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
				Spec: AerospikeClusterTemplateSpec{
					Storage: &TemplateStorage{
						VolumeMode: corev1.PersistentVolumeBlock,
					},
				},
			},
			verify: func(t *testing.T, tmpl *AerospikeClusterTemplate) {
				if tmpl.Spec.Storage.VolumeMode != corev1.PersistentVolumeBlock {
					t.Errorf("VolumeMode = %q, want %q (should not be overwritten)",
						tmpl.Spec.Storage.VolumeMode, corev1.PersistentVolumeBlock)
				}
			},
		},
		{
			name: "pre-set accessModes are not overwritten",
			tmpl: &AerospikeClusterTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
				Spec: AerospikeClusterTemplateSpec{
					Storage: &TemplateStorage{
						AccessModes: []corev1.PersistentVolumeAccessMode{
							corev1.ReadWriteMany,
						},
					},
				},
			},
			verify: func(t *testing.T, tmpl *AerospikeClusterTemplate) {
				if len(tmpl.Spec.Storage.AccessModes) != 1 {
					t.Fatalf("AccessModes length = %d, want 1", len(tmpl.Spec.Storage.AccessModes))
				}
				if tmpl.Spec.Storage.AccessModes[0] != corev1.ReadWriteMany {
					t.Errorf("AccessModes[0] = %q, want %q (should not be overwritten)",
						tmpl.Spec.Storage.AccessModes[0], corev1.ReadWriteMany)
				}
			},
		},
	}

	d := &AerospikeClusterTemplateDefaulter{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := d.Default(context.Background(), tt.tmpl); err != nil {
				t.Fatalf("Default() unexpected error: %v", err)
			}
			tt.verify(t, tt.tmpl)
		})
	}
}

// --- Validator tests ---

func TestAerospikeClusterTemplateValidate(t *testing.T) {
	tests := []struct {
		name        string
		tmpl        *AerospikeClusterTemplate
		wantErr     bool
		errContains string
		wantWarning string
	}{
		{
			name: "valid minimal template passes",
			tmpl: &AerospikeClusterTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "minimal", Namespace: "default"},
				Spec:       AerospikeClusterTemplateSpec{},
			},
			wantErr: false,
		},
		{
			name: "empty spec passes (all optional)",
			tmpl: &AerospikeClusterTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "empty", Namespace: "default"},
				Spec:       AerospikeClusterTemplateSpec{},
			},
			wantErr: false,
		},
		// V-T01: podAntiAffinityLevel validation
		{
			name: "V-T01: invalid podAntiAffinityLevel is rejected",
			tmpl: &AerospikeClusterTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "bad-level", Namespace: "default"},
				Spec: AerospikeClusterTemplateSpec{
					Scheduling: &TemplateScheduling{
						PodAntiAffinityLevel: "hard",
					},
				},
			},
			wantErr:     true,
			errContains: "podAntiAffinityLevel",
		},
		{
			name: "V-T01: podAntiAffinityLevel=none passes",
			tmpl: &AerospikeClusterTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "level-none", Namespace: "default"},
				Spec: AerospikeClusterTemplateSpec{
					Scheduling: &TemplateScheduling{
						PodAntiAffinityLevel: PodAntiAffinityNone,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "V-T01: podAntiAffinityLevel=preferred passes",
			tmpl: &AerospikeClusterTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "level-preferred", Namespace: "default"},
				Spec: AerospikeClusterTemplateSpec{
					Scheduling: &TemplateScheduling{
						PodAntiAffinityLevel: PodAntiAffinityPreferred,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "V-T01: podAntiAffinityLevel=required passes",
			tmpl: &AerospikeClusterTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "level-required", Namespace: "default"},
				Spec: AerospikeClusterTemplateSpec{
					Scheduling: &TemplateScheduling{
						PodAntiAffinityLevel: PodAntiAffinityRequired,
					},
				},
			},
			wantErr: false,
		},
		// V-T05: podManagementPolicy validation
		{
			name: "V-T05: invalid podManagementPolicy is rejected",
			tmpl: &AerospikeClusterTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "bad-policy", Namespace: "default"},
				Spec: AerospikeClusterTemplateSpec{
					Scheduling: &TemplateScheduling{
						PodManagementPolicy: appsv1.PodManagementPolicyType("Immediate"),
					},
				},
			},
			wantErr:     true,
			errContains: "podManagementPolicy",
		},
		{
			name: "V-T05: podManagementPolicy=OrderedReady passes",
			tmpl: &AerospikeClusterTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "ordered", Namespace: "default"},
				Spec: AerospikeClusterTemplateSpec{
					Scheduling: &TemplateScheduling{
						PodManagementPolicy: appsv1.OrderedReadyPodManagement,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "V-T05: podManagementPolicy=Parallel passes",
			tmpl: &AerospikeClusterTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "parallel", Namespace: "default"},
				Spec: AerospikeClusterTemplateSpec{
					Scheduling: &TemplateScheduling{
						PodManagementPolicy: appsv1.ParallelPodManagement,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "V-T05: empty podManagementPolicy passes (optional)",
			tmpl: &AerospikeClusterTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "empty-policy", Namespace: "default"},
				Spec: AerospikeClusterTemplateSpec{
					Scheduling: &TemplateScheduling{},
				},
			},
			wantErr: false,
		},
		// V-T02: maxRacksPerNode validation
		{
			name: "V-T02: negative maxRacksPerNode is rejected",
			tmpl: &AerospikeClusterTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "neg-racks", Namespace: "default"},
				Spec: AerospikeClusterTemplateSpec{
					RackConfig: &TemplateRackConfig{
						MaxRacksPerNode: -1,
					},
				},
			},
			wantErr:     true,
			errContains: "maxRacksPerNode",
		},
		{
			name: "V-T02: zero maxRacksPerNode passes",
			tmpl: &AerospikeClusterTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "zero-racks", Namespace: "default"},
				Spec: AerospikeClusterTemplateSpec{
					RackConfig: &TemplateRackConfig{
						MaxRacksPerNode: 0,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "V-T02: positive maxRacksPerNode passes",
			tmpl: &AerospikeClusterTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "pos-racks", Namespace: "default"},
				Spec: AerospikeClusterTemplateSpec{
					RackConfig: &TemplateRackConfig{
						MaxRacksPerNode: 3,
					},
				},
			},
			wantErr: false,
		},
		// V-T03: localPVRequired warning
		{
			name: "V-T03: localPVRequired=true with empty storageClassName produces warning",
			tmpl: &AerospikeClusterTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "localpv-warn", Namespace: "default"},
				Spec: AerospikeClusterTemplateSpec{
					Storage: &TemplateStorage{
						LocalPVRequired:  true,
						StorageClassName: "",
					},
				},
			},
			wantErr:     false,
			wantWarning: "localPVRequired=true",
		},
		{
			name: "V-T03: localPVRequired=true with storageClassName set produces no warning",
			tmpl: &AerospikeClusterTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "localpv-ok", Namespace: "default"},
				Spec: AerospikeClusterTemplateSpec{
					Storage: &TemplateStorage{
						LocalPVRequired:  true,
						StorageClassName: "local-storage",
					},
				},
			},
			wantErr: false,
		},
		// V-T04: resources requests != limits warning
		{
			name: "V-T04: resources where requests != limits produces warning",
			tmpl: &AerospikeClusterTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "resources-warn", Namespace: "default"},
				Spec: AerospikeClusterTemplateSpec{
					Resources: &corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("500m"),
							corev1.ResourceMemory: resource.MustParse("1Gi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("1000m"),
							corev1.ResourceMemory: resource.MustParse("2Gi"),
						},
					},
				},
			},
			wantErr:     false,
			wantWarning: "Guaranteed QoS",
		},
		{
			name: "V-T04: resources where requests == limits produces no warning",
			tmpl: &AerospikeClusterTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "resources-ok", Namespace: "default"},
				Spec: AerospikeClusterTemplateSpec{
					Resources: &corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("1"),
							corev1.ResourceMemory: resource.MustParse("2Gi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("1"),
							corev1.ResourceMemory: resource.MustParse("2Gi"),
						},
					},
				},
			},
			wantErr: false,
		},
		// Heartbeat mode validation
		{
			name: "heartbeat mode=multicast is rejected for CE",
			tmpl: &AerospikeClusterTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "hb-multicast", Namespace: "default"},
				Spec: AerospikeClusterTemplateSpec{
					AerospikeConfig: &TemplateAerospikeConfig{
						Network: &TemplateNetworkConfig{
							Heartbeat: &TemplateHeartbeatConfig{
								Mode: "multicast",
							},
						},
					},
				},
			},
			wantErr:     true,
			errContains: "heartbeat.mode",
		},
		{
			name: "heartbeat mode=mesh passes",
			tmpl: &AerospikeClusterTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "hb-mesh", Namespace: "default"},
				Spec: AerospikeClusterTemplateSpec{
					AerospikeConfig: &TemplateAerospikeConfig{
						Network: &TemplateNetworkConfig{
							Heartbeat: &TemplateHeartbeatConfig{
								Mode: "mesh",
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "heartbeat mode empty passes (optional)",
			tmpl: &AerospikeClusterTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "hb-empty", Namespace: "default"},
				Spec: AerospikeClusterTemplateSpec{
					AerospikeConfig: &TemplateAerospikeConfig{
						Network: &TemplateNetworkConfig{
							Heartbeat: &TemplateHeartbeatConfig{
								Mode: "",
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "nil aerospikeConfig passes",
			tmpl: &AerospikeClusterTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "no-config", Namespace: "default"},
				Spec: AerospikeClusterTemplateSpec{
					AerospikeConfig: nil,
				},
			},
			wantErr: false,
		},
		{
			name: "multiple errors are all reported",
			tmpl: &AerospikeClusterTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "multi-err", Namespace: "default"},
				Spec: AerospikeClusterTemplateSpec{
					Scheduling: &TemplateScheduling{
						PodAntiAffinityLevel: "invalid",
						PodManagementPolicy:  appsv1.PodManagementPolicyType("BadPolicy"),
					},
					RackConfig: &TemplateRackConfig{
						MaxRacksPerNode: -5,
					},
					AerospikeConfig: &TemplateAerospikeConfig{
						Network: &TemplateNetworkConfig{
							Heartbeat: &TemplateHeartbeatConfig{
								Mode: "multicast",
							},
						},
					},
				},
			},
			wantErr:     true,
			errContains: "podAntiAffinityLevel",
		},
	}

	v := &AerospikeClusterTemplateValidator{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test ValidateCreate
			warnings, err := v.ValidateCreate(context.Background(), tt.tmpl)
			if tt.wantErr {
				if err == nil {
					t.Fatal("ValidateCreate() expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("ValidateCreate() error = %q, want it to contain %q",
						err.Error(), tt.errContains)
				}
			} else {
				if err != nil {
					t.Fatalf("ValidateCreate() unexpected error: %v", err)
				}
			}

			if tt.wantWarning != "" {
				found := false
				for _, w := range warnings {
					if strings.Contains(w, tt.wantWarning) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("ValidateCreate() warnings = %v, want warning containing %q",
						warnings, tt.wantWarning)
				}
			}

			// Test ValidateUpdate uses the same validate() logic
			warningsUpdate, errUpdate := v.ValidateUpdate(context.Background(), tt.tmpl, tt.tmpl)
			if tt.wantErr {
				if errUpdate == nil {
					t.Fatal("ValidateUpdate() expected error, got nil")
				}
			} else {
				if errUpdate != nil {
					t.Fatalf("ValidateUpdate() unexpected error: %v", errUpdate)
				}
			}

			if tt.wantWarning != "" {
				found := false
				for _, w := range warningsUpdate {
					if strings.Contains(w, tt.wantWarning) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("ValidateUpdate() warnings = %v, want warning containing %q",
						warningsUpdate, tt.wantWarning)
				}
			}
		})
	}
}

// --- V-T06/V-T07/V-T08 wiring tests (P0-2) ---
//
// These checks were implemented in internal/template/validation.go but were
// only called from tests, never from the template webhook itself. Without
// the webhook wiring, an AerospikeClusterTemplate could be created with an
// enterprise image, an out-of-range size, or a malformed monitoring port,
// and silently propagate that to every cluster that referenced it.

// TestAerospikeClusterTemplateValidate_EnterpriseImageRejected pins V-T06.
func TestAerospikeClusterTemplateValidate_EnterpriseImageRejected(t *testing.T) {
	v := &AerospikeClusterTemplateValidator{}
	tmpl := &AerospikeClusterTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "ee-image", Namespace: "default"},
		Spec: AerospikeClusterTemplateSpec{
			Image: "aerospike/aerospike-server-enterprise:8.1",
		},
	}
	_, err := v.ValidateCreate(context.Background(), tmpl)
	if err == nil {
		t.Fatal("expected error for enterprise template image")
	}
	if !strings.Contains(err.Error(), "enterprise") {
		t.Errorf("expected 'enterprise' in error, got: %v", err)
	}
}

// TestAerospikeClusterTemplateValidate_SizeOutOfRangeRejected pins V-T07.
// CRD validation already enforces 1<=size<=8 but the webhook check also
// runs before CRD pruning and produces a clearer error.
func TestAerospikeClusterTemplateValidate_SizeOutOfRangeRejected(t *testing.T) {
	v := &AerospikeClusterTemplateValidator{}
	bigSize := int32(99)
	tmpl := &AerospikeClusterTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "big-size", Namespace: "default"},
		Spec: AerospikeClusterTemplateSpec{
			Size: &bigSize,
		},
	}
	_, err := v.ValidateCreate(context.Background(), tmpl)
	if err == nil {
		t.Fatal("expected error for spec.size=99")
	}
	if !strings.Contains(err.Error(), "size must be between 1 and 8") {
		t.Errorf("expected size range error, got: %v", err)
	}
}

// TestAerospikeClusterTemplateValidate_MonitoringPortOutOfRangeRejected
// pins V-T08.
func TestAerospikeClusterTemplateValidate_MonitoringPortOutOfRangeRejected(t *testing.T) {
	v := &AerospikeClusterTemplateValidator{}
	tmpl := &AerospikeClusterTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "bad-port", Namespace: "default"},
		Spec: AerospikeClusterTemplateSpec{
			Monitoring: &AerospikeMonitoringSpec{
				Enabled:       true,
				ExporterImage: "aerospike/aerospike-prometheus-exporter:1.0.0",
				Port:          70000,
			},
		},
	}
	_, err := v.ValidateCreate(context.Background(), tmpl)
	if err == nil {
		t.Fatal("expected error for monitoring.port=70000")
	}
	if !strings.Contains(err.Error(), "monitoring.port") {
		t.Errorf("expected monitoring.port error, got: %v", err)
	}
}

func TestAerospikeClusterTemplateValidateDelete(t *testing.T) {
	v := &AerospikeClusterTemplateValidator{}
	tmpl := &AerospikeClusterTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "delete-test", Namespace: "default"},
	}

	warnings, err := v.ValidateDelete(context.Background(), tmpl)
	if err != nil {
		t.Fatalf("ValidateDelete() unexpected error: %v", err)
	}
	if len(warnings) > 0 {
		t.Errorf("ValidateDelete() unexpected warnings: %v", warnings)
	}
}

// --- templateResourcesEqualRequestsLimits tests ---

func TestTemplateResourcesEqualRequestsLimits(t *testing.T) {
	tests := []struct {
		name      string
		resources *corev1.ResourceRequirements
		want      bool
	}{
		{
			name: "both requests and limits nil returns true",
			resources: &corev1.ResourceRequirements{
				Requests: nil,
				Limits:   nil,
			},
			want: true,
		},
		{
			name: "CPU equal and memory equal returns true",
			resources: &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("500m"),
					corev1.ResourceMemory: resource.MustParse("1Gi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("500m"),
					corev1.ResourceMemory: resource.MustParse("1Gi"),
				},
			},
			want: true,
		},
		{
			name: "CPU equal returns true when no memory specified",
			resources: &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("1"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("1"),
				},
			},
			want: true,
		},
		{
			name: "memory unequal returns false",
			resources: &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("1"),
					corev1.ResourceMemory: resource.MustParse("1Gi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("1"),
					corev1.ResourceMemory: resource.MustParse("2Gi"),
				},
			},
			want: false,
		},
		{
			name: "CPU unequal returns false",
			resources: &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("500m"),
					corev1.ResourceMemory: resource.MustParse("1Gi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("1"),
					corev1.ResourceMemory: resource.MustParse("1Gi"),
				},
			},
			want: false,
		},
		{
			name: "request CPU present but limit CPU missing returns false",
			resources: &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("500m"),
				},
				Limits: corev1.ResourceList{},
			},
			want: false,
		},
		{
			name: "limit memory present but request memory missing returns false",
			resources: &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{},
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("1Gi"),
				},
			},
			want: false,
		},
		{
			name: "empty resource lists returns true",
			resources: &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{},
				Limits:   corev1.ResourceList{},
			},
			want: true,
		},
		{
			name: "equivalent quantities in different units returns true",
			resources: &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("1000m"),
					corev1.ResourceMemory: resource.MustParse("1Gi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("1"),
					corev1.ResourceMemory: resource.MustParse("1073741824"), // 1Gi in bytes
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := templateResourcesEqualRequestsLimits(tt.resources)
			if got != tt.want {
				t.Errorf("templateResourcesEqualRequestsLimits() = %v, want %v", got, tt.want)
			}
		})
	}
}

// --- CE network-TLS rejection on the template path ---

// TestAerospikeClusterTemplateValidate_TLSKeysRejected mirrors the cluster
// webhook's network-TLS check on the template's own aerospikeConfig maps. A
// template's service / namespaceDefaults map is merged into every cluster that
// references it, so an Enterprise-only tls-* key there crashes the CE asd
// process on each of those clusters at once.
func TestAerospikeClusterTemplateValidate_TLSKeysRejected(t *testing.T) {
	tests := []struct {
		name     string
		config   *TemplateAerospikeConfig
		wantPath string
	}{
		{
			name: "tls-port in service",
			config: &TemplateAerospikeConfig{
				Service: &AerospikeConfigSpec{
					Value: map[string]any{"tls-port": 4333},
				},
			},
			wantPath: "spec.aerospikeConfig.service.tls-port",
		},
		{
			name: "tls-authenticate-client in service",
			config: &TemplateAerospikeConfig{
				Service: &AerospikeConfigSpec{
					Value: map[string]any{"tls-authenticate-client": "any"},
				},
			},
			wantPath: "spec.aerospikeConfig.service.tls-authenticate-client",
		},
		{
			name: "tls-name in namespaceDefaults",
			config: &TemplateAerospikeConfig{
				NamespaceDefaults: &AerospikeConfigSpec{
					Value: map[string]any{"tls-name": "aerospike-tls"},
				},
			},
			wantPath: "spec.aerospikeConfig.namespaceDefaults.tls-name",
		},
		{
			// These maps are PreserveUnknownFields, so a nested network map is
			// checked rather than assumed impossible.
			name: "nested network.tls in service",
			config: &TemplateAerospikeConfig{
				Service: &AerospikeConfigSpec{
					Value: map[string]any{
						"network": map[string]any{
							"tls": map[string]any{
								"aerospike-tls": map[string]any{"cert-file": "/cert.pem"},
							},
						},
					},
				},
			},
			wantPath: "spec.aerospikeConfig.service.network must not contain 'tls' section",
		},
		{
			name: "nested network.heartbeat.tls-port in service",
			config: &TemplateAerospikeConfig{
				Service: &AerospikeConfigSpec{
					Value: map[string]any{
						"network": map[string]any{
							"heartbeat": map[string]any{"tls-port": 3012},
						},
					},
				},
			},
			wantPath: "spec.aerospikeConfig.service.network.heartbeat.tls-port",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v := &AerospikeClusterTemplateValidator{}
			tmpl := &AerospikeClusterTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "tls-tmpl", Namespace: "default"},
				Spec: AerospikeClusterTemplateSpec{
					AerospikeConfig: tc.config,
				},
			}

			_, err := v.ValidateCreate(context.Background(), tmpl)
			if err == nil {
				t.Fatalf("ValidateCreate() = nil, want error for %s", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantPath) {
				t.Errorf("error must name %q, got: %v", tc.wantPath, err)
			}
			if !strings.Contains(err.Error(), "TLS is Enterprise-only") {
				t.Errorf("error must name the Enterprise feature, got: %v", err)
			}
		})
	}
}

// TestAerospikeClusterTemplateValidate_TLSKeysRejectedOnUpdate pins that the
// check runs on UPDATE too — editing a live template re-bases every cluster
// that references it.
func TestAerospikeClusterTemplateValidate_TLSKeysRejectedOnUpdate(t *testing.T) {
	v := &AerospikeClusterTemplateValidator{}
	oldTmpl := &AerospikeClusterTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "tls-tmpl", Namespace: "default"},
		Spec: AerospikeClusterTemplateSpec{
			AerospikeConfig: &TemplateAerospikeConfig{
				Service: &AerospikeConfigSpec{
					Value: map[string]any{"proto-fd-max": 15000},
				},
			},
		},
	}
	newTmpl := oldTmpl.DeepCopy()
	newTmpl.Spec.AerospikeConfig.Service.Value["tls-port"] = 4333

	_, err := v.ValidateUpdate(context.Background(), oldTmpl, newTmpl)
	if err == nil {
		t.Fatal("ValidateUpdate() = nil, want error for tls-port added on update")
	}
	if !strings.Contains(err.Error(), "spec.aerospikeConfig.service.tls-port") {
		t.Errorf("expected the offending key path in the error, got: %v", err)
	}
}

// TestAerospikeClusterTemplateValidate_NonTLSKeysAccepted is the
// over-rejection guard: ordinary CE template defaults must still pass.
func TestAerospikeClusterTemplateValidate_NonTLSKeysAccepted(t *testing.T) {
	v := &AerospikeClusterTemplateValidator{}
	tmpl := &AerospikeClusterTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "plain-tmpl", Namespace: "default"},
		Spec: AerospikeClusterTemplateSpec{
			AerospikeConfig: &TemplateAerospikeConfig{
				Service: &AerospikeConfigSpec{
					Value: map[string]any{
						"proto-fd-max":    15000,
						"enable-security": false,
					},
				},
				NamespaceDefaults: &AerospikeConfigSpec{
					Value: map[string]any{
						"replication-factor": 2,
						"default-ttl":        "30d",
					},
				},
			},
		},
	}

	if _, err := v.ValidateCreate(context.Background(), tmpl); err != nil {
		t.Errorf("ValidateCreate() = %v, want nil for plain CE template defaults", err)
	}
}
