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
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	ackov1alpha1 "github.com/ksr/aerospike-ce-kubernetes-operator/api/v1alpha1"
)

func TestMergeTemplateSpec_NilInputs(t *testing.T) {
	result := MergeTemplateSpec(nil, nil)
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestMergeTemplateSpec_NilBase(t *testing.T) {
	override := &ackov1alpha1.AerospikeClusterTemplateSpec{
		RackConfig: &ackov1alpha1.TemplateRackConfig{MaxRacksPerNode: 2},
	}
	result := MergeTemplateSpec(nil, override)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.RackConfig == nil || result.RackConfig.MaxRacksPerNode != 2 {
		t.Errorf("expected MaxRacksPerNode=2, got %+v", result.RackConfig)
	}
}

func TestMergeTemplateSpec_NilOverride(t *testing.T) {
	base := &ackov1alpha1.AerospikeClusterTemplateSpec{
		RackConfig: &ackov1alpha1.TemplateRackConfig{MaxRacksPerNode: 1},
	}
	result := MergeTemplateSpec(base, nil)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.RackConfig == nil || result.RackConfig.MaxRacksPerNode != 1 {
		t.Errorf("expected MaxRacksPerNode=1, got %+v", result.RackConfig)
	}
}

func TestMergeTemplateSpec_OverrideTakesPrecedence(t *testing.T) {
	base := &ackov1alpha1.AerospikeClusterTemplateSpec{
		RackConfig: &ackov1alpha1.TemplateRackConfig{MaxRacksPerNode: 1},
		Scheduling: &ackov1alpha1.TemplateScheduling{
			PodAntiAffinityLevel: ackov1alpha1.PodAntiAffinityPreferred,
		},
	}
	override := &ackov1alpha1.AerospikeClusterTemplateSpec{
		Scheduling: &ackov1alpha1.TemplateScheduling{
			PodAntiAffinityLevel: ackov1alpha1.PodAntiAffinityRequired,
		},
	}
	result := MergeTemplateSpec(base, override)

	if result.RackConfig == nil || result.RackConfig.MaxRacksPerNode != 1 {
		t.Errorf("expected base RackConfig to be preserved")
	}
	if result.Scheduling == nil || result.Scheduling.PodAntiAffinityLevel != ackov1alpha1.PodAntiAffinityRequired {
		t.Errorf("expected override scheduling to take precedence")
	}
}

func TestMergeTemplateSpec_AerospikeConfigDeepMerge(t *testing.T) {
	base := &ackov1alpha1.AerospikeClusterTemplateSpec{
		AerospikeConfig: &ackov1alpha1.TemplateAerospikeConfig{
			Service: &ackov1alpha1.AerospikeConfigSpec{
				Value: map[string]any{
					"proto-fd-max": 15000,
					"base-key":     "base-value",
				},
			},
		},
	}
	override := &ackov1alpha1.AerospikeClusterTemplateSpec{
		AerospikeConfig: &ackov1alpha1.TemplateAerospikeConfig{
			Service: &ackov1alpha1.AerospikeConfigSpec{
				Value: map[string]any{
					"proto-fd-max": 20000,
					"override-key": "override-value",
				},
			},
		},
	}
	result := MergeTemplateSpec(base, override)

	if result.AerospikeConfig == nil || result.AerospikeConfig.Service == nil {
		t.Fatal("expected AerospikeConfig.Service to be set")
	}
	// override-key should be present
	if result.AerospikeConfig.Service.Value["override-key"] != "override-value" {
		t.Errorf("override-key missing from merged service")
	}
	// base-key should be present (from base, not overridden)
	if result.AerospikeConfig.Service.Value["base-key"] != "base-value" {
		t.Errorf("base-key should be preserved in merged service")
	}
	// override value should win
	if result.AerospikeConfig.Service.Value["proto-fd-max"] != 20000 {
		t.Errorf("expected proto-fd-max=20000, got %v", result.AerospikeConfig.Service.Value["proto-fd-max"])
	}
}

func TestApplyAerospikeConfig_NamespaceDefaults(t *testing.T) {
	cluster := &ackov1alpha1.AerospikeCluster{}
	cluster.Spec.AerospikeConfig = &ackov1alpha1.AerospikeConfigSpec{
		Value: map[string]any{
			"namespaces": []any{
				map[string]any{"name": "test", "replication-factor": 1},
			},
		},
	}

	tmplConfig := &ackov1alpha1.TemplateAerospikeConfig{
		NamespaceDefaults: &ackov1alpha1.AerospikeConfigSpec{
			Value: map[string]any{
				"memory-size":        int64(1073741824),
				"replication-factor": 2, // should be overridden by namespace's own value
			},
		},
	}

	if err := applyAerospikeConfig(tmplConfig, cluster); err != nil {
		t.Fatalf("applyAerospikeConfig() unexpected error: %v", err)
	}

	nsList, ok := cluster.Spec.AerospikeConfig.Value["namespaces"].([]any)
	if !ok || len(nsList) == 0 {
		t.Fatal("expected namespaces to be set")
	}
	nsMap, ok := nsList[0].(map[string]any)
	if !ok {
		t.Fatal("expected namespace to be a map")
	}

	// memory-size from defaults should be applied
	if nsMap["memory-size"] != int64(1073741824) {
		t.Errorf("expected memory-size to be applied from defaults, got %v", nsMap["memory-size"])
	}
	// replication-factor should keep the namespace's own value (1), not the default (2)
	if nsMap["replication-factor"] != 1 {
		t.Errorf("expected replication-factor=1 (from namespace), got %v", nsMap["replication-factor"])
	}
}

func TestNeedsResync(t *testing.T) {
	tests := []struct {
		name    string
		cluster *ackov1alpha1.AerospikeCluster
		want    bool
	}{
		{
			name:    "no templateRef",
			cluster: &ackov1alpha1.AerospikeCluster{},
			want:    false,
		},
		{
			name: "templateRef set, no snapshot",
			cluster: &ackov1alpha1.AerospikeCluster{
				Spec: ackov1alpha1.AerospikeClusterSpec{
					TemplateRef: &ackov1alpha1.TemplateRef{Name: "prod"},
				},
			},
			want: true,
		},
		{
			name: "templateRef set, snapshot exists, no annotation",
			cluster: &ackov1alpha1.AerospikeCluster{
				Spec: ackov1alpha1.AerospikeClusterSpec{
					TemplateRef: &ackov1alpha1.TemplateRef{Name: "prod"},
				},
				Status: ackov1alpha1.AerospikeClusterStatus{
					TemplateSnapshot: &ackov1alpha1.TemplateSnapshotStatus{Name: "prod"},
				},
			},
			want: false,
		},
		{
			name: "templateRef set, snapshot exists, resync annotation",
			cluster: &ackov1alpha1.AerospikeCluster{
				Spec: ackov1alpha1.AerospikeClusterSpec{
					TemplateRef: &ackov1alpha1.TemplateRef{Name: "prod"},
				},
				Status: ackov1alpha1.AerospikeClusterStatus{
					TemplateSnapshot: &ackov1alpha1.TemplateSnapshotStatus{Name: "prod"},
				},
			},
			want: true,
		},
	}

	// Add annotation to the last test case
	tests[3].cluster.Annotations = map[string]string{AnnotationResyncTemplate: "true"}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NeedsResync(tt.cluster)
			if got != tt.want {
				t.Errorf("NeedsResync() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestApplyTemplate_Resources(t *testing.T) {
	cluster := &ackov1alpha1.AerospikeCluster{
		Spec: ackov1alpha1.AerospikeClusterSpec{
			Size:  1,
			Image: testImageCE8,
		},
	}

	// Resources should be applied when not set in cluster spec.
	templateSpec := &ackov1alpha1.AerospikeClusterTemplateSpec{}
	if err := ApplyTemplate(templateSpec, cluster); err != nil {
		t.Fatalf("ApplyTemplate() unexpected error: %v", err)
	}

	// With nil resources in template, nothing should be set.
	if cluster.Spec.PodSpec != nil && cluster.Spec.PodSpec.AerospikeContainerSpec != nil &&
		cluster.Spec.PodSpec.AerospikeContainerSpec.Resources != nil {
		t.Errorf("expected no resources to be set when template has none")
	}
}

// ---- applyStorage tests ----

func TestApplyStorage_CreatesVolumeFromTemplate(t *testing.T) {
	cluster := &ackov1alpha1.AerospikeCluster{}
	qty := resource.MustParse("50Gi")
	tmplStorage := &ackov1alpha1.TemplateStorage{
		StorageClassName: "local-path",
		Resources: corev1.VolumeResourceRequirements{
			Requests: corev1.ResourceList{corev1.ResourceStorage: qty},
		},
	}

	applyStorage(tmplStorage, cluster)

	if cluster.Spec.Storage == nil || len(cluster.Spec.Storage.Volumes) == 0 {
		t.Fatal("expected storage volume to be created")
	}
	vol := cluster.Spec.Storage.Volumes[0]
	if vol.Name != defaultDataVolumeName {
		t.Errorf("expected volume name %q, got %q", defaultDataVolumeName, vol.Name)
	}
	if vol.Source.PersistentVolume == nil {
		t.Fatal("expected PersistentVolume to be set")
	}
	if vol.Source.PersistentVolume.StorageClass != "local-path" {
		t.Errorf("expected storageClass=local-path, got %q", vol.Source.PersistentVolume.StorageClass)
	}
	if vol.Source.PersistentVolume.Size != "50Gi" {
		t.Errorf("expected size=50Gi, got %q", vol.Source.PersistentVolume.Size)
	}
}

func TestApplyStorage_SkipsIfVolumeAlreadySet(t *testing.T) {
	cluster := &ackov1alpha1.AerospikeCluster{
		Spec: ackov1alpha1.AerospikeClusterSpec{
			Storage: &ackov1alpha1.AerospikeStorageSpec{
				Volumes: []ackov1alpha1.VolumeSpec{
					{Name: "existing"},
				},
			},
		},
	}
	tmplStorage := &ackov1alpha1.TemplateStorage{StorageClassName: "other"}

	applyStorage(tmplStorage, cluster)

	if len(cluster.Spec.Storage.Volumes) != 1 || cluster.Spec.Storage.Volumes[0].Name != "existing" {
		t.Error("expected existing volume to be preserved when cluster already has volumes")
	}
}

func TestApplyStorage_DefaultsApplied(t *testing.T) {
	cluster := &ackov1alpha1.AerospikeCluster{}
	tmplStorage := &ackov1alpha1.TemplateStorage{StorageClassName: "standard"}

	applyStorage(tmplStorage, cluster)

	if cluster.Spec.Storage == nil || len(cluster.Spec.Storage.Volumes) == 0 {
		t.Fatal("expected volume to be created")
	}
	pv := cluster.Spec.Storage.Volumes[0].Source.PersistentVolume
	if pv.Size != "1Gi" {
		t.Errorf("expected default size=1Gi, got %q", pv.Size)
	}
	if pv.VolumeMode != corev1.PersistentVolumeFilesystem {
		t.Errorf("expected default volumeMode=Filesystem, got %v", pv.VolumeMode)
	}
	if len(pv.AccessModes) != 1 || pv.AccessModes[0] != corev1.ReadWriteOnce {
		t.Errorf("expected default accessModes=[ReadWriteOnce], got %v", pv.AccessModes)
	}
}

// ---- applyScheduling tests ----

func TestApplyScheduling_Tolerations(t *testing.T) {
	cluster := &ackov1alpha1.AerospikeCluster{}
	scheduling := &ackov1alpha1.TemplateScheduling{
		Tolerations: []corev1.Toleration{
			{Key: "aerospike", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule},
		},
	}

	applyScheduling(scheduling, cluster)

	if cluster.Spec.PodSpec == nil || len(cluster.Spec.PodSpec.Tolerations) == 0 {
		t.Fatal("expected tolerations to be applied")
	}
	if cluster.Spec.PodSpec.Tolerations[0].Key != "aerospike" {
		t.Errorf("expected toleration key=aerospike, got %q", cluster.Spec.PodSpec.Tolerations[0].Key)
	}
}

func TestApplyScheduling_TolerationsNotOverriddenIfAlreadySet(t *testing.T) {
	cluster := &ackov1alpha1.AerospikeCluster{
		Spec: ackov1alpha1.AerospikeClusterSpec{
			PodSpec: &ackov1alpha1.AerospikePodSpec{
				Tolerations: []corev1.Toleration{{Key: "existing"}},
			},
		},
	}
	scheduling := &ackov1alpha1.TemplateScheduling{
		Tolerations: []corev1.Toleration{{Key: "from-template"}},
	}

	applyScheduling(scheduling, cluster)

	if cluster.Spec.PodSpec.Tolerations[0].Key != "existing" {
		t.Error("existing tolerations should not be overridden by template")
	}
}

func TestApplyScheduling_TopologySpreadConstraints(t *testing.T) {
	cluster := &ackov1alpha1.AerospikeCluster{}
	scheduling := &ackov1alpha1.TemplateScheduling{
		TopologySpreadConstraints: []corev1.TopologySpreadConstraint{
			{MaxSkew: 1, TopologyKey: testTopologyZone, WhenUnsatisfiable: corev1.DoNotSchedule},
		},
	}

	applyScheduling(scheduling, cluster)

	if cluster.Spec.PodSpec == nil || len(cluster.Spec.PodSpec.TopologySpreadConstraints) == 0 {
		t.Fatal("expected TopologySpreadConstraints to be applied")
	}
	if cluster.Spec.PodSpec.TopologySpreadConstraints[0].TopologyKey != testTopologyZone {
		t.Errorf("expected topologyKey=zone, got %q", cluster.Spec.PodSpec.TopologySpreadConstraints[0].TopologyKey)
	}
}

func TestApplyScheduling_PodManagementPolicy(t *testing.T) {
	cluster := &ackov1alpha1.AerospikeCluster{}
	scheduling := &ackov1alpha1.TemplateScheduling{
		PodManagementPolicy: appsv1.OrderedReadyPodManagement,
	}

	applyScheduling(scheduling, cluster)

	if cluster.Spec.PodSpec == nil || cluster.Spec.PodSpec.PodManagementPolicy != appsv1.OrderedReadyPodManagement {
		t.Errorf("expected PodManagementPolicy=OrderedReady, got %v", cluster.Spec.PodSpec.PodManagementPolicy)
	}
}

func TestDeepMergeMapBaseFirst(t *testing.T) {
	base := map[string]any{
		"key1": "base-value",
		"key2": "base-only",
		"nested": map[string]any{
			"a": 1,
			"b": "base",
		},
	}
	override := map[string]any{
		"key1": "override-value",
		"key3": "override-only",
		"nested": map[string]any{
			"a": 99,
			"c": "override",
		},
	}

	result := deepMergeMapBaseFirst(base, override)

	if result["key1"] != "override-value" {
		t.Errorf("override should win for key1")
	}
	if result["key2"] != "base-only" {
		t.Errorf("base-only key should be preserved")
	}
	if result["key3"] != "override-only" {
		t.Errorf("override-only key should be present")
	}

	nested, ok := result["nested"].(map[string]any)
	if !ok {
		t.Fatal("nested should be a map")
	}
	if nested["a"] != 99 {
		t.Errorf("nested.a should be overridden to 99")
	}
	if nested["b"] != "base" {
		t.Errorf("nested.b should be preserved from base")
	}
	if nested["c"] != "override" {
		t.Errorf("nested.c should be present from override")
	}
}

// --- MergeTemplateSpec: deep copy isolation ---

func TestMergeTemplateSpec_AerospikeConfigIsolatedFromBase(t *testing.T) {
	base := &ackov1alpha1.AerospikeClusterTemplateSpec{
		AerospikeConfig: &ackov1alpha1.TemplateAerospikeConfig{
			NamespaceDefaults: &ackov1alpha1.AerospikeConfigSpec{
				Value: map[string]any{"replication-factor": 2},
			},
		},
	}
	// override touches Service but NOT NamespaceDefaults
	override := &ackov1alpha1.AerospikeClusterTemplateSpec{
		AerospikeConfig: &ackov1alpha1.TemplateAerospikeConfig{
			Service: &ackov1alpha1.AerospikeConfigSpec{
				Value: map[string]any{"proto-fd-max": 20000},
			},
		},
	}
	result := MergeTemplateSpec(base, override)

	// Mutating the merged result's NamespaceDefaults must NOT affect the base.
	result.AerospikeConfig.NamespaceDefaults.Value["replication-factor"] = 99
	if base.AerospikeConfig.NamespaceDefaults.Value["replication-factor"] != 2 {
		t.Error("mutating merge result must not affect the original base (shallow copy bug)")
	}
}

func TestMergeTemplateSpec_NetworkConfigIsolatedFromBase(t *testing.T) {
	base := &ackov1alpha1.AerospikeClusterTemplateSpec{
		AerospikeConfig: &ackov1alpha1.TemplateAerospikeConfig{
			Network: &ackov1alpha1.TemplateNetworkConfig{
				Heartbeat: &ackov1alpha1.TemplateHeartbeatConfig{
					Mode:     "mesh",
					Interval: 150,
					Timeout:  10,
				},
			},
		},
	}
	// override touches heartbeat partially
	override := &ackov1alpha1.AerospikeClusterTemplateSpec{
		AerospikeConfig: &ackov1alpha1.TemplateAerospikeConfig{
			Network: &ackov1alpha1.TemplateNetworkConfig{
				Heartbeat: &ackov1alpha1.TemplateHeartbeatConfig{
					Interval: 250,
				},
			},
		},
	}
	result := MergeTemplateSpec(base, override)

	if result.AerospikeConfig.Network.Heartbeat.Interval != 250 {
		t.Errorf("expected override interval=250, got %d", result.AerospikeConfig.Network.Heartbeat.Interval)
	}
	if result.AerospikeConfig.Network.Heartbeat.Mode != "mesh" {
		t.Errorf("expected base mode=mesh preserved, got %q", result.AerospikeConfig.Network.Heartbeat.Mode)
	}
}

func TestMergeTemplateSpec_SchedulingIsolatedFromBase(t *testing.T) {
	base := &ackov1alpha1.AerospikeClusterTemplateSpec{
		Scheduling: &ackov1alpha1.TemplateScheduling{
			PodAntiAffinityLevel: ackov1alpha1.PodAntiAffinityPreferred,
			Tolerations: []corev1.Toleration{
				{Key: "base-key", Operator: corev1.TolerationOpExists},
			},
		},
	}
	// override touches PodAntiAffinityLevel but NOT Tolerations
	override := &ackov1alpha1.AerospikeClusterTemplateSpec{
		Scheduling: &ackov1alpha1.TemplateScheduling{
			PodAntiAffinityLevel: ackov1alpha1.PodAntiAffinityRequired,
		},
	}
	result := MergeTemplateSpec(base, override)

	// Mutating result's Tolerations must NOT affect base.
	result.Scheduling.Tolerations[0].Key = testMutatedValue
	if base.Scheduling.Tolerations[0].Key != "base-key" {
		t.Error("mutating merge result must not affect the original base (shallow copy bug)")
	}
}

func TestMergeTemplateSpec_NodeAffinityIsolatedFromOverride(t *testing.T) {
	base := &ackov1alpha1.AerospikeClusterTemplateSpec{
		Scheduling: &ackov1alpha1.TemplateScheduling{
			PodAntiAffinityLevel: ackov1alpha1.PodAntiAffinityPreferred,
		},
	}
	override := &ackov1alpha1.AerospikeClusterTemplateSpec{
		Scheduling: &ackov1alpha1.TemplateScheduling{
			NodeAffinity: &corev1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{
						{MatchExpressions: []corev1.NodeSelectorRequirement{
							{Key: "zone", Operator: corev1.NodeSelectorOpIn, Values: []string{"us-east-1a"}},
						}},
					},
				},
			},
		},
	}
	result := MergeTemplateSpec(base, override)

	result.Scheduling.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.
		NodeSelectorTerms[0].MatchExpressions[0].Values[0] = testMutatedValue
	if override.Scheduling.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.
		NodeSelectorTerms[0].MatchExpressions[0].Values[0] != "us-east-1a" {
		t.Error("mutating merge result must not affect the override (shallow copy bug in NodeAffinity)")
	}
}

func TestMergeTemplateSpec_TolerationsIsolatedFromOverride(t *testing.T) {
	base := &ackov1alpha1.AerospikeClusterTemplateSpec{}
	override := &ackov1alpha1.AerospikeClusterTemplateSpec{
		Scheduling: &ackov1alpha1.TemplateScheduling{
			Tolerations: []corev1.Toleration{
				{Key: "original", Operator: corev1.TolerationOpExists},
			},
		},
	}
	result := MergeTemplateSpec(base, override)

	result.Scheduling.Tolerations[0].Key = testMutatedValue
	if override.Scheduling.Tolerations[0].Key != "original" {
		t.Error("mutating merge result must not affect the override (shallow copy bug in Tolerations)")
	}
}

func TestMergeTemplateSpec_TopologySpreadConstraintsIsolatedFromOverride(t *testing.T) {
	base := &ackov1alpha1.AerospikeClusterTemplateSpec{}
	override := &ackov1alpha1.AerospikeClusterTemplateSpec{
		Scheduling: &ackov1alpha1.TemplateScheduling{
			TopologySpreadConstraints: []corev1.TopologySpreadConstraint{
				{MaxSkew: 1, TopologyKey: testTopologyZone, WhenUnsatisfiable: corev1.DoNotSchedule},
			},
		},
	}
	result := MergeTemplateSpec(base, override)

	result.Scheduling.TopologySpreadConstraints[0].TopologyKey = testMutatedValue
	if override.Scheduling.TopologySpreadConstraints[0].TopologyKey != testTopologyZone {
		t.Error("mutating merge result must not affect the override (shallow copy bug in TopologySpreadConstraints)")
	}
}

// --- MergeTemplateSpec: Image ---

func TestMergeTemplateSpec_ImageOverrideTakesPrecedence(t *testing.T) {
	base := &ackov1alpha1.AerospikeClusterTemplateSpec{Image: testImageCE8Old}
	override := &ackov1alpha1.AerospikeClusterTemplateSpec{Image: testImageCE8}
	result := MergeTemplateSpec(base, override)
	if result.Image != testImageCE8 {
		t.Errorf("expected override image, got %q", result.Image)
	}
}

func TestMergeTemplateSpec_ImageBasePreservedWhenOverrideEmpty(t *testing.T) {
	base := &ackov1alpha1.AerospikeClusterTemplateSpec{Image: testImageCE8}
	override := &ackov1alpha1.AerospikeClusterTemplateSpec{}
	result := MergeTemplateSpec(base, override)
	if result.Image != testImageCE8 {
		t.Errorf("expected base image to be preserved, got %q", result.Image)
	}
}

// --- MergeTemplateSpec: Size ---

func TestMergeTemplateSpec_SizeOverrideTakesPrecedence(t *testing.T) {
	baseSize := int32(1)
	overrideSize := int32(6)
	base := &ackov1alpha1.AerospikeClusterTemplateSpec{Size: &baseSize}
	override := &ackov1alpha1.AerospikeClusterTemplateSpec{Size: &overrideSize}
	result := MergeTemplateSpec(base, override)
	if result.Size == nil || *result.Size != 6 {
		t.Errorf("expected override size=6")
	}
}

func TestMergeTemplateSpec_SizeBasePreservedWhenOverrideNil(t *testing.T) {
	baseSize := int32(6)
	base := &ackov1alpha1.AerospikeClusterTemplateSpec{Size: &baseSize}
	override := &ackov1alpha1.AerospikeClusterTemplateSpec{}
	result := MergeTemplateSpec(base, override)
	if result.Size == nil || *result.Size != 6 {
		t.Errorf("expected base size=6 to be preserved")
	}
}

func TestMergeTemplateSpec_SizeDeepCopied(t *testing.T) {
	baseSize := int32(3)
	base := &ackov1alpha1.AerospikeClusterTemplateSpec{Size: &baseSize}
	result := MergeTemplateSpec(base, nil)
	// Mutating baseSize should not affect the result.
	baseSize = 99
	if result.Size == nil || *result.Size != 3 {
		t.Errorf("expected deep copy: result size should remain 3")
	}
}

// --- MergeTemplateSpec: Monitoring ---

func TestMergeTemplateSpec_MonitoringOverrideReplaces(t *testing.T) {
	base := &ackov1alpha1.AerospikeClusterTemplateSpec{
		Monitoring: &ackov1alpha1.AerospikeMonitoringSpec{Port: 9145},
	}
	override := &ackov1alpha1.AerospikeClusterTemplateSpec{
		Monitoring: &ackov1alpha1.AerospikeMonitoringSpec{Port: 9200},
	}
	result := MergeTemplateSpec(base, override)
	if result.Monitoring == nil || result.Monitoring.Port != 9200 {
		t.Errorf("expected override monitoring port=9200")
	}
}

func TestMergeTemplateSpec_MonitoringBasePreservedWhenOverrideNil(t *testing.T) {
	base := &ackov1alpha1.AerospikeClusterTemplateSpec{
		Monitoring: &ackov1alpha1.AerospikeMonitoringSpec{Enabled: true, Port: 9145},
	}
	override := &ackov1alpha1.AerospikeClusterTemplateSpec{}
	result := MergeTemplateSpec(base, override)
	if result.Monitoring == nil || !result.Monitoring.Enabled || result.Monitoring.Port != 9145 {
		t.Errorf("expected base monitoring to be preserved")
	}
}

// --- MergeTemplateSpec: AerospikeNetworkPolicy ---

func TestMergeTemplateSpec_NetworkPolicyOverrideReplaces(t *testing.T) {
	base := &ackov1alpha1.AerospikeClusterTemplateSpec{
		AerospikeNetworkPolicy: &ackov1alpha1.AerospikeNetworkPolicy{
			AccessType: ackov1alpha1.AerospikeNetworkTypePod,
		},
	}
	override := &ackov1alpha1.AerospikeClusterTemplateSpec{
		AerospikeNetworkPolicy: &ackov1alpha1.AerospikeNetworkPolicy{
			AccessType: ackov1alpha1.AerospikeNetworkTypeHostExternal,
		},
	}
	result := MergeTemplateSpec(base, override)
	if result.AerospikeNetworkPolicy == nil || result.AerospikeNetworkPolicy.AccessType != ackov1alpha1.AerospikeNetworkTypeHostExternal {
		t.Errorf("expected override network policy access type=hostExternal")
	}
}

func TestMergeTemplateSpec_NetworkPolicyBasePreservedWhenOverrideNil(t *testing.T) {
	base := &ackov1alpha1.AerospikeClusterTemplateSpec{
		AerospikeNetworkPolicy: &ackov1alpha1.AerospikeNetworkPolicy{
			AccessType: ackov1alpha1.AerospikeNetworkTypePod,
		},
	}
	override := &ackov1alpha1.AerospikeClusterTemplateSpec{}
	result := MergeTemplateSpec(base, override)
	if result.AerospikeNetworkPolicy == nil || result.AerospikeNetworkPolicy.AccessType != ackov1alpha1.AerospikeNetworkTypePod {
		t.Errorf("expected base network policy to be preserved")
	}
}

// --- ApplyTemplate integration: image and size ---

func TestApplyTemplate_ImageAndSizeApplied(t *testing.T) {
	size := int32(3)
	tmplSpec := &ackov1alpha1.AerospikeClusterTemplateSpec{
		Image: testImageCE8,
		Size:  &size,
	}
	cluster := newCluster()
	if err := ApplyTemplate(tmplSpec, cluster); err != nil {
		t.Fatalf("ApplyTemplate() unexpected error: %v", err)
	}

	if cluster.Spec.Image != testImageCE8 {
		t.Errorf("expected image to be applied, got %q", cluster.Spec.Image)
	}
	if cluster.Spec.Size != 3 {
		t.Errorf("expected size=3, got %d", cluster.Spec.Size)
	}
}

func TestApplyTemplate_ClusterValuesTakePrecedenceOverTemplate(t *testing.T) {
	size := int32(6)
	tmplSpec := &ackov1alpha1.AerospikeClusterTemplateSpec{
		Image: testImageCE8,
		Size:  &size,
		Monitoring: &ackov1alpha1.AerospikeMonitoringSpec{
			Port: 9145,
		},
		AerospikeNetworkPolicy: &ackov1alpha1.AerospikeNetworkPolicy{
			AccessType: ackov1alpha1.AerospikeNetworkTypePod,
		},
	}
	cluster := newCluster()
	cluster.Spec.Image = testImageCE8Old
	cluster.Spec.Size = 1
	cluster.Spec.Monitoring = &ackov1alpha1.AerospikeMonitoringSpec{Port: 9200}
	cluster.Spec.AerospikeNetworkPolicy = &ackov1alpha1.AerospikeNetworkPolicy{
		AccessType: ackov1alpha1.AerospikeNetworkTypeHostInternal,
	}

	if err := ApplyTemplate(tmplSpec, cluster); err != nil {
		t.Fatalf("ApplyTemplate() unexpected error: %v", err)
	}

	if cluster.Spec.Image != testImageCE8Old {
		t.Errorf("expected cluster image to be preserved, got %q", cluster.Spec.Image)
	}
	if cluster.Spec.Size != 1 {
		t.Errorf("expected cluster size to be preserved (1), got %d", cluster.Spec.Size)
	}
	if cluster.Spec.Monitoring.Port != 9200 {
		t.Errorf("expected cluster monitoring port to be preserved (9200), got %d", cluster.Spec.Monitoring.Port)
	}
	if cluster.Spec.AerospikeNetworkPolicy.AccessType != ackov1alpha1.AerospikeNetworkTypeHostInternal {
		t.Errorf("expected cluster network policy to be preserved")
	}
}

// --- post-merge CE re-validation tests (P0-3) ---

// TestValidateMergedSpecCE_RejectsEnterpriseImage covers the defence-in-depth
// re-validation that runs after ApplyTemplate. Even if a CE constraint were
// bypassed at admission time (failurePolicy=Ignore, future code path that
// produces an enterprise tag, ...), the resolver must refuse to materialise
// an Enterprise image into the cluster spec.
func TestValidateMergedSpecCE_RejectsEnterpriseImage(t *testing.T) {
	spec := &ackov1alpha1.AerospikeClusterSpec{
		Size:  3,
		Image: "aerospike/aerospike-server-enterprise:8.1",
	}
	errs := validateMergedSpecCE(spec)
	if len(errs) == 0 {
		t.Fatal("expected error for enterprise image in merged spec")
	}
}

// TestValidateMergedSpecCE_RejectsOversizedCluster pins the post-merge size
// check (defence in depth — also enforced at admission).
func TestValidateMergedSpecCE_RejectsOversizedCluster(t *testing.T) {
	spec := &ackov1alpha1.AerospikeClusterSpec{
		Size:  99,
		Image: testImageCE8,
	}
	errs := validateMergedSpecCE(spec)
	if len(errs) == 0 {
		t.Fatal("expected error for size=99 in merged spec")
	}
}

// TestValidateMergedSpecCE_RejectsXdrInConfig pins that an xdr stanza in the
// merged aerospikeConfig is rejected by the post-merge validator.
func TestValidateMergedSpecCE_RejectsXdrInConfig(t *testing.T) {
	spec := &ackov1alpha1.AerospikeClusterSpec{
		Size:  3,
		Image: testImageCE8,
		AerospikeConfig: &ackov1alpha1.AerospikeConfigSpec{
			Value: map[string]any{
				"xdr": map[string]any{},
			},
		},
	}
	errs := validateMergedSpecCE(spec)
	if len(errs) == 0 {
		t.Fatal("expected error for xdr in merged config")
	}
}

// TestValidateMergedSpecCE_RejectsEnterpriseNamespaceKey covers the per-namespace
// enterprise-only key check on the merged namespaces list. Without this, a
// strong-consistency value reaching merged config (via overrides ->
// namespaceDefaults -> applyNamespaceDefaults) would slip past per-namespace
// checks.
func TestValidateMergedSpecCE_RejectsEnterpriseNamespaceKey(t *testing.T) {
	spec := &ackov1alpha1.AerospikeClusterSpec{
		Size:  3,
		Image: testImageCE8,
		AerospikeConfig: &ackov1alpha1.AerospikeConfigSpec{
			Value: map[string]any{
				"namespaces": []any{
					map[string]any{
						"name":               "test",
						"strong-consistency": true,
					},
				},
			},
		},
	}
	errs := validateMergedSpecCE(spec)
	if len(errs) == 0 {
		t.Fatal("expected error for strong-consistency in merged namespace")
	}
}

// TestValidateMergedSpecCE_AcceptsValidCESpec ensures the post-merge check
// does not over-reject a normal CE configuration.
func TestValidateMergedSpecCE_AcceptsValidCESpec(t *testing.T) {
	spec := &ackov1alpha1.AerospikeClusterSpec{
		Size:  3,
		Image: testImageCE8,
		AerospikeConfig: &ackov1alpha1.AerospikeConfigSpec{
			Value: map[string]any{
				"namespaces": []any{
					map[string]any{
						"name":               "test",
						"replication-factor": 1,
					},
				},
			},
		},
	}
	if errs := validateMergedSpecCE(spec); len(errs) > 0 {
		t.Errorf("unexpected errors for valid CE spec: %v", errs)
	}
}

// --- applyAerospikeConfig non-map service guard (P1-3) ---

// TestApplyAerospikeConfig_RefusesNonMapService pins the defensive type
// check: if cluster.Spec.AerospikeConfig.service is somehow not a map (a
// future bypass of the cluster webhook), applyAerospikeConfig must error
// rather than silently overwrite the user's value with template defaults.
func TestApplyAerospikeConfig_RefusesNonMapService(t *testing.T) {
	cluster := &ackov1alpha1.AerospikeCluster{}
	cluster.Spec.AerospikeConfig = &ackov1alpha1.AerospikeConfigSpec{
		Value: map[string]any{
			"service": "this-should-be-a-map",
		},
	}
	tmplConfig := &ackov1alpha1.TemplateAerospikeConfig{
		Service: &ackov1alpha1.AerospikeConfigSpec{
			Value: map[string]any{"proto-fd-max": 25000},
		},
	}

	err := applyAerospikeConfig(tmplConfig, cluster)
	if err == nil {
		t.Fatal("expected error for non-map service value")
	}

	// Make sure we did NOT overwrite the user's payload with the template merge.
	if got, ok := cluster.Spec.AerospikeConfig.Value["service"].(string); !ok || got != "this-should-be-a-map" {
		t.Errorf("expected original service value preserved, got %v", cluster.Spec.AerospikeConfig.Value["service"])
	}
}
