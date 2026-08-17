package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"reflect"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	ackov1alpha1 "github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/api/v1alpha1"
	"github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/internal/utils"
)

var serviceMonitorGVK = schema.GroupVersionKind{
	Group:   "monitoring.coreos.com",
	Version: "v1",
	Kind:    "ServiceMonitor",
}

var prometheusRuleGVK = schema.GroupVersionKind{
	Group:   "monitoring.coreos.com",
	Version: "v1",
	Kind:    "PrometheusRule",
}

// reconcileMonitoring reconciles the metrics Service plus the two optional
// Prometheus-Operator resources.
//
// ServiceMonitor and PrometheusRule are best-effort: the CRDs may not be
// installed, and the operator's ClusterRole may not grant access to them (a
// chart install that predates the RBAC entry, or an RBAC-only install trimmed
// by the cluster admin). Both of those conditions must degrade the individual
// feature, never fail the reconcile — an error returned from here reaches
// handleReconcileError, and after maxFailedReconciles consecutive failures the
// circuit breaker freezes scale, rolling restart, config and ACL
// reconciliation for the whole cluster. Losing the entire control loop over an
// optional monitoring integration is never the right trade.
func (r *AerospikeClusterReconciler) reconcileMonitoring(
	ctx context.Context,
	cluster *ackov1alpha1.AerospikeCluster,
) error {
	log := logf.FromContext(ctx)
	log.V(1).Info("Reconciling monitoring resources", "cluster", cluster.Name)

	monitoringEnabled := cluster.Spec.Monitoring != nil && cluster.Spec.Monitoring.Enabled

	// Reconcile metrics Service
	if err := r.reconcileMetricsService(ctx, cluster, monitoringEnabled); err != nil {
		return fmt.Errorf("reconciling metrics service: %w", err)
	}

	// Reconcile ServiceMonitor
	smEnabled := monitoringEnabled &&
		cluster.Spec.Monitoring.ServiceMonitor != nil &&
		cluster.Spec.Monitoring.ServiceMonitor.Enabled

	if err := r.reconcileServiceMonitor(ctx, cluster, smEnabled); err != nil {
		// Only log and skip if the CRD is not installed or the operator is not
		// allowed to touch it; propagate other errors.
		switch {
		case meta.IsNoMatchError(err):
			log.Info("ServiceMonitor CRD not installed, skipping")
		case errors.IsForbidden(err):
			logForbiddenMonitoringResource(
				log, smEnabled, "ServiceMonitor", utils.ServiceMonitorName(cluster.Name), err)
		default:
			return fmt.Errorf("reconciling ServiceMonitor: %w", err)
		}
	}

	// Reconcile PrometheusRule
	prEnabled := monitoringEnabled &&
		cluster.Spec.Monitoring.PrometheusRule != nil &&
		cluster.Spec.Monitoring.PrometheusRule.Enabled

	if err := r.reconcilePrometheusRule(ctx, cluster, prEnabled); err != nil {
		switch {
		case meta.IsNoMatchError(err):
			log.Info("PrometheusRule CRD not installed, skipping")
		case errors.IsForbidden(err):
			logForbiddenMonitoringResource(
				log, prEnabled, "PrometheusRule", utils.PrometheusRuleName(cluster.Name), err)
		default:
			return fmt.Errorf("reconciling PrometheusRule: %w", err)
		}
	}

	return nil
}

// logForbiddenMonitoringResource reports a 403 on an optional monitoring
// resource at a level that matches what the user loses.
//
// A 403 is not only an RBAC gap. The API server returns Forbidden for a
// ValidatingWebhook or policy-engine (Kyverno / Gatekeeper) denial and for
// ResourceQuota exhaustion too, so any of those degrades the feature by this
// same path. Degrading is still the right call — freezing the whole control
// loop over an optional integration is worse — but when the resource is
// *enabled* the operator is silently not delivering something the user asked
// for, and that must not be buried in an Info line. There is no
// MonitoringReady status condition to read instead, so the log is currently the
// only signal; publishing a degraded condition is the proper follow-up.
//
// When the resource is disabled the 403 only blocks the stale-object cleanup
// Get. That is worth reporting, but not at Error on every reconcile for a
// feature nobody asked for.
func logForbiddenMonitoringResource(log logr.Logger, enabled bool, kind, name string, err error) {
	if enabled {
		log.Error(err, "Access denied for an enabled monitoring resource; feature degraded",
			"kind", kind, "name", name)
		return
	}
	log.Info("Access denied for monitoring resource, skipping stale-object cleanup",
		"kind", kind, "name", name, "error", err.Error())
}

func (r *AerospikeClusterReconciler) reconcileMetricsService(
	ctx context.Context,
	cluster *ackov1alpha1.AerospikeCluster,
	enabled bool,
) error {
	log := logf.FromContext(ctx)
	svcName := utils.MetricsServiceName(cluster.Name)

	existing := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: svcName, Namespace: cluster.Namespace}, existing)

	if !enabled {
		if err == nil {
			log.Info("Deleting metrics Service", "name", svcName)
			if delErr := r.Delete(ctx, existing); delErr != nil && !errors.IsNotFound(delErr) {
				return delErr
			}
			return nil
		}
		// A non-NotFound Get error during cleanup must not be swallowed:
		// otherwise a transient API failure makes the operator report success
		// while the metrics Service is never deleted, leaking the resource.
		if !errors.IsNotFound(err) {
			return fmt.Errorf("getting metrics service %s for cleanup: %w", svcName, err)
		}
		return nil
	}

	port := cluster.Spec.Monitoring.Port
	labels := utils.LabelsForCluster(cluster.Name)
	selectorLabels := utils.SelectorLabelsForCluster(cluster.Name)

	desired := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      svcName,
			Namespace: cluster.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: selectorLabels,
			Ports: []corev1.ServicePort{
				{
					Name:       "metrics",
					Port:       port,
					TargetPort: intstr.FromInt32(port),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}

	if errors.IsNotFound(err) {
		if err := r.setOwnerRef(cluster, desired); err != nil {
			return err
		}
		log.Info("Creating metrics Service", "name", svcName)
		return r.Create(ctx, desired)
	} else if err != nil {
		return fmt.Errorf("getting metrics service %s: %w", svcName, err)
	}

	if !metricsServiceNeedsUpdate(existing, desired) {
		log.V(1).Info("Metrics Service unchanged, skipping update", "name", svcName)
		return nil
	}

	existing.Spec.Type = desired.Spec.Type
	existing.Spec.Ports = desired.Spec.Ports
	existing.Spec.Selector = desired.Spec.Selector
	existing.Labels = labels
	log.Info("Updating metrics Service", "name", svcName)
	if err := r.Update(ctx, existing); err != nil {
		return fmt.Errorf("updating metrics service %s: %w", svcName, err)
	}
	return nil
}

func (r *AerospikeClusterReconciler) reconcileServiceMonitor(
	ctx context.Context,
	cluster *ackov1alpha1.AerospikeCluster,
	enabled bool,
) error {
	log := logf.FromContext(ctx)
	smName := utils.ServiceMonitorName(cluster.Name)

	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(serviceMonitorGVK)

	err := r.Get(ctx, types.NamespacedName{Name: smName, Namespace: cluster.Namespace}, existing)

	if !enabled {
		if err == nil {
			log.Info("Deleting ServiceMonitor", "name", smName)
			if delErr := r.Delete(ctx, existing); delErr != nil && !errors.IsNotFound(delErr) {
				return delErr
			}
			return nil
		}
		// NotFound or a missing CRD means there is nothing to clean up. Any
		// other Get error must be surfaced — swallowing it would let a transient
		// API failure leak a stale ServiceMonitor while reporting success.
		if errors.IsNotFound(err) || meta.IsNoMatchError(err) {
			return nil
		}
		return fmt.Errorf("getting ServiceMonitor %s for cleanup: %w", smName, err)
	}

	// CRD not installed — return the error so the caller can decide
	if err != nil && meta.IsNoMatchError(err) {
		return err
	}

	monitoring := cluster.Spec.Monitoring
	interval := monitoring.ServiceMonitor.Interval

	labels := utils.LabelsForCluster(cluster.Name)
	labels = mergeAdditionalLabels(labels, monitoring.ServiceMonitor.Labels)

	selectorLabels := utils.SelectorLabelsForCluster(cluster.Name)

	smSpec := map[string]any{
		"selector": map[string]any{
			"matchLabels": toStringMap(selectorLabels),
		},
		"endpoints": []any{
			map[string]any{
				"port":     "metrics",
				"interval": interval,
				"path":     "/metrics",
			},
		},
		"namespaceSelector": map[string]any{
			"matchNames": []any{cluster.Namespace},
		},
	}

	if errors.IsNotFound(err) {
		sm := &unstructured.Unstructured{}
		sm.SetGroupVersionKind(serviceMonitorGVK)
		sm.SetName(smName)
		sm.SetNamespace(cluster.Namespace)
		sm.SetLabels(labels)
		sm.Object["spec"] = smSpec

		if err := r.setOwnerRef(cluster, sm); err != nil {
			return err
		}
		log.Info("Creating ServiceMonitor", "name", smName)
		return r.Create(ctx, sm)
	} else if err != nil {
		return fmt.Errorf("getting ServiceMonitor %s: %w", smName, err)
	}

	// Skip no-op updates to avoid unnecessary API writes and reconcile loops.
	if !unstructuredResourceChanged(existing, smSpec, labels) {
		log.V(1).Info("ServiceMonitor unchanged, skipping update", "name", smName)
		return nil
	}

	// Update existing
	existing.Object["spec"] = smSpec
	existing.SetLabels(labels)
	log.Info("Updating ServiceMonitor", "name", smName)
	if err := r.Update(ctx, existing); err != nil {
		return fmt.Errorf("updating ServiceMonitor %s: %w", smName, err)
	}
	return nil
}

func toStringMap(m map[string]string) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func metricsServiceNeedsUpdate(existing, desired *corev1.Service) bool {
	return existing.Spec.Type != desired.Spec.Type ||
		!reflect.DeepEqual(existing.Spec.Ports, desired.Spec.Ports) ||
		!maps.Equal(existing.Spec.Selector, desired.Spec.Selector) ||
		!maps.Equal(existing.Labels, desired.Labels)
}

func (r *AerospikeClusterReconciler) reconcilePrometheusRule(
	ctx context.Context,
	cluster *ackov1alpha1.AerospikeCluster,
	enabled bool,
) error {
	log := logf.FromContext(ctx)
	prName := utils.PrometheusRuleName(cluster.Name)

	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(prometheusRuleGVK)

	err := r.Get(ctx, types.NamespacedName{Name: prName, Namespace: cluster.Namespace}, existing)

	if !enabled {
		if err == nil {
			log.Info("Deleting PrometheusRule", "name", prName)
			if delErr := r.Delete(ctx, existing); delErr != nil && !errors.IsNotFound(delErr) {
				return delErr
			}
			return nil
		}
		// NotFound or a missing CRD means there is nothing to clean up. Any
		// other Get error must be surfaced — swallowing it would let a transient
		// API failure leak a stale PrometheusRule while reporting success.
		if errors.IsNotFound(err) || meta.IsNoMatchError(err) {
			return nil
		}
		return fmt.Errorf("getting PrometheusRule %s for cleanup: %w", prName, err)
	}

	// CRD not installed — return the error so the caller can decide
	if err != nil && meta.IsNoMatchError(err) {
		return err
	}

	monitoring := cluster.Spec.Monitoring
	labels := utils.LabelsForCluster(cluster.Name)
	labels = mergeAdditionalLabels(labels, monitoring.PrometheusRule.Labels)

	// Build rule groups: use custom rules if provided, otherwise default rules.
	var groups []any
	if len(monitoring.PrometheusRule.CustomRules) > 0 {
		for _, raw := range monitoring.PrometheusRule.CustomRules {
			var ruleGroup map[string]any
			if err := json.Unmarshal(raw.Raw, &ruleGroup); err != nil {
				return fmt.Errorf("parsing custom PrometheusRule group: %w", err)
			}
			groups = append(groups, ruleGroup)
		}
	} else {
		groups = defaultAlertRules(cluster.Name, cluster.Namespace)
	}

	prSpec := map[string]any{
		"groups": groups,
	}

	if errors.IsNotFound(err) {
		pr := &unstructured.Unstructured{}
		pr.SetGroupVersionKind(prometheusRuleGVK)
		pr.SetName(prName)
		pr.SetNamespace(cluster.Namespace)
		pr.SetLabels(labels)
		pr.Object["spec"] = prSpec

		if err := r.setOwnerRef(cluster, pr); err != nil {
			return err
		}
		log.Info("Creating PrometheusRule", "name", prName)
		return r.Create(ctx, pr)
	} else if err != nil {
		return fmt.Errorf("getting PrometheusRule %s: %w", prName, err)
	}

	// Skip no-op updates to avoid unnecessary API writes and reconcile loops.
	if !unstructuredResourceChanged(existing, prSpec, labels) {
		log.V(1).Info("PrometheusRule unchanged, skipping update", "name", prName)
		return nil
	}

	// Update existing
	existing.Object["spec"] = prSpec
	existing.SetLabels(labels)
	log.Info("Updating PrometheusRule", "name", prName)
	if err := r.Update(ctx, existing); err != nil {
		return fmt.Errorf("updating PrometheusRule %s: %w", prName, err)
	}
	return nil
}

func unstructuredResourceChanged(
	existing *unstructured.Unstructured,
	desiredSpec map[string]any,
	desiredLabels map[string]string,
) bool {
	currentSpec, found, err := unstructured.NestedFieldCopy(existing.Object, "spec")
	if err != nil || !found {
		return true
	}
	return !reflect.DeepEqual(currentSpec, desiredSpec) || !maps.Equal(existing.GetLabels(), desiredLabels)
}

// defaultAlertRules returns the default Prometheus alert rules for an Aerospike cluster.
func defaultAlertRules(clusterName, namespace string) []any {
	jobLabel := fmt.Sprintf("%s-metrics", clusterName)

	return []any{
		map[string]any{
			"name": fmt.Sprintf("%s.rules", clusterName),
			"rules": []any{
				map[string]any{
					"alert": "AerospikeNodeDown",
					"expr":  fmt.Sprintf(`up{job="%s",namespace="%s"} == 0`, jobLabel, namespace),
					"for":   "1m",
					"labels": map[string]any{
						"severity": "critical",
						"cluster":  clusterName,
					},
					"annotations": map[string]any{
						"summary":     fmt.Sprintf("Aerospike node down in cluster %s", clusterName),
						"description": "{{ $labels.pod }} has been down for more than 1 minute.",
					},
				},
				map[string]any{
					"alert": "AerospikeNamespaceStopWrites",
					"expr":  fmt.Sprintf(`aerospike_namespace_stop_writes{job="%s",namespace="%s"} == 1`, jobLabel, namespace),
					"for":   "0m",
					"labels": map[string]any{
						"severity": "critical",
						"cluster":  clusterName,
					},
					"annotations": map[string]any{
						"summary":     fmt.Sprintf("Aerospike namespace stop-writes in cluster %s", clusterName),
						"description": "Namespace {{ $labels.ns }} on {{ $labels.pod }} has stopped accepting writes.",
					},
				},
				map[string]any{
					"alert": "AerospikeHighDiskUsage",
					"expr":  fmt.Sprintf(`aerospike_namespace_device_used_bytes{job="%s",namespace="%s"} / aerospike_namespace_device_total_bytes{job="%s",namespace="%s"} > 0.8`, jobLabel, namespace, jobLabel, namespace),
					"for":   "5m",
					"labels": map[string]any{
						"severity": "warning",
						"cluster":  clusterName,
					},
					"annotations": map[string]any{
						"summary":     fmt.Sprintf("Aerospike high disk usage in cluster %s", clusterName),
						"description": "Namespace {{ $labels.ns }} on {{ $labels.pod }} disk usage is above 80%%.",
					},
				},
				map[string]any{
					"alert": "AerospikeHighMemoryUsage",
					"expr":  fmt.Sprintf(`aerospike_namespace_memory_used_bytes{job="%s",namespace="%s"} / aerospike_namespace_memory_total_bytes{job="%s",namespace="%s"} > 0.8`, jobLabel, namespace, jobLabel, namespace),
					"for":   "5m",
					"labels": map[string]any{
						"severity": "warning",
						"cluster":  clusterName,
					},
					"annotations": map[string]any{
						"summary":     fmt.Sprintf("Aerospike high memory usage in cluster %s", clusterName),
						"description": "Namespace {{ $labels.ns }} on {{ $labels.pod }} memory usage is above 80%%.",
					},
				},
				map[string]any{
					"alert": "AerospikeReconcileStale",
					"expr":  fmt.Sprintf(`time() - acko_last_reconcile_timestamp_seconds{namespace="%s",name="%s"} > 300`, namespace, clusterName),
					"for":   "5m",
					"labels": map[string]any{
						"severity": "warning",
						"cluster":  clusterName,
					},
					"annotations": map[string]any{
						"summary":     fmt.Sprintf("Aerospike operator reconciliation stale for cluster %s", clusterName),
						"description": fmt.Sprintf("The operator has not reconciled cluster %s in the last 5+ minutes.", clusterName),
					},
				},
				map[string]any{
					"alert": "AerospikeClusterSizeMismatch",
					"expr":  fmt.Sprintf(`acko_cluster_ready_pods{namespace="%s",name="%s"} != acko_cluster_as_size{namespace="%s",name="%s"}`, namespace, clusterName, namespace, clusterName),
					"for":   "2m",
					"labels": map[string]any{
						"severity": "warning",
						"cluster":  clusterName,
					},
					"annotations": map[string]any{
						"summary":     fmt.Sprintf("K8s pod count differs from Aerospike cluster-size for cluster %s", clusterName),
						"description": fmt.Sprintf("The number of ready K8s pods does not match the Aerospike cluster-size reported by asinfo for cluster %s.", clusterName),
					},
				},
				map[string]any{
					"alert": "AerospikeOperatorCircuitBreakerActive",
					"expr":  fmt.Sprintf(`acko_circuit_breaker_active{namespace="%s",name="%s"} == 1`, namespace, clusterName),
					"for":   "5m",
					"labels": map[string]any{
						"severity": "critical",
						"cluster":  clusterName,
					},
					"annotations": map[string]any{
						"summary":     fmt.Sprintf("Aerospike operator circuit breaker is active for cluster %s", clusterName),
						"description": fmt.Sprintf("The operator has hit 10+ consecutive reconciliation failures for cluster %s/%s. Manual investigation required.", namespace, clusterName),
					},
				},
			},
		},
	}
}
