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
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	ackov1alpha1 "github.com/ksr/aerospike-ce-kubernetes-operator/api/v1alpha1"
)

// ValidateResolvedSpec performs cross-field validation on the resolved spec.
// It returns a list of warning messages (non-fatal).
func ValidateResolvedSpec(
	resolved *ackov1alpha1.AerospikeClusterSpec,
	templateSpec *ackov1alpha1.AerospikeClusterTemplateSpec,
) []string {
	if templateSpec == nil {
		return nil
	}

	var warnings []string

	scheduling := templateSpec.Scheduling
	rackConfig := templateSpec.RackConfig

	// maxRacksPerNode=1 implies strict anti-affinity and local PV.
	if rackConfig != nil && rackConfig.MaxRacksPerNode == 1 {
		if scheduling == nil || scheduling.PodAntiAffinityLevel != ackov1alpha1.PodAntiAffinityRequired {
			warnings = append(warnings, "maxRacksPerNode=1 requires podAntiAffinityLevel=required to ensure one rack per node")
		}
		if templateSpec.Storage == nil || !templateSpec.Storage.LocalPVRequired {
			warnings = append(warnings, "maxRacksPerNode=1 recommends localPVRequired=true to pin storage to the scheduled node")
		}
		if resolved.PodSpec != nil && len(resolved.PodSpec.Tolerations) == 0 {
			warnings = append(warnings, "maxRacksPerNode=1 in production should define tolerations to control node access")
		}
	}

	// Guaranteed QoS: requests must equal limits.
	if templateSpec.Resources != nil {
		if !resourceRequestsEqualLimits(templateSpec.Resources) {
			warnings = append(warnings, "for Guaranteed QoS, resource requests must equal resource limits")
		}
	}

	return warnings
}

// resourceRequestsEqualLimits checks if resource requests equal limits.
func resourceRequestsEqualLimits(r *corev1.ResourceRequirements) bool {
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

// ValidateTemplateSpec validates the template spec for correctness.
// Returns errors (fatal) and warnings (non-fatal).
func ValidateTemplateSpec(spec *ackov1alpha1.AerospikeClusterTemplateSpec) ([]string, []string) {
	var errs []string
	var warnings []string

	if spec == nil {
		return nil, nil
	}

	// V-T01: podAntiAffinityLevel validation.
	if spec.Scheduling != nil {
		level := spec.Scheduling.PodAntiAffinityLevel
		if level != "" &&
			level != ackov1alpha1.PodAntiAffinityNone &&
			level != ackov1alpha1.PodAntiAffinityPreferred &&
			level != ackov1alpha1.PodAntiAffinityRequired {
			errs = append(errs, fmt.Sprintf("scheduling.podAntiAffinityLevel must be one of: none, preferred, required (got %q)", level))
		}
	}

	// V-T02: maxRacksPerNode validation.
	if spec.RackConfig != nil && spec.RackConfig.MaxRacksPerNode < 0 {
		errs = append(errs, fmt.Sprintf("rackConfig.maxRacksPerNode must be >= 0 (got %d)", spec.RackConfig.MaxRacksPerNode))
	}

	// V-T03: localPVRequired requires storageClassName.
	if spec.Storage != nil && spec.Storage.LocalPVRequired && spec.Storage.StorageClassName == "" {
		warnings = append(warnings, "storage.localPVRequired=true is specified but storage.storageClassName is empty; local PV scheduling may fail")
	}

	// V-T04: Guaranteed QoS warning.
	if spec.Resources != nil {
		if !resourceRequestsEqualLimits(spec.Resources) {
			warnings = append(warnings, "for Guaranteed QoS, resource requests should equal resource limits")
		}
	}

	// V-T05: podManagementPolicy validation.
	if spec.Scheduling != nil {
		pm := spec.Scheduling.PodManagementPolicy
		if pm != "" {
			switch pm {
			case "OrderedReady", "Parallel":
				// valid
			default:
				errs = append(errs, fmt.Sprintf("scheduling.podManagementPolicy must be one of: OrderedReady, Parallel (got %q)", pm))
			}
		}
	}

	// Validate storage resources if specified.
	if spec.Storage != nil {
		if spec.Storage.Resources.Requests != nil {
			if qty, ok := spec.Storage.Resources.Requests[corev1.ResourceStorage]; ok {
				if qty.Cmp(resource.MustParse("0")) <= 0 {
					errs = append(errs, "storage.resources.requests.storage must be > 0")
				}
			}
		}
	}

	// V-T06: Image must be a CE image — enterprise images are rejected.
	imgErrs, imgWarnings := validateTemplateImage(spec.Image)
	errs = append(errs, imgErrs...)
	warnings = append(warnings, imgWarnings...)

	// V-T07: Size must be in the CE-allowed range (1–8) when specified.
	if spec.Size != nil {
		if *spec.Size < 1 || *spec.Size > 8 {
			errs = append(errs, fmt.Sprintf("size must be between 1 and 8 (CE limit), got %d", *spec.Size))
		}
	}

	// V-T08: Monitoring port must be in valid range when specified.
	if spec.Monitoring != nil && spec.Monitoring.Port != 0 {
		if spec.Monitoring.Port < 1 || spec.Monitoring.Port > 65535 {
			errs = append(errs, fmt.Sprintf("monitoring.port must be between 1 and 65535, got %d", spec.Monitoring.Port))
		}
	}

	return errs, warnings
}

// validateTemplateImage enforces the CE-only image constraint for templates.
//
// CE constraint enforcement: when AerospikeClusterTemplate.spec.image points
// at an enterprise build, every cluster that references this template would
// silently inherit an enterprise image. Previously this was a warning only,
// so the constraint could be bypassed via templateRef. We now reject:
//  1. images whose lower-cased reference contains "aerospike-server-enterprise"
//     (the canonical enterprise repository name on Docker Hub), and
//  2. images whose tag/repository contains a generic "enterprise" substring.
//
// CE images typically contain a "ce-" tag prefix (e.g. aerospike:ce-8.1.1.1);
// retagged custom-registry CE images that omit "ce-" are reported as a
// warning so users can confirm but are not blocked.
func validateTemplateImage(image string) (errs []string, warnings []string) {
	if image == "" {
		return nil, nil
	}

	imageLower := strings.ToLower(image)
	switch {
	case strings.Contains(imageLower, "aerospike-server-enterprise"):
		errs = append(errs, fmt.Sprintf(
			"template image %q references the enterprise repository "+
				"(aerospike-server-enterprise); CE clusters must use a CE image (CE constraint)",
			image,
		))
	case strings.Contains(imageLower, "enterprise"):
		errs = append(errs, fmt.Sprintf(
			"template image %q appears to be an enterprise image (contains 'enterprise'); "+
				"CE clusters must use a CE image (CE constraint)",
			image,
		))
	case !strings.Contains(image, "ce-"):
		warnings = append(warnings, fmt.Sprintf(
			"template image %q may not be a CE image; CE images typically contain 'ce-' (e.g., aerospike:ce-8.1.1.1). "+
				"Retagged or custom-registry CE images that omit 'ce-' will also trigger this warning.",
			image,
		))
	}
	return errs, warnings
}
