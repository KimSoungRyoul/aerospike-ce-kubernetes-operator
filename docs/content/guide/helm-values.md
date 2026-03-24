---
sidebar_position: 13
title: Helm Values Reference
---

# Helm Values Reference

This page documents all configurable values for the `aerospike-ce-kubernetes-operator` Helm chart.

## CRD Management

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `crds.install` | bool | `true` | Install aerospike-ce-kubernetes-operator-crds as a subchart dependency. Set to `false` if CRDs are managed separately (e.g., via GitOps). |
| `crds.keep` | bool | `true` | Retain CRDs on `helm uninstall`. Actual keep behavior is enforced by the `helm.sh/resource-policy: keep` annotation on each CRD template. |

## Operator

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `replicaCount` | int | `1` | Number of operator replicas. Typically 1 is sufficient as leader election handles HA. |
| `image.repository` | string | `ghcr.io/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator` | Operator container image repository. |
| `image.tag` | string | `""` | Container image tag. Defaults to `Chart.appVersion` when empty. |
| `image.pullPolicy` | string | `IfNotPresent` | Image pull policy: `Always`, `IfNotPresent`, or `Never`. |
| `imagePullSecrets` | list | `[]` | Image pull secrets for private registries. |
| `nameOverride` | string | `""` | Override the chart name used in resource names. |
| `fullnameOverride` | string | `""` | Override the full resource name (takes precedence over `nameOverride`). |

## Service Account

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `serviceAccount.annotations` | object | `{}` | Annotations for the operator service account. Useful for IAM roles (e.g., EKS IRSA, GKE Workload Identity). |

## Resources

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `resources.limits.cpu` | string | `500m` | CPU limit for the operator pod. |
| `resources.limits.memory` | string | `256Mi` | Memory limit for the operator pod. |
| `resources.requests.cpu` | string | `100m` | CPU request for the operator pod. |
| `resources.requests.memory` | string | `128Mi` | Memory request for the operator pod. |

## Webhook

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `webhook.enabled` | bool | `true` | Enable admission webhooks for CR validation and defaulting. |
| `webhook.port` | int | `9443` | Webhook server listen port. |

## cert-manager Integration

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `certManager.enabled` | bool | `true` | Use cert-manager to provision webhook TLS certificates. Requires cert-manager to be installed in the cluster. When disabled, provide a TLS secret manually via `webhookTlsSecret`. |
| `certManager.issuer.type` | string | `selfSigned` | Issuer type: `selfSigned`, `ca`, or `clusterIssuer`. |
| `certManager.issuer.name` | string | `""` | Name of an existing ClusterIssuer (only used when type is `clusterIssuer`). |
| `certManager.issuer.caSecretName` | string | `""` | CA secret name containing `tls.crt` and `tls.key` (only used when type is `ca`). |
| `certManager.duration` | string | `""` | Certificate duration (default: `8760h` = 1 year). |
| `certManager.renewBefore` | string | `""` | Certificate renewal time before expiry (default: `2880h` = 120 days). |
| `webhookTlsSecret` | string | `""` | Manually provide a TLS secret for the webhook server. Only used when `certManager.enabled` is `false` and `webhook.enabled` is `true`. The secret must contain `tls.crt` and `tls.key`. |

## Monitoring - ServiceMonitor

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `serviceMonitor.enabled` | bool | `false` | Create a ServiceMonitor resource for Prometheus Operator. |
| `serviceMonitor.interval` | string | — | Scrape interval (e.g., `30s`). |
| `serviceMonitor.scrapeTimeout` | string | — | Scrape timeout (e.g., `10s`). |
| `serviceMonitor.additionalLabels` | object | `{}` | Additional labels for ServiceMonitor discovery. |

## Monitoring - PrometheusRule

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `prometheusRule.enabled` | bool | `false` | Create PrometheusRule resource with operator alerting rules. |
| `prometheusRule.additionalLabels` | object | `{}` | Additional labels for PrometheusRule discovery. |
| `prometheusRule.rules` | list | `[]` | Custom alerting rules to append or override defaults. When empty, built-in default rules are used. |

## Monitoring - Grafana Dashboard

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `grafanaDashboard.enabled` | bool | `false` | Create a ConfigMap with a Grafana dashboard for the operator. Requires Grafana sidecar to be configured with dashboard auto-discovery. |
| `grafanaDashboard.sidecarLabel` | string | `grafana_dashboard` | Grafana sidecar label key for dashboard auto-discovery. |
| `grafanaDashboard.sidecarLabelValue` | string | `"1"` | Grafana sidecar label value. |
| `grafanaDashboard.folder` | string | `""` | Grafana folder annotation for organizing dashboards. |

## Network Policy

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `networkPolicy.enabled` | bool | `false` | Create standard Kubernetes NetworkPolicy resources. Mutually exclusive with `cilium.enabled`. |

## Cilium Network Policy

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `cilium.enabled` | bool | `false` | Create CiliumNetworkPolicy resources instead of standard NetworkPolicy. Mutually exclusive with `networkPolicy.enabled`. Requires Cilium CNI. |
| `cilium.l7Enabled` | bool | `false` | Enable L7 (application-layer) policy rules for Aerospike ports. |

## Pod Disruption Budget

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `podDisruptionBudget.enabled` | bool | `false` | Create a PodDisruptionBudget for the operator deployment. |
| `podDisruptionBudget.minAvailable` | int | `1` | Minimum available pods. Mutually exclusive with `maxUnavailable`. |
| `podDisruptionBudget.maxUnavailable` | int | — | Maximum unavailable pods. Mutually exclusive with `minAvailable`. |

## Horizontal Pod Autoscaler

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `autoscaling.enabled` | bool | `false` | Enable HPA for the operator deployment. Only useful when running multiple replicas. |
| `autoscaling.minReplicas` | int | `1` | Minimum number of replicas. |
| `autoscaling.maxReplicas` | int | `3` | Maximum number of replicas. |
| `autoscaling.targetCPUUtilizationPercentage` | int | `80` | Target average CPU utilization percentage. |
| `autoscaling.targetMemoryUtilizationPercentage` | int | — | Target average memory utilization percentage (optional). |

## Scheduling

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `nodeSelector` | object | `{}` | Node selector labels for operator pod scheduling. |
| `tolerations` | list | `[]` | Tolerations for operator pod scheduling. |
| `affinity` | object | `{}` | Affinity rules for operator pod scheduling. |
| `topologySpreadConstraints` | list | `[]` | Topology spread constraints for operator pod scheduling. |
| `priorityClassName` | string | `""` | Priority class name for operator pod. |

## Extra Annotations and Labels

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `podAnnotations` | object | `{}` | Additional annotations for the operator pods. |
| `podLabels` | object | `{}` | Additional labels for the operator pods. |

## UI - Aerospike Cluster Manager

The Aerospike Cluster Manager is a full-stack web dashboard deployed alongside the operator. It provides a visual interface for monitoring and managing Aerospike clusters.

### General

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `ui.enabled` | bool | `false` | Enable the Aerospike Cluster Manager web UI. |
| `ui.replicaCount` | int | `1` | Number of UI replicas. |
| `ui.image.repository` | string | `ghcr.io/aerospike-ce-ecosystem/aerospike-cluster-manager` | UI container image repository. |
| `ui.image.tag` | string | `"latest"` | UI container image tag. UI is versioned independently from the operator. |
| `ui.image.pullPolicy` | string | `IfNotPresent` | Image pull policy. |
| `ui.imagePullSecrets` | list | `[]` | Image pull secrets for private registries. |

### Service Account & RBAC

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `ui.serviceAccount.create` | bool | `true` | Create a service account for the UI. |
| `ui.serviceAccount.annotations` | object | `{}` | Annotations for the UI service account. |
| `ui.rbac.create` | bool | `true` | Create ClusterRole and ClusterRoleBinding for K8s API access. |

When `ui.rbac.create=true`, the generated ClusterRole includes the following permissions:

| API Group | Resources | Verbs |
|-----------|-----------|-------|
| `acko.io` | `aerospikeclusters`, `aerospikeclustertemplates` | get, list, watch, create, update, patch, delete |
| `acko.io` | `aerospikeclusters/status` | get |
| `acko.io` | `aerospikeclustertemplates/status` | get |
| `""` (core) | `pods`, `services`, `persistentvolumeclaims` | delete, get, list, watch |
| `""` (core) | `pods/log` | get |
| `""` (core) | `configmaps` | get, list, watch |
| `""` (core) | `secrets` | list |
| `""` (core) | `persistentvolumes` | get, list |
| `""` (core) | `nodes` | get, list |
| `""` (core) | `events` | get, list, watch |
| `""` (core) | `namespaces` | create, list |
| `storage.k8s.io` | `storageclasses` | list |
| `autoscaling` | `horizontalpodautoscalers` | get, list, watch, create, update, patch, delete |

### Service

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `ui.service.type` | string | `ClusterIP` | Service type: `ClusterIP`, `NodePort`, or `LoadBalancer`. |
| `ui.service.frontendPort` | int | `3000` | Frontend port (Next.js web UI). |
| `ui.service.backendPort` | int | `8000` | Backend port (FastAPI REST API). |
| `ui.service.annotations` | object | `{}` | Annotations for the UI Service. |

### Ingress

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `ui.ingress.enabled` | bool | `false` | Enable ingress for external access. |
| `ui.ingress.className` | string | `""` | Ingress class name. |
| `ui.ingress.annotations` | object | `{}` | Ingress annotations. |
| `ui.ingress.hosts` | list | See values.yaml | Ingress host rules. |
| `ui.ingress.tls` | list | `[]` | Ingress TLS configuration. |

### PostgreSQL (Embedded Sidecar)

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `ui.postgresql.enabled` | bool | `true` | Deploy an embedded PostgreSQL sidecar container. Disable to use an external PostgreSQL instance. |
| `ui.postgresql.image.repository` | string | `postgres` | PostgreSQL container image. |
| `ui.postgresql.image.tag` | string | `"17-alpine"` | PostgreSQL image tag. |
| `ui.postgresql.image.pullPolicy` | string | `IfNotPresent` | Image pull policy. |
| `ui.postgresql.database` | string | `aerospike_manager` | Database name. |
| `ui.postgresql.username` | string | `aerospike` | Database user. |
| `ui.postgresql.password` | string | `aerospike` | Database password (embedded sidecar only). |
| `ui.postgresql.existingSecret` | string | `""` | Existing Secret name containing `POSTGRES_PASSWORD` and `DATABASE_URL` keys. |
| `ui.postgresql.resources.requests.cpu` | string | `50m` | CPU request. |
| `ui.postgresql.resources.requests.memory` | string | `128Mi` | Memory request. |
| `ui.postgresql.resources.limits.cpu` | string | `250m` | CPU limit. |
| `ui.postgresql.resources.limits.memory` | string | `256Mi` | Memory limit. |

The embedded PostgreSQL sidecar includes a **startup probe** that runs `pg_isready` to verify database readiness before the UI container begins accepting traffic. This prevents the backend from attempting database connections before PostgreSQL has finished initialization.

### Persistence

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `ui.persistence.enabled` | bool | `true` | Enable persistent storage for the embedded PostgreSQL database. |
| `ui.persistence.storageClassName` | string | `""` | Storage class name (empty = default). |
| `ui.persistence.accessMode` | string | `ReadWriteOnce` | Access mode. |
| `ui.persistence.size` | string | `1Gi` | Volume size. |

### Deployment Strategy & Graceful Shutdown

The UI Deployment uses an explicit update strategy based on the PostgreSQL configuration:

- **With embedded PostgreSQL** (`ui.postgresql.enabled=true`): Uses `Recreate` strategy because the PVC can only be mounted by one pod at a time.
- **Without embedded PostgreSQL** (`ui.postgresql.enabled=false`): Uses `RollingUpdate` strategy with `maxSurge: 1` and `maxUnavailable: 0` for zero-downtime deployments.

The UI container includes a **preStop lifecycle hook** (`sleep 5`) to allow in-flight requests to complete before the pod is terminated. Combined with `terminationGracePeriodSeconds` (default: 45), this ensures graceful shutdown during rollouts and node drains.

### K8s Cluster Management

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `ui.k8s.enabled` | bool | `true` | Enable Kubernetes cluster management features (Create Cluster). |

### UI Resources

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `ui.resources.requests.cpu` | string | `100m` | CPU request. |
| `ui.resources.requests.memory` | string | `256Mi` | Memory request. |
| `ui.resources.limits.cpu` | string | `200m` | CPU limit. |
| `ui.resources.limits.memory` | string | `512Mi` | Memory limit. |

### Security Context

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `ui.podSecurityContext.runAsNonRoot` | bool | `true` | Run pod as non-root. |
| `ui.podSecurityContext.runAsUser` | int | `1001` | User ID. |
| `ui.podSecurityContext.fsGroup` | int | `1001` | Filesystem group ID. |
| `ui.podSecurityContext.seccompProfile.type` | string | `RuntimeDefault` | Seccomp profile type. |
| `ui.securityContext.allowPrivilegeEscalation` | bool | `false` | Disallow privilege escalation. |
| `ui.securityContext.readOnlyRootFilesystem` | bool | `false` | Read-only root filesystem. |
| `ui.securityContext.capabilities.drop` | list | `["ALL"]` | Drop all Linux capabilities. |

### Probes

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `ui.livenessProbe.httpGet.path` | string | `/api/health` | Liveness probe path. |
| `ui.livenessProbe.httpGet.port` | string | `backend` | Liveness probe port. |
| `ui.livenessProbe.initialDelaySeconds` | int | `15` | Initial delay. |
| `ui.livenessProbe.periodSeconds` | int | `20` | Check period. |
| `ui.livenessProbe.timeoutSeconds` | int | `5` | Timeout. |
| `ui.readinessProbe.httpGet.path` | string | `/api/health` | Readiness probe path. |
| `ui.readinessProbe.httpGet.port` | string | `backend` | Readiness probe port. |
| `ui.readinessProbe.initialDelaySeconds` | int | `5` | Initial delay. |
| `ui.readinessProbe.periodSeconds` | int | `10` | Check period. |
| `ui.readinessProbe.timeoutSeconds` | int | `5` | Timeout. |
| `ui.startupProbe.httpGet.path` | string | `/api/health` | Startup probe path. |
| `ui.startupProbe.httpGet.port` | string | `backend` | Startup probe port. |
| `ui.startupProbe.periodSeconds` | int | `5` | Check period. |
| `ui.startupProbe.timeoutSeconds` | int | `3` | Timeout. |
| `ui.startupProbe.failureThreshold` | int | `30` | Max failures before giving up (allows 150s startup). |

### Environment

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `ui.env.frontendPort` | string | (from `ui.service.frontendPort`) | Frontend port injected into the ConfigMap as `FRONTEND_PORT`. Automatically derived from the service port configuration. |
| `ui.env.backendPort` | string | (from `ui.service.backendPort`) | Backend port injected into the ConfigMap as `BACKEND_PORT`. Automatically derived from the service port configuration. |
| `ui.env.corsOrigins` | string | `""` | Backend CORS origins. Empty means no CORS (frontend proxies via Next.js rewrites). |
| `ui.env.logLevel` | string | `"INFO"` | Log level: `DEBUG`, `INFO`, `WARNING`, `ERROR`. |
| `ui.env.logFormat` | string | `"text"` | Log format: `text` for human-readable, `json` for structured logging. |
| `ui.env.databaseUrl` | string | `""` | External PostgreSQL connection URL. Only used when `postgresql.enabled` is `false`. |
| `ui.env.dbPoolSize` | int | `5` | DB connection pool size. |
| `ui.env.dbPoolOverflow` | int | `10` | Max overflow connections beyond pool size. |
| `ui.env.dbPoolTimeout` | int | `30` | Pool checkout timeout in seconds. |
| `ui.env.k8sApiTimeout` | int | `30` | Kubernetes API request timeout in seconds. |

### UI Monitoring

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `ui.metrics.serviceMonitor.enabled` | bool | `false` | Create a ServiceMonitor for the UI backend metrics endpoint. |
| `ui.metrics.serviceMonitor.interval` | string | `30s` | Scrape interval. |
| `ui.metrics.serviceMonitor.scrapeTimeout` | string | `10s` | Scrape timeout. |
| `ui.metrics.serviceMonitor.labels` | object | `{}` | Additional labels for ServiceMonitor discovery. |

The UI ServiceMonitor scrapes the backend metrics endpoint at `/api/metrics`. This path is explicitly set in the ServiceMonitor template to ensure Prometheus collects application-level metrics correctly.

### UI Scheduling

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `ui.nodeSelector` | object | `{}` | Node selector for UI pods. |
| `ui.tolerations` | list | `[]` | Tolerations for UI pods. |
| `ui.affinity` | object | `{}` | Affinity rules for UI pods. |
| `ui.topologySpreadConstraints` | list | `[]` | Topology spread constraints for UI pods. |
| `ui.podAnnotations` | object | `{}` | Additional annotations for UI pods. |
| `ui.podLabels` | object | `{}` | Additional labels for UI pods. |
| `ui.terminationGracePeriodSeconds` | int | `45` | Termination grace period in seconds. |

### UI Aerospike Ports

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `ui.aerospikePorts.service` | int | `3000` | Aerospike service port. |
| `ui.aerospikePorts.fabric` | int | `3001` | Aerospike fabric port. |
| `ui.aerospikePorts.heartbeat` | int | `3002` | Aerospike heartbeat port. |

### UI Network Policy

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `ui.networkPolicy.enabled` | bool | `false` | Enable NetworkPolicy for restricting UI traffic. |
| `ui.networkPolicy.ingressFrom` | list | `[]` | Optional ingress source restrictions. |

### UI Pod Disruption Budget

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `ui.podDisruptionBudget.enabled` | bool | `false` | Enable PDB for UI pods. |
| `ui.podDisruptionBudget.minAvailable` | int | `1` | Minimum available pods. |
| `ui.podDisruptionBudget.maxUnavailable` | int | — | Maximum unavailable pods. |

### UI Autoscaling

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `ui.autoscaling.enabled` | bool | `false` | Enable HPA for UI. |
| `ui.autoscaling.minReplicas` | int | `1` | Minimum replicas. |
| `ui.autoscaling.maxReplicas` | int | `3` | Maximum replicas. |
| `ui.autoscaling.targetCPUUtilizationPercentage` | int | `80` | Target CPU utilization. |
| `ui.autoscaling.targetMemoryUtilizationPercentage` | int | — | Target memory utilization (optional). |

### Extra Environment Variables

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `ui.extraEnv` | list | `[]` | Extra environment variables for the UI container. Supports standard Kubernetes env var syntax including `valueFrom` references. |

### UI Helm Tests

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `ui.tests.enabled` | bool | `true` | Enable Helm test pods for UI (run with `helm test <release>`). |

## Default AerospikeClusterTemplates

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `defaultTemplates.enabled` | bool | `true` | Create pre-built AerospikeClusterTemplate resources (minimal, soft-rack, hard-rack). Templates are cluster-scoped and accessible from all namespaces. |

The three default template tiers are configured under `defaultTemplates.templates.minimal`, `defaultTemplates.templates.soft-rack`, and `defaultTemplates.templates.hard-rack`. See [Template Management](./templates.md) for details on each tier.
