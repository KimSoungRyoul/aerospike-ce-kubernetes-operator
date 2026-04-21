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
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	ackov1alpha1 "github.com/ksr/aerospike-ce-kubernetes-operator/api/v1alpha1"
	"github.com/ksr/aerospike-ce-kubernetes-operator/internal/utils"
)

// seedsFinderServiceName returns the name of the seeds finder LoadBalancer service.
func seedsFinderServiceName(clusterName string) string {
	return clusterName + "-seeds-lb"
}

// reconcileSeedsFinderService creates, updates, or deletes the LoadBalancer service
// used for external seed discovery based on spec.seedsFinderServices.loadBalancer.
func (r *AerospikeClusterReconciler) reconcileSeedsFinderService(
	ctx context.Context,
	cluster *ackov1alpha1.AerospikeCluster,
) error {
	log := logf.FromContext(ctx)
	svcName := seedsFinderServiceName(cluster.Name)

	// When the LoadBalancer spec is nil, clean up the service if it exists.
	if cluster.Spec.SeedsFinderServices == nil || cluster.Spec.SeedsFinderServices.LoadBalancer == nil {
		return r.cleanupSeedsFinderService(ctx, cluster, svcName)
	}

	lbSpec := cluster.Spec.SeedsFinderServices.LoadBalancer
	labels := utils.LabelsForCluster(cluster.Name)
	selectorLabels := utils.SelectorLabelsForCluster(cluster.Name)

	// Apply user-specified labels without overwriting operator-managed labels.
	labels = mergeAdditionalLabels(labels, lbSpec.Labels)

	// Build desired annotations.
	var desiredAnnotations map[string]string
	if lbSpec.Annotations != nil {
		desiredAnnotations = make(map[string]string)
		maps.Copy(desiredAnnotations, lbSpec.Annotations)
	}

	// Resolve port defaults.
	port := lbSpec.Port
	if port == 0 {
		port = 3000
	}
	targetPort := lbSpec.TargetPort
	if targetPort == 0 {
		targetPort = 3000
	}

	desiredPorts := []corev1.ServicePort{
		{
			Name:       "service",
			Port:       port,
			TargetPort: intstr.FromInt32(targetPort),
			Protocol:   corev1.ProtocolTCP,
		},
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
			Spec: corev1.ServiceSpec{
				Type:     corev1.ServiceTypeLoadBalancer,
				Selector: selectorLabels,
				Ports:    desiredPorts,
			},
		}

		if lbSpec.ExternalTrafficPolicy != "" {
			svc.Spec.ExternalTrafficPolicy = corev1.ServiceExternalTrafficPolicy(lbSpec.ExternalTrafficPolicy)
		}
		if len(lbSpec.LoadBalancerSourceRanges) > 0 {
			svc.Spec.LoadBalancerSourceRanges = lbSpec.LoadBalancerSourceRanges
		}

		if err := r.setOwnerRef(cluster, svc); err != nil {
			return err
		}
		log.Info("Creating seeds finder LoadBalancer service", "name", svcName)
		if err := r.Create(ctx, svc); err != nil {
			return fmt.Errorf("creating seeds finder service %s: %w", svcName, err)
		}
		r.Recorder.Eventf(cluster, corev1.EventTypeNormal, EventServiceCreated, "Created seeds finder service %s", svcName)
		return nil
	} else if err != nil {
		return fmt.Errorf("getting seeds finder service %s: %w", svcName, err)
	}

	// Update the existing service if it has drifted.
	if seedsFinderServiceNeedsUpdate(existing, labels, desiredAnnotations, selectorLabels, desiredPorts, lbSpec) {
		existing.Labels = labels
		existing.Annotations = reconcileAnnotations(existing.Annotations, desiredAnnotations)
		existing.Spec.Selector = selectorLabels
		existing.Spec.Ports = desiredPorts
		if lbSpec.ExternalTrafficPolicy != "" {
			existing.Spec.ExternalTrafficPolicy = corev1.ServiceExternalTrafficPolicy(lbSpec.ExternalTrafficPolicy)
		} else {
			existing.Spec.ExternalTrafficPolicy = corev1.ServiceExternalTrafficPolicyCluster
		}
		if len(lbSpec.LoadBalancerSourceRanges) > 0 {
			existing.Spec.LoadBalancerSourceRanges = lbSpec.LoadBalancerSourceRanges
		} else {
			existing.Spec.LoadBalancerSourceRanges = nil
		}
		log.Info("Updating seeds finder LoadBalancer service", "name", svcName)
		if err := r.Update(ctx, existing); err != nil {
			return fmt.Errorf("updating seeds finder service %s: %w", svcName, err)
		}
		r.Recorder.Eventf(cluster, corev1.EventTypeNormal, EventServiceUpdated, "Updated seeds finder service %s", svcName)
	}

	return nil
}

// cleanupSeedsFinderService deletes the seeds finder LoadBalancer service if it exists.
func (r *AerospikeClusterReconciler) cleanupSeedsFinderService(
	ctx context.Context,
	cluster *ackov1alpha1.AerospikeCluster,
	svcName string,
) error {
	log := logf.FromContext(ctx)

	existing := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: svcName, Namespace: cluster.Namespace}, existing)
	if errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("getting seeds finder service %s for cleanup: %w", svcName, err)
	}

	log.Info("Deleting seeds finder LoadBalancer service", "name", svcName)
	if err := r.Delete(ctx, existing); err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("deleting seeds finder service %s: %w", svcName, err)
	}
	r.Recorder.Eventf(cluster, corev1.EventTypeNormal, EventServiceDeleted, "Deleted seeds finder service %s", svcName)
	return nil
}

// seedsFinderServiceNeedsUpdate returns true if the existing seeds finder service
// differs from the desired state.
func seedsFinderServiceNeedsUpdate(
	existing *corev1.Service,
	desiredLabels map[string]string,
	desiredAnnotations map[string]string,
	desiredSelector map[string]string,
	desiredPorts []corev1.ServicePort,
	lbSpec *ackov1alpha1.LoadBalancerSpec,
) bool {
	if !equalAnnotations(existing.Annotations, desiredAnnotations) {
		return true
	}
	if !maps.Equal(existing.Labels, desiredLabels) {
		return true
	}
	if !maps.Equal(existing.Spec.Selector, desiredSelector) {
		return true
	}
	if servicePortsChanged(existing.Spec.Ports, desiredPorts) {
		return true
	}
	desiredPolicy := corev1.ServiceExternalTrafficPolicyCluster
	if lbSpec.ExternalTrafficPolicy != "" {
		desiredPolicy = corev1.ServiceExternalTrafficPolicy(lbSpec.ExternalTrafficPolicy)
	}
	if existing.Spec.ExternalTrafficPolicy != desiredPolicy {
		return true
	}
	if !sourceRangesEqual(existing.Spec.LoadBalancerSourceRanges, lbSpec.LoadBalancerSourceRanges) {
		return true
	}
	return false
}

// sourceRangesEqual compares two string slices for equality, treating nil and
// empty slices as equivalent.
func sourceRangesEqual(a, b []string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
