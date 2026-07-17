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
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ackov1alpha1 "github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/api/v1alpha1"
)

const (
	testImageCE8     = "aerospike:ce-8.1.1.1"
	testImageCE8Old  = "aerospike:ce-8.0.0.0"
	testTopologyZone = "zone"
	testMutatedValue = "mutated"
)

// newCluster returns a minimal AerospikeCluster for testing.
// This helper is shared across apply_cluster_test.go and resolver_test.go
// (both in package template), so it is defined once here.
func newCluster() *ackov1alpha1.AerospikeCluster {
	return &ackov1alpha1.AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
	}
}

// --- applyImage ---

func TestApplyImage_AppliedWhenClusterImageEmpty(t *testing.T) {
	cluster := newCluster()
	applyImage(testImageCE8, cluster)
	if cluster.Spec.Image != testImageCE8 {
		t.Errorf("expected image to be applied, got %q", cluster.Spec.Image)
	}
}

func TestApplyImage_NotOverriddenWhenClusterImageSet(t *testing.T) {
	cluster := newCluster()
	cluster.Spec.Image = testImageCE8Old
	applyImage(testImageCE8, cluster)
	if cluster.Spec.Image != testImageCE8Old {
		t.Errorf("expected cluster image to be preserved, got %q", cluster.Spec.Image)
	}
}

func TestApplyImage_NoopWhenTemplateImageEmpty(t *testing.T) {
	cluster := newCluster()
	applyImage("", cluster)
	if cluster.Spec.Image != "" {
		t.Errorf("expected no change, got %q", cluster.Spec.Image)
	}
}

// --- applySize ---

func TestApplySize_AppliedWhenClusterSizeZero(t *testing.T) {
	cluster := newCluster()
	size := int32(3)
	applySize(&size, cluster)
	if cluster.Spec.Size != 3 {
		t.Errorf("expected size=3, got %d", cluster.Spec.Size)
	}
}

func TestApplySize_NotOverriddenWhenClusterSizeSet(t *testing.T) {
	cluster := newCluster()
	cluster.Spec.Size = 6
	size := int32(1)
	applySize(&size, cluster)
	if cluster.Spec.Size != 6 {
		t.Errorf("expected cluster size to be preserved (6), got %d", cluster.Spec.Size)
	}
}

func TestApplySize_NoopWhenTemplateSizeNil(t *testing.T) {
	cluster := newCluster()
	applySize(nil, cluster)
	if cluster.Spec.Size != 0 {
		t.Errorf("expected no change, got %d", cluster.Spec.Size)
	}
}

// --- applyMonitoring ---

func TestApplyMonitoring_AppliedWhenClusterMonitoringNil(t *testing.T) {
	cluster := newCluster()
	tmpl := &ackov1alpha1.AerospikeMonitoringSpec{
		Enabled: true,
		Port:    9145,
	}
	applyMonitoring(tmpl, cluster)
	if cluster.Spec.Monitoring == nil {
		t.Fatal("expected monitoring to be applied")
	}
	if !cluster.Spec.Monitoring.Enabled {
		t.Errorf("expected Enabled=true")
	}
	if cluster.Spec.Monitoring.Port != 9145 {
		t.Errorf("expected Port=9145, got %d", cluster.Spec.Monitoring.Port)
	}
}

func TestApplyMonitoring_NotOverriddenWhenClusterMonitoringSet(t *testing.T) {
	cluster := newCluster()
	cluster.Spec.Monitoring = &ackov1alpha1.AerospikeMonitoringSpec{
		Enabled: false,
		Port:    9200,
	}
	tmpl := &ackov1alpha1.AerospikeMonitoringSpec{
		Enabled: true,
		Port:    9145,
	}
	applyMonitoring(tmpl, cluster)
	if cluster.Spec.Monitoring.Port != 9200 {
		t.Errorf("expected cluster monitoring port to be preserved (9200), got %d", cluster.Spec.Monitoring.Port)
	}
}

func TestApplyMonitoring_NoopWhenTemplateMonitoringNil(t *testing.T) {
	cluster := newCluster()
	applyMonitoring(nil, cluster)
	if cluster.Spec.Monitoring != nil {
		t.Errorf("expected no change")
	}
}

func TestApplyMonitoring_DefaultsExporterImageWhenEmpty(t *testing.T) {
	cluster := newCluster()
	tmpl := &ackov1alpha1.AerospikeMonitoringSpec{
		Enabled: true,
		Port:    9145,
		// ExporterImage intentionally left empty
	}
	applyMonitoring(tmpl, cluster)
	if cluster.Spec.Monitoring.ExporterImage != ackov1alpha1.DefaultExporterImage {
		t.Errorf("expected ExporterImage to default to %q, got %q",
			ackov1alpha1.DefaultExporterImage, cluster.Spec.Monitoring.ExporterImage)
	}
}

func TestApplyMonitoring_DefaultsPortWhenZero(t *testing.T) {
	cluster := newCluster()
	tmpl := &ackov1alpha1.AerospikeMonitoringSpec{
		Enabled:       true,
		ExporterImage: "custom:latest",
		// Port intentionally left as zero
	}
	applyMonitoring(tmpl, cluster)
	if cluster.Spec.Monitoring.Port != ackov1alpha1.DefaultExporterPort {
		t.Errorf("expected Port to default to %d, got %d",
			ackov1alpha1.DefaultExporterPort, cluster.Spec.Monitoring.Port)
	}
}

func TestApplyMonitoring_NoDefaultsWhenDisabled(t *testing.T) {
	cluster := newCluster()
	tmpl := &ackov1alpha1.AerospikeMonitoringSpec{
		Enabled: false,
		// ExporterImage and Port left empty/zero
	}
	applyMonitoring(tmpl, cluster)
	if cluster.Spec.Monitoring.ExporterImage != "" {
		t.Errorf("expected ExporterImage to remain empty when Enabled=false, got %q",
			cluster.Spec.Monitoring.ExporterImage)
	}
	if cluster.Spec.Monitoring.Port != 0 {
		t.Errorf("expected Port to remain 0 when Enabled=false, got %d",
			cluster.Spec.Monitoring.Port)
	}
}

func TestApplyMonitoring_DefaultsDoNotOverwriteExistingValues(t *testing.T) {
	cluster := newCluster()
	cluster.Spec.Monitoring = &ackov1alpha1.AerospikeMonitoringSpec{
		Enabled:       true,
		ExporterImage: "custom-exporter:v2",
		Port:          9999,
	}
	tmpl := &ackov1alpha1.AerospikeMonitoringSpec{
		Enabled: true,
	}
	applyMonitoring(tmpl, cluster)
	if cluster.Spec.Monitoring.ExporterImage != "custom-exporter:v2" {
		t.Errorf("expected ExporterImage to be preserved, got %q",
			cluster.Spec.Monitoring.ExporterImage)
	}
	if cluster.Spec.Monitoring.Port != 9999 {
		t.Errorf("expected Port to be preserved (9999), got %d",
			cluster.Spec.Monitoring.Port)
	}
}

func TestApplyMonitoring_DeepCopied(t *testing.T) {
	cluster := newCluster()
	tmpl := &ackov1alpha1.AerospikeMonitoringSpec{Enabled: true, Port: 9145}
	applyMonitoring(tmpl, cluster)

	// Mutating the template should not affect the applied value.
	tmpl.Port = 9999
	if cluster.Spec.Monitoring.Port != 9145 {
		t.Errorf("expected deep copy: cluster port should remain 9145, got %d", cluster.Spec.Monitoring.Port)
	}
}

// --- applyNetworkPolicy ---

func TestApplyNetworkPolicy_AppliedWhenClusterPolicyNil(t *testing.T) {
	cluster := newCluster()
	tmpl := &ackov1alpha1.AerospikeNetworkPolicy{
		AccessType: ackov1alpha1.AerospikeNetworkTypePod,
	}
	applyNetworkPolicy(tmpl, cluster)
	if cluster.Spec.AerospikeNetworkPolicy == nil {
		t.Fatal("expected network policy to be applied")
	}
	if cluster.Spec.AerospikeNetworkPolicy.AccessType != ackov1alpha1.AerospikeNetworkTypePod {
		t.Errorf("expected AccessType=pod, got %q", cluster.Spec.AerospikeNetworkPolicy.AccessType)
	}
}

func TestApplyNetworkPolicy_NotOverriddenWhenClusterPolicySet(t *testing.T) {
	cluster := newCluster()
	cluster.Spec.AerospikeNetworkPolicy = &ackov1alpha1.AerospikeNetworkPolicy{
		AccessType: ackov1alpha1.AerospikeNetworkTypeHostInternal,
	}
	tmpl := &ackov1alpha1.AerospikeNetworkPolicy{
		AccessType: ackov1alpha1.AerospikeNetworkTypePod,
	}
	applyNetworkPolicy(tmpl, cluster)
	if cluster.Spec.AerospikeNetworkPolicy.AccessType != ackov1alpha1.AerospikeNetworkTypeHostInternal {
		t.Errorf("expected cluster policy to be preserved (hostInternal), got %q", cluster.Spec.AerospikeNetworkPolicy.AccessType)
	}
}

func TestApplyNetworkPolicy_NoopWhenTemplateNil(t *testing.T) {
	cluster := newCluster()
	applyNetworkPolicy(nil, cluster)
	if cluster.Spec.AerospikeNetworkPolicy != nil {
		t.Errorf("expected no change")
	}
}

func TestApplyNetworkPolicy_DeepCopied(t *testing.T) {
	cluster := newCluster()
	tmpl := &ackov1alpha1.AerospikeNetworkPolicy{AccessType: ackov1alpha1.AerospikeNetworkTypePod}
	applyNetworkPolicy(tmpl, cluster)

	// Mutating the template should not affect the applied value.
	tmpl.AccessType = ackov1alpha1.AerospikeNetworkTypeHostExternal
	if cluster.Spec.AerospikeNetworkPolicy.AccessType != ackov1alpha1.AerospikeNetworkTypePod {
		t.Errorf("expected deep copy: cluster access type should remain pod")
	}
}
