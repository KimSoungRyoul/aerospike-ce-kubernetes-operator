//go:build e2e

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

package utils

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ackov1alpha1 "github.com/ksr/aerospike-ce-kubernetes-operator/api/v1alpha1"
	internalutils "github.com/ksr/aerospike-ce-kubernetes-operator/internal/utils"
)

// --- Namespace helpers ---

// EnsureNamespace creates a namespace if it does not already exist.
func EnsureNamespace(ctx context.Context, c client.Client, name string) error {
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
	err := c.Create(ctx, ns)
	if errors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

// DeleteNamespace deletes a namespace, ignoring NotFound errors.
func DeleteNamespace(ctx context.Context, c client.Client, name string) error {
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
	return client.IgnoreNotFound(c.Delete(ctx, ns))
}

// --- AerospikeCluster CRUD ---

// GetCluster retrieves an AerospikeCluster by name and namespace.
func GetCluster(ctx context.Context, c client.Client, name, ns string) (*ackov1alpha1.AerospikeCluster, error) {
	cluster := &ackov1alpha1.AerospikeCluster{}
	err := c.Get(ctx, types.NamespacedName{Name: name, Namespace: ns}, cluster)
	return cluster, err
}

// PatchCluster applies a JSON merge patch to an AerospikeCluster.
func PatchCluster(ctx context.Context, c client.Client, name, ns string, patch []byte) error {
	cluster := &ackov1alpha1.AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
	}
	return c.Patch(ctx, cluster, client.RawPatch(types.MergePatchType, patch))
}

// DeleteCluster deletes an AerospikeCluster, ignoring NotFound errors.
func DeleteCluster(ctx context.Context, c client.Client, name, ns string) error {
	cluster := &ackov1alpha1.AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
	}
	return client.IgnoreNotFound(c.Delete(ctx, cluster))
}

// --- Pod helpers ---

// ListClusterPods lists all pods belonging to a cluster using its selector labels.
func ListClusterPods(ctx context.Context, c client.Client, clusterName, ns string) (*corev1.PodList, error) {
	podList := &corev1.PodList{}
	err := c.List(ctx, podList,
		client.InNamespace(ns),
		client.MatchingLabels(internalutils.SelectorLabelsForCluster(clusterName)),
	)
	return podList, err
}

// CountReadyPods counts pods that are Running with the Ready condition True.
func CountReadyPods(ctx context.Context, c client.Client, clusterName, ns string) (int, error) {
	podList, err := ListClusterPods(ctx, c, clusterName, ns)
	if err != nil {
		return 0, err
	}
	count := 0
	for i := range podList.Items {
		pod := &podList.Items[i]
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, cond := range pod.Status.Conditions {
			if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
				count++
				break
			}
		}
	}
	return count, nil
}

// --- StatefulSet helpers ---

// ListClusterStatefulSets lists all StatefulSets belonging to a cluster.
func ListClusterStatefulSets(ctx context.Context, c client.Client, clusterName, ns string) (*appsv1.StatefulSetList, error) {
	stsList := &appsv1.StatefulSetList{}
	err := c.List(ctx, stsList,
		client.InNamespace(ns),
		client.MatchingLabels(internalutils.SelectorLabelsForCluster(clusterName)),
	)
	return stsList, err
}

// --- PVC helpers ---

// ListClusterPVCs lists all PersistentVolumeClaims belonging to a cluster.
func ListClusterPVCs(ctx context.Context, c client.Client, clusterName, ns string) (*corev1.PersistentVolumeClaimList, error) {
	pvcList := &corev1.PersistentVolumeClaimList{}
	err := c.List(ctx, pvcList,
		client.InNamespace(ns),
		client.MatchingLabels(internalutils.SelectorLabelsForCluster(clusterName)),
	)
	return pvcList, err
}

// --- Resource existence helpers ---

// resourceExists checks if an arbitrary Kubernetes resource exists.
func resourceExists(ctx context.Context, c client.Client, obj client.Object, name, ns string) (bool, error) {
	err := c.Get(ctx, types.NamespacedName{Name: name, Namespace: ns}, obj)
	if errors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// ServiceExists checks if a Service exists.
func ServiceExists(ctx context.Context, c client.Client, name, ns string) (bool, error) {
	return resourceExists(ctx, c, &corev1.Service{}, name, ns)
}

// ConfigMapExists checks if a ConfigMap exists.
func ConfigMapExists(ctx context.Context, c client.Client, name, ns string) (bool, error) {
	return resourceExists(ctx, c, &corev1.ConfigMap{}, name, ns)
}

// PDBExists checks if a PodDisruptionBudget exists.
func PDBExists(ctx context.Context, c client.Client, name, ns string) (bool, error) {
	return resourceExists(ctx, c, &policyv1.PodDisruptionBudget{}, name, ns)
}

// --- AerospikeClusterTemplate helpers ---

// PatchTemplate applies a JSON merge patch to an AerospikeClusterTemplate.
// Templates are cluster-scoped, so no namespace is needed.
func PatchTemplate(ctx context.Context, c client.Client, name string, patch []byte) error {
	template := &ackov1alpha1.AerospikeClusterTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: name},
	}
	return c.Patch(ctx, template, client.RawPatch(types.MergePatchType, patch))
}
