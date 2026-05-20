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
	"strings"
	"testing"

	ackov1alpha1 "github.com/ksr/aerospike-ce-kubernetes-operator/api/v1alpha1"
)

// --- V-T06: Image CE-tag warning ---

func TestValidateTemplateSpec_V_T06_NoWarningForCEImage(t *testing.T) {
	spec := &ackov1alpha1.AerospikeClusterTemplateSpec{
		Image: testImageCE8,
	}
	errs, warnings := ValidateTemplateSpec(spec)
	if len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
	for _, w := range warnings {
		if strings.Contains(w, "may not be a CE image") {
			t.Errorf("unexpected CE-image warning for a valid CE image: %q", w)
		}
	}
}

func TestValidateTemplateSpec_V_T06_WarningForNonCEImage(t *testing.T) {
	spec := &ackov1alpha1.AerospikeClusterTemplateSpec{
		Image: "myregistry.io/aerospike:8.1.1.1",
	}
	errs, warnings := ValidateTemplateSpec(spec)
	if len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
	found := false
	for _, w := range warnings {
		if strings.Contains(w, "may not be a CE image") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected CE-image warning for non-CE image, got warnings: %v", warnings)
	}
}

func TestValidateTemplateSpec_V_T06_NoWarningWhenImageEmpty(t *testing.T) {
	spec := &ackov1alpha1.AerospikeClusterTemplateSpec{}
	_, warnings := ValidateTemplateSpec(spec)
	for _, w := range warnings {
		if strings.Contains(w, "may not be a CE image") {
			t.Errorf("unexpected CE-image warning when image is empty: %q", w)
		}
	}
}

func TestValidateTemplateSpec_V_T06_RejectsEnterpriseRepository(t *testing.T) {
	spec := &ackov1alpha1.AerospikeClusterTemplateSpec{
		Image: "aerospike/aerospike-server-enterprise:8.1.0.0",
	}
	errs, _ := ValidateTemplateSpec(spec)
	if len(errs) == 0 {
		t.Error("expected error for enterprise-repository image")
	}
}

func TestValidateTemplateSpec_V_T06_RejectsEnterpriseEditionTag(t *testing.T) {
	// A custom registry name with no "enterprise" substring must still be
	// rejected when the tag carries an Enterprise Edition prefix, matching
	// the cluster webhook's isEnterpriseTag check.
	for _, img := range []string{
		"myregistry.io/aerospike:ee-8.0.0.1_1",
		"myregistry.io/aerospike:ent-8.0.0",
	} {
		spec := &ackov1alpha1.AerospikeClusterTemplateSpec{Image: img}
		errs, _ := ValidateTemplateSpec(spec)
		if len(errs) == 0 {
			t.Errorf("expected error for Enterprise Edition tag %q", img)
		}
	}
}

// --- V-T07: Size range (1–8) ---

func TestValidateTemplateSpec_V_T07_SizeBelowMinIsError(t *testing.T) {
	size := int32(0)
	spec := &ackov1alpha1.AerospikeClusterTemplateSpec{Size: &size}
	errs, _ := ValidateTemplateSpec(spec)
	if len(errs) == 0 {
		t.Errorf("expected error for size=0")
	}
}

func TestValidateTemplateSpec_V_T07_SizeAboveMaxIsError(t *testing.T) {
	size := int32(9)
	spec := &ackov1alpha1.AerospikeClusterTemplateSpec{Size: &size}
	errs, _ := ValidateTemplateSpec(spec)
	if len(errs) == 0 {
		t.Errorf("expected error for size=9")
	}
}

func TestValidateTemplateSpec_V_T07_SizeBoundariesAreValid(t *testing.T) {
	for _, s := range []int32{1, 4, 8} {
		size := s
		spec := &ackov1alpha1.AerospikeClusterTemplateSpec{Size: &size}
		errs, _ := ValidateTemplateSpec(spec)
		if len(errs) != 0 {
			t.Errorf("expected no error for size=%d, got %v", s, errs)
		}
	}
}

func TestValidateTemplateSpec_V_T07_SizeNilNoError(t *testing.T) {
	spec := &ackov1alpha1.AerospikeClusterTemplateSpec{}
	errs, _ := ValidateTemplateSpec(spec)
	if len(errs) != 0 {
		t.Errorf("expected no error when size is nil, got %v", errs)
	}
}

// --- V-T08: Monitoring port range (1–65535) ---

func TestValidateTemplateSpec_V_T08_PortZeroNoError(t *testing.T) {
	// Port=0 means "not set"; should not trigger validation.
	spec := &ackov1alpha1.AerospikeClusterTemplateSpec{
		Monitoring: &ackov1alpha1.AerospikeMonitoringSpec{Port: 0},
	}
	errs, _ := ValidateTemplateSpec(spec)
	if len(errs) != 0 {
		t.Errorf("expected no error for monitoring.port=0, got %v", errs)
	}
}

func TestValidateTemplateSpec_V_T08_ValidPortNoError(t *testing.T) {
	spec := &ackov1alpha1.AerospikeClusterTemplateSpec{
		Monitoring: &ackov1alpha1.AerospikeMonitoringSpec{Port: 9145},
	}
	errs, _ := ValidateTemplateSpec(spec)
	if len(errs) != 0 {
		t.Errorf("expected no error for monitoring.port=9145, got %v", errs)
	}
}

func TestValidateTemplateSpec_V_T08_PortAboveMaxIsError(t *testing.T) {
	spec := &ackov1alpha1.AerospikeClusterTemplateSpec{
		Monitoring: &ackov1alpha1.AerospikeMonitoringSpec{Port: 65536},
	}
	errs, _ := ValidateTemplateSpec(spec)
	if len(errs) == 0 {
		t.Errorf("expected error for monitoring.port=65536")
	}
}

func TestValidateTemplateSpec_V_T08_MonitoringNilNoError(t *testing.T) {
	spec := &ackov1alpha1.AerospikeClusterTemplateSpec{}
	errs, _ := ValidateTemplateSpec(spec)
	if len(errs) != 0 {
		t.Errorf("expected no error when monitoring is nil, got %v", errs)
	}
}
