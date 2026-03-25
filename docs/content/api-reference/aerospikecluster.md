---
sidebar_position: 1
title: AerospikeCluster API Reference
---

# AerospikeCluster API Reference

This page documents the `AerospikeCluster` Custom Resource Definition (CRD) types.

**API Group:** `acko.io`
**API Version:** `v1alpha1`
**Kind:** `AerospikeCluster`
**Short Names:** `asc`

---

## AerospikeCluster

AerospikeCluster is the Schema for the `aerospikeclusters` API. It manages the lifecycle of an Aerospike Community Edition cluster.

| Field | Type | Description |
|---|---|---|
| `apiVersion` | string | `acko.io/v1alpha1` |
| `kind` | string | `AerospikeCluster` |
| `metadata` | [ObjectMeta](https://kubernetes.io/docs/reference/kubernetes-api/common-definitions/object-meta/) | Standard object metadata |
| `spec` | [AerospikeClusterSpec](#aerospikeclusterspec) | Desired state of the cluster |
| `status` | [AerospikeClusterStatus](#aerospikeclusterstatus) | Observed state of the cluster |

---

## AerospikeClusterSpec

Defines the desired state of an Aerospike CE cluster.

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `size` | int32 | Yes | — | Number of Aerospike pods. CE max: 8. |
| `image` | string | Yes | — | Aerospike CE container image (e.g., `aerospike:ce-8.1.1.1`). |
| `aerospikeConfig` | [AerospikeConfigSpec](#aerospikeconfigspec) | No | — | Raw Aerospike configuration map, converted to `aerospike.conf`. |
| `storage` | [AerospikeStorageSpec](#aerospikestoragespec) | No | — | Volume definitions for Aerospike pods. |
| `rackConfig` | [RackConfig](#rackconfig) | No | — | Rack-aware deployment topology. |
| `aerospikeNetworkPolicy` | [AerospikeNetworkPolicy](#aerospikenetworkpolicy) | No | — | Client access network configuration. |
| `podSpec` | [AerospikePodSpec](#aerospikepodspec) | No | — | Pod-level configuration. |
| `aerospikeAccessControl` | [AerospikeAccessControlSpec](#aerospikeaccesscontrolspec) | No | — | ACL roles and users. |
| `monitoring` | [AerospikeMonitoringSpec](#aerospikemonitoringspec) | No | — | Prometheus monitoring configuration. |
| `networkPolicyConfig` | [NetworkPolicyConfig](#networkpolicyconfig) | No | — | Automatic NetworkPolicy creation. |
| `bandwidthConfig` | [BandwidthConfig](#bandwidthconfig) | No | — | CNI bandwidth annotations. |
| `enableDynamicConfigUpdate` | *bool | No | — | Enable runtime config changes via `set-config`. |
| `rollingUpdateBatchSize` | *int32 | No | `1` | Number of pods to restart in parallel during rolling update. |
| `disablePDB` | *bool | No | `false` | Disable PodDisruptionBudget creation. |
| `maxUnavailable` | [IntOrString](https://pkg.go.dev/k8s.io/apimachinery/pkg/util/intstr#IntOrString) | No | `1` | Max pods unavailable during disruption. |
| `paused` | *bool | No | `false` | Stop reconciliation when true. |
| `seedsFinderServices` | [SeedsFinderServices](#seedsfinderservices) | No | — | LoadBalancer service for seed discovery. |
| `k8sNodeBlockList` | []string | No | — | Node names to exclude from scheduling. |
| `operations` | [][OperationSpec](#operationspec) | No | — | On-demand operations (WarmRestart, PodRestart). Max 1 at a time. |
| `validationPolicy` | [ValidationPolicySpec](#validationpolicyspec) | No | — | Controls webhook validation behavior. |
| `headlessService` | [AerospikeServiceSpec](#aerospikeservicespec) | No | — | Custom metadata for the headless service. |
| `podService` | [AerospikeServiceSpec](#aerospikeservicespec) | No | — | Custom metadata for per-pod services. Creates individual Service per pod when set. |
| `enableRackIDOverride` | *bool | No | `false` | Enable dynamic rack ID assignment via pod annotations. |
| `templateRef` | [TemplateRef](#templateref) | No | — | Reference to an `AerospikeClusterTemplate`. When set, the template spec is resolved and stored as a snapshot at creation time. |
| `overrides` | [AerospikeClusterTemplateSpec](./aerospikeclustertemplate#aerospikeclustertemplatespec) | No | — | Fields that override the referenced template. Merge priority: overrides > template > operator defaults. |

---

## TemplateRef

Reference to an `AerospikeClusterTemplate` in the same namespace.

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | Yes | Name of the `AerospikeClusterTemplate` resource |

---

## TemplateSnapshotStatus

Recorded in `status.templateSnapshot` after a template is resolved.

| Field | Type | Description |
|---|---|---|
| `name` | string | Name of the referenced template |
| `resourceVersion` | string | ResourceVersion of the template at snapshot time |
| `snapshotTimestamp` | Time | When the snapshot was taken |
| `synced` | bool | Whether the cluster uses the latest template version. Set to `false` when the template changes after the snapshot. |
| `spec` | [AerospikeClusterTemplateSpec](./aerospikeclustertemplate#aerospikeclustertemplatespec) | Resolved template spec at snapshot time. |

---

## AerospikeConfigSpec

Holds the raw Aerospike configuration as an unstructured JSON/YAML object. The operator converts this to `aerospike.conf` format.

This is a `map[string]interface{}` wrapper. Access via `.Value` in Go code. In YAML, write the Aerospike configuration directly:

```yaml
aerospikeConfig:
  service:
    cluster-name: my-cluster
    proto-fd-max: 15000
  network:
    service:
      port: 3000
    heartbeat:
      mode: mesh
      port: 3002
    fabric:
      port: 3001
  namespaces:
    - name: testns
      replication-factor: 2
      storage-engine:
        type: device
        file: /opt/aerospike/data/testns.dat
        filesize: 4294967296
  logging:
    - name: /var/log/aerospike/aerospike.log
      context: any info
```

---

## AerospikeClusterStatus

Observed state of the Aerospike CE cluster.

| Field | Type | Description |
|---|---|---|
| `phase` | string | Cluster phase: `InProgress`, `Completed`, `Error`, `ScalingUp`, `ScalingDown`, `WaitingForMigration`, `RollingRestart`, `ACLSync`, `Paused`, `Deleting`. |
| `size` | int32 | Current number of ready pods. |
| `health` | string | Human-readable pod readiness summary in `ready/total` format (e.g., `3/3`). |
| `conditions` | [][Condition](https://kubernetes.io/docs/reference/kubernetes-api/common-definitions/conditions/) | Latest observations of cluster state. |
| `pods` | map[string][AerospikePodStatus](#aerospikepodstatus) | Per-pod status information, keyed by pod name. |
| `observedGeneration` | int64 | Most recent generation observed by the controller. |
| `selector` | string | Label selector string for HPA compatibility. |
| `aerospikeConfig` | [AerospikeConfigSpec](#aerospikeconfigspec) | Last applied Aerospike configuration. |
| `operationStatus` | [OperationStatus](#operationstatus) | Current on-demand operation status. |
| `phaseReason` | string | Human-readable explanation of the current phase (e.g., "Rolling restart in progress for rack 1"). |
| `appliedSpec` | [AerospikeClusterSpec](#aerospikeclusterspec) | Copy of the last successfully reconciled spec. Used to detect configuration drift. |
| `aerospikeClusterSize` | int32 | Aerospike cluster-size as reported by `asinfo`. May differ from K8s pod count during split-brain or rolling restarts. |
| `operatorVersion` | string | Version of the operator that last reconciled this cluster. |
| `pendingRestartPods` | []string | Pods queued for restart in the current rolling restart. Cleared when complete. |
| `lastReconcileTime` | [Time](https://kubernetes.io/docs/reference/kubernetes-api/common-definitions/object-meta/#System) | Timestamp of the last successful reconciliation. |
| `templateSnapshot` | [TemplateSnapshotStatus](#templatesnapshotstatus) | Resolved template spec at last sync time. |
| `failedReconcileCount` | int32 | Number of consecutive failed reconciliations. Reset to 0 on success. When this exceeds the circuit breaker threshold (default 10), the operator backs off exponentially. |
| `lastReconcileError` | string | Error message from the most recent failed reconciliation. Cleared on success. |
| `migrationStatus` | [MigrationStatus](#migrationstatus) | Cluster-level data migration progress. Updated on each reconciliation by querying Aerospike nodes for partition migration statistics. |

---

## MigrationStatus

Tracks the current data migration state of the cluster.

| Field | Type | Description |
|---|---|---|
| `inProgress` | bool | Whether any data migration is currently happening. |
| `remainingPartitions` | int64 | Total number of partitions still to be migrated across all nodes. `0` means migration is complete. |
| `lastChecked` | [Time](https://kubernetes.io/docs/reference/kubernetes-api/common-definitions/object-meta/#System) | Timestamp of the last migration status check. |

---

## Condition Types

The operator maintains the following condition types in `status.conditions`:

| Type | Description |
|---|---|
| `Available` | At least one pod is ready to serve requests. |
| `Ready` | All desired pods are running and ready. |
| `ConfigApplied` | All pods have the desired Aerospike configuration. |
| `ACLSynced` | ACL roles and users are synchronized with the cluster. |
| `MigrationComplete` | No data migrations are pending. |
| `ReconciliationPaused` | Reconciliation is paused by the user (`spec.paused: true`). |

---

## AerospikePodStatus

Per-pod status information.

| Field | Type | Description |
|---|---|---|
| `podIP` | string | Pod IP address. |
| `hostIP` | string | Host node IP address. |
| `image` | string | Container image running on the pod. |
| `podPort` | int32 | Aerospike service port on the pod. |
| `servicePort` | int32 | Aerospike service port exposed via node/LB. |
| `rack` | int | Rack ID assigned to this pod. |
| `initializedVolumes` | []string | Volumes that have been initialized. |
| `isRunningAndReady` | bool | Whether the pod is running and ready. |
| `configHash` | string | SHA256 hash of the applied config. |
| `podSpecHash` | string | Hash of the pod template spec. |
| `dynamicConfigStatus` | string | Dynamic config update result: `Applied`, `Failed`, `Pending`, or empty. |
| `dirtyVolumes` | []string | Volumes needing initialization or cleanup. |
| `nodeID` | string | Aerospike-assigned node identifier (e.g., `BB9020012AC4202`). Empty if unreachable. |
| `clusterName` | string | Aerospike cluster name as reported by the node. |
| `accessEndpoints` | []string | Network endpoints (`host:port`) for direct client access via `asinfo "service"`. |
| `readinessGateSatisfied` | bool | Whether `acko.io/aerospike-ready` gate is `True`. Only meaningful when `readinessGateEnabled=true`. |
| `lastRestartReason` | [RestartReason](#restartreason) | Reason the pod was last restarted by the operator. |
| `lastRestartTime` | [Time](https://kubernetes.io/docs/reference/kubernetes-api/common-definitions/object-meta/#System) | When the pod was last restarted by the operator. |
| `unstableSince` | [Time](https://kubernetes.io/docs/reference/kubernetes-api/common-definitions/object-meta/#System) | First time this pod became NotReady. Reset to `nil` when Ready. |
| `migratingPartitions` | *int64 | Number of partitions this pod is currently migrating. Populated by querying the node's `migrate_partitions_remaining` statistic. `nil` if unreachable. |

---

## RestartReason

Describes why a pod was restarted by the operator.

| Value | Description |
|---|---|
| `ConfigChanged` | Cold restart triggered by an Aerospike config change. |
| `ImageChanged` | Pod image was updated. |
| `PodSpecChanged` | Pod spec (resources, env, etc.) changed. |
| `ManualRestart` | On-demand pod restart (`OperationPodRestart`). |
| `WarmRestart` | On-demand or rolling warm restart (SIGUSR1). |

---

## AerospikeStorageSpec

Defines storage volumes for Aerospike pods.

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `volumes` | [][VolumeSpec](#volumespec) | No | — | List of volumes to attach. |
| `cleanupThreads` | int32 | No | `1` | Max threads for volume cleanup/init. |
| `filesystemVolumePolicy` | [AerospikeVolumePolicy](#aerospikevolumepolicy) | No | — | Default policy for filesystem-mode persistent volumes. Per-volume settings override this. |
| `blockVolumePolicy` | [AerospikeVolumePolicy](#aerospikevolumepolicy) | No | — | Default policy for block-mode persistent volumes. Per-volume settings override this. |
| `localStorageClasses` | []string | No | — | StorageClass names using local storage (e.g., `local-path`). Volumes using these classes require special handling on pod restart. |
| `deleteLocalStorageOnRestart` | *bool | No | — | Delete local PVCs before pod restart, forcing re-provisioning on new node. |

---

## AerospikeVolumePolicy

Default policies for a category of persistent volumes (filesystem or block).

| Field | Type | Default | Description |
|---|---|---|---|
| `initMethod` | string | `none` | Default init method for this volume category. |
| `wipeMethod` | string | `none` | Default wipe method for this volume category. |
| `cascadeDelete` | *bool | `nil` | Delete PVCs when the CR is deleted. |

---

## VolumeSpec

Defines a single volume attachment.

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `name` | string | Yes | — | Volume name. |
| `source` | [VolumeSource](#volumesource) | Yes | — | Volume source (PVC, emptyDir, secret, configMap, hostPath). |
| `aerospike` | [AerospikeVolumeAttachment](#aerospikevolumeattachment) | No | — | Mount path in Aerospike container. |
| `sidecars` | [][VolumeAttachment](#volumeattachment) | No | — | Volume mounts for sidecar containers. |
| `initContainers` | [][VolumeAttachment](#volumeattachment) | No | — | Volume mounts for init containers. |
| `initMethod` | string | No | `none` | Init method: `none`, `deleteFiles`, `dd`, `blkdiscard`, `headerCleanup`. |
| `wipeMethod` | string | No | `none` | Wipe method for dirty volumes: `none`, `deleteFiles`, `dd`, `blkdiscard`, `headerCleanup`, `blkdiscardWithHeaderCleanup`. |
| `cascadeDelete` | *bool | No | `nil` | Delete PVC when CR is deleted. When `nil`, falls back to global volume policy. |

---

## VolumeSource

Describes the volume data source. Exactly one field should be set.

| Field | Type | Description |
|---|---|---|
| `persistentVolume` | [PersistentVolumeSpec](#persistentvolumespec) | Create a PVC. |
| `emptyDir` | [EmptyDirVolumeSource](https://kubernetes.io/docs/reference/kubernetes-api/config-and-storage-resources/volume/#emptyDir) | Use emptyDir. |
| `secret` | [SecretVolumeSource](https://kubernetes.io/docs/reference/kubernetes-api/config-and-storage-resources/volume/#secret) | Use a Kubernetes Secret. |
| `configMap` | [ConfigMapVolumeSource](https://kubernetes.io/docs/reference/kubernetes-api/config-and-storage-resources/volume/#configMap) | Use a Kubernetes ConfigMap. |
| `hostPath` | [HostPathVolumeSource](https://kubernetes.io/docs/reference/kubernetes-api/config-and-storage-resources/volume/#hostPath) | Use a path on the host node. |

---

## PersistentVolumeSpec

Defines a persistent volume claim template.

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `storageClass` | string | No | — | StorageClass name. |
| `volumeMode` | string | No | `Filesystem` | `Filesystem` or `Block`. |
| `size` | string | Yes | — | Storage size (e.g., `10Gi`). |
| `accessModes` | []string | No | — | Access modes (e.g., `ReadWriteOnce`). |
| `selector` | [LabelSelector](https://kubernetes.io/docs/reference/kubernetes-api/common-definitions/label-selector/) | No | — | Label selector for PV binding. |
| `metadata` | [AerospikeObjectMeta](#aerospikeobjectmeta) | No | — | Custom labels and annotations for the PVC. |

---

## AerospikeVolumeAttachment

Defines how a volume is mounted in the Aerospike container.

| Field | Type | Required | Description |
|---|---|---|---|
| `path` | string | Yes | Mount path in the container. |
| `readOnly` | bool | No | Mount the volume as read-only. |
| `subPath` | string | No | Mount only a sub-path of the volume. |
| `subPathExpr` | string | No | Expanded path using environment variables. Mutually exclusive with `subPath`. |
| `mountPropagation` | [MountPropagationMode](https://kubernetes.io/docs/concepts/storage/volumes/#mount-propagation) | No | How mounts are propagated: `None`, `HostToContainer`, `Bidirectional`. |

---

## VolumeAttachment

Defines a volume mount for sidecar or init containers.

| Field | Type | Required | Description |
|---|---|---|---|
| `containerName` | string | Yes | Target container name. |
| `path` | string | Yes | Mount path in the container. |
| `readOnly` | bool | No | Mount the volume as read-only. |
| `subPath` | string | No | Mount only a sub-path of the volume. |
| `subPathExpr` | string | No | Expanded path using environment variables. Mutually exclusive with `subPath`. |
| `mountPropagation` | [MountPropagationMode](https://kubernetes.io/docs/concepts/storage/volumes/#mount-propagation) | No | How mounts are propagated: `None`, `HostToContainer`, `Bidirectional`. |

---

## AerospikeNetworkPolicy

Defines network access configuration.

| Field | Type | Default | Description |
|---|---|---|---|
| `accessType` | string | `pod` | Client access type: `pod`, `hostInternal`, `hostExternal`, `configuredIP`. |
| `alternateAccessType` | string | `pod` | Alternate access type. |
| `fabricType` | string | `pod` | Fabric (inter-node) network type. |
| `customAccessNetworkNames` | []string | — | Network names for `configuredIP` access. |
| `customAlternateAccessNetworkNames` | []string | — | Network names for `configuredIP` alternate access. |
| `customFabricNetworkNames` | []string | — | Network names for `configuredIP` fabric. |

---

## SeedsFinderServices

Configures external seed discovery via LoadBalancer.

| Field | Type | Description |
|---|---|---|
| `loadBalancer` | [LoadBalancerSpec](#loadbalancerspec) | LoadBalancer service configuration. |

---

## LoadBalancerSpec

Defines a LoadBalancer service.

| Field | Type | Default | Description |
|---|---|---|---|
| `annotations` | map[string]string | — | Service annotations. |
| `labels` | map[string]string | — | Service labels. |
| `externalTrafficPolicy` | string | — | `Cluster` or `Local`. |
| `port` | int32 | `3000` | External port. |
| `targetPort` | int32 | `3000` | Container target port. |
| `loadBalancerSourceRanges` | []string | — | Allowed source CIDRs. |

---

## AerospikePodSpec

Pod-level customization for Aerospike pods.

| Field | Type | Description |
|---|---|---|
| `aerospikeContainer` | [AerospikeContainerSpec](#aerospikecontainerspec) | Aerospike container customization. |
| `sidecars` | [][Container](https://kubernetes.io/docs/reference/kubernetes-api/workload-resources/pod-v1/#Container) | Sidecar containers. |
| `initContainers` | [][Container](https://kubernetes.io/docs/reference/kubernetes-api/workload-resources/pod-v1/#Container) | Additional init containers. |
| `imagePullSecrets` | [][LocalObjectReference](https://kubernetes.io/docs/reference/kubernetes-api/common-definitions/local-object-reference/) | Image pull secrets. |
| `nodeSelector` | map[string]string | Node labels for scheduling. |
| `tolerations` | [][Toleration](https://kubernetes.io/docs/reference/kubernetes-api/workload-resources/pod-v1/#scheduling) | Pod tolerations. |
| `affinity` | [Affinity](https://kubernetes.io/docs/reference/kubernetes-api/workload-resources/pod-v1/#scheduling) | Affinity/anti-affinity rules. |
| `securityContext` | [PodSecurityContext](https://kubernetes.io/docs/reference/kubernetes-api/workload-resources/pod-v1/#security-context) | Pod-level security attributes. |
| `serviceAccountName` | string | ServiceAccount name. |
| `dnsPolicy` | string | DNS policy for the pod. |
| `hostNetwork` | bool | Enable host networking. |
| `multiPodPerHost` | *bool | Allow multiple pods on the same node. |
| `terminationGracePeriodSeconds` | *int64 | Pod termination grace period. |
| `topologySpreadConstraints` | [][TopologySpreadConstraint](https://kubernetes.io/docs/reference/kubernetes-api/workload-resources/pod-v1/#scheduling) | How pods spread across topology domains. |
| `podManagementPolicy` | string | StatefulSet pod management: `OrderedReady` (default) or `Parallel`. |
| `metadata` | [AerospikePodMetadata](#aerospikepodmetadata) | Additional pod labels/annotations. |
| `readinessGateEnabled` | *bool | Enable custom readiness gate `acko.io/aerospike-ready`. Pods excluded from Service endpoints until Aerospike joins cluster mesh and finishes migrations. |
| `priorityClassName` | string | PriorityClass name for pod scheduling priority and preemption. The PriorityClass must already exist in the cluster. |

---

## AerospikeContainerSpec

Customizes the Aerospike server container.

| Field | Type | Description |
|---|---|---|
| `resources` | [ResourceRequirements](https://kubernetes.io/docs/reference/kubernetes-api/workload-resources/pod-v1/#resources) | CPU/memory requests and limits. |
| `securityContext` | [SecurityContext](https://kubernetes.io/docs/reference/kubernetes-api/workload-resources/pod-v1/#security-context-1) | Container-level security attributes. |

---

## AerospikePodMetadata

Extra labels and annotations for pods.

| Field | Type | Description |
|---|---|---|
| `labels` | map[string]string | Additional pod labels. |
| `annotations` | map[string]string | Additional pod annotations. |

---

## RackConfig

Defines rack-aware deployment configuration.

| Field | Type | Required | Description |
|---|---|---|---|
| `racks` | [][Rack](#rack) | Yes | List of rack definitions (min 1). |
| `namespaces` | []string | No | Aerospike namespace names that are rack-aware. |
| `scaleDownBatchSize` | [IntOrString](https://pkg.go.dev/k8s.io/apimachinery/pkg/util/intstr#IntOrString) | No | Pods to scale down simultaneously per rack. Int or percent string (e.g., `"25%"`). Default: 1. |
| `maxIgnorablePods` | [IntOrString](https://pkg.go.dev/k8s.io/apimachinery/pkg/util/intstr#IntOrString) | No | Max pending/failed pods to ignore during reconciliation. |
| `rollingUpdateBatchSize` | [IntOrString](https://pkg.go.dev/k8s.io/apimachinery/pkg/util/intstr#IntOrString) | No | Pods to restart simultaneously per rack. Int or percent string. Takes precedence over `spec.rollingUpdateBatchSize`. |

---

## Rack

Defines a single rack in the cluster topology.

| Field | Type | Required | Description |
|---|---|---|---|
| `id` | int | Yes | Unique rack identifier (>= 1). Rack ID 0 is reserved for the default rack. |
| `zone` | string | No | Zone label value (`topology.kubernetes.io/zone`). |
| `region` | string | No | Region label value (`topology.kubernetes.io/region`). |
| `nodeName` | string | No | Constrain to a specific node. |
| `rackLabel` | string | No | Custom label for rack affinity. Schedules to nodes with `acko.io/rack=<rackLabel>`. Must be unique across racks. |
| `revision` | string | No | Version identifier for controlled rack migrations. |
| `aerospikeConfig` | [AerospikeConfigSpec](#aerospikeconfigspec) | No | Per-rack Aerospike config override. |
| `storage` | [AerospikeStorageSpec](#aerospikestoragespec) | No | Per-rack storage override. |
| `podSpec` | [RackPodSpec](#rackpodspec) | No | Per-rack pod scheduling override. |

---

## RackPodSpec

Rack-level pod scheduling overrides.

| Field | Type | Description |
|---|---|---|
| `affinity` | [Affinity](https://kubernetes.io/docs/reference/kubernetes-api/workload-resources/pod-v1/#scheduling) | Rack-level affinity override. |
| `tolerations` | [][Toleration](https://kubernetes.io/docs/reference/kubernetes-api/workload-resources/pod-v1/#scheduling) | Rack-level tolerations override. |
| `nodeSelector` | map[string]string | Rack-level node selector override. |

---

## AerospikeAccessControlSpec

Defines ACL configuration.

| Field | Type | Description |
|---|---|---|
| `roles` | [][AerospikeRoleSpec](#aerospikerolespec) | Aerospike role definitions. |
| `users` | [][AerospikeUserSpec](#aerospikeuserspec) | Aerospike user definitions. |
| `adminPolicy` | [AerospikeClientAdminPolicy](#aerospikeclientadminpolicy) | Admin client timeout policy. |

---

## AerospikeRoleSpec

Defines an Aerospike role.

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | Yes | Role name. |
| `privileges` | []string | Yes | Privilege strings: `read`, `write`, `read-write`, `read-write-udf`, `sys-admin`, `user-admin`, `data-admin`, `truncate`. Supports namespace scoping (e.g., `read-write.testns`). |
| `whitelist` | []string | No | Allowed CIDR ranges. |

---

## AerospikeUserSpec

Defines an Aerospike user.

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | Yes | Username. |
| `secretName` | string | Yes | Kubernetes Secret name containing the password (key: `password`). |
| `roles` | []string | Yes | Assigned role names (min 1). |

---

## AerospikeClientAdminPolicy

Admin client timeout settings.

| Field | Type | Default | Description |
|---|---|---|---|
| `timeout` | int | `2000` | Admin operation timeout in milliseconds. |

---

## AerospikeMonitoringSpec

Prometheus monitoring configuration.

| Field | Type | Default | Description |
|---|---|---|---|
| `enabled` | bool | `false` | Enable Prometheus exporter sidecar. |
| `exporterImage` | string | `aerospike/aerospike-prometheus-exporter:1.16.1` | Exporter container image. |
| `port` | int32 | `9145` | Metrics port. |
| `resources` | [ResourceRequirements](https://kubernetes.io/docs/reference/kubernetes-api/workload-resources/pod-v1/#resources) | — | Exporter resource limits. |
| `env` | [][EnvVar](https://kubernetes.io/docs/reference/kubernetes-api/workload-resources/pod-v1/#environment-variables) | — | Additional environment variables for the exporter container. |
| `metricLabels` | map[string]string | — | Custom labels added to all exported metrics via `METRIC_LABELS` env var. |
| `serviceMonitor` | [ServiceMonitorSpec](#servicemonitorspec) | — | ServiceMonitor configuration. |
| `prometheusRule` | [PrometheusRuleSpec](#prometheusrulespec) | — | PrometheusRule configuration for cluster alerts. |

---

## ServiceMonitorSpec

ServiceMonitor configuration for Prometheus Operator.

| Field | Type | Default | Description |
|---|---|---|---|
| `enabled` | bool | `false` | Create ServiceMonitor resource. |
| `interval` | string | `30s` | Scrape interval. |
| `labels` | map[string]string | — | Additional labels for ServiceMonitor discovery. |

---

## PrometheusRuleSpec

PrometheusRule configuration for Aerospike cluster alerts.

| Field | Type | Default | Description |
|---|---|---|---|
| `enabled` | bool | `false` | Create PrometheusRule resource. |
| `labels` | map[string]string | — | Additional labels for PrometheusRule discovery. |
| `customRules` | []JSON | — | Custom rule groups replacing built-in alerts (NodeDown, StopWrites, HighDiskUsage, HighMemoryUsage). Each entry must be a complete Prometheus rule group object with `name` and `rules` fields. |

---

## NetworkPolicyConfig

Automatic NetworkPolicy creation.

| Field | Type | Default | Description |
|---|---|---|---|
| `enabled` | bool | `false` | Enable NetworkPolicy creation. |
| `type` | string | `kubernetes` | Policy type: `kubernetes` or `cilium`. |

---

## BandwidthConfig

Bandwidth annotations for CNI traffic shaping.

| Field | Type | Description |
|---|---|---|
| `ingress` | string | Max ingress bandwidth (e.g., `1Gbps`, `500Mbps`). |
| `egress` | string | Max egress bandwidth (e.g., `1Gbps`, `500Mbps`). |

---

## OperationSpec

Defines an on-demand operation to trigger on cluster pods.

| Field | Type | Required | Description |
|---|---|---|---|
| `kind` | string | Yes | Operation type: `WarmRestart` (SIGUSR1) or `PodRestart` (delete/recreate). |
| `id` | string | Yes | Unique operation identifier (1-20 characters). |
| `podList` | []string | No | Specific pod names to target. Empty means all pods. |

---

## OperationStatus

Tracks the status of an on-demand operation.

| Field | Type | Description |
|---|---|---|
| `id` | string | Operation identifier. |
| `kind` | string | Operation type: `WarmRestart` or `PodRestart`. |
| `phase` | string | Operation phase: `InProgress`, `Completed`, or `Error`. |
| `completedPods` | []string | Pods that have completed the operation. |
| `failedPods` | []string | Pods where the operation failed. |

---

## ValidationPolicySpec

Controls webhook validation behavior.

| Field | Type | Default | Description |
|---|---|---|---|
| `skipWorkDirValidate` | bool | `false` | Skip validation that the Aerospike work directory is on persistent storage. |

---

## AerospikeServiceSpec

Defines custom metadata for a Kubernetes Service.

| Field | Type | Required | Description |
|---|---|---|---|
| `metadata` | [AerospikeObjectMeta](#aerospikeobjectmeta) | No | Custom annotations and labels for the service. |

---

## AerospikeObjectMeta

Custom metadata for Kubernetes objects.

| Field | Type | Description |
|---|---|---|
| `annotations` | map[string]string | Custom annotations. |
| `labels` | map[string]string | Custom labels. |
