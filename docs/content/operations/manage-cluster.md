---
sidebar_position: 1
title: Manage Cluster
---

# Manage an Aerospike Cluster

Use these day-2 procedures to scale clusters, roll out updates, change configuration, and diagnose failures.

## Scaling

Change `spec.size` to scale the cluster up or down.

```bash
kubectl -n aerospike patch asc aerospike-3node --type merge -p '{"spec":{"size":5}}'
```

The operator creates or removes pods until the cluster reaches the requested size. In a multi-rack deployment, it distributes pods evenly across racks.

:::warning
`spec.size` must not exceed 8 (CE limit). `replication-factor` must not exceed the new cluster size.
:::

## Rolling Updates

### Image Update

Change `spec.image` to roll out a new image:

```yaml
spec:
  image: aerospike:ce-8.1.1.1   # Change to new version
```

The operator uses the `OnDelete` update strategy. It deletes pods one at a time or in configured batches, then waits for replacements to become ready before continuing.

### Batch Size

Control how many pods restart simultaneously:

```yaml
spec:
  rollingUpdateBatchSize: 2   # Restart 2 pods at a time (default: 1)
```

For rack-aware deployments, you can set batch size per rack in `rackConfig`. This takes precedence over `spec.rollingUpdateBatchSize`:

```yaml
spec:
  rackConfig:
    rollingUpdateBatchSize: "50%"   # Restart 50% of pods per rack simultaneously
```

#### Batch Size: Integer vs Percentage

Batch size accepts either an integer or a percentage string:

| Format | Example | Behavior (size=6) |
|--------|---------|-------------------|
| Integer | `2` | Exactly 2 pods at a time |
| Percentage | `"50%"` | 50% of 6 = 3 pods at a time |
| Percentage | `"25%"` | 25% of 6 = 2 pods at a time (rounded up) |

:::tip
A percentage string must include the `%` suffix (e.g., `"50%"`). The percentage is calculated against the total pod count per rack, rounded up to at least 1.
:::

#### Batch Restart Resilience

When restarting multiple pods in a batch, the operator continues to the next pod if an individual pod restart fails, rather than aborting the entire batch. This means:

- If 1 out of 3 pods in a batch fails to restart, the remaining 2 are still restarted
- Failed pods are recorded and reported via a `RestartFailed` warning event
- The operator returns an error only if **all** pods in the batch fail
- On the next reconciliation, only the pods that were not successfully restarted are retried

```bash
# Check for restart failures
kubectl get events --field-selector reason=RestartFailed -n aerospike
```

### Scale Down Batch Size

Control how many pods are removed simultaneously per rack during scale-down:

```yaml
spec:
  rackConfig:
    scaleDownBatchSize: 2            # Remove 2 pods per rack at a time
    # scaleDownBatchSize: "25%"      # Or use percentage
```

`scaleDownBatchSize` applies **per rack** during scale-down operations. This prevents removing too many pods at once, which could cause data unavailability.

### Max Ignorable Pods

Allow reconciliation to continue even when some pods are in Pending/Failed state:

```yaml
spec:
  rackConfig:
    maxIgnorablePods: 1   # Ignore up to 1 stuck pod and continue reconciling
```

This is useful when pods are stuck due to scheduling issues and you don't want to block the entire reconciliation.

#### Batch Size Summary

| Field | Scope | Default | Description |
|-------|-------|---------|-------------|
| `spec.rollingUpdateBatchSize` | Cluster-wide | 1 | Pods restarted simultaneously during rolling update |
| `rackConfig.rollingUpdateBatchSize` | Per rack | inherits from spec | Overrides cluster-level batch size per rack |
| `rackConfig.scaleDownBatchSize` | Per rack | all at once | Pods removed simultaneously during scale-down |
| `rackConfig.maxIgnorablePods` | Per rack | 0 | Stuck pods to ignore during reconciliation |

## Configuration Updates

### Static Configuration Changes

Any change to `spec.aerospikeConfig` triggers a rolling restart to apply the new configuration. The operator regenerates `aerospike.conf` in each pod's ConfigMap.

### Dynamic Configuration Updates

Enable runtime configuration changes without pod restarts:

```yaml
spec:
  enableDynamicConfigUpdate: true
```

When enabled, the operator uses Aerospike's `set-config` command to apply configuration changes at runtime where possible. Only changes that cannot be applied dynamically trigger a rolling restart.

#### Which settings are dynamic?

Most Aerospike service and namespace parameters are dynamically configurable. Examples include:

| Category | Dynamic Parameters |
|----------|-------------------|
| Service | `proto-fd-max`, `transaction-pending-limit`, `batch-max-buffers-per-queue` |
| Namespace | `high-water-memory-pct`, `high-water-disk-pct`, `stop-writes-pct`, `nsup-period`, `default-ttl` |
| Not Dynamic | `replication-factor`, `storage-engine type`, `name` (requires restart) |

#### Two-Phase Commit (2PC) for Batch Updates

When multiple pods need dynamic config changes, the operator uses a Two-Phase Commit pattern to prevent partial application across the cluster:

1. **Phase 1 — Validate**: The operator validates all changes on every pod (syntax check + node responsiveness probe). If ANY pod fails validation, the entire batch is aborted with no changes applied.
2. **Phase 2 — Apply**: Changes are applied to each pod sequentially. Each pod has an independent 30-second timeout.
   - If any pod fails during apply, all previously updated pods are **rolled back** in reverse order.
   - If rollback also fails, the cluster enters `ConfigDegraded` phase (see below).
3. After successful application, the operator falls back to a **cold restart** only for pods where dynamic update was not possible.

#### ConfigDegraded Phase

If a dynamic config rollback fails on one or more pods, the cluster transitions to `ConfigDegraded` phase. This means some pods may have inconsistent configuration. The operator will:

1. Set `status.phase: ConfigDegraded` with details about affected pods
2. Set the `DynamicConfigDegraded` status condition to `True`
3. Attempt cold restart recovery on the next reconcile to bring all pods back to a consistent state

Check for degraded state:

```bash
# Check cluster phase
kubectl -n aerospike get asc <name> -o jsonpath='{.status.phase}'

# Check DynamicConfigDegraded condition
kubectl -n aerospike get asc <name> -o jsonpath='{.status.conditions[?(@.type=="DynamicConfigDegraded")].message}'
```

#### Rollback on Partial Failure

When applying dynamic config changes, the operator tracks each successfully applied change per pod. If any change fails mid-way:

1. The operator **rolls back** all previously applied changes in reverse order using the original values
2. Cross-pod rollback is performed when a batch apply fails — all previously updated pods are reverted
3. If rollback fails, the cluster enters `ConfigDegraded` phase and the operator falls back to a **cold restart**

You can observe rollback activity in the operator logs:

```bash
kubectl -n aerospike-operator logs -l control-plane=controller-manager | grep -i "rollback\|dynamic config\|2PC"
```

#### Pre-flight Validation

Before applying any dynamic changes, the operator validates all changes as a batch. If any change contains invalid characters (e.g., `;` or `:` in parameter values), the entire batch is rejected before any `set-config` command is sent. This prevents partial application due to obviously invalid input.

#### Checking dynamicConfigStatus

After a config change with `enableDynamicConfigUpdate: true`, check per-pod status:

```bash
kubectl -n aerospike get asc aerospike-3node -o jsonpath='{.status.pods}' | jq '.[] | {name: .podName, dynamicConfig: .dynamicConfigStatus}'
```

| Status | Meaning |
|--------|---------|
| `Applied` | Dynamic config was applied successfully at runtime |
| `Failed` | Dynamic update failed — a rolling restart will be triggered |
| `Pending` | Waiting for the operator to apply the change |
| (empty) | No dynamic config change was attempted |

## Pod Readiness Gates

By default, a pod is considered "ready" when Kubernetes reports `PodReady=True`. This means the pod
may be added to Service endpoints before Aerospike has fully joined the cluster mesh and completed
data migrations — potentially routing client requests to a node with incomplete replicas.

Enable the custom readiness gate `acko.io/aerospike-ready` to ensure each pod is excluded from
Service endpoints until Aerospike is truly ready:

```yaml
spec:
  podSpec:
    readinessGateEnabled: true
```

When enabled, the operator:
1. Injects `acko.io/aerospike-ready` into every pod's `spec.readinessGates`
2. Patches the pod's `status.conditions` to `True` only after:
   - The pod's Aerospike process has joined the cluster mesh, **and**
   - All data migrations are complete (`cluster-stable: true`)
3. Holds rolling restarts — the next pod is not deleted until the previous pod's gate is satisfied

:::info
Changing `readinessGateEnabled` triggers a rolling restart because `ReadinessGates` is immutable
after pod creation. The operator handles this automatically.
:::

:::note
This is an **opt-in** feature. Existing clusters with `readinessGateEnabled` unset (or `false`)
behave exactly as before.
:::

### Observing Gate Status

Check the per-pod gate condition:

```bash
kubectl -n aerospike get pod aerospike-3node-0 \
  -o jsonpath='{.status.conditions}' | jq '.[] | select(.type=="acko.io/aerospike-ready")'
```

The operator also reflects the gate status in the cluster's pod status:

```bash
kubectl -n aerospike get asc aerospike-3node \
  -o jsonpath='{.status.pods}' | jq 'to_entries[] | {pod: .key, gateOk: .value.readinessGateSatisfied}'
```

### Rolling Restart Behavior with Readiness Gates

Without readiness gates:
```
Pod-2 deleted → Pod-2 Running → Pod-1 deleted → ...   (K8s Ready = enough)
```

With `readinessGateEnabled: true`:
```
Pod-2 deleted → Pod-2 Running → Gate=False (migrating) → Gate=True → Pod-1 deleted → ...
```

If a restart is blocked waiting for the gate, a `ReadinessGateBlocking` warning event is emitted:

```bash
kubectl -n aerospike get events --field-selector reason=ReadinessGateBlocking
```

## Pausing and Resuming Reconciliation

Temporarily stop the operator from reconciling a cluster by setting `spec.paused: true`. While paused, the operator skips all reconciliation for this cluster -- no scaling, rolling restarts, config changes, or ACL syncs will be performed. The cluster phase changes to `Paused` and the `ReconciliationPaused` condition is set to `True` atomically. A `ReconciliationPaused` Kubernetes event is also recorded.

**When to pause:**

- During planned infrastructure maintenance (node upgrades, storage migrations)
- To prevent the operator from interfering with manual debugging
- Before making multiple spec changes that you want to apply as a single batch
- When investigating a stuck cluster without the operator retrying

```bash
# Pause reconciliation
kubectl -n aerospike patch asc aerospike-3node --type merge -p '{"spec":{"paused":true}}'

# Verify paused state
kubectl -n aerospike get asc aerospike-3node -o jsonpath='{.status.phase}'
# Output: Paused

# Check pause event
kubectl -n aerospike get events --field-selector reason=ReconciliationPaused
```

To resume reconciliation, set `paused` to `false` or remove it entirely. The operator immediately begins reconciling the cluster toward its desired state. On resume, stale error state (failed reconcile count, last error) accumulated before pause is automatically cleared. A `ReconciliationResumed` event is recorded:

```bash
# Resume reconciliation
kubectl -n aerospike patch asc aerospike-3node --type merge -p '{"spec":{"paused":null}}'

# Verify resume event
kubectl -n aerospike get events --field-selector reason=ReconciliationResumed
```

:::warning
Pausing does not stop the Aerospike cluster itself -- pods continue running and serving traffic. It only stops the operator from making changes to the cluster's Kubernetes resources.
:::

## Cluster Status and Conditions

The operator provides detailed status information through the `status` subresource. Understanding these fields helps with monitoring and troubleshooting.

### Phase

The `status.phase` field provides a high-level view of what the operator is doing:

```bash
kubectl -n aerospike get asc
```

| Phase | Meaning |
|---|---|
| `Completed` | Cluster is healthy and matches the desired spec |
| `InProgress` | Generic reconciliation in progress |
| `ScalingUp` | Adding pods to the cluster |
| `ScalingDown` | Removing pods from the cluster |
| `WaitingForMigration` | Scale-down deferred until data migration completes |
| `RollingRestart` | Rolling restart in progress (config/image/podSpec change) |
| `ACLSync` | ACL roles and users are being synchronized |
| `Paused` | Reconciliation paused by user |
| `Deleting` | Cluster teardown in progress |
| `Error` | Unrecoverable error; check `status.lastReconcileError` |

The `status.phaseReason` field provides additional context (e.g., "Rolling restart in progress for rack 1").

### Health

The `status.health` field gives a quick "ready/total" summary:

```bash
kubectl -n aerospike get asc aerospike-3node -o jsonpath='{.status.health}'
# Output: 3/3
```

A value of `2/3` means 2 out of 3 pods are ready. This maps to the `HEALTH` column in `kubectl get asc` output.

### Conditions

The operator maintains six condition types that provide a detailed breakdown of cluster health:

```bash
kubectl -n aerospike get asc aerospike-3node -o jsonpath='{.status.conditions}' | jq .
```

| Condition | True When |
|---|---|
| `Available` | At least one pod is ready to serve requests |
| `Ready` | All desired pods are running and ready |
| `ConfigApplied` | All pods have the desired Aerospike configuration |
| `ACLSynced` | ACL roles and users match the spec |
| `MigrationComplete` | No data migrations are pending |
| `ReconciliationPaused` | `spec.paused` is `true` |

**Example: Checking if a cluster is fully operational**

A cluster is fully healthy when `Ready`, `ConfigApplied`, `MigrationComplete`, and `ACLSynced` (if ACL is configured) are all `True`:

```bash
kubectl -n aerospike get asc aerospike-3node \
  -o jsonpath='{range .status.conditions[*]}{.type}={.status}{"\n"}{end}'
```

Expected healthy output:
```
Available=True
Ready=True
ConfigApplied=True
ACLSynced=True
MigrationComplete=True
ReconciliationPaused=False
```

### Aerospike Cluster Size vs Kubernetes Pod Count

The `status.aerospikeClusterSize` field reflects the cluster size as reported by Aerospike's `asinfo` command. This may temporarily differ from `status.size` (the number of ready Kubernetes pods) during:

- Rolling restarts (a pod is being replaced)
- Network partitions (split-brain scenarios)
- Pod startup (Aerospike has not yet joined the mesh)

```bash
kubectl -n aerospike get asc aerospike-3node \
  -o jsonpath='K8s pods: {.status.size}, Aerospike cluster-size: {.status.aerospikeClusterSize}'
```

If these values diverge for an extended period, investigate pod connectivity and mesh heartbeat configuration.

### Migration Status Monitoring

The operator tracks data migration progress in `status.migrationStatus`. On each reconciliation, it queries every Aerospike node's `migrate_partitions_remaining` statistic and aggregates the results.

**When does migration occur?**

- **Scaling up/down** — adding or removing nodes triggers partition rebalancing.
- **Rolling restarts** — each restarted node must re-receive its partition copies.
- **Rack changes** — moving pods between racks redistributes data.
- **Replication factor changes** — increasing RF creates new replica copies.

**Checking migration status:**

```bash
# Cluster-level migration status
kubectl -n aerospike get asc aerospike-3node \
  -o jsonpath='{.status.migrationStatus}' | jq .
```

Example output:
```json
{
  "inProgress": true,
  "remainingPartitions": 142857,
  "lastChecked": "2026-03-13T10:30:00Z"
}
```

**Per-pod migration records:**

```bash
kubectl -n aerospike get asc aerospike-3node \
  -o jsonpath='{.status.pods}' | jq 'to_entries[] | {pod: .key, migrating: .value.migratingPartitions}'
```

**Quick check via jsonpath:**

```bash
# Check if migration is in progress (returns true/false)
kubectl -n aerospike get asc aerospike-3node \
  -o jsonpath='InProgress={.status.migrationStatus.inProgress} Remaining={.status.migrationStatus.remainingPartitions}'

# Check MigrationComplete condition directly
kubectl -n aerospike get asc aerospike-3node \
  -o jsonpath='{range .status.conditions[?(@.type=="MigrationComplete")]}{.status}{end}'
```

**Prometheus metric:**

The operator exposes `acko_cluster_migrating_partitions` as a Prometheus gauge metric with `namespace` and `name` labels. This enables alerting on long-running migrations:

```promql
# Alert when migration has been running for more than 30 minutes
acko_cluster_migrating_partitions{namespace="aerospike", name="aerospike-3node"} > 0

# Track migration progress rate (partitions migrated per second)
deriv(acko_cluster_migrating_partitions[5m])

# Alert on stalled migration (remaining partitions not decreasing)
deriv(acko_cluster_migrating_partitions[10m]) >= 0
  and acko_cluster_migrating_partitions > 0
```

:::tip
The `MigrationComplete` condition in `status.conditions` is set to `True` when `migrationStatus.remainingPartitions` reaches 0. Use it for simple health checks without parsing the full migration status.
:::

## Secret-Triggered ACL Sync

The operator watches Kubernetes Secrets referenced by `aerospikeAccessControl.users[*].secretName`. When a Secret's data changes (e.g., password rotation), the operator automatically triggers a reconciliation to sync the updated password to Aerospike — **without any changes to the AerospikeCluster CR**.

This enables zero-touch password rotation workflows:

```bash
# Rotate a user's password by updating the Secret
kubectl -n aerospike create secret generic app-secret \
  --from-literal=password='new-password-here' \
  --dry-run=client -o yaml | kubectl apply -f -
```

The operator detects the Secret data change and runs an ACL sync to update the user's password in Aerospike. You can verify the sync via events:

```bash
kubectl get events --field-selector reason=ACLSyncStarted -n aerospike
kubectl get events --field-selector reason=ACLSyncCompleted -n aerospike
```

:::info
Only Secrets that are actively referenced by an AerospikeCluster's ACL configuration trigger reconciliation. Unrelated Secret changes in the same namespace are ignored.
:::

## On-Demand Operations

Trigger pod restarts declaratively via `spec.operations`. Only one operation can be active at a time.

### WarmRestart

Sends SIGUSR1 to the Aerospike process for a graceful restart without pod deletion:

```yaml
spec:
  operations:
    - kind: WarmRestart
      id: "config-reload-v2"       # Unique ID (1-20 chars)
      podList:                      # Optional: empty = all pods
        - aerospike-3node-0
        - aerospike-3node-1
```

### PodRestart

Deletes and recreates pods (cold restart):

```yaml
spec:
  operations:
    - kind: PodRestart
      id: "cold-restart-01"
      podList:
        - aerospike-3node-2
```

### Checking Operation Status

```bash
kubectl -n aerospike get asc aerospike-3node -o jsonpath='{.status.operationStatus}' | jq .
```

The status includes `phase` (`InProgress`, `Completed`, `Error`), `completedPods`, and `failedPods`.

:::warning
- Operations cannot be modified while InProgress
- The operation `id` must be unique (1-20 characters)
- Remove the operation from the spec after it completes
:::

## Service Customization

### Headless Service Metadata

Add custom annotations and labels to the headless service (used for pod discovery):

```yaml
spec:
  headlessService:
    metadata:
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "8687"
      labels:
        monitoring: enabled
```

### Per-Pod Services

When `podService` is set, the operator creates an individual ClusterIP Service for each pod, enabling direct pod-level access:

```yaml
spec:
  podService:
    metadata:
      annotations:
        external-dns.alpha.kubernetes.io/hostname: "aero.example.com"
      labels:
        service-type: pod-local
```

**Use case:** External DNS integration, pod-level load balancing, or direct client access to specific pods.

## Validation Policy

Control webhook validation behavior:

```yaml
spec:
  validationPolicy:
    skipWorkDirValidate: true   # Skip work directory PV validation
```

| Field | Default | Description |
|---|---|---|
| `skipWorkDirValidate` | `false` | Skip validation that work directory is on persistent storage |

This is useful for development environments or in-memory deployments that don't require persistent work directories.

## Storage

### Volume Types

| Source | Use Case |
|---|---|
| `persistentVolume` | Durable data (survives pod restarts) |
| `emptyDir` | Temporary scratch space |
| `secret` | Credentials, TLS certs |
| `configMap` | Custom config files |
| `hostPath` | Node-local path (dev/test only) |

### Global Volume Policy

Set default policies for all persistent volumes by category (filesystem or block). Per-volume settings always override the global policy.

```yaml
storage:
  filesystemVolumePolicy:
    initMethod: deleteFiles
    wipeMethod: deleteFiles
    cascadeDelete: true
  blockVolumePolicy:
    initMethod: blkdiscard
    wipeMethod: blkdiscardWithHeaderCleanup
```

The operator resolves settings with this precedence:
1. **Per-volume** `initMethod` / `wipeMethod` / `cascadeDelete`
2. **Global policy** (based on `volumeMode`: `filesystemVolumePolicy` or `blockVolumePolicy`)
3. **Default**: `none` / `none` / `false`

### Cascade Delete

When `cascadeDelete: true`, PVCs are automatically deleted when the AerospikeCluster CR is deleted. This can be set per-volume or via global policy.

```yaml
storage:
  # Global: delete all filesystem PVCs on CR deletion
  filesystemVolumePolicy:
    cascadeDelete: true
  volumes:
    - name: data-vol
      source:
        persistentVolume:
          storageClass: standard
          size: 10Gi
      # Inherits cascadeDelete: true from filesystemVolumePolicy
```

#### PVC Cleanup Safety During Scale-Down

During scale-down, the operator verifies that all scaled-down pods have fully terminated before deleting their PVCs. This prevents a race condition where a PVC is deleted while the pod's Aerospike process is still writing data.

If any scaled-down pods are still running or terminating, PVC cleanup is **deferred** to the next reconciliation cycle. The operator logs this as:

```
Deferring PVC cleanup: scaled-down pods still terminating
```

Once all scaled-down pods are confirmed terminated, the operator deletes only PVCs for volumes with `cascadeDelete: true`. Non-cascade PVCs are always preserved.

You can monitor PVC cleanup activity via events:

```bash
# Successful cleanup
kubectl get events --field-selector reason=PVCCleanedUp -n aerospike

# Failed cleanup
kubectl get events --field-selector reason=PVCCleanupFailed -n aerospike
```

### Volume Init Methods

| Method | Description |
|---|---|
| `none` | No initialization (default) |
| `deleteFiles` | Delete all files in the volume |
| `dd` | Zero-fill the volume with `dd` |
| `blkdiscard` | Discard blocks (block devices only) |
| `headerCleanup` | Clear Aerospike file headers only |

```yaml
storage:
  volumes:
    - name: data-vol
      initMethod: deleteFiles
```

### Wipe Methods

Wipe methods are similar to init methods but apply to **dirty volumes** (volumes that need cleanup after unclean shutdown). The `wipeMethod` field supports the following values:

| Method | Description |
|---|---|
| `none` | No wiping (default) |
| `deleteFiles` | Delete all files in the volume |
| `dd` | Zero-fill the device using `dd` |
| `blkdiscard` | Discard all blocks on the device |
| `headerCleanup` | Clear Aerospike file headers only |
| `blkdiscardWithHeaderCleanup` | Discard blocks and then clear Aerospike headers |

```yaml
storage:
  volumes:
    - name: data-vol
      wipeMethod: headerCleanup
```

### HostPath Volumes

:::warning
HostPath volumes are **not recommended for production**. Data is tied to a specific node and is not portable across pod rescheduling.
:::

```yaml
storage:
  volumes:
    - name: host-logs
      source:
        hostPath:
          path: /var/log/aerospike
          type: DirectoryOrCreate
      aerospike:
        path: /opt/aerospike/logs
```

### PVC Custom Metadata

Add custom labels and annotations to PersistentVolumeClaims:

```yaml
storage:
  volumes:
    - name: data-vol
      source:
        persistentVolume:
          storageClass: standard
          size: 50Gi
          metadata:
            labels:
              backup-policy: "daily"
            annotations:
              volume.kubernetes.io/storage-provisioner: "ebs.csi.aws.com"
```

### PVC Management

The operator creates PersistentVolumeClaims for each pod's storage volumes. You can monitor PVC status through the Cluster Manager UI, which shows:

- **PVC binding status** -- Bound, Pending, Released, or Failed
- **Pod binding** -- which pod each PVC is currently bound to
- **Orphaned PVCs** -- PVCs that are bound to a PersistentVolume but not mounted by any running pod (can occur after scale-down or pod rescheduling)
- **Capacity and StorageClass** -- provisioned storage size and the StorageClass used

You can also check PVC status via kubectl:

```bash
# List all PVCs for a cluster
kubectl -n aerospike get pvc -l app=aerospike-ce-3node

# Check for orphaned PVCs (PVCs without a matching pod)
kubectl -n aerospike get pvc -l app=aerospike-ce-3node -o custom-columns=NAME:.metadata.name,STATUS:.status.phase,POD:.metadata.annotations.acko\\.io/pod-name
```

:::note
PVCs with `cascadeDelete: true` are automatically cleaned up when the CR is deleted or after scale-down. See [Cascade Delete](#cascade-delete) for details.
:::

### Volume Mount Options

Advanced mount options for Aerospike and sidecar containers:

```yaml
storage:
  volumes:
    - name: shared-data
      source:
        emptyDir: {}
      aerospike:
        path: /opt/aerospike/shared
        readOnly: false
        subPath: "aerospike-data"       # Mount a sub-directory
        mountPropagation: HostToContainer
      sidecars:
        - containerName: exporter
          path: /shared
          readOnly: true
```

| Option | Description |
|---|---|
| `readOnly` | Mount the volume as read-only |
| `subPath` | Mount a specific sub-directory of the volume |
| `subPathExpr` | Like subPath but supports environment variable expansion |
| `mountPropagation` | Control mount propagation (`None`, `HostToContainer`, `Bidirectional`) |

:::note
`subPath` and `subPathExpr` are mutually exclusive.
:::

### Local Storage

Mark storage classes as local to enable special handling during pod restarts:

```yaml
storage:
  localStorageClasses:
    - local-path
    - openebs-hostpath
  deleteLocalStorageOnRestart: true
```

When `deleteLocalStorageOnRestart: true`, the operator deletes PVCs backed by local storage classes **before** pod deletion during a cold restart. This forces re-provisioning on the new node, which is necessary because local storage is node-bound.

## Network Configuration

### Access Type

Control how clients discover and connect to Aerospike nodes:

| Type | Description |
|---|---|
| `pod` | Pod IP (default, in-cluster clients) |
| `hostInternal` | Node internal IP |
| `hostExternal` | Node external IP |
| `configuredIP` | Custom IP from pod annotations |

```yaml
spec:
  aerospikeNetworkPolicy:
    accessType: pod
    alternateAccessType: hostExternal
    fabricType: pod
```

### LoadBalancer

Expose the cluster via a LoadBalancer service:

```yaml
spec:
  seedsFinderServices:
    loadBalancer:
      port: 3000
      targetPort: 3000
      annotations:
        service.beta.kubernetes.io/aws-load-balancer-type: "nlb"
      loadBalancerSourceRanges:
        - "10.0.0.0/8"
```

### NetworkPolicy

Enable automatic NetworkPolicy creation:

```yaml
spec:
  networkPolicyConfig:
    enabled: true
    type: kubernetes    # or "cilium" for CiliumNetworkPolicy
```

### Bandwidth Shaping

Set bandwidth limits via CNI annotations (e.g., Cilium):

```yaml
spec:
  bandwidthConfig:
    ingress: "1Gbps"
    egress: "1Gbps"
```

## HorizontalPodAutoscaler (HPA)

AerospikeCluster supports the `scale` subresource, which enables integration with Kubernetes HPA. The operator exposes `status.selector` and `status.size` for HPA compatibility.

### Create an HPA

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: aerospike-hpa
  namespace: aerospike
spec:
  scaleTargetRef:
    apiVersion: acko.io/v1alpha1
    kind: AerospikeCluster
    name: aerospike-3node
  minReplicas: 2
  maxReplicas: 8    # CE maximum
  metrics:
    - type: Resource
      resource:
        name: cpu
        target:
          type: Utilization
          averageUtilization: 70
    - type: Resource
      resource:
        name: memory
        target:
          type: Utilization
          averageUtilization: 80
```

```bash
kubectl apply -f hpa.yaml
```

### Check HPA Status

```bash
kubectl -n aerospike get hpa aerospike-hpa
```

:::warning
The CE edition has a maximum cluster size of 8 nodes. Set `maxReplicas` to 8 or fewer.
HPA scales `spec.size`, which triggers the operator's normal scaling logic (rack-aware distribution, migration-aware scale-down).
:::

:::note
When using HPA, avoid manually changing `spec.size` — let the autoscaler manage it. If you need to temporarily override, pause the HPA first.
:::

---

## Pod Disruption Budget

By default, the operator creates a PodDisruptionBudget to protect the cluster during maintenance.

### Disable PDB

```yaml
spec:
  disablePDB: true
```

### Custom MaxUnavailable

```yaml
spec:
  maxUnavailable: 1         # Can be integer or percentage string like "25%"
```

## Host Network

Enable host networking for direct node port access:

```yaml
spec:
  podSpec:
    hostNetwork: true
    # Defaults applied automatically:
    #   multiPodPerHost: false
    #   dnsPolicy: ClusterFirstWithHostNet
```

## Rack Label Scheduling

Use `rackLabel` to schedule pods to nodes with a specific label. The operator sets a node affinity for `acko.io/rack=<rackLabel>`:

```yaml
spec:
  rackConfig:
    racks:
      - id: 1
        rackLabel: zone-a    # Pods scheduled to nodes with acko.io/rack=zone-a
      - id: 2
        rackLabel: zone-b
      - id: 3
        rackLabel: zone-c
```

:::warning
`rackLabel` values must be unique across racks.
:::

You can also attach a `revision` to each rack for controlled migrations:

```yaml
spec:
  rackConfig:
    racks:
      - id: 1
        rackLabel: zone-a
        revision: "v1.0"     # Version identifier for migration tracking
```

## Node Scheduling

### Node Selector

```yaml
spec:
  podSpec:
    nodeSelector:
      node-type: aerospike
```

### Tolerations

```yaml
spec:
  podSpec:
    tolerations:
      - key: "dedicated"
        operator: "Equal"
        value: "aerospike"
        effect: "NoSchedule"
```

### Node Block List

Prevent scheduling on specific nodes:

```yaml
spec:
  k8sNodeBlockList:
    - node-maintenance-01
    - node-maintenance-02
```

## Troubleshooting

### Check Cluster Phase

```bash
kubectl -n aerospike get asc
```

| Phase | Meaning |
|---|---|
| `InProgress` | Reconciliation is in progress (generic) |
| `Completed` | Cluster is healthy and up-to-date |
| `Error` | Reconciliation encountered an unrecoverable error |
| `ScalingUp` | Cluster is scaling up (adding pods) |
| `ScalingDown` | Cluster is scaling down (removing pods) |
| `RollingRestart` | A rolling restart is in progress |
| `ACLSync` | ACL roles and users are being synchronized |
| `Paused` | Reconciliation is paused by the user |
| `Deleting` | Cluster is being deleted |

### Check Conditions

```bash
kubectl -n aerospike get asc aerospike-3node -o jsonpath='{.status.conditions}' | jq .
```

### Check Pod Status

```bash
kubectl -n aerospike get asc aerospike-3node -o jsonpath='{.status.pods}' | jq .
```

Each pod status includes:

| Field | Description |
|---|---|
| `podIP` | Pod IP address |
| `hostIP` | Node IP address |
| `image` | Running container image |
| `rack` | Rack ID |
| `isRunningAndReady` | Whether the pod is healthy |
| `configHash` | SHA256 of applied config |
| `dynamicConfigStatus` | `Applied`, `Failed`, `Pending`, or empty |
| `nodeID` | Aerospike-assigned node identifier (e.g., `BB9020012AC4202`) |
| `clusterName` | Aerospike cluster name as reported by the node |
| `accessEndpoints` | Network endpoints for direct client access |
| `readinessGateSatisfied` | `true` when `acko.io/aerospike-ready` gate is satisfied (requires `readinessGateEnabled: true`) |
| `lastRestartReason` | Why the pod was last restarted: `ConfigChanged`, `ImageChanged`, `PodSpecChanged`, `ManualRestart`, `WarmRestart` |
| `lastRestartTime` | Timestamp of the last operator-initiated restart |
| `unstableSince` | First time this pod became NotReady; reset when Ready |
| `migratingPartitions` | Number of partitions this pod is currently migrating; `nil` if unreachable |

### Check Operator Logs

```bash
kubectl -n aerospike-operator logs -l control-plane=controller-manager -f
```

### Circuit Breaker and Exponential Backoff

The operator includes a built-in circuit breaker to prevent excessive retries on persistently failing clusters. After **10 consecutive reconciliation failures**, the operator enters a backoff state:

| Consecutive Failures | Backoff Delay |
|---------------------|---------------|
| 1 | 2 seconds |
| 2 | 4 seconds |
| 3 | 8 seconds |
| 5 | 32 seconds |
| 8+ | ~4.3 minutes (capped at 256 seconds) |

While the circuit breaker is active, a `CircuitBreakerActive` warning event is emitted with the failure count and last error. After a successful reconciliation, the counter resets and a `CircuitBreakerReset` event is emitted.

```bash
# Check if the circuit breaker is active
kubectl get events --field-selector reason=CircuitBreakerActive -n aerospike

# Check the failure count and last error
kubectl -n aerospike get asc aerospike-3node \
  -o jsonpath='{.status.failedReconcileCount}{"\t"}{.status.lastReconcileError}'
```

:::info
**Permanent validation errors** (e.g., invalid Aerospike config structure, missing ACL secrets, invalid privilege codes) **immediately** activate the circuit breaker by setting `failedReconcileCount` to the maximum threshold. A `PermanentError` event is emitted, and the `ReconcileHealthy` status condition is set to `False` with reason `PermanentError`. These errors will never self-heal — fix the spec to recover.
:::

### Common Issues

**Phase stuck at InProgress:**
- Check operator logs for error details
- Verify storage class exists: `kubectl get sc`
- Verify image is pullable: `kubectl -n aerospike describe pod <pod-name>`
- Check if the circuit breaker is active (see above) — the operator may be backing off

**Pod CrashLoopBackOff:**
- Check Aerospike logs: `kubectl -n aerospike logs <pod-name> -c aerospike`
- Verify `aerospikeConfig` is valid (namespace names, storage paths)

**PVCs not cleaned up after scale-down:**
- The operator defers PVC deletion until all scaled-down pods have fully terminated
- Check if pods are stuck in `Terminating` state: `kubectl -n aerospike get pods`
- If pods are stuck, investigate the cause (finalizers, volume detach issues) and resolve the stuck pods first
- PVC cleanup will proceed automatically on the next reconciliation after pods are gone

**Dynamic config changes trigger a restart instead of applying at runtime:**
- Verify `enableDynamicConfigUpdate: true` is set in the spec
- Check if the changed parameters are static (e.g., `replication-factor`, `storage-engine type`) — static changes always require a restart
- If a partial dynamic update failed, the operator rolls back applied changes across all pods (2PC pattern) and falls back to a cold restart. Check operator logs for `rollback` or `2PC` messages
- If the cluster is in `ConfigDegraded` phase, rollback itself failed — the operator will recover via cold restart on the next reconcile
- Ensure parameter values do not contain `;` or `:` characters, which are invalid in `set-config` commands

**Webhook rejection:**
- Read the error message — the webhook validates CE constraints
- Check [CE Validation Rules](../getting-started/create-cluster#ce-validation-rules)
- Common webhook errors:

| Error Message | Cause | Fix |
|---------------|-------|-----|
| `spec.size must not exceed 8` | CE cluster size limit | Reduce `spec.size` to 8 or fewer |
| `replication-factor N exceeds cluster size M` | RF larger than node count | Lower `replication-factor` or increase `spec.size` |
| `must have at least one user with both 'sys-admin' and 'user-admin' roles` | Missing admin user for ACL management | Assign both roles to at least one user |
| `user X references undefined role Y` | Custom role not declared | Add the role to `aerospikeAccessControl.roles` or use a built-in role |
| `maximum 2 namespaces allowed` | CE namespace limit | Remove extra namespaces |

**ACL sync failures:**
- Check that referenced Secrets exist and contain a `password` key
- Verify the admin user has both `sys-admin` and `user-admin` roles
- Check `ACLSyncError` events: `kubectl get events --field-selector reason=ACLSyncError -n aerospike`

## Kubernetes Events

The operator emits Kubernetes Events for every significant lifecycle transition.
Use `kubectl get events` to observe cluster activity in real time:

```bash
# Watch events for a specific cluster
kubectl get events --field-selector involvedObject.name=my-cluster -w

# Show all AerospikeCluster events in a namespace
kubectl get events --field-selector involvedObject.kind=AerospikeCluster -n aerospike
```

### Event Reference

| Reason | Type | Description |
|--------|------|-------------|
| `RollingRestartStarted` | Normal | Rolling restart loop began; shows rack ID and pod count |
| `RollingRestartCompleted` | Normal | Rolling restart completed for all targeted pods |
| `PodWarmRestarted` | Normal | Pod received SIGUSR1 (no downtime config reload) |
| `PodColdRestarted` | Normal | Pod deleted and recreated for a full restart |
| `RestartFailed` | Warning | Failed to restart a pod during rolling restart |
| `LocalPVCDeleteFailed` | Warning | Local PVC deletion failed before cold restart |
| `ConfigMapCreated` | Normal | Rack ConfigMap created for the first time |
| `ConfigMapUpdated` | Normal | Rack ConfigMap updated with new configuration |
| `DynamicConfigApplied` | Normal | Config changes applied to a pod without restart |
| `DynamicConfigStatusFailed` | Warning | Dynamic config status update failed |
| `DynamicConfigDegraded` | Warning | Cluster entered ConfigDegraded phase due to rollback failure |
| `DynamicConfigRollback` | Normal/Warning | Dynamic config rollback result (success or partial failure) |
| `StatefulSetCreated` | Normal | Rack StatefulSet created for the first time |
| `StatefulSetUpdated` | Normal | Rack StatefulSet spec updated |
| `RackScaled` | Normal | Rack replica count changed; shows old and new counts |
| `ACLSyncStarted` | Normal | ACL role/user synchronization began |
| `ACLSyncCompleted` | Normal | ACL roles and users synchronized successfully |
| `ACLSyncError` | Warning | ACL synchronization encountered an error |
| `PDBCreated` | Normal | PodDisruptionBudget created |
| `PDBUpdated` | Normal | PodDisruptionBudget updated |
| `ServiceCreated` | Normal | Headless service created |
| `ServiceUpdated` | Normal | Headless service updated |
| `ClusterDeletionStarted` | Normal | Cluster teardown began (finalizer active) |
| `FinalizerRemoved` | Normal | Storage finalizer removed; object will be deleted |
| `ReadinessGateSatisfied` | Normal | Pod readiness gate `acko.io/aerospike-ready` set to True |
| `ReadinessGateBlocking` | Warning | Rolling restart blocked waiting for readiness gate |
| `TemplateApplied` | Normal | ClusterTemplate spec applied to this cluster |
| `TemplateDrifted` | Warning | Cluster spec drifted from its template |
| `TemplateResolutionError` | Warning | Failed to resolve or apply a ClusterTemplate |
| `ValidationWarning` | Warning | Non-blocking validation warning detected |
| `PVCCleanedUp` | Normal | Orphaned PVCs deleted after scale-down |
| `PVCCleanupFailed` | Warning | Failed to delete orphaned PVCs after scale-down |
| `CircuitBreakerActive` | Warning | Reconciliation backed off after consecutive failures |
| `CircuitBreakerReset` | Normal | Circuit breaker reset after successful reconciliation |
| `PermanentError` | Warning | Permanent validation error detected, automatic retries halted |
| `ReconcileError` | Warning | Reconciliation loop encountered an unrecoverable error |
| `Operation` | Normal | On-demand operation event |
