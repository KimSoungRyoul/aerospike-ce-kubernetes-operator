---
sidebar_position: 1
title: Installation
---

import Tabs from '@theme/Tabs';
import TabItem from '@theme/TabItem';

# Installation

Install ACKO from the Helm OCI chart or from a local chart checkout.

## Prerequisites

- Kubernetes cluster v1.26+
- `kubectl` configured for the target cluster
- [cert-manager](https://cert-manager.io/) for webhook TLS

### cert-manager

Install cert-manager before ACKO so the admission webhook can use TLS:

```bash
helm repo add jetstack https://charts.jetstack.io
helm repo update
helm install cert-manager jetstack/cert-manager \
  --namespace cert-manager \
  --create-namespace \
  --set crds.enabled=true
```

Wait until cert-manager is ready:

```bash
kubectl -n cert-manager wait --for=condition=Available deployment/cert-manager --timeout=60s
kubectl -n cert-manager wait --for=condition=Available deployment/cert-manager-webhook --timeout=60s
```

## Install the Operator

<Tabs groupId="install-method">
<TabItem value="helm-oci" label="Helm OCI (Recommended)" default>

The simplest installation method using the published OCI Helm chart.

```bash
helm install aerospike-ce-kubernetes-operator oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator \
  -n aerospike-operator --create-namespace
```

### Customizing Helm Values

You can override default values:

```bash
helm install aerospike-ce-kubernetes-operator oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator \
  -n aerospike-operator --create-namespace \
  --set replicaCount=2 \
  --set resources.limits.memory=256Mi
```

To see all available values:

```bash
helm show values oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator
```

### Cluster-admin pre-install (RBAC-only) {#cluster-admin-pre-install-rbac-only}

On restricted / multi-tenant clusters, the privileged **cluster-scoped** resources — the operator's `ClusterRole`/`ClusterRoleBinding` (the "cluster admin" grant) — are often installed by a platform team, separately from the namespaced operator workload and from the CRDs (which may be GitOps-managed). The `rbac.create` value supports this split:

- `rbac.create` defaults to `null`, which **tracks `operator.enabled`** — so existing installs are unaffected.
- Set it explicitly to decouple the cluster-scoped RBAC from the operator workload.

```bash
# Step 1 (cluster-admin, once): install ONLY the cluster-scoped RBAC.
#   No CRDs, no operator Deployment, no webhook, no UI.
helm install aerospike-ce-kubernetes-operator oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator \
  -n aerospike-operator --create-namespace \
  --set operator.enabled=false \
  --set crds.install=false \
  --set rbac.create=true \
  --set webhook.enabled=false --set certManager.enabled=false \
  --set ui.api.enabled=false --set ui.web.enabled=false

# Step 2 (app team): bring up the operator workload as a follow-up.
#   Same release name keeps the RBAC and aligns the ServiceAccount subject.
helm upgrade aerospike-ce-kubernetes-operator oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator \
  -n aerospike-operator \
  --set operator.enabled=true \
  --set crds.install=false
```

When installing from the chart source, the ready-made preset `values-cluster-admin.yaml` bundles the step-1 overrides:

```bash
helm install aerospike-ce-kubernetes-operator ./charts/aerospike-ce-kubernetes-operator \
  -f ./charts/aerospike-ce-kubernetes-operator/values-cluster-admin.yaml \
  -n aerospike-operator --create-namespace
```

Step 1 renders **only** the manager + metrics `ClusterRole`/`ClusterRoleBinding`. The `ServiceAccount` those bindings reference (and the namespaced leader-election `Role`/`RoleBinding`) are created with the operator workload in step 2 — a `ClusterRoleBinding` referencing a not-yet-created `ServiceAccount` is valid in Kubernetes, and the two steps own disjoint resources. To keep them as **separate** releases (owned by different teams), align resource names via `fullnameOverride` and set `rbac.create=false` on the operator release.

#### Pin a Cluster Manager UI release

The Cluster Manager UI (`ui-api` and `ui-web` Deployments) is versioned independently from the operator. The chart defaults `ui.imageTag` to `latest` so a fresh install always picks up the newest `aerospike-cluster-manager` image (Deployment `imagePullPolicy` defaults to `Always` to match). For reproducible deployments, override it with a specific cluster-manager release:

```bash
helm install aerospike-ce-kubernetes-operator oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator \
  -n aerospike-operator --create-namespace \
  --set ui.imageTag=0.30.0
```

Or override a single component via `ui.api.image.tag` / `ui.web.image.tag`. Available tags are listed at [aerospike-cluster-manager-api](https://github.com/aerospike-ce-ecosystem/aerospike-cluster-manager/pkgs/container/aerospike-cluster-manager-api) and [aerospike-cluster-manager-web](https://github.com/aerospike-ce-ecosystem/aerospike-cluster-manager/pkgs/container/aerospike-cluster-manager-web).

</TabItem>
<TabItem value="local-build" label="Local Build">

For developers and contributors building from source.

#### Quick Start with `make run-local`

The easiest way to get a full local development stack running. This single command handles everything — Kind cluster creation, image builds, cert-manager installation, and Helm deployment (operator + Cluster Manager UI).

```bash
git clone --recursive https://github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator.git
cd aerospike-ce-kubernetes-operator
make run-local
```

`make run-local` performs the following steps automatically:
1. Refresh Helm sub-chart dependency (CRD bundle)
2. Create a fresh Kind cluster (`kind-config.yaml` — 3 worker nodes with zone labels)
3. Build operator image
4. Build Cluster Manager image
5. Load both images into the Kind cluster
6. Install cert-manager
7. Deploy operator + UI via `helm upgrade -i` (UI is on by default)

Once complete, access the UI:

```bash
kubectl port-forward -n aerospike-operator svc/aerospike-ce-kubernetes-operator-ui-web 3100:3100
```

Other useful local development commands:

| Command | Description |
|---------|-------------|
| `make stop-local` | Delete the Kind cluster |
| `make reload-cluster-manager` | Rebuild images, reload into Kind, and redeploy (for iterative development) |

:::info Prerequisites
- [Kind](https://kind.sigs.k8s.io/) installed
- [Helm](https://helm.sh/) v3 installed
- [Podman](https://podman.io/) installed (or set `CONTAINER_TOOL=docker`)
:::

#### Manual Deployment (Custom Registry)

If you need to push images to a custom registry instead of loading into Kind:

```bash
git clone https://github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator.git
cd aerospike-ce-kubernetes-operator

# Build and push the operator image to your registry
make docker-build docker-push IMG=<your-registry>/aerospike-ce-kubernetes-operator:latest

# Install CRDs
make install

# Deploy the operator
make deploy IMG=<your-registry>/aerospike-ce-kubernetes-operator:latest
```

</TabItem>
</Tabs>

## Monitoring (Optional)

The Helm chart includes built-in support for Prometheus Operator monitoring resources.
All monitoring features are **disabled by default** and require [Prometheus Operator](https://github.com/prometheus-operator/prometheus-operator) (commonly installed via [kube-prometheus-stack](https://github.com/prometheus-community/helm-charts/tree/main/charts/kube-prometheus-stack)).

### ServiceMonitor

Creates a `ServiceMonitor` resource so Prometheus automatically scrapes operator metrics.

```bash
helm install aerospike-ce-kubernetes-operator oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator \
  -n aerospike-operator --create-namespace \
  --set serviceMonitor.enabled=true \
  --set serviceMonitor.additionalLabels.release=prometheus
```

:::tip
The `additionalLabels.release=prometheus` label must match your Prometheus Operator's `serviceMonitorSelector`. Check with:
```bash
kubectl get prometheus -A -o jsonpath='{.items[*].spec.serviceMonitorSelector}'
```
:::

| Parameter | Default | Description |
|-----------|---------|-------------|
| `serviceMonitor.enabled` | `false` | Create ServiceMonitor resource |
| `serviceMonitor.interval` | — | Scrape interval (e.g., `"30s"`) |
| `serviceMonitor.scrapeTimeout` | — | Scrape timeout (e.g., `"10s"`) |
| `serviceMonitor.additionalLabels` | `{}` | Extra labels for Prometheus selector matching |

### PrometheusRule

Creates a `PrometheusRule` resource with built-in alerting rules for the operator.

```bash
helm install aerospike-ce-kubernetes-operator oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator \
  -n aerospike-operator --create-namespace \
  --set serviceMonitor.enabled=true \
  --set prometheusRule.enabled=true
```

**Built-in alerts** (used when `prometheusRule.rules` is empty):

| Alert | Condition | Severity |
|-------|-----------|----------|
| `AerospikeOperatorDown` | Operator unreachable for 5m | critical |
| `AerospikeOperatorReconcileErrors` | Reconcile error rate > 0 for 15m | warning |
| `AerospikeOperatorSlowReconcile` | p99 reconcile time > 60s for 10m | warning |
| `AerospikeOperatorWorkQueueDepth` | Queue depth > 10 for 10m | warning |
| `AerospikeOperatorHighMemory` | Memory > 256Mi for 10m | warning |
| `AerospikeOperatorPodRestarts` | > 3 restarts in 1h | warning |

To provide **custom rules** instead of the built-in defaults, use a `values.yaml` file:

```yaml
prometheusRule:
  enabled: true
  rules:
    - alert: CustomAerospikeAlert
      expr: up{job="aerospike"} == 0
      for: 5m
      labels:
        severity: critical
      annotations:
        summary: "Custom Aerospike alert"
```

| Parameter | Default | Description |
|-----------|---------|-------------|
| `prometheusRule.enabled` | `false` | Create PrometheusRule resource |
| `prometheusRule.additionalLabels` | `{}` | Extra labels for Prometheus selector matching |
| `prometheusRule.rules` | `[]` | Custom rules; when empty, built-in defaults are used |

### Grafana Dashboard

Creates a `ConfigMap` with a pre-built Grafana dashboard. Requires the [Grafana sidecar](https://github.com/grafana/helm-charts/tree/main/charts/grafana#sidecar-for-dashboards) to be configured for auto-discovery.

```bash
helm install aerospike-ce-kubernetes-operator oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator \
  -n aerospike-operator --create-namespace \
  --set grafanaDashboard.enabled=true
```

The dashboard includes panels for: **Reconcile Rate**, **Reconcile Errors**, **Reconcile Duration (p99/p50)**, **Work Queue Depth**, **Operator Memory Usage**, and **Operator CPU Usage**.

| Parameter | Default | Description |
|-----------|---------|-------------|
| `grafanaDashboard.enabled` | `false` | Create dashboard ConfigMap |
| `grafanaDashboard.sidecarLabel` | `grafana_dashboard` | Label key for Grafana sidecar discovery |
| `grafanaDashboard.sidecarLabelValue` | `"1"` | Label value for Grafana sidecar discovery |
| `grafanaDashboard.folder` | `""` | Grafana folder name for organizing dashboards |

### Setting Up Grafana with Dashboard Auto-Discovery

A step-by-step guide to install Grafana with sidecar enabled and view the operator dashboard via port-forward.

**1. Add Grafana Helm repository:**

```bash
helm repo add grafana https://grafana.github.io/helm-charts
helm repo update
```

**2. Install Grafana with sidecar enabled:**

```bash
helm install grafana grafana/grafana \
  -n monitoring --create-namespace \
  --set sidecar.dashboards.enabled=true \
  --set sidecar.dashboards.label=grafana_dashboard \
  --set sidecar.dashboards.labelValue="1" \
  --set sidecar.dashboards.searchNamespace=ALL \
  --set sidecar.datasources.enabled=true
```

:::info
`sidecar.dashboards.searchNamespace=ALL` enables the sidecar to discover dashboard ConfigMaps across **all namespaces**, including `aerospike-operator` where the operator chart deploys the dashboard ConfigMap. Without this, the sidecar only watches its own namespace.
:::

**3. Install (or upgrade) the operator with the dashboard enabled:**

```bash
helm install aerospike-ce-kubernetes-operator oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator \
  -n aerospike-operator --create-namespace \
  --set grafanaDashboard.enabled=true
```

**4. Get the Grafana admin password:**

```bash
kubectl -n monitoring get secret grafana -o jsonpath="{.data.admin-password}" | base64 -d; echo
```

**5. Port-forward to access Grafana:**

```bash
kubectl -n monitoring port-forward svc/grafana 3000:80
```

**6.** Open [http://localhost:3000](http://localhost:3000) in your browser. Log in with:
- **Username:** `admin`
- **Password:** the value from step 4

The **"Aerospike CE Operator"** dashboard will appear automatically under **Dashboards**. If you set `grafanaDashboard.folder`, it will be organized in the specified folder.

:::tip
If the dashboard does not appear, verify the ConfigMap was created and has the correct label:
```bash
kubectl -n aerospike-operator get configmap -l grafana_dashboard=1
```
:::

### Full Monitoring Stack Example

Enable all monitoring features at once:

```bash
helm install aerospike-ce-kubernetes-operator oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator \
  -n aerospike-operator --create-namespace \
  --set serviceMonitor.enabled=true \
  --set serviceMonitor.additionalLabels.release=prometheus \
  --set prometheusRule.enabled=true \
  --set grafanaDashboard.enabled=true \
  --set grafanaDashboard.folder=Aerospike
```

## Verify Installation

Check the operator pod is running:

```bash
kubectl -n aerospike-operator get pods
```

Expected output:

```
NAME                                    READY   STATUS    RESTARTS   AGE
aerospike-ce-kubernetes-operator-controller-manager-xxxxx-yyyyy     1/1     Running   0          30s
```

Check the CRD is registered:

```bash
kubectl get crd aerospikeclusters.acko.io
```

## Quick Start

:::info Prerequisites
- [Kind](https://kind.sigs.k8s.io/) installed
- [Helm](https://helm.sh/) v3 installed
- [kubectl](https://kubernetes.io/docs/tasks/tools/) installed
:::

<Tabs groupId="quickstart">
<TabItem value="simple" label="Simple" default>

Operator + Cluster Manager UI on a Kind cluster.

```bash
# Create Kind cluster
kind create cluster

# Install cert-manager
helm repo add jetstack https://charts.jetstack.io
helm repo update jetstack
helm install cert-manager jetstack/cert-manager \
  --namespace cert-manager \
  --create-namespace \
  --set crds.enabled=true \
  --wait

# Install Aerospike Operator (UI enabled by default)
helm install aerospike-ce-kubernetes-operator oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator \
  -n aerospike-operator --create-namespace \
  --wait

# Deploy a sample Aerospike cluster
kubectl create namespace aerospike --dry-run=client -o yaml | kubectl apply -f -
kubectl apply -f config/samples/acko_v1alpha1_aerospikecluster.yaml

# Verify
kubectl -n aerospike wait --for=condition=Ready pod/aerospike-basic-0-0 --timeout=120s
kubectl -n aerospike exec -it aerospike-basic-0-0 -- asinfo -v status

# Access the UI
kubectl -n aerospike-operator port-forward svc/aerospike-ce-kubernetes-operator-ui-web 3100:3100
```

</TabItem>
<TabItem value="full-monitoring" label="Full Monitoring">

Operator + UI + Prometheus + Grafana dashboard on a Kind cluster.

```bash
# Create Kind cluster
kind create cluster

# =============================================================================
# 1. Install cert-manager
# =============================================================================
helm repo add jetstack https://charts.jetstack.io
helm repo update jetstack
helm install cert-manager jetstack/cert-manager \
  --namespace cert-manager \
  --create-namespace \
  --set crds.enabled=true \
  --wait

# =============================================================================
# 2. Install Prometheus Operator (kube-prometheus-stack)
# =============================================================================
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update prometheus-community
helm install prometheus prometheus-community/kube-prometheus-stack \
  --namespace monitoring \
  --create-namespace \
  --set grafana.enabled=false \
  --wait

# =============================================================================
# 3. Install Aerospike Operator (UI enabled by default, all monitoring enabled)
# =============================================================================
helm install aerospike-ce-kubernetes-operator oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator \
  -n aerospike-operator --create-namespace \
  --set serviceMonitor.enabled=true \
  --set serviceMonitor.additionalLabels.release=prometheus \
  --set prometheusRule.enabled=true \
  --set grafanaDashboard.enabled=true \
  --set grafanaDashboard.folder=Aerospike \
  --wait

# =============================================================================
# 4. Install Grafana with sidecar dashboard auto-discovery
# =============================================================================
helm repo add grafana https://grafana.github.io/helm-charts
helm repo update grafana
helm install grafana grafana/grafana \
  --namespace monitoring \
  --set sidecar.dashboards.enabled=true \
  --set sidecar.dashboards.label=grafana_dashboard \
  --set sidecar.dashboards.labelValue="1" \
  --set sidecar.dashboards.searchNamespace=ALL \
  --set sidecar.datasources.enabled=true \
  --wait

# =============================================================================
# 5. Deploy a sample Aerospike cluster
# =============================================================================
kubectl create namespace aerospike --dry-run=client -o yaml | kubectl apply -f -
kubectl apply -f config/samples/acko_v1alpha1_aerospikecluster.yaml

echo "Waiting for Aerospike pod to be ready..."
kubectl -n aerospike wait --for=condition=Ready pod/aerospike-basic-0-0 --timeout=120s

# =============================================================================
# 6. Verify
# =============================================================================
echo "=== Aerospike cluster info ==="
kubectl -n aerospike exec -it aerospike-basic-0-0 -- asinfo -v status
kubectl -n aerospike exec -it aerospike-basic-0-0 -- asinfo -v build

# =============================================================================
# 7. Access
# =============================================================================
# UI
kubectl -n aerospike-operator port-forward svc/aerospike-ce-kubernetes-operator-ui-web 3100:3100 &

# Grafana
GRAFANA_PASSWORD=$(kubectl -n monitoring get secret grafana \
  -o jsonpath="{.data.admin-password}" | base64 -d)
echo ""
echo "=== Grafana ==="
echo "URL:      http://localhost:3001"
echo "Username: admin"
echo "Password: ${GRAFANA_PASSWORD}"
echo ""
kubectl -n monitoring port-forward svc/grafana 3001:80
```

</TabItem>
</Tabs>

## Uninstall

:::warning
Always delete AerospikeCluster resources before uninstalling the operator. Removing the operator first will leave orphaned StatefulSets and PVCs.
:::

<Tabs groupId="install-method">
<TabItem value="helm-oci" label="Helm" default>

```bash
# Delete all Aerospike clusters first
kubectl delete asc --all --all-namespaces

# Uninstall the operator
helm uninstall aerospike-ce-kubernetes-operator -n aerospike-operator

# (Optional) Uninstall CRDs — WARNING: this deletes all AerospikeCluster data
# helm uninstall aerospike-ce-kubernetes-operator-crds

# (Optional) Delete the namespace
kubectl delete namespace aerospike-operator
```

</TabItem>
<TabItem value="local-build" label="Local Build">

If you used `make run-local`, simply delete the Kind cluster:

```bash
make stop-local
```

If you deployed manually to an existing cluster:

```bash
# Delete all Aerospike clusters first
kubectl delete asc --all --all-namespaces

# Remove the operator
make undeploy

# Remove CRDs
make uninstall
```

</TabItem>
</Tabs>
