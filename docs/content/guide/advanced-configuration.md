---
sidebar_position: 7
title: Advanced Configuration
---

# Advanced Configuration

This guide covers advanced operator features for fine-tuning Aerospike cluster behavior, pod scheduling, rack-level overrides, and validation policies.

---

## EnableRackIDOverride

By default, the operator assigns rack IDs to pods based on the StatefulSet ordinal and the rack definition. When `enableRackIDOverride` is enabled, the operator allows dynamic rack ID assignment via pod annotations, giving you manual control over which rack a pod belongs to.

This is useful when you need to migrate pods between racks without scaling down and back up, or when integrating with external orchestration tools that manage rack placement.

```yaml
apiVersion: acko.io/v1alpha1
kind: AerospikeCluster
metadata:
  name: aerospike-cluster
spec:
  size: 4
  image: aerospike:ce-8.1.1.1
  enableRackIDOverride: true
  rackConfig:
    racks:
      - id: 1
        zone: us-east-1a
      - id: 2
        zone: us-east-1b
  aerospikeConfig:
    service:
      cluster-name: my-cluster
    namespaces:
      - name: test
        memory-size: 1073741824
        replication-factor: 2
        storage-engine:
          type: memory
    network:
      service:
        port: 3000
      heartbeat:
        port: 3002
      fabric:
        port: 3001
```

:::caution
Use `enableRackIDOverride` only when you have a specific need for manual rack assignment. In most cases, the operator's automatic rack distribution is sufficient and safer.
:::

---

## ValidationPolicy

The `validationPolicy` field controls which validation checks the operator performs. Currently it supports one option: `skipWorkDirValidate`.

### skipWorkDirValidate

By default, the operator validates that the Aerospike work directory (`/opt/aerospike`) is backed by persistent storage. This prevents data loss from ephemeral volumes. Setting `skipWorkDirValidate: true` disables this check.

**When to use:**

- Development or test clusters where persistence is not required
- Clusters using `emptyDir` or host-path volumes for the work directory
- Ephemeral benchmarking environments

```yaml
apiVersion: acko.io/v1alpha1
kind: AerospikeCluster
metadata:
  name: aerospike-dev
spec:
  size: 1
  image: aerospike:ce-8.1.1.1
  validationPolicy:
    skipWorkDirValidate: true
  aerospikeConfig:
    service:
      cluster-name: dev-cluster
    namespaces:
      - name: test
        memory-size: 1073741824
        replication-factor: 1
        storage-engine:
          type: memory
    network:
      service:
        port: 3000
      heartbeat:
        port: 3002
      fabric:
        port: 3001
```

---

## Pod Scheduling

The `spec.podSpec` field provides full control over Kubernetes scheduling primitives. All standard Kubernetes scheduling fields are supported.

### nodeSelector

Constrain Aerospike pods to nodes with specific labels.

```yaml
spec:
  podSpec:
    nodeSelector:
      disktype: ssd
      kubernetes.io/arch: amd64
```

### Tolerations

Allow Aerospike pods to be scheduled on tainted nodes.

```yaml
spec:
  podSpec:
    tolerations:
      - key: "dedicated"
        operator: "Equal"
        value: "aerospike"
        effect: "NoSchedule"
      - key: "node.kubernetes.io/disk-pressure"
        operator: "Exists"
        effect: "NoSchedule"
```

### Affinity

Define pod affinity and anti-affinity rules for precise scheduling control.

```yaml
spec:
  podSpec:
    affinity:
      podAntiAffinity:
        preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            podAffinityTerm:
              labelSelector:
                matchExpressions:
                  - key: app
                    operator: In
                    values:
                      - aerospike
              topologyKey: kubernetes.io/hostname
      nodeAffinity:
        requiredDuringSchedulingIgnoredDuringExecution:
          nodeSelectorTerms:
            - matchExpressions:
                - key: topology.kubernetes.io/zone
                  operator: In
                  values:
                    - us-east-1a
                    - us-east-1b
```

:::tip
When `podSpec.multiPodPerHost` is `false` (or `nil` with `hostNetwork: true`), the operator automatically injects a `RequiredDuringSchedulingIgnoredDuringExecution` pod anti-affinity rule to ensure one Aerospike pod per node. You do not need to manually configure this.
:::

### topologySpreadConstraints

Distribute pods evenly across failure domains (zones, nodes, etc.).

```yaml
spec:
  podSpec:
    topologySpreadConstraints:
      - maxSkew: 1
        topologyKey: topology.kubernetes.io/zone
        whenUnsatisfiable: DoNotSchedule
        labelSelector:
          matchLabels:
            app: aerospike
      - maxSkew: 1
        topologyKey: kubernetes.io/hostname
        whenUnsatisfiable: ScheduleAnyway
        labelSelector:
          matchLabels:
            app: aerospike
```

### podManagementPolicy

Controls how pods are created during scaling. Valid values:

| Value | Behavior |
|-------|----------|
| `OrderedReady` | Pods are created sequentially, each must be ready before the next is started (default) |
| `Parallel` | All pods are created simultaneously, useful for faster initial deployment |

```yaml
spec:
  podSpec:
    podManagementPolicy: Parallel
```

### priorityClassName

Set a [PriorityClass](https://kubernetes.io/docs/concepts/scheduling-eviction/pod-priority-preemption/) for Aerospike pods. The PriorityClass must already exist in the cluster.

```yaml
spec:
  podSpec:
    priorityClassName: high-priority
```

### Full Pod Scheduling Example

```yaml
apiVersion: acko.io/v1alpha1
kind: AerospikeCluster
metadata:
  name: aerospike-production
spec:
  size: 6
  image: aerospike:ce-8.1.1.1
  podSpec:
    podManagementPolicy: OrderedReady
    nodeSelector:
      disktype: ssd
    tolerations:
      - key: "dedicated"
        operator: "Equal"
        value: "aerospike"
        effect: "NoSchedule"
    affinity:
      podAntiAffinity:
        requiredDuringSchedulingIgnoredDuringExecution:
          - labelSelector:
              matchExpressions:
                - key: app
                  operator: In
                  values:
                    - aerospike
            topologyKey: kubernetes.io/hostname
    topologySpreadConstraints:
      - maxSkew: 1
        topologyKey: topology.kubernetes.io/zone
        whenUnsatisfiable: DoNotSchedule
        labelSelector:
          matchLabels:
            app: aerospike
    terminationGracePeriodSeconds: 600
    priorityClassName: high-priority
  aerospikeConfig:
    service:
      cluster-name: production
    namespaces:
      - name: data
        memory-size: 4294967296
        replication-factor: 2
        storage-engine:
          type: memory
    network:
      service:
        port: 3000
      heartbeat:
        port: 3002
      fabric:
        port: 3001
```

---

## Pod Customization

The `spec.podSpec` field provides access to several Kubernetes pod-level features beyond scheduling. This section covers sidecar containers, init containers, security contexts, service accounts, and image pull secrets.

### Custom Sidecars

Add sidecar containers to every Aerospike pod. These run alongside the main Aerospike server container and share the pod's network namespace and volumes.

```yaml
spec:
  podSpec:
    sidecars:
      - name: log-forwarder
        image: fluent/fluent-bit:latest
        resources:
          requests:
            cpu: 50m
            memory: 64Mi
          limits:
            cpu: 100m
            memory: 128Mi
        volumeMounts:
          - name: aerospike-logs
            mountPath: /var/log/aerospike
            readOnly: true
  storage:
    volumes:
      - name: aerospike-logs
        source:
          emptyDir: {}
        aerospike:
          path: /var/log/aerospike
        sidecars:
          - containerName: log-forwarder
            path: /var/log/aerospike
            readOnly: true
```

:::tip
When using the monitoring exporter sidecar (`spec.monitoring.enabled: true`), the operator automatically adds it. You do not need to include it in the `sidecars` list.
:::

### Init Containers

Add init containers that run before the Aerospike server starts. These execute after the operator's built-in init container (which handles volume initialization).

**Use cases:** Pre-populating data, setting file permissions, downloading configuration from external sources, waiting for dependencies.

```yaml
spec:
  podSpec:
    initContainers:
      - name: set-permissions
        image: busybox:1.36
        command: ["sh", "-c", "chown -R 1000:1000 /opt/aerospike/data"]
        volumeMounts:
          - name: data-vol
            mountPath: /opt/aerospike/data
  storage:
    volumes:
      - name: data-vol
        source:
          persistentVolume:
            storageClass: standard
            size: 50Gi
        aerospike:
          path: /opt/aerospike/data
        initContainers:
          - containerName: set-permissions
            path: /opt/aerospike/data
```

### Security Context

Set pod-level and container-level security attributes to meet your organization's security policies.

**Pod-level security context** applies to all containers in the pod:

```yaml
spec:
  podSpec:
    securityContext:
      runAsUser: 1000
      runAsGroup: 1000
      fsGroup: 1000
      runAsNonRoot: true
```

**Container-level security context** applies only to the Aerospike server container:

```yaml
spec:
  podSpec:
    aerospikeContainer:
      securityContext:
        allowPrivilegeEscalation: false
        readOnlyRootFilesystem: false
        capabilities:
          drop:
            - ALL
```

### Service Account

Specify a custom ServiceAccount for Aerospike pods. This is useful when pods need access to cloud provider APIs (e.g., for backup/restore via IAM roles) or when running in a namespace with restricted RBAC.

```yaml
spec:
  podSpec:
    serviceAccountName: aerospike-sa
```

### Image Pull Secrets

Reference Kubernetes Secrets containing container registry credentials. Required when pulling from private registries.

```yaml
spec:
  podSpec:
    imagePullSecrets:
      - name: my-registry-secret
```

Create the secret first:

```bash
kubectl -n aerospike create secret docker-registry my-registry-secret \
  --docker-server=registry.example.com \
  --docker-username=myuser \
  --docker-password=mypassword
```

---

## Secret and ConfigMap Volumes

In addition to PVC, emptyDir, and hostPath volumes, you can mount Kubernetes Secrets and ConfigMaps directly into Aerospike pods. This is useful for TLS certificates, custom configuration files, or credential injection.

### Secret Volume

Mount a Kubernetes Secret as files in the Aerospike container:

```yaml
spec:
  storage:
    volumes:
      - name: aerospike-creds
        source:
          secret:
            secretName: aerospike-credentials
            items:
              - key: tls.crt
                path: tls.crt
              - key: tls.key
                path: tls.key
        aerospike:
          path: /opt/aerospike/certs
          readOnly: true
```

### ConfigMap Volume

Mount a ConfigMap to provide additional configuration files:

```yaml
spec:
  storage:
    volumes:
      - name: custom-config
        source:
          configMap:
            name: aerospike-custom-config
        aerospike:
          path: /opt/aerospike/custom
          readOnly: true
```

---

## Rack-level Overrides

Each rack in `spec.rackConfig.racks` can override cluster-level settings for `aerospikeConfig`, `storage`, and `podSpec`. This enables heterogeneous configurations across failure domains.

### Per-rack aerospikeConfig

Override Aerospike configuration at the rack level. The rack-level config is merged with the cluster-level config, with rack values taking precedence.

```yaml
spec:
  aerospikeConfig:
    service:
      cluster-name: my-cluster
    namespaces:
      - name: data
        memory-size: 4294967296
        replication-factor: 2
        storage-engine:
          type: memory
    network:
      service:
        port: 3000
      heartbeat:
        port: 3002
      fabric:
        port: 3001
  rackConfig:
    racks:
      - id: 1
        aerospikeConfig:
          namespaces:
            - name: data
              memory-size: 8589934592
              replication-factor: 2
              storage-engine:
                type: memory
      - id: 2
```

In this example, rack 1 uses 8 GB of memory for the `data` namespace while rack 2 inherits the cluster-level 4 GB setting.

### Per-rack storage

Override the storage configuration for a specific rack, for example to use a different StorageClass in different availability zones.

```yaml
spec:
  storage:
    volumes:
      - name: data-vol
        storageClass: gp3
        size: 50Gi
        path: /opt/aerospike/data
        volumeMode: Filesystem
  rackConfig:
    racks:
      - id: 1
        storage:
          volumes:
            - name: data-vol
              storageClass: io2
              size: 100Gi
              path: /opt/aerospike/data
              volumeMode: Filesystem
      - id: 2
```

### Per-rack podSpec (affinity, tolerations, nodeSelector)

The `Rack.podSpec` field (`RackPodSpec`) supports three scheduling overrides:

| Field | Type | Description |
|-------|------|-------------|
| `affinity` | `corev1.Affinity` | Override cluster-level affinity rules |
| `tolerations` | `[]corev1.Toleration` | Override cluster-level tolerations |
| `nodeSelector` | `map[string]string` | Override cluster-level node selector |

```yaml
spec:
  rackConfig:
    racks:
      - id: 1
        zone: us-east-1a
        podSpec:
          nodeSelector:
            topology.kubernetes.io/zone: us-east-1a
          tolerations:
            - key: "zone-a-dedicated"
              operator: "Exists"
              effect: "NoSchedule"
          affinity:
            nodeAffinity:
              requiredDuringSchedulingIgnoredDuringExecution:
                nodeSelectorTerms:
                  - matchExpressions:
                      - key: node.kubernetes.io/instance-type
                        operator: In
                        values:
                          - m5.2xlarge
                          - m5.4xlarge
      - id: 2
        zone: us-east-1b
        podSpec:
          nodeSelector:
            topology.kubernetes.io/zone: us-east-1b
```

### Rack topology shortcuts

Each rack also provides shortcut fields for common scheduling scenarios:

| Field | Description |
|-------|-------------|
| `zone` | Equivalent to a node affinity on `topology.kubernetes.io/zone` |
| `region` | Equivalent to a node affinity on `topology.kubernetes.io/region` |
| `nodeName` | Constrains the rack to a specific Kubernetes node |
| `rackLabel` | Schedules pods to nodes with `acko.io/rack=<rackLabel>` |

```yaml
spec:
  rackConfig:
    racks:
      - id: 1
        zone: us-east-1a
      - id: 2
        zone: us-east-1b
      - id: 3
        region: us-west-2
        rackLabel: high-perf
```

### Soft Rack Support

Multiple racks can share the same `nodeName`. This enables "soft rack" deployments where rack-awareness is used for logical data distribution, but the underlying pods may run on the same Kubernetes node. This is useful in environments with limited nodes (e.g., development or staging) where you still want Aerospike to treat pods as belonging to separate racks for replication purposes.

```yaml
spec:
  rackConfig:
    racks:
      - id: 1
        nodeName: worker-1
      - id: 2
        nodeName: worker-1    # Same node as rack 1 — allowed
      - id: 3
        nodeName: worker-2
```

**When to use soft racks:**

- **Staging environments** with fewer nodes than racks -- rack-awareness is preserved logically even though pods share a node.
- **Preferred anti-affinity** deployments where the scheduler may co-locate racks on the same node when resources are constrained.
- **Single-node development** clusters that need to test multi-rack behavior.

:::caution
While soft racks allow replica distribution across logical racks, co-located racks on the same physical node do not provide true fault isolation. If the shared node fails, all racks on that node are lost simultaneously. Use hard anti-affinity (`rackLabel` with unique nodes) for production fault tolerance.
:::

**Validation rules:**

- `nodeName` does **not** require uniqueness across racks (unlike `rackLabel`, which must be unique).
- `rackLabel` must still be unique across all racks -- use `nodeName` instead of `rackLabel` when you need shared-node racks.
- Rack IDs must remain unique regardless of the scheduling strategy.

---

## Rack Configuration

The `spec.rackConfig` top-level fields control rack-wide behavior for namespace awareness, scaling, and rolling updates.

### namespaces

Specifies which Aerospike namespaces are rack-aware. If empty, all namespaces use the default replication factor. CE supports a maximum of 2 namespaces.

```yaml
spec:
  rackConfig:
    namespaces:
      - data
      - cache
    racks:
      - id: 1
      - id: 2
```

### scaleDownBatchSize

Controls how many pods are removed simultaneously per rack during scale-down operations. Accepts an absolute integer or a percentage string. Defaults to 1.

```yaml
spec:
  rackConfig:
    scaleDownBatchSize: 2
    racks:
      - id: 1
      - id: 2
```

With a percentage:

```yaml
spec:
  rackConfig:
    scaleDownBatchSize: "25%"
    racks:
      - id: 1
      - id: 2
```

### maxIgnorablePods

The maximum number of pending or failed pods to ignore during reconciliation. This is useful when pods are stuck due to scheduling issues (e.g., insufficient resources, node affinity mismatch) and you want the operator to continue reconciling healthy pods. Accepts an absolute integer or a percentage string.

```yaml
spec:
  rackConfig:
    maxIgnorablePods: 1
    racks:
      - id: 1
      - id: 2
```

### rollingUpdateBatchSize

Controls how many pods are restarted simultaneously per rack during a rolling restart. Accepts an absolute integer or a percentage string. Defaults to 1. This field takes precedence over `spec.rollingUpdateBatchSize` when set.

```yaml
spec:
  rackConfig:
    rollingUpdateBatchSize: 2
    racks:
      - id: 1
      - id: 2
```

:::note
There are two `rollingUpdateBatchSize` fields:
- `spec.rollingUpdateBatchSize` (integer only, cluster-wide default)
- `spec.rackConfig.rollingUpdateBatchSize` (integer or percentage, per-rack override, takes precedence)
:::

---

## Pod Metadata

Add custom labels and annotations to Aerospike pods via `spec.podSpec.metadata`. These are applied to every pod managed by the operator, in addition to the operator's own labels.

This is useful for:

- Service mesh injection (e.g., Istio sidecar annotations)
- Monitoring label selectors
- Cost allocation tags
- External tool integration

```yaml
spec:
  podSpec:
    metadata:
      labels:
        team: platform
        cost-center: "12345"
        environment: production
      annotations:
        sidecar.istio.io/inject: "true"
        prometheus.io/scrape: "true"
        prometheus.io/port: "9145"
```

---

## ReadinessGateEnabled

When `readinessGateEnabled` is set to `true`, the operator injects a custom Pod Readiness Gate with the condition type `acko.io/aerospike-ready` into each pod's spec. The operator patches the pod's `status.conditions` to set this gate to `True` only after the Aerospike node has:

1. Joined the cluster mesh
2. Finished all pending data migrations

Until the readiness gate is satisfied, the pod is excluded from Service endpoints, preventing traffic from being routed to nodes that are not fully ready to serve requests.

Defaults to `false` for backward compatibility.

```yaml
apiVersion: acko.io/v1alpha1
kind: AerospikeCluster
metadata:
  name: aerospike-with-readiness-gate
spec:
  size: 3
  image: aerospike:ce-8.1.1.1
  podSpec:
    readinessGateEnabled: true
  aerospikeConfig:
    service:
      cluster-name: my-cluster
    namespaces:
      - name: data
        memory-size: 2147483648
        replication-factor: 2
        storage-engine:
          type: memory
    network:
      service:
        port: 3000
      heartbeat:
        port: 3002
      fabric:
        port: 3001
```

You can verify the readiness gate status on individual pods:

```bash
kubectl get pod aerospike-with-readiness-gate-1-0 -o jsonpath='{.status.conditions[?(@.type=="acko.io/aerospike-ready")]}'
```

The per-pod status field `readinessGateSatisfied` in `status.pods` also reflects whether the gate is currently `True`.

:::tip
Enable readiness gates in production clusters to ensure zero-downtime rolling updates. During a rolling restart, the new pod will not receive client traffic until it has fully joined the cluster and completed data migrations.
:::
