package configgen

import (
	"strings"
	"testing"

	v1alpha1 "github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/api/v1alpha1"
)

// --- the two per-pod-service branches must agree about the port ---
//
// The init container substitutes MY_EXTERNAL_PORT only on the NodePort path
// (aerospike-init.sh); on the LoadBalancer path it resolves the address alone,
// because a LoadBalancer serves the service port itself.
//
// So a user-specified alternate-access-port — perfectly reasonable while using
// NodePort — survived a switch to LoadBalancer and made every peer advertise
// <lb-address>:<stale-port>, a port nothing listens on. Clients then fail to
// reach peers via --services-alternate, which looks exactly like the port having
// been dropped.

func externalAccessConfig(alternatePort any) map[string]any {
	svc := map[string]any{"address": "any", "port": 3000}
	if alternatePort != nil {
		svc["alternate-access-port"] = alternatePort
	}
	return map[string]any{
		"network": map[string]any{
			"service":   svc,
			"heartbeat": map[string]any{"mode": "mesh", "port": 3002},
		},
	}
}

func hostInternalPolicy() *v1alpha1.AerospikeNetworkPolicy {
	return &v1alpha1.AerospikeNetworkPolicy{
		AccessType:          v1alpha1.AerospikeNetworkTypePod,
		AlternateAccessType: v1alpha1.AerospikeNetworkTypeHostInternal,
	}
}

func alternatePortOf(t *testing.T, config map[string]any) (any, bool) {
	t.Helper()
	svc := config["network"].(map[string]any)["service"].(map[string]any)
	v, ok := svc["alternate-access-port"]
	return v, ok
}

func TestInjectAccessAddressPlaceholders_PortAgreesWithServiceType(t *testing.T) {
	tests := []struct {
		name           string
		podServiceType string
		userPort       any
		wantPort       any
		wantAbsent     bool
		wantOverride   string // substring; empty means no override reported
	}{
		{
			// The regression. A LoadBalancer serves the service port, so a
			// leftover alternate-access-port advertises a port nothing listens on.
			name:           "LoadBalancer drops a stale user port",
			podServiceType: "LoadBalancer",
			userPort:       30000,
			wantAbsent:     true,
			wantOverride:   "removed network.service.alternate-access-port (30000)",
		},
		{
			name:           "LoadBalancer with no user port stays clean",
			podServiceType: "LoadBalancer",
			wantAbsent:     true,
		},
		{
			// The operator creates the NodePort service and Kubernetes allocates
			// the port, so the live service is the authority — but the override
			// must be reported, not silent.
			name:           "NodePort overrides a user port and says so",
			podServiceType: "NodePort",
			userPort:       30000,
			wantPort:       placeholderExternalPort,
			wantOverride:   "overrode network.service.alternate-access-port (30000)",
		},
		{
			name:           "NodePort with no user port needs no override notice",
			podServiceType: "NodePort",
			wantPort:       placeholderExternalPort,
		},
		{
			// The issue #218 configuration: the NodePort Service is created by
			// hand, so spec.podService is unset and neither branch fires. The
			// user's port must survive untouched.
			name:     "no per-pod service leaves the user's port alone",
			userPort: 30000,
			wantPort: 30000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := externalAccessConfig(tt.userPort)
			overrides := InjectAccessAddressPlaceholders(config, hostInternalPolicy(), tt.podServiceType)

			got, present := alternatePortOf(t, config)
			if tt.wantAbsent {
				if present {
					t.Errorf("alternate-access-port = %v, want absent so Aerospike falls back to the service port", got)
				}
			} else if got != tt.wantPort {
				t.Errorf("alternate-access-port = %v, want %v", got, tt.wantPort)
			}

			joined := strings.Join(overrides, "; ")
			if tt.wantOverride == "" {
				if len(overrides) != 0 {
					t.Errorf("overrides = %v, want none", overrides)
				}
			} else if !strings.Contains(joined, tt.wantOverride) {
				t.Errorf("overrides = %q, want one containing %q", joined, tt.wantOverride)
			}
		})
	}
}

// TestGenerateConfForPod_UserAlternatePortSurvives is the end-to-end check for
// the configuration issue #218 reports: alternateAccessType hostInternal, the
// NodePort Service created by hand (so spec.podService is unset), and
// alternate-access-port set in spec.aerospikeConfig.network.service.
func TestGenerateConfForPod_UserAlternatePortSurvives(t *testing.T) {
	config := externalAccessConfig(30000)
	config["service"] = map[string]any{"cluster-name": "demo"}

	InjectAccessAddressPlaceholders(config, hostInternalPolicy(), "")
	out, err := GenerateConfForPod(config, "demo-svc", "aerospike", []string{"demo-0-0"}, 3002)
	if err != nil {
		t.Fatalf("GenerateConfForPod() error = %v", err)
	}

	for _, want := range []string{
		"access-address MY_POD_IP",
		"alternate-access-address MY_NODE_IP",
		"alternate-access-port 30000",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("generated aerospike.conf is missing %q:\n%s", want, out)
		}
	}
}
