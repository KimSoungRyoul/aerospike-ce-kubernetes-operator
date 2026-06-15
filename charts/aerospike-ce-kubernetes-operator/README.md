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
> helm install acko ./charts/aerospike-ce-kubernetes-operator \
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
helm install acko oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator \
  --version 1.3.1 \
  --namespace aerospike-operator --create-namespace
```

### Method 2: Separate CRD chart (GitOps / ArgoCD / Flux)

Install `aerospike-ce-kubernetes-operator-crds` once, then the operator separately. This is the recommended
approach for GitOps workflows to control CRD lifecycle independently.

```bash
# Step 1: Install CRDs (once per cluster)
helm install acko-crds oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator-crds \
  --version 1.3.1

# Step 2: Install operator (skip CRD installation)
helm install acko oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator \
  --version 1.3.1 \
  --set crds.install=false \
  --namespace aerospike-operator --create-namespace
```

### Method 3: Cluster-admin pre-install (RBAC-only, restricted clusters)

On restricted / multi-tenant clusters the privileged, cluster-scoped resources
(the operator's `ClusterRole`/`ClusterRoleBinding` — the "cluster admin" grant)
are often installed by a platform team, separately from the namespaced operator
workload and from CRDs (which may be GitOps-managed). The chart supports this
split via the `rbac.create` toggle:

- `rbac.create` defaults to **`null`**, which **tracks `operator.enabled`** — so
  existing installs are unaffected.
- Set it explicitly to decouple the cluster-scoped RBAC from the operator
  workload.

```bash
# Step 1 (cluster-admin, once): install ONLY the cluster-scoped RBAC.
#   - no CRDs (crds.install=false), no operator Deployment, no webhook, no UI
#   - ready-made preset: values-cluster-admin.yaml
helm install acko ./charts/aerospike-ce-kubernetes-operator \
  -f ./charts/aerospike-ce-kubernetes-operator/values-cluster-admin.yaml \
  --namespace aerospike-operator --create-namespace

# Step 2 (app team): bring up the operator workload as a follow-up.
#   Same release name keeps the RBAC the release already owns and lines up the
#   ServiceAccount/ClusterRoleBinding subjects. CRDs stay externally managed.
helm upgrade acko ./charts/aerospike-ce-kubernetes-operator \
  --namespace aerospike-operator \
  --set operator.enabled=true \
  --set crds.install=false
```

To keep the two phases as **separate releases** (e.g. owned by different teams),
align the resource names with `fullnameOverride` across both releases and set
`rbac.create=false` on the operator release so it does not try to re-create the
`ClusterRole`/`ClusterRoleBinding` the cluster-admin release already owns. The
two releases then own **disjoint** resources — the cluster-admin release owns
only the cluster-scoped RBAC, the operator release owns the workload plus the
namespaced `ServiceAccount` and leader-election `Role`/`RoleBinding` — so there
is no `ServiceAccount` ownership conflict.

`rbac.create=true` with `operator.enabled=false` renders **only** the manager +
metrics `ClusterRole`/`ClusterRoleBinding` — nothing else. The
`ServiceAccount` those bindings reference (and the namespaced leader-election
`Role`/`RoleBinding`) are created with the operator workload; a `ClusterRoleBinding`
referencing a not-yet-created `ServiceAccount` is valid in Kubernetes.

### From Local Chart

```bash
helm install acko ./charts/aerospike-ce-kubernetes-operator \
  --namespace aerospike-operator --create-namespace
```

### With monitoring enabled

```bash
helm install acko oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator \
  --version 1.3.1 \
  --namespace aerospike-operator --create-namespace \
  --set serviceMonitor.enabled=true \
  --set prometheusRule.enabled=true \
  --set grafanaDashboard.enabled=true
```

### With operator OpenTelemetry export

The operator can additionally export its own **traces, metrics, and logs**
to an OTLP/gRPC collector. Off by default; metrics are the whole
controller-runtime + ACKO Prometheus registry bridged to OTLP, so the
`/metrics` scrape endpoint keeps working alongside the push.

```yaml
observability:
  otel:
    enabled: true
    endpoint: otel-collector.observability.svc.cluster.local:4317  # OTLP/gRPC
    serviceName: ""                                                # override service.name
    headers: ""                                                    # collector auth
    resourceAttributes: "deployment.environment=prod,team=platform"
    sampler: parentbased_traceidratio
    samplerArg: "1.0"
    collectorPort: 4317                                            # NetworkPolicy egress port
```

`endpoint` is required when `enabled: true` (rendering fails fast otherwise).
All settings map to the OTel SDK standard environment variables.

When `networkPolicy.enabled` or `cilium.enabled` is set, enabling OTel
automatically opens an egress rule to the collector on `collectorPort`
(default `4317`) — the locked-down operator egress would otherwise drop all
exported telemetry. See
[Monitoring — OpenTelemetry export](https://aerospike-ce-ecosystem.github.io/aerospike-ce-kubernetes-operator/operations/monitoring)
for the full reference.

### With Cilium network policy

```bash
helm install acko oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator \
  --version 1.3.1 \
  --namespace aerospike-operator --create-namespace \
  --set cilium.enabled=true
```

### With Cluster Manager UI

The Cluster Manager UI (api + web) is deployed by default. A plain
`helm install` of the chart already brings both Deployments up:

```bash
helm install acko oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator \
  --version 1.3.1 \
  --namespace aerospike-operator --create-namespace
```

To skip the UI entirely (operator-only install), set both toggles
to `false`:

```bash
helm install acko oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator \
  --version 1.3.1 \
  --namespace aerospike-operator --create-namespace \
  --set ui.api.enabled=false --set ui.web.enabled=false
```

Access the UI:
```bash
kubectl port-forward svc/<release-name>-aerospike-ce-kubernetes-operator-ui-web 3100:3100 -n aerospike-operator
# Open http://localhost:3100
```

#### Independent api / web toggles (chart 0.3.0+)

The Cluster Manager ships as two Deployments — `api` (FastAPI) and `web`
(Next.js). The per-component toggles `ui.api.enabled` and
`ui.web.enabled` (both defaulting to true) decide which Deployments are
created. This lets you run API-only or Web-only modes without forking
the chart. (Chart 0.4.0+: the legacy `ui.enabled` master switch was
removed — use these two toggles directly.)

```yaml
# API only — Swagger / CLI / external UI talks to the API directly
ui:
  web:
    enabled: false

# Web only — point the web frontend at an external API instance
ui:
  api:
    enabled: false
  web:
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
      # aerospike-py Rust-core instrumentation
      aerospikePyLogLevel: ""                       # empty → inherit LOG_LEVEL
      aerospikePyTracing: true                      # emit aerospike.<op> spans
```

When `ui.api.otel.enabled=false` (default), the deployment sets
`OTEL_SDK_DISABLED=true` and the API uses NoOp providers — zero overhead.

`aerospikePyTracing` starts aerospike-py's own OTLP span exporter so every
Aerospike operation is traced, and `aerospikePyLogLevel` routes the client's
Rust-core logs into the API log stream. The API exporter speaks OTLP/gRPC and
HTTP; aerospike-py's exporter is **gRPC only**, so use a gRPC `endpoint` when
`aerospikePyTracing` is on.

#### Log routing via external OTel Collector

ACM emits structured JSON to stdout (with `request_id` /
`otelTraceID` / `otelSpanID` correlation fields) and delegates all
external routing — PII redaction, sampling, vendor exporters (Datadog,
Loki, Elasticsearch, Sentry, ...) — to an **external OpenTelemetry Collector**
that the operator runs elsewhere in the cluster. This chart does NOT
deploy a Collector; it only opt-in deploys a per-pod OTLP-forwarder
sidecar.

Two patterns:

1. **Node-level DaemonSet OTel Collector** scraping container logs.
   Default — leave `ui.api.logging.fileMirror.enabled=false` and
   `ui.api.logging.sidecar.enabled=false`. ACM stdout is the only sink;
   your existing Collector DaemonSet picks it up from
   `/var/log/containers/*.log`.

2. **Pod-internal OTLP-forwarder sidecar** (when DaemonSet scraping is
   not an option — e.g. multi-tenant clusters where each tenant brings
   its own Collector, or no permission for a hostPath DaemonSet). Enable
   `fileMirror` + `sidecar`. The chart wires a shared `emptyDir`, the
   api writes a rotating file, and a fluent-bit sidecar forwards via
   OTLP/gRPC to the Collector you specify:

```yaml
ui:
  env:
    logFormat: "json"     # recommended so the Collector can parse fields
  api:
    logging:
      fileMirror:
        enabled: true     # /var/log/acm/api.log on a shared emptyDir
      sidecar:
        enabled: true
        otlp:
          endpoint: "otel-collector.observability.svc.cluster.local:4317"
          # tls: true
          # headers: "x-tenant=acm,authorization=Bearer ..."
```

The chart renders a default fluent-bit config that tails the file,
parses JSON, and forwards via the `opentelemetry` output plugin. To
plug a different shipper (vector, promtail, vendor agent), override
`sidecar.image` and `sidecar.config.content`.

Validation guards:

- `sidecar.enabled=true` without `fileMirror.enabled=true` → install
  fails (sidecar would have nothing to tail).
- `sidecar.enabled=true` without `sidecar.otlp.endpoint` AND without
  `sidecar.config.content` → install fails (default template needs an
  endpoint; bring your own config if you want different routing).
- `sidecar.config.content` containing a `---` line or starting with `%`
  → install fails (would corrupt the rendered ConfigMap YAML).

See [docs/observability.md](https://github.com/aerospike-ce-ecosystem/aerospike-cluster-manager/blob/main/docs/observability.md)
and [docs/logging.md](https://github.com/aerospike-ce-ecosystem/aerospike-cluster-manager/blob/main/docs/logging.md)
in the cluster-manager repo for the full reference and example fluent-bit
sidecar configuration.

#### Database backend

The api persists cluster connection metadata in a database. The backend is
selected by `ui.database.type`:

| Backend | Where the data lives | Use when |
|---------|----------------------|----------|
| `sqlite` (default) | Embedded SQLite file inside the api container, on a PVC (`ui.database.sqlite.persistence`) | Single-instance installs. Zero extra infrastructure. SQLite is single-writer, so `ui.replicaCount` must stay `1`. |
| `postgresql`, external (`deploy=false`) | An **external** PostgreSQL instance you operate (RDS / Cloud SQL / AlloyDB / an in-cluster PostgreSQL operator) | HA / multi-replica. The chart provisions no database. |
| `postgresql`, chart-managed (`deploy=true`) | A single-replica PostgreSQL **Deployment** the chart provisions (Service + data PVC + Secret) | A turnkey PostgreSQL with no external dependency. Convenient, **not** highly available. |

SQLite (default) — nothing to configure:

```bash
helm install acko oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator \
  --version 1.3.1 --namespace aerospike-operator --create-namespace \
  --set ui.database.sqlite.persistence.size=2Gi
```

External PostgreSQL — supply the connection URL (or an existing Secret with a
`DATABASE_URL` key via `ui.database.postgresql.existingSecret`):

```bash
helm install acko oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator \
  --version 1.3.1 --namespace aerospike-operator --create-namespace \
  --set ui.database.type=postgresql \
  --set ui.database.postgresql.databaseUrl='postgresql://user:pass@db-host:5432/aerospike_manager'
```

Chart-managed PostgreSQL — let the chart run a single-replica PostgreSQL
`Deployment` and wire the api to it:

```bash
helm install acko oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator \
  --version 1.3.1 --namespace aerospike-operator --create-namespace \
  --set ui.database.type=postgresql \
  --set ui.database.postgresql.deploy=true
```

The data PVC carries `helm.sh/resource-policy: keep`, so it survives `helm
uninstall` — delete it by hand to discard the database. This backend is
single-replica (`Recreate` update strategy on a `ReadWriteOnce` volume); run an
external PostgreSQL for HA.

> **GitOps (ArgoCD / Flux / kustomize `helmCharts`) — keep the password stable.**
> With `deploy=true` and an empty `auth.password`, the chart generates a random
> `POSTGRES_PASSWORD` and preserves it by re-reading the live Secret. That
> `lookup` only works for a server-side `helm install` / `helm upgrade` — a
> **client-side `helm template`** (what GitOps tools run) cannot read the
> cluster, so it regenerates the password on **every** render and breaks the
> running database. Pick one of:
>
> - **`ui.database.postgresql.auth.password`** — set an explicit password
>   (`[A-Za-z0-9._~-]` characters only). Simple, but the password sits in values.
> - **`ui.database.postgresql.existingSecret`** — point at a Secret you manage
>   (sealed-secret / vault) carrying both `POSTGRES_PASSWORD` and a `DATABASE_URL`
>   (`postgresql://<user>:<pass>@<release>-…-ui-postgres:5432/<db>`). The chart
>   then renders no Secret of its own. This keeps credentials out of Git and is
>   the recommended GitOps path.

#### Migrating off the embedded PostgreSQL sidecar

Earlier chart versions ran a `postgres` container as a sidecar in the api pod,
with its data on the `<release>-…-ui-data` PVC. That sidecar is **removed**.
There is **no automatic data migration**:

- `ui.database.type=sqlite` → the old PostgreSQL files are stranded on the PVC; the api starts on a fresh, empty SQLite database.
- `ui.database.type=postgresql` → Helm deletes the old PVC; with a `Delete` reclaim policy the data is destroyed.

To protect against silent data loss, the chart **blocks any upgrade** from a
release that still has the embedded-mode Secret (`<release>-…-ui-db` with a
`POSTGRES_PASSWORD` key) until you set
`ui.database.acknowledgeEmbeddedPostgresRemoval=true`.

**Migration runbook (preserve your data → external PostgreSQL):**

```bash
# 1. Back up the embedded database (old release still running)
kubectl exec -n <ns> deploy/<release>-aerospike-ce-kubernetes-operator-ui-api \
  -c postgres -- pg_dump -U aerospike -d aerospike_manager --no-owner --no-privileges \
  > acm-backup.sql

# 2. Restore into your external PostgreSQL (RDS / Cloud SQL / AlloyDB / …)
psql "postgresql://user:pass@db-host:5432/aerospike_manager" -v ON_ERROR_STOP=1 < acm-backup.sql

# 3. Upgrade — point at the external DB and acknowledge the sidecar removal
helm upgrade <release> oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator \
  -n <ns> --reuse-values \
  --set ui.database.type=postgresql \
  --set ui.database.postgresql.databaseUrl='postgresql://user:pass@db-host:5432/aerospike_manager' \
  --set ui.database.acknowledgeEmbeddedPostgresRemoval=true
```

The api's `init_db()` uses `CREATE TABLE IF NOT EXISTS`, so restoring the full
dump first and then starting the new api is safe. There is no automated
PostgreSQL→SQLite path — to land on SQLite instead, re-add connections from the
UI.

**Value mapping:** `ui.postgresql.enabled: true` → `ui.database.type: postgresql` + `ui.database.postgresql.databaseUrl`; `ui.postgresql.enabled: false` → `ui.database.type: sqlite`; `ui.persistence.*` → `ui.database.sqlite.persistence.*`; `ui.env.databaseUrl` → `ui.database.postgresql.databaseUrl`. Stale `ui.postgresql.*` / `ui.persistence.*` keys fail the install with a migration message.

#### Upgrading the chart-managed PostgreSQL (StatefulSet → Deployment)

Chart **v1.6.0** ran the chart-managed PostgreSQL (`deploy=true`) as a
`StatefulSet` whose data lived on a `volumeClaimTemplate` PVC named
`data-<release>-…-ui-postgres-0`. Newer chart versions run it as a
`Deployment` with a standalone PVC named `<release>-…-ui-postgres-data`.

A plain `helm upgrade` would delete the StatefulSet (its PVC is **retained**
but orphaned) and start the Deployment on a **new, empty** PVC — the database
is silently lost. To protect against that, the chart **blocks the upgrade**
when it detects the leftover StatefulSet, until you set
`ui.database.postgresql.acknowledgeStatefulSetMigration=true`.

**Migration runbook (preserve your data):**

```bash
# 1. Back up the database from the running StatefulSet pod
kubectl exec -n <ns> <release>-aerospike-ce-kubernetes-operator-ui-postgres-0 \
  -- pg_dump -U aerospike -d aerospike_manager --no-owner --no-privileges \
  > acm-pg-backup.sql

# 2. Upgrade — the chart replaces the StatefulSet with a Deployment + new PVC
helm upgrade <release> oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator \
  -n <ns> --reuse-values \
  --set ui.database.postgresql.acknowledgeStatefulSetMigration=true

# 3. Restore into the new Deployment's database
kubectl exec -i -n <ns> deploy/<release>-aerospike-ce-kubernetes-operator-ui-postgres \
  -- psql -U aerospike -d aerospike_manager -v ON_ERROR_STOP=1 < acm-pg-backup.sql
```

The old `data-<release>-…-ui-postgres-0` PVC is left untouched — delete it by
hand once the restore is verified. On a fresh install there is no StatefulSet,
so the gate never fires.

#### Customizing the UI deployment

You can customize the UI with per-component image, service, resource, and environment values:

```bash
helm install acko oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator \
  --version 1.3.1 \
  --namespace aerospike-operator --create-namespace \
  --set ui.imageTag=0.30.0 \
  --set ui.api.image.repository=ghcr.io/aerospike-ce-ecosystem/aerospike-cluster-manager-api \
  --set ui.web.image.repository=ghcr.io/aerospike-ce-ecosystem/aerospike-cluster-manager-web \
  --set ui.web.service.type=LoadBalancer \
  --set ui.web.service.annotations."service\.beta\.kubernetes\.io/aws-load-balancer-type"=nlb \
  --set ui.api.resources.requests.cpu=200m \
  --set ui.api.resources.requests.memory=512Mi \
  --set ui.api.resources.limits.cpu=500m \
  --set ui.api.resources.limits.memory=1Gi
```

Shared extra environment variables can be passed to both UI containers via
`ui.extraEnv`; API-only environment via `ui.api.extraEnv` /
`ui.api.extraEnvFrom`; web-only environment via `ui.web.extraEnv` /
`ui.web.extraEnvFrom`:

```yaml
ui:
  extraEnv:
    - name: LOG_LEVEL
      value: DEBUG
    - name: CUSTOM_VAR
      valueFrom:
        secretKeyRef:
          name: my-secret
          key: value
  api:
    extraEnv:
      - name: API_ONLY_SETTING
        value: enabled
```

### ACKO Agent (embedded AI copilot)

The web UI ships an optional, off-by-default AI assistant ("ACKO Agent",
cluster-manager >= 0.31). Enable it by giving ONLY the web container an LLM
model and the matching provider API key — use the web-scoped values so the
key never reaches the api container:

```bash
kubectl -n aerospike-operator create secret generic acko-agent-secrets \
  --from-literal=COPILOT_MODEL=anthropic/claude-sonnet-4-5 \
  --from-literal=ANTHROPIC_API_KEY=<your-key>
```

```yaml
ui:
  web:
    extraEnvFrom:
      - secretRef:
          name: acko-agent-secrets
```

#### OpenAI-compatible gateway (e.g. NAVER MODEL CONNECT Open Models)

To route the agent through an OpenAI-compatible gateway instead of the public
OpenAI API, add `COPILOT_BASE_URL` and use an `openai/*` model id with
`OPENAI_API_KEY`. For NAMC Open Models API (issue the key from the project's
Open Models API tab in the MODEL CONNECT console):

```bash
kubectl -n aerospike-operator create secret generic acko-agent-secrets \
  --from-literal=COPILOT_MODEL=openai/gpt-oss-120b \
  --from-literal=COPILOT_BASE_URL=https://namc-aigw.io.naver.com \
  --from-literal=OPENAI_API_KEY=<MODEL CONNECT Open Models API key>
```

Pick a model that supports function/tool calling so the agent's read tools
work. The `ui.web.extraEnvFrom` snippet above is unchanged.

Without the secret the UI renders unchanged and the agent stays disabled.
See the cluster-manager README for the full COPILOT_* variable reference
(COPILOT_REQUIRE_AUTH, COPILOT_OIDC_ISSUER_URL, ...).

### Full example

```bash
helm install acko oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator \
  --version 1.3.1 \
  --namespace aerospike-operator --create-namespace \
  --set serviceMonitor.enabled=true \
  --set prometheusRule.enabled=true \
  --set grafanaDashboard.enabled=true \
  --set podDisruptionBudget.enabled=true \
  --set cilium.enabled=true
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
    chart: aerospike-ce-kubernetes-operator-crds
    targetRevision: "1.3.1"
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
    chart: aerospike-ce-kubernetes-operator
    targetRevision: "1.3.1"
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
      chart: aerospike-ce-kubernetes-operator-crds
      version: "1.3.1"
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
      chart: aerospike-ce-kubernetes-operator
      version: "1.3.1"
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
| `operator.enabled` | Render the controller-manager workload and its supporting resources |
| `crds.install` | Install the CRDs subchart (set `false` when CRDs are managed separately) |
| `rbac.create` | Create the operator's cluster-scoped RBAC ("cluster admin" grant). `null` tracks `operator.enabled`; set explicitly for an RBAC-only / split install (see Method 3) |
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
