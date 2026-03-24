---
sidebar_position: 1
title: Create Cluster
---

import Tabs from '@theme/Tabs';
import TabItem from '@theme/TabItem';

# Create an Aerospike Cluster

This guide explains how to deploy Aerospike CE clusters using the `AerospikeCluster` CRD.

## Sample Configurations

<Tabs>
<TabItem value="minimal" label="Minimal (1-Node)" default>

The simplest cluster: a single-node in-memory deployment.

```yaml
apiVersion: acko.io/v1alpha1
kind: AerospikeCluster
metadata:
  name: aerospike-basic
  namespace: aerospike
spec:
  size: 1
  image: aerospike:ce-8.1.1.1
  aerospikeConfig:
    namespaces:
      - name: test
        replication-factor: 1
        storage-engine:
          type: memory
          data-size: 1073741824   # 1 GiB
```

**Use case:** Development, testing, quick prototyping.

```bash
kubectl create namespace aerospike
kubectl apply -f config/samples/acko_v1alpha1_aerospikecluster.yaml
```

</TabItem>
<TabItem value="3node" label="3-Node PV Storage">

A production-like setup with resource limits and persistent storage.

```yaml
apiVersion: acko.io/v1alpha1
kind: AerospikeCluster
metadata:
  name: aerospike-3node
  namespace: aerospike
spec:
  size: 3
  image: aerospike:ce-8.1.1.1

  # Pod resource limits
  podSpec:
    aerospikeContainer:
      resources:
        requests:
          memory: "2Gi"
          cpu: "1"
        limits:
          memory: "4Gi"
          cpu: "2"

  # Persistent storage
  storage:
    volumes:
      - name: data-vol
        source:
          persistentVolume:
            storageClass: standard
            size: 10Gi
            volumeMode: Filesystem
        aerospike:
          path: /opt/aerospike/data
        cascadeDelete: true       # Delete PVC when CR is deleted

      - name: workdir
        source:
          emptyDir: {}
        aerospike:
          path: /opt/aerospike/work

  aerospikeConfig:
    service:
      cluster-name: aerospike-3node
      proto-fd-max: 15000

    network:
      service:
        address: any
        port: 3000
      heartbeat:
        mode: mesh
        port: 3002
      fabric:
        address: any
        port: 3001

    namespaces:
      - name: testns
        replication-factor: 2
        storage-engine:
          type: device
          file: /opt/aerospike/data/testns.dat
          filesize: 4294967296    # 4 GiB
```

**Use case:** Production workloads with data persistence and replication.

```bash
kubectl apply -f config/samples/aerospike-cluster-3node.yaml
```

</TabItem>
<TabItem value="multirack" label="6-Node Multi-Rack">

Spread pods across failure domains using rack-aware deployment.

```yaml
apiVersion: acko.io/v1alpha1
kind: AerospikeCluster
metadata:
  name: aerospike-multirack
  namespace: aerospike
spec:
  size: 6
  image: aerospike:ce-8.1.1.1

  # 3 racks — pods are distributed evenly (2 per rack)
  rackConfig:
    namespaces:
      - testns              # Rack-aware namespace
    rollingUpdateBatchSize: "50%"  # Restart 50% of pods per rack during rolling update
    scaleDownBatchSize: 1          # Remove 1 pod per rack during scale-down
    maxIgnorablePods: 1            # Continue reconciling if 1 pod is stuck
    racks:
      - id: 1
        rackLabel: zone-a          # Schedule to nodes with acko.io/rack=zone-a
        revision: "v1.0"
      - id: 2
        rackLabel: zone-b
        revision: "v1.0"
      - id: 3
        rackLabel: zone-c
        revision: "v1.0"

  podSpec:
    aerospikeContainer:
      resources:
        requests:
          memory: "512Mi"
          cpu: "250m"
        limits:
          memory: "1Gi"
          cpu: "500m"

  storage:
    volumes:
      - name: data-vol
        source:
          persistentVolume:
            storageClass: standard
            size: 5Gi
        aerospike:
          path: /opt/aerospike/data
        cascadeDelete: false      # Keep PVCs on deletion

  aerospikeConfig:
    namespaces:
      - name: testns
        replication-factor: 2
        storage-engine:
          type: device
          file: /opt/aerospike/data/testns.dat
          filesize: 4294967296
```

**Use case:** High availability across zones with rack-label-based scheduling. Each rack ID creates a separate StatefulSet (`<name>-<rackID>`) and ConfigMap.

```bash
kubectl apply -f config/samples/aerospike-cluster-multirack.yaml
```

</TabItem>
<TabItem value="monitoring" label="With Monitoring">

A 3-node cluster with Prometheus exporter sidecar and ServiceMonitor.

```yaml
apiVersion: acko.io/v1alpha1
kind: AerospikeCluster
metadata:
  name: aerospike-ce-monitored
  namespace: aerospike
spec:
  size: 3
  image: aerospike:ce-8.1.1.1

  monitoring:
    enabled: true
    exporterImage: aerospike/aerospike-prometheus-exporter:latest
    port: 9145
    resources:
      requests:
        cpu: "100m"
        memory: "64Mi"
    metricLabels:
      environment: production
    serviceMonitor:
      enabled: true
      interval: "30s"
      labels:
        release: prometheus    # Match your Prometheus Operator selector
    prometheusRule:
      enabled: true
      labels:
        release: prometheus    # Match your Prometheus Operator ruleSelector

  storage:
    volumes:
      - name: data-vol
        source:
          persistentVolume:
            storageClass: standard
            size: 10Gi
        aerospike:
          path: /opt/aerospike/data
        cascadeDelete: true

  aerospikeConfig:
    namespaces:
      - name: testns
        replication-factor: 2
        storage-engine:
          type: device
          file: /opt/aerospike/data/testns.dat
          filesize: 4294967296
```

**Use case:** Production with Prometheus/Grafana observability. The operator injects an exporter sidecar into each pod, creates a ServiceMonitor for automatic discovery, and generates a PrometheusRule with built-in alerts (NodeDown, StopWrites, HighDiskUsage, HighMemoryUsage). You can replace the built-in alerts with custom rules using `prometheusRule.customRules`. See the [Monitoring guide](monitoring.md) for details.

</TabItem>
</Tabs>

## Webhook Defaults

The operator's mutating webhook automatically sets the following defaults if not specified:

| Field | Default Value | Description |
|---|---|---|
| `aerospikeConfig.service.cluster-name` | `metadata.name` | Aerospike cluster name |
| `aerospikeConfig.service.proto-fd-max` | `15000` | Max client connections |
| `aerospikeConfig.network.service.port` | `3000` | Client service port |
| `aerospikeConfig.network.heartbeat.port` | `3002` | Heartbeat port |
| `aerospikeConfig.network.heartbeat.mode` | `mesh` | Heartbeat mode |
| `aerospikeConfig.network.fabric.port` | `3001` | Fabric (inter-node) port |
| `monitoring.exporterImage` | `aerospike/aerospike-prometheus-exporter:1.16.1` | Exporter image (when monitoring enabled) |
| `monitoring.port` | `9145` | Exporter metrics port (when monitoring enabled) |
| `monitoring.serviceMonitor.interval` | `30s` | Scrape interval (when ServiceMonitor enabled) |
| `podSpec.multiPodPerHost` | `false` | One pod per node (when hostNetwork enabled) |
| `podSpec.dnsPolicy` | `ClusterFirstWithHostNet` | DNS policy (when hostNetwork enabled) |

## CE Validation Rules

The validating webhook enforces Community Edition constraints:

| Rule | Constraint | Error |
|---|---|---|
| Cluster size | `spec.size` max 8 | `spec.size N exceeds CE maximum of 8` |
| Namespace count | max 2 namespaces | `namespaces count N exceeds CE maximum of 2` |
| XDR | Not allowed | `must not contain 'xdr' section` |
| TLS | Not allowed | `must not contain 'tls' section` |
| Security | Not allowed (CE 8.x) | `must not contain 'security' section` |
| Heartbeat mode | Must be `mesh` | `must be 'mesh' for CE` |
| Image | Must be CE image, CE 8+ only | `Enterprise Edition image not allowed`, `CE 7.x is no longer supported` |
| Replication factor | must not exceed `spec.size` | `replication-factor N exceeds cluster size` |
| Replication factor range | 1 to 4 | `must be between 1 and 4` |
| Rack IDs | Must be unique | `duplicate rack ID` |
| Rack labels | Must be unique across racks | `duplicate rackLabel` |
| Operations | Max 1 active at a time | `only one operation allowed` |
| Operation ID | 1-20 characters | `id must be between 1 and 20 characters` |
| Operations (in-progress) | Cannot modify while InProgress | `cannot modify operations while InProgress` |
| `scaleDownBatchSize` | Must be positive | `must be positive` |
| `rollingUpdateBatchSize` (rackConfig) | Must be positive | `must be positive` |
| `maxIgnorablePods` | Must be >= 0 | `must not be negative` |

### Enterprise-Only Namespace Keys

The following namespace configuration keys are blocked for CE:

| Key | Reason |
|---|---|
| `compression`, `compression-level` | Data compression is Enterprise-only |
| `durable-delete` | Durable deletes is Enterprise-only |
| `fast-restart` | Fast restart is Enterprise-only |
| `index-type` | Flash/pmem index is Enterprise-only |
| `sindex-type` | Flash/pmem sindex is Enterprise-only |
| `rack-id` | Use operator `rackConfig` instead |
| `strong-consistency` | Strong consistency is Enterprise-only |
| `tomb-raider-eligible-age`, `tomb-raider-period` | Tomb-raider is Enterprise-only |

### Warnings

The webhook also emits warnings (non-blocking) for:

- Untagged images or `latest` tag usage
- `hostNetwork=true` with `multiPodPerHost=true` (port conflict risk)
- `hostNetwork=true` with non-`ClusterFirstWithHostNet` DNS policy
- `data-in-memory=true` (doubles memory usage)
- `rollingUpdateBatchSize` greater than cluster size
