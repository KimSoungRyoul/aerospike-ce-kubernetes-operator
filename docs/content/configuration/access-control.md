---
sidebar_position: 3
title: Access Control (ACL)
---

# Access Control (ACL)

This guide covers configuring authentication and authorization for Aerospike CE clusters using the operator.

## Overview

Aerospike CE supports access control (ACL) to restrict who can connect to the cluster and what operations they can perform. When ACL is enabled:

- All client connections must authenticate with a username and password
- Each user is assigned one or more **roles** that define their permissions
- The operator manages role and user creation via the Aerospike admin API

:::warning
Aerospike CE 8.x does not support the `security` stanza in `aerospike.conf`. ACL is managed entirely through the operator's `aerospikeAccessControl` spec, which uses the Aerospike admin API to configure users and roles at runtime.
:::

## Prerequisites

### Create Kubernetes Secrets

Each user's password must be stored in a Kubernetes Secret. The Secret must contain a `password` key:

```bash
# Create a secret for the admin user
kubectl -n aerospike create secret generic admin-secret \
  --from-literal=password='admin-password-here'

# Create a secret for an application user
kubectl -n aerospike create secret generic app-secret \
  --from-literal=password='app-password-here'
```

## Basic ACL Configuration

The minimal ACL configuration requires at least one user with both `sys-admin` and `user-admin` roles:

```yaml
apiVersion: acko.io/v1alpha1
kind: AerospikeCluster
metadata:
  name: aerospike-acl
  namespace: aerospike
spec:
  size: 3
  image: aerospike:ce-8.1.1.1
  aerospikeAccessControl:
    users:
      - name: admin
        secretName: admin-secret
        roles:
          - sys-admin
          - user-admin
      - name: appuser
        secretName: app-secret
        roles:
          - read-write
  aerospikeConfig:
    service:
      cluster-name: aerospike-acl
    namespaces:
      - name: test
        replication-factor: 2
        storage-engine:
          type: memory
```

## Built-in Roles

Aerospike CE provides the following predefined roles. These can be assigned directly to users without defining them in the `roles` list:

| Role | Description |
|------|-------------|
| `user-admin` | Create/drop users, grant/revoke roles |
| `sys-admin` | Cluster administration (truncate, config, info commands) |
| `data-admin` | Index management (create/drop secondary indexes, UDFs) |
| `read` | Read records |
| `write` | Write (insert/update/delete) records |
| `read-write` | Read and write records |
| `read-write-udf` | Read, write, and execute UDFs |
| `truncate` | Truncate namespaces/sets |

:::info
The `superuser` role exists only in Aerospike Enterprise Edition. CE clusters must use the built-in roles listed above or define custom roles.
:::

## Custom Roles

Define custom roles with fine-grained privileges:

```yaml
spec:
  aerospikeAccessControl:
    roles:
      - name: inventory-reader
        privileges:
          - read.inventory           # Read on 'inventory' namespace
      - name: orders-writer
        privileges:
          - read-write.orders        # Read-write on 'orders' namespace
          - read-write.orders.items  # Read-write on 'orders.items' set
    users:
      - name: admin
        secretName: admin-secret
        roles:
          - sys-admin
          - user-admin
      - name: inventory-svc
        secretName: inventory-secret
        roles:
          - inventory-reader
      - name: orders-svc
        secretName: orders-secret
        roles:
          - orders-writer
```

### Privilege Format

Privileges follow the format: `<code>[.<namespace>[.<set>]]`

| Code | Description |
|------|-------------|
| `read` | Read records |
| `write` | Write records |
| `read-write` | Read and write records |
| `read-write-udf` | Read, write, and execute UDFs |
| `sys-admin` | System administration |
| `user-admin` | User administration |
| `data-admin` | Data administration |
| `truncate` | Truncate data |

**Examples:**
- `read` — global read on all namespaces
- `read-write.myns` — read-write on the `myns` namespace
- `write.myns.myset` — write on the `myset` set within `myns`

## Role Whitelists (IP Allowlisting)

Each custom role can include a `whitelist` field to restrict which client IP addresses are allowed to authenticate with that role. Whitelisted CIDRs apply at the Aerospike server level, providing an additional layer of network-based access control beyond Kubernetes NetworkPolicy.

```yaml
spec:
  aerospikeAccessControl:
    roles:
      - name: internal-reader
        privileges:
          - read.data
        whitelist:
          - "10.0.0.0/8"         # Allow only internal network
          - "172.16.0.0/12"
      - name: monitoring-role
        privileges:
          - read
        whitelist:
          - "10.100.0.0/16"      # Allow only the monitoring subnet
    users:
      - name: admin
        secretName: admin-secret
        roles:
          - sys-admin
          - user-admin
      - name: internal-app
        secretName: internal-app-secret
        roles:
          - internal-reader
      - name: prometheus
        secretName: prometheus-secret
        roles:
          - monitoring-role
```

**When to use whitelists:**

- Restrict database access to specific application subnets
- Limit monitoring access to the Prometheus/Grafana network range
- Add defense-in-depth alongside Kubernetes NetworkPolicy

:::note
Whitelists apply per role. If a user has multiple roles, the user can connect from any IP allowed by any of their assigned roles. Built-in roles (e.g., `read-write`) do not support whitelists -- you must create a custom role to use this feature.
:::

## Admin Policy

The `adminPolicy` field configures the timeout for admin API operations (user/role creation, password changes, etc.) performed by the operator. This is useful when the Aerospike cluster is under heavy load and admin operations may take longer than the default 2-second timeout.

```yaml
spec:
  aerospikeAccessControl:
    adminPolicy:
      timeout: 5000    # 5 seconds (default: 2000ms)
    users:
      - name: admin
        secretName: admin-secret
        roles:
          - sys-admin
          - user-admin
```

| Field | Default | Range | Description |
|-------|---------|-------|-------------|
| `timeout` | `2000` | 100 -- 30000 ms | Admin operation timeout in milliseconds |

Increase this value if you see `ACLSyncError` events with timeout-related error messages, especially during initial cluster creation with many users/roles or under heavy load.

## Password Rotation

The operator watches Kubernetes Secrets referenced by `aerospikeAccessControl.users[*].secretName`. When a Secret's data changes, the operator automatically syncs the updated password to Aerospike without requiring any change to the AerospikeCluster CR.

```bash
# Rotate a user's password by updating the Secret
kubectl -n aerospike create secret generic app-secret \
  --from-literal=password='new-password-here' \
  --dry-run=client -o yaml | kubectl apply -f -
```

Verify the sync via events:

```bash
kubectl get events --field-selector reason=ACLSyncStarted -n aerospike
kubectl get events --field-selector reason=ACLSyncCompleted -n aerospike
```

:::info
Only Secrets actively referenced by an AerospikeCluster's ACL configuration trigger reconciliation. Unrelated Secret changes in the same namespace are ignored.
:::

## Webhook Validation Rules

The operator's admission webhook enforces the following ACL rules:

1. **Admin user required**: At least one user must have both `sys-admin` and `user-admin` roles. Without this, the operator cannot manage ACL after initial setup.

2. **Role cross-validation**: Every role referenced by a user must be either a built-in role or explicitly defined in the `roles` list. Referencing an undefined role is rejected.

3. **Privilege code validation**: Each privilege in a custom role must use a valid privilege code. Invalid codes are rejected with a descriptive error.

4. **Secret requirement**: Every user must have a `secretName` pointing to a Kubernetes Secret containing the password.

## Troubleshooting

### Common Validation Errors

**"must have at least one user with both 'sys-admin' and 'user-admin' roles"**

Ensure at least one user has both roles:

```yaml
users:
  - name: admin
    secretName: admin-secret
    roles:
      - sys-admin
      - user-admin    # Both roles required on the same user
```

**"user X references undefined role Y"**

The role must be a built-in role or defined in the `roles` list:

```yaml
roles:
  - name: custom-role       # Define the custom role
    privileges:
      - read.myns
users:
  - name: myuser
    secretName: myuser-secret
    roles:
      - custom-role          # Now this reference is valid
```

**"role X has invalid privilege code Y"**

Use only valid privilege codes (`read`, `write`, `read-write`, `read-write-udf`, `sys-admin`, `user-admin`, `data-admin`, `truncate`):

```yaml
roles:
  - name: my-role
    privileges:
      - read-write.myns     # Valid
      # - admin.myns        # Invalid — use 'sys-admin' or 'data-admin'
```

### Checking ACL Status

After deployment, verify ACL is working:

```bash
# Check operator logs for ACL sync
kubectl -n aerospike-operator logs -l control-plane=controller-manager | grep -i acl

# Connect to the cluster with credentials
kubectl -n aerospike exec -it aerospike-acl-0-0 -- asadm -Uadmin -Padmin-password-here

# List users (inside asadm)
manage acl show users
```
