package v1alpha1

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func boolPtr(b bool) *bool { return &b }

// --- Defaulter tests ---

func TestDefaultMonitoring_SetsDefaults(t *testing.T) {
	d := &AerospikeClusterDefaulter{}
	cluster := &AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			Monitoring: &AerospikeMonitoringSpec{
				Enabled: true,
				ServiceMonitor: &ServiceMonitorSpec{
					Enabled: true,
				},
			},
		},
	}

	if err := d.Default(context.Background(), cluster); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := cluster.Spec.Monitoring
	if m.ExporterImage != DefaultExporterImage {
		t.Errorf("ExporterImage = %q, want %q", m.ExporterImage, DefaultExporterImage)
	}
	if m.Port != DefaultExporterPort {
		t.Errorf("Port = %d, want %d", m.Port, DefaultExporterPort)
	}
	if m.ServiceMonitor.Interval != defaultScrapeInterval {
		t.Errorf("Interval = %q, want %q", m.ServiceMonitor.Interval, defaultScrapeInterval)
	}
}

func TestDefaultMonitoring_NoopWhenDisabled(t *testing.T) {
	d := &AerospikeClusterDefaulter{}
	cluster := &AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
		},
	}

	if err := d.Default(context.Background(), cluster); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cluster.Spec.Monitoring != nil {
		t.Error("monitoring should remain nil when not set")
	}
}

func TestDefaultMonitoring_PreservesCustomValues(t *testing.T) {
	d := &AerospikeClusterDefaulter{}
	cluster := &AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			Monitoring: &AerospikeMonitoringSpec{
				Enabled:       true,
				ExporterImage: "custom-exporter:v2",
				Port:          9999,
			},
		},
	}

	if err := d.Default(context.Background(), cluster); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cluster.Spec.Monitoring.ExporterImage != "custom-exporter:v2" {
		t.Errorf("custom ExporterImage was overwritten: %q", cluster.Spec.Monitoring.ExporterImage)
	}
	if cluster.Spec.Monitoring.Port != 9999 {
		t.Errorf("custom Port was overwritten: %d", cluster.Spec.Monitoring.Port)
	}
}

// --- HostNetwork defaulting tests ---

func TestDefaultHostNetwork_SetsMultiPodPerHostFalse(t *testing.T) {
	d := &AerospikeClusterDefaulter{}
	cluster := &AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			PodSpec: &AerospikePodSpec{
				HostNetwork: true,
			},
		},
	}

	if err := d.Default(context.Background(), cluster); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cluster.Spec.PodSpec.MultiPodPerHost == nil {
		t.Fatal("multiPodPerHost should be set")
	}
	if *cluster.Spec.PodSpec.MultiPodPerHost {
		t.Error("multiPodPerHost should default to false for hostNetwork=true")
	}
}

func TestDefaultHostNetwork_SetsDNSPolicy(t *testing.T) {
	d := &AerospikeClusterDefaulter{}
	cluster := &AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			PodSpec: &AerospikePodSpec{
				HostNetwork: true,
			},
		},
	}

	if err := d.Default(context.Background(), cluster); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cluster.Spec.PodSpec.DNSPolicy != corev1.DNSClusterFirstWithHostNet {
		t.Errorf("dnsPolicy = %q, want %q", cluster.Spec.PodSpec.DNSPolicy, corev1.DNSClusterFirstWithHostNet)
	}
}

func TestDefaultHostNetwork_PreservesExplicitMultiPodPerHost(t *testing.T) {
	d := &AerospikeClusterDefaulter{}
	cluster := &AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			PodSpec: &AerospikePodSpec{
				HostNetwork:     true,
				MultiPodPerHost: boolPtr(true),
			},
		},
	}

	if err := d.Default(context.Background(), cluster); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !*cluster.Spec.PodSpec.MultiPodPerHost {
		t.Error("should not override explicitly set multiPodPerHost=true")
	}
}

func TestDefaultHostNetwork_NoopWhenHostNetworkFalse(t *testing.T) {
	d := &AerospikeClusterDefaulter{}
	cluster := &AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			PodSpec: &AerospikePodSpec{
				HostNetwork: false,
			},
		},
	}

	if err := d.Default(context.Background(), cluster); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cluster.Spec.PodSpec.MultiPodPerHost != nil {
		t.Error("multiPodPerHost should remain nil when hostNetwork=false")
	}
	if cluster.Spec.PodSpec.DNSPolicy != "" {
		t.Errorf("dnsPolicy should remain empty when hostNetwork=false, got %q", cluster.Spec.PodSpec.DNSPolicy)
	}
}

func TestDefaultHostNetwork_PreservesExplicitDNSPolicy(t *testing.T) {
	d := &AerospikeClusterDefaulter{}
	cluster := &AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			PodSpec: &AerospikePodSpec{
				HostNetwork: true,
				DNSPolicy:   corev1.DNSDefault,
			},
		},
	}

	if err := d.Default(context.Background(), cluster); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cluster.Spec.PodSpec.DNSPolicy != corev1.DNSDefault {
		t.Errorf("should not override explicitly set dnsPolicy, got %q", cluster.Spec.PodSpec.DNSPolicy)
	}
}

// --- Validator tests ---

func TestValidate_HostNetworkMultiPodPerHostWarning(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			PodSpec: &AerospikePodSpec{
				HostNetwork:     true,
				MultiPodPerHost: boolPtr(true),
			},
		},
	}

	warnings, err := v.validate(cluster)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := slices.Contains(warnings, "hostNetwork=true with multiPodPerHost=true may cause port conflicts")
	if !found {
		t.Error("expected warning about hostNetwork+multiPodPerHost, got none")
	}
}

func TestValidate_HostNetworkDNSPolicyWarning(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			PodSpec: &AerospikePodSpec{
				HostNetwork: true,
				DNSPolicy:   corev1.DNSDefault,
			},
		},
	}

	warnings, err := v.validate(cluster)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := slices.Contains(warnings, "hostNetwork=true with dnsPolicy other than ClusterFirstWithHostNet may cause DNS resolution issues")
	if !found {
		t.Errorf("expected DNS warning, got warnings: %v", warnings)
	}
}

func TestValidate_NoWarningsForValidHostNetwork(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			PodSpec: &AerospikePodSpec{
				HostNetwork:     true,
				MultiPodPerHost: boolPtr(false),
				DNSPolicy:       corev1.DNSClusterFirstWithHostNet,
			},
		},
	}

	warnings, err := v.validate(cluster)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(warnings) != 0 {
		t.Errorf("expected no warnings, got %v", warnings)
	}
}

func TestValidate_NoHostNetworkNoWarnings(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
		},
	}

	warnings, err := v.validate(cluster)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(warnings) != 0 {
		t.Errorf("expected no warnings, got %v", warnings)
	}
}

// --- Existing validation tests (ensure no regressions) ---

func TestValidate_CEClusterSizeLimit(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  9,
			Image: "aerospike:ce-8.1.1.1",
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Error("expected error for size > 8")
	}
}

func TestValidate_EnterpriseImageRejected(t *testing.T) {
	v := &AerospikeClusterValidator{}

	tests := []struct {
		image string
	}{
		{"aerospike:ee-8.0.0.1_1"},
		{"aerospike-enterprise:8.0.0"},
		// Enterprise image pulled through a registry that includes a port: the
		// host:port colon must not be mistaken for the tag separator, otherwise
		// the ee-/ent- tag is not detected and the image bypasses the CE guard.
		{"myregistry.io:5000/aerospike:ee-8.0.0.1_1"},
		{"localhost:32000/aerospike:ent-8.1.0"},
	}

	for _, tc := range tests {
		cluster := &AerospikeCluster{
			Spec: AerospikeClusterSpec{
				Size:  3,
				Image: tc.image,
			},
		}
		_, err := v.validate(cluster)
		if err == nil {
			t.Errorf("expected error for enterprise image %q", tc.image)
		}
	}
}

func TestValidate_XDRRejected(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeConfig: &AerospikeConfigSpec{
				Value: map[string]any{
					"xdr": map[string]any{},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Error("expected error for XDR section")
	}
}

func TestValidate_TLSRejected(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeConfig: &AerospikeConfigSpec{
				Value: map[string]any{
					"tls": map[string]any{},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Error("expected error for TLS section")
	}
}

func TestValidate_MaxNamespaces(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeConfig: &AerospikeConfigSpec{
				Value: map[string]any{
					"namespaces": []any{
						map[string]any{"name": "ns1"},
						map[string]any{"name": "ns2"},
						map[string]any{"name": "ns3"},
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Error("expected error for > 2 namespaces")
	}
}

func TestValidate_DuplicateNamespaceNamesRejected(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeConfig: &AerospikeConfigSpec{
				Value: map[string]any{
					"namespaces": []any{
						map[string]any{"name": "test"},
						map[string]any{"name": "test"},
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Fatal("expected error for duplicate namespace names")
	}
	if !strings.Contains(err.Error(), "duplicate namespace name") {
		t.Errorf("expected duplicate namespace name error, got: %v", err)
	}
}

func TestValidate_DistinctNamespaceNamesAccepted(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeConfig: &AerospikeConfigSpec{
				Value: map[string]any{
					"namespaces": []any{
						map[string]any{"name": "ns1"},
						map[string]any{"name": "ns2"},
					},
				},
			},
		},
	}

	if _, err := v.validate(cluster); err != nil {
		t.Errorf("expected no error for distinct namespace names, got: %v", err)
	}
}

func TestValidate_DuplicateRackIDs(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  4,
			Image: "aerospike:ce-8.1.1.1",
			RackConfig: &RackConfig{
				Racks: []Rack{
					{ID: 1},
					{ID: 1},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Error("expected error for duplicate rack IDs")
	}
}

// --- Enterprise-only namespace config validation tests ---

func TestValidate_EnterpriseOnlyNamespaceKeys(t *testing.T) {
	v := &AerospikeClusterValidator{}

	enterpriseKeys := []string{
		"compression", "compression-level", "durable-delete", "fast-restart",
		"index-type", "sindex-type", "rack-id", "strong-consistency",
		"tomb-raider-eligible-age", "tomb-raider-period",
	}

	for _, key := range enterpriseKeys {
		cluster := &AerospikeCluster{
			Spec: AerospikeClusterSpec{
				Size:  3,
				Image: "aerospike:ce-8.1.1.1",
				AerospikeConfig: &AerospikeConfigSpec{
					Value: map[string]any{
						"namespaces": []any{
							map[string]any{
								"name": "test",
								key:    "some-value",
							},
						},
					},
				},
			},
		}

		_, err := v.validate(cluster)
		if err == nil {
			t.Errorf("expected error for enterprise-only namespace key %q", key)
		}
	}
}

func TestValidate_HeartbeatModeMulticastRejected(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeConfig: &AerospikeConfigSpec{
				Value: map[string]any{
					"network": map[string]any{
						"heartbeat": map[string]any{
							"mode": "multicast",
							"port": 3002,
						},
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Error("expected error for heartbeat mode=multicast (CE only supports mesh)")
	}
}

func TestValidate_HeartbeatModeMeshAccepted(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeConfig: &AerospikeConfigSpec{
				Value: map[string]any{
					"network": map[string]any{
						"heartbeat": map[string]any{
							"mode": "mesh",
							"port": 3002,
						},
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err != nil {
		t.Errorf("unexpected error for heartbeat mode=mesh: %v", err)
	}
}

func TestValidate_DataInMemoryWarning(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeConfig: &AerospikeConfigSpec{
				Value: map[string]any{
					"namespaces": []any{
						map[string]any{
							"name": "myns",
							"storage-engine": map[string]any{
								"type":           "device",
								"data-in-memory": true,
								"file":           "/data/myns.dat",
								"filesize":       "4G",
							},
						},
					},
				},
			},
		},
	}

	warnings, err := v.validate(cluster)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, w := range warnings {
		if strings.Contains(w, "data-in-memory=true") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected warning about data-in-memory, got warnings: %v", warnings)
	}
}

func TestValidate_ReplicationFactorOutOfRange(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeConfig: &AerospikeConfigSpec{
				Value: map[string]any{
					"namespaces": []any{
						map[string]any{
							"name":               "test",
							"replication-factor": 5,
						},
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Error("expected error for replication-factor > 4")
	}
}

func TestValidate_ValidNamespaceConfigAccepted(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeConfig: &AerospikeConfigSpec{
				Value: map[string]any{
					"namespaces": []any{
						map[string]any{
							"name":               "test",
							"replication-factor": 2,
							"memory-size":        "4G",
							"storage-engine": map[string]any{
								"type": "memory",
							},
						},
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err != nil {
		t.Errorf("unexpected error for valid namespace config: %v", err)
	}
}

// --- Security section validation tests ---

// TestValidate_SecuritySectionAllowed verifies that the security stanza is allowed
// in aerospikeConfig. CE 6.0+ supports enable-security and default-password-file.
func TestValidate_SecuritySectionAllowed(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeConfig: &AerospikeConfigSpec{
				Value: map[string]any{
					"security": map[string]any{},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err != nil {
		t.Errorf("expected security section to be allowed in CE, got: %v", err)
	}
}

// TestValidate_SecurityEnableSecurityAllowed verifies that enable-security is
// a valid CE security setting.
func TestValidate_SecurityEnableSecurityAllowed(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeConfig: &AerospikeConfigSpec{
				Value: map[string]any{
					"security": map[string]any{
						"enable-security": true,
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err != nil {
		t.Errorf("expected security.enable-security to be allowed in CE, got: %v", err)
	}
}

// TestValidate_SecurityDefaultPasswordFileAllowed verifies that default-password-file
// is a valid CE security setting.
func TestValidate_SecurityDefaultPasswordFileAllowed(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeConfig: &AerospikeConfigSpec{
				Value: map[string]any{
					"security": map[string]any{
						"enable-security":       true,
						"default-password-file": "/etc/aerospike/secret/password.txt",
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err != nil {
		t.Errorf("expected security.default-password-file to be allowed in CE, got: %v", err)
	}
}

// TestValidate_SecurityEnterpriseOnlyKeysRejected verifies that enterprise-only
// security sub-keys (tls, ldap, log, syslog) are rejected.
func TestValidate_SecurityEnterpriseOnlyKeysRejected(t *testing.T) {
	v := &AerospikeClusterValidator{}

	enterpriseKeys := []struct {
		key  string
		desc string
	}{
		{"tls", "TLS within security"},
		{"ldap", "LDAP within security"},
		{"log", "security audit log"},
		{"syslog", "security syslog sink"},
	}

	for _, tc := range enterpriseKeys {
		t.Run(tc.key, func(t *testing.T) {
			cluster := &AerospikeCluster{
				Spec: AerospikeClusterSpec{
					Size:  3,
					Image: "aerospike:ce-8.1.1.1",
					AerospikeConfig: &AerospikeConfigSpec{
						Value: map[string]any{
							"security": map[string]any{
								tc.key: map[string]any{},
							},
						},
					},
				},
			}

			_, err := v.validate(cluster)
			if err == nil {
				t.Errorf("expected error for enterprise-only security.%s, but got none", tc.key)
			}
			expected := fmt.Sprintf("security.%s is not allowed in CE", tc.key)
			if err != nil && !strings.Contains(err.Error(), expected) {
				t.Errorf("expected error to contain %q, got: %v", expected, err)
			}
		})
	}
}

// TestValidate_SecurityMixedCEAndEnterpriseKeys verifies that a security stanza
// with both CE-allowed and enterprise-only keys is rejected.
func TestValidate_SecurityMixedCEAndEnterpriseKeys(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeConfig: &AerospikeConfigSpec{
				Value: map[string]any{
					"security": map[string]any{
						"enable-security": true,
						"ldap":            map[string]any{"query-base-dn": "dc=example,dc=com"},
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Error("expected error for enterprise-only security.ldap mixed with CE settings")
	}
}

// --- ACL validation tests ---

func TestValidate_ACLWithAdminUser(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeAccessControl: &AerospikeAccessControlSpec{
				Users: []AerospikeUserSpec{
					{
						Name:       "admin",
						SecretName: "admin-secret",
						Roles:      []string{"sys-admin", "user-admin"},
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err != nil {
		t.Errorf("unexpected error for valid ACL config: %v", err)
	}
}

func TestValidate_ACLWithoutAdminUser(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeAccessControl: &AerospikeAccessControlSpec{
				Users: []AerospikeUserSpec{
					{
						Name:       "reader",
						SecretName: "reader-secret",
						Roles:      []string{"read"},
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Error("expected error for ACL without admin user")
	}
	if !strings.Contains(err.Error(), "sys-admin") {
		t.Errorf("error should mention 'sys-admin', got: %v", err)
	}
}

func TestValidate_ACLWithSplitAdminRoles(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeAccessControl: &AerospikeAccessControlSpec{
				Users: []AerospikeUserSpec{
					{
						Name:       "sysadmin",
						SecretName: "sa-secret",
						Roles:      []string{"sys-admin"},
					},
					{
						Name:       "useradmin",
						SecretName: "ua-secret",
						Roles:      []string{"user-admin"},
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Error("expected error when sys-admin and user-admin are on different users")
	}
}

// --- RollingUpdateBatchSize validation tests ---

func int32Ptr(i int32) *int32 { return &i }

func TestValidate_RollingUpdateBatchSizeGreaterThanSize(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:                   3,
			Image:                  "aerospike:ce-8.1.1.1",
			RollingUpdateBatchSize: int32Ptr(5),
		},
	}

	warnings, err := v.validate(cluster)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, w := range warnings {
		if strings.Contains(w, "rollingUpdateBatchSize") && strings.Contains(w, "greater than cluster size") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected warning about batchSize > clusterSize, got warnings: %v", warnings)
	}
}

func TestValidate_RollingUpdateBatchSizeEqualToSize(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:                   3,
			Image:                  "aerospike:ce-8.1.1.1",
			RollingUpdateBatchSize: int32Ptr(3),
		},
	}

	warnings, err := v.validate(cluster)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// batchSize == clusterSize should not produce a warning
	for _, w := range warnings {
		if strings.Contains(w, "rollingUpdateBatchSize") {
			t.Errorf("unexpected batchSize warning: %v", w)
		}
	}
}

func TestValidate_RollingUpdateBatchSizeLessThanSize(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:                   4,
			Image:                  "aerospike:ce-8.1.1.1",
			RollingUpdateBatchSize: int32Ptr(2),
		},
	}

	warnings, err := v.validate(cluster)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, w := range warnings {
		if strings.Contains(w, "rollingUpdateBatchSize") {
			t.Errorf("unexpected batchSize warning: %v", w)
		}
	}
}

func TestValidate_RollingUpdateBatchSizeNil(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
		},
	}

	warnings, err := v.validate(cluster)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, w := range warnings {
		if strings.Contains(w, "rollingUpdateBatchSize") {
			t.Errorf("unexpected batchSize warning: %v", w)
		}
	}
}

// TestValidate_RollingUpdateBatchSizeDeferredToTemplate confirms that the
// batchSize > size warning is suppressed when spec.size is 0 and a templateRef
// is set. Without the guard every templateRef-backed cluster with
// rollingUpdateBatchSize>0 produces a spurious "all pods may restart
// simultaneously" warning because spec.Size is 0 at admission time.
func TestValidate_RollingUpdateBatchSizeDeferredToTemplate(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:                   0,
			TemplateRef:            &TemplateRef{Name: "ce-template"},
			RollingUpdateBatchSize: int32Ptr(2),
		},
	}

	warnings, err := v.validate(cluster)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, w := range warnings {
		if strings.Contains(w, "rollingUpdateBatchSize") {
			t.Errorf("batchSize warning should be deferred to template resolution, got: %v", w)
		}
	}
}

// --- Defaulter core behavior tests ---

func TestDefault_SetsClusterName(t *testing.T) {
	d := &AerospikeClusterDefaulter{}
	cluster := &AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "my-cluster", Namespace: "default"},
		Spec: AerospikeClusterSpec{
			Size:  1,
			Image: "aerospike:ce-8.1.1.1",
		},
	}

	if err := d.Default(context.Background(), cluster); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	svc := cluster.Spec.AerospikeConfig.Value["service"].(map[string]any)
	if svc["cluster-name"] != "my-cluster" {
		t.Errorf("cluster-name = %v, want 'my-cluster'", svc["cluster-name"])
	}
	if svc["proto-fd-max"] != defaultProtoFdMax {
		t.Errorf("proto-fd-max = %v, want %d", svc["proto-fd-max"], defaultProtoFdMax)
	}
}

func TestDefault_SetsNetworkPorts(t *testing.T) {
	d := &AerospikeClusterDefaulter{}
	cluster := &AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: AerospikeClusterSpec{
			Size:  1,
			Image: "aerospike:ce-8.1.1.1",
		},
	}

	if err := d.Default(context.Background(), cluster); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	net := cluster.Spec.AerospikeConfig.Value["network"].(map[string]any)
	svcNet := net["service"].(map[string]any)
	hb := net["heartbeat"].(map[string]any)
	fabric := net["fabric"].(map[string]any)

	if svcNet["port"] != int(DefaultServicePort) {
		t.Errorf("service port = %v, want %d", svcNet["port"], int(DefaultServicePort))
	}
	if hb["port"] != int(DefaultHeartbeatPort) {
		t.Errorf("heartbeat port = %v, want %d", hb["port"], int(DefaultHeartbeatPort))
	}
	if hb["mode"] != defaultHeartbeatMode {
		t.Errorf("heartbeat mode = %v, want %q", hb["mode"], defaultHeartbeatMode)
	}
	if fabric["port"] != int(DefaultFabricPort) {
		t.Errorf("fabric port = %v, want %d", fabric["port"], int(DefaultFabricPort))
	}
}

func TestDefault_PreservesExistingConfig(t *testing.T) {
	d := &AerospikeClusterDefaulter{}
	cluster := &AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: AerospikeClusterSpec{
			Size:  1,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeConfig: &AerospikeConfigSpec{
				Value: map[string]any{
					"service": map[string]any{
						"cluster-name": "custom-name",
						"proto-fd-max": 20000,
					},
					"network": map[string]any{
						"service": map[string]any{
							"port": 4000,
						},
					},
				},
			},
		},
	}

	if err := d.Default(context.Background(), cluster); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	svc := cluster.Spec.AerospikeConfig.Value["service"].(map[string]any)
	if svc["cluster-name"] != "custom-name" {
		t.Errorf("should preserve custom cluster-name, got %v", svc["cluster-name"])
	}
	if svc["proto-fd-max"] != 20000 {
		t.Errorf("should preserve custom proto-fd-max, got %v", svc["proto-fd-max"])
	}

	net := cluster.Spec.AerospikeConfig.Value["network"].(map[string]any)
	svcNet := net["service"].(map[string]any)
	if svcNet["port"] != 4000 {
		t.Errorf("should preserve custom service port, got %v", svcNet["port"])
	}
}

// --- Storage validation tests ---

func TestValidate_Storage_NoSourceSpecified(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			Storage: &AerospikeStorageSpec{
				Volumes: []VolumeSpec{
					{Name: "data"},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Error("expected error when no volume source is specified")
	}
	if !strings.Contains(err.Error(), "exactly one volume source") {
		t.Errorf("error should mention 'exactly one volume source', got: %v", err)
	}
}

func TestValidate_Storage_MultipleSourcesRejected(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			Storage: &AerospikeStorageSpec{
				Volumes: []VolumeSpec{
					{
						Name: "data",
						Source: VolumeSource{
							PersistentVolume: &PersistentVolumeSpec{Size: "10Gi"},
							EmptyDir:         &corev1.EmptyDirVolumeSource{},
						},
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Error("expected error when multiple volume sources are specified")
	}
	if !strings.Contains(err.Error(), "only one volume source") {
		t.Errorf("error should mention 'only one volume source', got: %v", err)
	}
}

func TestValidate_Storage_HostPathWarning(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			Storage: &AerospikeStorageSpec{
				Volumes: []VolumeSpec{
					{
						Name: "host-data",
						Source: VolumeSource{
							HostPath: &corev1.HostPathVolumeSource{Path: "/mnt/data"},
						},
					},
				},
			},
		},
	}

	warnings, err := v.validate(cluster)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, w := range warnings {
		if strings.Contains(w, "hostPath") && strings.Contains(w, "not recommended") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected hostPath warning, got warnings: %v", warnings)
	}
}

func TestValidate_Storage_SubPathAndSubPathExprMutuallyExclusive_Aerospike(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			Storage: &AerospikeStorageSpec{
				Volumes: []VolumeSpec{
					{
						Name: "data",
						Source: VolumeSource{
							EmptyDir: &corev1.EmptyDirVolumeSource{},
						},
						Aerospike: &AerospikeVolumeAttachment{
							Path:        "/data",
							SubPath:     "sub",
							SubPathExpr: "$(POD_NAME)",
						},
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Error("expected error when both subPath and subPathExpr are set")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("error should mention 'mutually exclusive', got: %v", err)
	}
}

func TestValidate_Storage_SubPathAndSubPathExprMutuallyExclusive_Sidecar(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			Storage: &AerospikeStorageSpec{
				Volumes: []VolumeSpec{
					{
						Name: "data",
						Source: VolumeSource{
							EmptyDir: &corev1.EmptyDirVolumeSource{},
						},
						Sidecars: []VolumeAttachment{
							{
								ContainerName: "exporter",
								Path:          "/data",
								SubPath:       "sub",
								SubPathExpr:   "$(POD_NAME)",
							},
						},
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Error("expected error when sidecar has both subPath and subPathExpr")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("error should mention 'mutually exclusive', got: %v", err)
	}
}

func TestValidate_Storage_SubPathAndSubPathExprMutuallyExclusive_InitContainer(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			Storage: &AerospikeStorageSpec{
				Volumes: []VolumeSpec{
					{
						Name: "data",
						Source: VolumeSource{
							EmptyDir: &corev1.EmptyDirVolumeSource{},
						},
						InitContainers: []VolumeAttachment{
							{
								ContainerName: "init",
								Path:          "/data",
								SubPath:       "sub",
								SubPathExpr:   "$(POD_NAME)",
							},
						},
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Error("expected error when init container has both subPath and subPathExpr")
	}
}

func TestValidate_Storage_DuplicateContainerName_Sidecars(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			Storage: &AerospikeStorageSpec{
				Volumes: []VolumeSpec{
					{
						Name: "data",
						Source: VolumeSource{
							EmptyDir: &corev1.EmptyDirVolumeSource{},
						},
						Sidecars: []VolumeAttachment{
							{ContainerName: "exporter", Path: "/data"},
							{ContainerName: "exporter", Path: "/data2"},
						},
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Fatal("expected error for duplicate containerName in sidecars")
	}
	if !strings.Contains(err.Error(), "sidecars[1] containerName \"exporter\" duplicates sidecars[0]") {
		t.Errorf("error should mention duplicate sidecars containerName, got: %v", err)
	}
}

func TestValidate_Storage_DuplicateContainerName_InitContainers(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			Storage: &AerospikeStorageSpec{
				Volumes: []VolumeSpec{
					{
						Name: "data",
						Source: VolumeSource{
							EmptyDir: &corev1.EmptyDirVolumeSource{},
						},
						InitContainers: []VolumeAttachment{
							{ContainerName: "init", Path: "/data"},
							{ContainerName: "init", Path: "/data2"},
						},
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Fatal("expected error for duplicate containerName in initContainers")
	}
	if !strings.Contains(err.Error(), "initContainers[1] containerName \"init\" duplicates initContainers[0]") {
		t.Errorf("error should mention duplicate initContainers containerName, got: %v", err)
	}
}

func TestValidate_Storage_DuplicateContainerName_SidecarVsInitContainer(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			Storage: &AerospikeStorageSpec{
				Volumes: []VolumeSpec{
					{
						Name: "data",
						Source: VolumeSource{
							EmptyDir: &corev1.EmptyDirVolumeSource{},
						},
						Sidecars: []VolumeAttachment{
							{ContainerName: "shared", Path: "/data"},
						},
						InitContainers: []VolumeAttachment{
							{ContainerName: "shared", Path: "/data"},
						},
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Fatal("expected error when sidecars and initContainers share a containerName")
	}
	if !strings.Contains(err.Error(), "initContainers[0] containerName \"shared\" duplicates sidecars[0]") {
		t.Errorf("error should mention sidecars/initContainers cross duplicate, got: %v", err)
	}
}

// --- PodSpec sidecar / initContainer name validation tests ---

func TestValidate_PodSpec_DuplicateContainerName_Sidecars(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			PodSpec: &AerospikePodSpec{
				Sidecars: []corev1.Container{
					{Name: "exporter", Image: "exporter:v1"},
					{Name: "exporter", Image: "exporter:v2"},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Fatal("expected error for duplicate container name in podSpec.sidecars")
	}
	if !strings.Contains(err.Error(), `spec.podSpec.sidecars[1] name "exporter" duplicates sidecars[0]`) {
		t.Errorf("error should mention duplicate sidecars name, got: %v", err)
	}
}

func TestValidate_PodSpec_DuplicateContainerName_InitContainers(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			PodSpec: &AerospikePodSpec{
				InitContainers: []corev1.Container{
					{Name: "warmup", Image: "warmup:v1"},
					{Name: "warmup", Image: "warmup:v2"},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Fatal("expected error for duplicate container name in podSpec.initContainers")
	}
	if !strings.Contains(err.Error(), `spec.podSpec.initContainers[1] name "warmup" duplicates initContainers[0]`) {
		t.Errorf("error should mention duplicate initContainers name, got: %v", err)
	}
}

func TestValidate_PodSpec_DuplicateContainerName_SidecarVsInitContainer(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			PodSpec: &AerospikePodSpec{
				Sidecars: []corev1.Container{
					{Name: "shared", Image: "side:v1"},
				},
				InitContainers: []corev1.Container{
					{Name: "shared", Image: "init:v1"},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Fatal("expected error when podSpec sidecars and initContainers share a name")
	}
	if !strings.Contains(err.Error(), `spec.podSpec.initContainers[0] name "shared" duplicates sidecars[0]`) {
		t.Errorf("error should mention cross sidecars/initContainers duplicate, got: %v", err)
	}
}

func TestValidate_PodSpec_SidecarConflictWithBuiltinContainer(t *testing.T) {
	cases := []struct {
		builtinName string
	}{
		{"aerospike-server"},
		{"aerospike-init"},
	}
	for _, tc := range cases {
		t.Run(tc.builtinName, func(t *testing.T) {
			v := &AerospikeClusterValidator{}
			cluster := &AerospikeCluster{
				Spec: AerospikeClusterSpec{
					Size:  3,
					Image: "aerospike:ce-8.1.1.1",
					PodSpec: &AerospikePodSpec{
						Sidecars: []corev1.Container{
							{Name: tc.builtinName, Image: "user:v1"},
						},
					},
				},
			}

			_, err := v.validate(cluster)
			if err == nil {
				t.Fatalf("expected error when sidecar name %q collides with built-in container", tc.builtinName)
			}
			want := fmt.Sprintf(`spec.podSpec.sidecars[0] name %q conflicts with operator built-in container name`, tc.builtinName)
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error should mention built-in conflict, got: %v", err)
			}
		})
	}
}

func TestValidate_PodSpec_InitContainerConflictWithBuiltinContainer(t *testing.T) {
	cases := []struct {
		builtinName string
	}{
		{"aerospike-server"},
		{"aerospike-init"},
	}
	for _, tc := range cases {
		t.Run(tc.builtinName, func(t *testing.T) {
			v := &AerospikeClusterValidator{}
			cluster := &AerospikeCluster{
				Spec: AerospikeClusterSpec{
					Size:  3,
					Image: "aerospike:ce-8.1.1.1",
					PodSpec: &AerospikePodSpec{
						InitContainers: []corev1.Container{
							{Name: tc.builtinName, Image: "user:v1"},
						},
					},
				},
			}

			_, err := v.validate(cluster)
			if err == nil {
				t.Fatalf("expected error when initContainer name %q collides with built-in container", tc.builtinName)
			}
			want := fmt.Sprintf(`spec.podSpec.initContainers[0] name %q conflicts with operator built-in container name`, tc.builtinName)
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error should mention built-in conflict, got: %v", err)
			}
		})
	}
}

func TestValidate_PodSpec_BuiltinConflictSuppressesDuplicateError(t *testing.T) {
	cases := []struct {
		name        string
		podSpec     *AerospikePodSpec
		conflictMsg string
	}{
		{
			name: "sidecars both aerospike-server",
			podSpec: &AerospikePodSpec{
				Sidecars: []corev1.Container{
					{Name: "aerospike-server", Image: "user:v1"},
					{Name: "aerospike-server", Image: "user:v2"},
				},
			},
			conflictMsg: `spec.podSpec.sidecars[0] name "aerospike-server" conflicts with operator built-in container name`,
		},
		{
			name: "initContainers both aerospike-init",
			podSpec: &AerospikePodSpec{
				InitContainers: []corev1.Container{
					{Name: "aerospike-init", Image: "user:v1"},
					{Name: "aerospike-init", Image: "user:v2"},
				},
			},
			conflictMsg: `spec.podSpec.initContainers[0] name "aerospike-init" conflicts with operator built-in container name`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := &AerospikeClusterValidator{}
			cluster := &AerospikeCluster{
				Spec: AerospikeClusterSpec{
					Size:    3,
					Image:   "aerospike:ce-8.1.1.1",
					PodSpec: tc.podSpec,
				},
			}

			_, err := v.validate(cluster)
			if err == nil {
				t.Fatal("expected built-in conflict error")
			}
			if !strings.Contains(err.Error(), tc.conflictMsg) {
				t.Errorf("error should mention built-in conflict, got: %v", err)
			}
			if got := strings.Count(err.Error(), "duplicates"); got != 0 {
				t.Errorf("expected no duplicate-name errors when built-in conflict fires, got %d in: %v", got, err)
			}
		})
	}
}

func TestValidate_Storage_DeleteLocalStorageWithoutClasses(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			Storage: &AerospikeStorageSpec{
				DeleteLocalStorageOnRestart: boolPtr(true),
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Error("expected error when deleteLocalStorageOnRestart is true but localStorageClasses is empty")
	}
	if !strings.Contains(err.Error(), "localStorageClasses") {
		t.Errorf("error should mention 'localStorageClasses', got: %v", err)
	}
}

func TestValidate_Storage_LocalStorageClassesWithoutDeleteFlag(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			Storage: &AerospikeStorageSpec{
				LocalStorageClasses: []string{"local-path"},
				Volumes: []VolumeSpec{
					{
						Name: "data",
						Source: VolumeSource{
							PersistentVolume: &PersistentVolumeSpec{Size: "10Gi"},
						},
					},
				},
			},
		},
	}

	warnings, err := v.validate(cluster)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, w := range warnings {
		if strings.Contains(w, "localStorageClasses") && strings.Contains(w, "deleteLocalStorageOnRestart") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected warning about localStorageClasses without deleteLocalStorageOnRestart, got: %v", warnings)
	}
}

func TestValidate_Storage_ValidConfig(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			Storage: &AerospikeStorageSpec{
				LocalStorageClasses:         []string{"local-path"},
				DeleteLocalStorageOnRestart: boolPtr(true),
				Volumes: []VolumeSpec{
					{
						Name: "data",
						Source: VolumeSource{
							PersistentVolume: &PersistentVolumeSpec{
								Size:         "10Gi",
								StorageClass: "local-path",
							},
						},
						Aerospike:  &AerospikeVolumeAttachment{Path: "/data"},
						InitMethod: VolumeInitMethodDeleteFiles,
						WipeMethod: VolumeWipeMethodDeleteFiles,
					},
					{
						Name: "logs",
						Source: VolumeSource{
							EmptyDir: &corev1.EmptyDirVolumeSource{},
						},
						Aerospike: &AerospikeVolumeAttachment{Path: "/logs"},
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err != nil {
		t.Errorf("unexpected error for valid storage config: %v", err)
	}
}

func TestValidate_Storage_DeleteLocalStorageFalse_NoError(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			Storage: &AerospikeStorageSpec{
				DeleteLocalStorageOnRestart: boolPtr(false),
			},
		},
	}

	_, err := v.validate(cluster)
	if err != nil {
		t.Errorf("unexpected error when deleteLocalStorageOnRestart is false: %v", err)
	}
}

func TestValidate_Storage_CascadeDeleteOnNonPersistent(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			Storage: &AerospikeStorageSpec{
				Volumes: []VolumeSpec{
					{
						Name: "data",
						Source: VolumeSource{
							EmptyDir: &corev1.EmptyDirVolumeSource{},
						},
						CascadeDelete: boolPtr(true),
					},
				},
			},
		},
	}

	// CascadeDelete on non-persistent should not cause error but should warn
	warnings, err := v.validate(cluster)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	found := false
	for _, w := range warnings {
		if strings.Contains(w, "cascadeDelete") && strings.Contains(w, "no effect") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected warning about cascadeDelete on non-persistent volume, got warnings: %v", warnings)
	}
}

func TestValidate_Storage_DuplicateVolumeNames(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			Storage: &AerospikeStorageSpec{
				Volumes: []VolumeSpec{
					{
						Name: "data",
						Source: VolumeSource{
							PersistentVolume: &PersistentVolumeSpec{Size: "10Gi"},
						},
					},
					{
						Name: "data",
						Source: VolumeSource{
							PersistentVolume: &PersistentVolumeSpec{Size: "20Gi"},
						},
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Error("expected error for duplicate volume names")
	}
	if !strings.Contains(err.Error(), "duplicate volume name") {
		t.Errorf("error should mention 'duplicate volume name', got: %v", err)
	}
}

func TestValidate_Storage_GlobalPoliciesAccepted(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			Storage: &AerospikeStorageSpec{
				FilesystemVolumePolicy: &AerospikeVolumePolicy{
					InitMethod:    VolumeInitMethodDeleteFiles,
					WipeMethod:    VolumeWipeMethodDeleteFiles,
					CascadeDelete: boolPtr(true),
				},
				BlockVolumePolicy: &AerospikeVolumePolicy{
					InitMethod:    VolumeInitMethodBlkdiscard,
					WipeMethod:    VolumeWipeMethodBlkdiscard,
					CascadeDelete: boolPtr(false),
				},
				Volumes: []VolumeSpec{
					{
						Name: "data",
						Source: VolumeSource{
							PersistentVolume: &PersistentVolumeSpec{Size: "10Gi"},
						},
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err != nil {
		t.Errorf("unexpected error for valid storage with global policies: %v", err)
	}
}

// --- isEnterpriseTag tests ---

func TestIsEnterpriseTag(t *testing.T) {
	tests := []struct {
		image    string
		expected bool
	}{
		{"aerospike:ce-8.1.1.1", false},
		{"aerospike:ee-8.0.0.1_1", true},
		{"aerospike:EE-8.0.0.1", true},
		{"aerospike:ent-8.0.0", true},
		{"aerospike:ENT-8.1.0", true},
		{"aerospike:latest", false},
		{"aerospike", false},
		{"myrepo/aerospike:ce-8.1.1.1", false},
		{"myrepo/aerospike:ee-8.1.1.1", true},
		{"myrepo/aerospike:ent-8.1.1.1", true},
		// Registry references with a port: the host:port colon must NOT be read
		// as the tag separator, otherwise an Enterprise image slips through.
		{"myregistry.io:5000/aerospike:ce-8.1.1.1", false},
		{"myregistry.io:5000/aerospike:ee-8.0.0.1_1", true},
		{"localhost:32000/aerospike:ent-8.1.0", true},
		// A ported registry reference with no tag is untagged (the only colon is
		// the registry port), so it is not an enterprise tag.
		{"myregistry.io:5000/aerospike", false},
	}

	for _, tc := range tests {
		got := isEnterpriseTag(tc.image)
		if got != tc.expected {
			t.Errorf("isEnterpriseTag(%q) = %v, want %v", tc.image, got, tc.expected)
		}
	}
}

// --- imageTag tests ---

// TestImageTag covers tag extraction including the registry-with-port case that
// a naive SplitN(image, ":", 2) misparsed (reading the host:port colon as the
// tag separator), which silently bypassed the CE version / enterprise guards.
func TestImageTag(t *testing.T) {
	tests := []struct {
		image string
		want  string
	}{
		{"aerospike:ce-8.1.1.1", "ce-8.1.1.1"},
		{"aerospike", ""},
		{"aerospike:", ""},
		{"myrepo/aerospike:ce-8.1.1.1", "ce-8.1.1.1"},
		// Registry with a port: tag is after the last colon, which follows the
		// last '/'.
		{"myregistry.io:5000/aerospike:ce-8.1.1.1", "ce-8.1.1.1"},
		{"localhost:32000/aerospike:ent-8.1.0", "ent-8.1.0"},
		// Ported registry, no tag: the only colon is the registry port, so the
		// reference is untagged.
		{"myregistry.io:5000/aerospike", ""},
		{"localhost:32000/aerospike", ""},
		// Digest-pinned refs: the "@sha256:..." suffix must be stripped before
		// locating the tag colon, otherwise the digest hex is returned as the tag
		// and the CE-version / enterprise guards are silently skipped.
		{"aerospike:ce-8.1.1.1@sha256:abc123", "ce-8.1.1.1"},
		{"myregistry.io:5000/aerospike:ce-8.1.1.1@sha256:abc123", "ce-8.1.1.1"},
		// Digest only, no tag: still untagged after stripping the digest.
		{"aerospike@sha256:abc123", ""},
		{"myregistry.io:5000/aerospike@sha256:abc123", ""},
	}

	for _, tc := range tests {
		if got := imageTag(tc.image); got != tc.want {
			t.Errorf("imageTag(%q) = %q, want %q", tc.image, got, tc.want)
		}
	}
}

// --- Percentage validation edge case tests ---

func TestValidatePositiveIntOrString_PercentageEdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		val       intstr.IntOrString
		wantError bool
	}{
		{"valid 50%", intstr.FromString("50%"), false},
		{"valid 1%", intstr.FromString("1%"), false},
		{"valid 100%", intstr.FromString("100%"), false},
		{"invalid 0%", intstr.FromString("0%"), true},
		{"invalid 101%", intstr.FromString("101%"), true},
		{"invalid abc%", intstr.FromString("abc%"), true},
		{"invalid -5%", intstr.FromString("-5%"), true},
		{"invalid 200%", intstr.FromString("200%"), true},
		{"not a percentage", intstr.FromString("hello"), true},
		{"valid int", intstr.FromInt32(5), false},
		{"invalid zero int", intstr.FromInt32(0), true},
		{"invalid negative int", intstr.FromInt32(-1), true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			val := tc.val
			result := validateIntOrString(&val, "testField", 1)
			if tc.wantError && result == "" {
				t.Errorf("expected error for %v, got none", tc.val)
			}
			if !tc.wantError && result != "" {
				t.Errorf("unexpected error for %v: %s", tc.val, result)
			}
		})
	}
}

func TestValidateNonNegativeIntOrString_PercentageEdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		val       intstr.IntOrString
		wantError bool
	}{
		{"valid 50%", intstr.FromString("50%"), false},
		{"valid 0%", intstr.FromString("0%"), false},
		{"valid 100%", intstr.FromString("100%"), false},
		{"invalid 101%", intstr.FromString("101%"), true},
		{"invalid abc%", intstr.FromString("abc%"), true},
		{"invalid -5%", intstr.FromString("-5%"), true},
		{"not a percentage", intstr.FromString("hello"), true},
		{"valid zero int", intstr.FromInt32(0), false},
		{"valid positive int", intstr.FromInt32(5), false},
		{"invalid negative int", intstr.FromInt32(-1), true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			val := tc.val
			result := validateIntOrString(&val, "testField", 0)
			if tc.wantError && result == "" {
				t.Errorf("expected error for %v, got none", tc.val)
			}
			if !tc.wantError && result != "" {
				t.Errorf("unexpected error for %v: %s", tc.val, result)
			}
		})
	}
}

// --- ACL role cross-reference tests ---

func TestValidate_ACLUserReferencesUndefinedRole(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeAccessControl: &AerospikeAccessControlSpec{
				Roles: []AerospikeRoleSpec{
					{
						Name:       "custom-reader",
						Privileges: []string{"read"},
					},
				},
				Users: []AerospikeUserSpec{
					{
						Name:       "admin",
						SecretName: "admin-secret",
						Roles:      []string{"sys-admin", "user-admin"},
					},
					{
						Name:       "app-user",
						SecretName: "app-secret",
						Roles:      []string{"custom-reader", "nonexistent-role"},
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Fatal("expected error for user referencing undefined role")
	}
	if !strings.Contains(err.Error(), "nonexistent-role") {
		t.Errorf("error should mention 'nonexistent-role', got: %v", err)
	}
	// "custom-reader" is defined in Roles, should not be flagged
	if strings.Contains(err.Error(), "custom-reader") {
		t.Errorf("error should not mention 'custom-reader' (it is defined), got: %v", err)
	}
}

func TestValidate_ACLUserReferencesBuiltinRolesOnly(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeAccessControl: &AerospikeAccessControlSpec{
				Users: []AerospikeUserSpec{
					{
						Name:       "admin",
						SecretName: "admin-secret",
						Roles:      []string{"sys-admin", "user-admin"},
					},
					{
						Name:       "reader",
						SecretName: "reader-secret",
						Roles:      []string{"read"},
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err != nil {
		t.Errorf("unexpected error for users referencing only built-in roles: %v", err)
	}
}

func TestValidate_ACLUserReferencesCustomDefinedRole(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeAccessControl: &AerospikeAccessControlSpec{
				Roles: []AerospikeRoleSpec{
					{
						Name:       "app-role",
						Privileges: []string{"read-write"},
					},
				},
				Users: []AerospikeUserSpec{
					{
						Name:       "admin",
						SecretName: "admin-secret",
						Roles:      []string{"sys-admin", "user-admin"},
					},
					{
						Name:       "app-user",
						SecretName: "app-secret",
						Roles:      []string{"app-role"},
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err != nil {
		t.Errorf("unexpected error for user referencing a defined custom role: %v", err)
	}
}

// --- Operations InProgress change attempt tests ---

func TestValidateUpdate_RejectsOperationChangeWhileInProgress(t *testing.T) {
	v := &AerospikeClusterValidator{}

	oldCluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			Operations: []OperationSpec{
				{Kind: OperationWarmRestart, ID: "op-1"},
			},
		},
		Status: AerospikeClusterStatus{
			OperationStatus: &OperationStatus{
				ID:    "op-1",
				Kind:  OperationWarmRestart,
				Phase: AerospikePhaseInProgress,
			},
		},
	}

	newCluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			Operations: []OperationSpec{
				{Kind: OperationPodRestart, ID: "op-2"},
			},
		},
	}

	_, err := v.ValidateUpdate(context.Background(), oldCluster, newCluster)
	if err == nil {
		t.Fatal("expected error when changing operation while one is InProgress")
	}
	if !strings.Contains(err.Error(), "InProgress") {
		t.Errorf("error should mention 'InProgress', got: %v", err)
	}
}

func TestValidateUpdate_RejectsPodListChangeWhileInProgress(t *testing.T) {
	v := &AerospikeClusterValidator{}

	oldCluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			Operations: []OperationSpec{
				{Kind: OperationWarmRestart, ID: "op-1", PodList: []string{"pod-0", "pod-1"}},
			},
		},
		Status: AerospikeClusterStatus{
			OperationStatus: &OperationStatus{
				ID:    "op-1",
				Kind:  OperationWarmRestart,
				Phase: AerospikePhaseInProgress,
			},
		},
	}

	// Same ID and Kind, but PodList changed — must still be rejected to avoid
	// silently redirecting an in-flight operation to a different pod set.
	newCluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			Operations: []OperationSpec{
				{Kind: OperationWarmRestart, ID: "op-1", PodList: []string{"pod-0", "pod-2"}},
			},
		},
	}

	_, err := v.ValidateUpdate(context.Background(), oldCluster, newCluster)
	if err == nil {
		t.Fatal("expected error when changing PodList of an InProgress operation")
	}
	if !strings.Contains(err.Error(), "InProgress") {
		t.Errorf("error should mention 'InProgress', got: %v", err)
	}
}

func TestValidateUpdate_AllowsSameOperationWhileInProgress(t *testing.T) {
	v := &AerospikeClusterValidator{}

	oldCluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			Operations: []OperationSpec{
				{Kind: OperationWarmRestart, ID: "op-1"},
			},
		},
		Status: AerospikeClusterStatus{
			OperationStatus: &OperationStatus{
				ID:    "op-1",
				Kind:  OperationWarmRestart,
				Phase: AerospikePhaseInProgress,
			},
		},
	}

	newCluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			Operations: []OperationSpec{
				{Kind: OperationWarmRestart, ID: "op-1"},
			},
		},
	}

	_, err := v.ValidateUpdate(context.Background(), oldCluster, newCluster)
	if err != nil {
		t.Errorf("unexpected error when keeping same operation while InProgress: %v", err)
	}
}

func TestValidateUpdate_AllowsOperationChangeWhenCompleted(t *testing.T) {
	v := &AerospikeClusterValidator{}

	oldCluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			Operations: []OperationSpec{
				{Kind: OperationWarmRestart, ID: "op-1"},
			},
		},
		Status: AerospikeClusterStatus{
			OperationStatus: &OperationStatus{
				ID:    "op-1",
				Kind:  OperationWarmRestart,
				Phase: AerospikePhaseCompleted,
			},
		},
	}

	newCluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			Operations: []OperationSpec{
				{Kind: OperationPodRestart, ID: "op-2"},
			},
		},
	}

	_, err := v.ValidateUpdate(context.Background(), oldCluster, newCluster)
	if err != nil {
		t.Errorf("unexpected error when changing operation after previous completed: %v", err)
	}
}

// --- Replication-factor vs spec.size cross-validation tests ---

func TestValidate_ReplicationFactorExceedsClusterSize(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  2,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeConfig: &AerospikeConfigSpec{
				Value: map[string]any{
					"namespaces": []any{
						map[string]any{
							"name":               "test",
							"replication-factor": 3,
						},
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Fatal("expected error when replication-factor exceeds cluster size")
	}
	if !strings.Contains(err.Error(), "exceeds cluster size") {
		t.Errorf("error should mention 'exceeds cluster size', got: %v", err)
	}
}

func TestValidate_ReplicationFactorEqualsClusterSize(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeConfig: &AerospikeConfigSpec{
				Value: map[string]any{
					"namespaces": []any{
						map[string]any{
							"name":               "test",
							"replication-factor": 3,
						},
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err != nil {
		t.Errorf("unexpected error when replication-factor equals cluster size: %v", err)
	}
}

// --- Monitoring validation tests ---

func TestValidate_MonitoringPortConflict(t *testing.T) {
	v := &AerospikeClusterValidator{}

	conflictPorts := map[int32]string{
		3000: "service",
		3001: "fabric",
		3002: "heartbeat",
		3003: "info",
	}

	for port, portName := range conflictPorts {
		cluster := &AerospikeCluster{
			Spec: AerospikeClusterSpec{
				Size:  3,
				Image: "aerospike:ce-8.1.1.1",
				Monitoring: &AerospikeMonitoringSpec{
					Enabled:       true,
					ExporterImage: "exporter:v1",
					Port:          port,
				},
			},
		}

		_, err := v.validate(cluster)
		if err == nil {
			t.Errorf("expected error for monitoring port %d conflicting with %s", port, portName)
		}
		if !strings.Contains(err.Error(), "conflicts") {
			t.Errorf("error for port %d should mention 'conflicts', got: %v", port, err)
		}
	}
}

func TestValidate_MonitoringPortNoConflict(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			Monitoring: &AerospikeMonitoringSpec{
				Enabled:       true,
				ExporterImage: "exporter:v1",
				Port:          9145,
			},
		},
	}

	_, err := v.validate(cluster)
	if err != nil {
		t.Errorf("unexpected error for valid monitoring port: %v", err)
	}
}

func TestValidate_MonitoringEmptyImage(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			Monitoring: &AerospikeMonitoringSpec{
				Enabled:       true,
				ExporterImage: "",
				Port:          9145,
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Error("expected error for empty exporter image")
	}
	if !strings.Contains(err.Error(), "exporterImage") {
		t.Errorf("error should mention 'exporterImage', got: %v", err)
	}
}

func TestValidate_MonitoringLatestTagWarning(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			Monitoring: &AerospikeMonitoringSpec{
				Enabled:       true,
				ExporterImage: "aerospike/aerospike-prometheus-exporter:latest",
				Port:          9145,
			},
		},
	}

	warnings, err := v.validate(cluster)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, w := range warnings {
		if strings.Contains(w, "monitoring.exporterImage") && strings.Contains(w, "latest") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected warning about 'latest' tag, got warnings: %v", warnings)
	}
}

func TestValidate_MonitoringNoTagWarning(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			Monitoring: &AerospikeMonitoringSpec{
				Enabled:       true,
				ExporterImage: "aerospike/aerospike-prometheus-exporter",
				Port:          9145,
			},
		},
	}

	warnings, err := v.validate(cluster)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, w := range warnings {
		if strings.Contains(w, "monitoring.exporterImage") && strings.Contains(w, "no tag") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected warning about missing tag, got warnings: %v", warnings)
	}
}

func TestValidate_MonitoringValidConfig(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			Monitoring: &AerospikeMonitoringSpec{
				Enabled:       true,
				ExporterImage: "aerospike/aerospike-prometheus-exporter:1.16.1",
				Port:          9145,
			},
		},
	}

	warnings, err := v.validate(cluster)
	if err != nil {
		t.Errorf("unexpected error for valid monitoring config: %v", err)
	}
	for _, w := range warnings {
		if strings.Contains(w, "monitoring") {
			t.Errorf("unexpected monitoring warning: %v", w)
		}
	}
}

func TestValidate_ServiceMonitorIntervalInvalid(t *testing.T) {
	v := &AerospikeClusterValidator{}

	// Values the Prometheus Operator's Duration schema rejects. ACKO must catch
	// these at admission instead of letting the ServiceMonitor fail at apply.
	invalidIntervals := []string{
		"30",        // missing unit
		"5 seconds", // not a duration string
		"thirty",    // non-numeric
		"30S",       // wrong-case unit
		"1.5s",      // fractional not allowed
		"-5s",       // negative
	}

	for _, interval := range invalidIntervals {
		cluster := &AerospikeCluster{
			Spec: AerospikeClusterSpec{
				Size:  3,
				Image: "aerospike:ce-8.1.1.1",
				Monitoring: &AerospikeMonitoringSpec{
					Enabled:       true,
					ExporterImage: "aerospike/aerospike-prometheus-exporter:1.16.1",
					Port:          9145,
					ServiceMonitor: &ServiceMonitorSpec{
						Enabled:  true,
						Interval: interval,
					},
				},
			},
		}

		_, err := v.validate(cluster)
		if err == nil {
			t.Errorf("expected error for invalid serviceMonitor interval %q", interval)
			continue
		}
		if !strings.Contains(err.Error(), "serviceMonitor.interval") {
			t.Errorf("interval %q: error should mention 'serviceMonitor.interval', got: %v", interval, err)
		}
	}
}

func TestValidate_ServiceMonitorIntervalValid(t *testing.T) {
	v := &AerospikeClusterValidator{}

	// Values the Prometheus Operator's Duration schema accepts, plus the empty
	// string (defaulter fills it in before reconcile).
	validIntervals := []string{"30s", "1m", "500ms", "2h30m", "1y2w3d", "0", ""}

	for _, interval := range validIntervals {
		cluster := &AerospikeCluster{
			Spec: AerospikeClusterSpec{
				Size:  3,
				Image: "aerospike:ce-8.1.1.1",
				Monitoring: &AerospikeMonitoringSpec{
					Enabled:       true,
					ExporterImage: "aerospike/aerospike-prometheus-exporter:1.16.1",
					Port:          9145,
					ServiceMonitor: &ServiceMonitorSpec{
						Enabled:  true,
						Interval: interval,
					},
				},
			},
		}

		_, err := v.validate(cluster)
		if err != nil {
			t.Errorf("unexpected error for valid serviceMonitor interval %q: %v", interval, err)
		}
	}
}

// --- ServiceMonitor / PrometheusRule label validation tests ---
//
// Mirrors the serviceMonitor.interval fix (#316): the reconciler copies these
// labels verbatim onto the ServiceMonitor/PrometheusRule via SetLabels, so an
// invalid label passes the API server but is rejected by the Prometheus
// Operator CRD at apply time. Admission must reject it up front.

func TestValidate_MonitoringLabelsRejectsInvalid(t *testing.T) {
	v := &AerospikeClusterValidator{}
	longKey := strings.Repeat("a", 70)   // >63 chars name segment
	longValue := strings.Repeat("b", 71) // >63 chars label value
	cases := []struct {
		name      string
		labels    map[string]string
		fieldPath string
		setSM     bool // labels on serviceMonitor (else prometheusRule)
	}{
		{
			name:      "serviceMonitor invalid key chars",
			labels:    map[string]string{"bad key!": "value"},
			fieldPath: "monitoring.serviceMonitor.labels",
			setSM:     true,
		},
		{
			name:      "serviceMonitor over-long key",
			labels:    map[string]string{longKey: "value"},
			fieldPath: "monitoring.serviceMonitor.labels",
			setSM:     true,
		},
		{
			name:      "prometheusRule over-long value",
			labels:    map[string]string{"team": longValue},
			fieldPath: "monitoring.prometheusRule.labels",
			setSM:     false,
		},
		{
			name:      "prometheusRule invalid value chars",
			labels:    map[string]string{"team": "bad value!"},
			fieldPath: "monitoring.prometheusRule.labels",
			setSM:     false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := &AerospikeMonitoringSpec{
				Enabled:       true,
				ExporterImage: "exporter:v1",
				Port:          9145,
			}
			if tc.setSM {
				m.ServiceMonitor = &ServiceMonitorSpec{Enabled: true, Labels: tc.labels}
			} else {
				m.PrometheusRule = &PrometheusRuleSpec{Enabled: true, Labels: tc.labels}
			}
			cluster := &AerospikeCluster{
				Spec: AerospikeClusterSpec{
					Size:       3,
					Image:      "aerospike:ce-8.1.1.1",
					Monitoring: m,
				},
			}
			_, err := v.validate(cluster)
			if err == nil {
				t.Fatalf("expected error for invalid labels %v, got nil", tc.labels)
			}
			if !strings.Contains(err.Error(), tc.fieldPath) {
				t.Errorf("expected error to mention field path %q, got: %v", tc.fieldPath, err)
			}
		})
	}
}

func TestValidate_MonitoringLabelsAcceptsValid(t *testing.T) {
	v := &AerospikeClusterValidator{}
	validLabels := map[string]string{
		"team":                      "platform",
		"app.kubernetes.io/part-of": "aerospike",
		"release":                   "kube-prometheus-stack",
		"empty-value-allowed":       "",
	}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			Monitoring: &AerospikeMonitoringSpec{
				Enabled:        true,
				ExporterImage:  "exporter:v1",
				Port:           9145,
				ServiceMonitor: &ServiceMonitorSpec{Enabled: true, Interval: "30s", Labels: validLabels},
				PrometheusRule: &PrometheusRuleSpec{Enabled: true, Labels: validLabels},
			},
		},
	}
	_, err := v.validate(cluster)
	if err != nil {
		t.Errorf("expected no error for valid labels, got: %v", err)
	}
}

func TestValidate_ServiceMonitorIntervalSkippedWhenDisabled(t *testing.T) {
	v := &AerospikeClusterValidator{}

	// An invalid interval must not be rejected when the ServiceMonitor itself is
	// disabled — no ServiceMonitor is created, so the interval is never applied.
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			Monitoring: &AerospikeMonitoringSpec{
				Enabled:       true,
				ExporterImage: "aerospike/aerospike-prometheus-exporter:1.16.1",
				Port:          9145,
				ServiceMonitor: &ServiceMonitorSpec{
					Enabled:  false,
					Interval: "not-a-duration",
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err != nil {
		t.Errorf("unexpected error for invalid interval on a disabled ServiceMonitor: %v", err)
	}
}

func TestValidate_MonitoringDisabledSkipsValidation(t *testing.T) {
	v := &AerospikeClusterValidator{}
	// Invalid config but disabled — should pass
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			Monitoring: &AerospikeMonitoringSpec{
				Enabled:       false,
				ExporterImage: "",
				Port:          3000,
			},
		},
	}

	_, err := v.validate(cluster)
	if err != nil {
		t.Errorf("disabled monitoring should not be validated, got error: %v", err)
	}
}

// --- Default monitoring image version test ---

func TestDefaultMonitoring_DefaultImageVersion(t *testing.T) {
	d := &AerospikeClusterDefaulter{}
	cluster := &AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			Monitoring: &AerospikeMonitoringSpec{
				Enabled: true,
			},
		},
	}

	if err := d.Default(context.Background(), cluster); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "aerospike/aerospike-prometheus-exporter:1.16.1"
	if cluster.Spec.Monitoring.ExporterImage != expected {
		t.Errorf("ExporterImage = %q, want %q", cluster.Spec.Monitoring.ExporterImage, expected)
	}
}

// --- CustomRules validation tests ---

func TestValidate_CustomRules_ValidStructure(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			Monitoring: &AerospikeMonitoringSpec{
				Enabled:       true,
				ExporterImage: "exporter:v1",
				Port:          9145,
				PrometheusRule: &PrometheusRuleSpec{
					Enabled: true,
					CustomRules: []apiextensionsv1.JSON{
						{Raw: []byte(`{"name":"custom.rules","rules":[{"alert":"TestAlert","expr":"up==0"}]}`)},
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err != nil {
		t.Errorf("unexpected error for valid custom rules: %v", err)
	}
}

func TestValidate_CustomRules_MissingName(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			Monitoring: &AerospikeMonitoringSpec{
				Enabled:       true,
				ExporterImage: "exporter:v1",
				Port:          9145,
				PrometheusRule: &PrometheusRuleSpec{
					Enabled: true,
					CustomRules: []apiextensionsv1.JSON{
						{Raw: []byte(`{"rules":[{"alert":"TestAlert","expr":"up==0"}]}`)},
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Error("expected error for custom rule missing 'name'")
	}
	if !strings.Contains(err.Error(), "missing required field 'name'") {
		t.Errorf("error should mention missing 'name', got: %v", err)
	}
}

func TestValidate_CustomRules_MissingRules(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			Monitoring: &AerospikeMonitoringSpec{
				Enabled:       true,
				ExporterImage: "exporter:v1",
				Port:          9145,
				PrometheusRule: &PrometheusRuleSpec{
					Enabled: true,
					CustomRules: []apiextensionsv1.JSON{
						{Raw: []byte(`{"name":"custom.rules"}`)},
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Error("expected error for custom rule missing 'rules'")
	}
	if !strings.Contains(err.Error(), "missing required field 'rules'") {
		t.Errorf("error should mention missing 'rules', got: %v", err)
	}
}

func TestValidate_CustomRules_InvalidJSON(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			Monitoring: &AerospikeMonitoringSpec{
				Enabled:       true,
				ExporterImage: "exporter:v1",
				Port:          9145,
				PrometheusRule: &PrometheusRuleSpec{
					Enabled: true,
					CustomRules: []apiextensionsv1.JSON{
						{Raw: []byte(`{invalid json}`)},
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Error("expected error for invalid JSON in custom rules")
	}
	if !strings.Contains(err.Error(), "invalid JSON") {
		t.Errorf("error should mention 'invalid JSON', got: %v", err)
	}
}

func TestValidate_CustomRules_MissingBothFields(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			Monitoring: &AerospikeMonitoringSpec{
				Enabled:       true,
				ExporterImage: "exporter:v1",
				Port:          9145,
				PrometheusRule: &PrometheusRuleSpec{
					Enabled: true,
					CustomRules: []apiextensionsv1.JSON{
						{Raw: []byte(`{"foo":"bar"}`)},
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Error("expected error for custom rule missing both 'name' and 'rules'")
	}
	if !strings.Contains(err.Error(), "name") || !strings.Contains(err.Error(), "rules") {
		t.Errorf("error should mention both missing fields, got: %v", err)
	}
}

func TestValidate_CustomRulesRulesMustBeArray(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			Monitoring: &AerospikeMonitoringSpec{
				Enabled:       true,
				ExporterImage: "exporter:v1",
				Port:          9145,
				PrometheusRule: &PrometheusRuleSpec{
					Enabled: true,
					CustomRules: []apiextensionsv1.JSON{
						{Raw: []byte(`{"name":"custom.rules","rules":{}}`)},
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Fatal("expected error for custom rule with object-typed 'rules'")
	}
	if !strings.Contains(err.Error(), "'rules' must be a JSON array") {
		t.Errorf("error should mention type mismatch for 'rules', got: %v", err)
	}
}

func TestValidate_CustomRulesRulesEmptyArray(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			Monitoring: &AerospikeMonitoringSpec{
				Enabled:       true,
				ExporterImage: "exporter:v1",
				Port:          9145,
				PrometheusRule: &PrometheusRuleSpec{
					Enabled: true,
					CustomRules: []apiextensionsv1.JSON{
						{Raw: []byte(`{"name":"custom.rules","rules":[]}`)},
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Fatal("expected error for custom rule with empty 'rules' array")
	}
	if !strings.Contains(err.Error(), "must contain at least one rule") {
		t.Errorf("error should mention empty rules array, got: %v", err)
	}
}

func TestValidate_CustomRulesNameMustBeString(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			Monitoring: &AerospikeMonitoringSpec{
				Enabled:       true,
				ExporterImage: "exporter:v1",
				Port:          9145,
				PrometheusRule: &PrometheusRuleSpec{
					Enabled: true,
					CustomRules: []apiextensionsv1.JSON{
						{Raw: []byte(`{"name":123,"rules":[{"alert":"TestAlert","expr":"up==0"}]}`)},
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Fatal("expected error for custom rule with non-string 'name'")
	}
	if !strings.Contains(err.Error(), "'name' must be a string") {
		t.Errorf("error should mention type mismatch for 'name', got: %v", err)
	}
}

// --- MetricLabels validation tests ---

func TestValidate_MetricLabels_ValidLabels(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			Monitoring: &AerospikeMonitoringSpec{
				Enabled:       true,
				ExporterImage: "exporter:v1",
				Port:          9145,
				MetricLabels: map[string]string{
					"env":    "prod",
					"region": "us-west-2",
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err != nil {
		t.Errorf("unexpected error for valid metric labels: %v", err)
	}
}

func TestValidate_MetricLabels_KeyContainsEquals(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			Monitoring: &AerospikeMonitoringSpec{
				Enabled:       true,
				ExporterImage: "exporter:v1",
				Port:          9145,
				MetricLabels: map[string]string{
					"a=b": "value",
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Error("expected error for metric label key containing '='")
	}
	if !strings.Contains(err.Error(), "must contain only") {
		t.Errorf("error should mention allowed characters, got: %v", err)
	}
}

func TestValidate_MetricLabels_ValueContainsCommaAndEquals(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			Monitoring: &AerospikeMonitoringSpec{
				Enabled:       true,
				ExporterImage: "exporter:v1",
				Port:          9145,
				MetricLabels: map[string]string{
					"env":    "prod,staging",
					"region": "us-east=1",
				},
			},
		},
	}

	// ',' and '=' in values are valid (they are inside TOML-quoted strings).
	_, err := v.validate(cluster)
	if err != nil {
		t.Errorf("unexpected error for metric label values with ',' or '=': %v", err)
	}
}

func TestValidate_MetricLabels_KeyContainsControlChar(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			Monitoring: &AerospikeMonitoringSpec{
				Enabled:       true,
				ExporterImage: "exporter:v1",
				Port:          9145,
				MetricLabels:  map[string]string{"key\x00name": "value"},
			},
		},
	}
	_, err := v.validate(cluster)
	if err == nil {
		t.Error("expected error for control character in metric label key")
	}
}

func TestValidate_MetricLabels_ValueContainsControlChar(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			Monitoring: &AerospikeMonitoringSpec{
				Enabled:       true,
				ExporterImage: "exporter:v1",
				Port:          9145,
				MetricLabels:  map[string]string{"env": "prod\x01staging"},
			},
		},
	}
	_, err := v.validate(cluster)
	if err == nil {
		t.Error("expected error for control character in metric label value")
	}
	if !strings.Contains(err.Error(), "control characters") {
		t.Errorf("error should mention control characters, got: %v", err)
	}
}

// --- validateAccessControl gap tests ---

func TestValidate_ACLInvalidPrivilegeCodeSuperuser(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeAccessControl: &AerospikeAccessControlSpec{
				Roles: []AerospikeRoleSpec{
					{
						Name:       "super-role",
						Privileges: []string{"superuser"},
					},
				},
				Users: []AerospikeUserSpec{
					{
						Name:       "admin",
						SecretName: "admin-secret",
						Roles:      []string{"sys-admin", "user-admin"},
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Fatal("expected error for enterprise-only privilege code 'superuser'")
	}
	if !strings.Contains(err.Error(), "invalid privilege code") {
		t.Errorf("error should mention 'invalid privilege code', got: %v", err)
	}
	if !strings.Contains(err.Error(), "superuser") {
		t.Errorf("error should mention 'superuser', got: %v", err)
	}
}

func TestValidate_ACLUserMissingSecretName(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeAccessControl: &AerospikeAccessControlSpec{
				Users: []AerospikeUserSpec{
					{
						Name:       "admin",
						SecretName: "admin-secret",
						Roles:      []string{"sys-admin", "user-admin"},
					},
					{
						Name:  "app-user",
						Roles: []string{"read"},
						// SecretName intentionally omitted
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Fatal("expected error for user without secretName")
	}
	if !strings.Contains(err.Error(), "secretName") {
		t.Errorf("error should mention 'secretName', got: %v", err)
	}
	if !strings.Contains(err.Error(), "app-user") {
		t.Errorf("error should mention the user name 'app-user', got: %v", err)
	}
}

// --- validateRackConfig gap tests ---

func TestValidate_DuplicateRackLabels(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  4,
			Image: "aerospike:ce-8.1.1.1",
			RackConfig: &RackConfig{
				Racks: []Rack{
					{ID: 1, RackLabel: "zone-a"},
					{ID: 2, RackLabel: "zone-a"},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Fatal("expected error for duplicate rack labels")
	}
	if !strings.Contains(err.Error(), "duplicate rackLabel") {
		t.Errorf("error should mention 'duplicate rackLabel', got: %v", err)
	}
	if !strings.Contains(err.Error(), "zone-a") {
		t.Errorf("error should mention the duplicate label 'zone-a', got: %v", err)
	}
}

// TestValidate_RackAerospikeConfigEnterpriseBypass verifies that enterprise-only
// stanzas and CE-constraint violations injected through a per-rack
// aerospikeConfig override are rejected by the webhook. A rack's aerospikeConfig
// is DeepMerged into the effective config and rendered into the rack's ConfigMap
// (getEffectiveConfig), so without per-rack validation it was a silent CE bypass.
func TestValidate_RackAerospikeConfigEnterpriseBypass(t *testing.T) {
	v := &AerospikeClusterValidator{}

	cases := []struct {
		name      string
		rackCfg   map[string]any
		wantInErr string
	}{
		{
			name:      "xdr via rack override",
			rackCfg:   map[string]any{"xdr": map[string]any{}},
			wantInErr: "xdr",
		},
		{
			name:      "tls via rack override",
			rackCfg:   map[string]any{"tls": map[string]any{}},
			wantInErr: "tls",
		},
		{
			name: "exceeds CE namespace count via rack override",
			rackCfg: map[string]any{
				"namespaces": []any{
					map[string]any{"name": "ns1"},
					map[string]any{"name": "ns2"},
					map[string]any{"name": "ns3"},
				},
			},
			wantInErr: "exceeds CE maximum",
		},
		{
			name: "multicast heartbeat via rack override",
			rackCfg: map[string]any{
				"network": map[string]any{
					"heartbeat": map[string]any{"mode": "multicast"},
				},
			},
			wantInErr: "mesh",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cluster := &AerospikeCluster{
				Spec: AerospikeClusterSpec{
					Size:  3,
					Image: "aerospike:ce-8.1.1.1",
					RackConfig: &RackConfig{
						Racks: []Rack{
							{ID: 1, AerospikeConfig: &AerospikeConfigSpec{Value: tc.rackCfg}},
						},
					},
				},
			}

			_, err := v.validate(cluster)
			if err == nil {
				t.Fatalf("expected error for rack aerospikeConfig %q", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantInErr) {
				t.Errorf("error should mention %q, got: %v", tc.wantInErr, err)
			}
			if !strings.Contains(err.Error(), "rackConfig.racks[id=1].aerospikeConfig") {
				t.Errorf("error should be scoped to the rack, got: %v", err)
			}
		})
	}
}

// TestValidate_RackAerospikeConfigValidCEPasses ensures a legitimate CE rack
// override is not rejected by the new per-rack validation.
func TestValidate_RackAerospikeConfigValidCEPasses(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			RackConfig: &RackConfig{
				Racks: []Rack{
					{ID: 1, AerospikeConfig: &AerospikeConfigSpec{Value: map[string]any{
						"namespaces": []any{
							map[string]any{"name": "test", "replication-factor": 2},
						},
					}}},
				},
			},
		},
	}

	if _, err := v.validate(cluster); err != nil {
		t.Fatalf("valid CE rack aerospikeConfig should pass, got: %v", err)
	}
}

// --- validateOperations gap tests ---

func TestValidate_MultipleOperationsRejected(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			Operations: []OperationSpec{
				{Kind: OperationWarmRestart, ID: "op-1"},
				{Kind: OperationPodRestart, ID: "op-2"},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Fatal("expected error for more than 1 operation")
	}
	if !strings.Contains(err.Error(), "only one operation") {
		t.Errorf("error should mention 'only one operation', got: %v", err)
	}
}

func TestValidate_DuplicateOperationIDs(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			Operations: []OperationSpec{
				{Kind: OperationWarmRestart, ID: "op-dup"},
				{Kind: OperationPodRestart, ID: "op-dup"},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Fatal("expected error for duplicate operation IDs")
	}
	if !strings.Contains(err.Error(), "duplicate operation id") {
		t.Errorf("error should mention 'duplicate operation id', got: %v", err)
	}
}

func TestValidate_OperationIDTooLong(t *testing.T) {
	v := &AerospikeClusterValidator{}
	longID := "this-id-is-way-too-long-for-validation"
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			Operations: []OperationSpec{
				{Kind: OperationWarmRestart, ID: longID},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Fatal("expected error for operation ID > 20 characters")
	}
	if !strings.Contains(err.Error(), "1-20 characters") {
		t.Errorf("error should mention '1-20 characters', got: %v", err)
	}
}

func TestValidate_OperationIDEmpty(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			Operations: []OperationSpec{
				{Kind: OperationWarmRestart, ID: ""},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Fatal("expected error for empty operation ID")
	}
	if !strings.Contains(err.Error(), "1-20 characters") {
		t.Errorf("error should mention '1-20 characters', got: %v", err)
	}
}

// --- validateWorkDirectory gap tests ---

func TestValidate_WorkDirectoryWithoutVolumeWarning(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeConfig: &AerospikeConfigSpec{
				Value: map[string]any{
					"service": map[string]any{
						"work-directory": "/opt/aerospike",
					},
				},
			},
			// No storage configured — no volume for the work directory
		},
	}

	warnings, err := v.validate(cluster)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, w := range warnings {
		if strings.Contains(w, "work-directory") && strings.Contains(w, "no persistent volume") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected warning about work-directory without persistent volume, got warnings: %v", warnings)
	}
}

func TestValidate_WorkDirectoryWithVolumeNoWarning(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeConfig: &AerospikeConfigSpec{
				Value: map[string]any{
					"service": map[string]any{
						"work-directory": "/opt/aerospike",
					},
				},
			},
			Storage: &AerospikeStorageSpec{
				Volumes: []VolumeSpec{
					{
						Name: "workdir",
						Source: VolumeSource{
							PersistentVolume: &PersistentVolumeSpec{Size: "10Gi"},
						},
						Aerospike: &AerospikeVolumeAttachment{Path: "/opt/aerospike"},
					},
				},
			},
		},
	}

	warnings, err := v.validate(cluster)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, w := range warnings {
		if strings.Contains(w, "work-directory") {
			t.Errorf("unexpected work-directory warning when volume is present: %v", w)
		}
	}
}

func TestValidate_WorkDirectorySkippedWhenPolicySet(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeConfig: &AerospikeConfigSpec{
				Value: map[string]any{
					"service": map[string]any{
						"work-directory": "/opt/aerospike",
					},
				},
			},
			ValidationPolicy: &ValidationPolicySpec{
				SkipWorkDirValidate: true,
			},
			// No storage — would normally warn, but skip policy is set
		},
	}

	warnings, err := v.validate(cluster)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, w := range warnings {
		if strings.Contains(w, "work-directory") {
			t.Errorf("unexpected work-directory warning when skipWorkDirValidate is true: %v", w)
		}
	}
}

// --- ValidateUpdate with in-progress operation removing operations ---

func TestValidateUpdate_RejectsRemovingOperationWhileInProgress(t *testing.T) {
	v := &AerospikeClusterValidator{}

	oldCluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			Operations: []OperationSpec{
				{Kind: OperationWarmRestart, ID: "op-1"},
			},
		},
		Status: AerospikeClusterStatus{
			OperationStatus: &OperationStatus{
				ID:    "op-1",
				Kind:  OperationWarmRestart,
				Phase: AerospikePhaseInProgress,
			},
		},
	}

	// New cluster removes operations entirely
	newCluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:       3,
			Image:      "aerospike:ce-8.1.1.1",
			Operations: []OperationSpec{},
		},
	}

	_, err := v.ValidateUpdate(context.Background(), oldCluster, newCluster)
	if err == nil {
		t.Fatal("expected error when removing operation while one is InProgress")
	}
	if !strings.Contains(err.Error(), "cannot change operations") {
		t.Errorf("error should mention 'cannot change operations', got: %v", err)
	}
}

// --- Replication factor float64 bounds check tests ---

func TestValidate_ReplicationFactorNonIntegerFloat(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeConfig: &AerospikeConfigSpec{
				Value: map[string]any{
					"namespaces": []any{
						map[string]any{
							"name":               "test",
							"replication-factor": float64(2.5),
						},
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Fatal("expected error when replication-factor is non-integer float")
	}
	if !strings.Contains(err.Error(), "must be a positive integer") {
		t.Errorf("error should mention 'must be a positive integer', got: %v", err)
	}
}

func TestValidate_ReplicationFactorNegativeFloat(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeConfig: &AerospikeConfigSpec{
				Value: map[string]any{
					"namespaces": []any{
						map[string]any{
							"name":               "test",
							"replication-factor": float64(-1),
						},
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Fatal("expected error when replication-factor is negative")
	}
	if !strings.Contains(err.Error(), "must be a positive integer") {
		t.Errorf("error should mention 'must be a positive integer', got: %v", err)
	}
}

func TestValidate_ReplicationFactorValidFloat(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeConfig: &AerospikeConfigSpec{
				Value: map[string]any{
					"namespaces": []any{
						map[string]any{
							"name":               "test",
							"replication-factor": float64(2),
						},
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err != nil {
		t.Errorf("unexpected error for valid float64 replication-factor: %v", err)
	}
}

// TestValidate_ReplicationFactorStringType verifies that a string-typed
// replication-factor (the common YAML mistake `replication-factor: "2"`) is
// rejected with an error that names the real problem — the wrong type — rather
// than the misleading "must be >= 1, got 0" that the unhandled-type fall-through
// previously produced.
func TestValidate_ReplicationFactorStringType(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeConfig: &AerospikeConfigSpec{
				Value: map[string]any{
					"namespaces": []any{
						map[string]any{
							"name":               "test",
							"replication-factor": "2",
						},
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Fatal("expected error when replication-factor is a string")
	}
	if !strings.Contains(err.Error(), "must be an integer") {
		t.Errorf("error should mention the type mismatch ('must be an integer'), got: %v", err)
	}
	if strings.Contains(err.Error(), "got 0") {
		t.Errorf("error should not report the misleading 'got 0' for a string value, got: %v", err)
	}
}

// TestValidateNamespaceConfig_ReplicationFactorStringType verifies the
// per-namespace CE validation path (validateNamespaceConfig) rejects a
// string-typed replication-factor explicitly instead of silently skipping the
// range check, and that the message is consistent with the
// validateReplicationFactor path ("must be an integer, got string").
func TestValidateNamespaceConfig_ReplicationFactorStringType(t *testing.T) {
	v := &AerospikeClusterValidator{}
	nsMap := map[string]any{
		"name":               "test",
		"replication-factor": "2",
	}

	errs, _ := v.validateNamespaceConfig(nsMap, 0)
	if len(errs) == 0 {
		t.Fatal("expected error when replication-factor is a string")
	}
	found := false
	for _, e := range errs {
		if strings.Contains(e, "replication-factor must be an integer, got string") {
			found = true
		}
		if strings.Contains(e, "got 0") {
			t.Errorf("error should not report the misleading 'got 0' for a string value, got: %v", e)
		}
	}
	if !found {
		t.Errorf("expected 'replication-factor must be an integer, got string' error, got: %v", errs)
	}
}

// TestValidateNamespaceConfig_ReplicationFactorNonIntegerJSONNumber verifies
// that a json.Number that cannot be parsed as an integer is rejected instead
// of silently passing the range check.
func TestValidateNamespaceConfig_ReplicationFactorNonIntegerJSONNumber(t *testing.T) {
	v := &AerospikeClusterValidator{}
	nsMap := map[string]any{
		"name":               "test",
		"replication-factor": json.Number("2.5"),
	}

	errs, _ := v.validateNamespaceConfig(nsMap, 0)
	if len(errs) == 0 {
		t.Fatal("expected error when replication-factor is a non-integer json.Number")
	}
	if !strings.Contains(errs[0], "replication-factor must be an integer, got 2.5") {
		t.Errorf("expected 'replication-factor must be an integer, got 2.5' error, got: %v", errs)
	}
}

// TestValidateReplicationFactor_StringType exercises the cross-check path
// (validateReplicationFactor) directly with a string-typed value to pin the
// "must be an integer, got string" message emitted by its default case.
func TestValidateReplicationFactor_StringType(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeConfig: &AerospikeConfigSpec{
				Value: map[string]any{
					"namespaces": []any{
						map[string]any{
							"name":               "test",
							"replication-factor": "2",
						},
					},
				},
			},
		},
	}

	errs := v.validateReplicationFactor(cluster)
	if len(errs) == 0 {
		t.Fatal("expected error when replication-factor is a string")
	}
	if !strings.Contains(errs[0], "replication-factor must be an integer, got string") {
		t.Errorf("expected 'replication-factor must be an integer, got string' error, got: %v", errs)
	}
}

// --- Rack ID validation tests ---

func TestValidate_RackIDZero(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			RackConfig: &RackConfig{
				Racks: []Rack{
					{ID: 0},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Fatal("expected error when rack ID is 0")
	}
	if !strings.Contains(err.Error(), "rack ID must be > 0") {
		t.Errorf("error should mention 'rack ID must be > 0', got: %v", err)
	}
}

func TestValidate_RackIDNegative(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			RackConfig: &RackConfig{
				Racks: []Rack{
					{ID: -1},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Fatal("expected error when rack ID is negative")
	}
	if !strings.Contains(err.Error(), "rack ID must be > 0") {
		t.Errorf("error should mention 'rack ID must be > 0', got: %v", err)
	}
}

func TestValidate_RackIDPositive(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  6,
			Image: "aerospike:ce-8.1.1.1",
			RackConfig: &RackConfig{
				Racks: []Rack{
					{ID: 1},
					{ID: 2},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err != nil {
		t.Errorf("unexpected error for valid rack IDs: %v", err)
	}
}

// --- Empty image validation tests ---

func TestValidate_EmptyImage(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  1,
			Image: "",
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Fatal("expected error for empty image")
	}
	if !strings.Contains(err.Error(), "spec.image must not be empty") {
		t.Errorf("error = %q, want it to mention empty image", err.Error())
	}
}

// --- Storage volume path validation tests ---

func TestValidate_StorageRelativePath(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  1,
			Image: "aerospike:ce-8.1.1.1",
			Storage: &AerospikeStorageSpec{
				Volumes: []VolumeSpec{
					{
						Name: "data",
						Source: VolumeSource{
							EmptyDir: &corev1.EmptyDirVolumeSource{},
						},
						Aerospike: &AerospikeVolumeAttachment{
							Path: "opt/aerospike/data", // missing leading /
						},
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Fatal("expected error for relative aerospike path")
	}
	if !strings.Contains(err.Error(), "must be an absolute path") {
		t.Errorf("error = %q, want it to mention absolute path", err.Error())
	}
}

func TestValidate_StorageAbsolutePathOK(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  1,
			Image: "aerospike:ce-8.1.1.1",
			Storage: &AerospikeStorageSpec{
				Volumes: []VolumeSpec{
					{
						Name: "data",
						Source: VolumeSource{
							EmptyDir: &corev1.EmptyDirVolumeSource{},
						},
						Aerospike: &AerospikeVolumeAttachment{
							Path: "/opt/aerospike/data",
						},
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err != nil {
		t.Errorf("unexpected error for absolute path: %v", err)
	}
}

// --- Rack node name tests ---

// TestValidate_DuplicateRackNodeName verifies that multiple racks sharing the same
// nodeName is allowed (e.g. soft-rack / preferred anti-affinity deploys all racks
// on the same node intentionally).
func TestValidate_DuplicateRackNodeName(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  4,
			Image: "aerospike:ce-8.1.1.1",
			RackConfig: &RackConfig{
				Racks: []Rack{
					{ID: 1, NodeName: "node-1"},
					{ID: 2, NodeName: "node-1"}, // same node — valid for preferred anti-affinity
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err != nil {
		t.Errorf("unexpected error for racks sharing the same nodeName: %v", err)
	}
}

func TestValidate_UniqueRackNodeNamesOK(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  4,
			Image: "aerospike:ce-8.1.1.1",
			RackConfig: &RackConfig{
				Racks: []Rack{
					{ID: 1, NodeName: "node-1"},
					{ID: 2, NodeName: "node-2"},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err != nil {
		t.Errorf("unexpected error for unique node names: %v", err)
	}
}

// --- Immutable rack ID validation tests ---

func TestValidateUpdate_RackIDRename(t *testing.T) {
	v := &AerospikeClusterValidator{}
	oldCluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  4,
			Image: "aerospike:ce-8.1.1.1",
			RackConfig: &RackConfig{
				Racks: []Rack{
					{ID: 1},
					{ID: 2},
				},
			},
		},
	}
	newCluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  4,
			Image: "aerospike:ce-8.1.1.1",
			RackConfig: &RackConfig{
				Racks: []Rack{
					{ID: 1},
					{ID: 3}, // renamed from 2 → 3
				},
			},
		},
	}

	_, err := v.ValidateUpdate(context.Background(), oldCluster, newCluster)
	if err == nil {
		t.Fatal("expected error for rack ID rename")
	}
	if !strings.Contains(err.Error(), "cannot add new rack IDs") {
		t.Errorf("error = %q, want it to mention rack ID change", err.Error())
	}
}

func TestValidateUpdate_RackAddRemoveOK(t *testing.T) {
	v := &AerospikeClusterValidator{}
	oldCluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  4,
			Image: "aerospike:ce-8.1.1.1",
			RackConfig: &RackConfig{
				Racks: []Rack{
					{ID: 1},
					{ID: 2},
				},
			},
		},
	}
	newCluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  6,
			Image: "aerospike:ce-8.1.1.1",
			RackConfig: &RackConfig{
				Racks: []Rack{
					{ID: 1},
					{ID: 2},
					{ID: 3}, // new rack added
				},
			},
		},
	}

	_, err := v.ValidateUpdate(context.Background(), oldCluster, newCluster)
	if err != nil {
		t.Errorf("unexpected error for rack addition: %v", err)
	}
}

// --- MaxUnavailable validation tests ---

func TestValidate_MaxUnavailableExceedsClusterSize(t *testing.T) {
	v := &AerospikeClusterValidator{}
	mu := intstr.FromInt32(4)
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:           3,
			Image:          "aerospike:ce-8.1.1.1",
			MaxUnavailable: &mu,
		},
	}

	warnings, err := v.validate(cluster)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, w := range warnings {
		if strings.Contains(w, "maxUnavailable") && strings.Contains(w, "cluster size") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected warning about maxUnavailable >= clusterSize, got warnings: %v", warnings)
	}
}

func TestValidate_MaxUnavailableEqualsClusterSize(t *testing.T) {
	v := &AerospikeClusterValidator{}
	mu := intstr.FromInt32(3)
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:           3,
			Image:          "aerospike:ce-8.1.1.1",
			MaxUnavailable: &mu,
		},
	}

	warnings, err := v.validate(cluster)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, w := range warnings {
		if strings.Contains(w, "maxUnavailable") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected warning about maxUnavailable >= clusterSize, got warnings: %v", warnings)
	}
}

func TestValidate_MaxUnavailableLessThanClusterSize(t *testing.T) {
	v := &AerospikeClusterValidator{}
	mu := intstr.FromInt32(1)
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:           3,
			Image:          "aerospike:ce-8.1.1.1",
			MaxUnavailable: &mu,
		},
	}

	warnings, err := v.validate(cluster)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, w := range warnings {
		if strings.Contains(w, "maxUnavailable") {
			t.Errorf("unexpected maxUnavailable warning: %v", w)
		}
	}
}

func TestValidate_MaxUnavailablePercentage100(t *testing.T) {
	v := &AerospikeClusterValidator{}
	mu := intstr.FromString("100%")
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:           3,
			Image:          "aerospike:ce-8.1.1.1",
			MaxUnavailable: &mu,
		},
	}

	warnings, err := v.validate(cluster)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, w := range warnings {
		if strings.Contains(w, "maxUnavailable") && strings.Contains(w, "100%") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected warning about maxUnavailable 100%%, got warnings: %v", warnings)
	}
}

func TestValidate_MaxUnavailableNil(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
		},
	}

	warnings, err := v.validate(cluster)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, w := range warnings {
		if strings.Contains(w, "maxUnavailable") {
			t.Errorf("unexpected maxUnavailable warning: %v", w)
		}
	}
}

// TestValidate_MaxUnavailableDeferredToTemplate confirms that the
// maxUnavailable >= size warning is suppressed when spec.size is 0 and a
// templateRef is set. Without the guard every templateRef-backed cluster with
// maxUnavailable set produces a spurious "PodDisruptionBudget will not prevent
// full disruption" warning because spec.Size is 0 at admission time.
func TestValidate_MaxUnavailableDeferredToTemplate(t *testing.T) {
	v := &AerospikeClusterValidator{}
	mu := intstr.FromInt32(1)
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:           0,
			TemplateRef:    &TemplateRef{Name: "ce-template"},
			MaxUnavailable: &mu,
		},
	}

	warnings, err := v.validate(cluster)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, w := range warnings {
		if strings.Contains(w, "maxUnavailable") {
			t.Errorf("maxUnavailable warning should be deferred to template resolution, got: %v", w)
		}
	}
}

// TestValidate_MaxUnavailableStructural locks in the fail-fast structural
// validation for spec.maxUnavailable. Before this check the webhook only WARNED
// (for >=size / >=100%) and let negative ints, non-percentage strings, and
// out-of-range percentages through, so K8s rejected them only at PDB apply time
// as an opaque reconcilePDB error (which can trip the circuit breaker). These
// must now be hard ERROR-level admission failures, while structurally-valid
// values still admit cleanly.
func TestValidate_MaxUnavailableStructural(t *testing.T) {
	negative := intstr.FromInt(-1)
	overPercent := intstr.FromString("150%")
	nonNumeric := intstr.FromString("abc")
	validPercent := intstr.FromString("25%")
	validInt := intstr.FromInt(1)

	tests := []struct {
		name    string
		mu      *intstr.IntOrString
		wantErr bool
	}{
		{"negative int rejected", &negative, true},
		{"percentage over 100 rejected", &overPercent, true},
		{"non-percentage string rejected", &nonNumeric, true},
		{"valid percentage admitted", &validPercent, false},
		{"valid int admitted", &validInt, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &AerospikeClusterValidator{}
			cluster := &AerospikeCluster{
				Spec: AerospikeClusterSpec{
					Size:           8,
					Image:          "aerospike:ce-8.1.1.1",
					MaxUnavailable: tt.mu,
				},
			}

			_, err := v.validate(cluster)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected validation error for maxUnavailable=%v, got nil", *tt.mu)
				}
				if !strings.Contains(err.Error(), "maxUnavailable") {
					t.Errorf("error should mention maxUnavailable, got: %v", err)
				}
			} else if err != nil {
				t.Fatalf("expected no error for maxUnavailable=%v, got: %v", *tt.mu, err)
			}
		})
	}
}

// --- ServiceMonitor / Monitoring consistency tests ---

func TestValidate_ServiceMonitorEnabledWithoutMonitoring(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			Monitoring: &AerospikeMonitoringSpec{
				Enabled: false,
				ServiceMonitor: &ServiceMonitorSpec{
					Enabled: true,
				},
			},
		},
	}

	warnings, err := v.validate(cluster)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, w := range warnings {
		if strings.Contains(w, "serviceMonitor") && strings.Contains(w, "monitoring.enabled is false") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected warning about serviceMonitor with monitoring disabled, got warnings: %v", warnings)
	}
}

func TestValidate_PrometheusRuleEnabledWithoutMonitoring(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			Monitoring: &AerospikeMonitoringSpec{
				Enabled: false,
				PrometheusRule: &PrometheusRuleSpec{
					Enabled: true,
				},
			},
		},
	}

	warnings, err := v.validate(cluster)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, w := range warnings {
		if strings.Contains(w, "prometheusRule") && strings.Contains(w, "monitoring.enabled is false") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected warning about prometheusRule with monitoring disabled, got warnings: %v", warnings)
	}
}

func TestValidate_MonitoringDisabledSubfeaturesDisabled(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			Monitoring: &AerospikeMonitoringSpec{
				Enabled: false,
				ServiceMonitor: &ServiceMonitorSpec{
					Enabled: false,
				},
			},
		},
	}

	warnings, err := v.validate(cluster)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, w := range warnings {
		if strings.Contains(w, "serviceMonitor") || strings.Contains(w, "prometheusRule") {
			t.Errorf("unexpected monitoring warning: %v", w)
		}
	}
}

// --- Empty privilege string validation tests ---

func TestValidate_ACLEmptyPrivilegeStringRejected(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeAccessControl: &AerospikeAccessControlSpec{
				Roles: []AerospikeRoleSpec{
					{Name: "custom-role", Privileges: []string{"read", "", "write"}},
				},
				Users: []AerospikeUserSpec{
					{Name: "admin", SecretName: "admin-secret", Roles: []string{"sys-admin", "user-admin"}},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Fatal("expected error for empty privilege string")
	}
	if !strings.Contains(err.Error(), "must not be empty or whitespace-only") {
		t.Errorf("expected 'must not be empty or whitespace-only' error, got: %v", err)
	}
}

func TestValidate_ACLWhitespacePrivilegeStringRejected(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeAccessControl: &AerospikeAccessControlSpec{
				Roles: []AerospikeRoleSpec{
					{Name: "custom-role", Privileges: []string{"  "}},
				},
				Users: []AerospikeUserSpec{
					{Name: "admin", SecretName: "admin-secret", Roles: []string{"sys-admin", "user-admin"}},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Fatal("expected error for whitespace-only privilege string")
	}
	if !strings.Contains(err.Error(), "must not be empty or whitespace-only") {
		t.Errorf("expected 'must not be empty or whitespace-only' error, got: %v", err)
	}
}

func TestValidate_ACLLeadingTrailingWhitespacePrivilegeRejected(t *testing.T) {
	v := &AerospikeClusterValidator{}
	tests := []struct {
		name    string
		privStr string
	}{
		{"leading space", " read-write"},
		{"trailing space", "read-write "},
		{"both sides", " read-write "},
		{"tab prefix", "\twrite"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cluster := &AerospikeCluster{
				Spec: AerospikeClusterSpec{
					Size:  3,
					Image: "aerospike:ce-8.1.1.1",
					AerospikeAccessControl: &AerospikeAccessControlSpec{
						Roles: []AerospikeRoleSpec{
							{Name: "custom-role", Privileges: []string{tc.privStr}},
						},
						Users: []AerospikeUserSpec{
							{Name: "admin", SecretName: "admin-secret", Roles: []string{"sys-admin", "user-admin"}},
						},
					},
				},
			}
			_, err := v.validate(cluster)
			if err == nil {
				t.Fatalf("expected error for privilege %q with surrounding whitespace", tc.privStr)
			}
			if !strings.Contains(err.Error(), "must not have leading or trailing whitespace") {
				t.Errorf("expected 'must not have leading or trailing whitespace' error, got: %v", err)
			}
		})
	}
}

func TestValidate_ACLUserWithEmptyRoleName(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeAccessControl: &AerospikeAccessControlSpec{
				Users: []AerospikeUserSpec{
					{
						Name:       "admin",
						SecretName: "admin-secret",
						Roles:      []string{"sys-admin", "user-admin"},
					},
					{
						Name:       "app-user",
						SecretName: "app-secret",
						Roles:      []string{"read", ""},
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Fatal("expected error for user with empty role name")
	}
	if !strings.Contains(err.Error(), "must not be empty") {
		t.Errorf("expected 'must not be empty' error, got: %v", err)
	}
	if !strings.Contains(err.Error(), `"app-user"`) {
		t.Errorf("expected error to name the offending user 'app-user', got: %v", err)
	}
	if !strings.Contains(err.Error(), "roles[1]") {
		t.Errorf("expected error to name the offending index roles[1], got: %v", err)
	}
}

func TestValidate_ACLEmptyRoleNameDoesNotAlsoTriggerUndefinedRole(t *testing.T) {
	// An empty-string role entry should be flagged once as "must not be empty"
	// and NOT also flagged as "references undefined role" — the continue in the
	// validator should skip the defined-role check for that entry.
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeAccessControl: &AerospikeAccessControlSpec{
				Users: []AerospikeUserSpec{
					{
						Name:       "admin",
						SecretName: "admin-secret",
						Roles:      []string{"sys-admin", "user-admin"},
					},
					{
						Name:       "app-user",
						SecretName: "app-secret",
						Roles:      []string{""},
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Fatal("expected error for user with empty role name")
	}
	msg := err.Error()
	if !strings.Contains(msg, "must not be empty") {
		t.Errorf("expected 'must not be empty' error, got: %v", err)
	}
	// The undefined-role check below the empty-string guard should be skipped
	// via continue, so we must NOT see the undefined-role message for this user.
	if strings.Contains(msg, `user "app-user" references undefined role ""`) {
		t.Errorf("empty role name should not also trigger 'references undefined role' for the same entry, got: %v", err)
	}
}

// --- Overrides without TemplateRef validation tests ---

func TestValidate_OverridesWithoutTemplateRefRejected(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:      3,
			Image:     "aerospike:ce-8.1.1.1",
			Overrides: &AerospikeClusterTemplateSpec{},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Fatal("expected error when overrides is set without templateRef")
	}
	if !strings.Contains(err.Error(), "spec.overrides can only be set when spec.templateRef is specified") {
		t.Errorf("expected overrides/templateRef error, got: %v", err)
	}
}

func TestValidate_OverridesWithTemplateRefOK(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:        3,
			Image:       "aerospike:ce-8.1.1.1",
			TemplateRef: &TemplateRef{Name: "my-template"},
			Overrides:   &AerospikeClusterTemplateSpec{},
		},
	}

	_, err := v.validate(cluster)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- Operation phase update edge case tests ---

func TestValidateUpdate_RejectsAddingOperationWhileInProgress(t *testing.T) {
	v := &AerospikeClusterValidator{}
	oldCluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			// Old operations already cleared from spec
			Operations: []OperationSpec{},
		},
		Status: AerospikeClusterStatus{
			OperationStatus: &OperationStatus{
				ID:    "op-1",
				Kind:  OperationPodRestart,
				Phase: AerospikePhaseInProgress,
			},
		},
	}
	newCluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			// Attempt to add new operation while another is in progress
			Operations: []OperationSpec{
				{Kind: OperationWarmRestart, ID: "op-2"},
			},
		},
		Status: AerospikeClusterStatus{
			OperationStatus: &OperationStatus{
				ID:    "op-1",
				Kind:  OperationPodRestart,
				Phase: AerospikePhaseInProgress,
			},
		},
	}

	_, err := v.ValidateUpdate(context.Background(), oldCluster, newCluster)
	if err == nil {
		t.Fatal("expected error when adding operation while another is InProgress")
	}
	if !strings.Contains(err.Error(), "cannot change operations") {
		t.Errorf("expected 'cannot change operations' error, got: %v", err)
	}
}

func TestValidateUpdate_RejectsOperationKindChangeWhileInProgress(t *testing.T) {
	v := &AerospikeClusterValidator{}
	oldCluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			Operations: []OperationSpec{
				{Kind: OperationPodRestart, ID: "op-1"},
			},
		},
		Status: AerospikeClusterStatus{
			OperationStatus: &OperationStatus{
				ID:    "op-1",
				Kind:  OperationPodRestart,
				Phase: AerospikePhaseInProgress,
			},
		},
	}
	newCluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			Operations: []OperationSpec{
				// Same ID but different Kind
				{Kind: OperationWarmRestart, ID: "op-1"},
			},
		},
		Status: AerospikeClusterStatus{
			OperationStatus: &OperationStatus{
				ID:    "op-1",
				Kind:  OperationPodRestart,
				Phase: AerospikePhaseInProgress,
			},
		},
	}

	_, err := v.ValidateUpdate(context.Background(), oldCluster, newCluster)
	if err == nil {
		t.Fatal("expected error when changing operation kind while InProgress")
	}
	if !strings.Contains(err.Error(), "cannot change operations") {
		t.Errorf("expected 'cannot change operations' error, got: %v", err)
	}
}

// --- Scoped privilege validation tests ---

func TestValidate_ACLScopedPrivilegeAccepted(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeAccessControl: &AerospikeAccessControlSpec{
				Roles: []AerospikeRoleSpec{
					{Name: "ns-reader", Privileges: []string{"read.myns", "write.myns.myset"}},
				},
				Users: []AerospikeUserSpec{
					{Name: "admin", SecretName: "admin-secret", Roles: []string{"sys-admin", "user-admin"}},
					{Name: "reader", SecretName: "reader-secret", Roles: []string{"ns-reader"}},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err != nil {
		t.Fatalf("expected scoped privileges to be accepted, got: %v", err)
	}
}

func TestValidate_ACLGlobalPrivilegeScopedRejected(t *testing.T) {
	v := &AerospikeClusterValidator{}
	for _, code := range []string{"sys-admin", "user-admin", "data-admin"} {
		t.Run(code, func(t *testing.T) {
			cluster := &AerospikeCluster{
				Spec: AerospikeClusterSpec{
					Size:  3,
					Image: "aerospike:ce-8.1.1.1",
					AerospikeAccessControl: &AerospikeAccessControlSpec{
						Roles: []AerospikeRoleSpec{
							{Name: "bad-role", Privileges: []string{code + ".myns"}},
						},
						Users: []AerospikeUserSpec{
							{Name: "admin", SecretName: "admin-secret", Roles: []string{"sys-admin", "user-admin"}},
						},
					},
				},
			}

			_, err := v.validate(cluster)
			if err == nil {
				t.Fatalf("expected error for scoped global-only privilege %q", code+".myns")
			}
			if !strings.Contains(err.Error(), "global-only privilege") {
				t.Errorf("error should mention 'global-only privilege', got: %v", err)
			}
		})
	}
}

func TestValidate_ACLGlobalPrivilegeSetScopedRejected(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeAccessControl: &AerospikeAccessControlSpec{
				Roles: []AerospikeRoleSpec{
					{Name: "bad-role", Privileges: []string{"sys-admin.myns.myset"}},
				},
				Users: []AerospikeUserSpec{
					{Name: "admin", SecretName: "admin-secret", Roles: []string{"sys-admin", "user-admin"}},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Fatal("expected error for set-scoped global-only privilege 'sys-admin.myns.myset'")
	}
	if !strings.Contains(err.Error(), "global-only privilege") {
		t.Errorf("error should mention 'global-only privilege', got: %v", err)
	}
}

func TestValidate_ACLGlobalPrivilegeUnscopedAccepted(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeAccessControl: &AerospikeAccessControlSpec{
				Roles: []AerospikeRoleSpec{
					// Unscoped admin privileges are valid; truncate is scopable.
					{Name: "ops", Privileges: []string{"sys-admin", "user-admin", "data-admin", "truncate.myns"}},
				},
				Users: []AerospikeUserSpec{
					{Name: "admin", SecretName: "admin-secret", Roles: []string{"sys-admin", "user-admin"}},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err != nil {
		t.Fatalf("expected unscoped admin privileges and scoped truncate to be accepted, got: %v", err)
	}
}

// TestValidate_ACLScopedPrivilegeMalformedRejected covers scopable privileges
// whose namespace/set scope is structurally invalid. Aerospike rejects these at
// role-sync time (leaving the cluster in ACLSyncError), so the webhook must
// reject them at admission with an actionable message instead.
func TestValidate_ACLScopedPrivilegeMalformedRejected(t *testing.T) {
	cases := []struct {
		name    string
		priv    string
		wantMsg string
	}{
		{"trailing dot empty namespace", "read.", "namespace scope must not be empty"},
		{"empty namespace before set", "read..myset", "namespace scope must not be empty"},
		{"trailing dot empty set", "read.myns.", "set scope must not be empty"},
		{"too many scope components", "read.myns.myset.extra", "scope must be"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := &AerospikeClusterValidator{}
			cluster := &AerospikeCluster{
				Spec: AerospikeClusterSpec{
					Size:  3,
					Image: "aerospike:ce-8.1.1.1",
					AerospikeAccessControl: &AerospikeAccessControlSpec{
						Roles: []AerospikeRoleSpec{
							{Name: "bad-role", Privileges: []string{tc.priv}},
						},
						Users: []AerospikeUserSpec{
							{Name: "admin", SecretName: "admin-secret", Roles: []string{"sys-admin", "user-admin"}},
						},
					},
				},
			}

			_, err := v.validate(cluster)
			if err == nil {
				t.Fatalf("expected error for malformed scoped privilege %q", tc.priv)
			}
			if !strings.Contains(err.Error(), tc.wantMsg) {
				t.Errorf("error should mention %q, got: %v", tc.wantMsg, err)
			}
		})
	}
}

// TestValidate_ACLScopedSetPrivilegeAccepted confirms a well-formed
// namespace.set scope on a scopable privilege is still accepted (guards against
// the new structural check over-rejecting valid input).
func TestValidate_ACLScopedSetPrivilegeAccepted(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeAccessControl: &AerospikeAccessControlSpec{
				Roles: []AerospikeRoleSpec{
					{Name: "set-reader", Privileges: []string{"read-write-udf.myns.myset"}},
				},
				Users: []AerospikeUserSpec{
					{Name: "admin", SecretName: "admin-secret", Roles: []string{"sys-admin", "user-admin"}},
				},
			},
		},
	}

	if _, err := v.validate(cluster); err != nil {
		t.Fatalf("expected well-formed namespace.set scope to be accepted, got: %v", err)
	}
}

// --- Defaulting idempotency test ---

func TestDefault_IsIdempotent(t *testing.T) {
	d := &AerospikeClusterDefaulter{}
	cluster := &AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			PodSpec: &AerospikePodSpec{
				HostNetwork: true,
			},
			Monitoring: &AerospikeMonitoringSpec{
				Enabled: true,
				ServiceMonitor: &ServiceMonitorSpec{
					Enabled: true,
				},
			},
		},
	}

	// Apply defaults twice
	if err := d.Default(context.Background(), cluster); err != nil {
		t.Fatalf("first default: %v", err)
	}

	// Capture state after first default
	firstImage := cluster.Spec.Monitoring.ExporterImage
	firstPort := cluster.Spec.Monitoring.Port
	firstInterval := cluster.Spec.Monitoring.ServiceMonitor.Interval
	firstDNS := cluster.Spec.PodSpec.DNSPolicy

	if err := d.Default(context.Background(), cluster); err != nil {
		t.Fatalf("second default: %v", err)
	}

	// Verify nothing changed
	if cluster.Spec.Monitoring.ExporterImage != firstImage {
		t.Errorf("ExporterImage changed after second default: %q vs %q", firstImage, cluster.Spec.Monitoring.ExporterImage)
	}
	if cluster.Spec.Monitoring.Port != firstPort {
		t.Errorf("Port changed after second default: %d vs %d", firstPort, cluster.Spec.Monitoring.Port)
	}
	if cluster.Spec.Monitoring.ServiceMonitor.Interval != firstInterval {
		t.Errorf("Interval changed after second default: %q vs %q", firstInterval, cluster.Spec.Monitoring.ServiceMonitor.Interval)
	}
	if cluster.Spec.PodSpec.DNSPolicy != firstDNS {
		t.Errorf("DNSPolicy changed after second default: %q vs %q", firstDNS, cluster.Spec.PodSpec.DNSPolicy)
	}
}

// --- Image tag validation edge cases ---

func TestValidate_ImageWithDigestAccepted(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  1,
			Image: "aerospike:ce-8.1.1.1@sha256:abc123",
		},
	}

	_, err := v.validate(cluster)
	if err != nil {
		t.Fatalf("unexpected error for image with digest: %v", err)
	}
}

// TestValidate_DigestPinnedGuardsStillFire verifies that the CE-version and
// enterprise-edition guards are NOT bypassed by a "@sha256:..." digest suffix.
// Before the digest was stripped in imageTag, the digest's colon was misread as
// the tag separator, parseMajorVersion/isEnterpriseTag failed, and these images
// slipped through.
func TestValidate_DigestPinnedGuardsStillFire(t *testing.T) {
	tests := []struct {
		name    string
		image   string
		wantSub string // expected substring in the rejection error
	}{
		{name: "ce-7 digest-pinned rejected", image: "aerospike:ce-7.0.0.0@sha256:abc123", wantSub: "requires Aerospike CE"},
		{name: "ee digest-pinned rejected", image: "aerospike:ee-8.0.0.1@sha256:abc123", wantSub: "Enterprise Edition"},
		{name: "ported registry ce-7 digest rejected", image: "myregistry.io:5000/aerospike:ce-7.0.0.0@sha256:abc123", wantSub: "requires Aerospike CE"},
	}

	v := &AerospikeClusterValidator{}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cluster := &AerospikeCluster{
				Spec: AerospikeClusterSpec{Size: 1, Image: tc.image},
			}
			_, err := v.validate(cluster)
			if err == nil {
				t.Fatalf("validate() with image %q: expected error, got none", tc.image)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("validate() with image %q: expected error containing %q, got: %v", tc.image, tc.wantSub, err)
			}
		})
	}
}

func TestValidate_ImageEnterpriseEETag(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  1,
			Image: "aerospike:ee-8.0.0.1",
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Fatal("expected error for enterprise ee- tag image")
	}
	if !strings.Contains(err.Error(), "Enterprise Edition") {
		t.Errorf("expected enterprise image error, got: %v", err)
	}
}

func TestValidate_DuplicateUserNames(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeAccessControl: &AerospikeAccessControlSpec{
				Users: []AerospikeUserSpec{
					{
						Name:       "admin",
						SecretName: "admin-secret",
						Roles:      []string{"sys-admin", "user-admin"},
					},
					{
						Name:       "admin",
						SecretName: "admin-secret-2",
						Roles:      []string{"read"},
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Fatal("expected error for duplicate user names")
	}
	if !strings.Contains(err.Error(), "duplicate user name") {
		t.Errorf("error should mention 'duplicate user name', got: %v", err)
	}
}

func TestValidate_DuplicateRoleNames(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeAccessControl: &AerospikeAccessControlSpec{
				Users: []AerospikeUserSpec{
					{
						Name:       "admin",
						SecretName: "admin-secret",
						Roles:      []string{"sys-admin", "user-admin"},
					},
				},
				Roles: []AerospikeRoleSpec{
					{
						Name:       "custom-role",
						Privileges: []string{"read"},
					},
					{
						Name:       "custom-role",
						Privileges: []string{"write"},
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Fatal("expected error for duplicate role names")
	}
	if !strings.Contains(err.Error(), "duplicate role name") {
		t.Errorf("error should mention 'duplicate role name', got: %v", err)
	}
}

func TestValidate_AccessControl_EmptyRoleName(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeAccessControl: &AerospikeAccessControlSpec{
				Users: []AerospikeUserSpec{
					{
						Name:       "admin",
						SecretName: "admin-secret",
						Roles:      []string{"sys-admin", "user-admin"},
					},
				},
				Roles: []AerospikeRoleSpec{
					{
						Name:       "custom-role",
						Privileges: []string{"read"},
					},
					{
						Name:       "",
						Privileges: []string{"write"},
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Fatal("expected error for empty role name in accessControl.roles")
	}
	if !strings.Contains(err.Error(), "accessControl.roles[1]: role name must not be empty") {
		t.Errorf("error should mention 'accessControl.roles[1]: role name must not be empty', got: %v", err)
	}
	// Empty entry must be skipped (via continue) so it does not collide with a
	// later empty entry as a "duplicate role name" — there is no defined empty
	// role on the cluster.
	if strings.Contains(err.Error(), `duplicate role name ""`) {
		t.Errorf("empty role name should not also trigger 'duplicate role name', got: %v", err)
	}
}

func TestValidate_UniqueUserNames(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeAccessControl: &AerospikeAccessControlSpec{
				Users: []AerospikeUserSpec{
					{
						Name:       "admin",
						SecretName: "admin-secret",
						Roles:      []string{"sys-admin", "user-admin"},
					},
					{
						Name:       "reader",
						SecretName: "reader-secret",
						Roles:      []string{"read"},
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err != nil {
		t.Errorf("unexpected error for unique user names: %v", err)
	}
}

func TestValidate_MonitoringPortOutOfRange_Negative(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			Monitoring: &AerospikeMonitoringSpec{
				Enabled:       true,
				ExporterImage: "exporter:v1",
				Port:          -1,
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Fatal("expected error for negative monitoring port")
	}
	if !strings.Contains(err.Error(), "must be in range 1-65535") {
		t.Errorf("error should mention port range, got: %v", err)
	}
}

func TestValidate_MonitoringPortOutOfRange_Zero(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			Monitoring: &AerospikeMonitoringSpec{
				Enabled:       true,
				ExporterImage: "exporter:v1",
				Port:          0,
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Fatal("expected error for zero monitoring port")
	}
	if !strings.Contains(err.Error(), "must be in range 1-65535") {
		t.Errorf("error should mention port range, got: %v", err)
	}
}

func TestValidate_MonitoringPortOutOfRange_TooHigh(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			Monitoring: &AerospikeMonitoringSpec{
				Enabled:       true,
				ExporterImage: "exporter:v1",
				Port:          70000,
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Fatal("expected error for port > 65535")
	}
	if !strings.Contains(err.Error(), "must be in range 1-65535") {
		t.Errorf("error should mention port range, got: %v", err)
	}
}

// --- Rack config IntOrString validation tests ---

func TestValidate_RackConfig_ScaleDownBatchSize_Valid(t *testing.T) {
	v := &AerospikeClusterValidator{}
	bs := intstr.FromInt32(2)
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			RackConfig: &RackConfig{
				Racks:              []Rack{{ID: 1}},
				ScaleDownBatchSize: &bs,
			},
		},
	}

	_, err := v.validate(cluster)
	if err != nil {
		t.Errorf("expected no error for valid ScaleDownBatchSize, got: %v", err)
	}
}

func TestValidate_RackConfig_ScaleDownBatchSize_Zero(t *testing.T) {
	v := &AerospikeClusterValidator{}
	bs := intstr.FromInt32(0)
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			RackConfig: &RackConfig{
				Racks:              []Rack{{ID: 1}},
				ScaleDownBatchSize: &bs,
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Error("expected error for zero ScaleDownBatchSize")
	}
	if !strings.Contains(err.Error(), "scaleDownBatchSize") {
		t.Errorf("error should mention scaleDownBatchSize, got: %v", err)
	}
}

func TestValidate_RackConfig_ScaleDownBatchSize_ValidPercentage(t *testing.T) {
	v := &AerospikeClusterValidator{}
	bs := intstr.FromString("50%")
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			RackConfig: &RackConfig{
				Racks:              []Rack{{ID: 1}},
				ScaleDownBatchSize: &bs,
			},
		},
	}

	_, err := v.validate(cluster)
	if err != nil {
		t.Errorf("expected no error for valid percentage ScaleDownBatchSize, got: %v", err)
	}
}

func TestValidate_RackConfig_MaxIgnorablePods_Zero(t *testing.T) {
	v := &AerospikeClusterValidator{}
	mp := intstr.FromInt32(0)
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			RackConfig: &RackConfig{
				Racks:            []Rack{{ID: 1}},
				MaxIgnorablePods: &mp,
			},
		},
	}

	_, err := v.validate(cluster)
	if err != nil {
		t.Errorf("expected no error for zero MaxIgnorablePods (min=0), got: %v", err)
	}
}

func TestValidate_RackConfig_MaxIgnorablePods_Negative(t *testing.T) {
	v := &AerospikeClusterValidator{}
	mp := intstr.FromInt32(-1)
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			RackConfig: &RackConfig{
				Racks:            []Rack{{ID: 1}},
				MaxIgnorablePods: &mp,
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Error("expected error for negative MaxIgnorablePods")
	}
	if !strings.Contains(err.Error(), "maxIgnorablePods") {
		t.Errorf("error should mention maxIgnorablePods, got: %v", err)
	}
}

func TestValidate_RackConfig_RollingUpdateBatchSize_Valid(t *testing.T) {
	v := &AerospikeClusterValidator{}
	bs := intstr.FromInt32(3)
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			RackConfig: &RackConfig{
				Racks:                  []Rack{{ID: 1}},
				RollingUpdateBatchSize: &bs,
			},
		},
	}

	_, err := v.validate(cluster)
	if err != nil {
		t.Errorf("expected no error for valid RollingUpdateBatchSize, got: %v", err)
	}
}

func TestValidate_RackConfig_RollingUpdateBatchSize_Zero(t *testing.T) {
	v := &AerospikeClusterValidator{}
	bs := intstr.FromInt32(0)
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			RackConfig: &RackConfig{
				Racks:                  []Rack{{ID: 1}},
				RollingUpdateBatchSize: &bs,
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Error("expected error for zero RollingUpdateBatchSize")
	}
	if !strings.Contains(err.Error(), "rollingUpdateBatchSize") {
		t.Errorf("error should mention rollingUpdateBatchSize, got: %v", err)
	}
}

func TestValidate_RackConfig_RollingUpdateBatchSize_ValidPercentage(t *testing.T) {
	v := &AerospikeClusterValidator{}
	bs := intstr.FromString("25%")
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			RackConfig: &RackConfig{
				Racks:                  []Rack{{ID: 1}},
				RollingUpdateBatchSize: &bs,
			},
		},
	}

	_, err := v.validate(cluster)
	if err != nil {
		t.Errorf("expected no error for valid percentage RollingUpdateBatchSize, got: %v", err)
	}
}

func TestValidate_CE7ImageRejected(t *testing.T) {
	v := &AerospikeClusterValidator{}

	tests := []struct {
		name  string
		image string
	}{
		{"ce-7 tag", "aerospike:ce-7.2.0.6"},
		{"ce-7.0 tag", "aerospike:ce-7.0.0.0"},
		// CE 7.x image pulled through a ported registry: the host:port colon must
		// not be mistaken for the tag separator, otherwise parseMajorVersion fails
		// and the minimum-version gate is silently skipped.
		{"ported registry ce-7", "myregistry.io:5000/aerospike:ce-7.0.0.0"},
		{"ported registry ce-7 localhost", "localhost:32000/aerospike:ce-7.2.0.6"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cluster := &AerospikeCluster{
				Spec: AerospikeClusterSpec{
					Size:  1,
					Image: tc.image,
				},
			}
			_, err := v.validate(cluster)
			if err == nil {
				t.Fatalf("expected error for CE 7 image %q", tc.image)
			}
			if !strings.Contains(err.Error(), "CE 7.x is no longer supported") {
				t.Errorf("expected CE 7 rejection message, got: %v", err)
			}
		})
	}
}

// --- Replication factor zero/negative-zero validation tests ---

func TestValidate_ReplicationFactorNegativeZeroFloat(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeConfig: &AerospikeConfigSpec{
				Value: map[string]any{
					"namespaces": []any{
						map[string]any{
							"name":               "test",
							"replication-factor": float64(math.Copysign(0, -1)),
						},
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Fatal("expected error when replication-factor is float64(-0.0)")
	}
	if !strings.Contains(err.Error(), "replication-factor must be >= 1") {
		t.Errorf("error should mention 'replication-factor must be >= 1', got: %v", err)
	}
}

func TestValidate_ReplicationFactorIntZero(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeConfig: &AerospikeConfigSpec{
				Value: map[string]any{
					"namespaces": []any{
						map[string]any{
							"name":               "test",
							"replication-factor": int(0),
						},
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Fatal("expected error when replication-factor is int(0)")
	}
	if !strings.Contains(err.Error(), "replication-factor must be >= 1") {
		t.Errorf("error should mention 'replication-factor must be >= 1', got: %v", err)
	}
}

func TestValidate_ReplicationFactorFloatZero(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeConfig: &AerospikeConfigSpec{
				Value: map[string]any{
					"namespaces": []any{
						map[string]any{
							"name":               "test",
							"replication-factor": float64(0.0),
						},
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Fatal("expected error when replication-factor is float64(0.0)")
	}
	if !strings.Contains(err.Error(), "replication-factor must be >= 1") {
		t.Errorf("error should mention 'replication-factor must be >= 1', got: %v", err)
	}
}

func TestValidate_ReplicationFactorIntOneAccepted(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeConfig: &AerospikeConfigSpec{
				Value: map[string]any{
					"namespaces": []any{
						map[string]any{
							"name":               "test",
							"replication-factor": int(1),
						},
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err != nil {
		t.Errorf("unexpected error for replication-factor int(1): %v", err)
	}
}

// --- Rack ID add+remove detection tests ---

func TestValidateUpdate_RackRemoveAndAddDifferentCount_Increase(t *testing.T) {
	v := &AerospikeClusterValidator{}
	oldCluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  4,
			Image: "aerospike:ce-8.1.1.1",
			RackConfig: &RackConfig{
				Racks: []Rack{
					{ID: 1},
					{ID: 2},
				},
			},
		},
	}
	// Remove rack 1, add rack 2 and 3 (count changes 2→3)
	newCluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  6,
			Image: "aerospike:ce-8.1.1.1",
			RackConfig: &RackConfig{
				Racks: []Rack{
					{ID: 2},
					{ID: 3},
					{ID: 4},
				},
			},
		},
	}

	_, err := v.ValidateUpdate(context.Background(), oldCluster, newCluster)
	if err == nil {
		t.Fatal("expected error when adding and removing rack IDs simultaneously (count increase)")
	}
	if !strings.Contains(err.Error(), "cannot add new rack IDs") {
		t.Errorf("error should mention 'cannot add new rack IDs', got: %v", err)
	}
}

func TestValidateUpdate_RackRemoveAndAddDifferentCount_Decrease(t *testing.T) {
	v := &AerospikeClusterValidator{}
	oldCluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  6,
			Image: "aerospike:ce-8.1.1.1",
			RackConfig: &RackConfig{
				Racks: []Rack{
					{ID: 1},
					{ID: 2},
					{ID: 3},
				},
			},
		},
	}
	// Remove rack 1 and 2, add rack 4 (count changes 3→2)
	newCluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  4,
			Image: "aerospike:ce-8.1.1.1",
			RackConfig: &RackConfig{
				Racks: []Rack{
					{ID: 3},
					{ID: 4},
				},
			},
		},
	}

	_, err := v.ValidateUpdate(context.Background(), oldCluster, newCluster)
	if err == nil {
		t.Fatal("expected error when adding and removing rack IDs simultaneously (count decrease)")
	}
	if !strings.Contains(err.Error(), "cannot add new rack IDs") {
		t.Errorf("error should mention 'cannot add new rack IDs', got: %v", err)
	}
}

func TestValidateUpdate_RackOnlyAddOK(t *testing.T) {
	v := &AerospikeClusterValidator{}
	oldCluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  4,
			Image: "aerospike:ce-8.1.1.1",
			RackConfig: &RackConfig{
				Racks: []Rack{
					{ID: 1},
				},
			},
		},
	}
	newCluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  6,
			Image: "aerospike:ce-8.1.1.1",
			RackConfig: &RackConfig{
				Racks: []Rack{
					{ID: 1},
					{ID: 2},
					{ID: 3},
				},
			},
		},
	}

	_, err := v.ValidateUpdate(context.Background(), oldCluster, newCluster)
	if err != nil {
		t.Errorf("unexpected error for rack-only addition: %v", err)
	}
}

func TestValidateUpdate_RackOnlyRemoveOK(t *testing.T) {
	v := &AerospikeClusterValidator{}
	oldCluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  6,
			Image: "aerospike:ce-8.1.1.1",
			RackConfig: &RackConfig{
				Racks: []Rack{
					{ID: 1},
					{ID: 2},
					{ID: 3},
				},
			},
		},
	}
	newCluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  4,
			Image: "aerospike:ce-8.1.1.1",
			RackConfig: &RackConfig{
				Racks: []Rack{
					{ID: 1},
				},
			},
		},
	}

	_, err := v.ValidateUpdate(context.Background(), oldCluster, newCluster)
	if err != nil {
		t.Errorf("unexpected error for rack-only removal: %v", err)
	}
}

// Dropping rackConfig entirely from a cluster that had explicit racks is a
// rename-like operation: the controller falls back to the default rack (ID 0),
// tearing down every explicit-rack StatefulSet and creating a fresh rack-0 one.
// It must be rejected just like an in-spec simultaneous add+remove.
func TestValidateUpdate_RackConfigDroppedRejected(t *testing.T) {
	v := &AerospikeClusterValidator{}
	oldCluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  4,
			Image: "aerospike:ce-8.1.1.1",
			RackConfig: &RackConfig{
				Racks: []Rack{
					{ID: 1},
					{ID: 2},
				},
			},
		},
	}
	// rackConfig removed entirely -> effective rack set becomes {0}.
	newCluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  4,
			Image: "aerospike:ce-8.1.1.1",
		},
	}

	_, err := v.ValidateUpdate(context.Background(), oldCluster, newCluster)
	if err == nil {
		t.Fatal("expected error when dropping rackConfig from a cluster with explicit racks")
	}
	if !strings.Contains(err.Error(), "cannot add new rack IDs") {
		t.Errorf("error should mention 'cannot add new rack IDs', got: %v", err)
	}
	// The implicit default rack 0 is the added ID; explicit racks are removed.
	if !strings.Contains(err.Error(), "[0]") {
		t.Errorf("error should name the implicit default rack ID 0, got: %v", err)
	}
	if !strings.Contains(err.Error(), "[1 2]") {
		t.Errorf("error should name the removed rack IDs [1 2] in sorted order, got: %v", err)
	}
	// When the default rack (ID 0) is involved there is no valid two-step path, so
	// the message must steer toward data migration rather than an impossible add/remove.
	if !strings.Contains(err.Error(), "migrating data") {
		t.Errorf("error should guide the user to create a new cluster and migrate data, got: %v", err)
	}
}

// An empty Racks slice resolves to the same default rack (ID 0) as a nil
// rackConfig, so it must be rejected on the same data-loss grounds.
func TestValidateUpdate_RackConfigEmptiedRejected(t *testing.T) {
	v := &AerospikeClusterValidator{}
	oldCluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  4,
			Image: "aerospike:ce-8.1.1.1",
			RackConfig: &RackConfig{
				Racks: []Rack{{ID: 5}},
			},
		},
	}
	newCluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:       4,
			Image:      "aerospike:ce-8.1.1.1",
			RackConfig: &RackConfig{Racks: []Rack{}},
		},
	}

	_, err := v.ValidateUpdate(context.Background(), oldCluster, newCluster)
	if err == nil {
		t.Fatal("expected error when emptying rackConfig.racks for a cluster with explicit racks")
	}
	if !strings.Contains(err.Error(), "cannot add new rack IDs") {
		t.Errorf("error should mention 'cannot add new rack IDs', got: %v", err)
	}
}

// Adding a first explicit rack to a default-rack (no rackConfig) cluster swaps
// the implicit rack 0 for the new explicit rack — a rename-like transition that
// must be rejected.
func TestValidateUpdate_RackConfigAddedToDefaultRejected(t *testing.T) {
	v := &AerospikeClusterValidator{}
	oldCluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  4,
			Image: "aerospike:ce-8.1.1.1",
		},
	}
	newCluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  4,
			Image: "aerospike:ce-8.1.1.1",
			RackConfig: &RackConfig{
				Racks: []Rack{{ID: 1}},
			},
		},
	}

	_, err := v.ValidateUpdate(context.Background(), oldCluster, newCluster)
	if err == nil {
		t.Fatal("expected error when adding the first explicit rack to a default-rack cluster")
	}
	if !strings.Contains(err.Error(), "cannot add new rack IDs") {
		t.Errorf("error should mention 'cannot add new rack IDs', got: %v", err)
	}
}

// Keeping the effective rack set unchanged across the nil<->empty default-rack
// forms is a no-op for topology and must be allowed. (Rack ID 0 cannot be
// specified explicitly — validateRackConfig reserves it — so only nil and the
// empty slice are valid spellings of the default rack.)
func TestValidateUpdate_RackConfigDefaultEquivalentForms_OK(t *testing.T) {
	v := &AerospikeClusterValidator{}
	base := AerospikeClusterSpec{Size: 4, Image: "aerospike:ce-8.1.1.1"}

	cases := []struct {
		name         string
		oldRC, newRC *RackConfig
	}{
		{"nil to nil", nil, nil},
		{"nil to empty", nil, &RackConfig{Racks: []Rack{}}},
		{"empty to nil", &RackConfig{Racks: []Rack{}}, nil},
		{"empty to empty", &RackConfig{Racks: []Rack{}}, &RackConfig{Racks: []Rack{}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			oldCluster := &AerospikeCluster{Spec: base}
			oldCluster.Spec.RackConfig = tc.oldRC
			newCluster := &AerospikeCluster{Spec: base}
			newCluster.Spec.RackConfig = tc.newRC

			if _, err := v.ValidateUpdate(context.Background(), oldCluster, newCluster); err != nil {
				t.Errorf("unexpected error for topology-preserving update: %v", err)
			}
		})
	}
}

func TestValidate_CE8ImageAccepted(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  1,
			Image: "aerospike:ce-8.1.1.1",
		},
	}
	_, err := v.validate(cluster)
	if err != nil {
		t.Errorf("expected no error for CE 8 image, got: %v", err)
	}
}

// --- Network port uniqueness validation tests ---

func TestValidate_NetworkPortConflict_ServiceAndHeartbeat(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeConfig: &AerospikeConfigSpec{
				Value: map[string]any{
					"network": map[string]any{
						"service":   map[string]any{"port": 3000},
						"heartbeat": map[string]any{"port": 3000, "mode": "mesh"},
						"fabric":    map[string]any{"port": 3001},
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Fatal("expected error for port conflict, got nil")
	}
	if !strings.Contains(err.Error(), "network port conflict") {
		t.Errorf("expected 'network port conflict' error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "service.port and heartbeat.port") {
		t.Errorf("expected error to mention service and heartbeat, got: %v", err)
	}
}

func TestValidate_NetworkPortConflict_AllThreeSame(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeConfig: &AerospikeConfigSpec{
				Value: map[string]any{
					"network": map[string]any{
						"service":   map[string]any{"port": 3000},
						"heartbeat": map[string]any{"port": 3000, "mode": "mesh"},
						"fabric":    map[string]any{"port": 3000},
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Fatal("expected error for all ports same, got nil")
	}
	// Should report multiple conflicts
	errStr := err.Error()
	count := strings.Count(errStr, "network port conflict")
	if count < 2 {
		t.Errorf("expected at least 2 port conflict errors when all 3 ports are same, got %d in: %v", count, err)
	}
}

func TestValidate_NetworkPortConflict_Float64Ports(t *testing.T) {
	v := &AerospikeClusterValidator{}
	// JSON unmarshaling produces float64 for numbers
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeConfig: &AerospikeConfigSpec{
				Value: map[string]any{
					"network": map[string]any{
						"service":   map[string]any{"port": float64(3000)},
						"heartbeat": map[string]any{"port": float64(3000), "mode": "mesh"},
						"fabric":    map[string]any{"port": float64(3001)},
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Fatal("expected error for port conflict with float64 ports, got nil")
	}
	if !strings.Contains(err.Error(), "network port conflict") {
		t.Errorf("expected 'network port conflict' error, got: %v", err)
	}
}

func TestValidate_NetworkPortUniqueness_DistinctPorts(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeConfig: &AerospikeConfigSpec{
				Value: map[string]any{
					"network": map[string]any{
						"service":   map[string]any{"port": 3000},
						"heartbeat": map[string]any{"port": 3002, "mode": "mesh"},
						"fabric":    map[string]any{"port": 3001},
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err != nil {
		t.Errorf("expected no error for distinct ports, got: %v", err)
	}
}

func TestValidate_NetworkPortConflict_ServiceUsesInfoPort(t *testing.T) {
	// service.port=3003 collides with the reserved Aerospike info port,
	// even though no two of {service, heartbeat, fabric} share the same value.
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeConfig: &AerospikeConfigSpec{
				Value: map[string]any{
					"network": map[string]any{
						"service":   map[string]any{"port": 3003},
						"heartbeat": map[string]any{"port": 3002, "mode": "mesh"},
						"fabric":    map[string]any{"port": 3001},
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Fatal("expected error for service.port=3003 vs reserved info port, got nil")
	}
	if !strings.Contains(err.Error(), "conflicts with reserved port 3003") {
		t.Errorf("expected reserved-port error mentioning 3003, got: %v", err)
	}
}

func TestValidate_NetworkPort_OutOfRange(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeConfig: &AerospikeConfigSpec{
				Value: map[string]any{
					"network": map[string]any{
						"service":   map[string]any{"port": 0},
						"heartbeat": map[string]any{"port": 99999, "mode": "mesh"},
						"fabric":    map[string]any{"port": 3001},
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Fatal("expected error for out-of-range ports, got nil")
	}
	if !strings.Contains(err.Error(), "service.port=0 must be in range") {
		t.Errorf("expected range error for service.port=0, got: %v", err)
	}
	if !strings.Contains(err.Error(), "heartbeat.port=99999 must be in range") {
		t.Errorf("expected range error for heartbeat.port=99999, got: %v", err)
	}
}

func TestValidate_NetworkPort_StringTypeRejected(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeConfig: &AerospikeConfigSpec{
				Value: map[string]any{
					"network": map[string]any{
						"service":   map[string]any{"port": "3000"},
						"heartbeat": map[string]any{"port": 3002, "mode": "mesh"},
						"fabric":    map[string]any{"port": 3001},
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Fatal("expected error for string-typed port, got nil")
	}
	if !strings.Contains(err.Error(), `service.port must be an integer, got string "3000"`) {
		t.Errorf("expected explicit string-type error, got: %v", err)
	}
}

func TestValidate_NetworkPort_CustomServicePortRejected(t *testing.T) {
	// A custom service port is syntactically valid but unsupported: the operator
	// hard-codes the container port and the asinfo health probes to 3000, so a
	// cluster with service.port=4000 never reaches Ready. Admission must reject it.
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeConfig: &AerospikeConfigSpec{
				Value: map[string]any{
					"network": map[string]any{
						"service":   map[string]any{"port": 4000},
						"heartbeat": map[string]any{"port": 3002, "mode": "mesh"},
						"fabric":    map[string]any{"port": 3001},
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Fatal("expected error for custom service.port=4000, got nil")
	}
	if !strings.Contains(err.Error(), "network.service.port=4000 is not supported") {
		t.Errorf("expected unsupported-port error for service.port=4000, got: %v", err)
	}
}

func TestValidate_NetworkPort_CustomHeartbeatAndFabricPortRejected(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeConfig: &AerospikeConfigSpec{
				Value: map[string]any{
					"network": map[string]any{
						"service":   map[string]any{"port": 3000},
						"heartbeat": map[string]any{"port": 3100, "mode": "mesh"},
						"fabric":    map[string]any{"port": 3200},
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Fatal("expected error for custom heartbeat/fabric ports, got nil")
	}
	if !strings.Contains(err.Error(), "network.heartbeat.port=3100 is not supported") {
		t.Errorf("expected unsupported-port error for heartbeat.port=3100, got: %v", err)
	}
	if !strings.Contains(err.Error(), "network.fabric.port=3200 is not supported") {
		t.Errorf("expected unsupported-port error for fabric.port=3200, got: %v", err)
	}
}

func TestValidate_NetworkPort_DefaultPortsAccepted(t *testing.T) {
	// The exact fixed ports must pass cleanly — this guards against the
	// fixed-port check rejecting a correctly-configured cluster.
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeConfig: &AerospikeConfigSpec{
				Value: map[string]any{
					"network": map[string]any{
						"service":   map[string]any{"port": int(DefaultServicePort)},
						"heartbeat": map[string]any{"port": int(DefaultHeartbeatPort), "mode": "mesh"},
						"fabric":    map[string]any{"port": int(DefaultFabricPort)},
					},
				},
			},
		},
	}

	if errs := v.validateNetworkPortUniqueness(cluster); len(errs) != 0 {
		t.Errorf("expected no errors for default ports, got: %v", errs)
	}
}

func TestValidate_NetworkPortUniqueness_NoNetworkSection(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeConfig: &AerospikeConfigSpec{
				Value: map[string]any{},
			},
		},
	}

	_, err := v.validate(cluster)
	if err != nil {
		t.Errorf("expected no error when network section is absent, got: %v", err)
	}
}

// --- Rack batch size percentage warning tests ---

func TestValidate_RackBatchSize_PercentageResolvesToZero(t *testing.T) {
	v := &AerospikeClusterValidator{}
	bs := intstr.FromString("10%")
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  5,
			Image: "aerospike:ce-8.1.1.1",
			RackConfig: &RackConfig{
				Racks:                  []Rack{{ID: 1}},
				RollingUpdateBatchSize: &bs,
			},
		},
	}

	warnings, err := v.validate(cluster)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, w := range warnings {
		if strings.Contains(w, "resolves to 0 pods") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected warning about batch size resolving to 0, got warnings: %v", warnings)
	}
}

func TestValidate_RackBatchSize_PercentageResolvesToNonZero(t *testing.T) {
	v := &AerospikeClusterValidator{}
	bs := intstr.FromString("50%")
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  8,
			Image: "aerospike:ce-8.1.1.1",
			RackConfig: &RackConfig{
				Racks:                  []Rack{{ID: 1}},
				RollingUpdateBatchSize: &bs,
			},
		},
	}

	warnings, err := v.validate(cluster)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, w := range warnings {
		if strings.Contains(w, "resolves to 0 pods") {
			t.Errorf("unexpected rack batch size warning: %v", w)
		}
	}
}

func TestValidate_RackBatchSize_IntegerValue(t *testing.T) {
	v := &AerospikeClusterValidator{}
	bs := intstr.FromInt32(2)
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  5,
			Image: "aerospike:ce-8.1.1.1",
			RackConfig: &RackConfig{
				Racks:                  []Rack{{ID: 1}},
				RollingUpdateBatchSize: &bs,
			},
		},
	}

	warnings, err := v.validate(cluster)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, w := range warnings {
		if strings.Contains(w, "resolves to 0 pods") {
			t.Errorf("unexpected rack batch size warning for integer value: %v", w)
		}
	}
}

func TestValidate_RackBatchSize_NilRackConfig(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
		},
	}

	warnings, err := v.validate(cluster)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, w := range warnings {
		if strings.Contains(w, "resolves to 0 pods") {
			t.Errorf("unexpected rack batch size warning when rackConfig is nil: %v", w)
		}
	}
}

// TestValidate_RackBatchSizeDeferredToTemplate confirms that the
// rackConfig.rollingUpdateBatchSize percentage-resolves-to-0 warning is
// suppressed when spec.size is 0 and a templateRef is set. Without the guard
// every templateRef-backed cluster with a percentage
// rackConfig.rollingUpdateBatchSize produces a spurious "resolves to 0 pods
// for cluster size 0" warning because spec.Size is 0 at admission time.
func TestValidate_RackBatchSizeDeferredToTemplate(t *testing.T) {
	v := &AerospikeClusterValidator{}
	bs := intstr.FromString("50%")
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:        0,
			TemplateRef: &TemplateRef{Name: "ce-template"},
			RackConfig: &RackConfig{
				Racks:                  []Rack{{ID: 1}},
				RollingUpdateBatchSize: &bs,
			},
		},
	}

	warnings, err := v.validate(cluster)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, w := range warnings {
		if strings.Contains(w, "rackConfig.rollingUpdateBatchSize") || strings.Contains(w, "resolves to 0 pods") {
			t.Errorf("rackConfig.rollingUpdateBatchSize warning should be deferred to template resolution, got: %v", w)
		}
	}
}

// --- ServiceMonitor uniqueness webhook tests ---

// newServiceMonitorUnstructured builds an *unstructured.Unstructured shaped
// like a ServiceMonitor object that the fake client can store and the
// validator can list back via the same GVK used by the reconciler.
func newServiceMonitorUnstructured(name, namespace string, ownerRefs []metav1.OwnerReference) *unstructured.Unstructured {
	sm := &unstructured.Unstructured{}
	sm.SetGroupVersionKind(serviceMonitorGVK)
	sm.SetName(name)
	sm.SetNamespace(namespace)
	if len(ownerRefs) > 0 {
		sm.SetOwnerReferences(ownerRefs)
	}
	// Minimal spec so the fake client retains the object faithfully.
	sm.Object["spec"] = map[string]any{}
	return sm
}

// newServiceMonitorAwareClient returns a fake client that knows the
// ServiceMonitor GVK as a list kind (required for unstructured List to work
// against the fake client's tracker).
func newServiceMonitorAwareClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}

	smListGVK := serviceMonitorGVK
	smListGVK.Kind = "ServiceMonitorList"
	scheme.AddKnownTypeWithName(serviceMonitorGVK, &unstructured.Unstructured{})
	scheme.AddKnownTypeWithName(smListGVK, &unstructured.UnstructuredList{})

	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objs...).
		Build()
}

func makeMonitoringEnabledCluster(uid types.UID) *AerospikeCluster {
	return &AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo",
			Namespace: "ns-a",
			UID:       uid,
		},
		Spec: AerospikeClusterSpec{
			Size:  1,
			Image: "aerospike:ce-8.1.1.1",
			Monitoring: &AerospikeMonitoringSpec{
				Enabled: true,
				ServiceMonitor: &ServiceMonitorSpec{
					Enabled: true,
				},
			},
		},
	}
}

// TestValidateServiceMonitorUniqueness_RejectsConflictingOwner ensures that
// a pre-existing ServiceMonitor owned by a *different* AerospikeCluster causes
// the webhook to reject the new CR.
func TestValidateServiceMonitorUniqueness_RejectsConflictingOwner(t *testing.T) {
	cluster := makeMonitoringEnabledCluster("uid-new")
	smName := serviceMonitorName(cluster)

	otherOwner := metav1.OwnerReference{
		APIVersion: GroupVersion.String(),
		Kind:       "AerospikeCluster",
		Name:       "demo-other",
		UID:        "uid-other",
	}
	existing := newServiceMonitorUnstructured(smName, "ns-a", []metav1.OwnerReference{otherOwner})

	v := &AerospikeClusterValidator{Client: newServiceMonitorAwareClient(t, existing)}

	err := v.validateServiceMonitorUniqueness(context.Background(), cluster)
	if err == nil {
		t.Fatal("expected error for conflicting ServiceMonitor owner, got nil")
	}
	if !strings.Contains(err.Error(), smName) {
		t.Errorf("error should mention ServiceMonitor name %q, got: %v", smName, err)
	}
	if !strings.Contains(err.Error(), "demo-other") {
		t.Errorf("error should point at conflicting AerospikeCluster %q, got: %v", "demo-other", err)
	}
}

// TestValidateServiceMonitorUniqueness_NoOwnerRefs ensures that a
// ServiceMonitor with no owner references at all (e.g. hand-rolled by an
// admin) blocks the new CR — the operator must not silently take it over.
func TestValidateServiceMonitorUniqueness_NoOwnerRefs(t *testing.T) {
	cluster := makeMonitoringEnabledCluster("uid-new")
	smName := serviceMonitorName(cluster)
	existing := newServiceMonitorUnstructured(smName, "ns-a", nil)

	v := &AerospikeClusterValidator{Client: newServiceMonitorAwareClient(t, existing)}

	err := v.validateServiceMonitorUniqueness(context.Background(), cluster)
	if err == nil {
		t.Fatal("expected error for ServiceMonitor without matching owner ref, got nil")
	}
	if !strings.Contains(err.Error(), "no owner reference matches this AerospikeCluster") {
		t.Errorf("error should describe the missing owner-ref case, got: %v", err)
	}
}

// TestValidateServiceMonitorUniqueness_IdempotentSameOwner ensures that an
// existing ServiceMonitor whose owner UID matches *this* cluster is treated
// as ours — required to keep ValidateUpdate idempotent.
func TestValidateServiceMonitorUniqueness_IdempotentSameOwner(t *testing.T) {
	cluster := makeMonitoringEnabledCluster("uid-self")
	smName := serviceMonitorName(cluster)

	selfOwner := metav1.OwnerReference{
		APIVersion: GroupVersion.String(),
		Kind:       "AerospikeCluster",
		Name:       cluster.Name,
		UID:        cluster.UID,
	}
	existing := newServiceMonitorUnstructured(smName, "ns-a", []metav1.OwnerReference{selfOwner})

	v := &AerospikeClusterValidator{Client: newServiceMonitorAwareClient(t, existing)}

	if err := v.validateServiceMonitorUniqueness(context.Background(), cluster); err != nil {
		t.Fatalf("expected no error when SM is owned by this cluster, got: %v", err)
	}
}

// TestValidateServiceMonitorUniqueness_NoConflict ensures that listing
// returning no matching ServiceMonitor is allowed.
func TestValidateServiceMonitorUniqueness_NoConflict(t *testing.T) {
	cluster := makeMonitoringEnabledCluster("uid-new")

	// Pre-populate an unrelated ServiceMonitor in the same namespace.
	otherOwner := metav1.OwnerReference{
		APIVersion: GroupVersion.String(),
		Kind:       "AerospikeCluster",
		Name:       "unrelated",
		UID:        "uid-unrelated",
	}
	unrelated := newServiceMonitorUnstructured("unrelated-monitor", "ns-a",
		[]metav1.OwnerReference{otherOwner})

	v := &AerospikeClusterValidator{Client: newServiceMonitorAwareClient(t, unrelated)}

	if err := v.validateServiceMonitorUniqueness(context.Background(), cluster); err != nil {
		t.Fatalf("expected no error when no SM with the cluster's name exists, got: %v", err)
	}
}

// TestValidateServiceMonitorUniqueness_DifferentNamespaceIsNotAConflict
// confirms that a same-named ServiceMonitor in a *different* namespace does
// not block creation. The webhook must scope the lookup to cluster.Namespace.
func TestValidateServiceMonitorUniqueness_DifferentNamespaceIsNotAConflict(t *testing.T) {
	cluster := makeMonitoringEnabledCluster("uid-new")
	smName := serviceMonitorName(cluster)

	otherOwner := metav1.OwnerReference{
		APIVersion: GroupVersion.String(),
		Kind:       "AerospikeCluster",
		Name:       "demo-other",
		UID:        "uid-other",
	}
	existing := newServiceMonitorUnstructured(smName, "ns-b",
		[]metav1.OwnerReference{otherOwner})

	v := &AerospikeClusterValidator{Client: newServiceMonitorAwareClient(t, existing)}

	if err := v.validateServiceMonitorUniqueness(context.Background(), cluster); err != nil {
		t.Fatalf("expected no error for SM in a different namespace, got: %v", err)
	}
}

// TestValidateServiceMonitorUniqueness_MonitoringDisabled ensures that the
// check is a no-op when monitoring is disabled at any level — no list call,
// no error, regardless of what exists in the cluster.
func TestValidateServiceMonitorUniqueness_MonitoringDisabled(t *testing.T) {
	tests := []struct {
		name       string
		monitoring *AerospikeMonitoringSpec
	}{
		{
			name:       "monitoring spec is nil",
			monitoring: nil,
		},
		{
			name:       "monitoring.enabled=false",
			monitoring: &AerospikeMonitoringSpec{Enabled: false},
		},
		{
			name: "monitoring.enabled=true but serviceMonitor missing",
			monitoring: &AerospikeMonitoringSpec{
				Enabled: true,
			},
		},
		{
			name: "monitoring.enabled=true but serviceMonitor.enabled=false",
			monitoring: &AerospikeMonitoringSpec{
				Enabled:        true,
				ServiceMonitor: &ServiceMonitorSpec{Enabled: false},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cluster := &AerospikeCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "demo",
					Namespace: "ns-a",
					UID:       "uid-new",
				},
				Spec: AerospikeClusterSpec{
					Size:       1,
					Image:      "aerospike:ce-8.1.1.1",
					Monitoring: tc.monitoring,
				},
			}

			// Even if a conflicting SM exists, the check must be a no-op.
			otherOwner := metav1.OwnerReference{
				APIVersion: GroupVersion.String(),
				Kind:       "AerospikeCluster",
				Name:       "demo-other",
				UID:        "uid-other",
			}
			existing := newServiceMonitorUnstructured(serviceMonitorName(cluster), "ns-a",
				[]metav1.OwnerReference{otherOwner})

			v := &AerospikeClusterValidator{Client: newServiceMonitorAwareClient(t, existing)}

			if err := v.validateServiceMonitorUniqueness(context.Background(), cluster); err != nil {
				t.Fatalf("expected no error when monitoring is disabled, got: %v", err)
			}
		})
	}
}

// TestValidateServiceMonitorUniqueness_NilClient ensures the check is a no-op
// when no client has been wired in (the unit-test path used by the rest of
// this file's tests). This protects existing tests that build the validator
// without a client.
func TestValidateServiceMonitorUniqueness_NilClient(t *testing.T) {
	cluster := makeMonitoringEnabledCluster("uid-new")
	v := &AerospikeClusterValidator{}

	if err := v.validateServiceMonitorUniqueness(context.Background(), cluster); err != nil {
		t.Fatalf("expected no error when validator has no client, got: %v", err)
	}
}

// TestValidateCreate_RejectsDuplicateServiceMonitor exercises the full
// ValidateCreate entry point to confirm the new check is wired into the
// webhook chain (not just callable directly).
func TestValidateCreate_RejectsDuplicateServiceMonitor(t *testing.T) {
	cluster := makeMonitoringEnabledCluster("uid-new")
	// Provide enough monitoring config to pass the pure validate() pass so we
	// reach the client-dependent ServiceMonitor uniqueness check.
	cluster.Spec.Monitoring.ExporterImage = "aerospike/aerospike-prometheus-exporter:1.16.1"
	cluster.Spec.Monitoring.Port = DefaultExporterPort

	smName := serviceMonitorName(cluster)

	otherOwner := metav1.OwnerReference{
		APIVersion: GroupVersion.String(),
		Kind:       "AerospikeCluster",
		Name:       "demo-other",
		UID:        "uid-other",
	}
	existing := newServiceMonitorUnstructured(smName, "ns-a", []metav1.OwnerReference{otherOwner})

	v := &AerospikeClusterValidator{Client: newServiceMonitorAwareClient(t, existing)}

	_, err := v.ValidateCreate(context.Background(), cluster)
	if err == nil {
		t.Fatal("expected ValidateCreate to reject duplicate ServiceMonitor, got nil error")
	}
	if !strings.Contains(err.Error(), smName) {
		t.Errorf("error should mention ServiceMonitor name %q, got: %v", smName, err)
	}
	if !strings.Contains(err.Error(), "demo-other") {
		t.Errorf("error should point at conflicting AerospikeCluster %q, got: %v", "demo-other", err)
	}
}

// --- spec.overrides CE constraint validation tests (P0-1) ---

// TestValidate_OverridesEnterpriseImageRejected pins the CE-bypass fix: a
// templated cluster could previously set spec.overrides.image to an
// Enterprise tag and the cluster webhook would silently accept it. The
// resolved spec then materialised an Enterprise image inside a CE cluster.
func TestValidate_OverridesEnterpriseImageRejected(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:        3,
			Image:       "aerospike:ce-8.1.1.1",
			TemplateRef: &TemplateRef{Name: "prod"},
			Overrides: &AerospikeClusterTemplateSpec{
				Image: "aerospike/aerospike-server-enterprise:8.1",
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Fatal("expected error for enterprise image in spec.overrides")
	}
	if !strings.Contains(err.Error(), "spec.overrides.image") {
		t.Errorf("expected 'spec.overrides.image' in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "enterprise") {
		t.Errorf("expected 'enterprise' in error, got: %v", err)
	}
}

// TestValidate_OverridesXdrInServiceRejected covers the templated-bypass for
// xdr in spec.overrides.aerospikeConfig.service. Previously the cluster
// webhook only validated TemplateRef presence on spec.overrides and never
// scanned the contents.
func TestValidate_OverridesXdrInServiceRejected(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:        3,
			Image:       "aerospike:ce-8.1.1.1",
			TemplateRef: &TemplateRef{Name: "prod"},
			Overrides: &AerospikeClusterTemplateSpec{
				AerospikeConfig: &TemplateAerospikeConfig{
					Service: &AerospikeConfigSpec{
						Value: map[string]any{
							"xdr": map[string]any{"datacenters": []any{}},
						},
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Fatal("expected error for xdr in spec.overrides.aerospikeConfig.service")
	}
	if !strings.Contains(err.Error(), "spec.overrides.aerospikeConfig.service") {
		t.Errorf("expected service path in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "xdr") {
		t.Errorf("expected 'xdr' in error, got: %v", err)
	}
}

// TestValidate_OverridesEnterpriseSecurityInNamespaceDefaultsRejected
// covers the templated-bypass for enterprise security keys in
// spec.overrides.aerospikeConfig.namespaceDefaults. namespaceDefaults is
// merged as a base into every namespace, so a single overrides entry would
// otherwise leak enterprise auth into every namespace.
func TestValidate_OverridesEnterpriseSecurityInNamespaceDefaultsRejected(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:        3,
			Image:       "aerospike:ce-8.1.1.1",
			TemplateRef: &TemplateRef{Name: "prod"},
			Overrides: &AerospikeClusterTemplateSpec{
				AerospikeConfig: &TemplateAerospikeConfig{
					NamespaceDefaults: &AerospikeConfigSpec{
						Value: map[string]any{
							"security": map[string]any{
								"ldap": map[string]any{"server": "ldaps://example"},
							},
						},
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Fatal("expected error for security.ldap in spec.overrides.aerospikeConfig.namespaceDefaults")
	}
	if !strings.Contains(err.Error(), "spec.overrides.aerospikeConfig.namespaceDefaults") {
		t.Errorf("expected namespaceDefaults path in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "ldap") {
		t.Errorf("expected 'ldap' in error, got: %v", err)
	}
}

// TestValidate_OverridesSizeOutOfRangeRejected pins that overrides.size
// must respect the CE 1–8 bound. Without this, a templated cluster with
// overrides.size=99 would silently set spec.size=99 after the merge.
func TestValidate_OverridesSizeOutOfRangeRejected(t *testing.T) {
	v := &AerospikeClusterValidator{}
	bigSize := int32(99)
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:        3,
			Image:       "aerospike:ce-8.1.1.1",
			TemplateRef: &TemplateRef{Name: "prod"},
			Overrides: &AerospikeClusterTemplateSpec{
				Size: &bigSize,
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Fatal("expected error for spec.overrides.size=99")
	}
	if !strings.Contains(err.Error(), "spec.overrides.size") {
		t.Errorf("expected 'spec.overrides.size' in error, got: %v", err)
	}
}

// TestValidate_OverridesValidPasses ensures the new check does not
// over-reject CE-valid overrides: a CE image and a size of 5 in an
// overrides spec must still validate cleanly.
func TestValidate_OverridesValidPasses(t *testing.T) {
	v := &AerospikeClusterValidator{}
	size := int32(5)
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:        3,
			Image:       "aerospike:ce-8.1.1.1",
			TemplateRef: &TemplateRef{Name: "prod"},
			Overrides: &AerospikeClusterTemplateSpec{
				Image: "aerospike:ce-8.1.1.1",
				Size:  &size,
			},
		},
	}

	if _, err := v.validate(cluster); err != nil {
		t.Fatalf("unexpected error for valid CE overrides: %v", err)
	}
}

// --- templateRef immutability tests (P1-2) ---

// TestValidateUpdate_TemplateRefSwapRejected pins that swapping templateRef
// between two existing templates is rejected on update. Swapping would
// silently re-base every templated default and could cross between
// operator-managed configurations without a progressive rollout.
func TestValidateUpdate_TemplateRefSwapRejected(t *testing.T) {
	v := &AerospikeClusterValidator{}
	oldCluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:        3,
			Image:       "aerospike:ce-8.1.1.1",
			TemplateRef: &TemplateRef{Name: "prod"},
		},
	}
	newCluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:        3,
			Image:       "aerospike:ce-8.1.1.1",
			TemplateRef: &TemplateRef{Name: "staging"},
		},
	}

	_, err := v.ValidateUpdate(context.Background(), oldCluster, newCluster)
	if err == nil {
		t.Fatal("expected error when changing templateRef")
	}
	if !strings.Contains(err.Error(), "templateRef is immutable") {
		t.Errorf("expected 'templateRef is immutable' in error, got: %v", err)
	}
}

// TestValidateUpdate_TemplateRefAddRejected covers the add path:
// promoting a non-templated cluster to a templated one is also rejected.
func TestValidateUpdate_TemplateRefAddRejected(t *testing.T) {
	v := &AerospikeClusterValidator{}
	oldCluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
		},
	}
	newCluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:        3,
			Image:       "aerospike:ce-8.1.1.1",
			TemplateRef: &TemplateRef{Name: "prod"},
		},
	}

	_, err := v.ValidateUpdate(context.Background(), oldCluster, newCluster)
	if err == nil {
		t.Fatal("expected error when adding templateRef on update")
	}
	if !strings.Contains(err.Error(), "templateRef is immutable") {
		t.Errorf("expected 'templateRef is immutable' in error, got: %v", err)
	}
}

// TestValidateUpdate_TemplateRefRemoveRejected covers the remove path.
func TestValidateUpdate_TemplateRefRemoveRejected(t *testing.T) {
	v := &AerospikeClusterValidator{}
	oldCluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:        3,
			Image:       "aerospike:ce-8.1.1.1",
			TemplateRef: &TemplateRef{Name: "prod"},
		},
	}
	newCluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
		},
	}

	_, err := v.ValidateUpdate(context.Background(), oldCluster, newCluster)
	if err == nil {
		t.Fatal("expected error when removing templateRef on update")
	}
	if !strings.Contains(err.Error(), "templateRef is immutable") {
		t.Errorf("expected 'templateRef is immutable' in error, got: %v", err)
	}
}

// TestValidateUpdate_TemplateRefUnchangedAllowed pins that an update that
// preserves templateRef is still allowed (the immutability check must not
// over-reject identical refs).
func TestValidateUpdate_TemplateRefUnchangedAllowed(t *testing.T) {
	v := &AerospikeClusterValidator{}
	oldCluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:        3,
			Image:       "aerospike:ce-8.1.1.1",
			TemplateRef: &TemplateRef{Name: "prod"},
		},
	}
	newCluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:        4, // unrelated change
			Image:       "aerospike:ce-8.1.1.1",
			TemplateRef: &TemplateRef{Name: "prod"},
		},
	}

	if _, err := v.ValidateUpdate(context.Background(), oldCluster, newCluster); err != nil {
		t.Fatalf("unexpected error when templateRef unchanged: %v", err)
	}
}

// --- namespaces-as-map rejection (P1-1) ---

// TestValidate_NamespacesAsMapRejected covers the CE bypass: when
// aerospikeConfig.namespaces was provided as a map, the cluster webhook
// only counted entries and never ran per-namespace validation. A map-shaped
// namespaces value with enterprise-only keys (compression,
// strong-consistency, ...) would silently slip through to configgen.
func TestValidate_NamespacesAsMapRejected(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeConfig: &AerospikeConfigSpec{
				Value: map[string]any{
					"namespaces": map[string]any{
						"test": map[string]any{
							"replication-factor": 1,
						},
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Fatal("expected error when aerospikeConfig.namespaces is a map")
	}
	if !strings.Contains(err.Error(), "must be a list") {
		t.Errorf("expected 'must be a list' in error, got: %v", err)
	}
}

// TestValidate_NamespacesAsScalarRejected covers the remaining shape gap: when
// aerospikeConfig.namespaces is a scalar (string/number/bool) — e.g. the common
// YAML mistake `namespaces: default` — the type switch previously fell through
// silently, so the CR was admitted and configgen failed later at reconcile time.
func TestValidate_NamespacesAsScalarRejected(t *testing.T) {
	scalarValues := []any{
		"default",      // string
		3,              // int
		true,           // bool
		[]string{"ns"}, // typed slice (not []any)
	}
	for _, scalar := range scalarValues {
		v := &AerospikeClusterValidator{}
		cluster := &AerospikeCluster{
			Spec: AerospikeClusterSpec{
				Size:  3,
				Image: "aerospike:ce-8.1.1.1",
				AerospikeConfig: &AerospikeConfigSpec{
					Value: map[string]any{
						"namespaces": scalar,
					},
				},
			},
		}

		_, err := v.validate(cluster)
		if err == nil {
			t.Fatalf("expected error when aerospikeConfig.namespaces is %T", scalar)
		}
		if !strings.Contains(err.Error(), "must be a list") {
			t.Errorf("expected 'must be a list' in error for %T, got: %v", scalar, err)
		}
	}
}

// --- rack count vs cluster size (more racks than pods) ---

// TestValidate_MoreRacksThanSizeRejected covers the footgun where rackConfig
// defines more racks than spec.size: getRackSize then assigns 0 replicas to at
// least one rack, producing a 0-replica StatefulSet that never carries data.
func TestValidate_MoreRacksThanSizeRejected(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  2,
			Image: "aerospike:ce-8.1.1.1",
			RackConfig: &RackConfig{
				Racks: []Rack{
					{ID: 1},
					{ID: 2},
					{ID: 3},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Fatal("expected error when rack count exceeds spec.size")
	}
	if !strings.Contains(err.Error(), "rack count must not exceed spec.size") {
		t.Errorf("error should mention the rack-count/size mismatch, got: %v", err)
	}
}

// TestValidate_RackCountEqualToSizeAccepted confirms the boundary case
// (one pod per rack) is allowed.
func TestValidate_RackCountEqualToSizeAccepted(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			RackConfig: &RackConfig{
				Racks: []Rack{
					{ID: 1},
					{ID: 2},
					{ID: 3},
				},
			},
		},
	}

	if _, err := v.validate(cluster); err != nil {
		t.Fatalf("rack count equal to spec.size should be accepted, got: %v", err)
	}
}

// TestValidate_MoreRacksThanSizeAllowedWithTemplate confirms the rack/size
// cross-check is deferred when size is supplied by a referenced template
// (size == 0 && templateRef != nil).
func TestValidate_MoreRacksThanSizeAllowedWithTemplate(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:        0,
			TemplateRef: &TemplateRef{Name: "ce-template"},
			RackConfig: &RackConfig{
				Racks: []Rack{
					{ID: 1},
					{ID: 2},
				},
			},
		},
	}

	if _, err := v.validate(cluster); err != nil {
		t.Fatalf("rack/size check should be deferred to template resolution, got: %v", err)
	}
}

// --- CE logging-section validation tests (validateLoggingSection) ---

// TestValidate_CELoggingEnterpriseContextsRejected verifies that enterprise-only
// logging context keys (audit + the report-* family) are rejected by the
// admission webhook. Setting any of these on a CE cluster causes aerospikd to
// abort at startup with an "unknown context" error.
func TestValidate_CELoggingEnterpriseContextsRejected(t *testing.T) {
	v := &AerospikeClusterValidator{}

	entContexts := []string{
		"audit",
		"report-data-op",
		"report-data-op-user",
		"report-data-op-role",
		"report-sys-admin",
		"report-user-admin",
		"report-violation",
		"report-authentication",
	}

	for _, ctxKey := range entContexts {
		t.Run(ctxKey, func(t *testing.T) {
			cluster := &AerospikeCluster{
				Spec: AerospikeClusterSpec{
					Size:  3,
					Image: "aerospike:ce-8.1.1.1",
					AerospikeConfig: &AerospikeConfigSpec{
						Value: map[string]any{
							"logging": []any{
								map[string]any{
									"name": "/var/log/aerospike/aerospike.log",
									"any":  "info",
									ctxKey: "info",
								},
							},
						},
					},
				},
			}

			_, err := v.validate(cluster)
			if err == nil {
				t.Fatalf("expected error for enterprise-only logging context %q, got nil", ctxKey)
			}
			expected := fmt.Sprintf("logging[0].%s is not allowed in CE", ctxKey)
			if !strings.Contains(err.Error(), expected) {
				t.Errorf("expected error to contain %q, got: %v", expected, err)
			}
		})
	}
}

// TestValidate_CELoggingValidSinksAccepted verifies that all CE-supported
// logging sinks (console, stderr, syslog, file by path) are accepted along
// with arbitrary CE context names (e.g. info, any, namespace).
func TestValidate_CELoggingValidSinksAccepted(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeConfig: &AerospikeConfigSpec{
				Value: map[string]any{
					"logging": []any{
						map[string]any{"name": "console", "any": "info"},
						map[string]any{"name": "stderr", "any": "warning"},
						map[string]any{"name": "syslog", "any": "info"},
						map[string]any{"name": "/var/log/aerospike/aerospike.log", "any": "info", "namespace": "debug"},
					},
				},
			},
		},
	}

	if _, err := v.validate(cluster); err != nil {
		t.Errorf("expected valid CE logging config to be accepted, got: %v", err)
	}
}

// TestValidate_CELoggingMalformedEntries verifies that malformed logging
// entries are rejected with a descriptive error rather than panicking. Covers:
//   - logging value that is not a list
//   - logging entry that is not a map
//   - logging entry missing the required name key
//   - logging entry with a non-string name
func TestValidate_CELoggingMalformedEntries(t *testing.T) {
	v := &AerospikeClusterValidator{}

	cases := []struct {
		desc    string
		logging any
		wantSub string
	}{
		{
			desc:    "scalar instead of list",
			logging: "info",
			wantSub: "aerospikeConfig.logging must be a list",
		},
		{
			desc:    "entry is a string",
			logging: []any{"console"},
			wantSub: "logging[0] must be a map",
		},
		{
			desc:    "entry missing name",
			logging: []any{map[string]any{"any": "info"}},
			wantSub: "logging[0] is missing the required 'name' key",
		},
		{
			desc:    "entry with non-string name",
			logging: []any{map[string]any{"name": 42, "any": "info"}},
			wantSub: "logging[0].name must be a non-empty string",
		},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			cluster := &AerospikeCluster{
				Spec: AerospikeClusterSpec{
					Size:  3,
					Image: "aerospike:ce-8.1.1.1",
					AerospikeConfig: &AerospikeConfigSpec{
						Value: map[string]any{"logging": tc.logging},
					},
				},
			}

			// Must not panic regardless of input shape.
			_, err := v.validate(cluster)
			if err == nil {
				t.Fatalf("expected error for %s, got nil", tc.desc)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("expected error to contain %q, got: %v", tc.wantSub, err)
			}
		})
	}
}

// TestValidate_SecuritySectionMalformedNoPanic verifies that malformed
// security section shapes (nil interface, scalars, lists) produce a
// descriptive admission error and never panic. The original implementation
// used a single inline type assertion that could trip subtle nil-handling
// paths; validateSecuritySection now centralises the defensive checks.
func TestValidate_SecuritySectionMalformedNoPanic(t *testing.T) {
	v := &AerospikeClusterValidator{}

	cases := []struct {
		desc    string
		section any
	}{
		{"explicit nil", nil},
		{"scalar string", "enable-security"},
		{"scalar bool", true},
		{"list shape", []any{"enable-security"}},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			cluster := &AerospikeCluster{
				Spec: AerospikeClusterSpec{
					Size:  3,
					Image: "aerospike:ce-8.1.1.1",
					AerospikeConfig: &AerospikeConfigSpec{
						Value: map[string]any{"security": tc.section},
					},
				},
			}

			_, err := v.validate(cluster)
			if err == nil {
				t.Fatalf("expected error for malformed security section %s, got nil", tc.desc)
			}
			if !strings.Contains(err.Error(), "aerospikeConfig.security must be a map") {
				t.Errorf("expected 'security must be a map' error, got: %v", err)
			}
		})
	}
}

// --- CE minimum image-version validation tests ---

// TestParseMajorVersion covers direct major-version extraction, including the
// single-segment (dotless) tag case that previously returned an error and so
// silently bypassed the CE-version gate.
func TestParseMajorVersion(t *testing.T) {
	tests := []struct {
		name    string
		image   string
		want    int
		wantErr bool
	}{
		{name: "ce-prefixed full", image: "aerospike:ce-8.1.0.1", want: 8, wantErr: false},
		{name: "plain full", image: "aerospike:8.1.0.1", want: 8, wantErr: false},
		{name: "v-prefixed", image: "aerospike:v8.1", want: 8, wantErr: false},
		{name: "old ce-7 full", image: "aerospike:ce-7.0.0.0", want: 7, wantErr: false},
		// dotless single-segment tags must still yield the major version.
		{name: "ce-prefixed dotless 7", image: "aerospike:ce-7", want: 7, wantErr: false},
		{name: "plain dotless 7", image: "aerospike:7", want: 7, wantErr: false},
		{name: "ce-prefixed dotless 8", image: "aerospike:ce-8", want: 8, wantErr: false},
		{name: "v-prefixed dotless 8", image: "aerospike:v8", want: 8, wantErr: false},
		// Registry with a port: the host:port colon must not be read as the tag
		// separator, otherwise the major version is misparsed and the CE-version
		// gate is silently skipped.
		{name: "ported registry ce-7", image: "myregistry.io:5000/aerospike:ce-7.0.0.0", want: 7, wantErr: false},
		{name: "ported registry ce-8", image: "localhost:32000/aerospike:ce-8.1.1.1", want: 8, wantErr: false},
		// Digest-pinned refs: the digest suffix must be stripped so the real tag's
		// major version is parsed, not the digest hex (which would error and skip
		// the CE-version gate).
		{name: "digest-pinned ce-7", image: "aerospike:ce-7.0.0.0@sha256:abc123", want: 7, wantErr: false},
		{name: "digest-pinned ce-8", image: "aerospike:ce-8.1.1.1@sha256:abc123", want: 8, wantErr: false},
		{name: "ported registry digest-pinned ce-8", image: "myregistry.io:5000/aerospike:ce-8.1.1.1@sha256:abc123", want: 8, wantErr: false},
		// Digest only (no tag) must error so the caller skips it.
		{name: "digest only untagged", image: "aerospike@sha256:abc123", want: 0, wantErr: true},
		// Ported registry with no tag must error so the caller skips it.
		{name: "ported registry untagged", image: "myregistry.io:5000/aerospike", want: 0, wantErr: true},
		// genuinely unparseable tags must still error so the caller skips gracefully.
		{name: "unparseable latest", image: "aerospike:latest", want: 0, wantErr: true},
		{name: "unparseable empty tag", image: "aerospike:", want: 0, wantErr: true},
		{name: "no tag at all", image: "aerospike", want: 0, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseMajorVersion(tc.image)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseMajorVersion(%q) = %d, want error", tc.image, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseMajorVersion(%q) unexpected error: %v", tc.image, err)
			}
			if got != tc.want {
				t.Errorf("parseMajorVersion(%q) = %d, want %d", tc.image, got, tc.want)
			}
		})
	}
}

// TestValidate_CEImageVersionEnforcement verifies the end-to-end webhook
// behavior: CE images below the minimum major version are rejected (including
// single-segment tags, which used to slip through), CE-8+ images are accepted,
// and unparseable tags are skipped rather than false-rejected.
func TestValidate_CEImageVersionEnforcement(t *testing.T) {
	tests := []struct {
		name    string
		image   string
		wantErr bool
	}{
		{name: "ce-8 full ok", image: "aerospike:ce-8.1.0.1", wantErr: false},
		{name: "ce-8 four-segment ok", image: "aerospike:ce-8.1.1.1", wantErr: false},
		{name: "ce-7 full rejected", image: "aerospike:ce-7.0.0.0", wantErr: true},
		{name: "latest skipped", image: "aerospike:latest", wantErr: false},
		{name: "no tag skipped", image: "aerospike", wantErr: false},
		// dotless single-segment tags must enforce the CE-8 minimum too.
		{name: "ce-7 dotless rejected", image: "aerospike:ce-7", wantErr: true},
		{name: "plain 7 dotless rejected", image: "aerospike:7", wantErr: true},
		{name: "ce-8 dotless ok", image: "aerospike:ce-8", wantErr: false},
		{name: "v8 dotless ok", image: "aerospike:v8", wantErr: false},
	}

	v := &AerospikeClusterValidator{}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cluster := &AerospikeCluster{
				Spec: AerospikeClusterSpec{
					Size:  3,
					Image: tc.image,
				},
			}
			_, err := v.validate(cluster)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("validate() with image %q: expected error, got none", tc.image)
				}
				if !strings.Contains(err.Error(), "requires Aerospike CE") {
					t.Errorf("validate() with image %q: expected CE-minimum error, got: %v", tc.image, err)
				}
				return
			}
			if err != nil {
				t.Errorf("validate() with image %q: unexpected error: %v", tc.image, err)
			}
		})
	}
}

// --- CE network-TLS rejection tests ---
//
// The pre-existing CE TLS check looked only at the top-level
// aerospikeConfig["tls"] key. Real Aerospike TLS lives inside the network
// stanza — `network { tls <name> { ... } }` plus tls-port / tls-name /
// tls-authenticate-client on each of service, heartbeat and fabric — which is
// what the Aerospike documentation tells users to write. configgen renders any
// sub-map under `network` verbatim, so an unchecked stanza reached
// aerospike.conf and the CE asd process refused to start: permanent
// CrashLoopBackOff, no admission error naming the Enterprise feature, and on a
// live cluster the config-hash change rolled every pod into it batch by batch.

func TestValidate_NetworkTLSRejected(t *testing.T) {
	v := &AerospikeClusterValidator{}

	tests := []struct {
		name    string
		network map[string]any
		// wantPath is the key path the admission error must name so an operator
		// can find and delete the offending key.
		wantPath string
	}{
		{
			name: "network.tls certificate stanza",
			network: map[string]any{
				"tls": map[string]any{
					"aerospike-tls": map[string]any{
						"cert-file": "/etc/aerospike/cert.pem",
						"key-file":  "/etc/aerospike/key.pem",
					},
				},
			},
			wantPath: "aerospikeConfig.network must not contain 'tls' section",
		},
		{
			// Presence of the key is enough — a nil value still says the user
			// intended TLS, and configgen would render an empty stanza.
			name:     "network.tls with a nil value",
			network:  map[string]any{"tls": nil},
			wantPath: "aerospikeConfig.network must not contain 'tls' section",
		},
		{
			name: "network.service.tls-port",
			network: map[string]any{
				"service": map[string]any{
					"port":     3000,
					"tls-port": 4333,
				},
			},
			wantPath: "aerospikeConfig.network.service.tls-port",
		},
		{
			name: "network.service.tls-authenticate-client",
			network: map[string]any{
				"service": map[string]any{
					"tls-authenticate-client": "any",
				},
			},
			wantPath: "aerospikeConfig.network.service.tls-authenticate-client",
		},
		{
			// heartbeat has its own sub-generator (generateHeartbeatSubsection),
			// so it needs its own case rather than sharing the service path.
			name: "network.heartbeat.tls-port",
			network: map[string]any{
				"heartbeat": map[string]any{
					"mode":     "mesh",
					"tls-port": 3012,
				},
			},
			wantPath: "aerospikeConfig.network.heartbeat.tls-port",
		},
		{
			name: "network.heartbeat.tls-name",
			network: map[string]any{
				"heartbeat": map[string]any{
					"mode":     "mesh",
					"tls-name": "aerospike-tls",
				},
			},
			wantPath: "aerospikeConfig.network.heartbeat.tls-name",
		},
		{
			name: "network.fabric.tls-port",
			network: map[string]any{
				"fabric": map[string]any{
					"port":     3001,
					"tls-port": 3011,
				},
			},
			wantPath: "aerospikeConfig.network.fabric.tls-port",
		},
		{
			name: "network.fabric.tls-name",
			network: map[string]any{
				"fabric": map[string]any{
					"tls-name": "aerospike-tls",
				},
			},
			wantPath: "aerospikeConfig.network.fabric.tls-name",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cluster := &AerospikeCluster{
				Spec: AerospikeClusterSpec{
					Size:  3,
					Image: "aerospike:ce-8.1.1.1",
					AerospikeConfig: &AerospikeConfigSpec{
						Value: map[string]any{"network": tc.network},
					},
				},
			}

			_, err := v.validate(cluster)
			if err == nil {
				t.Fatalf("validate() = nil, want error for %s", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantPath) {
				t.Errorf("error must name the offending key path %q, got: %v", tc.wantPath, err)
			}
			// Every CE rejection names the Enterprise feature (goal 3-5), so an
			// operator can tell "not supported here" from "you typed it wrong".
			if !strings.Contains(err.Error(), "TLS is Enterprise-only") {
				t.Errorf("error must name the Enterprise feature, got: %v", err)
			}
		})
	}
}

// TestValidate_NetworkTLSRejectedAcrossAllEndpointsAtOnce pins that a full TLS
// configuration — the shape a user copies out of the Aerospike docs — is
// reported key by key rather than only at the first hit, so one admission
// round-trip tells the operator everything to remove.
func TestValidate_NetworkTLSRejectedAcrossAllEndpointsAtOnce(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeConfig: &AerospikeConfigSpec{
				Value: map[string]any{
					"network": map[string]any{
						"tls": map[string]any{
							"aerospike-tls": map[string]any{"cert-file": "/cert.pem"},
						},
						"service": map[string]any{
							"port":     3000,
							"tls-port": 4333,
							"tls-name": "aerospike-tls",
						},
						"heartbeat": map[string]any{
							"mode":     "mesh",
							"tls-port": 3012,
						},
						"fabric": map[string]any{
							"tls-port": 3011,
						},
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Fatal("validate() = nil, want error for a full network TLS configuration")
	}

	wantPaths := []string{
		"aerospikeConfig.network must not contain 'tls' section",
		"aerospikeConfig.network.service.tls-name",
		"aerospikeConfig.network.service.tls-port",
		"aerospikeConfig.network.heartbeat.tls-port",
		"aerospikeConfig.network.fabric.tls-port",
	}
	for _, want := range wantPaths {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("expected %q in error, got: %v", want, err)
		}
	}
}

// TestValidate_NetworkNonTLSKeysAccepted is the over-rejection guard: an
// ordinary CE network stanza must still pass. Without this the tls- prefix
// match could quietly reject valid configurations.
func TestValidate_NetworkNonTLSKeysAccepted(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeConfig: &AerospikeConfigSpec{
				Value: map[string]any{
					"network": map[string]any{
						"service": map[string]any{
							"port":           3000,
							"access-address": "10.0.0.1",
							"access-port":    3000,
						},
						"heartbeat": map[string]any{
							"mode":     "mesh",
							"port":     3002,
							"interval": 150,
							"timeout":  10,
						},
						"fabric": map[string]any{
							"port":             3001,
							"channel-bulk-fds": 2,
						},
						"info": map[string]any{
							"port": 3003,
						},
					},
				},
			},
		},
	}

	if _, err := v.validate(cluster); err != nil {
		t.Errorf("validate() = %v, want nil for a plain CE network stanza", err)
	}
}

// TestValidateUpdate_NetworkTLSRejected pins that the new check runs on UPDATE
// as well as CREATE. This is the path that matters most: adding TLS to a live
// cluster changes the config hash, so an admitted edit would roll every pod
// into a crash loop one batch at a time. ValidateUpdate funnels into the same
// validateWithCtx -> validate() chain as ValidateCreate, and this test pins
// that it stays that way.
func TestValidateUpdate_NetworkTLSRejected(t *testing.T) {
	v := &AerospikeClusterValidator{}

	oldCluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeConfig: &AerospikeConfigSpec{
				Value: map[string]any{
					"network": map[string]any{
						"heartbeat": map[string]any{"mode": "mesh", "port": 3002},
					},
				},
			},
		},
	}
	newCluster := oldCluster.DeepCopy()
	newCluster.Spec.AerospikeConfig.Value["network"] = map[string]any{
		"heartbeat": map[string]any{
			"mode":     "mesh",
			"port":     3002,
			"tls-port": 3012,
			"tls-name": "aerospike-tls",
		},
	}

	_, err := v.ValidateUpdate(context.Background(), oldCluster, newCluster)
	if err == nil {
		t.Fatal("ValidateUpdate() = nil, want error for network TLS added on update")
	}
	if !strings.Contains(err.Error(), "aerospikeConfig.network.heartbeat.tls-port") {
		t.Errorf("expected the offending key path in the error, got: %v", err)
	}
}

// TestValidate_PerRackNetworkTLSRejected covers the per-rack bypass route: a
// rack's aerospikeConfig is DeepMerged into the effective config and rendered
// into that rack's ConfigMap, so it must be held to the same CE constraints as
// the cluster-level config.
func TestValidate_PerRackNetworkTLSRejected(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			RackConfig: &RackConfig{
				Racks: []Rack{
					{
						ID: 1,
						AerospikeConfig: &AerospikeConfigSpec{
							Value: map[string]any{
								"network": map[string]any{
									"service": map[string]any{"tls-port": 4333},
								},
							},
						},
					},
				},
			},
		},
	}

	_, err := v.validate(cluster)
	if err == nil {
		t.Fatal("validate() = nil, want error for network TLS in a per-rack config")
	}
	if !strings.Contains(err.Error(), "aerospikeConfig.network.service.tls-port") {
		t.Errorf("expected the offending key path in the error, got: %v", err)
	}
}

// TestValidate_OverridesTLSKeysRejected covers the templated-bypass route for
// per-endpoint TLS keys: spec.overrides.aerospikeConfig.{service,
// namespaceDefaults} are merged into the cluster's effective config, so a
// tls-* key there reaches aerospike.conf exactly like a cluster-level one.
func TestValidate_OverridesTLSKeysRejected(t *testing.T) {
	tests := []struct {
		name      string
		overrides *AerospikeClusterTemplateSpec
		wantPath  string
	}{
		{
			name: "tls-port in overrides service",
			overrides: &AerospikeClusterTemplateSpec{
				AerospikeConfig: &TemplateAerospikeConfig{
					Service: &AerospikeConfigSpec{
						Value: map[string]any{"tls-port": 4333},
					},
				},
			},
			wantPath: "spec.overrides.aerospikeConfig.service.tls-port",
		},
		{
			name: "tls-name in overrides namespaceDefaults",
			overrides: &AerospikeClusterTemplateSpec{
				AerospikeConfig: &TemplateAerospikeConfig{
					NamespaceDefaults: &AerospikeConfigSpec{
						Value: map[string]any{"tls-name": "aerospike-tls"},
					},
				},
			},
			wantPath: "spec.overrides.aerospikeConfig.namespaceDefaults.tls-name",
		},
		{
			name: "nested network.tls in overrides service",
			overrides: &AerospikeClusterTemplateSpec{
				AerospikeConfig: &TemplateAerospikeConfig{
					Service: &AerospikeConfigSpec{
						Value: map[string]any{
							"network": map[string]any{
								"tls": map[string]any{"aerospike-tls": map[string]any{}},
							},
						},
					},
				},
			},
			wantPath: "spec.overrides.aerospikeConfig.service.network must not contain 'tls' section",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v := &AerospikeClusterValidator{}
			cluster := &AerospikeCluster{
				Spec: AerospikeClusterSpec{
					Size:        3,
					Image:       "aerospike:ce-8.1.1.1",
					TemplateRef: &TemplateRef{Name: "prod"},
					Overrides:   tc.overrides,
				},
			}

			_, err := v.validate(cluster)
			if err == nil {
				t.Fatalf("validate() = nil, want error for %s", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantPath) {
				t.Errorf("error must name %q, got: %v", tc.wantPath, err)
			}
			if !strings.Contains(err.Error(), "TLS is Enterprise-only") {
				t.Errorf("error must name the Enterprise feature, got: %v", err)
			}
		})
	}
}

// TestValidate_OverridesNonTLSKeysAccepted is the over-rejection guard for the
// overrides path.
func TestValidate_OverridesNonTLSKeysAccepted(t *testing.T) {
	v := &AerospikeClusterValidator{}
	cluster := &AerospikeCluster{
		Spec: AerospikeClusterSpec{
			Size:        3,
			Image:       "aerospike:ce-8.1.1.1",
			TemplateRef: &TemplateRef{Name: "prod"},
			Overrides: &AerospikeClusterTemplateSpec{
				AerospikeConfig: &TemplateAerospikeConfig{
					Service: &AerospikeConfigSpec{
						Value: map[string]any{
							"proto-fd-max":       15000,
							"enable-security":    false,
							"transaction-max-ms": 1000,
						},
					},
					NamespaceDefaults: &AerospikeConfigSpec{
						Value: map[string]any{
							"replication-factor": 2,
							"default-ttl":        "30d",
						},
					},
				},
			},
		},
	}

	if _, err := v.validate(cluster); err != nil {
		t.Errorf("validate() = %v, want nil for plain CE overrides", err)
	}
}
