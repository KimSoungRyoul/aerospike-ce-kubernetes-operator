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

// Package template implements the AerospikeClusterTemplate resolution logic.
// It converts template abstract fields into concrete AerospikeClusterSpec fields.
package template

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ackov1alpha1 "github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/api/v1alpha1"
	"github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/internal/utils"
)

// TranslatePodAntiAffinity converts a PodAntiAffinityLevel to a Kubernetes PodAntiAffinity struct.
// Returns nil for PodAntiAffinityNone or empty level.
func TranslatePodAntiAffinity(level ackov1alpha1.PodAntiAffinityLevel, clusterName string) *corev1.PodAntiAffinity {
	if level == "" || level == ackov1alpha1.PodAntiAffinityNone {
		return nil
	}

	term := corev1.PodAffinityTerm{
		LabelSelector: &metav1.LabelSelector{
			MatchLabels: utils.SelectorLabelsForCluster(clusterName),
		},
		TopologyKey: "kubernetes.io/hostname",
	}

	switch level {
	case ackov1alpha1.PodAntiAffinityRequired:
		return &corev1.PodAntiAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{term},
		}
	case ackov1alpha1.PodAntiAffinityPreferred:
		return &corev1.PodAntiAffinity{
			PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
				{Weight: 100, PodAffinityTerm: term},
			},
		}
	default:
		return nil
	}
}

// applyScheduling translates template scheduling settings into the cluster's PodSpec.
// It only sets fields that are not already explicitly set in the cluster spec.
func applyScheduling(scheduling *ackov1alpha1.TemplateScheduling, cluster *ackov1alpha1.AerospikeCluster) {
	if scheduling == nil {
		return
	}

	if cluster.Spec.PodSpec == nil {
		cluster.Spec.PodSpec = &ackov1alpha1.AerospikePodSpec{}
	}
	ps := cluster.Spec.PodSpec

	// Apply pod anti-affinity only when no explicit affinity is set.
	if level := scheduling.PodAntiAffinityLevel; level != "" {
		antiAffinity := TranslatePodAntiAffinity(level, cluster.Name)
		if ps.Affinity == nil {
			ps.Affinity = &corev1.Affinity{}
		}
		// Only apply if not already configured by the user.
		if ps.Affinity.PodAntiAffinity == nil && antiAffinity != nil {
			ps.Affinity.PodAntiAffinity = antiAffinity
		}
	}

	// Apply node affinity if not already set.
	if scheduling.NodeAffinity != nil {
		if ps.Affinity == nil {
			ps.Affinity = &corev1.Affinity{}
		}
		if ps.Affinity.NodeAffinity == nil {
			ps.Affinity.NodeAffinity = scheduling.NodeAffinity.DeepCopy()
		}
	}

	// Apply tolerations if not already set.
	if len(scheduling.Tolerations) > 0 && len(ps.Tolerations) == 0 {
		ps.Tolerations = make([]corev1.Toleration, len(scheduling.Tolerations))
		copy(ps.Tolerations, scheduling.Tolerations)
	}

	// Apply topology spread constraints if not already set.
	if len(scheduling.TopologySpreadConstraints) > 0 && len(ps.TopologySpreadConstraints) == 0 {
		ps.TopologySpreadConstraints = make([]corev1.TopologySpreadConstraint, len(scheduling.TopologySpreadConstraints))
		copy(ps.TopologySpreadConstraints, scheduling.TopologySpreadConstraints)
	}

	// Apply pod management policy if not already set.
	if scheduling.PodManagementPolicy != "" && ps.PodManagementPolicy == "" {
		ps.PodManagementPolicy = scheduling.PodManagementPolicy
	}
}
