package controller

import (
	"context"
	"fmt"
	"maps"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	ackov1alpha1 "github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/api/v1alpha1"
	"github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/internal/podutil"
	"github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/internal/utils"
)

// reconcileHeadlessService creates or updates the headless service for the cluster.
func (r *AerospikeClusterReconciler) reconcileHeadlessService(
	ctx context.Context,
	cluster *ackov1alpha1.AerospikeCluster,
) error {
	log := logf.FromContext(ctx)
	svcName := utils.HeadlessServiceName(cluster.Name)
	labels := utils.LabelsForCluster(cluster.Name)
	selectorLabels := utils.SelectorLabelsForCluster(cluster.Name)

	existing := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: svcName, Namespace: cluster.Namespace}, existing)

	desiredPorts := []corev1.ServicePort{
		{Name: "service", Port: podutil.ServicePort, TargetPort: intstr.FromInt32(podutil.ServicePort), Protocol: corev1.ProtocolTCP},
		{Name: "fabric", Port: podutil.FabricPort, TargetPort: intstr.FromInt32(podutil.FabricPort), Protocol: corev1.ProtocolTCP},
		{Name: "heartbeat", Port: podutil.HeartbeatPort, TargetPort: intstr.FromInt32(podutil.HeartbeatPort), Protocol: corev1.ProtocolTCP},
		{Name: "info", Port: podutil.InfoPort, TargetPort: intstr.FromInt32(podutil.InfoPort), Protocol: corev1.ProtocolTCP},
	}

	// Build desired annotations from custom metadata.
	var desiredAnnotations map[string]string
	if cluster.Spec.HeadlessService != nil && cluster.Spec.HeadlessService.Metadata != nil {
		if cluster.Spec.HeadlessService.Metadata.Annotations != nil {
			desiredAnnotations = make(map[string]string)
			maps.Copy(desiredAnnotations, cluster.Spec.HeadlessService.Metadata.Annotations)
		}
		labels = mergeAdditionalLabels(labels, cluster.Spec.HeadlessService.Metadata.Labels)
	}

	if errors.IsNotFound(err) {
		svc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:        svcName,
				Namespace:   cluster.Namespace,
				Labels:      labels,
				Annotations: desiredAnnotations,
			},
			Spec: corev1.ServiceSpec{
				ClusterIP:                corev1.ClusterIPNone,
				Selector:                 selectorLabels,
				PublishNotReadyAddresses: true,
				Ports:                    desiredPorts,
			},
		}
		if err := r.setOwnerRef(cluster, svc); err != nil {
			return err
		}
		log.Info("Creating headless service", "name", svcName)
		if err := r.Create(ctx, svc); err != nil {
			return fmt.Errorf("creating headless service %s: %w", svcName, err)
		}
		r.Recorder.Eventf(cluster, corev1.EventTypeNormal, EventServiceCreated, "Created headless service %s", svcName)
		return nil
	} else if err != nil {
		return fmt.Errorf("getting headless service %s: %w", svcName, err)
	}

	if headlessServiceNeedsUpdate(existing, labels, desiredAnnotations, selectorLabels, desiredPorts) {
		existing.Labels = labels
		existing.Annotations = reconcileAnnotations(existing.Annotations, desiredAnnotations)
		existing.Spec.Ports = desiredPorts
		existing.Spec.Selector = selectorLabels
		existing.Spec.PublishNotReadyAddresses = true
		log.Info("Updating headless service", "name", svcName)
		if err := r.Update(ctx, existing); err != nil {
			return fmt.Errorf("updating headless service %s: %w", svcName, err)
		}
		r.Recorder.Eventf(cluster, corev1.EventTypeNormal, EventServiceUpdated, "Updated headless service %s", svcName)
	}

	return nil
}

func headlessServiceNeedsUpdate(
	existing *corev1.Service,
	desiredLabels map[string]string,
	desiredAnnotations map[string]string,
	desiredSelector map[string]string,
	desiredPorts []corev1.ServicePort,
) bool {
	return !equalAnnotations(existing.Annotations, desiredAnnotations) ||
		!maps.Equal(existing.Labels, desiredLabels) ||
		!maps.Equal(existing.Spec.Selector, desiredSelector) ||
		!existing.Spec.PublishNotReadyAddresses ||
		servicePortsChanged(existing.Spec.Ports, desiredPorts)
}

// mergeAdditionalLabels adds user-specified labels without allowing them to
// overwrite operator-managed identity labels that selectors and cleanup rely on.
func mergeAdditionalLabels(base, additional map[string]string) map[string]string {
	if len(additional) == 0 {
		return base
	}
	for k, v := range additional {
		if _, exists := base[k]; exists {
			continue
		}
		base[k] = v
	}
	return base
}

// servicePortsChanged returns true if the existing ports differ from desired ports.
func servicePortsChanged(existing, desired []corev1.ServicePort) bool {
	if len(existing) != len(desired) {
		return true
	}
	for i, p := range existing {
		d := desired[i]
		if p.Name != d.Name || p.Port != d.Port ||
			p.TargetPort != d.TargetPort || p.Protocol != d.Protocol {
			return true
		}
	}
	return false
}

// equalAnnotations checks whether the existing annotations already match the
// desired state after reconciliation. It builds the expected annotation map
// (preserving system annotations, applying desired) and compares it to actual.
// This correctly detects additions, updates, and removals of operator-managed annotations.
func equalAnnotations(actual, desired map[string]string) bool {
	reconciled := reconcileAnnotations(actual, desired)
	return maps.Equal(actual, reconciled)
}

// reconcileAnnotations builds the target annotations map by preserving
// Kubernetes system annotations from existing and overlaying operator-managed
// annotations from desired. Operator-managed annotations not present in desired
// are removed, allowing annotation cleanup when users remove entries from the CR.
//
// System annotations (containing "kubernetes.io/" or "k8s.io/" in the key) are
// preserved to avoid conflicts with Kubernetes and admission controllers.
func reconcileAnnotations(existing, desired map[string]string) map[string]string {
	if existing == nil && desired == nil {
		return nil
	}
	result := make(map[string]string)
	// Preserve system annotations from existing.
	for k, v := range existing {
		if isSystemAnnotation(k) {
			result[k] = v
		}
	}
	// Overlay desired operator annotations.
	maps.Copy(result, desired)
	if len(result) == 0 {
		return nil
	}
	return result
}

// isSystemAnnotation returns true for annotations managed by Kubernetes itself
// or by admission controllers (e.g., kubectl.kubernetes.io/last-applied-configuration).
// It checks the domain prefix of the annotation key rather than using substring
// matching, so keys like "bypass-kubernetes.io/x" are correctly excluded.
func isSystemAnnotation(key string) bool {
	prefix, _, hasDomain := strings.Cut(key, "/")
	if !hasDomain {
		return false
	}
	return prefix == "kubernetes.io" || strings.HasSuffix(prefix, ".kubernetes.io") ||
		prefix == "k8s.io" || strings.HasSuffix(prefix, ".k8s.io")
}
