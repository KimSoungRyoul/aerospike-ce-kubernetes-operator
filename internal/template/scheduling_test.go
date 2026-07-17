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

	corev1 "k8s.io/api/core/v1"

	ackov1alpha1 "github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/api/v1alpha1"
)

func TestTranslatePodAntiAffinity(t *testing.T) {
	tests := []struct {
		name        string
		level       ackov1alpha1.PodAntiAffinityLevel
		clusterName string
		wantNil     bool
		wantType    string // "required" or "preferred"
	}{
		{
			name:    "empty level returns nil",
			level:   "",
			wantNil: true,
		},
		{
			name:    "none returns nil",
			level:   ackov1alpha1.PodAntiAffinityNone,
			wantNil: true,
		},
		{
			name:        "required returns required anti-affinity",
			level:       ackov1alpha1.PodAntiAffinityRequired,
			clusterName: "test-cluster",
			wantNil:     false,
			wantType:    "required",
		},
		{
			name:        "preferred returns preferred anti-affinity",
			level:       ackov1alpha1.PodAntiAffinityPreferred,
			clusterName: "test-cluster",
			wantNil:     false,
			wantType:    "preferred",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TranslatePodAntiAffinity(tt.level, tt.clusterName)
			if tt.wantNil {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
				return
			}
			if result == nil {
				t.Fatalf("expected non-nil result")
			}
			switch tt.wantType {
			case "required":
				if len(result.RequiredDuringSchedulingIgnoredDuringExecution) == 0 {
					t.Errorf("expected RequiredDuringScheduling to be set")
				}
				if len(result.PreferredDuringSchedulingIgnoredDuringExecution) != 0 {
					t.Errorf("expected PreferredDuringScheduling to be empty")
				}
				term := result.RequiredDuringSchedulingIgnoredDuringExecution[0]
				if term.TopologyKey != "kubernetes.io/hostname" {
					t.Errorf("expected topologyKey kubernetes.io/hostname, got %s", term.TopologyKey)
				}
			case "preferred":
				if len(result.PreferredDuringSchedulingIgnoredDuringExecution) == 0 {
					t.Errorf("expected PreferredDuringScheduling to be set")
				}
				if len(result.RequiredDuringSchedulingIgnoredDuringExecution) != 0 {
					t.Errorf("expected RequiredDuringScheduling to be empty")
				}
				term := result.PreferredDuringSchedulingIgnoredDuringExecution[0]
				if term.Weight != 100 {
					t.Errorf("expected weight 100, got %d", term.Weight)
				}
			}
		})
	}
}

func TestApplyScheduling_NodeAffinityDeepCopied(t *testing.T) {
	nodeAffinity := &corev1.NodeAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
			NodeSelectorTerms: []corev1.NodeSelectorTerm{
				{MatchExpressions: []corev1.NodeSelectorRequirement{
					{Key: "zone", Operator: corev1.NodeSelectorOpIn, Values: []string{"us-east-1a"}},
				}},
			},
		},
	}
	scheduling := &ackov1alpha1.TemplateScheduling{NodeAffinity: nodeAffinity}
	cluster := &ackov1alpha1.AerospikeCluster{}

	applyScheduling(scheduling, cluster)

	// Mutating the cluster's NodeAffinity must not affect the template's original.
	cluster.Spec.PodSpec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.
		NodeSelectorTerms[0].MatchExpressions[0].Values[0] = testMutatedValue
	if nodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.
		NodeSelectorTerms[0].MatchExpressions[0].Values[0] != "us-east-1a" {
		t.Error("applyScheduling must deep copy NodeAffinity to avoid shared pointers")
	}
}

func TestApplyScheduling_TolerationsDeepCopied(t *testing.T) {
	scheduling := &ackov1alpha1.TemplateScheduling{
		Tolerations: []corev1.Toleration{
			{Key: "original", Operator: corev1.TolerationOpExists},
		},
	}
	cluster := &ackov1alpha1.AerospikeCluster{}

	applyScheduling(scheduling, cluster)

	// Mutating the cluster's Tolerations slice must not affect the template.
	cluster.Spec.PodSpec.Tolerations[0].Key = testMutatedValue
	if scheduling.Tolerations[0].Key != "original" {
		t.Error("applyScheduling must deep copy Tolerations to avoid shared backing array")
	}
}

func TestApplyScheduling_TopologySpreadConstraintsDeepCopied(t *testing.T) {
	scheduling := &ackov1alpha1.TemplateScheduling{
		TopologySpreadConstraints: []corev1.TopologySpreadConstraint{
			{MaxSkew: 1, TopologyKey: testTopologyZone, WhenUnsatisfiable: corev1.DoNotSchedule},
		},
	}
	cluster := &ackov1alpha1.AerospikeCluster{}

	applyScheduling(scheduling, cluster)

	// Mutating the cluster's TopologySpreadConstraints must not affect the template.
	cluster.Spec.PodSpec.TopologySpreadConstraints[0].TopologyKey = testMutatedValue
	if scheduling.TopologySpreadConstraints[0].TopologyKey != testTopologyZone {
		t.Error("applyScheduling must deep copy TopologySpreadConstraints to avoid shared backing array")
	}
}
