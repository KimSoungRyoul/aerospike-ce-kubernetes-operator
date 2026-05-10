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
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var aerospikeclustertemplatelog = logf.Log.WithName("aerospikeclustertemplate-resource")

// SetupWebhookWithManager registers the webhooks for AerospikeClusterTemplate.
func (r *AerospikeClusterTemplate) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, r).
		WithDefaulter(&AerospikeClusterTemplateDefaulter{}).
		WithValidator(&AerospikeClusterTemplateValidator{}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-acko-io-v1alpha1-aerospikeclustertemplate,mutating=true,failurePolicy=fail,sideEffects=None,groups=acko.io,resources=aerospikeclustertemplates,verbs=create;update,versions=v1alpha1,name=maerospikeclustertemplate.kb.io,admissionReviewVersions=v1

// AerospikeClusterTemplateDefaulter implements admission.Defaulter for AerospikeClusterTemplate.
type AerospikeClusterTemplateDefaulter struct{}

var _ admission.Defaulter[*AerospikeClusterTemplate] = &AerospikeClusterTemplateDefaulter{}

// Default implements admission.Defaulter[*AerospikeClusterTemplate].
func (d *AerospikeClusterTemplateDefaulter) Default(_ context.Context, tmpl *AerospikeClusterTemplate) error {
	aerospikeclustertemplatelog.Info("Defaulting", "name", tmpl.Name)

	// Default scheduling.podAntiAffinityLevel to "preferred" if scheduling is set but level is empty.
	if tmpl.Spec.Scheduling != nil && tmpl.Spec.Scheduling.PodAntiAffinityLevel == "" {
		tmpl.Spec.Scheduling.PodAntiAffinityLevel = PodAntiAffinityPreferred
	}

	// Default storage.volumeMode to Filesystem if storage is specified.
	if tmpl.Spec.Storage != nil && tmpl.Spec.Storage.VolumeMode == "" {
		tmpl.Spec.Storage.VolumeMode = corev1.PersistentVolumeFilesystem
	}

	// Default storage.accessModes to ReadWriteOnce if not set.
	if tmpl.Spec.Storage != nil && len(tmpl.Spec.Storage.AccessModes) == 0 {
		tmpl.Spec.Storage.AccessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}
	}

	return nil
}

// +kubebuilder:webhook:path=/validate-acko-io-v1alpha1-aerospikeclustertemplate,mutating=false,failurePolicy=fail,sideEffects=None,groups=acko.io,resources=aerospikeclustertemplates,verbs=create;update,versions=v1alpha1,name=vaerospikeclustertemplate.kb.io,admissionReviewVersions=v1

// AerospikeClusterTemplateValidator implements admission.Validator for AerospikeClusterTemplate.
type AerospikeClusterTemplateValidator struct{}

var _ admission.Validator[*AerospikeClusterTemplate] = &AerospikeClusterTemplateValidator{}

// ValidateCreate implements admission.Validator[*AerospikeClusterTemplate].
func (v *AerospikeClusterTemplateValidator) ValidateCreate(_ context.Context, tmpl *AerospikeClusterTemplate) (admission.Warnings, error) {
	aerospikeclustertemplatelog.Info("Validating create", "name", tmpl.Name)
	return v.validate(tmpl)
}

// ValidateUpdate implements admission.Validator[*AerospikeClusterTemplate].
func (v *AerospikeClusterTemplateValidator) ValidateUpdate(_ context.Context, _, tmpl *AerospikeClusterTemplate) (admission.Warnings, error) {
	aerospikeclustertemplatelog.Info("Validating update", "name", tmpl.Name)
	return v.validate(tmpl)
}

// ValidateDelete implements admission.Validator[*AerospikeClusterTemplate].
func (v *AerospikeClusterTemplateValidator) ValidateDelete(_ context.Context, _ *AerospikeClusterTemplate) (admission.Warnings, error) {
	return nil, nil
}

// validate performs all template-specific validations.
func (v *AerospikeClusterTemplateValidator) validate(tmpl *AerospikeClusterTemplate) (admission.Warnings, error) {
	var allErrors []string
	var warnings admission.Warnings

	spec := &tmpl.Spec

	// V-T01: podAntiAffinityLevel must be one of: none, preferred, required.
	if spec.Scheduling != nil {
		level := spec.Scheduling.PodAntiAffinityLevel
		if level != "" &&
			level != PodAntiAffinityNone &&
			level != PodAntiAffinityPreferred &&
			level != PodAntiAffinityRequired {
			allErrors = append(allErrors, fmt.Sprintf(
				"spec.scheduling.podAntiAffinityLevel must be one of: none, preferred, required (got %q)", level))
		}

		// V-T05: podManagementPolicy validation.
		pm := spec.Scheduling.PodManagementPolicy
		if pm != "" {
			switch string(pm) {
			case "OrderedReady", "Parallel":
				// valid
			default:
				allErrors = append(allErrors, fmt.Sprintf(
					"spec.scheduling.podManagementPolicy must be one of: OrderedReady, Parallel (got %q)", pm))
			}
		}
	}

	// V-T02: maxRacksPerNode must be >= 0.
	if spec.RackConfig != nil && spec.RackConfig.MaxRacksPerNode < 0 {
		allErrors = append(allErrors, fmt.Sprintf(
			"spec.rackConfig.maxRacksPerNode must be >= 0 (got %d)", spec.RackConfig.MaxRacksPerNode))
	}

	// V-T03: localPVRequired=true warns if storageClassName is empty.
	if spec.Storage != nil && spec.Storage.LocalPVRequired && spec.Storage.StorageClassName == "" {
		warnings = append(warnings, "spec.storage.localPVRequired=true but spec.storage.storageClassName is empty; local PV scheduling may fail")
	}

	// V-T04: Guaranteed QoS warning when requests != limits.
	if spec.Resources != nil {
		if !templateResourcesEqualRequestsLimits(spec.Resources) {
			warnings = append(warnings, "for Guaranteed QoS, resource requests should equal resource limits in spec.resources")
		}
	}

	// V-T06: Image must not reference an Enterprise build (the canonical
	// Docker Hub repo is `aerospike-server-enterprise`; "ee-" / "ent-" /
	// generic "enterprise" substrings also indicate EE builds). Without this,
	// any cluster pointing at this template silently inherits an EE image,
	// bypassing the AerospikeCluster CE check.
	//
	// V-T07: Size must be 1–8 (CE limit) when the template explicitly supplies
	// a default — same bound as spec.size on AerospikeCluster.
	//
	// V-T08: Monitoring port must be in the valid TCP range when set —
	// catching templates that would later produce an invalid Service/SM.
	//
	// internal/template/validation.go has a ValidateTemplateSpec helper that
	// implements the same checks, but we cannot call it here without an
	// import cycle (internal/template -> api/v1alpha1). Inline the three
	// checks instead so the template webhook actually enforces them.
	allErrors = append(allErrors, validateTemplateImageCE(spec.Image)...)
	allErrors = append(allErrors, validateTemplateSizeCE(spec.Size)...)
	allErrors = append(allErrors, validateTemplateMonitoringPort(spec.Monitoring)...)

	// Validate aerospikeConfig if present: heartbeat mode must be mesh for CE.
	if spec.AerospikeConfig != nil {
		if spec.AerospikeConfig.Network != nil && spec.AerospikeConfig.Network.Heartbeat != nil {
			mode := spec.AerospikeConfig.Network.Heartbeat.Mode
			if mode != "" && mode != "mesh" {
				allErrors = append(allErrors, fmt.Sprintf(
					"spec.aerospikeConfig.network.heartbeat.mode must be 'mesh' for CE (got %q)", mode))
			}
		}

		// CE constraint: enterprise-only stanzas (xdr, tls) and enterprise-only
		// security sub-keys must be rejected on the template paths just as they
		// are on AerospikeCluster.spec.aerospikeConfig (see
		// validateAerospikeConfig in aerospikecluster_webhook.go). Without this,
		// a templateRef could silently inject these stanzas via
		// namespaceDefaults or service defaults, bypassing the CE webhook.
		//
		// namespaceDefaults additionally must reject enterpriseOnlyNamespaceKeys
		// (compression, strong-consistency, durable-delete, ...): the cluster
		// webhook checks those per-namespace, but namespaceDefaults is applied
		// as a base to every namespace and would otherwise be a silent bypass.
		if spec.AerospikeConfig.NamespaceDefaults != nil {
			allErrors = append(allErrors,
				validateTemplateConfigBannedKeys("spec.aerospikeConfig.namespaceDefaults",
					spec.AerospikeConfig.NamespaceDefaults.Value, true)...)
		}
		if spec.AerospikeConfig.Service != nil {
			allErrors = append(allErrors,
				validateTemplateConfigBannedKeys("spec.aerospikeConfig.service",
					spec.AerospikeConfig.Service.Value, false)...)
		}
	}

	if len(allErrors) > 0 {
		return warnings, fmt.Errorf("template validation failed: %s", strings.Join(allErrors, "; "))
	}

	return warnings, nil
}

// validateTemplateConfigBannedKeys rejects enterprise-only Aerospike config
// stanzas reachable from a template's namespaceDefaults or service map.
//
// Mirrors the CE constraints enforced in validateAerospikeConfig on the
// AerospikeCluster webhook:
//   - top-level "xdr" and "tls" keys are forbidden,
//   - "security" sub-keys listed in enterpriseOnlySecurityKeys are forbidden,
//   - when isNamespaceDefaults is true, top-level keys listed in
//     enterpriseOnlyNamespaceKeys are forbidden (compression, strong-
//     consistency, durable-delete, ...). These keys would otherwise leak into
//     every namespace via namespaceDefaults base merging and silently bypass
//     the per-namespace check in validateNamespaceConfig.
//
// fieldPath is the user-facing JSONPath prefix used in error messages
// (e.g. "spec.aerospikeConfig.namespaceDefaults"). isNamespaceDefaults must
// be true only for the namespaceDefaults path; service / other paths set
// false to keep the existing checks unchanged.
func validateTemplateConfigBannedKeys(fieldPath string, cfg map[string]any, isNamespaceDefaults bool) []string {
	if cfg == nil {
		return nil
	}
	var errs []string

	if _, exists := cfg["xdr"]; exists {
		errs = append(errs, fmt.Sprintf(
			"%s must not contain 'xdr' section (XDR is Enterprise-only)", fieldPath))
	}
	if _, exists := cfg["tls"]; exists {
		errs = append(errs, fmt.Sprintf(
			"%s must not contain 'tls' section (TLS is Enterprise-only)", fieldPath))
	}
	if secSection, exists := cfg["security"]; exists {
		secMap, ok := secSection.(map[string]any)
		if !ok {
			errs = append(errs, fmt.Sprintf(
				"%s.security must be a map, got %T", fieldPath, secSection))
		} else {
			for enterpriseKey, reason := range enterpriseOnlySecurityKeys {
				if _, found := secMap[enterpriseKey]; found {
					errs = append(errs, fmt.Sprintf(
						"%s.security.%s is not allowed in CE edition (%s)",
						fieldPath, enterpriseKey, reason))
				}
			}
		}
	}
	if isNamespaceDefaults {
		for enterpriseKey, reason := range enterpriseOnlyNamespaceKeys {
			if _, found := cfg[enterpriseKey]; found {
				errs = append(errs, fmt.Sprintf(
					"%s.%s is not allowed in CE edition (%s); namespaceDefaults is applied as a base to every namespace",
					fieldPath, enterpriseKey, reason))
			}
		}
	}
	return errs
}

// validateTemplateImageCE rejects template images that reference an
// Enterprise build. Mirrors the cluster-spec image check in
// validateSizeAndImage (aerospikecluster_webhook.go) so the same constraints
// apply whether a user supplies the image directly or via a template.
//
// Empty image is allowed: clusters that reference the template are required
// to supply spec.image themselves and the cluster webhook validates it.
func validateTemplateImageCE(image string) []string {
	if image == "" {
		return nil
	}
	imageLower := strings.ToLower(image)
	switch {
	case strings.Contains(imageLower, "aerospike-server-enterprise"):
		return []string{fmt.Sprintf(
			"spec.image %q references the enterprise repository "+
				"(aerospike-server-enterprise); CE clusters must use a CE image "+
				"such as aerospike:ce-8.1.1.1",
			image)}
	case strings.Contains(imageLower, "enterprise") || isEnterpriseTag(image):
		return []string{fmt.Sprintf(
			"spec.image %q is an Enterprise Edition image; only Community Edition images are allowed",
			image)}
	}
	return nil
}

// validateTemplateSizeCE enforces the CE size bound (1–8) when the template
// explicitly supplies a Size value. Nil Size is allowed (the cluster will
// then need to provide its own).
//
// The CRD already declares Minimum=1/Maximum=8 on Size, so this is defence
// in depth — webhook errors are clearer than CRD validation errors and run
// before anything else.
func validateTemplateSizeCE(size *int32) []string {
	if size == nil {
		return nil
	}
	if *size < 1 || *size > maxCEClusterSize {
		return []string{fmt.Sprintf(
			"spec.size must be between 1 and %d (CE limit), got %d", maxCEClusterSize, *size)}
	}
	return nil
}

// validateTemplateMonitoringPort enforces a valid TCP port range on
// monitoring.port when the template supplies a non-zero value. A zero port
// means "use the cluster default" and is left to the cluster webhook /
// defaulter.
func validateTemplateMonitoringPort(m *AerospikeMonitoringSpec) []string {
	if m == nil || m.Port == 0 {
		return nil
	}
	if m.Port < 1 || m.Port > 65535 {
		return []string{fmt.Sprintf(
			"spec.monitoring.port must be between 1 and 65535, got %d", m.Port)}
	}
	return nil
}

// templateResourcesEqualRequestsLimits checks if CPU and memory requests equal limits.
func templateResourcesEqualRequestsLimits(r *corev1.ResourceRequirements) bool {
	checkResource := func(name corev1.ResourceName) bool {
		req, hasReq := r.Requests[name]
		lim, hasLim := r.Limits[name]
		if !hasReq && !hasLim {
			return true
		}
		if hasReq != hasLim {
			return false
		}
		return req.Cmp(lim) == 0
	}
	return checkResource(corev1.ResourceCPU) && checkResource(corev1.ResourceMemory)
}
