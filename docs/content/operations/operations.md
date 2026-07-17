---
sidebar_position: 2
title: Operations
---

# Operations

Declare controlled pod restarts in `spec.operations`. The operator sequences each operation, reports its status, and emits Kubernetes events.

---

## Overview

Use an on-demand operation instead of deleting Aerospike pods manually. Add the operation to the cluster spec, and the operator handles ordering, status tracking, and event emission.

**Key constraints:**

- Only **one operation** can be active at a time.
- The operation `id` must be unique and contain 1--20 characters.
- You cannot modify an operation while it is `InProgress`.
- Remove the operation from the spec after it completes.

---

## Operation Types

### WarmRestart

A warm restart sends `SIGUSR1` to the Aerospike process, causing it to reload configuration without pod deletion. The pod stays running throughout the process.

```yaml
spec:
  operations:
    - kind: WarmRestart
      id: "config-reload-v2"
```

**When to use:**

- After dynamic configuration changes that require a process signal
- When you want to minimize downtime (no pod deletion/recreation)
- For configuration reloads that Aerospike supports via SIGUSR1

:::tip
WarmRestart is the least disruptive operation. The Aerospike process restarts in-place, preserving the pod's network identity and storage mounts.
:::

### PodRestart

A pod restart (cold restart) deletes and recreates the targeted pods. This is equivalent to a full restart cycle including volume reattachment.

```yaml
spec:
  operations:
    - kind: PodRestart
      id: "cold-restart-01"
```

**When to use:**

- When a warm restart is insufficient (e.g., the pod is in a bad state)
- To force volume re-initialization
- When the Aerospike process is unresponsive to SIGUSR1
- After node maintenance that requires fresh pod scheduling

---

## Targeting Specific Pods

By default, an operation targets **all pods** in the cluster. Use `podList` to target specific pods:

### All Pods (default)

```yaml
spec:
  operations:
    - kind: WarmRestart
      id: "reload-all"
      # podList omitted = all pods
```

### Specific Pods

```yaml
spec:
  operations:
    - kind: PodRestart
      id: "restart-pod-2"
      podList:
        - aerospike-3node-0
        - aerospike-3node-2
```

:::warning
When targeting specific pods, ensure you use the correct pod names. Pod names follow the pattern `<cluster-name>-<rack-id>-<ordinal>` for multi-rack deployments, or `<cluster-name>-<ordinal>` for single-rack clusters.
:::

---

## Triggering an Operation

### Step 1: Add the operation to the cluster spec

```bash
kubectl -n aerospike patch asc aerospike-3node --type merge -p '{
  "spec": {
    "operations": [
      {
        "kind": "WarmRestart",
        "id": "config-reload-v2"
      }
    ]
  }
}'
```

### Step 2: Monitor progress

```bash
# Check operation status
kubectl -n aerospike get asc aerospike-3node \
  -o jsonpath='{.status.operationStatus}' | jq .

# Watch events
kubectl -n aerospike get events \
  --field-selector involvedObject.name=aerospike-3node,reason=Operation -w
```

### Step 3: Remove the operation after completion

Once the operation reaches `Completed` or `Error` phase, remove it from the spec:

```bash
kubectl -n aerospike patch asc aerospike-3node --type merge -p '{
  "spec": {
    "operations": null
  }
}'
```

---

## Operation Status Tracking

The operator tracks operation progress in `status.operationStatus`:

```bash
kubectl -n aerospike get asc aerospike-3node \
  -o jsonpath='{.status.operationStatus}' | jq .
```

### Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Operation identifier (matches `spec.operations[].id`) |
| `kind` | string | Operation type: `WarmRestart` or `PodRestart` |
| `phase` | string | Current phase: `InProgress`, `Completed`, or `Error` |
| `completedPods` | []string | Pods that have completed the operation |
| `failedPods` | []string | Pods where the operation failed |

### Phase Transitions

```
                    ┌──────────────┐
                    │  InProgress  │
                    └──────┬───────┘
                           │
              ┌────────────┴────────────┐
              ▼                         ▼
     ┌────────────────┐       ┌─────────────┐
     │   Completed    │       │    Error     │
     └────────────────┘       └─────────────┘
```

| Phase | Meaning |
|-------|---------|
| `InProgress` | The operator is executing the operation on targeted pods |
| `Completed` | All targeted pods have been successfully restarted |
| `Error` | One or more pods failed. Check `failedPods` for details |

### Example Status Output

```json
{
  "id": "config-reload-v2",
  "kind": "WarmRestart",
  "phase": "InProgress",
  "completedPods": [
    "aerospike-3node-0",
    "aerospike-3node-1"
  ],
  "failedPods": []
}
```

---

## Pod Status After Operations

After an operation completes, individual pod status reflects the restart:

```bash
kubectl -n aerospike get asc aerospike-3node \
  -o jsonpath='{.status.pods}' | jq 'to_entries[] | {pod: .key, lastRestart: .value.lastRestartReason, time: .value.lastRestartTime}'
```

The `lastRestartReason` field records why the pod was last restarted:

| Value | Description |
|-------|-------------|
| `WarmRestart` | On-demand warm restart (SIGUSR1) |
| `ManualRestart` | On-demand cold restart (PodRestart) |
| `ConfigChanged` | Cold restart due to config change |
| `ImageChanged` | Cold restart due to image update |
| `PodSpecChanged` | Cold restart due to pod spec change |

---

## WarmRestart vs PodRestart

| Aspect | WarmRestart | PodRestart |
|--------|-------------|------------|
| **Mechanism** | SIGUSR1 signal to Aerospike process | Pod delete and recreate |
| **Downtime** | Minimal (process restarts in-place) | Full pod lifecycle (terminate, schedule, start) |
| **Storage** | Volumes remain mounted | Volumes are detached and reattached |
| **Network** | Pod IP preserved | Pod may get a new IP |
| **Use case** | Config reload, graceful restart | Stuck pods, volume reset, node migration |
| **Risk** | Low -- process restarts cleanly | Medium -- pod rescheduling, possible migration |

:::info
The cluster's `rollingUpdateBatchSize` does **not** apply to on-demand operations. Operations execute on all targeted pods according to the operator's internal sequencing.
:::

---

## Validation Rules

The webhook enforces these constraints on operations:

| Rule | Constraint | Error Message |
|------|-----------|---------------|
| Max operations | Only 1 operation at a time | `only one operation allowed` |
| ID length | 1--20 characters | `id must be between 1 and 20 characters` |
| In-progress lock | Cannot modify while `InProgress` | `cannot modify operations while InProgress` |

---

## Events

The operator emits events for operation lifecycle:

| Reason | Type | Description |
|--------|------|-------------|
| `Operation` | Normal | Operation started, pod restarted, or operation completed |
| `PodWarmRestarted` | Normal | Pod received SIGUSR1 |
| `PodColdRestarted` | Normal | Pod deleted and recreated |

```bash
kubectl -n aerospike get events \
  --field-selector involvedObject.name=aerospike-3node,reason=Operation
```

---

## Examples

### Reload Configuration Across All Pods

```yaml
apiVersion: acko.io/v1alpha1
kind: AerospikeCluster
metadata:
  name: aerospike-3node
  namespace: aerospike
spec:
  size: 3
  image: aerospike:ce-8.1.1.1
  operations:
    - kind: WarmRestart
      id: "reload-config-mar10"
  aerospikeConfig:
    namespaces:
      - name: test
        replication-factor: 2
        storage-engine:
          type: memory
```

### Cold Restart a Single Pod

```yaml
spec:
  operations:
    - kind: PodRestart
      id: "fix-pod-2"
      podList:
        - aerospike-3node-2
```

### Warm Restart a Subset of Pods

```yaml
spec:
  operations:
    - kind: WarmRestart
      id: "reload-rack-1"
      podList:
        - aerospike-3node-0
        - aerospike-3node-1
```

---

## Best Practices

1. **Use descriptive operation IDs** -- include a date or version reference (e.g., `config-reload-mar10`, `fix-pod-2-v3`) to make status tracking and event correlation easier.
2. **Prefer WarmRestart** when possible -- it minimizes disruption and avoids pod rescheduling.
3. **Remove completed operations** -- leaving stale operations in the spec does not cause problems, but keeping the spec clean avoids confusion.
4. **Check operation status before adding a new one** -- the webhook rejects a second operation while one is `InProgress`.
5. **Use `podList` for targeted restarts** -- avoid restarting the entire cluster when only one pod needs attention.
