---
sidebar_position: 4
title: Troubleshooting
---

# Troubleshooting

Start with the symptom table, then use the focused checks below to find and fix common ACKO failures.

## Symptom-Based Diagnosis

| Symptom | Check Command | Likely Cause | Resolution |
|---------|--------------|--------------|------------|
| Phase = `Error` | `kubectl get asc <name> -o jsonpath='{.status.lastReconcileError}'` | Invalid config, image pull failure, insufficient resources | Fix based on error message and re-apply |
| Phase = `WaitingForMigration` | `kubectl exec <pod> -- asinfo -v 'statistics' \| grep migrate` | Data migration in progress | Wait for completion (automatic) |
| Stuck at `InProgress` | `kubectl get pvc -n <ns> -l aerospike.io/cr-name=<name>` | PVC Pending, ImagePull failure, scheduling failure | Check StorageClass, image, and resources |
| `CircuitBreakerActive` event | `kubectl get asc <name> -o jsonpath='{.status.failedReconcileCount}'` | 10+ consecutive failures | Check `lastReconcileError`, fix root cause |
| Pod `CrashLoopBackOff` | `kubectl logs <pod> -c aerospike-server --previous` | Config parsing error, out of memory | Check server logs and fix config |
| Webhook rejects CR | Check `kubectl apply` error message | CE constraint violation | See [Validation Error Patterns](#validation-error-patterns) |
| `dynamicConfigStatus=Failed` | `kubectl get asc <name> -o jsonpath='{.status.pods}' \| jq '.[].dynamicConfigStatus'` | Non-dynamic parameter change | Set `enableDynamicConfigUpdate: false` to trigger rolling restart |
| `ReadinessGateBlocking` | `kubectl get pod <pod> -o jsonpath='{.status.conditions}' \| jq '.[]'` | Readiness gate not satisfied | Check pod Aerospike status and migration state |

## Common Issues and Solutions

### Cluster Stuck in InProgress

The cluster remains in the `InProgress` phase instead of moving to `Completed`.

**Possible causes:**

1. **PVC not binding** — The StorageClass does not exist or has no available capacity.
2. **Image pull failure** — The container image is not accessible from the cluster.
3. **Scheduling failure** — No nodes match the pod's scheduling constraints.
4. **Circuit breaker active** — The operator is backing off after repeated failures.

**Steps to diagnose:**

```bash
# Check cluster phase and reason
kubectl -n aerospike get asc <name> -o jsonpath='{.status.phase}{"\t"}{.status.phaseReason}'

# Check for pending PVCs
kubectl -n aerospike get pvc -l aerospike.io/cr-name=<name>

# Check pod events for scheduling or pull errors
kubectl -n aerospike describe pod <pod-name>

# Check operator logs
kubectl -n aerospike-operator logs -l control-plane=controller-manager | tail -50

# Check if circuit breaker is active
kubectl -n aerospike get asc <name> \
  -o jsonpath='{.status.failedReconcileCount}{"\t"}{.status.lastReconcileError}'
```

### Pods Not Ready (CrashLoopBackOff)

Aerospike pods start but crash repeatedly.

**Possible causes:**

1. **Invalid aerospikeConfig** — Namespace names, storage paths, or parameters are incorrect.
2. **Insufficient memory** — The container memory limit is too low for the configured namespaces.
3. **Storage path mismatch** — Configured file paths do not match mounted volumes.

**Steps to diagnose:**

```bash
# Check current crash logs
kubectl -n aerospike logs <pod-name> -c aerospike-server

# Check previous crash logs
kubectl -n aerospike logs <pod-name> -c aerospike-server --previous

# Check pod status details
kubectl -n aerospike describe pod <pod-name>
```

### Scaling Failures

Scale-up or scale-down does not complete.

**Scale-up issues:**
- Verify `spec.size` does not exceed 8 (CE limit).
- Check that `replication-factor` does not exceed the new cluster size.
- Ensure sufficient cluster resources (CPU, memory, storage).

**Scale-down issues:**
- The operator waits for data migrations to complete before removing pods.
- Check for `ScaleDownDeferred` events indicating migration is blocking scale-down.

```bash
# Check for scale-down deferral events
kubectl get events --field-selector reason=ScaleDownDeferred -n aerospike

# Check migration status
kubectl -n aerospike exec <pod-name> -c aerospike-server -- asinfo -v 'statistics' | grep migrate_partitions_remaining
```

### ACL Sync Failures

ACL roles or users fail to synchronize with the Aerospike cluster.

**Possible causes:**

1. **Missing Secret** — The referenced Kubernetes Secret does not exist.
2. **Missing password key** — The Secret does not contain a `password` key.
3. **No admin user** — No user has both `sys-admin` and `user-admin` roles.

**Steps to diagnose:**

```bash
# Check ACL sync events
kubectl get events --field-selector reason=ACLSyncError -n aerospike

# Verify the Secret exists and contains the password key
kubectl -n aerospike get secret <secret-name> -o jsonpath='{.data.password}' | base64 -d

# Check operator logs for ACL errors
kubectl -n aerospike-operator logs -l control-plane=controller-manager | grep -i acl
```

### Dynamic Config Failures

Configuration changes trigger a restart instead of applying dynamically.

**Possible causes:**

1. **`enableDynamicConfigUpdate` not set** — Dynamic updates are off by default.
2. **Static parameter changed** — Parameters like `replication-factor`, `storage-engine type`, and `name` always require a restart.
3. **Invalid characters** — Parameter values containing `;` or `:` are rejected by pre-flight validation.
4. **Partial failure with rollback** — If one change in a batch fails, the operator rolls back all applied changes across all pods and falls back to a cold restart.
5. **ConfigDegraded state** — If rollback itself fails, the cluster enters `ConfigDegraded` phase. The operator will attempt cold restart recovery.

**Steps to diagnose:**

```bash
# Check per-pod dynamic config status
kubectl -n aerospike get asc <name> -o jsonpath='{.status.pods}' | \
  jq '.[] | {name: .podName, dynamicConfig: .dynamicConfigStatus}'

# Check per-change details for a specific pod
kubectl -n aerospike get asc <name> -o jsonpath='{.status.pods.<pod-name>.dynamicConfigChanges}'

# Check for dynamic config events
kubectl get events --field-selector reason=DynamicConfigApplied -n aerospike
kubectl get events --field-selector reason=DynamicConfigDegraded -n aerospike
kubectl get events --field-selector reason=DynamicConfigRollback -n aerospike

# Check operator logs for 2PC and rollback activity
kubectl -n aerospike-operator logs -l control-plane=controller-manager | grep -i "rollback\|dynamic config\|2PC\|ConfigDegraded"
```

| Status | Meaning |
|--------|---------|
| `Applied` | Dynamic config applied successfully at runtime |
| `Failed` | Dynamic update failed — rolling restart will be triggered |
| `Pending` | Waiting for the operator to apply the change |
| (empty) | No dynamic config change was attempted |

If the cluster is in `ConfigDegraded` phase, check the `DynamicConfigDegraded` condition for details about which pods have inconsistent configuration.

## Circuit Breaker and Recovery

The operator includes a built-in circuit breaker to prevent excessive retries on persistently failing clusters.

### How It Works

After **10 consecutive reconciliation failures**, the operator enters a backoff state with exponential delays:

| Consecutive Failures | Backoff Delay |
|---------------------|---------------|
| 1 | 2 seconds |
| 2 | 4 seconds |
| 3 | 8 seconds |
| 5 | 32 seconds |
| 8+ | ~4.3 minutes (capped at 256 seconds) |

While the circuit breaker is active, a `CircuitBreakerActive` warning event is emitted with the failure count and last error.

:::info
**Permanent validation errors** (e.g., invalid Aerospike config structure, missing ACL secrets, invalid privilege codes) **immediately** activate the circuit breaker by setting `failedReconcileCount` to the maximum threshold. A `PermanentError` event is emitted, and the `ReconcileHealthy` status condition is set to `False` with reason `PermanentError`. These errors will never self-heal — fix the spec to recover.
:::

### Checking Circuit Breaker Status

```bash
# Check for active circuit breaker events
kubectl get events --field-selector reason=CircuitBreakerActive -n aerospike

# Check failure count and last error
kubectl -n aerospike get asc <name> \
  -o jsonpath='{.status.failedReconcileCount}{"\t"}{.status.lastReconcileError}'
```

### Resetting the Circuit Breaker

The circuit breaker resets automatically after a successful reconciliation. To trigger recovery:

1. **Fix the root cause** — Check `lastReconcileError` and resolve the underlying issue.
2. **Re-apply the corrected spec** — `kubectl apply -f <fixed-cr.yaml>`
3. **Verify reset** — Look for a `CircuitBreakerReset` event.

```bash
kubectl get events --field-selector reason=CircuitBreakerReset -n aerospike
```

**Manual reset via annotation:**

If you need to force an immediate reconcile (e.g., after fixing an external dependency), annotate the CR:

```bash
kubectl -n aerospike annotate asc <name> acko.io/force-reconcile=true --overwrite
```

The operator will clear the annotation after performing the reconciliation. This works even when the circuit breaker is active.

**Reset via Aerospike Cluster Manager UI:**

When the integrated UI is enabled (default; requires `ui.web.enabled=true`), the Reconciliation Health dashboard shows a **Reset Circuit Breaker** button that triggers the annotation-based reset with a single click.

## Debugging Commands

### Cluster Status

```bash
# List all clusters with phase
kubectl get asc -n <ns>

# Check specific cluster phase and reason
kubectl get asc <name> -o jsonpath='{.status.phase}'
kubectl get asc <name> -o jsonpath='{.status.phaseReason}'

# Check conditions
kubectl get asc <name> -o jsonpath='{.status.conditions}' | jq .

# Check circuit breaker state
kubectl get asc <name> -o jsonpath='{.status.failedReconcileCount}'
kubectl get asc <name> -o jsonpath='{.status.lastReconcileError}'
```

### Pod Status

```bash
# All pod statuses for a cluster
kubectl get asc <name> -o jsonpath='{.status.pods}' | jq .

# Ready pod count
kubectl get asc <name> -o jsonpath='{.status.size}'

# Pods pending restart
kubectl get asc <name> -o jsonpath='{.status.pendingRestartPods}'

# Template sync status
kubectl get asc <name> -o jsonpath='{.status.templateSnapshot.synced}'
```

### Events

```bash
# Events for a specific cluster (sorted by time)
kubectl get events -n <ns> --field-selector involvedObject.name=<name> --sort-by='.lastTimestamp'

# Watch events in real time
kubectl get events -n <ns> -w

# Filter by specific event reason
kubectl get events --field-selector reason=CircuitBreakerActive -n <ns>
kubectl get events --field-selector reason=ACLSyncError -n <ns>
kubectl get events --field-selector reason=RestartFailed -n <ns>
```

### Logs

```bash
# Operator logs
kubectl -n aerospike-operator logs -l control-plane=controller-manager -f

# Aerospike server logs (current)
kubectl -n <ns> logs <pod> -c aerospike-server -f

# Aerospike server logs (previous crash)
kubectl -n <ns> logs <pod> -c aerospike-server --previous
```

## Validation Error Patterns

The webhook validates CE constraints when creating or updating an AerospikeCluster. Below are common validation errors and how to fix them.

### Size and Image Errors

| Error Message | Cause | Fix |
|---------------|-------|-----|
| `spec.size N exceeds CE maximum of 8` | Cluster size exceeds CE limit | Set `spec.size` to 8 or fewer |
| `spec.image must not be empty` | No image specified and no templateRef | Set `spec.image` to a valid CE image |
| `spec.image "..." is an Enterprise Edition image` | Using an EE image tag | Use a Community Edition image (e.g., `aerospike:ce-8.1.1.1`) |

### Aerospike Config Errors

| Error Message | Cause | Fix |
|---------------|-------|-----|
| `must not contain 'xdr' section` | XDR is Enterprise-only | Remove the `xdr` section from `aerospikeConfig` |
| `must not contain 'tls' section` | TLS is Enterprise-only | Remove the `tls` section from `aerospikeConfig` |
| `namespaces count N exceeds CE maximum of 2` | More than 2 namespaces | Reduce to 2 or fewer namespaces |
| `heartbeat.mode must be 'mesh'` | Non-mesh heartbeat mode | Set `network.heartbeat.mode` to `mesh` |

### Enterprise-Only Namespace Keys

CE namespace configuration does not allow these keys:

`compression`, `compression-level`, `durable-delete`, `fast-restart`, `index-type`, `sindex-type`, `rack-id`, `strong-consistency`, `tomb-raider-eligible-age`, `tomb-raider-period`

Error format: `namespace[N] "name": 'key' is not allowed (reason)`

### ACL Validation Errors

| Error Message | Cause | Fix |
|---------------|-------|-----|
| `must have at least one user with both 'sys-admin' and 'user-admin' roles` | No admin user defined | Assign both roles to at least one user |
| `user "name" must have a secretName for password` | Missing password Secret reference | Add `secretName` to the user spec |
| `duplicate user name "name"` | Duplicate user names | Use unique names for each user |
| `user "name" references undefined role "role"` | Custom role not declared | Add the role to `aerospikeAccessControl.roles` or use a built-in role |

Valid privilege codes: `read`, `write`, `read-write`, `read-write-udf`, `sys-admin`, `user-admin`, `data-admin`, `truncate`

Privilege format: `"<code>"` / `"<code>.<namespace>"` / `"<code>.<namespace>.<set>"`

### Rack Config Validation Errors

| Error Message | Cause | Fix |
|---------------|-------|-----|
| `rack ID must be > 0` | Rack ID is 0 or negative | Use rack IDs starting from 1 |
| `duplicate rack ID N` | Same ID used in multiple racks | Use unique rack IDs |
| `duplicate rackLabel "label"` | Same label on multiple racks | Use unique rack labels |
| `rackConfig rack IDs cannot be changed` | Attempting to change rack IDs on update | Rack IDs are immutable after creation |

### Storage Validation Errors

| Error Message | Cause | Fix |
|---------------|-------|-----|
| `duplicate volume name "name"` | Same volume name used twice | Use unique volume names |
| `exactly one volume source must be specified` | Zero or multiple sources for a volume | Specify exactly one source (persistentVolume, emptyDir, etc.) |
| `persistentVolume.size must not be empty` | Missing PV size | Set a valid size (e.g., `10Gi`) |
| `aerospike.path must be an absolute path` | Relative path in volume mount | Use an absolute path (e.g., `/opt/aerospike/data`) |
| `subPath and subPathExpr are mutually exclusive` | Both set on the same mount | Use only one of `subPath` or `subPathExpr` |

### Namespace Validation Errors

| Error Message | Cause | Fix |
|---------------|-------|-----|
| `replication-factor must be between 1 and 4` | RF out of range | Set to a value between 1 and 4 |
| `replication-factor N exceeds cluster size M` | RF larger than node count | Lower RF or increase `spec.size` |

## Storage-Related Issues

### PVC Not Binding

PersistentVolumeClaims remain in `Pending` state.

```bash
# Check PVC status
kubectl -n aerospike get pvc -l aerospike.io/cr-name=<name>

# Check PVC events for details
kubectl -n aerospike describe pvc <pvc-name>

# Verify StorageClass exists
kubectl get sc
```

**Common causes:**
- StorageClass does not exist or is misconfigured.
- No PersistentVolumes available (for static provisioning).
- Insufficient storage capacity in the provisioner.
- Volume topology constraints prevent binding on the scheduled node.

### Cascade Delete Behavior

When `cascadeDelete: true` is set on a volume (or via global volume policy), PVCs are automatically deleted when:
- The AerospikeCluster CR is deleted.
- Pods are scaled down (after the pods have fully terminated).

**PVC cleanup during scale-down:**
- The operator waits for all scaled-down pods to terminate before deleting PVCs.
- If pods are stuck in `Terminating`, PVC cleanup is deferred to the next reconciliation.
- Check `PVCCleanedUp` and `PVCCleanupFailed` events for status.

```bash
# Check for stuck terminating pods
kubectl -n aerospike get pods | grep Terminating

# Check PVC cleanup events
kubectl get events --field-selector reason=PVCCleanedUp -n aerospike
kubectl get events --field-selector reason=PVCCleanupFailed -n aerospike
```

:::warning
PVCs without `cascadeDelete: true` are always preserved, even after CR deletion. You must delete them manually if no longer needed.
:::

### Local Storage Issues

When using local storage classes with `deleteLocalStorageOnRestart: true`:
- PVCs backed by local storage are deleted before pod deletion during cold restarts.
- This forces re-provisioning on the new node.
- If `deleteLocalStorageOnRestart` is not set, local PVCs persist and may block scheduling if the pod moves to a different node.

```bash
# Check for local PVC delete failures
kubectl get events --field-selector reason=LocalPVCDeleteFailed -n aerospike
```

## Network-Related Issues

### Pod Connectivity

If pods cannot connect to each other or clients cannot reach the cluster:

```bash
# Check pod IPs and readiness
kubectl -n aerospike get pods -o wide

# Verify the headless service
kubectl -n aerospike get svc

# Check Aerospike cluster mesh status
kubectl -n aerospike exec <pod-name> -c aerospike-server -- asinfo -v 'statistics' | grep cluster_size

# Check network endpoints in cluster status
kubectl -n aerospike get asc <name> -o jsonpath='{.status.pods}' | \
  jq '.[] | {pod: .podName, ip: .podIP, endpoints: .accessEndpoints}'
```

### Mesh Heartbeat Issues

The CE operator requires `heartbeat.mode` to be `mesh`. If nodes cannot form a cluster:

1. **Verify mesh mode** — Ensure `aerospikeConfig.network.heartbeat.mode` is set to `mesh`.
2. **Check mesh addresses** — The operator auto-configures mesh seed addresses via the headless service.
3. **DNS resolution** — Verify that pods can resolve the headless service DNS name.

```bash
# Check DNS resolution from within a pod
kubectl -n aerospike exec <pod-name> -c aerospike-server -- nslookup <headless-svc-name>

# Check Aerospike network info
kubectl -n aerospike exec <pod-name> -c aerospike-server -- asinfo -v 'mesh'
```

### Host Network Issues

When using `hostNetwork: true`:
- `multiPodPerHost` should be `false` to avoid port conflicts.
- `dnsPolicy` should be `ClusterFirstWithHostNet` for proper DNS resolution.
- The operator sets these defaults automatically, but mismatches trigger validation warnings.

## Event Reference

The operator emits Kubernetes Events for significant lifecycle transitions. Use these events to monitor cluster activity.

### Rolling Restart Events

| Reason | Type | Description |
|--------|------|-------------|
| `RollingRestartStarted` | Normal | Rolling restart loop began |
| `RollingRestartCompleted` | Normal | All targeted pods restarted |
| `PodWarmRestarted` | Normal | SIGUSR1 sent for config reload |
| `PodColdRestarted` | Normal | Pod deleted and recreated |
| `RestartFailed` | Warning | Pod restart failed |

### Configuration Events

| Reason | Type | Description |
|--------|------|-------------|
| `ConfigMapCreated` | Normal | Rack ConfigMap created |
| `ConfigMapUpdated` | Normal | ConfigMap content updated |
| `DynamicConfigApplied` | Normal | Runtime config change applied |
| `DynamicConfigStatusFailed` | Warning | Dynamic config change failed |
| `DynamicConfigDegraded` | Warning | Cluster entered ConfigDegraded phase |
| `DynamicConfigRollback` | Normal/Warning | Rollback result after failed batch apply |

### StatefulSet and Rack Events

| Reason | Type | Description |
|--------|------|-------------|
| `StatefulSetCreated` | Normal | Rack StatefulSet created |
| `StatefulSetUpdated` | Normal | StatefulSet spec updated |
| `RackScaled` | Normal | Rack pod count changed |
| `ScaleDownDeferred` | Warning | Scale-down blocked by data migration |

### ACL Events

| Reason | Type | Description |
|--------|------|-------------|
| `ACLSyncStarted` | Normal | ACL synchronization began |
| `ACLSyncCompleted` | Normal | ACL sync completed successfully |
| `ACLSyncError` | Warning | ACL sync encountered an error |

### Storage Events

| Reason | Type | Description |
|--------|------|-------------|
| `PVCCleanedUp` | Normal | Orphaned PVCs deleted after scale-down |
| `PVCCleanupFailed` | Warning | Failed to delete orphaned PVCs |
| `LocalPVCDeleteFailed` | Warning | Local PVC deletion failed before cold restart |

### Template Events

| Reason | Type | Description |
|--------|------|-------------|
| `TemplateApplied` | Normal | ClusterTemplate spec applied |
| `TemplateDrifted` | Warning | Cluster spec drifted from template |
| `TemplateResolutionError` | Warning | Failed to resolve a ClusterTemplate |

### Infrastructure Events

| Reason | Type | Description |
|--------|------|-------------|
| `PDBCreated` | Normal | PodDisruptionBudget created |
| `PDBUpdated` | Normal | PodDisruptionBudget updated |
| `ServiceCreated` | Normal | Headless service created |
| `ServiceUpdated` | Normal | Headless service updated |

### Lifecycle Events

| Reason | Type | Description |
|--------|------|-------------|
| `ClusterDeletionStarted` | Normal | Cluster teardown began |
| `FinalizerRemoved` | Normal | Finalizer removed, object will be deleted |
| `ReadinessGateSatisfied` | Normal | Pod readiness gate satisfied |
| `ReadinessGateBlocking` | Warning | Rolling restart blocked by readiness gate |

### Circuit Breaker Events

| Reason | Type | Description |
|--------|------|-------------|
| `CircuitBreakerActive` | Warning | Reconciliation backed off after consecutive failures |
| `CircuitBreakerReset` | Normal | Circuit breaker reset after success |
| `PermanentError` | Warning | Permanent validation error detected, automatic retries halted |

### Other Events

| Reason | Type | Description |
|--------|------|-------------|
| `ValidationWarning` | Warning | Non-blocking validation warning |
| `ReconcileError` | Warning | Reconciliation encountered an error |
| `Operation` | Normal | On-demand operation processed |

### Quiesce Events

| Reason | Type | Description |
|--------|------|-------------|
| `NodeQuiesceStarted` | Normal | Node quiesce started |
| `NodeQuiesced` | Normal | Node quiesce completed |
| `NodeQuiesceFailed` | Warning | Node quiesce failed |

## Operator Resource Sizing

The operator itself is lightweight, but incorrect sizing can cause slow reconciliation or OOM kills.

### Recommended Operator Resources

| Clusters Managed | CPU Request | CPU Limit | Memory Request | Memory Limit |
|-----------------|-------------|-----------|----------------|--------------|
| 1-5 | 100m | 500m | 128Mi | 256Mi |
| 5-20 | 250m | 1000m | 256Mi | 512Mi |
| 20+ | 500m | 2000m | 512Mi | 1Gi |

### Aerospike Pod Resource Guidelines

Memory limits must accommodate the sum of all namespace `data-size` values plus ~30% overhead for primary index, buffers, and internal structures:

```
Minimum Memory = Σ(namespace data-size) × 1.3
```

For example, a cluster with two 2GB memory namespaces needs at least `2 × 2GB × 1.3 = 5.2GB` memory limit.

:::tip
The Aerospike Cluster Manager UI automatically calculates and adjusts memory limits based on your namespace configuration during cluster creation.
:::

## Kubernetes Compatibility

| Operator Version | Kubernetes Versions | Controller-Runtime | CRD API Version |
|-----------------|--------------------|--------------------|-----------------|
| v0.1.x | v1.26 - v1.35 | v0.23.x | `acko.io/v1alpha1` |

:::note
The operator uses standard Kubernetes APIs and should work with any CNCF-certified distribution (EKS, GKE, AKS, OpenShift, k3s, etc.).
:::
