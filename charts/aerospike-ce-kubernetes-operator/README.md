# Aerospike CE Kubernetes Operator Helm Chart

Helm chart for deploying the Aerospike Community Edition Kubernetes Operator.

## Prerequisites

### 1. cert-manager (Required for webhooks)

The operator uses cert-manager to provision TLS certificates for admission webhooks.
Install cert-manager **before** installing this chart:

```bash
# Install cert-manager with CRDs
helm repo add jetstack https://charts.jetstack.io
helm repo update
helm install cert-manager jetstack/cert-manager \
  --namespace cert-manager --create-namespace \
  --set crds.enabled=true
```

Verify cert-manager is running:
```bash
kubectl -n cert-manager get pods
```

> **Alternative**: If you don't want cert-manager, disable it and provide a TLS secret manually:
> ```bash
> helm install acko ./charts/acko \
>   --set certManager.enabled=false \
>   --set webhookTlsSecret=my-webhook-tls
> ```

### 2. Prometheus Operator (Optional — for monitoring)

If you want to use `ServiceMonitor`, `PrometheusRule`, or Grafana dashboards,
install Prometheus Operator (kube-prometheus-stack) first:

```bash
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update
helm install kube-prometheus-stack prometheus-community/kube-prometheus-stack \
  --namespace monitoring --create-namespace \
  --set crds.enabled=true
```

## Installation

### Method 1: Single chart (Recommended for most users)

CRDs and the operator are installed together. `crds.install=true` is the default.

```bash
helm install acko oci://ghcr.io/aerospike-ce-ecosystem/charts/acko \
  --version 0.1.0 \
  --namespace aerospike-operator --create-namespace
```

### Method 2: Separate CRD chart (GitOps / ArgoCD / Flux)

Install `acko-crds` once, then the operator separately. This is the recommended
approach for GitOps workflows to control CRD lifecycle independently.

```bash
# Step 1: Install CRDs (once per cluster)
helm install acko-crds oci://ghcr.io/aerospike-ce-ecosystem/charts/acko-crds \
  --version 0.1.0

# Step 2: Install operator (skip CRD installation)
helm install acko oci://ghcr.io/aerospike-ce-ecosystem/charts/acko \
  --version 0.1.0 \
  --set crds.install=false \
  --namespace aerospike-operator --create-namespace
```

### From Local Chart

```bash
helm install acko ./charts/acko \
  --namespace aerospike-operator --create-namespace
```

### With monitoring enabled

```bash
helm install acko oci://ghcr.io/aerospike-ce-ecosystem/charts/acko \
  --version 0.1.0 \
  --namespace aerospike-operator --create-namespace \
  --set serviceMonitor.enabled=true \
  --set prometheusRule.enabled=true \
  --set grafanaDashboard.enabled=true
```

### With Cilium network policy

```bash
helm install acko oci://ghcr.io/aerospike-ce-ecosystem/charts/acko \
  --version 0.1.0 \
  --namespace aerospike-operator --create-namespace \
  --set cilium.enabled=true
```

### With Cluster Manager UI

```bash
helm install acko oci://ghcr.io/aerospike-ce-ecosystem/charts/acko \
  --version 0.1.0 \
  --namespace aerospike-operator --create-namespace \
  --set ui.enabled=true
```

Access the UI:
```bash
kubectl port-forward svc/<release-name>-acko-ui 3000:3000 -n aerospike-operator
# Open http://localhost:3000
```

#### Independent api / web toggles (chart 0.3.0+)

The Cluster Manager ships as two Deployments — `api` (FastAPI) and `web`
(Next.js). When `ui.enabled=true`, the per-component sub-toggles
`ui.api.enabled` and `ui.web.enabled` (both defaulting to true) decide
which Deployments are created. This lets you run API-only or Web-only
modes without forking the chart.

```yaml
# API only — Swagger / CLI / external UI talks to the API directly
ui:
  enabled: true
  web:
    enabled: false

# Web only — point the web frontend at an external API instance
ui:
  enabled: true
  api:
    enabled: false
  web:
    enabled: true
    env:
      apiUrl: "https://my-asm-api.example.com"
```

#### OpenTelemetry (chart 0.3.0+)

Enable OTel export via SDK-standard env vars surfaced as chart values:

```yaml
ui:
  api:
    otel:
      enabled: true
      endpoint: "http://otel-collector.observability.svc.cluster.local:4317"
      protocol: grpc                                # or http/protobuf
      sampler: parentbased_traceidratio
      samplerArg: "1.0"
      serviceName: aerospike-cluster-manager-api
      resourceAttributes: "deployment.environment=staging,team=platform"
      headers: ""
```

When `ui.api.otel.enabled=false` (default), the deployment sets
`OTEL_SDK_DISABLED=true` and the API uses NoOp providers — zero overhead.

#### Pluggable log handlers (chart 0.3.0+)

Forward API logs to NELO, Datadog, Loki, Sentry, or any
`logging.Handler` without rebuilding the upstream image. Either the
`extraPipPackages` init-container path or a custom image works:

```yaml
ui:
  api:
    extraPipPackages:
      - "pynelo>=1.0.0"
    logging:
      handlers: "pynelo:AsyncNeloHandler"
    extraEnv:
      - name: NELO_HOST
        value: "nelo-collector.svc.cluster.local"
    extraEnvFrom:
      - secretRef:
          name: nelo-token
```

For more elaborate routing (multiple handlers, formatters, filters), the
`ui.api.logging.dictConfig` value is written to a ConfigMap, mounted at
`/etc/asm/logging.yaml`, and applied via `logging.config.dictConfig`:

```yaml
ui:
  api:
    logging:
      dictConfig:
        version: 1
        disable_existing_loggers: false
        formatters:
          json:
            "()": pythonjsonlogger.json.JsonFormatter
            fmt: "%(asctime)s %(levelname)s %(name)s %(message)s %(otelTraceID)s"
        handlers:
          console:
            class: logging.StreamHandler
            formatter: json
        loggers:
          aerospike_cluster_manager_api:
            level: INFO
            handlers: [console]
            propagate: false
```

See [docs/observability.md](https://github.com/aerospike-ce-ecosystem/aerospike-cluster-manager/blob/main/docs/observability.md)
and [docs/logging.md](https://github.com/aerospike-ce-ecosystem/aerospike-cluster-manager/blob/main/docs/logging.md)
in the cluster-manager repo for the full reference, including airgap and
constraint notes.

#### Customizing the UI deployment

You can customize the UI with service annotations, resource defaults, and extra environment variables:

```bash
helm install acko oci://ghcr.io/aerospike-ce-ecosystem/charts/acko \
  --version 0.1.0 \
  --namespace aerospike-operator --create-namespace \
  --set ui.enabled=true \
  --set ui.service.type=LoadBalancer \
  --set ui.service.annotations."service\.beta\.kubernetes\.io/aws-load-balancer-type"=nlb \
  --set ui.resources.requests.cpu=200m \
  --set ui.resources.requests.memory=512Mi \
  --set ui.resources.limits.cpu=500m \
  --set ui.resources.limits.memory=1Gi
```

Extra environment variables can be passed via `ui.extraEnv`:

```yaml
ui:
  enabled: true
  extraEnv:
    - name: LOG_LEVEL
      value: DEBUG
    - name: CUSTOM_VAR
      valueFrom:
        secretKeyRef:
          name: my-secret
          key: value
```

### Full example

```bash
helm install acko oci://ghcr.io/aerospike-ce-ecosystem/charts/acko \
  --version 0.1.0 \
  --namespace aerospike-operator --create-namespace \
  --set serviceMonitor.enabled=true \
  --set prometheusRule.enabled=true \
  --set grafanaDashboard.enabled=true \
  --set podDisruptionBudget.enabled=true \
  --set cilium.enabled=true \
  --set ui.enabled=true
```

### GitOps — ArgoCD example

```yaml
# Application 1: CRDs (sync-wave 0, prune disabled)
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: acko-crds
  annotations:
    argocd.argoproj.io/sync-options: Replace=true
spec:
  source:
    repoURL: ghcr.io/aerospike-ce-ecosystem/charts
    chart: acko-crds
    targetRevision: "0.1.0"
  syncPolicy:
    automated:
      prune: false   # Never auto-delete CRDs
      selfHeal: true
---
# Application 2: Operator (sync-wave 1, depends on CRDs)
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: acko
spec:
  source:
    repoURL: ghcr.io/aerospike-ce-ecosystem/charts
    chart: acko
    targetRevision: "0.1.0"
    helm:
      values: |
        crds:
          install: false
  destination:
    namespace: aerospike-operator
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
    syncOptions:
      - CreateNamespace=true
```

### GitOps — Flux example

```yaml
# HelmRepository (OCI) — shared by both HelmReleases
apiVersion: source.toolkit.fluxcd.io/v1
kind: HelmRepository
metadata:
  name: acko
  namespace: flux-system
spec:
  type: oci
  url: oci://ghcr.io/aerospike-ce-ecosystem/charts
---
apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata:
  name: acko-crds
  namespace: flux-system
spec:
  chart:
    spec:
      chart: acko-crds
      version: "0.1.0"
      sourceRef:
        kind: HelmRepository
        name: acko
  install:
    crds: CreateReplace
  upgrade:
    crds: CreateReplace
---
apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata:
  name: acko
  namespace: flux-system
spec:
  dependsOn:
    - name: acko-crds
  targetNamespace: aerospike-operator
  chart:
    spec:
      chart: acko
      version: "0.1.0"
      sourceRef:
        kind: HelmRepository
        name: acko
  values:
    crds:
      install: false
```

## Deploy an Aerospike cluster

After the operator is running, create an Aerospike CE cluster:

```bash
kubectl create namespace aerospike

cat <<EOF | kubectl apply -f -
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
          data-size: 1073741824
EOF
```

Check the cluster status:
```bash
kubectl -n aerospike get asc
```

More sample CRs are in `config/samples/`.

## Configuration

See [values.yaml](values.yaml) for all available configuration options with descriptions.

### Key configuration sections

| Section | Description |
|---------|-------------|
| `certManager` | cert-manager integration for webhook TLS |
| `serviceMonitor` | Prometheus ServiceMonitor |
| `prometheusRule` | Prometheus alerting rules |
| `grafanaDashboard` | Grafana dashboard ConfigMap |
| `networkPolicy` | Standard Kubernetes NetworkPolicy |
| `cilium` | CiliumNetworkPolicy (alternative to networkPolicy) |
| `podDisruptionBudget` | PDB for operator pods |
| `autoscaling` | HPA for operator pods |
| `ui` | Aerospike Cluster Manager web UI |
| `defaultTemplates` | Pre-built AerospikeClusterTemplates (dev, stage, prod) |

## Uninstall

```bash
# Delete all Aerospike clusters first to avoid orphaned StatefulSets/PVCs
kubectl delete asc --all --all-namespaces

# Uninstall the operator
helm uninstall acko -n aerospike-operator
```

> **Note:** CRDs are protected with `helm.sh/resource-policy: keep` — they are
> **not** removed on `helm uninstall`. To remove CRDs explicitly (this deletes
> **all** AerospikeCluster resources and their data):
>
> ```bash
> kubectl delete crd aerospikeclusters.acko.io aerospikeclustertemplates.acko.io
> ```
