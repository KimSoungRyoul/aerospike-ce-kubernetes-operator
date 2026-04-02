---
sidebar_position: 1
title: Storage Configuration
---

# Storage Configuration

Aerospike clusters use persistent storage for namespace data, secondary indexes, and logs. The `spec.storage` field controls how volumes are provisioned, initialized, and cleaned up.

---

## Global Volume Policies

Global policies apply to all volumes unless overridden at the individual volume level.

### filesystemVolumePolicy

Applied to PVC volumes with `volumeMode: Filesystem` (the default).

| Field | Options | Description |
|-------|---------|-------------|
| `initMethod` | `none`, `dd`, `deleteFiles`, `blkdiscard` | How to initialize the volume on first use |
| `wipeMethod` | `none`, `dd`, `deleteFiles`, `blkdiscard`, `blkdiscardWithHeaderCleanup`, `headerCleanup` | How to wipe the volume before pod deletion |
| `cascadeDelete` | `true` / `false` | Delete the PVC when the pod is deleted |

### blockVolumePolicy

Applied to PVC volumes with `volumeMode: Block`.

Same fields as `filesystemVolumePolicy`. The recommended `initMethod` for block devices is `blkdiscard` (fast, hardware-accelerated) and `blkdiscardWithHeaderCleanup` for `wipeMethod`.

### cleanupThreads

Controls the maximum number of concurrent threads used during volume initialization (`initMethod`) and wipe (`wipeMethod`) operations. Higher values speed up volume preparation at the cost of increased I/O load.

| Value | Behavior |
|-------|----------|
| `1` (default) | One volume at a time -- safe, low I/O impact |
| `2` -- `4` | Moderate parallelism -- good for clusters with multiple volumes |
| `>4` | Aggressive -- only use with high-throughput storage backends |

```yaml
storage:
  cleanupThreads: 2
```

:::tip
Increase `cleanupThreads` if you have many volumes per pod and want faster pod startup. Keep it at 1 if your storage backend has limited IOPS or if volume initialization causes I/O contention with running workloads.
:::

### localStorageClasses

List of StorageClass names considered "local" storage (e.g., `local-path`, `openebs-hostpath`). Local PVCs are deleted before pod restart to ensure a clean state.

```yaml
storage:
  localStorageClasses:
    - local-path
    - openebs-hostpath
  deleteLocalStorageOnRestart: true
  cleanupThreads: 2
```

See [Local Storage](#local-storage) below for a detailed explanation.

---

## Volume Types

### PVC — Filesystem (default)

Standard block-backed filesystem. Suitable for data, logs, and WAL files.

```yaml
volumes:
  - name: data-vol
    source:
      persistentVolume:
        storageClass: standard
        size: 50Gi
        volumeMode: Filesystem   # default
    aerospike:
      path: /opt/aerospike/data
```

### PVC — Block Device

Raw block volume (no filesystem). Use for Aerospike device-mode storage engines that access storage directly.

```yaml
volumes:
  - name: sindex-vol
    source:
      persistentVolume:
        storageClass: fast-ssd
        size: 20Gi
        volumeMode: Block
    aerospike:
      path: /opt/aerospike/sindex
```

### EmptyDir

Ephemeral in-pod storage. Useful for shared data between the Aerospike container and sidecars (e.g., exporters). Data is lost on pod restart.

```yaml
volumes:
  - name: shared-data
    source:
      emptyDir: {}
    aerospike:
      path: /opt/aerospike/shared
      subPath: "aerospike-data"
    sidecars:
      - containerName: aerospike-prometheus-exporter
        path: /shared
        readOnly: true
```

### HostPath

Mount a directory from the host node. For development/testing only — data is tied to a specific node.

```yaml
volumes:
  - name: host-logs
    source:
      hostPath:
        path: /var/log/aerospike
        type: DirectoryOrCreate
    aerospike:
      path: /opt/aerospike/logs
```

### CSI Volume (mountPropagation)

For CSI drivers that require `HostToContainer` propagation (e.g., FUSE-based storage).

```yaml
volumes:
  - name: csi-data
    source:
      persistentVolume:
        storageClass: csi-driver
        size: 100Gi
    aerospike:
      path: /opt/aerospike/csi
      mountPropagation: HostToContainer
```

---

## Per-Volume Override

Each volume can override the global policy with its own `initMethod`, `wipeMethod`, and `cascadeDelete`:

```yaml
storage:
  filesystemVolumePolicy:
    initMethod: deleteFiles   # global default
  volumes:
    - name: data-vol
      source:
        persistentVolume:
          storageClass: standard
          size: 50Gi
      aerospike:
        path: /opt/aerospike/data
      initMethod: dd          # override: use dd for this volume only
```

Priority: **per-volume** > **global policy** > **operator default**

---

## PVC Metadata

You can add custom labels and annotations to PVCs — useful for backup tools, storage policy, and cost attribution:

```yaml
volumes:
  - name: data-vol
    source:
      persistentVolume:
        storageClass: standard
        size: 50Gi
        metadata:
          labels:
            app.kubernetes.io/data-tier: "aerospike"
            backup-policy: "daily"
          annotations:
            volume.kubernetes.io/storage-provisioner: "ebs.csi.aws.com"
    aerospike:
      path: /opt/aerospike/data
```

---

## Complete Example

The following example (`config/samples/aerospike-cluster-storage-advanced.yaml`) demonstrates all volume types and policy options in a single cluster:

```yaml
apiVersion: acko.io/v1alpha1
kind: AerospikeCluster
metadata:
  name: aerospike-storage-advanced
  namespace: aerospike
spec:
  size: 3
  image: aerospike:ce-8.1.1.1

  storage:
    # Global volume policies: applied to all persistent volumes unless overridden per-volume
    filesystemVolumePolicy:
      initMethod: deleteFiles
      wipeMethod: deleteFiles
      cascadeDelete: true
    blockVolumePolicy:
      initMethod: blkdiscard
      wipeMethod: blkdiscardWithHeaderCleanup

    # Local storage recognition: delete local PVCs before pod restart
    localStorageClasses:
      - local-path
      - openebs-hostpath
    deleteLocalStorageOnRestart: true

    cleanupThreads: 2

    volumes:
      # PVC with custom metadata (labels and annotations on the PVC itself)
      - name: data-vol
        source:
          persistentVolume:
            storageClass: standard
            size: 50Gi
            volumeMode: Filesystem
            metadata:
              labels:
                app.kubernetes.io/data-tier: "aerospike"
                backup-policy: "daily"
              annotations:
                volume.kubernetes.io/storage-provisioner: "ebs.csi.aws.com"
        aerospike:
          path: /opt/aerospike/data
        # Per-volume override: uses dd instead of global policy's deleteFiles
        initMethod: dd

      # Block device volume (uses blockVolumePolicy defaults)
      - name: sindex-vol
        source:
          persistentVolume:
            storageClass: fast-ssd
            size: 20Gi
            volumeMode: Block
        aerospike:
          path: /opt/aerospike/sindex
        # initMethod not set → falls back to blockVolumePolicy.initMethod (blkdiscard)

      # Volume with advanced mount options
      - name: shared-data
        source:
          emptyDir: {}
        aerospike:
          path: /opt/aerospike/shared
          readOnly: false
          subPath: "aerospike-data"
        sidecars:
          - containerName: aerospike-prometheus-exporter
            path: /shared
            readOnly: true

      # HostPath volume (for development/testing only)
      - name: host-logs
        source:
          hostPath:
            path: /var/log/aerospike
            type: DirectoryOrCreate
        aerospike:
          path: /opt/aerospike/logs

      # Volume with mountPropagation
      - name: csi-data
        source:
          persistentVolume:
            storageClass: csi-driver
            size: 100Gi
        aerospike:
          path: /opt/aerospike/csi
          mountPropagation: HostToContainer

      # Local storage volume
      - name: local-data
        source:
          persistentVolume:
            storageClass: local-path
            size: 10Gi
        aerospike:
          path: /opt/aerospike/local
        initMethod: deleteFiles
        wipeMethod: deleteFiles

  aerospikeConfig:
    service:
      cluster-name: storage-advanced-demo
    namespaces:
      - name: test
        indexes-memory-budget: 1073741824
        replication-factor: 2
        storage-engine:
          type: device
          devices:
            - /opt/aerospike/data/test.dat
          filesize: 4294967296
          data-in-memory: true
```

Apply it with:

```bash
kubectl apply -f config/samples/aerospike-cluster-storage-advanced.yaml
```

---

## Local Storage

Local persistent volumes (local PVs) are node-bound storage that provide high I/O performance but do not survive pod migration to a different node. The operator provides two fields to handle local storage correctly: `localStorageClasses` and `deleteLocalStorageOnRestart`.

### When to Use Local Storage

- **High-performance workloads** -- local NVMe or SSD storage provides the lowest latency
- **Bare-metal clusters** -- nodes with directly attached storage
- **Cost optimization** -- avoid network-attached storage overhead when data is replicated at the Aerospike level

:::warning
Local PVs are tied to a specific node. If a pod is rescheduled to a different node, it cannot reattach the old PV. The operator handles this automatically when `deleteLocalStorageOnRestart` is enabled.
:::

### Configuration

#### `localStorageClasses`

A list of StorageClass names that the operator should treat as local storage. Any PVC backed by one of these classes receives special handling during pod restarts.

Common local StorageClass examples:
- `local-path` (Rancher Local Path Provisioner)
- `openebs-hostpath` (OpenEBS)
- `local-storage` (Kubernetes `local` volume plugin)

#### `deleteLocalStorageOnRestart`

When set to `true`, the operator deletes PVCs backed by local storage classes **before** deleting the pod during a cold restart. This forces the PVC to be re-provisioned on whichever node the pod is rescheduled to.

Without this setting, the pod would be stuck in `Pending` state if rescheduled to a different node, because the local PV is only available on the original node.

### Example Configuration

```yaml
apiVersion: acko.io/v1alpha1
kind: AerospikeCluster
metadata:
  name: aerospike-local-storage
  namespace: aerospike
spec:
  size: 3
  image: aerospike:ce-8.1.1.1

  storage:
    localStorageClasses:
      - local-path
      - openebs-hostpath
    deleteLocalStorageOnRestart: true

    filesystemVolumePolicy:
      initMethod: deleteFiles
      cascadeDelete: true

    volumes:
      - name: data-vol
        source:
          persistentVolume:
            storageClass: local-path
            size: 50Gi
        aerospike:
          path: /opt/aerospike/data

  aerospikeConfig:
    namespaces:
      - name: data
        replication-factor: 2
        storage-engine:
          type: device
          file: /opt/aerospike/data/data.dat
          filesize: 42949672960
```

### How It Works

During a cold restart (image change, config change, or PodRestart operation):

1. The operator identifies PVCs backed by storage classes listed in `localStorageClasses`
2. If `deleteLocalStorageOnRestart` is `true`, those PVCs are deleted **before** the pod
3. The pod is deleted
4. Kubernetes schedules the pod (possibly on a different node)
5. The StorageClass provisioner creates a new local PV on the target node
6. The pod starts with a fresh volume

:::info
If local PVC deletion fails, the operator emits a `LocalPVCDeleteFailed` warning event and proceeds with the pod restart. The PVC will be cleaned up on the next reconciliation.
:::

### Local Storage with Rack Configuration

When using local storage with rack-aware deployments, combine `localStorageClasses` with `rackLabel` scheduling to ensure pods are placed on nodes with local storage available:

```yaml
spec:
  storage:
    localStorageClasses:
      - local-path
    deleteLocalStorageOnRestart: true
    volumes:
      - name: data-vol
        source:
          persistentVolume:
            storageClass: local-path
            size: 50Gi
        aerospike:
          path: /opt/aerospike/data

  rackConfig:
    racks:
      - id: 1
        rackLabel: zone-a    # Nodes with acko.io/rack=zone-a and local storage
      - id: 2
        rackLabel: zone-b
```

### Local Storage vs Network-Attached Storage

| Aspect | Local Storage | Network-Attached (EBS, PD, etc.) |
|--------|--------------|----------------------------------|
| **Latency** | Lowest (direct-attached) | Higher (network round-trip) |
| **Pod migration** | PVC must be deleted and re-provisioned | PVC reattaches automatically |
| **Data durability** | Node-level only | Independent of node lifecycle |
| **Cost** | Lower (no network storage fees) | Higher (provisioned IOPS, throughput) |
| **Best for** | High-throughput, Aerospike-replicated data | Single-replica or stateful workloads |

:::tip
When using local storage, always set `replication-factor: 2` or higher in Aerospike to ensure data survives node failures. The data on a lost local PV is recovered from replicas on other nodes.
:::
