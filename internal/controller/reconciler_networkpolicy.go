package controller

import (
	"context"
	"fmt"
	"maps"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	ackov1alpha1 "github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/api/v1alpha1"
	"github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/internal/podutil"
	"github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/internal/utils"
)

var ciliumNetworkPolicyGVK = schema.GroupVersionKind{
	Group:   "cilium.io",
	Version: "v2",
	Kind:    "CiliumNetworkPolicy",
}

func (r *AerospikeClusterReconciler) reconcileNetworkPolicy(
	ctx context.Context,
	cluster *ackov1alpha1.AerospikeCluster,
) error {
	log := logf.FromContext(ctx)
	log.V(1).Info("Reconciling network policy", "cluster", cluster.Name)

	npcEnabled := cluster.Spec.NetworkPolicyConfig != nil && cluster.Spec.NetworkPolicyConfig.Enabled
	npcType := ackov1alpha1.NetworkPolicyTypeKubernetes
	if cluster.Spec.NetworkPolicyConfig != nil && cluster.Spec.NetworkPolicyConfig.Type != "" {
		npcType = cluster.Spec.NetworkPolicyConfig.Type
	}

	switch npcType {
	case ackov1alpha1.NetworkPolicyTypeCilium:
		return r.reconcileCiliumNetworkPolicy(ctx, cluster, npcEnabled)
	default:
		return r.reconcileK8sNetworkPolicy(ctx, cluster, npcEnabled)
	}
}

func (r *AerospikeClusterReconciler) reconcileK8sNetworkPolicy(
	ctx context.Context,
	cluster *ackov1alpha1.AerospikeCluster,
	enabled bool,
) error {
	log := logf.FromContext(ctx)
	npName := utils.NetworkPolicyName(cluster.Name)

	existing := &networkingv1.NetworkPolicy{}
	err := r.Get(ctx, types.NamespacedName{Name: npName, Namespace: cluster.Namespace}, existing)

	if !enabled {
		if err == nil {
			log.Info("Deleting NetworkPolicy", "name", npName)
			if delErr := r.Delete(ctx, existing); delErr != nil && !errors.IsNotFound(delErr) {
				return delErr
			}
		}
		return nil
	}

	np := r.buildK8sNetworkPolicy(cluster, npName)

	if errors.IsNotFound(err) {
		if err := r.setOwnerRef(cluster, np); err != nil {
			return err
		}
		log.Info("Creating NetworkPolicy", "name", npName)
		return r.Create(ctx, np)
	} else if err != nil {
		return fmt.Errorf("getting NetworkPolicy %s: %w", npName, err)
	}

	if !k8sNetworkPolicyChanged(existing, np) {
		log.V(1).Info("NetworkPolicy unchanged, skipping update", "name", npName)
		return nil
	}

	existing.Spec = np.Spec
	existing.Labels = np.Labels
	log.Info("Updating NetworkPolicy", "name", npName)
	return r.Update(ctx, existing)
}

func (r *AerospikeClusterReconciler) buildK8sNetworkPolicy(
	cluster *ackov1alpha1.AerospikeCluster,
	name string,
) *networkingv1.NetworkPolicy {
	labels := utils.LabelsForCluster(cluster.Name)
	selectorLabels := utils.SelectorLabelsForCluster(cluster.Name)
	protocolTCP := corev1.ProtocolTCP

	servicePort := intstr.FromInt32(podutil.ServicePort)
	fabricPort := intstr.FromInt32(podutil.FabricPort)
	heartbeatPort := intstr.FromInt32(podutil.HeartbeatPort)

	ingressRules := []networkingv1.NetworkPolicyIngressRule{
		{
			// Intra-cluster: fabric + heartbeat
			Ports: []networkingv1.NetworkPolicyPort{
				{Protocol: &protocolTCP, Port: &fabricPort},
				{Protocol: &protocolTCP, Port: &heartbeatPort},
			},
			From: []networkingv1.NetworkPolicyPeer{
				{
					PodSelector: &metav1.LabelSelector{
						MatchLabels: selectorLabels,
					},
				},
			},
		},
		{
			// Client access: service port
			Ports: []networkingv1.NetworkPolicyPort{
				{Protocol: &protocolTCP, Port: &servicePort},
			},
		},
	}

	// Allow metrics port if monitoring is enabled
	if cluster.Spec.Monitoring != nil && cluster.Spec.Monitoring.Enabled {
		metricsPort := intstr.FromInt32(cluster.Spec.Monitoring.Port)
		ingressRules = append(ingressRules, networkingv1.NetworkPolicyIngressRule{
			Ports: []networkingv1.NetworkPolicyPort{
				{Protocol: &protocolTCP, Port: &metricsPort},
			},
		})
	}

	return &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: cluster.Namespace,
			Labels:    labels,
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: selectorLabels,
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
			},
			Ingress: ingressRules,
		},
	}
}

func (r *AerospikeClusterReconciler) reconcileCiliumNetworkPolicy(
	ctx context.Context,
	cluster *ackov1alpha1.AerospikeCluster,
	enabled bool,
) error {
	log := logf.FromContext(ctx)
	npName := utils.NetworkPolicyName(cluster.Name)

	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(ciliumNetworkPolicyGVK)

	err := r.Get(ctx, types.NamespacedName{Name: npName, Namespace: cluster.Namespace}, existing)

	if !enabled {
		if err == nil {
			log.Info("Deleting CiliumNetworkPolicy", "name", npName)
			if delErr := r.Delete(ctx, existing); delErr != nil && !errors.IsNotFound(delErr) {
				return delErr
			}
		}
		return nil
	}

	// CRD not installed — graceful skip
	if err != nil && meta.IsNoMatchError(err) {
		log.Info("CiliumNetworkPolicy CRD not installed, skipping")
		return nil
	}

	selectorLabels := utils.SelectorLabelsForCluster(cluster.Name)
	labels := utils.LabelsForCluster(cluster.Name)

	ingressRules := []any{
		map[string]any{
			"fromEndpoints": []any{
				map[string]any{
					"matchLabels": toStringMap(selectorLabels),
				},
			},
			"toPorts": []any{
				map[string]any{
					"ports": []any{
						map[string]any{"port": fmt.Sprintf("%d", podutil.FabricPort), "protocol": "TCP"},
						map[string]any{"port": fmt.Sprintf("%d", podutil.HeartbeatPort), "protocol": "TCP"},
					},
				},
			},
		},
		map[string]any{
			"toPorts": []any{
				map[string]any{
					"ports": []any{
						map[string]any{"port": fmt.Sprintf("%d", podutil.ServicePort), "protocol": "TCP"},
					},
				},
			},
		},
	}

	// Allow metrics port if monitoring is enabled
	if cluster.Spec.Monitoring != nil && cluster.Spec.Monitoring.Enabled {
		ingressRules = append(ingressRules, map[string]any{
			"toPorts": []any{
				map[string]any{
					"ports": []any{
						map[string]any{"port": fmt.Sprintf("%d", cluster.Spec.Monitoring.Port), "protocol": "TCP"},
					},
				},
			},
		})
	}

	spec := map[string]any{
		"endpointSelector": map[string]any{
			"matchLabels": toStringMap(selectorLabels),
		},
		"ingress": ingressRules,
	}

	if errors.IsNotFound(err) {
		cnp := &unstructured.Unstructured{}
		cnp.SetGroupVersionKind(ciliumNetworkPolicyGVK)
		cnp.SetName(npName)
		cnp.SetNamespace(cluster.Namespace)
		cnp.SetLabels(labels)
		cnp.Object["spec"] = spec

		if err := r.setOwnerRef(cluster, cnp); err != nil {
			return err
		}
		log.Info("Creating CiliumNetworkPolicy", "name", npName)
		return r.Create(ctx, cnp)
	} else if err != nil {
		return fmt.Errorf("getting CiliumNetworkPolicy %s: %w", npName, err)
	}

	if !unstructuredResourceChanged(existing, spec, labels) {
		log.V(1).Info("CiliumNetworkPolicy unchanged, skipping update", "name", npName)
		return nil
	}

	existing.Object["spec"] = spec
	existing.SetLabels(labels)
	log.Info("Updating CiliumNetworkPolicy", "name", npName)
	return r.Update(ctx, existing)
}

func k8sNetworkPolicyChanged(existing, desired *networkingv1.NetworkPolicy) bool {
	return !equality.Semantic.DeepEqual(existing.Spec, desired.Spec) ||
		!maps.Equal(existing.Labels, desired.Labels)
}
