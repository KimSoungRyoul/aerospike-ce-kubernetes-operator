package podutil

import (
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "github.com/ksr/aerospike-ce-kubernetes-operator/api/v1alpha1"
	"github.com/ksr/aerospike-ce-kubernetes-operator/internal/utils"
)

const (
	hostnameTopologyKey   = "kubernetes.io/hostname"
	zoneTopologyKey       = "topology.kubernetes.io/zone"
	exporterContainerName = "aerospike-prometheus-exporter"
)

func boolPtr(b bool) *bool { return &b }

// --- shouldInjectAntiAffinity tests ---

func TestShouldInjectAntiAffinity_NilPodSpec(t *testing.T) {
	cluster := &v1alpha1.AerospikeCluster{
		Spec: v1alpha1.AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
		},
	}
	if shouldInjectAntiAffinity(cluster) {
		t.Error("should not inject anti-affinity when podSpec is nil")
	}
}

func TestShouldInjectAntiAffinity_MultiPodPerHostNil(t *testing.T) {
	cluster := &v1alpha1.AerospikeCluster{
		Spec: v1alpha1.AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			PodSpec: &v1alpha1.AerospikePodSpec{
				MultiPodPerHost: nil, // nil = not set
			},
		},
	}
	if shouldInjectAntiAffinity(cluster) {
		t.Error("should not inject anti-affinity when multiPodPerHost is nil")
	}
}

func TestShouldInjectAntiAffinity_MultiPodPerHostFalse(t *testing.T) {
	cluster := &v1alpha1.AerospikeCluster{
		Spec: v1alpha1.AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			PodSpec: &v1alpha1.AerospikePodSpec{
				MultiPodPerHost: boolPtr(false),
			},
		},
	}
	if !shouldInjectAntiAffinity(cluster) {
		t.Error("should inject anti-affinity when multiPodPerHost=false")
	}
}

func TestShouldInjectAntiAffinity_MultiPodPerHostTrue(t *testing.T) {
	cluster := &v1alpha1.AerospikeCluster{
		Spec: v1alpha1.AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			PodSpec: &v1alpha1.AerospikePodSpec{
				MultiPodPerHost: boolPtr(true),
			},
		},
	}
	if shouldInjectAntiAffinity(cluster) {
		t.Error("should not inject anti-affinity when multiPodPerHost=true")
	}
}

// --- injectPodAntiAffinity tests ---

func TestInjectPodAntiAffinity_CreatesAffinityFromNil(t *testing.T) {
	podSpec := &corev1.PodSpec{}
	injectPodAntiAffinity(podSpec, "my-cluster")

	if podSpec.Affinity == nil || podSpec.Affinity.PodAntiAffinity == nil {
		t.Fatal("expected affinity to be set")
	}

	required := podSpec.Affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution
	if len(required) != 1 {
		t.Fatalf("expected 1 anti-affinity term, got %d", len(required))
	}

	term := required[0]
	if term.TopologyKey != hostnameTopologyKey {
		t.Errorf("topologyKey = %q, want %q", term.TopologyKey, hostnameTopologyKey)
	}

	expectedLabels := utils.SelectorLabelsForCluster("my-cluster")
	if term.LabelSelector == nil {
		t.Fatal("expected labelSelector to be set")
	}
	for k, v := range expectedLabels {
		if term.LabelSelector.MatchLabels[k] != v {
			t.Errorf("label %q = %q, want %q", k, term.LabelSelector.MatchLabels[k], v)
		}
	}
}

func TestInjectPodAntiAffinity_PreservesExistingNodeAffinity(t *testing.T) {
	existingNodeAffinity := &corev1.NodeAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
			NodeSelectorTerms: []corev1.NodeSelectorTerm{
				{
					MatchExpressions: []corev1.NodeSelectorRequirement{
						{Key: zoneTopologyKey, Operator: corev1.NodeSelectorOpIn, Values: []string{"us-east-1a"}},
					},
				},
			},
		},
	}

	podSpec := &corev1.PodSpec{
		Affinity: &corev1.Affinity{
			NodeAffinity: existingNodeAffinity,
		},
	}

	injectPodAntiAffinity(podSpec, "my-cluster")

	// Node affinity should be preserved
	if podSpec.Affinity.NodeAffinity != existingNodeAffinity {
		t.Error("existing node affinity was overwritten")
	}

	// Pod anti-affinity should be added
	if podSpec.Affinity.PodAntiAffinity == nil {
		t.Fatal("expected pod anti-affinity to be set")
	}
	if len(podSpec.Affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution) != 1 {
		t.Error("expected 1 anti-affinity term")
	}
}

func TestInjectPodAntiAffinity_AppendsToExistingAntiAffinity(t *testing.T) {
	podSpec := &corev1.PodSpec{
		Affinity: &corev1.Affinity{
			PodAntiAffinity: &corev1.PodAntiAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
					{
						LabelSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"existing": "rule"},
						},
						TopologyKey: zoneTopologyKey,
					},
				},
			},
		},
	}

	injectPodAntiAffinity(podSpec, "my-cluster")

	required := podSpec.Affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution
	if len(required) != 2 {
		t.Fatalf("expected 2 anti-affinity terms, got %d", len(required))
	}

	// First should be the existing one
	if required[0].TopologyKey != zoneTopologyKey {
		t.Error("existing anti-affinity term was not preserved")
	}

	// Second should be the injected one
	if required[1].TopologyKey != hostnameTopologyKey {
		t.Error("injected anti-affinity term not found")
	}
}

// --- buildExporterSidecar tests ---

func TestBuildExporterSidecar_Basic(t *testing.T) {
	monitoring := &v1alpha1.AerospikeMonitoringSpec{
		Enabled:       true,
		ExporterImage: "aerospike/aerospike-prometheus-exporter:1.16.1",
		Port:          9145,
	}

	c := buildExporterSidecar(monitoring, nil)

	if c.Name != exporterContainerName {
		t.Errorf("name = %q, want %q", c.Name, exporterContainerName)
	}
	if c.Image != monitoring.ExporterImage {
		t.Errorf("image = %q, want %q", c.Image, monitoring.ExporterImage)
	}

	if len(c.Ports) != 1 || c.Ports[0].ContainerPort != 9145 {
		t.Errorf("expected port 9145, got %v", c.Ports)
	}

	envMap := make(map[string]string)
	for _, e := range c.Env {
		envMap[e.Name] = e.Value
	}
	if envMap["AS_HOST"] != "localhost" {
		t.Errorf("AS_HOST = %q, want %q", envMap["AS_HOST"], "localhost")
	}
	if envMap["AS_PORT"] != "3000" {
		t.Errorf("AS_PORT = %q, want %q", envMap["AS_PORT"], "3000")
	}
}

func TestBuildExporterSidecar_WithResources(t *testing.T) {
	monitoring := &v1alpha1.AerospikeMonitoringSpec{
		Enabled:       true,
		ExporterImage: "exporter:v1",
		Port:          9145,
		Resources: &corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("50m"),
			},
		},
	}

	c := buildExporterSidecar(monitoring, nil)

	if c.Resources.Requests.Cpu().String() != "50m" {
		t.Errorf("expected CPU request 50m, got %s", c.Resources.Requests.Cpu().String())
	}
}

func TestBuildExporterSidecar_CustomPort(t *testing.T) {
	monitoring := &v1alpha1.AerospikeMonitoringSpec{
		Enabled:       true,
		ExporterImage: "exporter:v1",
		Port:          9999,
	}

	c := buildExporterSidecar(monitoring, nil)

	if c.Ports[0].ContainerPort != 9999 {
		t.Errorf("expected port 9999, got %d", c.Ports[0].ContainerPort)
	}
}

func TestBuildExporterSidecar_HealthProbes(t *testing.T) {
	monitoring := &v1alpha1.AerospikeMonitoringSpec{
		Enabled:       true,
		ExporterImage: "exporter:v1",
		Port:          9145,
	}

	c := buildExporterSidecar(monitoring, nil)

	// Readiness probe
	if c.ReadinessProbe == nil {
		t.Fatal("expected readiness probe to be set")
	}
	if c.ReadinessProbe.HTTPGet == nil {
		t.Fatal("expected readiness probe to use HTTPGet")
	}
	if c.ReadinessProbe.HTTPGet.Path != "/metrics" {
		t.Errorf("readiness probe path = %q, want /metrics", c.ReadinessProbe.HTTPGet.Path)
	}
	if c.ReadinessProbe.HTTPGet.Port.IntVal != 9145 {
		t.Errorf("readiness probe port = %d, want 9145", c.ReadinessProbe.HTTPGet.Port.IntVal)
	}
	if c.ReadinessProbe.InitialDelaySeconds != 10 {
		t.Errorf("readiness initialDelay = %d, want 10", c.ReadinessProbe.InitialDelaySeconds)
	}

	// Liveness probe
	if c.LivenessProbe == nil {
		t.Fatal("expected liveness probe to be set")
	}
	if c.LivenessProbe.HTTPGet == nil {
		t.Fatal("expected liveness probe to use HTTPGet")
	}
	if c.LivenessProbe.InitialDelaySeconds != 30 {
		t.Errorf("liveness initialDelay = %d, want 30", c.LivenessProbe.InitialDelaySeconds)
	}
	if c.LivenessProbe.PeriodSeconds != 30 {
		t.Errorf("liveness period = %d, want 30", c.LivenessProbe.PeriodSeconds)
	}
}

func TestBuildExporterSidecar_ACLAuth(t *testing.T) {
	monitoring := &v1alpha1.AerospikeMonitoringSpec{
		Enabled:       true,
		ExporterImage: "exporter:v1",
		Port:          9145,
	}
	acl := &v1alpha1.AerospikeAccessControlSpec{
		Users: []v1alpha1.AerospikeUserSpec{
			{
				Name:       "admin",
				SecretName: "admin-secret",
				Roles:      []string{"sys-admin", "user-admin"},
			},
		},
	}

	c := buildExporterSidecar(monitoring, acl)

	envMap := make(map[string]string)
	var passwordRef *corev1.SecretKeySelector
	for _, e := range c.Env {
		if e.ValueFrom != nil && e.ValueFrom.SecretKeyRef != nil {
			passwordRef = e.ValueFrom.SecretKeyRef
		} else {
			envMap[e.Name] = e.Value
		}
	}

	if envMap["AS_AUTH_USER"] != "admin" {
		t.Errorf("AS_AUTH_USER = %q, want %q", envMap["AS_AUTH_USER"], "admin")
	}
	if envMap["AS_AUTH_MODE"] != "internal" {
		t.Errorf("AS_AUTH_MODE = %q, want %q", envMap["AS_AUTH_MODE"], "internal")
	}
	if passwordRef == nil {
		t.Fatal("expected AS_AUTH_PASSWORD to use SecretKeyRef")
	}
	if passwordRef.Name != "admin-secret" {
		t.Errorf("secret name = %q, want %q", passwordRef.Name, "admin-secret")
	}
	if passwordRef.Key != "password" {
		t.Errorf("secret key = %q, want %q", passwordRef.Key, "password")
	}
}

func TestBuildExporterSidecar_NoACLAuth(t *testing.T) {
	monitoring := &v1alpha1.AerospikeMonitoringSpec{
		Enabled:       true,
		ExporterImage: "exporter:v1",
		Port:          9145,
	}

	c := buildExporterSidecar(monitoring, nil)

	for _, e := range c.Env {
		if e.Name == "AS_AUTH_USER" || e.Name == "AS_AUTH_PASSWORD" || e.Name == "AS_AUTH_MODE" {
			t.Errorf("unexpected ACL env var %q when ACL is nil", e.Name)
		}
	}
}

func TestBuildExporterSidecar_MetricLabels(t *testing.T) {
	monitoring := &v1alpha1.AerospikeMonitoringSpec{
		Enabled:       true,
		ExporterImage: "exporter:v1",
		Port:          9145,
		MetricLabels: map[string]string{
			"env":  "prod",
			"team": "platform",
		},
	}

	c := buildExporterSidecar(monitoring, nil)

	var metricLabelsValue string
	for _, e := range c.Env {
		if e.Name == "METRIC_LABELS" {
			metricLabelsValue = e.Value
		}
	}

	// Keys should be sorted
	expected := "env=prod,team=platform"
	if metricLabelsValue != expected {
		t.Errorf("METRIC_LABELS = %q, want %q", metricLabelsValue, expected)
	}
}

func TestBuildExporterSidecar_CustomEnv(t *testing.T) {
	monitoring := &v1alpha1.AerospikeMonitoringSpec{
		Enabled:       true,
		ExporterImage: "exporter:v1",
		Port:          9145,
		Env: []corev1.EnvVar{
			{Name: "CUSTOM_VAR", Value: "custom_value"},
			{Name: "AS_HOST", Value: "override"}, // intentional override
		},
	}

	c := buildExporterSidecar(monitoring, nil)

	// User env vars should be appended last
	envMap := make(map[string]string)
	for _, e := range c.Env {
		envMap[e.Name] = e.Value // last value wins
	}

	if envMap["CUSTOM_VAR"] != "custom_value" {
		t.Errorf("CUSTOM_VAR = %q, want %q", envMap["CUSTOM_VAR"], "custom_value")
	}
	// User override should take effect (last wins)
	if envMap["AS_HOST"] != "override" {
		t.Errorf("AS_HOST = %q, want %q (user override)", envMap["AS_HOST"], "override")
	}
}

// --- BuildPodTemplateSpec integration tests ---

func TestBuildPodTemplateSpec_MonitoringSidecarInjected(t *testing.T) {
	cluster := &v1alpha1.AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
		Spec: v1alpha1.AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			Monitoring: &v1alpha1.AerospikeMonitoringSpec{
				Enabled:       true,
				ExporterImage: "exporter:v1",
				Port:          9145,
			},
		},
	}

	pt := BuildPodTemplateSpec(cluster, nil, 0, "test-config", "abc123")

	found := false
	for _, c := range pt.Spec.Containers {
		if c.Name == exporterContainerName {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected exporter sidecar container to be injected")
	}
}

func TestBuildPodTemplateSpec_NoMonitoringSidecarWhenDisabled(t *testing.T) {
	cluster := &v1alpha1.AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
		Spec: v1alpha1.AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
		},
	}

	pt := BuildPodTemplateSpec(cluster, nil, 0, "test-config", "abc123")

	for _, c := range pt.Spec.Containers {
		if c.Name == exporterContainerName {
			t.Error("exporter sidecar should not be injected when monitoring is not set")
		}
	}
}

func TestBuildPodTemplateSpec_BandwidthAnnotations(t *testing.T) {
	cluster := &v1alpha1.AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
		Spec: v1alpha1.AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			BandwidthConfig: &v1alpha1.BandwidthConfig{
				Ingress: "1Gbps",
				Egress:  "500Mbps",
			},
		},
	}

	pt := BuildPodTemplateSpec(cluster, nil, 0, "test-config", "abc123")

	annotations := pt.Annotations
	if annotations["kubernetes.io/ingress-bandwidth"] != "1Gbps" {
		t.Errorf("ingress-bandwidth = %q, want %q", annotations["kubernetes.io/ingress-bandwidth"], "1Gbps")
	}
	if annotations["kubernetes.io/egress-bandwidth"] != "500Mbps" {
		t.Errorf("egress-bandwidth = %q, want %q", annotations["kubernetes.io/egress-bandwidth"], "500Mbps")
	}
}

func TestBuildPodTemplateSpec_NoBandwidthAnnotationsWhenNotSet(t *testing.T) {
	cluster := &v1alpha1.AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
		Spec: v1alpha1.AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
		},
	}

	pt := BuildPodTemplateSpec(cluster, nil, 0, "test-config", "abc123")

	if _, ok := pt.Annotations["kubernetes.io/ingress-bandwidth"]; ok {
		t.Error("ingress-bandwidth annotation should not be present")
	}
	if _, ok := pt.Annotations["kubernetes.io/egress-bandwidth"]; ok {
		t.Error("egress-bandwidth annotation should not be present")
	}
}

func TestBuildPodTemplateSpec_AntiAffinityInjectedWhenMultiPodPerHostFalse(t *testing.T) {
	cluster := &v1alpha1.AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
		Spec: v1alpha1.AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			PodSpec: &v1alpha1.AerospikePodSpec{
				MultiPodPerHost: boolPtr(false),
			},
		},
	}

	pt := BuildPodTemplateSpec(cluster, nil, 0, "test-config", "abc123")

	affinity := pt.Spec.Affinity
	if affinity == nil || affinity.PodAntiAffinity == nil {
		t.Fatal("expected anti-affinity to be set")
	}
	required := affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution
	if len(required) == 0 {
		t.Fatal("expected at least 1 required anti-affinity term")
	}
	if required[0].TopologyKey != hostnameTopologyKey {
		t.Error("expected topologyKey kubernetes.io/hostname")
	}
}

func TestBuildPodTemplateSpec_NoAntiAffinityWhenMultiPodPerHostTrue(t *testing.T) {
	cluster := &v1alpha1.AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
		Spec: v1alpha1.AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			PodSpec: &v1alpha1.AerospikePodSpec{
				MultiPodPerHost: boolPtr(true),
			},
		},
	}

	pt := BuildPodTemplateSpec(cluster, nil, 0, "test-config", "abc123")

	if pt.Spec.Affinity != nil && pt.Spec.Affinity.PodAntiAffinity != nil {
		required := pt.Spec.Affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution
		for _, r := range required {
			if r.TopologyKey == hostnameTopologyKey {
				t.Error("should not inject anti-affinity when multiPodPerHost=true")
			}
		}
	}
}

// --- applyRackAffinity tests ---

func TestApplyRackAffinity_Zone(t *testing.T) {
	podSpec := &corev1.PodSpec{}
	rack := &v1alpha1.Rack{ID: 1, Zone: "us-east-1a"}

	applyRackAffinity(podSpec, rack)

	if podSpec.Affinity == nil || podSpec.Affinity.NodeAffinity == nil {
		t.Fatal("expected node affinity to be set")
	}
	terms := podSpec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms
	if len(terms) != 1 {
		t.Fatalf("expected 1 NodeSelectorTerm, got %d", len(terms))
	}
	exprs := terms[0].MatchExpressions
	if len(exprs) != 1 {
		t.Fatalf("expected 1 MatchExpression, got %d", len(exprs))
	}
	if exprs[0].Key != zoneTopologyKey {
		t.Errorf("key = %q, want %q", exprs[0].Key, zoneTopologyKey)
	}
	if exprs[0].Operator != corev1.NodeSelectorOpIn {
		t.Errorf("operator = %q, want %q", exprs[0].Operator, corev1.NodeSelectorOpIn)
	}
	if len(exprs[0].Values) != 1 || exprs[0].Values[0] != "us-east-1a" {
		t.Errorf("values = %v, want [us-east-1a]", exprs[0].Values)
	}
}

func TestApplyRackAffinity_RegionAndNodeName(t *testing.T) {
	podSpec := &corev1.PodSpec{}
	rack := &v1alpha1.Rack{ID: 1, Region: "us-east-1", NodeName: "node-1"}

	applyRackAffinity(podSpec, rack)

	terms := podSpec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms
	exprs := terms[0].MatchExpressions
	if len(exprs) != 2 {
		t.Fatalf("expected 2 MatchExpressions, got %d", len(exprs))
	}
	keys := make(map[string]bool)
	for _, e := range exprs {
		keys[e.Key] = true
	}
	if !keys["topology.kubernetes.io/region"] {
		t.Error("expected region affinity")
	}
	if !keys["kubernetes.io/hostname"] {
		t.Error("expected hostname affinity")
	}
}

func TestApplyRackAffinity_RackLabel(t *testing.T) {
	podSpec := &corev1.PodSpec{}
	rack := &v1alpha1.Rack{ID: 1, RackLabel: "rack-a"}

	applyRackAffinity(podSpec, rack)

	exprs := podSpec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions
	if len(exprs) != 1 || exprs[0].Key != "acko.io/rack" {
		t.Errorf("expected acko.io/rack affinity, got key=%q", exprs[0].Key)
	}
	if exprs[0].Values[0] != "rack-a" {
		t.Errorf("values = %v, want [rack-a]", exprs[0].Values)
	}
}

func TestApplyRackAffinity_NoFieldsSet(t *testing.T) {
	podSpec := &corev1.PodSpec{}
	rack := &v1alpha1.Rack{ID: 1}

	applyRackAffinity(podSpec, rack)

	if podSpec.Affinity != nil {
		t.Error("expected affinity to remain nil when no rack fields are set")
	}
}

func TestApplyRackAffinity_AllFields(t *testing.T) {
	podSpec := &corev1.PodSpec{}
	rack := &v1alpha1.Rack{
		ID:        1,
		Zone:      "us-east-1a",
		Region:    "us-east-1",
		NodeName:  "node-1",
		RackLabel: "rack-a",
	}

	applyRackAffinity(podSpec, rack)

	exprs := podSpec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions
	if len(exprs) != 4 {
		t.Fatalf("expected 4 MatchExpressions, got %d", len(exprs))
	}
}

func TestApplyRackAffinity_PreservesExistingPodAntiAffinity(t *testing.T) {
	podSpec := &corev1.PodSpec{
		Affinity: &corev1.Affinity{
			PodAntiAffinity: &corev1.PodAntiAffinity{},
		},
	}
	rack := &v1alpha1.Rack{ID: 1, Zone: "us-west-2a"}

	applyRackAffinity(podSpec, rack)

	if podSpec.Affinity.PodAntiAffinity == nil {
		t.Error("existing PodAntiAffinity was overwritten")
	}
	if podSpec.Affinity.NodeAffinity == nil {
		t.Fatal("expected NodeAffinity to be set")
	}
}

// --- applyRackPodSpecOverrides tests ---

func TestApplyRackPodSpecOverrides_Affinity(t *testing.T) {
	podSpec := &corev1.PodSpec{
		Affinity: &corev1.Affinity{
			NodeAffinity: &corev1.NodeAffinity{},
		},
	}
	rackAffinity := &corev1.Affinity{
		PodAntiAffinity: &corev1.PodAntiAffinity{},
	}
	rackPod := &v1alpha1.RackPodSpec{Affinity: rackAffinity}

	applyRackPodSpecOverrides(podSpec, rackPod)

	if podSpec.Affinity != rackAffinity {
		t.Error("expected affinity to be overridden by rack")
	}
}

func TestApplyRackPodSpecOverrides_Tolerations(t *testing.T) {
	podSpec := &corev1.PodSpec{
		Tolerations: []corev1.Toleration{{Key: "original"}},
	}
	rackPod := &v1alpha1.RackPodSpec{
		Tolerations: []corev1.Toleration{{Key: "rack-taint", Operator: corev1.TolerationOpExists}},
	}

	applyRackPodSpecOverrides(podSpec, rackPod)

	if len(podSpec.Tolerations) != 1 || podSpec.Tolerations[0].Key != "rack-taint" {
		t.Error("expected tolerations to be replaced by rack tolerations")
	}
}

func TestApplyRackPodSpecOverrides_NodeSelector(t *testing.T) {
	podSpec := &corev1.PodSpec{
		NodeSelector: map[string]string{"cluster": "default"},
	}
	rackPod := &v1alpha1.RackPodSpec{
		NodeSelector: map[string]string{"zone": "us-east-1a"},
	}

	applyRackPodSpecOverrides(podSpec, rackPod)

	if len(podSpec.NodeSelector) != 1 || podSpec.NodeSelector["zone"] != "us-east-1a" {
		t.Error("expected nodeSelector to be replaced by rack nodeSelector")
	}
}

func TestApplyRackPodSpecOverrides_NilFields(t *testing.T) {
	origAffinity := &corev1.Affinity{NodeAffinity: &corev1.NodeAffinity{}}
	podSpec := &corev1.PodSpec{
		Affinity:     origAffinity,
		Tolerations:  []corev1.Toleration{{Key: "keep"}},
		NodeSelector: map[string]string{"keep": "true"},
	}
	rackPod := &v1alpha1.RackPodSpec{} // all nil/empty

	applyRackPodSpecOverrides(podSpec, rackPod)

	if podSpec.Affinity != origAffinity {
		t.Error("affinity should not change when rack affinity is nil")
	}
	if len(podSpec.Tolerations) != 1 || podSpec.Tolerations[0].Key != "keep" {
		t.Error("tolerations should not change when rack tolerations is empty")
	}
	if podSpec.NodeSelector["keep"] != "true" {
		t.Error("nodeSelector should not change when rack nodeSelector is empty")
	}
}

// --- PodNameForIndex tests ---

func TestPodNameForIndex(t *testing.T) {
	tests := []struct {
		stsName  string
		index    int
		expected string
	}{
		{"my-cluster-1", 0, "my-cluster-1-0"},
		{"my-cluster-1", 3, "my-cluster-1-3"},
		{"aero-2", 99, "aero-2-99"},
	}
	for _, tc := range tests {
		if got := PodNameForIndex(tc.stsName, tc.index); got != tc.expected {
			t.Errorf("PodNameForIndex(%q, %d) = %q, want %q", tc.stsName, tc.index, got, tc.expected)
		}
	}
}

// --- TerminationGracePeriodSeconds tests ---

func TestBuildPodTemplateSpec_DefaultTerminationGracePeriod(t *testing.T) {
	cluster := &v1alpha1.AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
		Spec: v1alpha1.AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
		},
	}

	pt := BuildPodTemplateSpec(cluster, nil, 0, "test-config", "abc123")

	if pt.Spec.TerminationGracePeriodSeconds == nil {
		t.Fatal("expected terminationGracePeriodSeconds to be set")
	}
	if *pt.Spec.TerminationGracePeriodSeconds != 60 {
		t.Errorf("terminationGracePeriodSeconds = %d, want 60", *pt.Spec.TerminationGracePeriodSeconds)
	}
}

func TestBuildPodTemplateSpec_CustomTerminationGracePeriod(t *testing.T) {
	customGrace := int64(120)
	cluster := &v1alpha1.AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
		Spec: v1alpha1.AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			PodSpec: &v1alpha1.AerospikePodSpec{
				TerminationGracePeriodSeconds: &customGrace,
			},
		},
	}

	pt := BuildPodTemplateSpec(cluster, nil, 0, "test-config", "abc123")

	if pt.Spec.TerminationGracePeriodSeconds == nil {
		t.Fatal("expected terminationGracePeriodSeconds to be set")
	}
	if *pt.Spec.TerminationGracePeriodSeconds != 120 {
		t.Errorf("terminationGracePeriodSeconds = %d, want 120", *pt.Spec.TerminationGracePeriodSeconds)
	}
}

// --- PreStop lifecycle hook integration test ---

func TestBuildPodTemplateSpec_AerospikeContainerHasPreStopHook(t *testing.T) {
	cluster := &v1alpha1.AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
		Spec: v1alpha1.AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
		},
	}

	pt := BuildPodTemplateSpec(cluster, nil, 0, "test-config", "abc123")

	var aerospikeContainer *corev1.Container
	for i := range pt.Spec.Containers {
		if pt.Spec.Containers[i].Name == AerospikeContainerName {
			aerospikeContainer = &pt.Spec.Containers[i]
			break
		}
	}
	if aerospikeContainer == nil {
		t.Fatal("aerospike container not found")
	}
	if aerospikeContainer.Lifecycle == nil || aerospikeContainer.Lifecycle.PreStop == nil {
		t.Fatal("expected preStop hook on aerospike container")
	}
	if aerospikeContainer.Lifecycle.PreStop.Exec == nil {
		t.Fatal("expected preStop to use exec handler")
	}
	cmd := aerospikeContainer.Lifecycle.PreStop.Exec.Command
	if len(cmd) != 2 {
		t.Fatalf("expected 2 command parts, got %d: %v", len(cmd), cmd)
	}
	if cmd[0] != "sleep" {
		t.Errorf("expected sleep, got %q", cmd[0])
	}
	expectedArg := fmt.Sprintf("%d", PreStopSleepSeconds)
	if cmd[1] != expectedArg {
		t.Errorf("sleep arg = %q, want %q", cmd[1], expectedArg)
	}
}

func TestBuildPodTemplateSpec_PriorityClassName(t *testing.T) {
	cluster := &v1alpha1.AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
		Spec: v1alpha1.AerospikeClusterSpec{
			Size:  1,
			Image: "aerospike:ce-8.1.1.1",
			PodSpec: &v1alpha1.AerospikePodSpec{
				PriorityClassName: "high-priority",
			},
		},
	}

	pt := BuildPodTemplateSpec(cluster, nil, 0, "test-config", "abc123")

	if pt.Spec.PriorityClassName != "high-priority" {
		t.Errorf("PriorityClassName = %q, want %q", pt.Spec.PriorityClassName, "high-priority")
	}
}

func TestBuildPodTemplateSpec_PriorityClassNameEmpty(t *testing.T) {
	cluster := &v1alpha1.AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
		Spec: v1alpha1.AerospikeClusterSpec{
			Size:  1,
			Image: "aerospike:ce-8.1.1.1",
		},
	}

	pt := BuildPodTemplateSpec(cluster, nil, 0, "test-config", "abc123")

	if pt.Spec.PriorityClassName != "" {
		t.Errorf("PriorityClassName = %q, want empty", pt.Spec.PriorityClassName)
	}
}

func TestBuildPodTemplateSpec_TopologySpreadConstraints(t *testing.T) {
	cluster := &v1alpha1.AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
		Spec: v1alpha1.AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			PodSpec: &v1alpha1.AerospikePodSpec{
				TopologySpreadConstraints: []corev1.TopologySpreadConstraint{
					{
						MaxSkew:           1,
						TopologyKey:       zoneTopologyKey,
						WhenUnsatisfiable: corev1.DoNotSchedule,
					},
				},
			},
		},
	}

	pt := BuildPodTemplateSpec(cluster, nil, 0, "test-config", "abc123")

	if len(pt.Spec.TopologySpreadConstraints) != 1 {
		t.Fatalf("TopologySpreadConstraints length = %d, want 1", len(pt.Spec.TopologySpreadConstraints))
	}
	if pt.Spec.TopologySpreadConstraints[0].TopologyKey != zoneTopologyKey {
		t.Errorf("TopologyKey = %q, want %q", pt.Spec.TopologySpreadConstraints[0].TopologyKey, zoneTopologyKey)
	}
}

func TestBuildPodTemplateSpec_InitContainerUsesClusterImage(t *testing.T) {
	cluster := &v1alpha1.AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
		Spec: v1alpha1.AerospikeClusterSpec{
			Size:  1,
			Image: "aerospike:ce-8.0.0.0",
		},
	}

	pt := BuildPodTemplateSpec(cluster, nil, 0, "test-config", "abc123")

	if len(pt.Spec.InitContainers) == 0 {
		t.Fatal("expected at least 1 init container")
	}
	initC := pt.Spec.InitContainers[0]
	if initC.Image != "aerospike:ce-8.0.0.0" {
		t.Errorf("init container image = %q, want %q", initC.Image, "aerospike:ce-8.0.0.0")
	}
	if initC.Name != InitContainerName {
		t.Errorf("init container name = %q, want %q", initC.Name, InitContainerName)
	}
}
