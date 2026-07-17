package utils

import (
	"testing"

	v1alpha1 "github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/api/v1alpha1"
)

func TestFindAdminUser(t *testing.T) {
	tests := []struct {
		name     string
		acl      *v1alpha1.AerospikeAccessControlSpec
		wantName string
		wantNil  bool
	}{
		{
			name:    "nil ACL returns nil",
			acl:     nil,
			wantNil: true,
		},
		{
			name: "ACL with no users returns nil",
			acl: &v1alpha1.AerospikeAccessControlSpec{
				Users: []v1alpha1.AerospikeUserSpec{},
			},
			wantNil: true,
		},
		{
			name: "user with only sys-admin role returns nil",
			acl: &v1alpha1.AerospikeAccessControlSpec{
				Users: []v1alpha1.AerospikeUserSpec{
					{
						Name:       "admin",
						SecretName: "admin-secret",
						Roles:      []string{"sys-admin"},
					},
				},
			},
			wantNil: true,
		},
		{
			name: "user with only user-admin role returns nil",
			acl: &v1alpha1.AerospikeAccessControlSpec{
				Users: []v1alpha1.AerospikeUserSpec{
					{
						Name:       "admin",
						SecretName: "admin-secret",
						Roles:      []string{"user-admin"},
					},
				},
			},
			wantNil: true,
		},
		{
			name: "user with both sys-admin and user-admin roles returns that user",
			acl: &v1alpha1.AerospikeAccessControlSpec{
				Users: []v1alpha1.AerospikeUserSpec{
					{
						Name:       "admin",
						SecretName: "admin-secret",
						Roles:      []string{"sys-admin", "user-admin"},
					},
				},
			},
			wantNil:  false,
			wantName: "admin",
		},
		{
			name: "user with both roles plus additional roles returns that user",
			acl: &v1alpha1.AerospikeAccessControlSpec{
				Users: []v1alpha1.AerospikeUserSpec{
					{
						Name:       "superadmin",
						SecretName: "superadmin-secret",
						Roles:      []string{"read-write", "sys-admin", "user-admin", "data-admin"},
					},
				},
			},
			wantNil:  false,
			wantName: "superadmin",
		},
		{
			name: "multiple users, second one qualifies returns the qualifying user",
			acl: &v1alpha1.AerospikeAccessControlSpec{
				Users: []v1alpha1.AerospikeUserSpec{
					{
						Name:       "reader",
						SecretName: "reader-secret",
						Roles:      []string{"read"},
					},
					{
						Name:       "admin",
						SecretName: "admin-secret",
						Roles:      []string{"sys-admin", "user-admin"},
					},
					{
						Name:       "writer",
						SecretName: "writer-secret",
						Roles:      []string{"read-write"},
					},
				},
			},
			wantNil:  false,
			wantName: "admin",
		},
		{
			name: "multiple users, none qualify returns nil",
			acl: &v1alpha1.AerospikeAccessControlSpec{
				Users: []v1alpha1.AerospikeUserSpec{
					{
						Name:       "sysadmin",
						SecretName: "sysadmin-secret",
						Roles:      []string{"sys-admin"},
					},
					{
						Name:       "useradmin",
						SecretName: "useradmin-secret",
						Roles:      []string{"user-admin"},
					},
				},
			},
			wantNil: true,
		},
		{
			name: "user with empty roles returns nil",
			acl: &v1alpha1.AerospikeAccessControlSpec{
				Users: []v1alpha1.AerospikeUserSpec{
					{
						Name:       "empty",
						SecretName: "empty-secret",
						Roles:      []string{},
					},
				},
			},
			wantNil: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FindAdminUser(tc.acl)
			if tc.wantNil {
				if got != nil {
					t.Errorf("FindAdminUser() = %v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("FindAdminUser() = nil, want non-nil")
			}
			if got.Name != tc.wantName {
				t.Errorf("FindAdminUser().Name = %q, want %q", got.Name, tc.wantName)
			}
		})
	}
}

func TestFindAdminUser_ReturnsPointerToOriginal(t *testing.T) {
	acl := &v1alpha1.AerospikeAccessControlSpec{
		Users: []v1alpha1.AerospikeUserSpec{
			{
				Name:       "admin",
				SecretName: "admin-secret",
				Roles:      []string{"sys-admin", "user-admin"},
			},
		},
	}

	got := FindAdminUser(acl)
	if got == nil {
		t.Fatal("FindAdminUser() = nil, want non-nil")
	}

	// Verify the returned pointer points to the original slice element
	if got != &acl.Users[0] {
		t.Error("FindAdminUser() should return a pointer to the original slice element")
	}
}
