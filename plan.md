# Aerospike CE Kubernetes Operator - Feature Implementation Plan

> Based on analysis of the official AKO (Aerospike Kubernetes Operator) documentation, this is a TODO list of features to implement compared to the current implementation state.
>
> Created: 2026-02-22 (P0 implementation complete: 2026-02-23, P1 implementation complete: 2026-02-23)
> Reference: https://aerospike.com/docs/kubernetes/
> PR(P0): https://github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/pull/7
> PR(P1): https://github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/pull/10

---

A total of 27 TODO items classified into 6 priority levels:

Priority: P0 ✅ Complete (PR #7)
Category: Configure Cluster
Item count: 6 (6/6 complete)
Key items: On-demand Operations, ScaleDownBatchSize, MaxIgnorablePods, ValidationPolicy, Headless/Pod Service customization, Rack additional fields
────────────────────────────────────────
Priority: P1 ✅ Complete (PR #10)
Category: Advanced Storage
Item count: 6 (6/6 complete)
Key items: Global Volume Policy, WipeMethod, HostPath, Local Storage awareness, Mount options, PVC Metadata
────────────────────────────────────────
Priority: P2
Category: Observability
Item count: 4
Key items: Operator Self-Monitoring, Grafana Dashboard, Alert Rules, Logging validation
────────────────────────────────────────
Priority: P3
Category: Security
Item count: 2
Key items: ACL Quota, Non-Root guide
────────────────────────────────────────
Priority: P4
Category: Advanced Networking
Item count: 2
Key items: CustomInterface (Multus CNI), Per-Pod Service
────────────────────────────────────────
Priority: P5
Category: Operational Convenience
Item count: 3
Key items: HPA, Init Container customization, Multi-Namespace Watch
────────────────────────────────────────
Priority: P6
Category: EE Compatibility Preparation
Item count: 4
Key items: TLS, LDAP, XDR, Strong Consistency

Each item includes the official AKO documentation URL, CRD YAML examples, and a concrete checklist.

## Currently Implemented Features (Already Implemented)

| Category | Feature | Status |
|---------|------|------|
| **Core** | Size (1-8 CE limit), Image, AerospikeConfig, Paused | Done |
| **Core** | RollingUpdateBatchSize | Done |
| **Storage** | PersistentVolume, EmptyDir, Secret, ConfigMap sources | Done |
| **Storage** | InitMethod (none, deleteFiles, dd, blkdiscard, headerCleanup) | Done |
| **Storage** | CascadeDelete, CleanupThreads | Done |
| **Storage** | Per-rack storage override | Done |
| **Network** | AccessType (pod, hostInternal, hostExternal, configuredIP) | Done |
| **Network** | AlternateAccessType, FabricType | Done |
| **Network** | Mesh seed auto-injection (heartbeat config) | Done |
| **Network** | LoadBalancer SeedsFinderServices | Done |
| **Network** | K8s NetworkPolicy / Cilium NetworkPolicy auto-generation | Done |
| **Network** | Bandwidth throttling (Ingress/Egress) | Done |
| **Rack** | Multi-rack (zone, region, nodeName based) | Done |
| **Rack** | Per-rack AerospikeConfig, Storage, PodSpec override | Done |
| **Pod** | Resources, SecurityContext, Sidecars, InitContainers | Done |
| **Pod** | Affinity, Tolerations, NodeSelector | Done |
| **Pod** | HostNetwork, MultiPodPerHost, DNSPolicy | Done |
| **Pod** | ImagePullSecrets, ServiceAccountName | Done |
| **Pod** | TerminationGracePeriodSeconds, Metadata (labels/annotations) | Done |
| **ACL** | Role CRUD (privileges, whitelist) | Done |
| **ACL** | User CRUD (K8s Secret-based password) | Done |
| **ACL** | Admin user required validation | Done |
| **Monitoring** | Prometheus exporter sidecar | Done |
| **Monitoring** | ServiceMonitor (Prometheus Operator) | Done |
| **Monitoring** | Metrics Service (ClusterIP) | Done |
| **PDB** | PodDisruptionBudget (MaxUnavailable, DisablePDB) | Done |
| **Dynamic Config** | EnableDynamicConfigUpdate, asinfo set-config | Done |
| **Restart** | Cold Restart (pod delete), Warm Restart (SIGUSR1) | Done |
| **Restart** | Config hash / PodSpec hash-based change detection | Done |
| **Node** | K8sNodeBlockList | Done |
| **Status** | Phase (InProgress/Completed/Error), Conditions | Done |
| **Status** | Per-pod status (IP, Image, Rack, Hashes) | Done |
| **Status** | DynamicConfigStatus per pod | Done |
| **Webhook** | Defaulter (ports, cluster-name, heartbeat mode) | Done |
| **Webhook** | Validator (CE limits: size<=8, ns<=2, no xdr/tls/ee-image) | Done |
| **Metrics** | Operator internal Prometheus metrics (reconcile duration, phase, etc.) | Done |
| **Operations** | On-Demand Operations (WarmRestart, PodRestart) with status tracking | Done (P0, PR #7) |
| **Operations** | Block changes during in-progress operations (ValidateUpdate) | Done (P0, PR #7) |
| **Rack** | ScaleDownBatchSize (IntOrString: absolute/percentage) | Done (P0, PR #7) |
| **Rack** | MaxIgnorablePods (ignore pending/failed pods) | Done (P0, PR #7) |
| **Rack** | RollingUpdateBatchSize at RackConfig level (IntOrString) | Done (P0, PR #7) |
| **Rack** | RackLabel (custom node affinity label) | Done (P0, PR #7) |
| **Rack** | Rack Revision (version identifier) | Done (P0, PR #7) |
| **Validation** | ValidationPolicy (skipWorkDirValidate) | Done (P0, PR #7) |
| **Service** | Headless Service custom annotations/labels | Done (P0, PR #7) |
| **Service** | Per-pod Service creation (spec.podService) | Done (P0, PR #7) |
| **Config** | EnableRackIDOverride (pod annotation-based rack ID) | **Removed (#344)** — only the CRD field ever landed; rack-id is Enterprise-only, so CE cannot implement it |
| **Storage** | Global Volume Policy (filesystemVolumePolicy / blockVolumePolicy) | Done (P1, PR #10) |
| **Storage** | WipeMethod (6 options: none, deleteFiles, dd, blkdiscard, headerCleanup, blkdiscardWithHeaderCleanup) | Done (P1, PR #10) |
| **Storage** | HostPath volume source (with webhook warning) | Done (P1, PR #10) |
| **Storage** | Local Storage awareness (localStorageClasses, deleteLocalStorageOnRestart) | Done (P1, PR #10) |
| **Storage** | Advanced Volume Mount options (readOnly, subPath, subPathExpr, mountPropagation) | Done (P1, PR #10) |
| **Storage** | PVC Metadata (custom labels/annotations) | Done (P1, PR #10) |

---

## TODO: Features to Implement (Priority Order)

### P0: Configure Aerospike Cluster (Highest Priority) ✅ Complete

> **Implementation complete**: 2026-02-23 | PR: [#7](https://github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/pull/7) (`feature/p0-implementation`)

#### 1. On-Demand Operations (spec.operations) ✅
> AKO docs: https://aerospike.com/docs/kubernetes/manage/node-maintenance/

Currently, automatic restarts only occur on config/image changes. AKO allows users to directly trigger restarts on specific pods on demand.

```yaml
spec:
  operations:
    - kind: WarmRestart    # WarmRestart | PodRestart
      id: "restart-001"    # Unique ID (1-20 characters)
      podList:             # Optional: target specific pods only (omit for all)
        - aerospike-0-0
        - aerospike-0-1
```

- [x] Define `OperationSpec` type (`kind`, `id`, `podList`)
  - `kind`: `WarmRestart` (SIGUSR1) / `PodRestart` (pod delete + recreate)
  - `id`: Unique string 1-20 characters (for tracking)
  - `podList`: Optional list of pod names (targets all pods if omitted)
  - `maxItems: 1` (only one operation at a time)
- [x] `OperationStatus` status tracking (`status.operationStatus`)
- [x] Reconciler logic for processing the operations field (`reconciler_operations.go`)
- [x] Status update after operation completes
- [x] Webhook validation: id validity, podList existence check
- [x] ValidateUpdate: block operation changes while InProgress

**Implementation files**:
- `api/v1alpha1/aerospikececluster_types.go` — `OperationSpec`, `OperationStatus`, `OperationKind` types
- `internal/controller/reconciler_operations.go` — `reconcileOperations`, `getOperationTargetPods`
- `api/v1alpha1/aerospikececluster_webhook.go` — `validateOperations`

#### 2. ScaleDownBatchSize / MaxIgnorablePods ✅
> AKO docs: https://aerospike.com/docs/kubernetes/manage/configure/rack-awareness/

```yaml
spec:
  rackConfig:
    rollingUpdateBatchSize: 2    # RackConfig level (IntOrString)
    scaleDownBatchSize: 1        # NEW: batch size during scale-down
    maxIgnorablePods: 1          # NEW: number of ignorable pending/failed pods
```

- [x] Add `ScaleDownBatchSize` field to `RackConfig` (`*intstr.IntOrString`)
- [x] Batch processing logic in scale-down reconciliation (`getScaleDownBatchSize`, `resolveIntOrPercent`)
- [x] Add `MaxIgnorablePods` field to `RackConfig` (`*intstr.IntOrString`)
- [x] Reconciler logic to ignore pending/failed pods (when evaluating cluster stability)
- [x] Add `RollingUpdateBatchSize` to `RackConfig` (`*intstr.IntOrString`, takes precedence over spec-level)

**Implementation files**:
- `api/v1alpha1/types_rack.go` — `ScaleDownBatchSize`, `MaxIgnorablePods`, `RollingUpdateBatchSize` fields
- `internal/controller/reconciler_statefulset.go` — `getScaleDownBatchSize`, `resolveIntOrPercent`
- `internal/controller/reconciler_restart.go` — `getRollingUpdateBatchSize`, `getMaxIgnorablePods`

#### 3. ValidationPolicy ✅
> AKO CRD Reference

```yaml
spec:
  validationPolicy:
    skipWorkDirValidate: false    # skip work directory PV mount validation
```

- [x] Define `ValidationPolicySpec` type
- [x] Add work directory volume mount validation to webhook (`validateWorkDirectory`)
- [x] `skipWorkDirValidate` flag to bypass validation

**Implementation files**:
- `api/v1alpha1/aerospikececluster_types.go` — `ValidationPolicySpec`
- `api/v1alpha1/aerospikececluster_webhook.go` — `validateWorkDirectory`

#### 4. Headless Service / Pod Service Customization ✅
> AKO docs: https://aerospike.com/docs/kubernetes/manage/configure/network-policy/

```yaml
spec:
  headlessService:
    metadata:
      annotations:
        service.beta.kubernetes.io/aws-load-balancer-type: "nlb"
      labels:
        custom-label: value
  podService:
    metadata:
      annotations: {}
      labels: {}
```

- [x] Add `spec.headlessService` field (`AerospikeServiceSpec` with metadata)
- [x] Add `spec.podService` field (per-pod Service creation)
- [x] Apply custom annotations/labels in headless Service reconciler
- [x] Implement per-pod Service reconciler (creates a ClusterIP Service named `<pod-name>-pod` for each pod)

**Implementation files**:
- `api/v1alpha1/aerospikececluster_types.go` — `AerospikeServiceSpec`, `AerospikeObjectMeta`
- `internal/controller/reconciler_services.go` — apply custom metadata to headless service
- `internal/controller/reconciler_pod_service.go` — `reconcilePodServices` (NEW)

#### 5. Additional Rack Fields ✅
> AKO docs: https://aerospike.com/docs/kubernetes/manage/configure/rack-awareness/

```yaml
spec:
  rackConfig:
    racks:
      - id: 1
        rackLabel: "rack-a"     # NEW: custom label-based affinity
        revision: "v2"          # NEW: version identifier
```

- [x] Add `RackLabel` field to `Rack` struct
- [x] Add `Revision` field to `Rack` struct
- [x] RackLabel-based node affinity scheduling logic (`acko.io/rack=<rackLabel>` NodeSelectorRequirement)
- [x] Webhook: validate RackLabel uniqueness

**Implementation files**:
- `api/v1alpha1/types_rack.go` — `RackLabel`, `Revision` fields
- `internal/podutil/pod.go` — RackLabel node affinity injection
- `api/v1alpha1/aerospikececluster_webhook.go` — RackLabel uniqueness validation

#### 6. EnableRackIDOverride — removed (#344)
> AKO CRD Reference

- [x] Add `spec.enableRackIDOverride` field
- [ ] ~~Dynamic rack ID assignment logic based on pod annotation~~ — never implemented

The field was added and marked done, but nothing ever read it. The logic it was
meant to enable writes `rack-id` into `aerospike.conf`, which is Enterprise-only
(`internal/template/resolver.go`, `enterpriseOnlyNamespaceKeysCE`), so CE cannot
implement it at all. The field was removed in #344 rather than left advertised
and inert.

---

### P1: Advanced Storage Features ✅ Complete

> **Implementation complete**: 2026-02-23 | PR: [#10](https://github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/pull/10) (`feature/p1-storage-advanced`)

#### 7. Global Volume Policy (filesystemVolumePolicy / blockVolumePolicy) ✅
> AKO docs: https://aerospike.com/docs/kubernetes/manage/storage/overview/

```yaml
spec:
  storage:
    filesystemVolumePolicy:
      initMethod: deleteFiles
      wipeMethod: deleteFiles
      cascadeDelete: true
    blockVolumePolicy:
      initMethod: blkdiscard
      wipeMethod: blkdiscardWithHeaderCleanup
```

- [x] Add `FilesystemVolumePolicy` to `AerospikeStorageSpec`
- [x] Add `BlockVolumePolicy` to `AerospikeStorageSpec`
- [x] Policy resolution logic: per-volume > global > default precedence (`internal/storage/policy.go`)
- [x] `CascadeDelete` as `*bool` type so per-volume explicit false can override global policy

**Implementation files**:
- `api/v1alpha1/types_storage.go` — `AerospikeVolumePolicy` type, `FilesystemVolumePolicy`, `BlockVolumePolicy` fields
- `internal/storage/policy.go` — `ResolveInitMethod`, `ResolveWipeMethod`, `ResolveCascadeDelete`
- `internal/storage/policy_test.go` — policy resolution tests (274 lines)

#### 8. WipeMethod (separate from InitMethod) ✅
> AKO docs: https://aerospike.com/docs/kubernetes/manage/storage/persistent-volume/

```yaml
storage:
  blockVolumePolicy:
    initMethod: blkdiscard
    wipeMethod: blkdiscardWithHeaderCleanup
  volumes:
    - name: data
      wipeMethod: headerCleanup  # per-volume override
```

- [x] Add `WipeMethod` field to `VolumeSpec` (6 options: none, deleteFiles, dd, blkdiscard, headerCleanup, blkdiscardWithHeaderCleanup)
- [x] Wipe vs init branching logic in init container (`WIPE_VOLUMES` env → runs before INIT)
- [x] Add `blkdiscardWithHeaderCleanup` method

**Implementation files**:
- `api/v1alpha1/types_storage.go` — `VolumeWipeMethod` type
- `internal/initcontainer/scripts/aerospike-init.sh` — `process_volumes()` helper, WIPE→INIT order
- `internal/podutil/container.go` — `buildWipeVolumesEnv()`

#### 9. HostPath Volume Source ✅
> AKO CRD Reference

```yaml
volumes:
  - name: host-logs
    source:
      hostPath:
        path: /var/log/aerospike
        type: DirectoryOrCreate
```

- [x] Add `HostPath` field to `VolumeSource` (`*corev1.HostPathVolumeSource`)
- [x] Handle hostPath volumes in StatefulSet builder
- [x] Webhook warning: alert when hostPath is used in production environments

**Implementation files**:
- `api/v1alpha1/types_storage.go` — `VolumeSource.HostPath`
- `internal/storage/volume.go` — `volumeForSpec()` hostPath case
- `api/v1alpha1/aerospikececluster_webhook.go` — `validateStorage()` hostPath warning

#### 10. Local Storage Awareness ✅
> AKO docs: https://aerospike.com/docs/kubernetes/manage/storage/local-volume/

```yaml
spec:
  storage:
    localStorageClasses:
      - local-path
      - openebs-hostpath
    deleteLocalStorageOnRestart: true
```

- [x] Add `LocalStorageClasses` field to `AerospikeStorageSpec`
- [x] Add `DeleteLocalStorageOnRestart` field to `AerospikeStorageSpec`
- [x] Local PVC deletion logic on pod cold restart (with NotFound error guard)
- [x] Webhook: error if `deleteLocalStorageOnRestart: true` and `localStorageClasses` is empty

**Implementation files**:
- `internal/storage/local.go` — `DeleteLocalPVCsForPod`, `GetLocalPVCsForPod`, `ParsePodName`
- `internal/controller/reconciler_restart.go` — call local PVC deletion from `coldRestartPod`
- `internal/storage/local_test.go` — tests (77 lines)

#### 11. Advanced Volume Mount Options ✅
> AKO CRD Reference

```yaml
volumes:
  - name: data
    aerospike:
      path: /opt/aerospike/data
      readOnly: false
      subPath: "subdir"
      subPathExpr: "$(POD_NAME)"  # mutually exclusive with subPath
      mountPropagation: HostToContainer
```

- [x] Add `ReadOnly`, `SubPath`, `SubPathExpr`, `MountPropagation` to `AerospikeVolumeAttachment`
- [x] Add the same fields to `VolumeAttachment` (sidecar/init containers)
- [x] Apply mount options in StatefulSet builder (`buildVolumeMount()` helper)
- [x] Webhook: validate `SubPath` and `SubPathExpr` are mutually exclusive

**Implementation files**:
- `api/v1alpha1/types_storage.go` — `AerospikeVolumeAttachment`, `VolumeAttachment` extensions
- `internal/storage/volume.go` — `buildVolumeMount()`

#### 12. PVC Metadata (Annotations/Labels) ✅
> AKO CRD Reference

```yaml
volumes:
  - name: data
    source:
      persistentVolume:
        size: 50Gi
        metadata:
          labels:
            backup-policy: "daily"
          annotations:
            volume.kubernetes.io/storage-provisioner: "ebs.csi.aws.com"
```

- [x] Add `Metadata` (`AerospikeObjectMeta`) field to `PersistentVolumeSpec`
- [x] Apply custom annotations/labels to PVC template (`maps.Clone`)

**Implementation files**:
- `api/v1alpha1/types_storage.go` — `PersistentVolumeSpec.Metadata`
- `internal/storage/volume.go` — `BuildVolumeClaimTemplates()` metadata application

---

### P2: Observability Enhancements

#### 13. Operator Self-Monitoring Enhancements
> AKO docs: https://aerospike.com/docs/kubernetes/observe/operator-monitoring/

The operator has internal metrics, but richer monitoring support is needed.

- [ ] Add dedicated `aerospike_acko_aerospikececluster_phase` Prometheus metric
- [ ] TLS support for operator /metrics endpoint (cert-manager integration)
- [ ] Generate operator-specific ServiceMonitor YAML (config/monitoring/)

#### 14. Grafana Dashboard Templates
> AKO docs: https://aerospike.com/docs/kubernetes/observe/clusters/

- [ ] Operator dashboard JSON template (controller-runtime, workqueue metrics)
- [ ] Aerospike cluster dashboard JSON template (APE exporter metrics)
- [ ] Place in config/monitoring/grafana/ directory

#### 15. Default Prometheus Alert Rules
> AKO docs: https://aerospike.com/docs/kubernetes/observe/clusters/

```yaml
# config/monitoring/prometheus/alert-rules.yaml
groups:
  - name: aerospike-alerts
    rules:
      - alert: AerospikeHighMemoryUtilization
        expr: aerospike_node_stats_system_free_mem_pct < 5
        for: 5m
        severity: critical
      - alert: AerospikeHighDiskUtilization
        expr: aerospike_namespace_device_available_pct < 5
        for: 5m
        severity: warning
```

- [ ] Write PrometheusRule CRD YAML template
- [ ] Alert rules for memory, disk, and namespace utilization
- [ ] Place in config/monitoring/prometheus/ directory

#### 16. Logging Configuration Guide/Validation
> AKO docs: https://aerospike.com/docs/kubernetes/observe/logs/

```yaml
aerospikeConfig:
  logging:
    - name: console
      any: info
      tls: debug
    - name: /var/log/aerospike/aerospike.log
      any: info
```

- [ ] Webhook validation for logging configuration (validate sink types, log levels)
- [ ] Volume mount validation for log file paths (check that PV is mounted)
- [ ] Add logging configuration example to sample CR

---

### P3: Security Enhancements

#### 17. Advanced ACL Features (ReadQuota/WriteQuota)
> AKO docs: https://aerospike.com/docs/kubernetes/manage/security/access-control/

```yaml
aerospikeAccessControl:
  roles:
    - name: rate-limited-reader
      privileges:
        - read.my-namespace
      readQuota: 10000     # NEW: max reads per second
      writeQuota: 5000     # NEW: max writes per second
```

**Note**: readQuota/writeQuota are Enterprise Edition-only features. For CE, only define the CRD fields and have the webhook warn/block when configured on CE.

- [ ] Add `ReadQuota`, `WriteQuota` fields to `AerospikeRoleSpec`
- [ ] Webhook: warning message when quota is configured with CE image
- [ ] (Optional) ACL reconciler quota configuration logic when EE image support is added

#### 18. Non-Root Cluster Execution Guide
> AKO docs: https://aerospike.com/docs/kubernetes/manage/security/nonroot-cluster/

- [ ] Add non-root execution example to sample CR
  ```yaml
  podSpec:
    securityContext:
      runAsNonRoot: true
      runAsUser: 1000
      fsGroup: 1000
  ```
- [ ] Webhook validation for required volume permissions when runAsNonRoot is set

---

### P4: Advanced Networking Features

#### 19. CustomInterface Network Type (Multus CNI)
> AKO docs: https://aerospike.com/docs/kubernetes/manage/configure/network-policy/

```yaml
spec:
  aerospikeNetworkPolicy:
    accessType: customInterface
    customAccessNetworkNames:
      - default/my-network-attachment
    fabricType: customInterface
    customFabricNetworkNames:
      - default/my-fabric-network
```

- [ ] Add `customInterface` to NetworkType
- [ ] Add additional fields: `CustomFabricNetworkNames`, `CustomTLSAccessNetworkNames`, etc.
- [ ] Inject `k8s.v1.cni.cncf.io/networks` annotation into Pod
- [ ] Custom interface IP resolution logic in config generation

#### 20. Per-Pod Service Creation ✅ (Implemented in P0)
> AKO CRD Reference

AKO creates an individual Service for each pod, enabling direct external access to a specific pod.

- [x] Per-pod Service reconciler based on `spec.podService` configuration (`reconciler_pod_service.go`)
- [x] Create ClusterIP Service named `<pod-name>-pod` for each pod
- [ ] NodePort / LoadBalancer type support (currently ClusterIP only)
- [x] Apply custom annotations/labels

---

### P5: Operational Convenience Features

#### 21. HPA (Horizontal Pod Autoscaler) Support
> AKO docs: https://aerospike.com/docs/kubernetes/manage/configure/hpa/

- [ ] Expose `spec.selector` (label selector string) in Status in proper HPA-compatible format
- [ ] Verify scale subresource registration (kubebuilder marker)
- [ ] KEDA + Prometheus-based autoscaling example documentation
- [ ] `minReplicaCount >= replication-factor` validation guide

#### 22. AerospikeInitContainer Customization
> AKO CRD Reference

AKO allows separate customization of the init container image.

```yaml
podSpec:
  aerospikeInitContainerSpec:
    imageRegistry: "my-registry.com"
    imageRegistryNamespace: "aerospike"
    imageNameAndTag: "aerospike-kubernetes-init:2.2.1"
    securityContext: ...
    resources: ...
```

- [ ] Define `AerospikeInitContainerSpec` type
- [ ] Support init container image customization
- [ ] Configure init container resources/securityContext

#### 23. Multi-Namespace Operator Watch
> AKO docs: https://aerospike.com/docs/kubernetes/manage/configure/multiple-aerospike-clusters/

- [ ] Multi-namespace watch support based on `WATCH_NAMESPACE` environment variable
- [ ] Per-namespace RBAC configuration guide/scripts
- [ ] Manage multiple namespace AerospikeCECluster resources simultaneously

---

### P6: Enterprise Edition Compatibility (Future EE Support)

> The features below are Enterprise Edition-only. For the CE operator, define CRD fields only and either block them in the webhook or activate them only when an EE image is detected.

#### 24. TLS Support
- [ ] Define `spec.operatorClientCertSpec` type
- [ ] Add TLS access type fields to network config (tlsAccessType, tlsAlternateAccessType, tlsFabricType)
- [ ] Auto-mount TLS cert volume
- [ ] Webhook: block TLS configuration when using CE image

#### 25. LDAP External Authentication
- [ ] Support `aerospikeConfig.security.ldap` configuration
- [ ] Mount LDAP-related Secrets (query-user-password-file)
- [ ] Webhook: block LDAP configuration when using CE image

#### 26. XDR (Cross-Datacenter Replication)
- [ ] Support `aerospikeConfig.xdr` configuration
- [ ] XDR Proxy integration guide
- [ ] Webhook: block XDR configuration when using CE image (already implemented)

#### 27. Strong Consistency
- [ ] Add `spec.rosterNodeBlockList` field
- [ ] Add `Rack.forceBlockFromRoster` field
- [ ] Roster management reconciler
- [ ] Webhook: block strong-consistency configuration when using CE image (already implemented)

---

## Implementation Priority Summary

| Priority | Category | Items | Status | Key Rationale |
|---------|---------|--------|------|----------|
| **P0** | Configure Cluster | 6 | ✅ Complete (PR #7) | Core AKO features, operationally essential |
| **P1** | Advanced Storage | 6 | ✅ Complete (PR #10) | Production storage management |
| **P2** | Observability | 4 | Not started | Monitoring/alerting enhancements |
| **P3** | Security | 2 | Not started | Security hardening |
| **P4** | Advanced Networking | 2 | 1/2 complete | Advanced networking |
| **P5** | Operational Convenience | 3 | Not started | Operational usability |
| **P6** | EE Compatibility | 4 | Not started | Future EE support preparation |

---

## AKO Official Documentation Reference by Feature

| Feature | AKO Documentation URL |
|------|-------------|
| Cluster Configuration | https://aerospike.com/docs/kubernetes/manage/configure/overview/ |
| Aerospike Config | https://aerospike.com/docs/kubernetes/manage/configure/aerospike-cluster/ |
| Rack Awareness | https://aerospike.com/docs/kubernetes/manage/configure/rack-awareness/ |
| Network Policy | https://aerospike.com/docs/kubernetes/manage/configure/network-policy/ |
| Pod Spec | https://aerospike.com/docs/kubernetes/manage/configure/pod-spec/ |
| Multiple Clusters | https://aerospike.com/docs/kubernetes/manage/configure/multiple-aerospike-clusters/ |
| Dynamic Config | https://aerospike.com/docs/kubernetes/manage/configure/dynamic-config/ |
| HPA | https://aerospike.com/docs/kubernetes/manage/configure/hpa/ |
| Storage Overview | https://aerospike.com/docs/kubernetes/manage/storage/overview/ |
| Persistent Volume | https://aerospike.com/docs/kubernetes/manage/storage/persistent-volume/ |
| Local Volume | https://aerospike.com/docs/kubernetes/manage/storage/local-volume/ |
| K8s Secrets | https://aerospike.com/docs/kubernetes/manage/security/kubernetes-secrets/ |
| Access Control | https://aerospike.com/docs/kubernetes/manage/security/access-control/ |
| Non-Root | https://aerospike.com/docs/kubernetes/manage/security/nonroot-cluster/ |
| Node Maintenance | https://aerospike.com/docs/kubernetes/manage/node-maintenance/ |
| Monitor Clusters | https://aerospike.com/docs/kubernetes/observe/clusters/ |
| Monitor Operator | https://aerospike.com/docs/kubernetes/observe/operator-monitoring/ |
| Logging | https://aerospike.com/docs/kubernetes/observe/logs/ |
| Config Reference | https://aerospike.com/docs/kubernetes/reference/config-reference/ |

---

## Reference: AKO vs CE Operator Architecture Comparison

| Item | AKO (Enterprise) | CE Operator (this project) |
|------|-------------------|--------------------------|
| CRD Group | `asdb.aerospike.com/v1` | `acko.io/v1alpha1` |
| Kind | `AerospikeCluster` | `AerospikeCECluster` |
| Size limit | Unlimited | 8 nodes |
| Namespace limit | Unlimited | 2 |
| TLS | Supported | Not supported (CE limitation) |
| XDR | Supported | Not supported (CE limitation) |
| LDAP | Supported | Not supported (CE limitation) |
| Strong Consistency | Supported | Not supported (CE limitation) |
| Heartbeat Mode | mesh + multicast | mesh only (CE limitation) |
| Init Container | Custom image | Built-in |
| Volume Sources | PV, EmptyDir, Secret, ConfigMap, HostPath | PV, EmptyDir, Secret, ConfigMap, HostPath ✅ |
| Volume Policy | Global + Per-volume + Wipe separated | Global + Per-volume + Wipe separated ✅ |
| Operations | WarmRestart, PodRestart (on-demand) | WarmRestart, PodRestart (on-demand) ✅ |
| Rack placement | rollingUpdateBatchSize + scaleDownBatchSize + maxIgnorablePods | rollingUpdateBatchSize + scaleDownBatchSize + maxIgnorablePods ✅ |
| ValidationPolicy | skipWorkDirValidate | skipWorkDirValidate ✅ |
| Service customization | headlessService + podService metadata | headlessService + podService metadata ✅ |
| Additional Rack fields | rackLabel, revision | rackLabel, revision ✅ |
