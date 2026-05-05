# Documentation TODO

## Multi-Cluster Operation Guide

**Priority:** High
**Location:** `content/getting-started/` or `content/operations/`

### Background

Production environments often separate the **management cluster** (where the operator and Cluster Manager UI run) from the **workload clusters** (where Aerospike pods actually run). This pattern provides:

- Isolation between control plane and data plane
- Independent scaling and upgrade cycles
- Central management of multiple Aerospike clusters across different K8s clusters

### Guide Should Cover

1. **Architecture Overview**
   - Management cluster: ACKO operator + Cluster Manager UI
   - Workload cluster(s): Aerospike CE pods only
   - Network connectivity requirements between clusters

2. **Management Cluster Setup**
   - Install ACKO operator (UI is on by default; toggle via `ui.api.enabled` / `ui.web.enabled`)
   - Configure RBAC for cross-cluster access
   - kubeconfig / ServiceAccount token management for remote clusters

3. **Workload Cluster Setup**
   - CRD installation (CRDs must exist on workload clusters)
   - Network requirements (operator must reach workload cluster API server)
   - Firewall / VPN considerations

4. **Cluster Manager UI Configuration**
   - Registering remote clusters in the UI
   - `ui.k8s.verifySsl=false` for clusters with self-signed certs
   - `ui.hostNetwork` / `ui.dnsConfig` for network access to remote Aerospike nodes

5. **Day-2 Operations**
   - Rolling upgrades across clusters
   - Monitoring setup (Prometheus federation or remote-write)
   - Troubleshooting cross-cluster connectivity
