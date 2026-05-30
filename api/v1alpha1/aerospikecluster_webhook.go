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

package v1alpha1

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var aerospikeclusterlog = logf.Log.WithName("aerospikecluster-resource")

var tomlBareKeyRe = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// prometheusDurationRe mirrors the validation pattern the Prometheus Operator
// applies to its Duration type (used by ServiceMonitor endpoints[].interval).
// The reconciler writes monitoring.serviceMonitor.interval verbatim into the
// ServiceMonitor's scrape interval (reconciler_monitoring.go), so an interval
// that does not match this pattern is accepted by the Kubernetes API server but
// rejected by the Prometheus Operator's own CRD schema when the ServiceMonitor
// is applied — surfacing as an opaque reconcile-time failure instead of a clear
// admission error. We replicate the pattern locally (like serviceMonitorGVK)
// rather than importing prometheus-operator, which is not a module dependency.
// Reference: monitoring.coreos.com/v1 Duration validation.
var prometheusDurationRe = regexp.MustCompile(
	`^(0|(([0-9]+)y)?(([0-9]+)w)?(([0-9]+)d)?(([0-9]+)h)?(([0-9]+)m)?(([0-9]+)s)?(([0-9]+)ms)?)$`)

const (
	maxCEClusterSize     = 8
	maxCENamespaces      = 2
	minCEMajorVersion    = 8
	defaultProtoFdMax    = 15000
	defaultHeartbeatMode = "mesh"

	defaultScrapeInterval = "30s"
)

// SetupWebhookWithManager registers the webhooks for AerospikeCluster.
func (r *AerospikeCluster) SetupWebhookWithManager(mgr ctrl.Manager) error {
	// APIReader bypasses the manager's delegating cache. The cached client can
	// swallow or wrap CRD-absent errors so meta.IsNoMatchError fails to match;
	// the API reader hits the apiserver directly and returns the original
	// no-match error, which is what validateServiceMonitorUniqueness relies on.
	return ctrl.NewWebhookManagedBy(mgr, r).
		WithDefaulter(&AerospikeClusterDefaulter{}).
		WithValidator(&AerospikeClusterValidator{
			Client:    mgr.GetClient(),
			APIReader: mgr.GetAPIReader(),
		}).
		Complete()
}

// serviceMonitorGVK is the GroupVersionKind used to query ServiceMonitors via
// the unstructured client. Mirrors internal/controller.serviceMonitorGVK; kept
// local here to avoid an import cycle from api -> internal/controller.
var serviceMonitorGVK = schema.GroupVersionKind{
	Group:   "monitoring.coreos.com",
	Version: "v1",
	Kind:    "ServiceMonitor",
}

// +kubebuilder:webhook:path=/mutate-acko-io-v1alpha1-aerospikecluster,mutating=true,failurePolicy=fail,sideEffects=None,groups=acko.io,resources=aerospikeclusters,verbs=create;update,versions=v1alpha1,name=maerospikecluster.kb.io,admissionReviewVersions=v1

// AerospikeClusterDefaulter implements admission.Defaulter for AerospikeCluster.
type AerospikeClusterDefaulter struct{}

var _ admission.Defaulter[*AerospikeCluster] = &AerospikeClusterDefaulter{}

// Default implements admission.Defaulter[*AerospikeCluster].
func (d *AerospikeClusterDefaulter) Default(ctx context.Context, cluster *AerospikeCluster) error {
	aerospikeclusterlog.Info("Defaulting", "name", cluster.Name, "namespace", cluster.Namespace)

	if cluster.Spec.AerospikeConfig == nil {
		cluster.Spec.AerospikeConfig = &AerospikeConfigSpec{
			Value: make(map[string]any),
		}
	}
	if cluster.Spec.AerospikeConfig.Value == nil {
		cluster.Spec.AerospikeConfig.Value = make(map[string]any)
	}

	if err := d.defaultServiceConfig(cluster); err != nil {
		return err
	}
	if err := d.defaultNetworkConfig(cluster); err != nil {
		return err
	}
	d.defaultMonitoring(cluster)
	d.defaultHostNetwork(cluster)

	return nil
}

// defaultServiceConfig sets defaults in aerospikeConfig.service.
// Returns an error if the "service" key exists but is not a map.
func (d *AerospikeClusterDefaulter) defaultServiceConfig(cluster *AerospikeCluster) error {
	config := cluster.Spec.AerospikeConfig.Value

	serviceSection, err := getOrCreateMapSection(config, "service")
	if err != nil {
		return err
	}

	if _, exists := serviceSection["cluster-name"]; !exists {
		serviceSection["cluster-name"] = cluster.Name
	}

	if _, exists := serviceSection["proto-fd-max"]; !exists {
		serviceSection["proto-fd-max"] = defaultProtoFdMax
	}

	config["service"] = serviceSection
	return nil
}

// defaultNetworkConfig sets defaults in aerospikeConfig.network.
// Returns an error if the "network" key (or any sub-section key) exists but is not a map.
func (d *AerospikeClusterDefaulter) defaultNetworkConfig(cluster *AerospikeCluster) error {
	config := cluster.Spec.AerospikeConfig.Value

	networkSection, err := getOrCreateMapSection(config, "network")
	if err != nil {
		return err
	}

	// Default values for each network sub-section.
	networkDefaults := map[string]map[string]any{
		"service":   {"port": int(DefaultServicePort)},
		"heartbeat": {"port": int(DefaultHeartbeatPort), "mode": defaultHeartbeatMode},
		"fabric":    {"port": int(DefaultFabricPort)},
	}

	for name, defs := range networkDefaults {
		section, err := getOrCreateMapSection(networkSection, name)
		if err != nil {
			return err
		}
		for k, v := range defs {
			if _, exists := section[k]; !exists {
				section[k] = v
			}
		}
		networkSection[name] = section
	}

	config["network"] = networkSection
	return nil
}

// defaultMonitoring sets default values for the monitoring spec when enabled.
func (d *AerospikeClusterDefaulter) defaultMonitoring(cluster *AerospikeCluster) {
	if cluster.Spec.Monitoring == nil || !cluster.Spec.Monitoring.Enabled {
		return
	}

	m := cluster.Spec.Monitoring
	if m.ExporterImage == "" {
		m.ExporterImage = DefaultExporterImage
	}
	if m.Port == 0 {
		m.Port = DefaultExporterPort
	}
	if m.ServiceMonitor != nil && m.ServiceMonitor.Enabled && m.ServiceMonitor.Interval == "" {
		m.ServiceMonitor.Interval = defaultScrapeInterval
	}
}

// defaultHostNetwork sets defaults for hostNetwork mode.
func (d *AerospikeClusterDefaulter) defaultHostNetwork(cluster *AerospikeCluster) {
	if cluster.Spec.PodSpec == nil || !cluster.Spec.PodSpec.HostNetwork {
		return
	}

	// Default multiPodPerHost to false when hostNetwork is enabled
	if cluster.Spec.PodSpec.MultiPodPerHost == nil {
		f := false
		cluster.Spec.PodSpec.MultiPodPerHost = &f
	}

	// Default dnsPolicy to ClusterFirstWithHostNet
	if cluster.Spec.PodSpec.DNSPolicy == "" {
		cluster.Spec.PodSpec.DNSPolicy = corev1.DNSClusterFirstWithHostNet
	}
}

// getOrCreateMapSection returns the sub-map at key or creates a new one.
// Returns an error when the key exists but is not a map (indicating invalid config).
func getOrCreateMapSection(m map[string]any, key string) (map[string]any, error) {
	if existing, ok := m[key]; ok {
		if existingMap, ok := existing.(map[string]any); ok {
			return existingMap, nil
		}
		return nil, fmt.Errorf("config key %q has type %T, expected map", key, existing)
	}
	newMap := make(map[string]any)
	m[key] = newMap
	return newMap, nil
}

// +kubebuilder:webhook:path=/validate-acko-io-v1alpha1-aerospikecluster,mutating=false,failurePolicy=fail,sideEffects=None,groups=acko.io,resources=aerospikeclusters,verbs=create;update,versions=v1alpha1,name=vaerospikecluster.kb.io,admissionReviewVersions=v1

// AerospikeClusterValidator implements admission.Validator for AerospikeCluster.
//
// Client is used by validations that need to query the cluster (e.g.
// validateServiceMonitorUniqueness, which Gets the ServiceMonitor in the
// target namespace). It may be nil in unit tests that exercise pure
// in-memory validations; client-dependent checks must be nil-safe.
//
// APIReader is a non-cached reader (mgr.GetAPIReader()). It is used for the
// ServiceMonitor uniqueness check so that meta.IsNoMatchError surfaces
// reliably when the monitoring CRD is absent — the cached delegating client
// can swallow or wrap that signal. Tests may leave APIReader nil; the check
// falls back to Client in that case.
//
// +kubebuilder:object:generate=false
type AerospikeClusterValidator struct {
	Client    client.Client
	APIReader client.Reader
}

var _ admission.Validator[*AerospikeCluster] = &AerospikeClusterValidator{}

// ValidateCreate implements admission.Validator[*AerospikeCluster].
func (v *AerospikeClusterValidator) ValidateCreate(ctx context.Context, cluster *AerospikeCluster) (admission.Warnings, error) {
	aerospikeclusterlog.Info("Validating create", "name", cluster.Name)
	return v.validateWithCtx(ctx, cluster)
}

// ValidateUpdate implements admission.Validator[*AerospikeCluster].
func (v *AerospikeClusterValidator) ValidateUpdate(ctx context.Context, oldCluster, cluster *AerospikeCluster) (admission.Warnings, error) {
	aerospikeclusterlog.Info("Validating update", "name", cluster.Name)

	// templateRef is immutable.
	//
	// Swapping the referenced template on a live cluster would silently
	// re-base every templated default (storage class, scheduling, image,
	// size, ...) and could cross between operator-managed configurations
	// without any progressive rollout. The merged spec is written into the
	// snapshot at admission time and consumed by the controller; a swap
	// here is effectively a hidden mass-edit.
	//
	// Default policy is immutable. If a future change introduces a
	// supported swap workflow it should require an explicit annotation +
	// no overrides, not a free swap.
	oldRef, newRef := oldCluster.Spec.TemplateRef, cluster.Spec.TemplateRef
	switch {
	case oldRef == nil && newRef != nil:
		return nil, fmt.Errorf(
			"spec.templateRef is immutable: cannot add templateRef to an existing cluster; create a new cluster that references the template")
	case oldRef != nil && newRef == nil:
		return nil, fmt.Errorf(
			"spec.templateRef is immutable: cannot remove templateRef from a cluster that was created with one (was %q)",
			oldRef.Name)
	case oldRef != nil && newRef != nil && oldRef.Name != newRef.Name:
		return nil, fmt.Errorf(
			"spec.templateRef is immutable: cannot change templateRef from %q to %q; create a new cluster instead",
			oldRef.Name, newRef.Name)
	}

	// Don't allow changing operations while one is InProgress
	if oldCluster.Status.OperationStatus != nil &&
		oldCluster.Status.OperationStatus.Phase == AerospikePhaseInProgress {
		oldOps := oldCluster.Spec.Operations
		newOps := cluster.Spec.Operations
		// Block if operation list changed (added, removed, or replaced)
		if len(oldOps) != len(newOps) {
			return nil, fmt.Errorf("cannot change operations while operation %q is InProgress", oldCluster.Status.OperationStatus.ID)
		}
		for i := range oldOps {
			if oldOps[i].ID != newOps[i].ID || oldOps[i].Kind != newOps[i].Kind ||
				!slices.Equal(oldOps[i].PodList, newOps[i].PodList) {
				return nil, fmt.Errorf("cannot change operations while operation %q is InProgress", oldCluster.Status.OperationStatus.ID)
			}
		}
	}

	// Prevent simultaneous addition and removal of rack IDs (which risks data loss
	// from a rename-like operation). Pure additions or pure removals are fine.
	//
	// effectiveRackIDs resolves both specs the same way the reconciler's getRacks
	// does: when rackConfig is nil or has no racks, the cluster runs a single
	// default rack with ID 0. Without that fallback the guard only fired when both
	// specs carried an explicit rackConfig, so dropping rackConfig entirely
	// (explicit racks -> default rack 0) — which tears down every explicit-rack
	// StatefulSet and creates a fresh rack-0 one — slipped through unchecked even
	// though it is exactly the rename-like, data-loss-risky operation the guard
	// exists to block.
	oldIDs := effectiveRackIDs(oldCluster.Spec.RackConfig)
	newIDs := effectiveRackIDs(cluster.Spec.RackConfig)

	// Collect IDs removed (in old but not new) and IDs added (in new but not old).
	var removedIDs []int
	for id := range oldIDs {
		if !newIDs[id] {
			removedIDs = append(removedIDs, id)
		}
	}
	var addedIDs []int
	for id := range newIDs {
		if !oldIDs[id] {
			addedIDs = append(addedIDs, id)
		}
	}
	if len(removedIDs) > 0 && len(addedIDs) > 0 {
		// Sort for a deterministic error message (map iteration order is random).
		slices.Sort(addedIDs)
		slices.Sort(removedIDs)
		// Rack ID 0 is the implicit default rack and can never be named explicitly
		// (validateRackConfig reserves it), so a transition that adds or removes it —
		// i.e. introducing or dropping rack-awareness on an existing cluster — has no
		// valid two-step path. Steer the user to the only safe route instead of
		// suggesting an impossible "add then remove".
		if slices.Contains(addedIDs, 0) || slices.Contains(removedIDs, 0) {
			return nil, fmt.Errorf("cannot add new rack IDs %v and remove existing rack IDs %v in the same update: this switches the cluster between the implicit default rack (ID 0) and explicit racks, which recreates StatefulSets and risks data loss; introduce or remove rack-awareness by creating a new cluster and migrating data instead", addedIDs, removedIDs)
		}
		return nil, fmt.Errorf("cannot add new rack IDs %v and remove existing rack IDs %v in the same update; please do this in two separate steps (first add, then remove, or vice versa)", addedIDs, removedIDs)
	}

	return v.validateWithCtx(ctx, cluster)
}

// ValidateDelete implements admission.Validator[*AerospikeCluster].
func (v *AerospikeClusterValidator) ValidateDelete(ctx context.Context, cluster *AerospikeCluster) (admission.Warnings, error) {
	return nil, nil
}

// effectiveRackIDs returns the set of rack IDs the cluster will actually run,
// mirroring the controller's getRacks fallback: an absent or empty rackConfig
// means a single default rack with ID 0. Keeping this in sync with getRacks lets
// ValidateUpdate's add/remove guard reason about the real rack topology rather
// than the raw spec, so the "drop rackConfig entirely" transition is treated as
// the simultaneous remove+add it really is.
func effectiveRackIDs(rackConfig *RackConfig) map[int]bool {
	if rackConfig == nil || len(rackConfig.Racks) == 0 {
		return map[int]bool{0: true}
	}
	ids := make(map[int]bool, len(rackConfig.Racks))
	for _, rack := range rackConfig.Racks {
		ids[rack.ID] = true
	}
	return ids
}

// serviceMonitorName returns the ServiceMonitor name produced by the
// reconciler for the given cluster. The format must stay in sync with
// internal/utils.ServiceMonitorName; both are kept identical and trivially
// short to avoid a cross-package dependency (api/v1alpha1 cannot import
// internal/utils, which already depends on api/v1alpha1). A drift guard
// (TestServiceMonitorNameMatchesUtilsHelper in webhook_drift_test.go) pins
// the two implementations together at test time.
func serviceMonitorName(cluster *AerospikeCluster) string {
	return fmt.Sprintf("%s-monitor", cluster.Name)
}

// validateWithCtx wraps validate() with checks that need cluster access via
// the Kubernetes client. Pure-config validations remain in validate() so they
// stay testable without a client.
func (v *AerospikeClusterValidator) validateWithCtx(ctx context.Context, cluster *AerospikeCluster) (admission.Warnings, error) {
	warnings, err := v.validate(cluster)
	if err != nil {
		// Surface config errors first; client-dependent checks would only add noise.
		return warnings, err
	}

	if smErr := v.validateServiceMonitorUniqueness(ctx, cluster); smErr != nil {
		return warnings, smErr
	}

	return warnings, nil
}

// validateServiceMonitorUniqueness rejects the CR if a ServiceMonitor with the
// name this cluster would produce already exists in the same namespace and is
// not owned by this cluster. This protects against silent overwrites when
// multiple AerospikeCluster CRs (or operator instances) are pointed at the
// same namespace.
//
// Idempotent updates remain allowed: a ServiceMonitor whose owner reference
// matches the cluster's UID is treated as belonging to this CR.
//
// The check is skipped when:
//   - monitoring or serviceMonitor is disabled (no SM would be created),
//   - the validator has no reader wired in (unit-test path),
//   - the ServiceMonitor CRD is not installed (the reconciler will surface
//     that error at apply time, with more useful context).
//
// We Get the SM by name (O(1)) instead of listing every ServiceMonitor in
// the namespace and filtering — listing was O(N) per admission and paid the
// cost on every CR create/update in busy namespaces. We prefer APIReader
// (non-cached) over Client because the delegating cache can swallow or
// wrap meta.IsNoMatchError when the monitoring CRD is absent.
func (v *AerospikeClusterValidator) validateServiceMonitorUniqueness(
	ctx context.Context,
	cluster *AerospikeCluster,
) error {
	reader := v.readerForServiceMonitor()
	if reader == nil {
		return nil
	}
	if cluster.Spec.Monitoring == nil || !cluster.Spec.Monitoring.Enabled {
		return nil
	}
	if cluster.Spec.Monitoring.ServiceMonitor == nil || !cluster.Spec.Monitoring.ServiceMonitor.Enabled {
		return nil
	}

	smName := serviceMonitorName(cluster)

	sm := &unstructured.Unstructured{}
	sm.SetGroupVersionKind(serviceMonitorGVK)

	err := reader.Get(ctx, client.ObjectKey{Name: smName, Namespace: cluster.Namespace}, sm)
	switch {
	case err == nil:
		// Fall through to ownership check below.
	case apierrors.IsNotFound(err):
		// Happy path: nothing already there with this name.
		return nil
	case meta.IsNoMatchError(err):
		// CRD not installed — the reconciler will skip ServiceMonitor reconciliation
		// with a clearer log message, so don't fail admission here.
		return nil
	default:
		return fmt.Errorf("getting ServiceMonitor %q in namespace %q: %w", smName, cluster.Namespace, err)
	}

	// If owned by this cluster (UID match), the existing SM is ours; allow
	// idempotent updates.
	for _, ref := range sm.GetOwnerReferences() {
		if ref.UID == cluster.UID {
			return nil
		}
	}

	// Identify the conflicting owner to make the error actionable.
	conflict := describeOwnerForError(sm)
	return fmt.Errorf(
		"ServiceMonitor %q already exists in namespace %q (%s); disable monitoring.serviceMonitor on this AerospikeCluster or rename to avoid the conflict",
		smName, cluster.Namespace, conflict,
	)
}

// readerForServiceMonitor returns the non-cached APIReader when wired in,
// falling back to Client for tests that only set the cached client. Returns
// nil if neither is set so callers can short-circuit to a no-op.
func (v *AerospikeClusterValidator) readerForServiceMonitor() client.Reader {
	if v.APIReader != nil {
		return v.APIReader
	}
	if v.Client != nil {
		return v.Client
	}
	return nil
}

// describeOwnerForError returns a short human-readable description of the
// existing ServiceMonitor's owner, suitable for inclusion in admission errors.
func describeOwnerForError(sm *unstructured.Unstructured) string {
	for _, ref := range sm.GetOwnerReferences() {
		if ref.Kind == "AerospikeCluster" {
			return fmt.Sprintf("owned by AerospikeCluster %q", ref.Name)
		}
	}
	if refs := sm.GetOwnerReferences(); len(refs) > 0 {
		return fmt.Sprintf("owned by %s %q", refs[0].Kind, refs[0].Name)
	}
	return "no owner reference matches this AerospikeCluster"
}

// validate performs all CE-specific validations.
func (v *AerospikeClusterValidator) validate(cluster *AerospikeCluster) (admission.Warnings, error) {
	var allErrors []string
	var warnings admission.Warnings

	// Validate size and image (may be supplied by template when templateRef is set).
	sizeErrors, imageErrors, imageWarnings := v.validateSizeAndImage(cluster)
	allErrors = append(allErrors, sizeErrors...)
	allErrors = append(allErrors, imageErrors...)
	warnings = append(warnings, imageWarnings...)

	// Validate aerospikeConfig
	if cluster.Spec.AerospikeConfig != nil {
		configErrors, configWarnings := v.validateAerospikeConfig(cluster.Spec.AerospikeConfig.Value)
		allErrors = append(allErrors, configErrors...)
		warnings = append(warnings, configWarnings...)
	}

	// Validate access control
	if cluster.Spec.AerospikeAccessControl != nil {
		acErrors := v.validateAccessControl(cluster.Spec.AerospikeAccessControl)
		allErrors = append(allErrors, acErrors...)
	}

	// Validate hostNetwork + multiPodPerHost
	if cluster.Spec.PodSpec != nil && cluster.Spec.PodSpec.HostNetwork {
		if cluster.Spec.PodSpec.MultiPodPerHost != nil && *cluster.Spec.PodSpec.MultiPodPerHost {
			warnings = append(warnings, "hostNetwork=true with multiPodPerHost=true may cause port conflicts")
		}
		if cluster.Spec.PodSpec.DNSPolicy != "" && cluster.Spec.PodSpec.DNSPolicy != corev1.DNSClusterFirstWithHostNet {
			warnings = append(warnings, "hostNetwork=true with dnsPolicy other than ClusterFirstWithHostNet may cause DNS resolution issues")
		}
	}

	// Validate podSpec.sidecars[] and podSpec.initContainers[] container names.
	if cluster.Spec.PodSpec != nil {
		allErrors = append(allErrors, validatePodSpecContainerNames(cluster.Spec.PodSpec)...)
	}

	// Validate rack config
	if cluster.Spec.RackConfig != nil {
		rackErrors := v.validateRackConfig(cluster.Spec.RackConfig)
		allErrors = append(allErrors, rackErrors...)

		// Reject more racks than pods. The reconciler distributes spec.size
		// evenly across racks (getRackSize); when len(racks) > size at least
		// one rack is assigned 0 replicas, producing a 0-replica StatefulSet
		// that never carries data. The size check is skipped when size is
		// supplied later by a template (size == 0 && templateRef != nil).
		numRacks := len(cluster.Spec.RackConfig.Racks)
		sizeKnownNow := cluster.Spec.Size != 0 || cluster.Spec.TemplateRef == nil
		if numRacks > 0 && sizeKnownNow && numRacks > int(cluster.Spec.Size) {
			allErrors = append(allErrors, fmt.Sprintf(
				"rackConfig defines %d racks but spec.size is %d; each rack must get at least 1 pod, so the rack count must not exceed spec.size",
				numRacks, cluster.Spec.Size))
		}
	}

	// Validate monitoring
	if cluster.Spec.Monitoring != nil {
		if cluster.Spec.Monitoring.Enabled {
			mErrors, mWarnings := v.validateMonitoring(cluster.Spec.Monitoring)
			allErrors = append(allErrors, mErrors...)
			warnings = append(warnings, mWarnings...)
		} else {
			// Warn if sub-features are enabled but monitoring itself is disabled
			if cluster.Spec.Monitoring.ServiceMonitor != nil && cluster.Spec.Monitoring.ServiceMonitor.Enabled {
				warnings = append(warnings, "monitoring.serviceMonitor.enabled is true but monitoring.enabled is false; ServiceMonitor will not be created")
			}
			if cluster.Spec.Monitoring.PrometheusRule != nil && cluster.Spec.Monitoring.PrometheusRule.Enabled {
				warnings = append(warnings, "monitoring.prometheusRule.enabled is true but monitoring.enabled is false; PrometheusRule will not be created")
			}
		}
	}

	// Validate storage
	if cluster.Spec.Storage != nil {
		storageErrors, storageWarnings := v.validateStorage(cluster.Spec.Storage)
		allErrors = append(allErrors, storageErrors...)
		warnings = append(warnings, storageWarnings...)
	}

	// Validate network port uniqueness
	if cluster.Spec.AerospikeConfig != nil {
		portErrors := v.validateNetworkPortUniqueness(cluster)
		allErrors = append(allErrors, portErrors...)
	}

	// Validate replication-factor, work directory, batch size, max unavailable, and operations
	rfErrors := v.validateReplicationFactor(cluster)
	allErrors = append(allErrors, rfErrors...)
	warnings = append(warnings, v.validateWorkDirectory(cluster)...)
	warnings = append(warnings, v.validateBatchSize(cluster)...)
	warnings = append(warnings, v.validateRackBatchSize(cluster)...)
	warnings = append(warnings, v.validateMaxUnavailable(cluster)...)
	if len(cluster.Spec.Operations) > 0 {
		allErrors = append(allErrors, v.validateOperations(cluster.Spec.Operations)...)
	}

	// Validate overrides requires templateRef
	if cluster.Spec.Overrides != nil && cluster.Spec.TemplateRef == nil {
		allErrors = append(allErrors, "spec.overrides can only be set when spec.templateRef is specified")
	}

	// Validate the contents of spec.overrides against CE constraints. Without
	// this, users could set spec.overrides.image to an Enterprise tag, or push
	// an xdr/tls/security-enterprise stanza via overrides.aerospikeConfig and
	// silently bypass the same checks the cluster-spec path enforces. The
	// template webhook validates these fields at template-create time, but
	// overrides only flow through the cluster webhook, so this check is the
	// only line of defence for them.
	if cluster.Spec.Overrides != nil {
		allErrors = append(allErrors, v.validateOverrides(cluster.Spec.Overrides)...)
	}

	if len(allErrors) > 0 {
		return warnings, fmt.Errorf("validation failed: %s", strings.Join(allErrors, "; "))
	}

	return warnings, nil
}

// validateOverrides applies the same CE constraints to spec.overrides that
// the AerospikeClusterTemplate webhook applies to template specs at create
// time:
//   - image must not reference an Enterprise build,
//   - size must be in the CE-allowed range (1–8) when explicitly set,
//   - aerospikeConfig.namespaceDefaults must not contain enterprise-only
//     namespace keys (compression, strong-consistency, ...) or xdr/tls/
//     enterprise security sub-keys,
//   - aerospikeConfig.service must not contain xdr/tls/enterprise security
//     sub-keys.
//
// nil sections are skipped silently — overrides intentionally allows partial
// patches and most fields are optional.
func (v *AerospikeClusterValidator) validateOverrides(overrides *AerospikeClusterTemplateSpec) []string {
	if overrides == nil {
		return nil
	}
	var errs []string

	// Image: same rules as spec.image — reject the enterprise repository name
	// and any "ee-"/"ent-"/contains-"enterprise" tag.
	if overrides.Image != "" {
		imageLower := strings.ToLower(overrides.Image)
		switch {
		case strings.Contains(imageLower, "aerospike-server-enterprise"):
			errs = append(errs, fmt.Sprintf(
				"spec.overrides.image %q references the enterprise repository "+
					"(aerospike-server-enterprise); CE clusters must use a CE image "+
					"such as aerospike:ce-8.1.1.1",
				overrides.Image))
		case strings.Contains(imageLower, "enterprise") || isEnterpriseTag(overrides.Image):
			errs = append(errs, fmt.Sprintf(
				"spec.overrides.image %q is an Enterprise Edition image; only Community Edition images are allowed",
				overrides.Image))
		}
	}

	// Size: respect the CE 1–8 bound when the override sets a value.
	if overrides.Size != nil {
		size := *overrides.Size
		if size < 1 || size > maxCEClusterSize {
			errs = append(errs, fmt.Sprintf(
				"spec.overrides.size %d must be between 1 and %d (CE limit)", size, maxCEClusterSize))
		}
	}

	// aerospikeConfig: reject enterprise-only stanzas/keys reachable from the
	// override map. Mirrors the template webhook's check (V-T xdr/tls/security/
	// enterpriseOnlyNamespaceKeys) — see validateTemplateConfigBannedKeys.
	if overrides.AerospikeConfig != nil {
		if overrides.AerospikeConfig.NamespaceDefaults != nil {
			errs = append(errs, validateTemplateConfigBannedKeys(
				"spec.overrides.aerospikeConfig.namespaceDefaults",
				overrides.AerospikeConfig.NamespaceDefaults.Value,
				true,
			)...)
		}
		if overrides.AerospikeConfig.Service != nil {
			errs = append(errs, validateTemplateConfigBannedKeys(
				"spec.overrides.aerospikeConfig.service",
				overrides.AerospikeConfig.Service.Value,
				false,
			)...)
		}
	}

	return errs
}

// validateAerospikeConfig checks the Aerospike configuration map.
func (v *AerospikeClusterValidator) validateAerospikeConfig(config map[string]any) ([]string, admission.Warnings) {
	var errors []string
	var warnings admission.Warnings

	// CE does not support XDR
	if _, exists := config["xdr"]; exists {
		errors = append(errors, "aerospikeConfig must not contain 'xdr' section (XDR is Enterprise-only)")
	}

	// CE does not support TLS
	if _, exists := config["tls"]; exists {
		errors = append(errors, "aerospikeConfig must not contain 'tls' section (TLS is Enterprise-only)")
	}

	// Count namespaces (CE limit: 2)
	if nsSection, exists := config["namespaces"]; exists {
		switch ns := nsSection.(type) {
		case []any:
			if len(ns) > maxCENamespaces {
				errors = append(errors, fmt.Sprintf("aerospikeConfig.namespaces count %d exceeds CE maximum of %d", len(ns), maxCENamespaces))
			}
			// Track namespace names to reject duplicates. configgen
			// (generateNamespaceSections) emits one `namespace <name> { ... }`
			// block per entry without de-duplicating, so two entries that share a
			// name produce two identical-named blocks. aerospikd rejects a
			// duplicate namespace definition at startup, leaving the pod in a
			// permanent CrashLoopBackOff. Reject it here so the user gets an
			// actionable admission error instead of a runtime crash — same
			// fail-fast philosophy as the map/scalar shape checks below.
			seenNsNames := make(map[string]bool, len(ns))
			// Validate each namespace's config
			for i, nsEntry := range ns {
				nsMap, ok := nsEntry.(map[string]any)
				if !ok {
					errors = append(errors, fmt.Sprintf("aerospikeConfig.namespaces[%d] must be a map, got %T", i, nsEntry))
					continue
				}
				if nameVal, hasName := nsMap["name"]; !hasName {
					errors = append(errors, fmt.Sprintf("aerospikeConfig.namespaces[%d] is missing required 'name' key", i))
				} else if name, ok := nameVal.(string); ok && name != "" {
					if seenNsNames[name] {
						errors = append(errors, fmt.Sprintf(
							"aerospikeConfig.namespaces[%d]: duplicate namespace name %q; each namespace must have a unique name", i, name))
					}
					seenNsNames[name] = true
				}
				nsErrors, nsWarnings := v.validateNamespaceConfig(nsMap, i)
				errors = append(errors, nsErrors...)
				warnings = append(warnings, nsWarnings...)
			}
		case map[string]any:
			// Reject the map shape outright. Previously we only counted entries
			// here, which silently skipped per-namespace validation
			// (enterprise-only keys, replication-factor bounds) — a CE bypass.
			// configgen also expects a list (named blocks emitted in order),
			// so the map form is structurally wrong even for a valid CE config.
			errors = append(errors, fmt.Sprintf(
				"aerospikeConfig.namespaces must be a list of namespace maps "+
					"(e.g. [{name: foo, ...}, {name: bar, ...}]), got map with %d entries; "+
					"per-namespace validation cannot run on the map form",
				len(ns)))
		default:
			// Any other scalar shape (string, number, bool, ...) — e.g. the
			// common YAML mistake `namespaces: default`. Previously this fell
			// through the type switch silently: the webhook accepted the CR,
			// then configgen failed permanently at reconcile time. Reject it
			// here so the user gets an actionable admission error.
			errors = append(errors, fmt.Sprintf(
				"aerospikeConfig.namespaces must be a list of namespace maps "+
					"(e.g. [{name: foo, ...}]), got %T", ns))
		}
	}

	// The security stanza is allowed in aerospikeConfig (CE 8+ supports
	// enable-security and default-password-file), but enterprise-only sub-keys
	// must be rejected. ACL is managed via the Aerospike client API when
	// aerospikeAccessControl is configured; the security section is intentionally
	// skipped during config generation (configgen).
	if secSection, exists := config["security"]; exists {
		errors = append(errors, validateSecuritySection(secSection)...)
	}

	// Validate top-level section types to catch config errors at admission time
	// rather than at runtime (where they would cause permanent configgen failures).
	if svc, exists := config["service"]; exists {
		if _, ok := svc.(map[string]any); !ok {
			errors = append(errors, fmt.Sprintf("aerospikeConfig.service must be a map, got %T", svc))
		}
	}
	if net, exists := config["network"]; exists {
		if _, ok := net.(map[string]any); !ok {
			errors = append(errors, fmt.Sprintf("aerospikeConfig.network must be a map, got %T", net))
		}
	}
	if logging, exists := config["logging"]; exists {
		errors = append(errors, validateLoggingSection(logging)...)
	}

	// Validate heartbeat mode is mesh (CE only supports mesh)
	if netCfg, ok := config["network"].(map[string]any); ok {
		if hbCfg, ok := netCfg["heartbeat"].(map[string]any); ok {
			if mode, ok := hbCfg["mode"].(string); ok && mode != "mesh" {
				errors = append(errors, fmt.Sprintf("aerospikeConfig.network.heartbeat.mode must be 'mesh' for CE (got %q); multicast is Enterprise-only", mode))
			}
		}
	}

	return errors, warnings
}

// enterpriseOnlySecurityKeys lists security sub-keys that are Enterprise-only.
// CE 8+ supports: enable-security, default-password-file.
// Enterprise-only: tls, ldap, log, syslog.
var enterpriseOnlySecurityKeys = map[string]string{
	"tls":    "TLS within the security stanza is Enterprise-only",
	"ldap":   "LDAP authentication is Enterprise-only",
	"log":    "security audit logging is Enterprise-only",
	"syslog": "security syslog sink is Enterprise-only",
}

// enterpriseOnlyLoggingContexts lists logging context keys that are Enterprise-only.
// These are valid Aerospike logging context names but only emit messages on
// Enterprise builds; setting them on CE causes aerospikd to abort at startup
// with an "unknown context" error because the audit subsystem is unlinked.
//
// References:
//   - https://aerospike.com/docs/server/operations/configure/log
//   - https://aerospike.com/docs/server/operations/configure/security/auditing
var enterpriseOnlyLoggingContexts = map[string]string{
	"audit":                 "security audit context is Enterprise-only",
	"report-data-op":        "data-op audit reporting is Enterprise-only",
	"report-data-op-user":   "data-op-user audit reporting is Enterprise-only",
	"report-data-op-role":   "data-op-role audit reporting is Enterprise-only",
	"report-sys-admin":      "sys-admin audit reporting is Enterprise-only",
	"report-user-admin":     "user-admin audit reporting is Enterprise-only",
	"report-violation":      "violation audit reporting is Enterprise-only",
	"report-authentication": "authentication audit reporting is Enterprise-only",
}

// validateSecuritySection enforces CE constraints on aerospikeConfig.security.
// The security stanza must be a map; enterprise-only sub-keys (tls, ldap, log,
// syslog) are rejected even when the value is nil or a non-map (the presence of
// the key alone is enough to know the user intended an enterprise feature).
//
// Defensive type checks: the function never panics regardless of what shape the
// caller hands it (nil interface, scalar, list, map). Unexpected types yield a
// descriptive admission error instead.
func validateSecuritySection(secSection any) []string {
	var errs []string
	secMap, ok := secSection.(map[string]any)
	if !ok {
		return []string{fmt.Sprintf("aerospikeConfig.security must be a map, got %T", secSection)}
	}
	for enterpriseKey, reason := range enterpriseOnlySecurityKeys {
		if _, found := secMap[enterpriseKey]; found {
			errs = append(errs, fmt.Sprintf(
				"aerospikeConfig.security.%s is not allowed in CE edition (%s)", enterpriseKey, reason))
		}
	}
	return errs
}

// validateLoggingSection enforces CE constraints on aerospikeConfig.logging.
// Aerospike CE accepts a list of sink entries (console / syslog / file by path),
// each a map of context-name -> level. Enterprise-only contexts (audit + the
// report-* family) trigger a startup crash on CE because the audit subsystem
// is unlinked. We reject them at admission time so the cluster never enters a
// permanent CrashLoopBackOff.
//
// Type assertions are defensive: malformed entries (non-map, missing/blank
// name, non-string context values) produce admission errors instead of panics.
// Mirrors the runtime checks in configgen.generateLoggingSection so the
// webhook fails fast on the same shapes that the renderer would reject later.
func validateLoggingSection(logging any) []string {
	logs, ok := logging.([]any)
	if !ok {
		return []string{fmt.Sprintf("aerospikeConfig.logging must be a list, got %T", logging)}
	}
	var errs []string
	for i, entry := range logs {
		logMap, ok := entry.(map[string]any)
		if !ok {
			errs = append(errs, fmt.Sprintf(
				"aerospikeConfig.logging[%d] must be a map, got %T", i, entry))
			continue
		}
		nameVal, hasName := logMap["name"]
		if !hasName {
			errs = append(errs, fmt.Sprintf(
				"aerospikeConfig.logging[%d] is missing the required 'name' key", i))
		} else if nameStr, ok := nameVal.(string); !ok || nameStr == "" {
			errs = append(errs, fmt.Sprintf(
				"aerospikeConfig.logging[%d].name must be a non-empty string, got %T", i, nameVal))
		}
		for ctxKey, reason := range enterpriseOnlyLoggingContexts {
			if _, found := logMap[ctxKey]; found {
				errs = append(errs, fmt.Sprintf(
					"aerospikeConfig.logging[%d].%s is not allowed in CE edition (%s)", i, ctxKey, reason))
			}
		}
	}
	return errs
}

// enterpriseOnlyNamespaceKeys lists namespace-level config keys that are Enterprise-only.
var enterpriseOnlyNamespaceKeys = map[string]string{
	"compression":              "data compression is Enterprise-only",
	"compression-level":        "data compression is Enterprise-only",
	"durable-delete":           "durable deletes is Enterprise-only",
	"fast-restart":             "fast restart is Enterprise-only",
	"index-type":               "index-type flash/pmem is Enterprise-only",
	"sindex-type":              "sindex-type flash/pmem is Enterprise-only",
	"rack-id":                  "rack-id in namespace is Enterprise-only; use operator rackConfig instead",
	"strong-consistency":       "strong consistency is Enterprise-only",
	"tomb-raider-eligible-age": "tomb-raider is Enterprise-only",
	"tomb-raider-period":       "tomb-raider is Enterprise-only",
}

// validateNamespaceConfig checks individual namespace config for CE-incompatible options.
func (v *AerospikeClusterValidator) validateNamespaceConfig(nsMap map[string]any, index int) ([]string, admission.Warnings) {
	var errors []string
	var warnings admission.Warnings

	nsName := "<unknown>"
	if name, ok := nsMap["name"].(string); ok {
		nsName = name
	}

	// Check for enterprise-only keys
	for key, reason := range enterpriseOnlyNamespaceKeys {
		if _, exists := nsMap[key]; exists {
			errors = append(errors, fmt.Sprintf("namespace[%d] %q: '%s' is not allowed (%s)", index, nsName, key, reason))
		}
	}

	// Warn about data-in-memory usage in storage-engine device
	if se, ok := nsMap["storage-engine"].(map[string]any); ok {
		if dim, ok := se["data-in-memory"]; ok {
			if dimBool, ok := dim.(bool); ok && dimBool {
				warnings = append(warnings, fmt.Sprintf(
					"namespace %q: data-in-memory=true doubles memory usage (data stored in both memory and disk); ensure sufficient memory-size",
					nsName,
				))
			}
		}
	}

	// Validate replication-factor: single-node clusters should use 1.
	// Accept every integer type a decoder can plausibly produce (int, int32,
	// int64, float64, json.Number); an unhandled type would otherwise silently
	// skip the range check.
	if rf, ok := nsMap["replication-factor"]; ok {
		switch v := rf.(type) {
		case int:
			if v < 1 || v > 4 {
				errors = append(errors, fmt.Sprintf("namespace[%d] %q: replication-factor must be between 1 and 4 (got %d)", index, nsName, v))
			}
		case int32:
			if v < 1 || v > 4 {
				errors = append(errors, fmt.Sprintf("namespace[%d] %q: replication-factor must be between 1 and 4 (got %d)", index, nsName, v))
			}
		case int64:
			if v < 1 || v > 4 {
				errors = append(errors, fmt.Sprintf("namespace[%d] %q: replication-factor must be between 1 and 4 (got %d)", index, nsName, v))
			}
		case float64:
			if v < 1 || v > 4 {
				errors = append(errors, fmt.Sprintf("namespace[%d] %q: replication-factor must be between 1 and 4 (got %v)", index, nsName, v))
			}
		case json.Number:
			if n, err := v.Int64(); err == nil && (n < 1 || n > 4) {
				errors = append(errors, fmt.Sprintf("namespace[%d] %q: replication-factor must be between 1 and 4 (got %s)", index, nsName, v.String()))
			}
		}
	}

	return errors, warnings
}

// aerospikeCEBuiltinRoles lists the predefined role/privilege names in Aerospike CE.
// In CE, every builtin role name is also a valid privilege code, so a single
// set serves both purposes. Enterprise adds "superuser" which is excluded here.
// Reference: https://aerospike.com/docs/server/operations/configure/security/access-control/index.html
var aerospikeCEBuiltinRoles = map[string]bool{
	"user-admin":     true,
	"sys-admin":      true,
	"data-admin":     true,
	"read":           true,
	"write":          true,
	"read-write":     true,
	"read-write-udf": true,
	"truncate":       true,
}

// aerospikeGlobalOnlyPrivileges lists the privilege codes that Aerospike treats
// as global-only: they apply to the whole cluster and cannot be scoped to a
// namespace or set. Attaching a scope (e.g. "sys-admin.myns") is accepted by the
// Kubernetes API server but rejected by the Aerospike server when the operator
// syncs the role, leaving the cluster in ACLSyncError. The data-plane privileges
// (read, write, read-write, read-write-udf, truncate) are intentionally absent
// here because they are scopable.
// Reference: https://aerospike.com/docs/server/operations/configure/security/access-control/index.html
var aerospikeGlobalOnlyPrivileges = map[string]bool{
	"sys-admin":  true,
	"user-admin": true,
	"data-admin": true,
}

// validateAccessControl validates the ACL configuration.
func (v *AerospikeClusterValidator) validateAccessControl(acl *AerospikeAccessControlSpec) []string {
	var errors []string

	// Check for duplicate user names
	seenUsers := make(map[string]bool)
	for _, user := range acl.Users {
		if seenUsers[user.Name] {
			errors = append(errors, fmt.Sprintf("accessControl.users: duplicate user name %q", user.Name))
		}
		seenUsers[user.Name] = true
	}

	hasAdmin := false
	for _, user := range acl.Users {
		if user.SecretName == "" {
			errors = append(errors, fmt.Sprintf("user %q must have a secretName for password", user.Name))
		}
		hasSysAdmin := false
		hasUserAdmin := false
		for _, role := range user.Roles {
			if role == "sys-admin" {
				hasSysAdmin = true
			}
			if role == "user-admin" {
				hasUserAdmin = true
			}
		}
		if hasSysAdmin && hasUserAdmin {
			hasAdmin = true
		}
	}

	if !hasAdmin {
		errors = append(errors, "aerospikeAccessControl must have at least one user with both 'sys-admin' and 'user-admin' roles (required for operator-managed cluster administration)")
	}

	// Check for duplicate role names
	seenRoles := make(map[string]bool)
	for i, role := range acl.Roles {
		if role.Name == "" {
			errors = append(errors, fmt.Sprintf("accessControl.roles[%d]: role name must not be empty", i))
			continue
		}
		if seenRoles[role.Name] {
			errors = append(errors, fmt.Sprintf("accessControl.roles: duplicate role name %q", role.Name))
		}
		seenRoles[role.Name] = true
	}

	// Validate that users reference only built-in or explicitly defined roles
	definedRoles := make(map[string]bool)
	for _, role := range acl.Roles {
		definedRoles[role.Name] = true
	}
	for _, user := range acl.Users {
		for i, roleName := range user.Roles {
			if roleName == "" {
				errors = append(errors, fmt.Sprintf("user %q roles[%d]: role name must not be empty", user.Name, i))
				continue
			}
			if !aerospikeCEBuiltinRoles[roleName] && !definedRoles[roleName] {
				errors = append(errors, fmt.Sprintf("user %q references undefined role %q", user.Name, roleName))
			}
		}
	}

	// Validate privilege codes in role definitions
	for _, role := range acl.Roles {
		for i, privStr := range role.Privileges {
			trimmed := strings.TrimSpace(privStr)
			if trimmed == "" {
				errors = append(errors, fmt.Sprintf("role %q privileges[%d]: privilege string must not be empty or whitespace-only", role.Name, i))
				continue
			}
			// Reject strings with leading/trailing whitespace: the original value is
			// stored as-is, so " read-write" would be sent to Aerospike verbatim and
			// rejected at runtime even though it looks valid after trimming.
			if privStr != trimmed {
				errors = append(errors, fmt.Sprintf("role %q privileges[%d]: privilege string %q must not have leading or trailing whitespace", role.Name, i, privStr))
				continue
			}
			// Format: "<code>" or "<code>.<namespace>" or "<code>.<namespace>.<set>"
			code, scope, scoped := strings.Cut(privStr, ".")
			if !aerospikeCEBuiltinRoles[code] {
				errors = append(errors, fmt.Sprintf("role %q has invalid privilege code %q; valid codes: read, write, read-write, read-write-udf, sys-admin, user-admin, data-admin, truncate", role.Name, code))
				continue
			}
			// Admin-level privileges are global-only: Aerospike rejects a
			// namespace/set scope on them at runtime ("privilege ... cannot be
			// scoped"), leaving the operator stuck in ACLSyncError. Catch the
			// mistake at admission time with an actionable message instead.
			if scoped && aerospikeGlobalOnlyPrivileges[code] {
				errors = append(errors, fmt.Sprintf(
					"role %q privilege %q: %q is a global-only privilege and cannot be scoped to a namespace or set (%q)",
					role.Name, privStr, code, scope))
			}
		}
	}

	return errors
}

// validateSizeAndImage validates spec.size and spec.image, accounting for the fact that
// both fields may be supplied by a template when spec.templateRef is set.
func (v *AerospikeClusterValidator) validateSizeAndImage(cluster *AerospikeCluster) (sizeErrors, imageErrors []string, imageWarnings admission.Warnings) {
	// size=0 is only allowed when templateRef is present (template will supply the default).
	if cluster.Spec.Size > maxCEClusterSize {
		sizeErrors = append(sizeErrors, fmt.Sprintf("spec.size %d exceeds CE maximum of %d", cluster.Spec.Size, maxCEClusterSize))
	}
	if cluster.Spec.Size == 0 && cluster.Spec.TemplateRef == nil {
		sizeErrors = append(sizeErrors, "spec.size must be set (1–8) when spec.templateRef is not specified")
	}

	// image=="" is only allowed when templateRef is present (template will supply the default).
	if cluster.Spec.Image == "" && cluster.Spec.TemplateRef == nil {
		imageErrors = append(imageErrors, "spec.image must not be empty when spec.templateRef is not specified")
	}

	// Validate image content only when the image is provided.
	if cluster.Spec.Image != "" {
		imageLower := strings.ToLower(cluster.Spec.Image)
		// Strictest check first: explicit enterprise repository name on Docker
		// Hub. Reported with a clear, repository-specific error so users can
		// tell at a glance why CE rejected the image.
		switch {
		case strings.Contains(imageLower, "aerospike-server-enterprise"):
			imageErrors = append(imageErrors, fmt.Sprintf(
				"spec.image %q references the enterprise repository "+
					"(aerospike-server-enterprise); CE clusters must use a CE image "+
					"such as aerospike:ce-8.1.1.1",
				cluster.Spec.Image))
		case strings.Contains(imageLower, "enterprise") || isEnterpriseTag(cluster.Spec.Image):
			imageErrors = append(imageErrors, fmt.Sprintf(
				"spec.image %q is an Enterprise Edition image; only Community Edition images are allowed",
				cluster.Spec.Image))
		}
		if !strings.Contains(cluster.Spec.Image, ":") {
			imageWarnings = append(imageWarnings, fmt.Sprintf("spec.image %q has no tag; use an explicit version tag (e.g., aerospike:ce-8.1.1.1) for reproducible deployments", cluster.Spec.Image))
		} else {
			parts := strings.SplitN(cluster.Spec.Image, ":", 2)
			if parts[1] == "latest" {
				imageWarnings = append(imageWarnings, "spec.image uses 'latest' tag; use an explicit version tag for reproducible deployments")
			} else {
				// Enforce minimum CE 8 version.
				if major, err := parseMajorVersion(cluster.Spec.Image); err == nil && major < minCEMajorVersion {
					imageErrors = append(imageErrors, fmt.Sprintf(
						"spec.image %q requires Aerospike CE %d or later (got major version %d); CE 7.x is no longer supported",
						cluster.Spec.Image, minCEMajorVersion, major))
				}
			}
		}
	}

	return sizeErrors, imageErrors, imageWarnings
}

// parseMajorVersion extracts the major version number from a container image tag
// such as "aerospike:ce-8.1.1.1" or "aerospike:8.1.0". Returns an error if the
// version cannot be determined.
func parseMajorVersion(image string) (int, error) {
	parts := strings.SplitN(image, ":", 2)
	if len(parts) != 2 {
		return 0, fmt.Errorf("image %q has no tag", image)
	}
	tag := parts[1]
	for _, prefix := range []string{"ce-", "ee-", "ent-"} {
		if after, found := strings.CutPrefix(tag, prefix); found {
			tag = after
			break
		}
	}
	// strings.Cut returns the full string as the first value when there is no
	// separator, so this handles both multi-segment ("8.1.0.1") and
	// single-segment ("8") tags. A genuinely unparseable tag (e.g. "latest" or
	// an empty tag) still fails strconv.Atoi below and returns an error, so the
	// caller keeps skipping such tags gracefully instead of false-rejecting.
	before, _, _ := strings.Cut(tag, ".")
	// Strip leading 'v' to handle tags like "v8.1.1" in addition to "8.1.1".
	before = strings.TrimPrefix(before, "v")
	return strconv.Atoi(before)
}

// isEnterpriseTag returns true if the image tag indicates an Enterprise Edition image
// (e.g., "aerospike:ee-8.0.0.1_1", "aerospike:ent-8.0.0").
func isEnterpriseTag(image string) bool {
	parts := strings.SplitN(image, ":", 2)
	if len(parts) != 2 {
		return false
	}

	tagLower := strings.ToLower(parts[1])
	return strings.HasPrefix(tagLower, "ee-") || strings.HasPrefix(tagLower, "ent-")
}

// hasVolumeForPath checks if any volume mounts to the given path.
func hasVolumeForPath(storage *AerospikeStorageSpec, path string) bool {
	if storage == nil {
		return false
	}
	for _, vol := range storage.Volumes {
		if vol.Aerospike != nil && vol.Aerospike.Path == path {
			return true
		}
	}
	return false
}

// validateReplicationFactor validates that replication-factor does not exceed cluster size.
//
// When spec.size is 0 and spec.templateRef is set, the size will be supplied
// later by the resolved template; in that case the rfInt > size cross-check is
// skipped and deferred to reconcile-time (post-template-merge) validation, to
// avoid spurious "replication-factor 3 exceeds cluster size 0" rejections when
// users legitimately omit size on the cluster CR and rely on the template.
func (v *AerospikeClusterValidator) validateReplicationFactor(cluster *AerospikeCluster) []string {
	if cluster.Spec.AerospikeConfig == nil {
		return nil
	}
	nsList, ok := cluster.Spec.AerospikeConfig.Value["namespaces"].([]any)
	if !ok {
		return nil
	}
	// Size is deferred to template resolution; skip the rfInt > size check.
	sizeDeferredToTemplate := cluster.Spec.Size == 0 && cluster.Spec.TemplateRef != nil
	var errors []string
	for _, ns := range nsList {
		nsMap, ok := ns.(map[string]any)
		if !ok {
			continue
		}
		nsName, _ := nsMap["name"].(string)
		rf, ok := nsMap["replication-factor"]
		if !ok {
			continue
		}
		rfInt := 0
		// Accept every integer type a decoder can plausibly produce. Missing
		// an int32 case here caused an int32-typed value to fall through with
		// rfInt=0 and trip the misleading "must be >= 1, got 0" branch below.
		switch val := rf.(type) {
		case int:
			rfInt = val
		case int32:
			rfInt = int(val)
		case int64:
			rfInt = int(val)
		case float64:
			if val != float64(int(val)) || val < 0 {
				errors = append(errors, fmt.Sprintf(
					"namespace %q: replication-factor must be a positive integer, got %v", nsName, val))
				continue
			}
			rfInt = int(val)
		case json.Number:
			n, err := val.Int64()
			if err != nil {
				errors = append(errors, fmt.Sprintf(
					"namespace %q: replication-factor must be a positive integer, got %s", nsName, val.String()))
				continue
			}
			rfInt = int(n)
		default:
			// A non-numeric value (most commonly a string from YAML quoting,
			// e.g. replication-factor: "2") would otherwise fall through with
			// rfInt=0 and trip the misleading "must be >= 1, got 0" branch
			// below. Surface the real problem — the wrong type — instead.
			errors = append(errors, fmt.Sprintf(
				"namespace %q: replication-factor must be an integer, got %T (%v)", nsName, rf, rf))
			continue
		}
		if rfInt < 1 {
			errors = append(errors, fmt.Sprintf(
				"namespace %q: replication-factor must be >= 1, got %d", nsName, rfInt))
			continue
		}
		if sizeDeferredToTemplate {
			// Cluster size is supplied by the referenced template; the
			// reconciler will re-validate after template merge.
			continue
		}
		if rfInt > int(cluster.Spec.Size) {
			errors = append(errors, fmt.Sprintf(
				"namespace %q: replication-factor %d exceeds cluster size %d",
				nsName, rfInt, cluster.Spec.Size))
		}
	}
	return errors
}

// validateWorkDirectory checks that the work directory has persistent storage.
func (v *AerospikeClusterValidator) validateWorkDirectory(cluster *AerospikeCluster) admission.Warnings {
	if cluster.Spec.ValidationPolicy != nil && cluster.Spec.ValidationPolicy.SkipWorkDirValidate {
		return nil
	}
	if cluster.Spec.AerospikeConfig == nil {
		return nil
	}
	svcCfg, ok := cluster.Spec.AerospikeConfig.Value["service"].(map[string]any)
	if !ok {
		return nil
	}
	workDir, ok := svcCfg["work-directory"].(string)
	if !ok || workDir == "" {
		return nil
	}
	if !hasVolumeForPath(cluster.Spec.Storage, workDir) {
		return admission.Warnings{fmt.Sprintf(
			"work-directory %q has no persistent volume; data may be lost on pod restart (set validationPolicy.skipWorkDirValidate to suppress)", workDir)}
	}
	return nil
}

// validateBatchSize checks the rolling update batch size against cluster size.
//
// When spec.size is 0 and spec.templateRef is set, the size will be supplied
// later by the resolved template; the bs > size comparison is skipped to avoid
// a spurious "all pods may restart simultaneously" warning that fires for every
// templateRef-backed cluster with rollingUpdateBatchSize>0. Mirrors the
// sizeDeferredToTemplate pattern used in validateReplicationFactor.
func (v *AerospikeClusterValidator) validateBatchSize(cluster *AerospikeCluster) admission.Warnings {
	if cluster.Spec.Size == 0 && cluster.Spec.TemplateRef != nil {
		return nil
	}
	if cluster.Spec.RollingUpdateBatchSize == nil {
		return nil
	}
	bs := *cluster.Spec.RollingUpdateBatchSize
	if bs > cluster.Spec.Size {
		return admission.Warnings{fmt.Sprintf("rollingUpdateBatchSize (%d) is greater than cluster size (%d); all pods may restart simultaneously", bs, cluster.Spec.Size)}
	}
	return nil
}

// validateMaxUnavailable warns if maxUnavailable is >= cluster size.
//
// When spec.size is 0 and spec.templateRef is set, the size will be supplied
// later by the resolved template; the mu >= size comparison is skipped to avoid
// a spurious "PodDisruptionBudget will not prevent full disruption" warning
// that fires for every templateRef-backed cluster with maxUnavailable set.
// Mirrors the sizeDeferredToTemplate pattern used in validateBatchSize (#305).
func (v *AerospikeClusterValidator) validateMaxUnavailable(cluster *AerospikeCluster) admission.Warnings {
	if cluster.Spec.Size == 0 && cluster.Spec.TemplateRef != nil {
		return nil
	}
	if cluster.Spec.MaxUnavailable == nil {
		return nil
	}
	mu := *cluster.Spec.MaxUnavailable
	if mu.Type == intstr.Int {
		if mu.IntVal >= cluster.Spec.Size {
			return admission.Warnings{fmt.Sprintf(
				"maxUnavailable (%d) is >= cluster size (%d); PodDisruptionBudget will not prevent full disruption",
				mu.IntVal, cluster.Spec.Size)}
		}
	} else {
		s := mu.StrVal
		if numStr, ok := strings.CutSuffix(s, "%"); ok {
			num, err := strconv.Atoi(numStr)
			if err == nil && num >= 100 {
				return admission.Warnings{fmt.Sprintf(
					"maxUnavailable (%s) allows 100%% disruption; PodDisruptionBudget will not protect availability", s)}
			}
		}
	}
	return nil
}

// validateRackConfig validates the rack configuration.
func (v *AerospikeClusterValidator) validateRackConfig(rackConfig *RackConfig) []string {
	var errors []string

	rackIDs := make(map[int]bool)
	rackLabels := make(map[string]bool)
	for _, rack := range rackConfig.Racks {
		if rack.ID <= 0 {
			errors = append(errors, fmt.Sprintf("rack ID must be > 0, got %d (rack ID 0 is reserved for the default rack)", rack.ID))
		}
		if rackIDs[rack.ID] {
			errors = append(errors, fmt.Sprintf("duplicate rack ID %d in rackConfig", rack.ID))
		}
		rackIDs[rack.ID] = true

		// Validate RackLabel uniqueness across racks
		if rack.RackLabel != "" {
			if rackLabels[rack.RackLabel] {
				errors = append(errors, fmt.Sprintf("duplicate rackLabel %q in rackConfig; each rack must have a unique rackLabel", rack.RackLabel))
			}
			rackLabels[rack.RackLabel] = true
		}
	}

	// Validate ScaleDownBatchSize is positive if set
	if rackConfig.ScaleDownBatchSize != nil {
		if err := validateIntOrString(rackConfig.ScaleDownBatchSize, "rackConfig.scaleDownBatchSize", 1); err != "" {
			errors = append(errors, err)
		}
	}

	// Validate MaxIgnorablePods is non-negative if set
	if rackConfig.MaxIgnorablePods != nil {
		if err := validateIntOrString(rackConfig.MaxIgnorablePods, "rackConfig.maxIgnorablePods", 0); err != "" {
			errors = append(errors, err)
		}
	}

	// Validate RollingUpdateBatchSize is positive if set
	if rackConfig.RollingUpdateBatchSize != nil {
		if err := validateIntOrString(rackConfig.RollingUpdateBatchSize, "rackConfig.rollingUpdateBatchSize", 1); err != "" {
			errors = append(errors, err)
		}
	}

	return errors
}

// validateOperations validates the on-demand operations spec.
func (v *AerospikeClusterValidator) validateOperations(ops []OperationSpec) []string {
	var errors []string

	if len(ops) > 1 {
		errors = append(errors, "only one operation can be specified at a time")
	}

	seenIDs := make(map[string]bool)
	for _, op := range ops {
		if len(op.ID) < 1 || len(op.ID) > 20 {
			errors = append(errors, fmt.Sprintf("operation id %q must be 1-20 characters", op.ID))
		}
		if seenIDs[op.ID] {
			errors = append(errors, fmt.Sprintf("duplicate operation id %q", op.ID))
		}
		seenIDs[op.ID] = true
	}

	return errors
}

// validateIntOrString validates that an IntOrString value meets a minimum bound.
// Use minValue=1 for positive validation, minValue=0 for non-negative validation.
func validateIntOrString(val *intstr.IntOrString, fieldName string, minValue int) string {
	if val == nil {
		return ""
	}
	label := "positive"
	if minValue == 0 {
		label = "non-negative"
	}
	if val.Type == intstr.Int {
		if int(val.IntVal) < minValue {
			return fmt.Sprintf("%s must be a %s integer (got %d)", fieldName, label, val.IntVal)
		}
	} else {
		s := val.StrVal
		if !strings.HasSuffix(s, "%") {
			return fmt.Sprintf("%s must be a %s integer or a percentage string (e.g., \"25%%\"); got %q", fieldName, label, s)
		}
		numStr := strings.TrimSuffix(s, "%")
		num, err := strconv.Atoi(numStr)
		if err != nil {
			return fmt.Sprintf("%s percentage %q is not a valid integer", fieldName, s)
		}
		if num < minValue || num > 100 {
			return fmt.Sprintf("%s percentage must be between %d and 100 (got %d)", fieldName, minValue, num)
		}
	}
	return ""
}

// validateStorage validates the storage configuration.
func (v *AerospikeClusterValidator) validateStorage(storage *AerospikeStorageSpec) ([]string, admission.Warnings) {
	var errors []string
	var warnings admission.Warnings

	// Check for duplicate volume names
	namesSeen := make(map[string]bool, len(storage.Volumes))
	for _, vol := range storage.Volumes {
		if namesSeen[vol.Name] {
			errors = append(errors, fmt.Sprintf("storage.volumes: duplicate volume name %q", vol.Name))
		}
		namesSeen[vol.Name] = true
	}

	for i, vol := range storage.Volumes {
		volErrors, volWarnings := v.validateVolume(vol, i)
		errors = append(errors, volErrors...)
		warnings = append(warnings, volWarnings...)
	}

	// Validate deleteLocalStorageOnRestart requires localStorageClasses
	if storage.DeleteLocalStorageOnRestart != nil && *storage.DeleteLocalStorageOnRestart {
		if len(storage.LocalStorageClasses) == 0 {
			errors = append(errors, "storage.deleteLocalStorageOnRestart is true but storage.localStorageClasses is empty; specify which storage classes are local")
		}
	}

	// Warn if local storage class is used but deleteLocalStorageOnRestart is not set
	if len(storage.LocalStorageClasses) > 0 && storage.DeleteLocalStorageOnRestart == nil {
		warnings = append(warnings, "storage.localStorageClasses is set but storage.deleteLocalStorageOnRestart is not configured; local PVCs will not be deleted on pod restart")
	}

	return errors, warnings
}

// validateVolume validates a single volume spec.
func (v *AerospikeClusterValidator) validateVolume(vol VolumeSpec, index int) ([]string, admission.Warnings) {
	var errors []string
	var warnings admission.Warnings

	// Validate exactly one volume source is specified
	sourceCount := 0
	if vol.Source.PersistentVolume != nil {
		sourceCount++
	}
	if vol.Source.EmptyDir != nil {
		sourceCount++
	}
	if vol.Source.Secret != nil {
		sourceCount++
	}
	if vol.Source.ConfigMap != nil {
		sourceCount++
	}
	if vol.Source.HostPath != nil {
		sourceCount++
	}
	if sourceCount == 0 {
		errors = append(errors, fmt.Sprintf("storage.volumes[%d] %q: exactly one volume source must be specified", index, vol.Name))
	} else if sourceCount > 1 {
		errors = append(errors, fmt.Sprintf("storage.volumes[%d] %q: only one volume source can be specified (found %d)", index, vol.Name, sourceCount))
	}

	// Warn about HostPath usage
	if vol.Source.HostPath != nil {
		warnings = append(warnings, fmt.Sprintf(
			"storage.volumes[%d] %q: hostPath volumes are not recommended for production; data is tied to a specific node and not portable",
			index, vol.Name))
	}

	// Warn about cascadeDelete on non-persistent volumes (has no effect)
	if vol.CascadeDelete != nil && *vol.CascadeDelete && vol.Source.PersistentVolume == nil {
		warnings = append(warnings, fmt.Sprintf(
			"storage.volumes[%d] %q: cascadeDelete has no effect on non-persistent volumes",
			index, vol.Name))
	}

	// Validate PV size is a valid Kubernetes quantity
	if vol.Source.PersistentVolume != nil {
		if vol.Source.PersistentVolume.Size == "" {
			errors = append(errors, fmt.Sprintf(
				"storage.volumes[%d] %q: persistentVolume.size must not be empty", index, vol.Name))
		} else if qty, err := resource.ParseQuantity(vol.Source.PersistentVolume.Size); err != nil {
			errors = append(errors, fmt.Sprintf(
				"storage.volumes[%d] %q: persistentVolume.size %q is not a valid Kubernetes quantity: %v",
				index, vol.Name, vol.Source.PersistentVolume.Size, err))
		} else if qty.Sign() <= 0 {
			errors = append(errors, fmt.Sprintf(
				"storage.volumes[%d] %q: persistentVolume.size must be a positive quantity (got %q)",
				index, vol.Name, vol.Source.PersistentVolume.Size))
		}
	}

	// Validate Aerospike mount path is absolute
	if vol.Aerospike != nil && vol.Aerospike.Path != "" && !strings.HasPrefix(vol.Aerospike.Path, "/") {
		errors = append(errors, fmt.Sprintf(
			"storage.volumes[%d] %q: aerospike.path must be an absolute path (got %q)",
			index, vol.Name, vol.Aerospike.Path))
	}

	// Validate SubPath and SubPathExpr are mutually exclusive (Aerospike attachment)
	if vol.Aerospike != nil && vol.Aerospike.SubPath != "" && vol.Aerospike.SubPathExpr != "" {
		errors = append(errors, fmt.Sprintf(
			"storage.volumes[%d] %q: aerospike.subPath and aerospike.subPathExpr are mutually exclusive",
			index, vol.Name))
	}

	errors = append(errors, validateVolumeAttachments(vol, index)...)

	return errors, warnings
}

// validateVolumeAttachments validates per-container volume attachments for a
// single volume: subPath/subPathExpr mutual exclusion and duplicate containerName
// detection within sidecars, within initContainers, and across the two lists
// (a Pod cannot have the same container name as both an init and a sidecar/main).
func validateVolumeAttachments(vol VolumeSpec, index int) []string {
	var errors []string

	sidecarNames := make(map[string]int, len(vol.Sidecars))
	for j, sc := range vol.Sidecars {
		if sc.SubPath != "" && sc.SubPathExpr != "" {
			errors = append(errors, fmt.Sprintf(
				"storage.volumes[%d] %q: sidecars[%d] %q subPath and subPathExpr are mutually exclusive",
				index, vol.Name, j, sc.ContainerName))
		}
		if prev, seen := sidecarNames[sc.ContainerName]; seen {
			errors = append(errors, fmt.Sprintf(
				"storage.volumes[%d] %q: sidecars[%d] containerName %q duplicates sidecars[%d]",
				index, vol.Name, j, sc.ContainerName, prev))
		} else {
			sidecarNames[sc.ContainerName] = j
		}
	}

	initNames := make(map[string]int, len(vol.InitContainers))
	for j, ic := range vol.InitContainers {
		if ic.SubPath != "" && ic.SubPathExpr != "" {
			errors = append(errors, fmt.Sprintf(
				"storage.volumes[%d] %q: initContainers[%d] %q subPath and subPathExpr are mutually exclusive",
				index, vol.Name, j, ic.ContainerName))
		}
		if prev, seen := initNames[ic.ContainerName]; seen {
			errors = append(errors, fmt.Sprintf(
				"storage.volumes[%d] %q: initContainers[%d] containerName %q duplicates initContainers[%d]",
				index, vol.Name, j, ic.ContainerName, prev))
		} else {
			initNames[ic.ContainerName] = j
		}
		if prev, seen := sidecarNames[ic.ContainerName]; seen {
			errors = append(errors, fmt.Sprintf(
				"storage.volumes[%d] %q: initContainers[%d] containerName %q duplicates sidecars[%d]",
				index, vol.Name, j, ic.ContainerName, prev))
		}
	}

	return errors
}

// builtinPodContainerNames lists container names reserved by the operator that
// users must not reuse for sidecars or extra init containers. The main server
// container name is sourced from the canonical AerospikeContainerName constant
// (api/v1alpha1/constants.go); the init container name is hard-coded here to
// mirror internal/podutil.InitContainerName without introducing an api ->
// internal import cycle. Keep these two strings in sync with podutil.
var builtinPodContainerNames = map[string]bool{
	AerospikeContainerName: true, // "aerospike-server"
	"aerospike-init":       true, // mirrors internal/podutil.InitContainerName
}

// validatePodSpecContainerNames rejects user-supplied sidecar / extra init
// container names that conflict with another user entry or with the operator's
// built-in containers. Kubernetes itself rejects Pods that share a name across
// containers / initContainers, so surfacing this at admission time gives a
// faster, clearer error than waiting for the StatefulSet to fail.
func validatePodSpecContainerNames(podSpec *AerospikePodSpec) []string {
	var errors []string

	sidecarNames := make(map[string]int, len(podSpec.Sidecars))
	for i, sc := range podSpec.Sidecars {
		if builtinPodContainerNames[sc.Name] {
			errors = append(errors, fmt.Sprintf(
				"spec.podSpec.sidecars[%d] name %q conflicts with operator built-in container name",
				i, sc.Name))
			continue
		}
		if prev, seen := sidecarNames[sc.Name]; seen {
			errors = append(errors, fmt.Sprintf(
				"spec.podSpec.sidecars[%d] name %q duplicates sidecars[%d]",
				i, sc.Name, prev))
		} else {
			sidecarNames[sc.Name] = i
		}
	}

	initNames := make(map[string]int, len(podSpec.InitContainers))
	for i, ic := range podSpec.InitContainers {
		if builtinPodContainerNames[ic.Name] {
			errors = append(errors, fmt.Sprintf(
				"spec.podSpec.initContainers[%d] name %q conflicts with operator built-in container name",
				i, ic.Name))
			continue
		}
		if prev, seen := initNames[ic.Name]; seen {
			errors = append(errors, fmt.Sprintf(
				"spec.podSpec.initContainers[%d] name %q duplicates initContainers[%d]",
				i, ic.Name, prev))
		} else {
			initNames[ic.Name] = i
		}
		if prev, seen := sidecarNames[ic.Name]; seen {
			errors = append(errors, fmt.Sprintf(
				"spec.podSpec.initContainers[%d] name %q duplicates sidecars[%d]",
				i, ic.Name, prev))
		}
	}

	return errors
}

// aerospikeReservedPorts lists ports used by Aerospike server that must not conflict.
var aerospikeReservedPorts = map[int32]string{
	3000: "service",
	3001: "fabric",
	3002: "heartbeat",
	3003: "info",
}

// validateMonitoring validates the monitoring configuration.
func (v *AerospikeClusterValidator) validateMonitoring(m *AerospikeMonitoringSpec) ([]string, admission.Warnings) {
	var errors []string
	var warnings admission.Warnings

	// Validate port is in valid TCP range.
	if m.Port < 1 || m.Port > 65535 {
		errors = append(errors, fmt.Sprintf("monitoring.port must be in range 1-65535 (got %d)", m.Port))
	}

	// Port conflict check with Aerospike reserved ports.
	if portName, ok := aerospikeReservedPorts[m.Port]; ok {
		errors = append(errors, fmt.Sprintf("monitoring.port %d conflicts with Aerospike %s port", m.Port, portName))
	}

	// Empty image check.
	if m.ExporterImage == "" {
		errors = append(errors, "monitoring.exporterImage must not be empty when monitoring is enabled")
	}

	// Warn about 'latest' tag on exporter image.
	if strings.Contains(m.ExporterImage, ":") {
		parts := strings.SplitN(m.ExporterImage, ":", 2)
		if parts[1] == "latest" {
			warnings = append(warnings, "monitoring.exporterImage uses 'latest' tag; use an explicit version tag for reproducible deployments")
		}
	} else if m.ExporterImage != "" {
		warnings = append(warnings, fmt.Sprintf("monitoring.exporterImage %q has no tag; use an explicit version tag for reproducible deployments", m.ExporterImage))
	}

	// Validate the ServiceMonitor scrape interval. The reconciler writes this
	// string verbatim into the ServiceMonitor's endpoints[].interval, where the
	// Prometheus Operator enforces its Duration pattern. An invalid value (e.g.
	// "30" with no unit, "5 seconds", or the matched-but-degenerate empty string
	// from a stray whitespace-only value) passes the Kubernetes API server but is
	// rejected by the Prometheus Operator CRD at apply time, leaving monitoring
	// silently un-reconciled. Reject it at admission with an actionable message.
	// The empty interval is allowed here because the defaulter fills it with the
	// 30s default before reconcile; only an explicitly set, invalid value fails.
	if m.ServiceMonitor != nil && m.ServiceMonitor.Enabled && m.ServiceMonitor.Interval != "" {
		if !prometheusDurationRe.MatchString(m.ServiceMonitor.Interval) {
			errors = append(errors, fmt.Sprintf(
				"monitoring.serviceMonitor.interval %q is not a valid Prometheus duration "+
					"(expected values like \"30s\", \"1m\", \"500ms\", \"2h30m\"); "+
					"the Prometheus Operator rejects other formats when the ServiceMonitor is applied",
				m.ServiceMonitor.Interval))
		}
	}

	// Validate MetricLabels keys and values for TOML compatibility.
	// TOML bare keys may only contain ASCII letters, digits, dashes, and
	// underscores. Values are TOML-quoted by the operator, so '=' and ','
	// are safe inside values; only control characters are rejected.
	for k, val := range m.MetricLabels {
		if !tomlBareKeyRe.MatchString(k) {
			errors = append(errors, fmt.Sprintf("monitoring.metricLabels key %q must contain only ASCII letters, digits, dashes, and underscores", k))
		}
		for _, r := range val {
			if r < 0x20 || r == 0x7f {
				errors = append(errors, fmt.Sprintf("monitoring.metricLabels[%q] value must not contain control characters", k))
				break
			}
		}
	}

	// Validate CustomRules structure.
	if m.PrometheusRule != nil {
		for i, raw := range m.PrometheusRule.CustomRules {
			var ruleGroup map[string]any
			if err := json.Unmarshal(raw.Raw, &ruleGroup); err != nil {
				errors = append(errors, fmt.Sprintf("monitoring.prometheusRule.customRules[%d]: invalid JSON: %v", i, err))
				continue
			}
			if name, ok := ruleGroup["name"]; !ok {
				errors = append(errors, fmt.Sprintf("monitoring.prometheusRule.customRules[%d]: missing required field 'name'", i))
			} else if nameStr, ok := name.(string); !ok {
				errors = append(errors, fmt.Sprintf("monitoring.prometheusRule.customRules[%d]: field 'name' must be a string, got %T", i, name))
			} else if nameStr == "" {
				errors = append(errors, fmt.Sprintf("monitoring.prometheusRule.customRules[%d]: field 'name' must not be empty", i))
			}
			rules, ok := ruleGroup["rules"]
			if !ok {
				errors = append(errors, fmt.Sprintf("monitoring.prometheusRule.customRules[%d]: missing required field 'rules'", i))
				continue
			}
			rulesArr, ok := rules.([]any)
			if !ok {
				errors = append(errors, fmt.Sprintf("monitoring.prometheusRule.customRules[%d]: field 'rules' must be a JSON array, got %T", i, rules))
				continue
			}
			if len(rulesArr) == 0 {
				errors = append(errors, fmt.Sprintf("monitoring.prometheusRule.customRules[%d]: field 'rules' must contain at least one rule", i))
			}
		}
	}

	return errors, warnings
}

// operatorFixedNetworkPorts maps each network subsection to the single port the
// operator supports. The Aerospike container's containerPort declarations, the
// liveness/readiness probes (asinfo -p <port>), the headless/per-pod Services,
// and the generated NetworkPolicy/CiliumNetworkPolicy all hard-code these
// values. A CR that sets a different port is silently broken — the probes query
// the wrong port so pods never become Ready — so admission must reject it
// instead of letting the cluster fail opaquely at runtime.
var operatorFixedNetworkPorts = map[string]int{
	"service":   int(DefaultServicePort),
	"heartbeat": int(DefaultHeartbeatPort),
	"fabric":    int(DefaultFabricPort),
}

// validateNetworkPortUniqueness checks that service, heartbeat, and fabric
// ports are valid TCP integers, distinct, do not collide with another
// Aerospike subsection's reserved port (e.g. service.port=3003 vs info), and
// match the fixed ports the operator hard-codes everywhere else.
func (v *AerospikeClusterValidator) validateNetworkPortUniqueness(cluster *AerospikeCluster) []string {
	netCfg, ok := cluster.Spec.AerospikeConfig.Value["network"].(map[string]any)
	if !ok {
		return nil
	}

	type portEntry struct {
		name string
		port int
	}

	var errors []string

	extractPort := func(sub string, raw any) (int, bool) {
		switch x := raw.(type) {
		case int:
			return x, true
		case float64:
			return int(x), true
		case int64:
			return int(x), true
		case string:
			// String-typed ports were previously silently dropped; surface
			// them so YAML shape mistakes (port: "3000") fail at admission.
			errors = append(errors, fmt.Sprintf(
				"aerospikeConfig.network.%s.port must be an integer, got string %q", sub, x))
			return 0, false
		default:
			errors = append(errors, fmt.Sprintf(
				"aerospikeConfig.network.%s.port must be an integer, got %T", sub, raw))
			return 0, false
		}
	}

	var ports []portEntry
	for _, sub := range []string{"service", "heartbeat", "fabric"} {
		subCfg, ok := netCfg[sub].(map[string]any)
		if !ok {
			continue
		}
		raw, exists := subCfg["port"]
		if !exists {
			continue
		}
		port, ok := extractPort(sub, raw)
		if !ok {
			continue
		}
		// TCP port range check; out-of-range values are rejected and skipped
		// so they do not poison the uniqueness / reserved-port checks below.
		if port < 1 || port > 65535 {
			errors = append(errors, fmt.Sprintf(
				"aerospikeConfig.network.%s.port=%d must be in range 1-65535", sub, port))
			continue
		}
		ports = append(ports, portEntry{name: sub, port: port})
	}

	// Cross-check user-configured ports against the reserved Aerospike port
	// table; reject when a subsection's port collides with the reserved port
	// of a *different* subsection (e.g. service.port=3003 → info port).
	for _, p := range ports {
		for reservedPort, reservedFor := range aerospikeReservedPorts {
			if int(reservedPort) == p.port && reservedFor != p.name {
				errors = append(errors, fmt.Sprintf(
					"aerospikeConfig.network.%s.port=%d conflicts with reserved port %d (used for %s)",
					p.name, p.port, reservedPort, reservedFor))
			}
		}
	}
	for i := 0; i < len(ports); i++ {
		for j := i + 1; j < len(ports); j++ {
			if ports[i].port == ports[j].port {
				errors = append(errors, fmt.Sprintf(
					"network port conflict: %s.port and %s.port are both %d",
					ports[i].name, ports[j].name, ports[i].port))
			}
		}
	}

	// Reject ports that differ from the fixed values the operator hard-codes
	// into container ports, probes, Services and NetworkPolicies. A custom port
	// is accepted by the API server but produces a cluster whose probes target
	// the wrong port, so pods never reach Ready. Fail fast at admission with an
	// actionable message instead.
	for _, p := range ports {
		if fixed, known := operatorFixedNetworkPorts[p.name]; known && p.port != fixed {
			errors = append(errors, fmt.Sprintf(
				"aerospikeConfig.network.%s.port=%d is not supported; the operator requires the fixed port %d "+
					"(container ports, health probes, Services and NetworkPolicies all assume it). "+
					"Remove the port override or set it to %d.",
				p.name, p.port, fixed, fixed))
		}
	}

	return errors
}

// validateRackBatchSize warns when a rack-level percentage batch size resolves to 0 pods.
//
// When spec.size is 0 and spec.templateRef is set, the size will be supplied
// later by the resolved template; the percentage-resolves-to-0 check is skipped
// to avoid a spurious "resolves to 0 pods for cluster size 0" warning that
// fires for every templateRef-backed cluster with a percentage
// rackConfig.rollingUpdateBatchSize. Mirrors the sizeDeferredToTemplate pattern
// used in validateBatchSize (#305).
func (v *AerospikeClusterValidator) validateRackBatchSize(cluster *AerospikeCluster) admission.Warnings {
	if cluster.Spec.RackConfig == nil || cluster.Spec.RackConfig.RollingUpdateBatchSize == nil {
		return nil
	}
	if cluster.Spec.Size == 0 && cluster.Spec.TemplateRef != nil {
		return nil
	}
	bs := cluster.Spec.RackConfig.RollingUpdateBatchSize
	if bs.Type != intstr.String {
		return nil
	}
	s := bs.StrVal
	numStr, ok := strings.CutSuffix(s, "%")
	if !ok {
		return nil
	}
	num, err := strconv.Atoi(numStr)
	if err != nil {
		return nil
	}
	resolved := int(cluster.Spec.Size) * num / 100
	if resolved == 0 {
		return admission.Warnings{fmt.Sprintf(
			"rackConfig.rollingUpdateBatchSize %q resolves to 0 pods for cluster size %d; effective batch size will be clamped to 1",
			s, cluster.Spec.Size)}
	}
	return nil
}
