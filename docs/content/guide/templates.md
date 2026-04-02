---
sidebar_position: 9
title: Template Management
---

# Template Management

The `AerospikeClusterTemplate` CRD enables reusable, organization-wide configuration profiles for Aerospike clusters. This guide covers the full template lifecycle: creation, usage, overrides, sync behavior, and best practices.

For the CRD field reference, see [AerospikeClusterTemplate API Reference](../api-reference/aerospikeclustertemplate.md).

---

## How Templates Work

Templates follow a **snapshot model**:

1. You create an `AerospikeClusterTemplate` with shared defaults (image, size, resources, scheduling, storage, monitoring, network policy, Aerospike config).
2. An `AerospikeCluster` references the template via `spec.templateRef.name`.
3. At cluster creation, the operator resolves the template and stores the full spec in `status.templateSnapshot`.
4. The cluster operates independently from that point. Template changes are **not** automatically propagated.

```
┌─────────────────────┐       templateRef        ┌─────────────────────┐
│  AerospikeCluster   │ ─────────────────────►   │ AerospikeCluster    │
│  (spec.templateRef) │                          │ Template            │
└─────────────────────┘                          └─────────────────────┘
         │                                                │
         ▼                                                │
  status.templateSnapshot                                 │
  (frozen copy of template spec)                          │
         │                                                │
         │   acko.io/resync-template=true                 │
         └────────────────────────────────────────────────┘
                     (manual resync)
```

:::info
`AerospikeClusterTemplate` is a **cluster-scoped** resource. It does not belong to any namespace. Reference it by name only.
:::

### Cross-Namespace Template Resolution

Because `AerospikeClusterTemplate` is cluster-scoped, any `AerospikeCluster` in any namespace can reference the same template by name. There is no need for namespace qualification or cross-namespace RBAC grants -- the operator resolves templates at the cluster scope regardless of where the referencing `AerospikeCluster` CR lives.

```yaml
# Namespace: team-alpha
apiVersion: acko.io/v1alpha1
kind: AerospikeCluster
metadata:
  name: alpha-cluster
  namespace: team-alpha
spec:
  templateRef:
    name: hard-rack          # References the cluster-scoped template
  aerospikeConfig:
    namespaces:
      - name: alpha-data
        storage-engine:
          type: memory
---
# Namespace: team-beta (same template, different namespace)
apiVersion: acko.io/v1alpha1
kind: AerospikeCluster
metadata:
  name: beta-cluster
  namespace: team-beta
spec:
  templateRef:
    name: hard-rack          # Same template as team-alpha
  aerospikeConfig:
    namespaces:
      - name: beta-data
        storage-engine:
          type: memory
```

Both clusters share the same template defaults (image, size, resources, scheduling, monitoring, etc.) while maintaining independent configurations and lifecycle. Template sync status (`status.templateSnapshot.synced`) is tracked per cluster, so resyncing one cluster does not affect the other.

This design enables platform teams to define organization-wide templates once and let application teams in different namespaces consume them without coordination overhead.

---

## Creating Templates

### Minimal Template (Development)

A lightweight template for development and quick prototyping:

```yaml
apiVersion: acko.io/v1alpha1
kind: AerospikeClusterTemplate
metadata:
  name: minimal
spec:
  description: "Development template - single node, minimal resources"
  image: aerospike:ce-8.1.1.1
  size: 1

  resources:
    requests:
      cpu: 100m
      memory: 256Mi
    limits:
      cpu: 100m
      memory: 256Mi

  scheduling:
    podAntiAffinityLevel: none

  storage:
    storageClassName: standard
    resources:
      requests:
        storage: 1Gi

  aerospikeConfig:
    namespaceDefaults:
      replication-factor: 1
      data-size: 1073741824   # 1 GiB
```

### Soft-Rack Template (Staging)

Provides rack awareness with soft anti-affinity, suitable for staging environments:

```yaml
apiVersion: acko.io/v1alpha1
kind: AerospikeClusterTemplate
metadata:
  name: soft-rack
spec:
  description: "Staging template - soft anti-affinity, 3 nodes"
  image: aerospike:ce-8.1.1.1
  size: 3

  resources:
    requests:
      cpu: 500m
      memory: 1Gi
    limits:
      cpu: 500m
      memory: 1Gi

  scheduling:
    podAntiAffinityLevel: preferred

  storage:
    storageClassName: standard
    resources:
      requests:
        storage: 10Gi

  aerospikeConfig:
    namespaceDefaults:
      replication-factor: 2
      data-size: 2147483648   # 2 GiB
```

### Hard-Rack Template (Production)

Enforces strict anti-affinity (one pod per node), local storage, and monitoring:

```yaml
apiVersion: acko.io/v1alpha1
kind: AerospikeClusterTemplate
metadata:
  name: hard-rack
spec:
  description: "Production template - hard anti-affinity, local PVs, monitoring"
  image: aerospike:ce-8.1.1.1
  size: 6

  monitoring:
    enabled: true
    port: 9145
    resources:
      requests:
        cpu: 50m
        memory: 64Mi
      limits:
        cpu: 200m
        memory: 128Mi
    serviceMonitor:
      enabled: true
      interval: 30s

  aerospikeNetworkPolicy:
    accessType: pod
    alternateAccessType: pod
    fabricType: pod

  scheduling:
    podAntiAffinityLevel: required
    tolerations:
      - key: "aerospike"
        operator: "Exists"
        effect: "NoSchedule"

  storage:
    storageClassName: local-path
    localPVRequired: true
    resources:
      requests:
        storage: 100Gi

  resources:
    requests:
      cpu: "2"
      memory: "4Gi"
    limits:
      cpu: "2"
      memory: "4Gi"

  aerospikeConfig:
    service:
      proto-fd-max: 15000
    namespaceDefaults:
      replication-factor: 2
      data-size: 2147483648   # 2 GiB
```

Apply templates:

```bash
kubectl apply -f config/samples/acko_v1alpha1_template_dev.yaml     # minimal
kubectl apply -f config/samples/acko_v1alpha1_template_stage.yaml   # soft-rack
kubectl apply -f config/samples/acko_v1alpha1_template_prod.yaml    # hard-rack
```

---

## Using Templates in Clusters

### Basic Reference

Reference a template by name. When the template supplies `image` and `size`, the cluster can omit those fields:

```yaml
apiVersion: acko.io/v1alpha1
kind: AerospikeCluster
metadata:
  name: my-cluster
spec:
  templateRef:
    name: soft-rack
  aerospikeConfig:
    namespaces:
      - name: data
        storage-engine:
          type: memory
```

### Overriding Template Defaults

Set `spec.image` or `spec.size` explicitly on the cluster to override the template value:

```yaml
spec:
  image: aerospike:ce-8.1.1.1   # override: pin a specific image
  size: 3                         # override: use 3 instead of template's default
  templateRef:
    name: hard-rack
```

---

## Template Overrides

Use `spec.overrides` to selectively modify template fields without creating a new template:

```yaml
spec:
  templateRef:
    name: hard-rack
  overrides:
    resources:
      requests:
        cpu: "4"
        memory: "8Gi"
      limits:
        cpu: "4"
        memory: "8Gi"
    scheduling:
      podAntiAffinityLevel: preferred   # relax anti-affinity for this cluster
```

### Merge Priority

Fields are resolved in this order (highest priority first):

1. **`spec.overrides`** -- cluster-specific overrides
2. **`template.spec`** -- template defaults
3. **Operator defaults** -- built-in fallback values

### Merge Behavior

| Field Type | Behavior | Example |
|------------|----------|---------|
| Scalar (string, int, bool) | Override replaces template value | `size: 3` overrides template's `size: 6` |
| Map (nested object) | Recursive merge -- only specified keys are overridden | `aerospikeConfig.service.proto-fd-max: 20000` overrides just that key |
| Array (list) | Override replaces entire array | `tolerations: [...]` replaces all template tolerations |

:::tip
For maps like `aerospikeConfig.service`, you only need to specify the keys you want to change. Unspecified keys inherit from the template.
:::

---

## Overridable Fields

Templates can supply defaults for these fields:

| Field | Description |
|-------|-------------|
| `image` | Aerospike CE container image |
| `size` | Default cluster size (1--8) |
| `resources` | CPU/memory requests and limits |
| `scheduling` | Anti-affinity level, tolerations, node affinity, topology spread |
| `storage` | Storage class, volume mode, size, local PV flag |
| `rackConfig` | Rack configuration defaults (maxRacksPerNode) |
| `aerospikeConfig` | Service and namespace default settings |
| `monitoring` | Prometheus exporter sidecar configuration |
| `aerospikeNetworkPolicy` | Client/fabric network access types |

---

## Template Sync Behavior

### Why Templates Are Not Auto-Propagated

Templates use a snapshot model for safety. Automatically propagating template changes to running production clusters could cause unexpected rolling restarts or configuration conflicts. Instead, the operator:

1. Detects template drift and sets `status.templateSnapshot.synced: false`
2. Emits a `TemplateDrifted` warning event on affected clusters
3. Waits for explicit resync approval per cluster

### Checking Sync Status

```bash
kubectl get aerospikecluster hard-rack-cluster \
  -o jsonpath='{.status.templateSnapshot}'
```

Example output:

```json
{
  "name": "hard-rack",
  "resourceVersion": "12345",
  "snapshotTimestamp": "2026-03-01T10:00:00Z",
  "synced": true
}
```

When `synced` is `false`, the cluster is running on an older template version.

### Resyncing a Cluster

To apply the updated template to a specific cluster:

```bash
kubectl annotate aerospikecluster hard-rack-cluster acko.io/resync-template=true
```

The operator will:

1. Re-fetch the template
2. Update `status.templateSnapshot` with the new spec
3. Emit a `TemplateApplied` event
4. Remove the annotation
5. Trigger reconciliation (which may cause a rolling restart if the config changed)

:::warning
Resync may trigger a rolling restart if the template changes affect the Aerospike configuration, image, or pod spec. Review the template diff before resyncing production clusters.
:::

---

## Template Lifecycle

### Checking Which Clusters Use a Template

The `status.usedBy` field tracks all clusters referencing the template:

```bash
kubectl get aerospikeclustertemplate hard-rack -o jsonpath='{.status.usedBy}'
```

```json
["hard-rack-cluster", "prod-cluster-east", "prod-cluster-west"]
```

### Deleting a Template

A template cannot be deleted while `usedBy` is non-empty. Remove all cluster references first, then delete:

```bash
# Check current references
kubectl get aerospikeclustertemplate hard-rack -o jsonpath='{.status.usedBy}'

# Remove template reference from a cluster (set image/size explicitly)
kubectl -n aerospike patch asc hard-rack-cluster --type merge \
  -p '{"spec":{"image":"aerospike:ce-8.1.1.1","size":3,"templateRef":null}}'

# Delete the template once usedBy is empty
kubectl delete aerospikeclustertemplate hard-rack
```

### Updating a Template

Update a template like any Kubernetes resource:

```bash
kubectl patch aerospikeclustertemplate hard-rack --type merge \
  -p '{"spec":{"image":"aerospike:ce-8.1.2.0"}}'
```

After updating, all referencing clusters will show `synced: false` and emit `TemplateDrifted` events. Resync each cluster individually when ready.

---

## Installing Default Templates via Helm

Use the `defaultTemplates.enabled=true` Helm value to create all three template tiers during installation:

```bash
helm install aerospike-ce-kubernetes-operator \
  oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator \
  -n aerospike-operator --create-namespace \
  --set defaultTemplates.enabled=true
```

Verify:

```bash
kubectl get aerospikeclustertemplate
# NAME         AGE
# minimal      10s
# soft-rack    10s
# hard-rack    10s
```

---

## Managing Templates via Cluster Manager UI

When the Cluster Manager UI is enabled (`ui.enabled=true` and `ui.k8s.enabled=true`), templates can be managed from the web interface:

- **Create** -- guided wizard for all template fields
- **View** -- detail page showing resolved spec and referencing clusters
- **Edit** -- patch/update templates (requires RBAC `patch` and `update` permissions)
- **Delete** -- remove unused templates (blocked if any cluster still references it)
- **Resync** -- trigger template resync from the cluster detail page

See [Cluster Manager UI -- Template Management](cluster-manager-ui.md#template-management) for details.

---

## Best Practices

1. **Name templates by environment tier** -- `minimal`, `soft-rack`, `hard-rack` communicate the intended use clearly.
2. **Use `description`** -- the `spec.description` field (up to 500 characters) helps teams understand a template's purpose.
3. **Pin images in production templates** -- avoid `latest` or untagged images. Pin to a specific CE version.
4. **Set Guaranteed QoS in production** -- set resource requests equal to limits for predictable performance.
5. **Resync staging before production** -- after updating a template, resync staging clusters first to validate changes.
6. **Use overrides sparingly** -- if many clusters need the same override, consider creating a new template tier instead.

---

## Template Comparison

| | `minimal` | `soft-rack` | `hard-rack` |
|---|-----------|-------------|-------------|
| **Purpose** | Dev / quick-start | Staging | Production |
| **size** | 1 | 3 | 6 |
| **anti-affinity** | none | preferred | required |
| **storage** | standard 1 Gi | standard 10 Gi | local-path 100 Gi |
| **rack guarantee** | none | soft (same node OK) | hard (1 node : 1 rack) |
| **resources** | 100 m / 256 Mi | 500 m / 1 Gi | 2 / 4 Gi (Guaranteed QoS) |
| **monitoring** | disabled | disabled | enabled |
| **RF** | 1 | 2 | 2 |
