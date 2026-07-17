package controller

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ackov1alpha1 "github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/api/v1alpha1"
	"github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/internal/utils"
)

// listClusterPods returns all pods matching the cluster's selector labels.
func (r *AerospikeClusterReconciler) listClusterPods(
	ctx context.Context,
	cluster *ackov1alpha1.AerospikeCluster,
) (*corev1.PodList, error) {
	podList := &corev1.PodList{}
	if err := r.List(ctx, podList,
		client.InNamespace(cluster.Namespace),
		client.MatchingLabels(utils.SelectorLabelsForCluster(cluster.Name)),
	); err != nil {
		return nil, err
	}
	return podList, nil
}

// listClusterStatefulSets returns all StatefulSets matching the cluster's selector labels.
func (r *AerospikeClusterReconciler) listClusterStatefulSets(
	ctx context.Context,
	cluster *ackov1alpha1.AerospikeCluster,
) (*appsv1.StatefulSetList, error) {
	stsList := &appsv1.StatefulSetList{}
	if err := r.List(ctx, stsList,
		client.InNamespace(cluster.Namespace),
		client.MatchingLabels(utils.SelectorLabelsForCluster(cluster.Name)),
	); err != nil {
		return nil, err
	}
	return stsList, nil
}

// refetchCluster re-reads the cluster from the API server to get the latest version.
func (r *AerospikeClusterReconciler) refetchCluster(
	ctx context.Context,
	nn types.NamespacedName,
) (*ackov1alpha1.AerospikeCluster, error) {
	latest := &ackov1alpha1.AerospikeCluster{}
	if err := r.Get(ctx, nn, latest); err != nil {
		return nil, err
	}
	return latest, nil
}

// setOwnerRef sets the controller reference on the given object.
func (r *AerospikeClusterReconciler) setOwnerRef(
	cluster *ackov1alpha1.AerospikeCluster,
	obj client.Object,
) error {
	if err := ctrl.SetControllerReference(cluster, obj, r.Scheme); err != nil {
		return fmt.Errorf("setting controller reference: %w", err)
	}
	return nil
}
