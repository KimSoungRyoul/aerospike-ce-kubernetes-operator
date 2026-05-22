package controller

import (
	"context"
	"testing"
	"time"

	ackov1alpha1 "github.com/ksr/aerospike-ce-kubernetes-operator/api/v1alpha1"
	ackoerrors "github.com/ksr/aerospike-ce-kubernetes-operator/internal/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
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

// TestGetAerospikeClientWithRetry_NoRetryOnValidationError verifies that the
// retry wrapper returns immediately on a permanent ValidationError instead of
// burning ~14s of exponential backoff (2s+4s+8s). A missing admin Secret on an
// ACL-enabled cluster surfaces such a validation error via getPasswordFromSecret.
func TestGetAerospikeClientWithRetry_NoRetryOnValidationError(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme(client-go) error = %v", err)
	}
	if err := ackov1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme(acko) error = %v", err)
	}

	cluster := &ackov1alpha1.AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo",
			Namespace: "default",
		},
		Spec: ackov1alpha1.AerospikeClusterSpec{
			AerospikeAccessControl: &ackov1alpha1.AerospikeAccessControlSpec{
				Users: []ackov1alpha1.AerospikeUserSpec{
					{
						Name:       "admin",
						SecretName: "missing-admin-secret", // not created in the fake client
						Roles:      []string{"sys-admin", "user-admin"},
					},
				},
			},
		},
	}

	r := &AerospikeClusterReconciler{
		Client: fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(cluster).
			Build(),
		Scheme: scheme,
	}

	start := time.Now()
	_, err := r.getAerospikeClientWithRetry(context.Background(), cluster)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("getAerospikeClientWithRetry() error = nil, want validation error")
	}
	if !ackoerrors.IsValidation(err) {
		t.Errorf("getAerospikeClientWithRetry() error is not a ValidationError: %v", err)
	}
	// With retries the wrapper would sleep at least 2s before the first retry.
	// A correct short-circuit returns near-instantly.
	if elapsed >= 2*time.Second {
		t.Errorf("getAerospikeClientWithRetry() took %v; expected immediate return on validation error", elapsed)
	}
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

func TestBuildAsinfoCommand(t *testing.T) {
	aclCluster := &ackov1alpha1.AerospikeCluster{
		Spec: ackov1alpha1.AerospikeClusterSpec{
			AerospikeAccessControl: &ackov1alpha1.AerospikeAccessControlSpec{
				Users: []ackov1alpha1.AerospikeUserSpec{
					{Name: "admin", SecretName: "admin-secret", Roles: []string{"sys-admin", "user-admin"}},
				},
			},
		},
	}

	tests := []struct {
		name     string
		verb     string
		cluster  *ackov1alpha1.AerospikeCluster
		port     int
		password string
		want     []string
	}{
		{
			name:    "recluster — no ACL",
			verb:    "recluster:",
			cluster: &ackov1alpha1.AerospikeCluster{},
			port:    3000,
			want:    []string{"asinfo", "-v", "recluster:", "-h", "localhost", "-p", "3000"},
		},
		{
			name:     "recluster — ACL enabled includes auth flags",
			verb:     "recluster:",
			cluster:  aclCluster,
			port:     3000,
			password: "s3cret",
			want:     []string{"asinfo", "-v", "recluster:", "-h", "localhost", "-p", "3000", "-U", "admin", "-P", "s3cret"},
		},
		{
			name:    "quiesce verb matches buildQuiesceCommand",
			verb:    "quiesce:",
			cluster: &ackov1alpha1.AerospikeCluster{},
			port:    4000,
			want:    []string{"asinfo", "-v", "quiesce:", "-h", "localhost", "-p", "4000"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := buildAsinfoCommand(tc.verb, tc.cluster, tc.port, tc.password)
			if len(got) != len(tc.want) {
				t.Fatalf("buildAsinfoCommand() returned %d args, want %d: got %v, want %v",
					len(got), len(tc.want), got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("buildAsinfoCommand()[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}
