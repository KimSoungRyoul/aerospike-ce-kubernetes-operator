---
sidebar_position: 2
title: AerospikeClusterTemplate API Reference
---

# AerospikeClusterTemplate API Reference

This page documents the `AerospikeClusterTemplate` Custom Resource Definition (CRD) types.

**API Group:** `acko.io`
**API Version:** `v1alpha1`
**Kind:** `AerospikeClusterTemplate`
**Short Names:** `asct`

---

## Overview

`AerospikeClusterTemplate` is a reusable configuration profile for `AerospikeCluster`. It lets you define shared settings (scheduling, storage, resources, Aerospike config) once and reference them from multiple clusters via `spec.templateRef`.

**Snapshot strategy:** The template spec is copied into `status.templateSnapshot` at cluster creation time. Subsequent template changes are **not** automatically propagated. To resync, set the annotation `acko.io/resync-template: "true"` on the cluster object.

---

## AerospikeClusterTemplate

| Field | Type | Description |
|---|---|---|
| `apiVersion` | string | `acko.io/v1alpha1` |
| `kind` | string | `AerospikeClusterTemplate` |
| `metadata` | [ObjectMeta](https://kubernetes.io/docs/reference/kubernetes-api/common-definitions/object-meta/) | Standard object metadata |
| `spec` | [AerospikeClusterTemplateSpec](#aerospikeclustertemplatespec) | Configuration profile |
| `status` | [AerospikeClusterTemplateStatus](#aerospikeclustertemplatestatus) | Observed state |

---

## AerospikeClusterTemplateSpec

| Field | Type | Description |
|---|---|---|
| `description` | string | Human-readable description of the template's purpose or target environment (max 500 characters). |
| `aerospikeConfig` | [TemplateAerospikeConfig](#templateaerospikeconfig) | Aerospike configuration defaults |
| `scheduling` | [TemplateScheduling](#templatescheduling) | Pod scheduling defaults |
| `storage` | [TemplateStorage](#templatestorage) | Data volume defaults |
| `resources` | [ResourceRequirements](https://kubernetes.io/docs/reference/kubernetes-api/workload-resources/pod-v1/#resources) | Container CPU/memory defaults |
| `rackConfig` | [TemplateRackConfig](#templaterackconfig) | Rack configuration defaults |
| `image` | string | Default Aerospike CE container image (e.g. `aerospike:ce-8.1.1.1`). Applied when `spec.image` is not set in the cluster. |
| `size` | integer | Default cluster size (1–8). Applied when `spec.size` is `0` (not set) in the cluster. |
| `monitoring` | [AerospikeMonitoringSpec](./aerospikecluster.md#aerospikemonitoringspec) | Default Prometheus exporter sidecar configuration. Applied when `spec.monitoring` is not set in the cluster. |
| `aerospikeNetworkPolicy` | [AerospikeNetworkPolicy](./aerospikecluster.md#aerospikenetworkpolicy) | Default network access configuration. Applied when `spec.aerospikeNetworkPolicy` is not set in the cluster. |

---

## TemplateAerospikeConfig

| Field | Type | Description |
|---|---|---|
| `namespaceDefaults` | object | Base configuration merged into every namespace defined in the cluster's `aerospikeConfig.namespaces`. Cluster-level settings override these defaults. |
| `service` | object | Defaults for the `service` section of `aerospikeConfig`. Cluster-level settings override these defaults. |
| `network` | [TemplateNetworkConfig](#templatenetworkconfig) | Network configuration defaults |

---

## TemplateNetworkConfig

| Field | Type | Description |
|---|---|---|
| `heartbeat` | [TemplateHeartbeatConfig](#templateheartbeatconfig) | Heartbeat configuration defaults |

---

## TemplateHeartbeatConfig

| Field | Type | Description |
|---|---|---|
| `mode` | string | Heartbeat mode. Must be `mesh` for CE. |
| `interval` | integer | Heartbeat interval in milliseconds |
| `timeout` | integer | Heartbeat timeout in milliseconds |

---

## TemplateScheduling

| Field | Type | Description |
|---|---|---|
| `podAntiAffinityLevel` | string | Pod anti-affinity policy: `none`, `preferred`, or `required`. `required` enforces one Aerospike pod per node. |
| `nodeAffinity` | [NodeAffinity](https://kubernetes.io/docs/reference/kubernetes-api/workload-resources/pod-v1/#NodeAffinity) | Node affinity rules for pod scheduling |
| `tolerations` | []Toleration | Pod scheduling tolerations |
| `topologySpreadConstraints` | []TopologySpreadConstraint | How pods are spread across topology domains |
| `podManagementPolicy` | string | StatefulSet pod management policy: `OrderedReady` or `Parallel` |

### PodAntiAffinityLevel values

| Value | Behavior |
|---|---|
| `none` | No anti-affinity rules are injected |
| `preferred` | Soft rule (weight=100): prefer spreading pods across nodes |
| `required` | Hard rule: exactly one Aerospike pod per node |

---

## TemplateStorage

| Field | Type | Description |
|---|---|---|
| `storageClassName` | string | Kubernetes StorageClass for the data PVC |
| `volumeMode` | string | `Filesystem` (default) or `Block` |
| `accessModes` | []string | PVC access modes (default: `ReadWriteOnce`) |
| `resources` | [VolumeResourceRequirements](https://kubernetes.io/docs/reference/kubernetes-api/config-and-storage-resources/persistent-volume-claim-v1/#VolumeResourceRequirements) | Storage size request |
| `localPVRequired` | boolean | When `true`, asserts that the StorageClass uses `WaitForFirstConsumer` binding mode (local PV) |

---

## TemplateRackConfig

| Field | Type | Description |
|---|---|---|
| `maxRacksPerNode` | integer | Maximum racks per Kubernetes node. When set to `1`, a warning is raised if `podAntiAffinityLevel` is not `required`. |

---

## AerospikeClusterTemplateStatus

| Field | Type | Description |
|---|---|---|
| `usedBy` | []string | List of `AerospikeCluster` names that reference this template |

---

## Using Templates in a Cluster

Reference a template via `spec.templateRef` in `AerospikeCluster`:

```yaml
apiVersion: acko.io/v1alpha1
kind: AerospikeCluster
metadata:
  name: my-cluster
spec:
  size: 3
  image: aerospike:ce-8.1.1.1
  templateRef:
    name: prod          # references AerospikeClusterTemplate named "prod"
  overrides:            # optional: override specific fields from the template
    resources:
      requests:
        cpu: "1"
        memory: "2Gi"
      limits:
        cpu: "2"
        memory: "2Gi"
```

### Resync after template update

Template changes are not automatically applied to clusters. To resync:

```bash
kubectl annotate aerospikecluster my-cluster acko.io/resync-template=true
```

The operator will re-fetch the template, update `status.templateSnapshot`, emit a `TemplateApplied` event, and remove the annotation.

---

## Validation Rules

| Rule | Description |
|---|---|
| V-T01 | `scheduling.podAntiAffinityLevel` must be `none`, `preferred`, or `required` |
| V-T02 | `rackConfig.maxRacksPerNode` must be >= 0 |
| V-T03 | `storage.localPVRequired=true` without `storageClassName` raises a warning |
| V-T04 | For Guaranteed QoS, resource requests should equal limits (warning) |
| V-T05 | `scheduling.podManagementPolicy` must be `OrderedReady` or `Parallel` |
| V-T06 | `image` should contain `ce-` to confirm it is a Community Edition image (warning) |
| V-T07 | `size` must be between 1 and 8 when specified (CE cluster limit) |
| V-T08 | `monitoring.port` must be between 1 and 65535 when specified |
