---
sidebar_position: 2
title: Networking
---

# Networking

Configure how Aerospike nodes advertise addresses, restrict traffic, shape bandwidth, expose external seeds, and label operator-managed services.

## AerospikeNetworkPolicy

`spec.aerospikeNetworkPolicy` controls which addresses Aerospike advertises to clients and peer nodes. Set it explicitly when clients connect from outside the Kubernetes cluster.

### Access Types

Each field accepts one of these four values:

| Value | Description |
|---|---|
| `pod` | Advertise the Pod IP (default) to in-cluster clients. |
| `hostInternal` | Advertise the Kubernetes node's internal IP to clients on the same private network. |
| `hostExternal` | Advertise the Kubernetes node's external IP to clients outside the cloud VPC. |
| `configuredIP` | Advertise a custom IP from pod annotations for multi-network setups such as Multus. |

### Fields

| Field | Default | Description |
|---|---|---|
| `accessType` | `pod` | How clients reach the Aerospike service port (3000). |
| `alternateAccessType` | `pod` | How clients from alternate networks reach the service port. |
| `fabricType` | `pod` | How Aerospike nodes communicate with each other (fabric/heartbeat). |

### Example: In-Cluster Only (Default)

```yaml
apiVersion: acko.io/v1alpha1
kind: AerospikeCluster
metadata:
  name: my-cluster
  namespace: aerospike
spec:
  size: 3
  image: aerospike:ce-8.1.1.1
  aerospikeNetworkPolicy:
    accessType: pod
    fabricType: pod
```

### Example: External Client Access via Host Network

When clients connect from outside the cluster, advertise the node's external IP:

```yaml
spec:
  aerospikeNetworkPolicy:
    accessType: hostExternal
    alternateAccessType: hostInternal
    fabricType: pod
```

In this setup:
- External clients connect using the node's external IP.
- Internal clients (alternate network) use the node's internal IP.
- Inter-node fabric traffic stays on the pod network for performance.

### Example: ConfiguredIP with Custom Network Names

For environments using secondary network interfaces (e.g., Multus CNI), use `configuredIP` with custom network names. The operator reads the IP from the pod's `k8s.v1.cni.cncf.io/network-status` annotation matching the specified network name.

```yaml
spec:
  aerospikeNetworkPolicy:
    accessType: configuredIP
    customAccessNetworkNames:
      - "aerospike-sriov-network"
    alternateAccessType: configuredIP
    customAlternateAccessNetworkNames:
      - "aerospike-macvlan-network"
    fabricType: configuredIP
    customFabricNetworkNames:
      - "aerospike-sriov-network"
```

:::warning
When using `configuredIP`, you must provide the corresponding `custom*NetworkNames` field. The operator will fail to resolve the IP if the network name does not match an entry in the pod's network-status annotation.
:::

## NetworkPolicyConfig

`spec.networkPolicyConfig` enables automatic creation of Kubernetes NetworkPolicy or Cilium CiliumNetworkPolicy resources. This restricts network traffic to only what the Aerospike cluster needs.

### Generated Rules

When enabled, the operator creates a NetworkPolicy with these ingress rules:

1. **Intra-cluster traffic**: Fabric (3001) and heartbeat (3002) ports are allowed only from pods matching the cluster's selector labels.
2. **Client access**: Service port (3000) is open to all sources.
3. **Metrics** (if monitoring is enabled): The configured metrics port is open to all sources for Prometheus scraping.

### Standard Kubernetes NetworkPolicy

```yaml
spec:
  networkPolicyConfig:
    enabled: true
    type: kubernetes
```

### Cilium CiliumNetworkPolicy

If your cluster uses Cilium as the CNI, you can generate a `CiliumNetworkPolicy` instead:

```yaml
spec:
  networkPolicyConfig:
    enabled: true
    type: cilium
```

:::info
If the CiliumNetworkPolicy CRD is not installed in the cluster, the operator logs a message and skips creation gracefully. No error is raised.
:::

### Disabling

Set `enabled: false` (or remove the field entirely) to delete any previously created NetworkPolicy:

```yaml
spec:
  networkPolicyConfig:
    enabled: false
```

## BandwidthConfig

`spec.bandwidthConfig` injects CNI bandwidth annotations on pod templates for traffic shaping. This is useful for limiting network throughput per pod to prevent noisy-neighbor issues in shared clusters.

The operator sets the standard `kubernetes.io/ingress-bandwidth` and `kubernetes.io/egress-bandwidth` annotations, which are recognized by CNI plugins such as the Cilium bandwidth manager and the bandwidth CNI plugin.

### Example

```yaml
spec:
  bandwidthConfig:
    ingress: "1Gbps"
    egress: "500Mbps"
```

Both fields are optional. You can set only ingress, only egress, or both:

```yaml
spec:
  bandwidthConfig:
    egress: "200Mbps"
```

:::note
Bandwidth shaping requires a CNI plugin that supports these annotations. If your CNI does not support them, the annotations are ignored.
:::

## SeedsFinderServices

`spec.seedsFinderServices` creates a LoadBalancer service for external seed discovery. This allows clients outside the Kubernetes cluster to discover Aerospike seed nodes via the LoadBalancer's external IP.

### LoadBalancer Configuration

| Field | Default | Description |
|---|---|---|
| `annotations` | — | Custom annotations on the LoadBalancer service (e.g., for cloud provider configuration). |
| `labels` | — | Custom labels on the LoadBalancer service. |
| `externalTrafficPolicy` | — | `Cluster` or `Local`. Use `Local` to preserve client source IP. |
| `port` | `3000` | External port on the LoadBalancer. |
| `targetPort` | `3000` | Container port to forward traffic to. |
| `loadBalancerSourceRanges` | — | Restrict traffic to specific CIDRs for security. |

### Example: Basic LoadBalancer

```yaml
spec:
  seedsFinderServices:
    loadBalancer:
      port: 3000
```

### Example: Production LoadBalancer with Restrictions

```yaml
spec:
  seedsFinderServices:
    loadBalancer:
      port: 3000
      targetPort: 3000
      externalTrafficPolicy: Local
      loadBalancerSourceRanges:
        - "10.0.0.0/8"
        - "172.16.0.0/12"
      annotations:
        service.beta.kubernetes.io/aws-load-balancer-type: "nlb"
        service.beta.kubernetes.io/aws-load-balancer-scheme: "internal"
      labels:
        environment: production
```

## HeadlessService

Use `spec.headlessService` to add custom annotations and labels to the headless service for a cluster. The operator always creates this service as `<cluster-name>-headless` for DNS-based pod discovery.

### Example

```yaml
spec:
  headlessService:
    metadata:
      annotations:
        prometheus.io/scrape: "true"
        external-dns.alpha.kubernetes.io/hostname: "aerospike.example.com"
      labels:
        team: platform
```

:::note
Operator-managed labels (e.g., `app.kubernetes.io/name`, `app.kubernetes.io/instance`) cannot be overridden by custom labels. If you specify a label key that conflicts with an operator-managed key, the operator's value takes precedence.
:::

## PodService

`spec.podService` creates an individual Service for each Aerospike pod. This is useful when you need stable, per-pod DNS names, service mesh integration, or **external client access via LoadBalancer/NodePort**.

When configured, the operator creates a Service named `<pod-name>-pod` for each pod, selecting that specific pod via `statefulset.kubernetes.io/pod-name`.

### Service Type

| Value | Description |
|---|---|
| `ClusterIP` | Default. Internal access only. Exposes only the service port (3000). |
| `NodePort` | Exposes the pod on each node's IP at a random port. All Aerospike ports (service, fabric, heartbeat) are exposed. |
| `LoadBalancer` | Creates a cloud LoadBalancer with a unique external IP per pod. All Aerospike ports are exposed. |

When `serviceType` is `LoadBalancer` or `NodePort`, the operator automatically:
1. Creates a Role/RoleBinding granting the pod's service account permission to read its own Service
2. Enables `automountServiceAccountToken` on the pod spec
3. Injects `EXTERNAL_SERVICE_TYPE` and `POD_NAMESPACE` environment variables into the init container
4. The init container queries the Kubernetes API at startup to discover its LoadBalancer external IP or NodePort, and injects it as `alternate-access-address` into `aerospike.conf`

### Example: ClusterIP (Default)

```yaml
spec:
  podService:
    metadata:
      labels:
        visibility: internal
```

### Example: LoadBalancer for External Access

```yaml
spec:
  podService:
    serviceType: LoadBalancer
    metadata:
      annotations:
        service.beta.kubernetes.io/aws-load-balancer-type: "nlb"
```

### Example: NodePort for External Access

```yaml
spec:
  podService:
    serviceType: NodePort
```

The operator automatically cleans up stale pod services after scale-down or when `podService` is removed from the spec.

## External Client Access

To allow clients outside the Kubernetes cluster to connect to Aerospike, configure per-pod LoadBalancer services and a seeds finder LoadBalancer.

### How It Works

Aerospike uses a **smart client** protocol: the client connects to a seed node, receives the full cluster topology (all node addresses), and then connects directly to each node. For external access, every node must advertise an externally reachable address.

```
1. Client connects to Seeds LB (single entry point)
2. Aerospike returns cluster topology with alternate-access-address (LB IPs)
3. Client connects directly to each node via its per-pod LB IP
```

### Configuration

```yaml
spec:
  # Per-pod LoadBalancer services — each pod gets a unique external IP
  podService:
    serviceType: LoadBalancer

  # Seeds finder LB — single entry point for initial client connection
  seedsFinderServices:
    loadBalancer:
      port: 3000
```

### Client Connection

Use the Seeds LB IP with the `--services-alternate` flag:

```bash
# aql
aql -h <seeds-lb-ip> -p 3000 --services-alternate

# asinfo
asinfo -h <seeds-lb-ip> -p 3000
```

```python
# Python
client = aerospike.client({
    "hosts": [("<seeds-lb-ip>", 3000)],
    "policies": {"use_services_alternate": True},
}).connect()
```

### Discovering Endpoints

```bash
# Seeds LB endpoint (shown in default output)
kubectl get asc
# NAME           RACKSIZE   HEALTH   PHASE       SEED
# my-aerospike   3          3/3      Completed   10.177.200.247:3000

# All per-pod endpoints (shown in wide output)
kubectl get asc -o wide
# ENDPOINTS: 10.177.200.244:3000,10.177.200.245:3000,10.177.200.246:3000
```

:::warning
The init container queries the Kubernetes API to resolve LoadBalancer IPs. If the LoadBalancer IP is not assigned within 120 seconds, the init container will fail and the pod will enter CrashLoopBackOff. Ensure your cloud provider or LB controller assigns IPs promptly.
:::

## Full Networking Example

Here is a comprehensive example combining multiple networking features:

```yaml
apiVersion: acko.io/v1alpha1
kind: AerospikeCluster
metadata:
  name: production-cluster
  namespace: aerospike
spec:
  size: 4
  image: aerospike:ce-8.1.1.1

  aerospikeNetworkPolicy:
    accessType: hostExternal
    alternateAccessType: hostInternal
    fabricType: pod

  networkPolicyConfig:
    enabled: true
    type: kubernetes

  bandwidthConfig:
    ingress: "2Gbps"
    egress: "1Gbps"

  seedsFinderServices:
    loadBalancer:
      port: 3000
      externalTrafficPolicy: Local
      loadBalancerSourceRanges:
        - "10.0.0.0/8"

  headlessService:
    metadata:
      annotations:
        external-dns.alpha.kubernetes.io/hostname: "aerospike-headless.example.com"

  podService:
    serviceType: LoadBalancer
    metadata:
      labels:
        mesh.istio.io/managed: "true"

  aerospikeConfig:
    service:
      proto-fd-max: 15000
    namespaces:
      - name: test
        replication-factor: 2
        storage-engine:
          type: memory
          data-size: 1073741824
    network:
      service:
        port: 3000
      fabric:
        port: 3001
      heartbeat:
        mode: mesh
        port: 3002
        interval: 150
        timeout: 10
```
