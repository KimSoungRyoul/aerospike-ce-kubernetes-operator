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

package template

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ackov1alpha1 "github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/api/v1alpha1"
	ackoerrors "github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/internal/errors"
)

const (
	// AnnotationResyncTemplate triggers a manual template resync when set to "true".
	AnnotationResyncTemplate = "acko.io/resync-template"

	// maxCEClusterSize is the CE node-count cap. Mirrors the constant of the
	// same name in api/v1alpha1; duplicated here to avoid an import cycle
	// (api/v1alpha1 already imports nothing from internal/template, but
	// re-exposing the constant from api/v1alpha1 isn't necessary either —
	// keeping a short package-local copy is the simplest path).
	maxCEClusterSize = 8
)

// enterpriseOnlySecurityKeysCE lists security sub-keys that are
// Enterprise-only on the CE edition (mirror of the map of the same name in
// api/v1alpha1). Kept package-local to avoid circular imports.
var enterpriseOnlySecurityKeysCE = map[string]string{
	"tls":    "TLS within the security stanza is Enterprise-only",
	"ldap":   "LDAP authentication is Enterprise-only",
	"log":    "security audit logging is Enterprise-only",
	"syslog": "security syslog sink is Enterprise-only",
}

// enterpriseOnlyNamespaceKeysCE lists namespace-level Aerospike config keys
// that are Enterprise-only (mirror of api/v1alpha1.enterpriseOnlyNamespaceKeys).
// Used by the post-merge re-validator in validateMergedConfigCE.
var enterpriseOnlyNamespaceKeysCE = map[string]string{
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

// imageTag returns the tag portion of a container image reference, or "" when
// the reference has no tag. A colon before the final '/' belongs to a registry
// host:port (e.g. "myregistry.io:5000/aerospike:ee-8.0.0"), not the tag, so the
// tag is taken from the last colon only when it follows the last '/'. A
// digest-pinned ref's "@<algo>:<hex>" suffix is stripped first so the digest's
// own colon is not misread as the tag separator. Mirrors
// api/v1alpha1.imageTag — duplicated to avoid an import cycle.
func imageTag(image string) string {
	if at := strings.LastIndex(image, "@"); at >= 0 && at > strings.LastIndex(image, "/") {
		image = image[:at]
	}
	lastColon := strings.LastIndex(image, ":")
	if lastColon < 0 {
		return ""
	}
	if slash := strings.LastIndex(image, "/"); slash > lastColon {
		return ""
	}
	return image[lastColon+1:]
}

// isEnterpriseTag returns true for image tags that indicate an Enterprise
// Edition build (e.g., "aerospike:ee-8.0.0.1_1", "aerospike:ent-8.0.0").
// Mirrors api/v1alpha1.isEnterpriseTag — duplicated to avoid an import
// cycle.
func isEnterpriseTag(image string) bool {
	tagLower := strings.ToLower(imageTag(image))
	return strings.HasPrefix(tagLower, "ee-") || strings.HasPrefix(tagLower, "ent-")
}

// tlsKeyPrefixCE is the prefix shared by every per-endpoint TLS key Aerospike
// accepts inside a network sub-stanza (tls-port, tls-name,
// tls-authenticate-client, ...). No CE configuration key starts with it.
// Mirrors api/v1alpha1.tlsKeyPrefix — duplicated to keep this file's
// package-local-mirror convention.
const tlsKeyPrefixCE = "tls-"

// tlsNetworkSubsectionsCE lists the network sub-stanzas that accept
// per-endpoint TLS keys. Mirrors api/v1alpha1.tlsNetworkSubsections.
var tlsNetworkSubsectionsCE = []string{"service", "heartbeat", "fabric"}

// validateMergedNetworkTLSCE rejects Enterprise-only TLS configuration inside
// the merged config's network stanza: the `network { tls <name> { ... } }`
// definition and per-endpoint tls-* keys under service / heartbeat / fabric.
//
// This is the post-merge half of the check — the merged map is what configgen
// actually consumes, and a template default combined with a cluster override
// can produce a TLS stanza that neither input carried on its own. Mirrors
// api/v1alpha1.validateNetworkTLSCE; see that function for why the top-level
// `tls` key alone does not cover the TLS surface.
func validateMergedNetworkTLSCE(config map[string]any) []string {
	netCfg, ok := config["network"].(map[string]any)
	if !ok {
		return nil
	}

	var errs []string

	if _, exists := netCfg["tls"]; exists {
		errs = append(errs,
			"merged aerospikeConfig.network must not contain 'tls' section (TLS is Enterprise-only)")
	}

	for _, subsection := range tlsNetworkSubsectionsCE {
		subMap, ok := netCfg[subsection].(map[string]any)
		if !ok {
			continue
		}
		// Sorted for a deterministic message: map iteration order is random and
		// a stanza can carry several tls-* keys at once.
		var tlsKeys []string
		for key := range subMap {
			if strings.HasPrefix(key, tlsKeyPrefixCE) {
				tlsKeys = append(tlsKeys, key)
			}
		}
		slices.Sort(tlsKeys)
		for _, key := range tlsKeys {
			errs = append(errs, fmt.Sprintf(
				"merged aerospikeConfig.network.%s.%s is not allowed in CE edition (TLS is Enterprise-only)",
				subsection, key))
		}
	}

	return errs
}

// validateMergedConfigCE reapplies the CE-specific aerospikeConfig checks
// (xdr/tls absent — including network TLS, no enterprise security sub-keys, no
// enterprise namespace keys) on the materialised cluster config map. Mirrors
// the CE checks performed by the cluster webhook on the raw aerospikeConfig.
func validateMergedConfigCE(config map[string]any) []string {
	if config == nil {
		return nil
	}
	var errs []string

	if _, exists := config["xdr"]; exists {
		errs = append(errs, "merged aerospikeConfig must not contain 'xdr' section (XDR is Enterprise-only)")
	}
	if _, exists := config["tls"]; exists {
		errs = append(errs, "merged aerospikeConfig must not contain 'tls' section (TLS is Enterprise-only)")
	}
	errs = append(errs, validateMergedNetworkTLSCE(config)...)
	if secSection, exists := config["security"]; exists {
		if secMap, ok := secSection.(map[string]any); ok {
			for enterpriseKey, reason := range enterpriseOnlySecurityKeysCE {
				if _, found := secMap[enterpriseKey]; found {
					errs = append(errs, fmt.Sprintf(
						"merged aerospikeConfig.security.%s is not allowed in CE edition (%s)",
						enterpriseKey, reason))
				}
			}
		}
	}
	// Per-namespace enterprise-only key check. namespaceDefaults already
	// flowed through the template+overrides check, but the merged namespaces
	// list is what configgen actually consumes.
	if nsSection, exists := config["namespaces"]; exists {
		if nsList, ok := nsSection.([]any); ok {
			for i, nsEntry := range nsList {
				nsMap, ok := nsEntry.(map[string]any)
				if !ok {
					continue
				}
				nsName := "<unknown>"
				if name, ok := nsMap["name"].(string); ok {
					nsName = name
				}
				for enterpriseKey, reason := range enterpriseOnlyNamespaceKeysCE {
					if _, found := nsMap[enterpriseKey]; found {
						errs = append(errs, fmt.Sprintf(
							"merged namespace[%d] %q: '%s' is not allowed (%s)",
							i, nsName, enterpriseKey, reason))
					}
				}
			}
		}
	}

	return errs
}

// ResolveResult holds the outcome of template resolution.
type ResolveResult struct {
	// SnapshotUpdated is true if the template snapshot was created or refreshed.
	SnapshotUpdated bool
	// AnnotationNeedsCleanup is true when the resync annotation was consumed and must
	// be removed from the cluster object by the caller (via a Patch call).
	AnnotationNeedsCleanup bool
	// Warnings contains non-fatal messages from validation.
	Warnings []string
}

// FetchAndSnapshot fetches the referenced template and stores it as a snapshot
// in the cluster's status. Returns the fetched template spec.
func FetchAndSnapshot(
	ctx context.Context,
	reader client.Reader,
	cluster *ackov1alpha1.AerospikeCluster,
) (*ackov1alpha1.AerospikeClusterTemplateSpec, bool, error) {
	if cluster.Spec.TemplateRef == nil {
		return nil, false, nil
	}

	tmpl := &ackov1alpha1.AerospikeClusterTemplate{}
	if err := reader.Get(ctx, types.NamespacedName{
		Name: cluster.Spec.TemplateRef.Name,
	}, tmpl); err != nil {
		if errors.IsNotFound(err) {
			return nil, false, ackoerrors.NewValidationf("template %q not found", cluster.Spec.TemplateRef.Name)
		}
		return nil, false, fmt.Errorf("fetching template %q: %w", cluster.Spec.TemplateRef.Name, err)
	}

	specCopy := tmpl.Spec.DeepCopy()
	snapshot := &ackov1alpha1.TemplateSnapshotStatus{
		Name:              tmpl.Name,
		ResourceVersion:   tmpl.ResourceVersion,
		SnapshotTimestamp: metav1.NewTime(time.Now()),
		Synced:            true,
		Spec:              specCopy,
	}
	cluster.Status.TemplateSnapshot = snapshot

	return specCopy, true, nil
}

// NeedsResync returns true if the template snapshot should be refreshed.
// This happens when:
// 1. No snapshot exists (first creation).
// 2. The "acko.io/resync-template: true" annotation is present.
func NeedsResync(cluster *ackov1alpha1.AerospikeCluster) bool {
	if cluster.Spec.TemplateRef == nil {
		return false
	}
	if cluster.Status.TemplateSnapshot == nil {
		return true
	}
	if cluster.Annotations != nil && cluster.Annotations[AnnotationResyncTemplate] == "true" {
		return true
	}
	return false
}

// ApplyTemplate applies the resolved template spec (after merge with overrides)
// to the cluster's spec in-memory. The API server object is not modified.
// Only fields not already explicitly set in the cluster spec are applied.
//
// Returns an error when applying aerospikeConfig fails (e.g. the cluster's
// aerospikeConfig.service exists with a non-map type and the template
// supplies service defaults — see applyAerospikeConfig). Other apply* steps
// are total functions and never fail.
func ApplyTemplate(resolvedTemplateSpec *ackov1alpha1.AerospikeClusterTemplateSpec, cluster *ackov1alpha1.AerospikeCluster) error {
	if resolvedTemplateSpec == nil {
		return nil
	}

	// Apply scheduling (pod anti-affinity, tolerations, node affinity).
	applyScheduling(resolvedTemplateSpec.Scheduling, cluster)

	// Apply storage defaults.
	applyStorage(resolvedTemplateSpec.Storage, cluster)

	// Apply Aerospike config defaults. May fail if the cluster's existing
	// aerospikeConfig.service value isn't a map (defensive check; the
	// cluster webhook normally rejects such specs at admission).
	if err := applyAerospikeConfig(resolvedTemplateSpec.AerospikeConfig, cluster); err != nil {
		return err
	}

	// Apply resource defaults.
	if resolvedTemplateSpec.Resources != nil {
		if cluster.Spec.PodSpec == nil {
			cluster.Spec.PodSpec = &ackov1alpha1.AerospikePodSpec{}
		}
		if cluster.Spec.PodSpec.AerospikeContainerSpec == nil {
			cluster.Spec.PodSpec.AerospikeContainerSpec = &ackov1alpha1.AerospikeContainerSpec{}
		}
		if cluster.Spec.PodSpec.AerospikeContainerSpec.Resources == nil {
			cluster.Spec.PodSpec.AerospikeContainerSpec.Resources = resolvedTemplateSpec.Resources.DeepCopy()
		}
	}

	// Apply image, size, monitoring, and network policy defaults.
	applyImage(resolvedTemplateSpec.Image, cluster)
	applySize(resolvedTemplateSpec.Size, cluster)
	applyMonitoring(resolvedTemplateSpec.Monitoring, cluster)
	applyNetworkPolicy(resolvedTemplateSpec.AerospikeNetworkPolicy, cluster)
	return nil
}

// Resolve is the main entry point for template resolution in the reconciler.
// It:
//  1. Checks if a resync is needed and fetches+snapshots the template if so.
//  2. Merges the snapshot spec with any cluster-level overrides.
//  3. Applies the merged spec to the cluster's in-memory spec.
//
// Returns ResolveResult and an error if the template fetch fails.
func Resolve(
	ctx context.Context,
	reader client.Reader,
	cluster *ackov1alpha1.AerospikeCluster,
) (ResolveResult, error) {
	result := ResolveResult{}

	if cluster.Spec.TemplateRef == nil {
		return result, nil
	}

	// Determine if we need to (re)fetch the template.
	if NeedsResync(cluster) {
		annotationTriggered := cluster.Annotations != nil && cluster.Annotations[AnnotationResyncTemplate] == "true"

		_, updated, err := FetchAndSnapshot(ctx, reader, cluster)
		if err != nil {
			return result, err
		}
		result.SnapshotUpdated = updated

		// Signal that the annotation must be deleted from the API server by the caller.
		// We do NOT delete it in-memory here to avoid a stale resourceVersion when the
		// caller subsequently patches the object.
		if annotationTriggered && updated {
			result.AnnotationNeedsCleanup = true
		}
	}

	// Build effective template spec: snapshot base + overrides.
	if cluster.Status.TemplateSnapshot == nil || cluster.Status.TemplateSnapshot.Spec == nil {
		return result, fmt.Errorf("template snapshot is missing or has no spec; cannot resolve template %q", cluster.Spec.TemplateRef.Name)
	}
	snapshotSpec := cluster.Status.TemplateSnapshot.Spec
	effectiveSpec := MergeTemplateSpec(snapshotSpec, cluster.Spec.Overrides)

	// Validate the effective spec.
	result.Warnings = ValidateResolvedSpec(&cluster.Spec, effectiveSpec)

	// Validate LocalPV StorageClass binding mode when localPVRequired is set.
	if effectiveSpec.Storage != nil && effectiveSpec.Storage.LocalPVRequired {
		if err := ValidateLocalPV(ctx, reader, effectiveSpec.Storage.StorageClassName); err != nil {
			result.Warnings = append(result.Warnings, "localPVRequired: "+err.Error())
		}
	}

	// Apply the effective template spec to the in-memory cluster spec.
	if err := ApplyTemplate(effectiveSpec, cluster); err != nil {
		return result, fmt.Errorf("applying template %q to cluster spec: %w", cluster.Spec.TemplateRef.Name, err)
	}

	// Post-template required field check: image and size must be resolvable after
	// applying both the cluster spec and the template. If either is still unset,
	// the template does not provide a sufficient default and reconciliation cannot proceed.
	if cluster.Spec.Image == "" {
		return result, fmt.Errorf(
			"spec.image is required: neither the cluster spec nor template %q provides an image",
			cluster.Spec.TemplateRef.Name,
		)
	}
	if cluster.Spec.Size == 0 {
		return result, fmt.Errorf(
			"spec.size is required: neither the cluster spec nor template %q provides a size",
			cluster.Spec.TemplateRef.Name,
		)
	}

	// Defence-in-depth: re-validate the materialised cluster spec against CE
	// constraints. The cluster webhook + template webhook already enforce
	// these on the wire, but the merged spec is the actual artefact that
	// configgen / the StatefulSet will consume — if a webhook is bypassed
	// (e.g. failurePolicy=Ignore on a future deployment) or a future merge
	// rule introduces a bypass, this catches it before any pod is created.
	if errs := validateMergedSpecCE(&cluster.Spec); len(errs) > 0 {
		return result, fmt.Errorf(
			"resolved spec violates CE constraints after applying template %q: %s",
			cluster.Spec.TemplateRef.Name,
			strings.Join(errs, "; "),
		)
	}

	return result, nil
}

// validateMergedSpecCE re-runs the core CE constraints on a fully-merged
// AerospikeClusterSpec. Returns a list of error messages; an empty list
// means the spec is acceptable.
//
// Checks (kept intentionally narrow — these are the bypassable ones):
//   - image is a CE image (no "aerospike-server-enterprise" repo, no
//     "enterprise" / "ee-" / "ent-" tag),
//   - size is in [1, maxCEClusterSize],
//   - aerospikeConfig top-level has no xdr/tls stanzas,
//   - aerospikeConfig.security has no enterprise-only sub-keys,
//   - aerospikeConfig.namespaces[*] has no enterprise-only namespace keys
//     (compression, strong-consistency, durable-delete, ...).
//
// Errors are returned (not warnings) because they describe configurations
// that would either be rejected by the CE server at start-up or violate the
// CE licence. They must not silently flow through to a StatefulSet.
func validateMergedSpecCE(spec *ackov1alpha1.AerospikeClusterSpec) []string {
	if spec == nil {
		return nil
	}
	var errs []string

	// Image
	if image := spec.Image; image != "" {
		imageLower := strings.ToLower(image)
		switch {
		case strings.Contains(imageLower, "aerospike-server-enterprise"):
			errs = append(errs, fmt.Sprintf(
				"merged spec.image %q references the enterprise repository (aerospike-server-enterprise)", image))
		case strings.Contains(imageLower, "enterprise") || isEnterpriseTag(image):
			errs = append(errs, fmt.Sprintf(
				"merged spec.image %q is an Enterprise Edition image", image))
		}
	}

	// Size
	if spec.Size < 1 || spec.Size > maxCEClusterSize {
		errs = append(errs, fmt.Sprintf(
			"merged spec.size %d must be between 1 and %d (CE limit)", spec.Size, maxCEClusterSize))
	}

	// aerospikeConfig
	if spec.AerospikeConfig != nil {
		errs = append(errs, validateMergedConfigCE(spec.AerospikeConfig.Value)...)
	}

	return errs
}
