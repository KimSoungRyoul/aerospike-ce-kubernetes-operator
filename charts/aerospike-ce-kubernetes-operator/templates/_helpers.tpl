{{/*
Expand the name of the chart.
*/}}
{{- define "aerospike-ce-kubernetes-operator.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "aerospike-ce-kubernetes-operator.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "aerospike-ce-kubernetes-operator.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels applied to all resources.
*/}}
{{- define "aerospike-ce-kubernetes-operator.labels" -}}
helm.sh/chart: {{ include "aerospike-ce-kubernetes-operator.chart" . }}
{{ include "aerospike-ce-kubernetes-operator.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels used for pod selection.
*/}}
{{- define "aerospike-ce-kubernetes-operator.selectorLabels" -}}
app.kubernetes.io/name: {{ include "aerospike-ce-kubernetes-operator.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
control-plane: controller-manager
{{- end }}

{{/*
Create the name of the service account to use.
*/}}
{{- define "aerospike-ce-kubernetes-operator.serviceAccountName" -}}
{{- include "aerospike-ce-kubernetes-operator.fullname" . }}
{{- end }}

{{/*
Webhook service name.
*/}}
{{- define "aerospike-ce-kubernetes-operator.webhookServiceName" -}}
{{- include "aerospike-ce-kubernetes-operator.fullname" . }}-webhook
{{- end }}

{{/*
Metrics service name.
*/}}
{{- define "aerospike-ce-kubernetes-operator.metricsServiceName" -}}
{{- include "aerospike-ce-kubernetes-operator.fullname" . }}-metrics
{{- end }}

{{/*
Cert-manager issuer name.
*/}}
{{- define "aerospike-ce-kubernetes-operator.issuerName" -}}
{{- include "aerospike-ce-kubernetes-operator.fullname" . }}-selfsigned-issuer
{{- end }}

{{/*
Cert-manager certificate secret name.
*/}}
{{- define "aerospike-ce-kubernetes-operator.certSecretName" -}}
{{- if and .Values.certManager.enabled .Values.webhook.enabled }}
{{- include "aerospike-ce-kubernetes-operator.fullname" . }}-webhook-cert
{{- else if .Values.webhookTlsSecret }}
{{- .Values.webhookTlsSecret }}
{{- else }}
{{- include "aerospike-ce-kubernetes-operator.fullname" . }}-webhook-cert
{{- end }}
{{- end }}

{{/*
Cert-manager certificate name.
*/}}
{{- define "aerospike-ce-kubernetes-operator.certName" -}}
{{- include "aerospike-ce-kubernetes-operator.fullname" . }}-serving-cert
{{- end }}

{{/*
Container image with tag.
*/}}
{{- define "aerospike-ce-kubernetes-operator.image" -}}
{{- printf "%s:%s" .Values.image.repository (default .Chart.AppVersion .Values.image.tag) }}
{{- end }}

{{/*
Pod labels combining selector labels with user-defined pod labels.
*/}}
{{- define "aerospike-ce-kubernetes-operator.podLabels" -}}
{{ include "aerospike-ce-kubernetes-operator.selectorLabels" . }}
{{- with .Values.podLabels }}
{{ toYaml . }}
{{- end }}
{{- end }}

{{/*
Namespace for the release.
*/}}
{{- define "aerospike-ce-kubernetes-operator.namespace" -}}
{{- .Release.Namespace }}
{{- end }}

{{/*
=============================================================================
UI (Aerospike Cluster Manager) helpers
=============================================================================
*/}}

{{/*
UI component name (constant).
*/}}
{{- define "aerospike-ce-kubernetes-operator.ui.name" -}}
aerospike-cluster-manager
{{- end }}

{{/*
UI fully qualified name.
*/}}
{{- define "aerospike-ce-kubernetes-operator.ui.fullname" -}}
{{- include "aerospike-ce-kubernetes-operator.fullname" . }}-ui
{{- end }}

{{/*
UI common labels.
*/}}
{{- define "aerospike-ce-kubernetes-operator.ui.labels" -}}
helm.sh/chart: {{ include "aerospike-ce-kubernetes-operator.chart" . }}
{{ include "aerospike-ce-kubernetes-operator.ui.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/component: ui
{{- end }}

{{/*
UI selector labels.
*/}}
{{- define "aerospike-ce-kubernetes-operator.ui.selectorLabels" -}}
app.kubernetes.io/name: {{ include "aerospike-ce-kubernetes-operator.ui.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
UI service account name.
*/}}
{{- define "aerospike-ce-kubernetes-operator.ui.serviceAccountName" -}}
{{- if .Values.ui.serviceAccount.create }}
{{- include "aerospike-ce-kubernetes-operator.ui.fullname" . }}
{{- else }}
{{- "default" }}
{{- end }}
{{- end }}

{{/*
UI component-specific fullnames (api / web).
*/}}
{{- define "aerospike-ce-kubernetes-operator.ui.api.fullname" -}}
{{- include "aerospike-ce-kubernetes-operator.ui.fullname" . }}-api
{{- end }}

{{- define "aerospike-ce-kubernetes-operator.ui.web.fullname" -}}
{{- include "aerospike-ce-kubernetes-operator.ui.fullname" . }}-web
{{- end }}

{{/*
UI component-specific selector labels (adds app.kubernetes.io/component override).
*/}}
{{- define "aerospike-ce-kubernetes-operator.ui.api.selectorLabels" -}}
{{ include "aerospike-ce-kubernetes-operator.ui.selectorLabels" . }}
app.kubernetes.io/component: ui-api
{{- end }}

{{- define "aerospike-ce-kubernetes-operator.ui.web.selectorLabels" -}}
{{ include "aerospike-ce-kubernetes-operator.ui.selectorLabels" . }}
app.kubernetes.io/component: ui-web
{{- end }}

{{/*
Chart-managed PostgreSQL (ui.database.postgresql.deploy=true) name & labels.
*/}}
{{- define "aerospike-ce-kubernetes-operator.ui.postgres.fullname" -}}
{{- include "aerospike-ce-kubernetes-operator.ui.fullname" . }}-postgres
{{- end }}

{{- define "aerospike-ce-kubernetes-operator.ui.postgres.selectorLabels" -}}
{{ include "aerospike-ce-kubernetes-operator.ui.selectorLabels" . }}
app.kubernetes.io/component: ui-postgres
{{- end }}

{{- define "aerospike-ce-kubernetes-operator.ui.postgres.labels" -}}
helm.sh/chart: {{ include "aerospike-ce-kubernetes-operator.chart" . }}
{{ include "aerospike-ce-kubernetes-operator.ui.postgres.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Name of the data PVC for the chart-managed PostgreSQL Deployment
(ui.database.postgresql.deploy=true with persistence.enabled=true).
*/}}
{{- define "aerospike-ce-kubernetes-operator.ui.postgres.pvcName" -}}
{{- include "aerospike-ce-kubernetes-operator.ui.postgres.fullname" . }}-data
{{- end }}

{{/*
Name of the Secret carrying the api's DATABASE_URL.
- existingSecret set -> the operator-supplied Secret (works for an external
  database AND, with deploy=true, as a GitOps-safe alternative to the chart's
  randomly-generated Secret — it then also carries POSTGRES_PASSWORD).
- deploy=true        -> the chart's own "<ui.fullname>-postgres" Secret.
- deploy=false       -> the chart's "<ui.fullname>-db" Secret built from
  databaseUrl.
*/}}
{{- define "aerospike-ce-kubernetes-operator.ui.database.secretName" -}}
{{- if .Values.ui.database.postgresql.existingSecret -}}
{{- .Values.ui.database.postgresql.existingSecret -}}
{{- else if .Values.ui.database.postgresql.deploy -}}
{{- include "aerospike-ce-kubernetes-operator.ui.postgres.fullname" . -}}
{{- else -}}
{{- printf "%s-db" (include "aerospike-ce-kubernetes-operator.ui.fullname" .) -}}
{{- end -}}
{{- end }}

{{/*
UI component-specific common labels.
*/}}
{{- define "aerospike-ce-kubernetes-operator.ui.api.labels" -}}
helm.sh/chart: {{ include "aerospike-ce-kubernetes-operator.chart" . }}
{{ include "aerospike-ce-kubernetes-operator.ui.api.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "aerospike-ce-kubernetes-operator.ui.web.labels" -}}
helm.sh/chart: {{ include "aerospike-ce-kubernetes-operator.chart" . }}
{{ include "aerospike-ce-kubernetes-operator.ui.web.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
UI image helpers (per-component).
Tag resolution order:
  1. ui.api.image.tag / ui.web.image.tag (per-component override)
  2. ui.imageTag (shared cluster-manager release, pinned in values.yaml)
  3. .Chart.AppVersion (last resort)
aerospike-cluster-manager is versioned independently from the operator, so
falling straight through to .Chart.AppVersion can resolve to a tag that
does not exist in ghcr.io. ui.imageTag is the intended default knob.
*/}}
{{- define "aerospike-ce-kubernetes-operator.ui.imageTag" -}}
{{- default .Chart.AppVersion .Values.ui.imageTag | toString -}}
{{- end }}

{{- define "aerospike-ce-kubernetes-operator.ui.api.image" -}}
{{- $tag := default (include "aerospike-ce-kubernetes-operator.ui.imageTag" .) .Values.ui.api.image.tag -}}
{{- printf "%s:%s" .Values.ui.api.image.repository ($tag | toString) -}}
{{- end }}

{{- define "aerospike-ce-kubernetes-operator.ui.web.image" -}}
{{- $tag := default (include "aerospike-ce-kubernetes-operator.ui.imageTag" .) .Values.ui.web.image.tag -}}
{{- printf "%s:%s" .Values.ui.web.image.repository ($tag | toString) -}}
{{- end }}

{{/*
UI component enablement.

ui.api.enabled and ui.web.enabled are independent toggles. Set both to
false to skip the UI entirely (operator-only install). Combinations:

  api.enabled=true,  web.enabled=true   (default) → both Deployments
  api.enabled=true,  web.enabled=false           → API only (Swagger / external UI)
  api.enabled=false, web.enabled=true            → Web only (point web.env.apiUrl at an external API)
  api.enabled=false, web.enabled=false           → operator only (no UI resources)

ui.anyEnabled returns "true" when at least one of api/web is on. Used
to gate UI-shared resources (configmap, serviceaccount, ingress,
networkpolicy) that are pointless without any UI Deployment.
*/}}
{{- define "aerospike-ce-kubernetes-operator.ui.api.enabled" -}}
{{- .Values.ui.api.enabled -}}
{{- end }}

{{- define "aerospike-ce-kubernetes-operator.ui.web.enabled" -}}
{{- .Values.ui.web.enabled -}}
{{- end }}

{{- define "aerospike-ce-kubernetes-operator.ui.anyEnabled" -}}
{{- or .Values.ui.api.enabled .Values.ui.web.enabled -}}
{{- end }}

{{/*
UI service names (used by web → api routing and by NOTES output).
*/}}
{{- define "aerospike-ce-kubernetes-operator.ui.api.serviceName" -}}
{{- include "aerospike-ce-kubernetes-operator.ui.api.fullname" . -}}
{{- end }}

{{- define "aerospike-ce-kubernetes-operator.ui.web.serviceName" -}}
{{- include "aerospike-ce-kubernetes-operator.ui.web.fullname" . -}}
{{- end }}

{{/*
=============================================================================
Operator / multi-cluster / OIDC enablement helpers
=============================================================================
Each helper renders the literal string "true" / "false" so callers can guard
templates with `eq (include "...") "true"`.
*/}}

{{- define "aerospike-ce-kubernetes-operator.operator.enabled" -}}
{{- $op := .Values.operator | default dict -}}
{{- if hasKey $op "enabled" -}}
{{- $op.enabled -}}
{{- else -}}
true
{{- end -}}
{{- end }}

{{- define "aerospike-ce-kubernetes-operator.multiCluster.enabled" -}}
{{- $mc := .Values.multiCluster | default dict -}}
{{- if hasKey $mc "enabled" -}}
{{- $mc.enabled -}}
{{- else -}}
false
{{- end -}}
{{- end }}

{{- define "aerospike-ce-kubernetes-operator.api.oidc.enabled" -}}
{{- $oidc := dig "api" "auth" "oidc" "enabled" false .Values.ui -}}
{{- $oidc -}}
{{- end }}

{{- define "aerospike-ce-kubernetes-operator.web.oidc.enabled" -}}
{{- $oidc := dig "web" "auth" "oidc" "enabled" false .Values.ui -}}
{{- $oidc -}}
{{- end }}

{{/*
ConfigMap names for the multi-cluster registry and SPA OIDC config.
*/}}
{{- define "aerospike-ce-kubernetes-operator.clusterRegistryConfigMapName" -}}
{{- include "aerospike-ce-kubernetes-operator.fullname" . }}-cluster-registry
{{- end }}

{{- define "aerospike-ce-kubernetes-operator.webOidcConfigMapName" -}}
{{- include "aerospike-ce-kubernetes-operator.fullname" . }}-web-oidc-config
{{- end }}

{{/*
=============================================================================
Validation gates (called from templates/_validations.tpl via NOTES.txt)
=============================================================================

These gates fail-fast at `helm template` / `helm install` time when the
values combination is contradictory. Each gate is a no-op when its
preconditions are not met, so the helper is safe to call unconditionally.
*/}}

{{- define "aerospike-ce-kubernetes-operator.validate.multiClusterVsApiUrl" -}}
{{- if eq (include "aerospike-ce-kubernetes-operator.ui.web.enabled" .) "true" -}}
{{- if and (eq (include "aerospike-ce-kubernetes-operator.multiCluster.enabled" .) "true") .Values.ui.web.env.apiUrl -}}
{{- fail "multiCluster.enabled=true conflicts with ui.web.env.apiUrl: routing is ambiguous. Either disable multi-cluster (and use ui.web.env.apiUrl for a single API) or clear ui.web.env.apiUrl and rely on the cluster registry." -}}
{{- end -}}
{{- end -}}
{{- end }}

{{- define "aerospike-ce-kubernetes-operator.validate.defaultTemplatesNeedCRDs" -}}
{{- if and .Values.defaultTemplates.enabled (not .Values.crds.install) -}}
{{- fail "defaultTemplates.enabled=true requires crds.install=true (the AerospikeClusterTemplate CRD must exist before templates can be applied). Either enable crds.install or set defaultTemplates.enabled=false." -}}
{{- end -}}
{{- end }}

{{- define "aerospike-ce-kubernetes-operator.validate.apiOidcIssuerUrl" -}}
{{- if eq (include "aerospike-ce-kubernetes-operator.ui.api.enabled" .) "true" -}}
{{- if eq (include "aerospike-ce-kubernetes-operator.api.oidc.enabled" .) "true" -}}
{{- if not .Values.ui.api.auth.oidc.issuerUrl -}}
{{- fail "ui.api.auth.oidc.enabled=true requires ui.api.auth.oidc.issuerUrl to be set (e.g. https://keycloak.example.com/realms/acko)." -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- end }}

{{- define "aerospike-ce-kubernetes-operator.validate.webOidcClientId" -}}
{{- if eq (include "aerospike-ce-kubernetes-operator.ui.web.enabled" .) "true" -}}
{{- if eq (include "aerospike-ce-kubernetes-operator.web.oidc.enabled" .) "true" -}}
{{- if not .Values.ui.web.auth.oidc.clientId -}}
{{- fail "ui.web.auth.oidc.enabled=true requires ui.web.auth.oidc.clientId to be set (the SPA's public client ID registered in the IdP)." -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- end }}

{{- define "aerospike-ce-kubernetes-operator.validate.caSecretName" -}}
{{- if and .Values.certManager.enabled (eq .Values.certManager.issuer.type "ca") (not .Values.certManager.issuer.caSecretName) -}}
{{- fail "certManager.issuer.type=ca requires certManager.issuer.caSecretName to be set (the existing Secret in .Release.Namespace that holds tls.crt + tls.key for the CA Issuer)." -}}
{{- end -}}
{{- end }}

{{- define "aerospike-ce-kubernetes-operator.validate.webhookTlsSource" -}}
{{- if and .Values.webhook.enabled (not .Values.certManager.enabled) (not .Values.webhookTlsSecret) -}}
{{- fail "webhook.enabled=true requires either certManager.enabled=true (auto-provision) or webhookTlsSecret to be set (manually provided Secret with tls.crt + tls.key). Otherwise the webhook server has no serving certificate and the apiserver rejects every admission call." -}}
{{- end -}}
{{- end }}

{{/*
Database backend validation.
The embedded PostgreSQL sidecar was removed: the api now runs on an embedded
SQLite file (default) or connects to an EXTERNAL PostgreSQL. Fail fast on
stale sidecar-era value keys and on invalid backend combinations.
*/}}
{{- define "aerospike-ce-kubernetes-operator.validate.databaseConfig" -}}
{{- if hasKey .Values.ui "postgresql" -}}
{{- fail "ui.postgresql.* has been removed: the embedded PostgreSQL sidecar is no longer shipped. Set ui.database.type=sqlite (embedded, default) or ui.database.type=postgresql with ui.database.postgresql.databaseUrl / existingSecret pointing at an EXTERNAL database. See the chart README 'Database' section for the migration table." -}}
{{- end -}}
{{- if hasKey .Values.ui "persistence" -}}
{{- fail "ui.persistence.* has been renamed: SQLite persistence now lives under ui.database.sqlite.persistence.*" -}}
{{- end -}}
{{- if hasKey .Values.ui.env "databaseUrl" -}}
{{- fail "ui.env.databaseUrl has moved to ui.database.postgresql.databaseUrl (and requires ui.database.type=postgresql)." -}}
{{- end -}}
{{- /* The backend enum is validated unconditionally: configmap.yaml renders
       ENABLE_POSTGRES from ui.database.type even in web-only installs, so a
       typo must not slip through (it would silently fall back to SQLite). */ -}}
{{- $type := .Values.ui.database.type -}}
{{- if not (has $type (list "sqlite" "postgresql")) -}}
{{- fail (printf "ui.database.type must be \"sqlite\" or \"postgresql\", got %q." $type) -}}
{{- end -}}
{{- /* Upgrade safety: chart versions before this one ran an embedded PostgreSQL
       sidecar whose database lived in the "<ui.fullname>-db" Secret (carrying a
       POSTGRES_PASSWORD key) + "<ui.fullname>-data" PVC. That sidecar is gone
       and there is NO automatic data migration. Detect the leftover
       embedded-mode Secret and refuse the upgrade until the operator
       acknowledges it (after backing the data up). `lookup` is empty on fresh
       installs and on `helm template`, so this fires only on a real upgrade. */ -}}
{{- if not .Values.ui.database.acknowledgeEmbeddedPostgresRemoval -}}
{{- $dbSecretName := printf "%s-db" (include "aerospike-ce-kubernetes-operator.ui.fullname" .) -}}
{{- $dbSecret := lookup "v1" "Secret" .Release.Namespace $dbSecretName -}}
{{- if and $dbSecret (hasKey (default (dict) $dbSecret.data) "POSTGRES_PASSWORD") -}}
{{- fail (printf "Upgrade blocked: Secret %q carries a POSTGRES_PASSWORD key, so this release previously ran the embedded PostgreSQL sidecar (removed in this chart version). Upgrading does NOT migrate that data -- with ui.database.type=sqlite the old database is stranded on the PVC, with type=postgresql the PVC is deleted. Back it up first (pg_dump the embedded database), restore it into your chosen backend, then re-run with ui.database.acknowledgeEmbeddedPostgresRemoval=true. See the chart README 'Database' migration section." $dbSecretName) -}}
{{- end -}}
{{- end -}}
{{- /* Upgrade safety: the chart-managed PostgreSQL shipped as a StatefulSet
       through chart v1.6.0; it now runs as a Deployment whose data PVC has a
       different name. A `helm upgrade` would strand the StatefulSet's volume
       and start the Deployment on a fresh, EMPTY database. Detect the leftover
       StatefulSet and refuse the upgrade until the operator acknowledges it
       (after backing the data up). `lookup` is empty on fresh installs and on
       `helm template`, so this fires only on a real in-cluster upgrade. */ -}}
{{- if and .Values.ui.database.postgresql.deploy (not .Values.ui.database.postgresql.acknowledgeStatefulSetMigration) -}}
{{- $pgName := include "aerospike-ce-kubernetes-operator.ui.postgres.fullname" . -}}
{{- if lookup "apps/v1" "StatefulSet" .Release.Namespace $pgName -}}
{{- fail (printf "Upgrade blocked: StatefulSet %q is the chart-managed PostgreSQL from chart v1.6.0 or earlier. This chart version runs it as a Deployment whose data PVC is named %q, while the StatefulSet's volume is %q -- upgrading would start an EMPTY database and strand that old volume. Back the data up (pg_dump), then set ui.database.postgresql.acknowledgeStatefulSetMigration=true to proceed and restore afterwards. See the chart README 'Database' migration section." $pgName (include "aerospike-ce-kubernetes-operator.ui.postgres.pvcName" .) (printf "data-%s-0" $pgName)) -}}
{{- end -}}
{{- end -}}
{{- /* The remaining checks only matter when the api Deployment is rendered. */ -}}
{{- if eq (include "aerospike-ce-kubernetes-operator.ui.api.enabled" .) "true" -}}
{{- if eq $type "postgresql" -}}
{{- if .Values.ui.database.postgresql.deploy -}}
{{- /* Chart-managed PostgreSQL: the chart provisions the database (Deployment
       + Service + data PVC). databaseUrl is an INLINE external connection URL
       and never applies here. existingSecret IS allowed — it lets an operator
       supply a pre-baked Secret (sealed-secret / vault) carrying
       POSTGRES_PASSWORD + DATABASE_URL instead of the chart's
       randomly-generated one. That is the GitOps-safe path: a client-side
       `helm template` (ArgoCD / Flux / kustomize) cannot preserve a random
       password across renders, so it would otherwise regenerate it every
       reconcile. */ -}}
{{- if .Values.ui.database.postgresql.databaseUrl -}}
{{- fail "ui.database.postgresql.deploy=true provisions a chart-managed PostgreSQL and builds DATABASE_URL itself — unset ui.database.postgresql.databaseUrl (it applies only to an external database). To supply credentials yourself, use ui.database.postgresql.existingSecret instead." -}}
{{- end -}}
{{- if .Values.ui.database.postgresql.existingSecret -}}
{{- /* The operator-supplied Secret carries the password, so auth.password
       would be silently ignored — reject the ambiguous combination. */ -}}
{{- if .Values.ui.database.postgresql.auth.password -}}
{{- fail "ui.database.postgresql.deploy=true with existingSecret reads the password from that Secret — unset ui.database.postgresql.auth.password (it would be ignored). The existingSecret must carry both POSTGRES_PASSWORD and a DATABASE_URL pointing at the chart's in-cluster PostgreSQL Service." -}}
{{- end -}}
{{- else -}}
{{- /* Chart-generated Secret: the password is embedded verbatim in
       DATABASE_URL; reject characters that would corrupt the connection
       string (the auto-generated default is always safe). */ -}}
{{- if and .Values.ui.database.postgresql.auth.password (not (regexMatch "^[A-Za-z0-9._~-]+$" .Values.ui.database.postgresql.auth.password)) -}}
{{- fail "ui.database.postgresql.auth.password contains a character outside [A-Za-z0-9._~-]. The chart embeds it verbatim in DATABASE_URL, so URL-special characters (@ : / ? # ...) would corrupt the connection string. Use a password from that set, or leave it empty to auto-generate one." -}}
{{- end -}}
{{- end -}}
{{- else -}}
{{- /* External PostgreSQL: a connection URL is mandatory. */ -}}
{{- if and (not .Values.ui.database.postgresql.databaseUrl) (not .Values.ui.database.postgresql.existingSecret) -}}
{{- fail "ui.database.type=postgresql requires either ui.database.postgresql.databaseUrl or ui.database.postgresql.existingSecret for an external database — or set ui.database.postgresql.deploy=true to let the chart provision a PostgreSQL StatefulSet." -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- if and (eq $type "sqlite") .Values.ui.database.postgresql.deploy -}}
{{- fail "ui.database.postgresql.deploy=true has no effect with ui.database.type=sqlite. Set ui.database.type=postgresql to deploy the chart-managed PostgreSQL, or unset the deploy flag." -}}
{{- end -}}
{{- if and (eq $type "sqlite") (gt (int .Values.ui.replicaCount) 1) -}}
{{- fail "ui.database.type=sqlite is single-writer and is incompatible with ui.replicaCount > 1. Switch to ui.database.type=postgresql with an external database for multi-replica / HA deployments." -}}
{{- end -}}
{{- end -}}
{{- end }}

{{/*
Aggregate validation entry-point. Templates can `include` this helper once
(typically from NOTES.txt or a dedicated _validations partial) to enforce
all gates uniformly.
*/}}
{{- define "aerospike-ce-kubernetes-operator.validate" -}}
{{- include "aerospike-ce-kubernetes-operator.validate.multiClusterVsApiUrl" . -}}
{{- include "aerospike-ce-kubernetes-operator.validate.defaultTemplatesNeedCRDs" . -}}
{{- include "aerospike-ce-kubernetes-operator.validate.apiOidcIssuerUrl" . -}}
{{- include "aerospike-ce-kubernetes-operator.validate.webOidcClientId" . -}}
{{- include "aerospike-ce-kubernetes-operator.validate.caSecretName" . -}}
{{- include "aerospike-ce-kubernetes-operator.validate.webhookTlsSource" . -}}
{{- include "aerospike-ce-kubernetes-operator.validate.databaseConfig" . -}}
{{- end }}
