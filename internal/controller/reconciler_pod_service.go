package controller

import (
	"context"
	"fmt"
	"maps"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	ackov1alpha1 "github.com/ksr/aerospike-ce-kubernetes-operator/api/v1alpha1"
	"github.com/ksr/aerospike-ce-kubernetes-operator/internal/podutil"
	"github.com/ksr/aerospike-ce-kubernetes-operator/internal/utils"
)

const podServiceLabel = "acko.io/pod-service"

// reconcilePodServices creates or updates individual Services for each pod
// when spec.podService is configured. It also cleans up stale pod services
// left behind after scale-down or when podService is disabled.
func (r *AerospikeClusterReconciler) reconcilePodServices(
	ctx context.Context,
	cluster *ackov1alpha1.AerospikeCluster,
) error {
	if cluster.Spec.PodService == nil {
		// PodService disabled — clean up any leftover pod services.
		return r.cleanupStalePodServices(ctx, cluster, nil)
	}

	log := logf.FromContext(ctx)

	pods, err := r.listClusterPods(ctx, cluster)
	if err != nil {
		return fmt.Errorf("listing cluster pods for pod services: %w", err)
	}

	serviceType := podServiceType(cluster.Spec.PodService)

	activePodNames := make(map[string]struct{}, len(pods.Items))
	for i := range pods.Items {
		pod := &pods.Items[i]
		activePodNames[pod.Name] = struct{}{}
		svcName := fmt.Sprintf("%s-pod", pod.Name)

		labels := utils.LabelsForCluster(cluster.Name)
		labels[podServiceLabel] = pod.Name

		desiredPorts := podServicePorts(serviceType)

		// Pod-specific selector
		podSelector := map[string]string{
			"statefulset.kubernetes.io/pod-name": pod.Name,
		}

		// Build desired annotations from custom metadata.
		var desiredAnnotations map[string]string
		if cluster.Spec.PodService.Metadata != nil {
			if cluster.Spec.PodService.Metadata.Annotations != nil {
				desiredAnnotations = make(map[string]string)
				maps.Copy(desiredAnnotations, cluster.Spec.PodService.Metadata.Annotations)
			}
			labels = mergeAdditionalLabels(labels, cluster.Spec.PodService.Metadata.Labels)
		}

		existing := &corev1.Service{}
		err := r.Get(ctx, types.NamespacedName{Name: svcName, Namespace: cluster.Namespace}, existing)

		if errors.IsNotFound(err) {
			svc := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:        svcName,
					Namespace:   cluster.Namespace,
					Labels:      labels,
					Annotations: desiredAnnotations,
				},
				Spec: podServiceSpec(serviceType, podSelector, desiredPorts),
			}

			if err := r.setOwnerRef(cluster, svc); err != nil {
				return err
			}

			log.Info("Creating per-pod service", "name", svcName, "pod", pod.Name)
			if err := r.Create(ctx, svc); err != nil {
				return fmt.Errorf("creating pod service %s: %w", svcName, err)
			}

			continue
		} else if err != nil {
			return fmt.Errorf("getting pod service %s: %w", svcName, err)
		}

		// Compare and update if needed.
		needsUpdate := podServiceNeedsUpdate(existing, labels, desiredAnnotations, podSelector, desiredPorts, serviceType)

		if needsUpdate {
			existing.Labels = labels
			existing.Annotations = reconcileAnnotations(existing.Annotations, desiredAnnotations)
			existing.Spec.Type = serviceType
			existing.Spec.Selector = podSelector
			existing.Spec.Ports = desiredPorts
			log.Info("Updating per-pod service", "name", svcName)
			if err := r.Update(ctx, existing); err != nil {
				return fmt.Errorf("updating pod service %s: %w", svcName, err)
			}
		} else {
			log.V(1).Info("Pod service up-to-date, no update needed", "name", svcName)
		}
	}

	return r.cleanupStalePodServices(ctx, cluster, activePodNames)
}

// cleanupStalePodServices removes pod services that no longer correspond to
// an active pod. When activePodNames is nil, all pod services for the cluster
// are removed (used when podService is disabled).
func (r *AerospikeClusterReconciler) cleanupStalePodServices(
	ctx context.Context,
	cluster *ackov1alpha1.AerospikeCluster,
	activePodNames map[string]struct{},
) error {
	log := logf.FromContext(ctx)

	// List all pod services belonging to this cluster.
	svcList := &corev1.ServiceList{}
	matchLabels := utils.SelectorLabelsForCluster(cluster.Name)
	if err := r.List(ctx, svcList,
		client.InNamespace(cluster.Namespace),
		client.MatchingLabels(matchLabels),
		client.HasLabels{podServiceLabel},
	); err != nil {
		return fmt.Errorf("listing pod services for cleanup: %w", err)
	}

	for i := range svcList.Items {
		svc := &svcList.Items[i]
		if svc.Labels == nil {
			continue
		}
		podName := svc.Labels[podServiceLabel]
		if podName == "" {
			continue
		}
		if _, active := activePodNames[podName]; !active {
			log.Info("Deleting stale pod service", "name", svc.Name, "pod", podName)
			if err := r.Delete(ctx, svc); err != nil && !errors.IsNotFound(err) {
				return fmt.Errorf("deleting stale pod service %s: %w", svc.Name, err)
			}
		}
	}

	return nil
}

func podServiceNeedsUpdate(
	existing *corev1.Service,
	desiredLabels map[string]string,
	desiredAnnotations map[string]string,
	desiredSelector map[string]string,
	desiredPorts []corev1.ServicePort,
	desiredType corev1.ServiceType,
) bool {
	return !equalAnnotations(existing.Annotations, desiredAnnotations) ||
		!maps.Equal(existing.Labels, desiredLabels) ||
		existing.Spec.Type != desiredType ||
		!maps.Equal(existing.Spec.Selector, desiredSelector) ||
		servicePortsChanged(existing.Spec.Ports, desiredPorts)
}

// podServiceType returns the desired service type from the PodService spec.
func podServiceType(spec *ackov1alpha1.AerospikeServiceSpec) corev1.ServiceType {
	if spec != nil && spec.ServiceType != "" {
		return corev1.ServiceType(spec.ServiceType)
	}
	return corev1.ServiceTypeClusterIP
}

// podServicePorts returns the ports to expose on per-pod services.
// For ClusterIP, only the service port is exposed.
// For NodePort/LoadBalancer, all Aerospike ports are exposed for full external access.
func podServicePorts(svcType corev1.ServiceType) []corev1.ServicePort {
	servicePorts := []corev1.ServicePort{
		{Name: "service", Port: podutil.ServicePort, TargetPort: intstr.FromInt32(podutil.ServicePort), Protocol: corev1.ProtocolTCP},
	}
	if svcType == corev1.ServiceTypeNodePort || svcType == corev1.ServiceTypeLoadBalancer {
		servicePorts = append(servicePorts,
			corev1.ServicePort{Name: "fabric", Port: podutil.FabricPort, TargetPort: intstr.FromInt32(podutil.FabricPort), Protocol: corev1.ProtocolTCP},
			corev1.ServicePort{Name: "heartbeat", Port: podutil.HeartbeatPort, TargetPort: intstr.FromInt32(podutil.HeartbeatPort), Protocol: corev1.ProtocolTCP},
		)
	}
	return servicePorts
}

// podServiceSpec builds the ServiceSpec for a per-pod service.
func podServiceSpec(svcType corev1.ServiceType, selector map[string]string, ports []corev1.ServicePort) corev1.ServiceSpec {
	return corev1.ServiceSpec{
		Type:     svcType,
		Selector: selector,
		Ports:    ports,
	}
}
