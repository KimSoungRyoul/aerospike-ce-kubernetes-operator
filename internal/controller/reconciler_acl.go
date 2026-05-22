package controller

import (
	"context"
	"fmt"
	"strings"

	aero "github.com/aerospike/aerospike-client-go/v8"
	corev1 "k8s.io/api/core/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	ackov1alpha1 "github.com/ksr/aerospike-ce-kubernetes-operator/api/v1alpha1"
	ackoerrors "github.com/ksr/aerospike-ce-kubernetes-operator/internal/errors"
	"github.com/ksr/aerospike-ce-kubernetes-operator/internal/metrics"
	"github.com/ksr/aerospike-ce-kubernetes-operator/internal/utils"
)

// builtinRoles are Aerospike predefined roles that must not be dropped.
var builtinRoles = map[string]bool{
	"user-admin":     true,
	"sys-admin":      true,
	"data-admin":     true,
	"read":           true,
	"write":          true,
	"read-write":     true,
	"read-write-udf": true,
	"truncate":       true,
}

// reconcileACL synchronizes ACL roles and users with the Aerospike cluster.
// Returns (synced bool, err error) where synced indicates whether ACL was
// actually applied (false when skipped due to no ready pods or unchanged spec).
func (r *AerospikeClusterReconciler) reconcileACL(
	ctx context.Context,
	cluster *ackov1alpha1.AerospikeCluster,
) (bool, error) {
	log := logf.FromContext(ctx).WithValues("cluster", cluster.Name)

	if cluster.Spec.AerospikeAccessControl == nil {
		return false, nil
	}

	log.V(1).Info("Starting ACL sync",
		"namespace", cluster.Namespace,
		"generation", cluster.Generation,
		"observedGeneration", cluster.Status.ObservedGeneration)

	// Check if any pod is ready before attempting ACL sync.
	podList, err := r.listClusterPods(ctx, cluster)
	if err != nil {
		return false, fmt.Errorf("listing pods for ACL sync: %w", err)
	}

	podReady := false
	for i := range podList.Items {
		if isPodReady(&podList.Items[i]) {
			podReady = true
			break
		}
	}
	if !podReady {
		log.Info("No ready pods, skipping ACL sync")
		return false, nil
	}

	r.Recorder.Eventf(cluster, corev1.EventTypeNormal, EventACLSyncStarted,
		"ACL synchronization started")

	// getClient hands out an Aerospike client for both the first attempt and the
	// transient-error retry. Every client it creates is tracked and closed when
	// reconcileACL returns, so the retry path never reuses a stale broken
	// connection and never leaks a connection either.
	var openClients []*aero.Client
	defer func() {
		for _, c := range openClients {
			closeAerospikeClient(c)
		}
	}()
	getClient := func(forRetry bool) (*aero.Client, error) {
		c, cErr := r.getAerospikeClientWithRetry(ctx, cluster)
		if cErr != nil {
			return nil, cErr
		}
		openClients = append(openClients, c)
		return c, nil
	}

	// Sync roles first (users may depend on roles).
	// Retry once on transient connection errors (connection reset, timeout, etc.)
	// that may occur during rolling restarts when nodes are briefly unavailable.
	// The retry uses a freshly-created client so a stale broken connection is
	// not reused.
	if err := retryOnTransientWithReconnect(getClient, func(c *aero.Client) error {
		return r.reconcileRoles(ctx, c, cluster)
	}); err != nil {
		metrics.ACLSyncTotal.WithLabelValues(cluster.Namespace, cluster.Name, "error").Inc()
		return false, fmt.Errorf("ACL sync roles: %w", err)
	}

	// Sync users with the same single-retry pattern for transient errors.
	if err := retryOnTransientWithReconnect(getClient, func(c *aero.Client) error {
		return r.reconcileUsers(ctx, c, cluster)
	}); err != nil {
		metrics.ACLSyncTotal.WithLabelValues(cluster.Namespace, cluster.Name, "error").Inc()
		return false, fmt.Errorf("ACL sync users: %w", err)
	}

	metrics.ACLSyncTotal.WithLabelValues(cluster.Namespace, cluster.Name, "success").Inc()
	r.Recorder.Eventf(cluster, corev1.EventTypeNormal, EventACLSyncCompleted,
		"ACL synchronized successfully")
	log.Info("ACL reconciliation completed")
	return true, nil
}

// reconcileRoles synchronizes roles: create missing, grant/revoke privileges, drop orphaned.
func (r *AerospikeClusterReconciler) reconcileRoles(
	ctx context.Context,
	aeroClient *aero.Client,
	cluster *ackov1alpha1.AerospikeCluster,
) error {
	log := logf.FromContext(ctx)

	adminPolicy := newAdminPolicy()

	// Query existing roles
	existingRoles, err := aeroClient.QueryRoles(adminPolicy)
	if err != nil {
		return fmt.Errorf("querying roles: %w", err)
	}

	existingRoleMap := make(map[string]*aero.Role, len(existingRoles))
	for _, role := range existingRoles {
		existingRoleMap[role.Name] = role
	}

	// Build desired role set
	desiredRoles := make(map[string]bool)
	for _, roleSpec := range cluster.Spec.AerospikeAccessControl.Roles {
		desiredRoles[roleSpec.Name] = true

		existing, exists := existingRoleMap[roleSpec.Name]
		if !exists {
			// Create the role
			privileges, err := roleParsedPrivileges(roleSpec)
			if err != nil {
				return ackoerrors.NewValidationf("parsing privileges for role %s: %v", roleSpec.Name, err)
			}
			log.Info("Creating role", "role", roleSpec.Name)
			if err := aeroClient.CreateRole(adminPolicy, roleSpec.Name, privileges, nil, 0, 0); err != nil {
				return fmt.Errorf("creating role %s: %w", roleSpec.Name, err)
			}
			continue
		}

		// Sync privileges: grant missing, revoke extra
		desiredPrivs, err := roleParsedPrivileges(roleSpec)
		if err != nil {
			return ackoerrors.NewValidationf("parsing privileges for role %s: %v", roleSpec.Name, err)
		}
		if err := syncRolePrivileges(aeroClient, adminPolicy, roleSpec.Name, existing.Privileges, desiredPrivs, log); err != nil {
			return fmt.Errorf("syncing privileges for role %s: %w", roleSpec.Name, err)
		}
	}

	// Drop orphaned custom roles (protect builtins)
	for _, role := range existingRoles {
		if !desiredRoles[role.Name] && !builtinRoles[role.Name] {
			log.Info("Dropping orphaned role", "role", role.Name)
			if err := aeroClient.DropRole(adminPolicy, role.Name); err != nil {
				return fmt.Errorf("dropping role %s: %w", role.Name, err)
			}
		}
	}

	return nil
}

// reconcileUsers synchronizes users: create missing, grant/revoke roles, change passwords, drop orphaned.
func (r *AerospikeClusterReconciler) reconcileUsers(
	ctx context.Context,
	aeroClient *aero.Client,
	cluster *ackov1alpha1.AerospikeCluster,
) error {
	log := logf.FromContext(ctx)

	adminPolicy := newAdminPolicy()

	// Query existing users
	existingUsers, err := aeroClient.QueryUsers(adminPolicy)
	if err != nil {
		return fmt.Errorf("querying users: %w", err)
	}

	existingUserMap := make(map[string]*aero.UserRoles, len(existingUsers))
	for _, user := range existingUsers {
		existingUserMap[user.User] = user
	}

	desiredUsers := make(map[string]bool)
	for _, userSpec := range cluster.Spec.AerospikeAccessControl.Users {
		desiredUsers[userSpec.Name] = true

		// Read desired password
		password, err := r.getPasswordFromSecret(ctx, cluster.Namespace, userSpec.SecretName)
		if err != nil {
			return fmt.Errorf("getting password for user %s: %w", userSpec.Name, err)
		}

		existing, exists := existingUserMap[userSpec.Name]
		if !exists {
			// Create user
			log.Info("Creating user", "user", userSpec.Name)
			if err := aeroClient.CreateUser(adminPolicy, userSpec.Name, password, userSpec.Roles); err != nil {
				return fmt.Errorf("creating user %s: %w", userSpec.Name, err)
			}
			continue
		}

		// Sync roles
		if err := syncUserRoles(aeroClient, adminPolicy, userSpec.Name, existing.Roles, userSpec.Roles, log); err != nil {
			return fmt.Errorf("syncing roles for user %s: %w", userSpec.Name, err)
		}

		// Always attempt password change. The Aerospike server's ChangePassword
		// is idempotent — it's a no-op when the password hasn't changed.
		// This ensures Secret-based password updates are applied even though
		// Secret changes don't increment the CR's generation.
		//
		// A failure here must be returned, not swallowed: if it were only
		// logged, reconcileACL would still report success and ACLSynced would
		// be set True even though the password was never rotated. Returning the
		// error propagates it up through reconcileACL so ACLSynced is set False
		// with a meaningful reason. Transient errors are retried by the
		// retryOnTransientWithClient wrapper around reconcileUsers.
		if err := aeroClient.ChangePassword(adminPolicy, userSpec.Name, password); err != nil {
			return fmt.Errorf("changing password for user %s: %w", userSpec.Name, err)
		}
	}

	// Drop orphaned users (protect the admin user the operator connects as).
	// Also protect the previously-configured admin user when the admin user name is
	// being changed: if we dropped the old admin before the new one is fully operational,
	// the operator would lose its connection credentials on the next reconcile.
	adminUser := utils.FindAdminUser(cluster.Spec.AerospikeAccessControl)
	var prevAdminUser *ackov1alpha1.AerospikeUserSpec
	if cluster.Status.AppliedSpec != nil {
		prevAdminUser = utils.FindAdminUser(cluster.Status.AppliedSpec.AerospikeAccessControl)
	}
	for _, user := range existingUsers {
		if !desiredUsers[user.User] {
			// Protect the connected admin user.
			if adminUser != nil && user.User == adminUser.Name {
				continue
			}
			// Protect the previously-configured admin user during a rename to avoid
			// locking the operator out before the new admin user is established.
			// Guard adminUser != nil: if the new spec has no admin user, this rename
			// branch should be skipped (the previous admin will be dropped normally).
			if prevAdminUser != nil && adminUser != nil && user.User == prevAdminUser.Name && prevAdminUser.Name != adminUser.Name {
				log.Info("Skipping drop of previous admin user during rename — ensure new admin is operational before next reconcile",
					"previousAdmin", prevAdminUser.Name, "newAdmin", adminUser.Name)
				r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventACLSyncError,
					"Admin user rename detected: previous admin %q not dropped yet. Ensure new admin %q is operational, then remove the old user manually.",
					prevAdminUser.Name, adminUser.Name)
				continue
			}
			log.Info("Dropping orphaned user", "user", user.User)
			if err := aeroClient.DropUser(adminPolicy, user.User); err != nil {
				return fmt.Errorf("dropping user %s: %w", user.User, err)
			}
		}
	}

	return nil
}

// syncRolePrivileges grants missing and revokes extra privileges for a role.
func syncRolePrivileges(
	aeroClient *aero.Client,
	policy *aero.AdminPolicy,
	roleName string,
	current, desired []aero.Privilege,
	log interface {
		Info(msg string, keysAndValues ...any)
	},
) error {
	currentSet := privilegeSet(current)
	desiredSet := privilegeSet(desired)

	// Grant missing
	var toGrant []aero.Privilege
	for key, priv := range desiredSet {
		if _, ok := currentSet[key]; !ok {
			toGrant = append(toGrant, priv)
		}
	}
	if len(toGrant) > 0 {
		log.Info("Granting privileges to role", "role", roleName, "count", len(toGrant))
		if err := aeroClient.GrantPrivileges(policy, roleName, toGrant); err != nil {
			return fmt.Errorf("granting privileges to role %s: %w", roleName, err)
		}
	}

	// Revoke extra
	var toRevoke []aero.Privilege
	for key, priv := range currentSet {
		if _, ok := desiredSet[key]; !ok {
			toRevoke = append(toRevoke, priv)
		}
	}
	if len(toRevoke) > 0 {
		log.Info("Revoking privileges from role", "role", roleName, "count", len(toRevoke))
		if err := aeroClient.RevokePrivileges(policy, roleName, toRevoke); err != nil {
			return fmt.Errorf("revoking privileges from role %s: %w", roleName, err)
		}
	}

	return nil
}

// syncUserRoles grants missing and revokes extra roles for a user.
func syncUserRoles(
	aeroClient *aero.Client,
	policy *aero.AdminPolicy,
	userName string,
	currentRoles, desiredRoles []string,
	log interface {
		Info(msg string, keysAndValues ...any)
	},
) error {
	currentSet := sliceToSet(currentRoles)
	desiredSet := sliceToSet(desiredRoles)

	// Grant missing
	var toGrant []string
	for role := range desiredSet {
		if !currentSet[role] {
			toGrant = append(toGrant, role)
		}
	}
	if len(toGrant) > 0 {
		log.Info("Granting roles to user", "user", userName, "roles", toGrant)
		if err := aeroClient.GrantRoles(policy, userName, toGrant); err != nil {
			return fmt.Errorf("granting roles to user %s: %w", userName, err)
		}
	}

	// Revoke extra
	var toRevoke []string
	for role := range currentSet {
		if !desiredSet[role] {
			toRevoke = append(toRevoke, role)
		}
	}
	if len(toRevoke) > 0 {
		log.Info("Revoking roles from user", "user", userName, "roles", toRevoke)
		if err := aeroClient.RevokeRoles(policy, userName, toRevoke); err != nil {
			return fmt.Errorf("revoking roles from user %s: %w", userName, err)
		}
	}

	return nil
}

// roleParsedPrivileges converts the spec's privilege strings to aero.Privilege objects.
func roleParsedPrivileges(roleSpec ackov1alpha1.AerospikeRoleSpec) ([]aero.Privilege, error) {
	privileges := make([]aero.Privilege, 0, len(roleSpec.Privileges))
	for _, privStr := range roleSpec.Privileges {
		priv, err := parsePrivilege(privStr)
		if err != nil {
			return nil, fmt.Errorf("role %q: %w", roleSpec.Name, err)
		}
		privileges = append(privileges, priv)
	}
	return privileges, nil
}

// parsePrivilege converts a privilege string like "read-write.testNamespace" to an aero.Privilege.
// Since the Code field type (privilegeCode) is unexported in aerospike-client-go,
// we construct a Privilege using exported constants (aero.Read, aero.Write, etc.).
func parsePrivilege(s string) (aero.Privilege, error) {
	// Format: "<code>" or "<code>.<namespace>" or "<code>.<namespace>.<set>"
	parts := strings.SplitN(s, ".", 3)

	// Start with a base privilege from the code string
	priv, err := privilegeFromCodeString(parts[0])
	if err != nil {
		return aero.Privilege{}, err
	}

	if len(parts) >= 2 {
		priv.Namespace = parts[1]
	}
	if len(parts) >= 3 {
		priv.SetName = parts[2]
	}

	return priv, nil
}

// privilegeFromCodeString returns an aero.Privilege with the Code matching the string.
// This works around the unexported privilegeCode type by using exported constants.
// Returns an error for unknown privilege codes instead of silently degrading.
func privilegeFromCodeString(s string) (aero.Privilege, error) {
	switch s {
	case "read":
		return aero.Privilege{Code: aero.Read}, nil
	case "write":
		return aero.Privilege{Code: aero.Write}, nil
	case "read-write":
		return aero.Privilege{Code: aero.ReadWrite}, nil
	case "read-write-udf":
		return aero.Privilege{Code: aero.ReadWriteUDF}, nil
	case "sys-admin":
		return aero.Privilege{Code: aero.SysAdmin}, nil
	case "user-admin":
		return aero.Privilege{Code: aero.UserAdmin}, nil
	case "data-admin":
		return aero.Privilege{Code: aero.DataAdmin}, nil
	case "truncate":
		return aero.Privilege{Code: aero.Truncate}, nil
	default:
		return aero.Privilege{}, fmt.Errorf("unknown privilege code %q; valid codes: read, write, read-write, read-write-udf, sys-admin, user-admin, data-admin, truncate", s)
	}
}

// privilegeKey returns a string key for a privilege for set comparison.
func privilegeKey(p aero.Privilege) string {
	return fmt.Sprintf("%s:%s:%s", p.Code, p.Namespace, p.SetName)
}

// privilegeSet converts a slice of privileges to a map keyed by privilegeKey.
func privilegeSet(privs []aero.Privilege) map[string]aero.Privilege {
	set := make(map[string]aero.Privilege, len(privs))
	for _, p := range privs {
		set[privilegeKey(p)] = p
	}
	return set
}

// isTransientAeroError checks if an Aerospike error is transient and worth retrying.
// This includes connection resets, timeouts, and cluster-not-ready errors.
func isTransientAeroError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "i/o timeout") ||
		strings.Contains(msg, "connection refused")
}

// newAdminPolicy returns an AdminPolicy with the standard operator timeout.
func newAdminPolicy() *aero.AdminPolicy {
	p := aero.NewAdminPolicy()
	p.Timeout = aeroInfoTimeout
	return p
}

// sliceToSet converts a string slice to a set.
func sliceToSet(ss []string) map[string]bool {
	set := make(map[string]bool, len(ss))
	for _, s := range ss {
		set[s] = true
	}
	return set
}
