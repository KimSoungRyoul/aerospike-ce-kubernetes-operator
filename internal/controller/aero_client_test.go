package controller

import (
	"testing"

	ackov1alpha1 "github.com/ksr/aerospike-ce-kubernetes-operator/api/v1alpha1"
)

func TestGetServicePort(t *testing.T) {
	tests := []struct {
		name    string
		cluster *ackov1alpha1.AerospikeCluster
		want    int
	}{
		{
			name:    "nil AerospikeConfig returns default",
			cluster: &ackov1alpha1.AerospikeCluster{},
			want:    defaultAeroPort,
		},
		{
			name: "empty config returns default",
			cluster: &ackov1alpha1.AerospikeCluster{
				Spec: ackov1alpha1.AerospikeClusterSpec{
					AerospikeConfig: &ackov1alpha1.AerospikeConfigSpec{
						Value: map[string]any{},
					},
				},
			},
			want: defaultAeroPort,
		},
		{
			name: "no network section returns default",
			cluster: &ackov1alpha1.AerospikeCluster{
				Spec: ackov1alpha1.AerospikeClusterSpec{
					AerospikeConfig: &ackov1alpha1.AerospikeConfigSpec{
						Value: map[string]any{
							"service": map[string]any{"cluster-name": "test"},
						},
					},
				},
			},
			want: defaultAeroPort,
		},
		{
			name: "no service in network returns default",
			cluster: &ackov1alpha1.AerospikeCluster{
				Spec: ackov1alpha1.AerospikeClusterSpec{
					AerospikeConfig: &ackov1alpha1.AerospikeConfigSpec{
						Value: map[string]any{
							"network": map[string]any{
								"heartbeat": map[string]any{"port": 3002},
							},
						},
					},
				},
			},
			want: defaultAeroPort,
		},
		{
			name: "custom port as int",
			cluster: &ackov1alpha1.AerospikeCluster{
				Spec: ackov1alpha1.AerospikeClusterSpec{
					AerospikeConfig: &ackov1alpha1.AerospikeConfigSpec{
						Value: map[string]any{
							"network": map[string]any{
								"service": map[string]any{"port": 4000},
							},
						},
					},
				},
			},
			want: 4000,
		},
		{
			name: "custom port as float64 (JSON deserialization)",
			cluster: &ackov1alpha1.AerospikeCluster{
				Spec: ackov1alpha1.AerospikeClusterSpec{
					AerospikeConfig: &ackov1alpha1.AerospikeConfigSpec{
						Value: map[string]any{
							"network": map[string]any{
								"service": map[string]any{"port": float64(5000)},
							},
						},
					},
				},
			},
			want: 5000,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := getServicePort(tc.cluster)
			if got != tc.want {
				t.Errorf("getServicePort() = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestGetAerospikeClientWithRetry_MethodSignature(t *testing.T) {
	// Verify the retry wrapper compiles with the correct method signature.
	// The assignment below fails at compile time if the signature changes.
	var r *AerospikeClusterReconciler
	fn := r.getAerospikeClientWithRetry
	// Use fn to satisfy the compiler.
	_ = fn
}

func TestBuildQuiesceCommand(t *testing.T) {
	tests := []struct {
		name     string
		cluster  *ackov1alpha1.AerospikeCluster
		port     int
		password string
		want     []string
	}{
		{
			name:    "no ACL — basic asinfo command",
			cluster: &ackov1alpha1.AerospikeCluster{},
			port:    3000,
			want:    []string{"asinfo", "-v", "quiesce:", "-h", "localhost", "-p", "3000"},
		},
		{
			name:    "custom port",
			cluster: &ackov1alpha1.AerospikeCluster{},
			port:    4000,
			want:    []string{"asinfo", "-v", "quiesce:", "-h", "localhost", "-p", "4000"},
		},
		{
			name: "ACL enabled — includes -U and -P flags",
			cluster: &ackov1alpha1.AerospikeCluster{
				Spec: ackov1alpha1.AerospikeClusterSpec{
					AerospikeAccessControl: &ackov1alpha1.AerospikeAccessControlSpec{
						Users: []ackov1alpha1.AerospikeUserSpec{
							{
								Name:       "admin",
								SecretName: "admin-secret",
								Roles:      []string{"sys-admin", "user-admin"},
							},
						},
					},
				},
			},
			port:     3000,
			password: "s3cret",
			want:     []string{"asinfo", "-v", "quiesce:", "-h", "localhost", "-p", "3000", "-U", "admin", "-P", "s3cret"},
		},
		{
			name: "ACL enabled but no admin user — no auth flags",
			cluster: &ackov1alpha1.AerospikeCluster{
				Spec: ackov1alpha1.AerospikeClusterSpec{
					AerospikeAccessControl: &ackov1alpha1.AerospikeAccessControlSpec{
						Users: []ackov1alpha1.AerospikeUserSpec{
							{
								Name:       "reader",
								SecretName: "reader-secret",
								Roles:      []string{"read"},
							},
						},
					},
				},
			},
			port: 3000,
			want: []string{"asinfo", "-v", "quiesce:", "-h", "localhost", "-p", "3000"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := buildQuiesceCommand(tc.cluster, tc.port, tc.password)
			if len(got) != len(tc.want) {
				t.Fatalf("buildQuiesceCommand() returned %d args, want %d: got %v, want %v",
					len(got), len(tc.want), got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("buildQuiesceCommand()[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}
