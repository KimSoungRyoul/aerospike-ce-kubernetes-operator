package configgen

import (
	"testing"

	v1alpha1 "github.com/ksr/aerospike-ce-kubernetes-operator/api/v1alpha1"
)

const (
	placeholderPodIP  = "MY_POD_IP"
	placeholderNodeIP = "MY_NODE_IP"
)

func TestInjectAccessAddressPlaceholders_NilPolicy(t *testing.T) {
	config := map[string]any{
		"network": map[string]any{
			"service": map[string]any{
				"port": 3000,
			},
		},
	}

	InjectAccessAddressPlaceholders(config, nil, "")

	svc := config["network"].(map[string]any)["service"].(map[string]any)
	if _, exists := svc["access-address"]; exists {
		t.Error("access-address should not be set when policy is nil")
	}
}

func TestInjectAccessAddressPlaceholders_PodType(t *testing.T) {
	config := map[string]any{
		"network": map[string]any{
			"service": map[string]any{
				"port": 3000,
			},
		},
	}

	policy := &v1alpha1.AerospikeNetworkPolicy{
		AccessType: v1alpha1.AerospikeNetworkTypePod,
	}

	InjectAccessAddressPlaceholders(config, policy, "")

	svc := config["network"].(map[string]any)["service"].(map[string]any)
	if svc["access-address"] != placeholderPodIP {
		t.Errorf("access-address = %v, want MY_POD_IP", svc["access-address"])
	}
}

func TestInjectAccessAddressPlaceholders_HostInternalType(t *testing.T) {
	config := map[string]any{
		"network": map[string]any{
			"service": map[string]any{
				"port": 3000,
			},
		},
	}

	policy := &v1alpha1.AerospikeNetworkPolicy{
		AccessType: v1alpha1.AerospikeNetworkTypeHostInternal,
	}

	InjectAccessAddressPlaceholders(config, policy, "")

	svc := config["network"].(map[string]any)["service"].(map[string]any)
	if svc["access-address"] != placeholderNodeIP {
		t.Errorf("access-address = %v, want MY_NODE_IP", svc["access-address"])
	}
}

func TestInjectAccessAddressPlaceholders_HostExternalType(t *testing.T) {
	config := map[string]any{
		"network": map[string]any{
			"service": map[string]any{
				"port": 3000,
			},
		},
	}

	policy := &v1alpha1.AerospikeNetworkPolicy{
		AccessType: v1alpha1.AerospikeNetworkTypeHostExternal,
	}

	InjectAccessAddressPlaceholders(config, policy, "")

	svc := config["network"].(map[string]any)["service"].(map[string]any)
	if svc["access-address"] != placeholderNodeIP {
		t.Errorf("access-address = %v, want MY_NODE_IP", svc["access-address"])
	}
}

func TestInjectAccessAddressPlaceholders_AlternateAccessType(t *testing.T) {
	config := map[string]any{
		"network": map[string]any{
			"service": map[string]any{
				"port": 3000,
			},
		},
	}

	policy := &v1alpha1.AerospikeNetworkPolicy{
		AccessType:          v1alpha1.AerospikeNetworkTypePod,
		AlternateAccessType: v1alpha1.AerospikeNetworkTypeHostInternal,
	}

	InjectAccessAddressPlaceholders(config, policy, "")

	svc := config["network"].(map[string]any)["service"].(map[string]any)
	if svc["access-address"] != placeholderPodIP {
		t.Errorf("access-address = %v, want MY_POD_IP", svc["access-address"])
	}
	if svc["alternate-access-address"] != placeholderNodeIP {
		t.Errorf("alternate-access-address = %v, want MY_NODE_IP", svc["alternate-access-address"])
	}
}

func TestInjectAccessAddressPlaceholders_DoesNotOverrideExisting(t *testing.T) {
	config := map[string]any{
		"network": map[string]any{
			"service": map[string]any{
				"port":           3000,
				"access-address": "10.0.0.1",
			},
		},
	}

	policy := &v1alpha1.AerospikeNetworkPolicy{
		AccessType: v1alpha1.AerospikeNetworkTypePod,
	}

	InjectAccessAddressPlaceholders(config, policy, "")

	svc := config["network"].(map[string]any)["service"].(map[string]any)
	if svc["access-address"] != "10.0.0.1" {
		t.Errorf("access-address = %v, should preserve existing value %q", svc["access-address"], "10.0.0.1")
	}
}

func TestInjectAccessAddressPlaceholders_ConfiguredIPType(t *testing.T) {
	config := map[string]any{
		"network": map[string]any{
			"service": map[string]any{
				"port": 3000,
			},
		},
	}

	policy := &v1alpha1.AerospikeNetworkPolicy{
		AccessType: v1alpha1.AerospikeNetworkTypeConfiguredIP,
	}

	InjectAccessAddressPlaceholders(config, policy, "")

	svc := config["network"].(map[string]any)["service"].(map[string]any)
	if _, exists := svc["access-address"]; exists {
		t.Error("access-address should not be set for configuredIP type")
	}
}

func TestInjectAccessAddressPlaceholders_NoNetworkSection(t *testing.T) {
	config := map[string]any{
		"service": map[string]any{
			"cluster-name": "test",
		},
	}

	policy := &v1alpha1.AerospikeNetworkPolicy{
		AccessType: v1alpha1.AerospikeNetworkTypePod,
	}

	// Should not panic
	InjectAccessAddressPlaceholders(config, policy, "")
}

func TestInjectAccessAddressPlaceholders_NoServiceSubsection(t *testing.T) {
	config := map[string]any{
		"network": map[string]any{
			"heartbeat": map[string]any{
				"mode": "mesh",
			},
		},
	}

	policy := &v1alpha1.AerospikeNetworkPolicy{
		AccessType: v1alpha1.AerospikeNetworkTypePod,
	}

	// Should not panic
	InjectAccessAddressPlaceholders(config, policy, "")
}

func TestInjectAccessAddressPlaceholders_LoadBalancerOverridesAlternateAccess(t *testing.T) {
	config := map[string]any{
		"network": map[string]any{
			"service": map[string]any{
				"port": 3000,
			},
		},
	}

	policy := &v1alpha1.AerospikeNetworkPolicy{
		AccessType:          v1alpha1.AerospikeNetworkTypePod,
		AlternateAccessType: v1alpha1.AerospikeNetworkTypePod,
	}

	InjectAccessAddressPlaceholders(config, policy, "LoadBalancer")

	svc := config["network"].(map[string]any)["service"].(map[string]any)
	if svc["access-address"] != placeholderPodIP {
		t.Errorf("access-address = %v, want MY_POD_IP", svc["access-address"])
	}
	// LoadBalancer should override alternate-access-address regardless of policy
	if svc["alternate-access-address"] != "MY_EXTERNAL_ADDRESS" {
		t.Errorf("alternate-access-address = %v, want MY_EXTERNAL_ADDRESS", svc["alternate-access-address"])
	}
}

func TestInjectAccessAddressPlaceholders_NodePortSetsExternalPort(t *testing.T) {
	config := map[string]any{
		"network": map[string]any{
			"service": map[string]any{
				"port": 3000,
			},
		},
	}

	policy := &v1alpha1.AerospikeNetworkPolicy{
		AccessType: v1alpha1.AerospikeNetworkTypePod,
	}

	InjectAccessAddressPlaceholders(config, policy, "NodePort")

	svc := config["network"].(map[string]any)["service"].(map[string]any)
	if svc["alternate-access-address"] != placeholderNodeIP {
		t.Errorf("alternate-access-address = %v, want MY_NODE_IP", svc["alternate-access-address"])
	}
	if svc["alternate-access-port"] != "MY_EXTERNAL_PORT" {
		t.Errorf("alternate-access-port = %v, want MY_EXTERNAL_PORT", svc["alternate-access-port"])
	}
}

func TestInjectAccessAddressPlaceholders_ClusterIPNoExternalOverride(t *testing.T) {
	config := map[string]any{
		"network": map[string]any{
			"service": map[string]any{
				"port": 3000,
			},
		},
	}

	policy := &v1alpha1.AerospikeNetworkPolicy{
		AccessType: v1alpha1.AerospikeNetworkTypePod,
	}

	InjectAccessAddressPlaceholders(config, policy, "ClusterIP")

	svc := config["network"].(map[string]any)["service"].(map[string]any)
	if _, exists := svc["alternate-access-address"]; exists {
		t.Error("alternate-access-address should not be set for ClusterIP")
	}
	if _, exists := svc["alternate-access-port"]; exists {
		t.Error("alternate-access-port should not be set for ClusterIP")
	}
}

func TestPlaceholderForNetworkType(t *testing.T) {
	tests := []struct {
		netType  v1alpha1.AerospikeNetworkType
		expected string
	}{
		{v1alpha1.AerospikeNetworkTypePod, placeholderPodIP},
		{v1alpha1.AerospikeNetworkTypeHostInternal, placeholderNodeIP},
		{v1alpha1.AerospikeNetworkTypeHostExternal, placeholderNodeIP},
		{v1alpha1.AerospikeNetworkTypeConfiguredIP, ""},
		{"unknown", ""},
	}

	for _, tc := range tests {
		got := placeholderForNetworkType(tc.netType)
		if got != tc.expected {
			t.Errorf("placeholderForNetworkType(%q) = %q, want %q", tc.netType, got, tc.expected)
		}
	}
}
