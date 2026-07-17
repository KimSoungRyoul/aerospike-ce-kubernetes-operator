package controller

import (
	"context"
	"testing"

	ackov1alpha1 "github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/api/v1alpha1"
	"github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/internal/utils"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestPDBNeedsUpdate(t *testing.T) {
	desiredLabels := utils.LabelsForCluster("demo")
	desiredSelector := utils.SelectorLabelsForCluster("demo")
	desiredMaxUnavailable := intstr.FromInt32(1)

	base := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Labels: desiredLabels,
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MaxUnavailable: &desiredMaxUnavailable,
			Selector: &metav1.LabelSelector{
				MatchLabels: desiredSelector,
			},
		},
	}

	tests := []struct {
		name     string
		mutate   func(*policyv1.PodDisruptionBudget)
		expected bool
	}{
		{
			name:     "unchanged",
			mutate:   func(_ *policyv1.PodDisruptionBudget) {},
			expected: false,
		},
		{
			name: "max unavailable drift",
			mutate: func(pdb *policyv1.PodDisruptionBudget) {
				value := intstr.FromInt32(2)
				pdb.Spec.MaxUnavailable = &value
			},
			expected: true,
		},
		{
			name: "selector drift",
			mutate: func(pdb *policyv1.PodDisruptionBudget) {
				pdb.Spec.Selector = &metav1.LabelSelector{
					MatchLabels: map[string]string{utils.InstanceLabel: "other"},
				}
			},
			expected: true,
		},
		{
			name: "selector match expressions drift",
			mutate: func(pdb *policyv1.PodDisruptionBudget) {
				pdb.Spec.Selector.MatchExpressions = []metav1.LabelSelectorRequirement{
					{
						Key:      "rack",
						Operator: metav1.LabelSelectorOpExists,
					},
				}
			},
			expected: true,
		},
		{
			name: "operator label drift",
			mutate: func(pdb *policyv1.PodDisruptionBudget) {
				pdb.Labels[utils.InstanceLabel] = "wrong"
			},
			expected: true,
		},
		{
			name: "extra external label is not drift",
			mutate: func(pdb *policyv1.PodDisruptionBudget) {
				pdb.Labels["custom"] = "external"
			},
			expected: false,
		},
		{
			name: "min available drift",
			mutate: func(pdb *policyv1.PodDisruptionBudget) {
				value := intstr.FromInt32(1)
				pdb.Spec.MinAvailable = &value
				pdb.Spec.MaxUnavailable = nil
			},
			expected: true,
		},
		{
			name: "missing selector",
			mutate: func(pdb *policyv1.PodDisruptionBudget) {
				pdb.Spec.Selector = nil
			},
			expected: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pdb := base.DeepCopy()
			tc.mutate(pdb)
			if got := pdbNeedsUpdate(pdb, desiredLabels, desiredSelector, desiredMaxUnavailable); got != tc.expected {
				t.Fatalf("pdbNeedsUpdate() = %v, want %v", got, tc.expected)
			}
		})
	}
}

func TestReconcilePDBRepairsGeneratedDrift(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme(client-go) error = %v", err)
	}
	if err := ackov1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme(acko) error = %v", err)
	}

	cluster := &ackov1alpha1.AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo",
			Namespace: "default",
		},
	}

	staleMaxUnavailable := intstr.FromInt32(2)
	staleMinAvailable := intstr.FromInt32(1)
	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.PDBName(cluster.Name),
			Namespace: cluster.Namespace,
			Labels: map[string]string{
				"custom": "drifted",
			},
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MinAvailable:   &staleMinAvailable,
			MaxUnavailable: &staleMaxUnavailable,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					utils.InstanceLabel: cluster.Name,
				},
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "rack",
						Operator: metav1.LabelSelectorOpExists,
					},
				},
			},
		},
	}

	reconciler := &AerospikeClusterReconciler{
		Client: fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(cluster, pdb).
			Build(),
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(8),
	}

	if err := reconciler.reconcilePDB(context.Background(), cluster); err != nil {
		t.Fatalf("reconcilePDB() error = %v", err)
	}

	updated := &policyv1.PodDisruptionBudget{}
	if err := reconciler.Get(context.Background(), types.NamespacedName{Name: pdb.Name, Namespace: pdb.Namespace}, updated); err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	wantLabels := utils.LabelsForCluster(cluster.Name)
	if updated.Spec.MinAvailable != nil {
		t.Fatalf("MinAvailable = %v, want nil", updated.Spec.MinAvailable)
	}
	if updated.Spec.MaxUnavailable == nil || !intOrStringEqual(*updated.Spec.MaxUnavailable, intstr.FromInt32(1)) {
		t.Fatalf("MaxUnavailable = %v, want 1", updated.Spec.MaxUnavailable)
	}
	if updated.Spec.Selector == nil {
		t.Fatal("Selector = nil, want generated selector")
	}
	if len(updated.Spec.Selector.MatchExpressions) != 0 {
		t.Fatalf("MatchExpressions = %v, want empty", updated.Spec.Selector.MatchExpressions)
	}
	if got := updated.Spec.Selector.MatchLabels; len(got) != len(utils.SelectorLabelsForCluster(cluster.Name)) || got[utils.InstanceLabel] != cluster.Name {
		t.Fatalf("Selector.MatchLabels = %v, want %v", got, utils.SelectorLabelsForCluster(cluster.Name))
	}
	for k, v := range wantLabels {
		if updated.Labels[k] != v {
			t.Fatalf("Labels[%q] = %q, want %q", k, updated.Labels[k], v)
		}
	}
	if updated.Labels["custom"] != "drifted" {
		t.Fatalf("external label 'custom' was not preserved, got %q", updated.Labels["custom"])
	}
}

func TestIntOrStringEqual(t *testing.T) {
	tests := []struct {
		name     string
		a, b     intstr.IntOrString
		expected bool
	}{
		{"same int", intstr.FromInt32(1), intstr.FromInt32(1), true},
		{"different int", intstr.FromInt32(1), intstr.FromInt32(2), false},
		{"same string", intstr.FromString("50%"), intstr.FromString("50%"), true},
		{"different string", intstr.FromString("50%"), intstr.FromString("25%"), false},
		{"int vs string same text", intstr.FromInt32(1), intstr.FromString("1"), false},
		{"zero values", intstr.IntOrString{}, intstr.IntOrString{}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := intOrStringEqual(tc.a, tc.b); got != tc.expected {
				t.Fatalf("intOrStringEqual() = %v, want %v", got, tc.expected)
			}
		})
	}
}
