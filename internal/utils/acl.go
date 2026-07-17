package utils

import (
	v1alpha1 "github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/api/v1alpha1"
)

// FindAdminUser returns the first user that has both "sys-admin" and "user-admin"
// roles, which is the user the operator uses to manage ACL and authenticate
// the Prometheus exporter.
func FindAdminUser(acl *v1alpha1.AerospikeAccessControlSpec) *v1alpha1.AerospikeUserSpec {
	if acl == nil {
		return nil
	}
	for i, user := range acl.Users {
		hasSysAdmin, hasUserAdmin := false, false
		for _, role := range user.Roles {
			switch role {
			case "sys-admin":
				hasSysAdmin = true
			case "user-admin":
				hasUserAdmin = true
			}
		}
		if hasSysAdmin && hasUserAdmin {
			return &acl.Users[i]
		}
	}
	return nil
}
