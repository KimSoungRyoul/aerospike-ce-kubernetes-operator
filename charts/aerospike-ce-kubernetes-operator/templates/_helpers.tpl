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
*/}}
{{- define "aerospike-ce-kubernetes-operator.ui.api.image" -}}
{{- printf "%s:%s" .Values.ui.api.image.repository (.Values.ui.api.image.tag | toString) -}}
{{- end }}

{{- define "aerospike-ce-kubernetes-operator.ui.web.image" -}}
{{- printf "%s:%s" .Values.ui.web.image.repository (.Values.ui.web.image.tag | toString) -}}
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
{{- if and (eq (include "aerospike-ce-kubernetes-operator.multiCluster.enabled" .) "true") .Values.ui.web.env.apiUrl -}}
{{- fail "multiCluster.enabled=true conflicts with ui.web.env.apiUrl: routing is ambiguous. Either disable multi-cluster (and use ui.web.env.apiUrl for a single API) or clear ui.web.env.apiUrl and rely on the cluster registry." -}}
{{- end -}}
{{- end }}

{{- define "aerospike-ce-kubernetes-operator.validate.defaultTemplatesNeedCRDs" -}}
{{- if and .Values.defaultTemplates.enabled (not .Values.crds.install) -}}
{{- fail "defaultTemplates.enabled=true requires crds.install=true (the AerospikeClusterTemplate CRD must exist before templates can be applied). Either enable crds.install or set defaultTemplates.enabled=false." -}}
{{- end -}}
{{- end }}

{{- define "aerospike-ce-kubernetes-operator.validate.apiOidcIssuerUrl" -}}
{{- if eq (include "aerospike-ce-kubernetes-operator.api.oidc.enabled" .) "true" -}}
{{- if not .Values.ui.api.auth.oidc.issuerUrl -}}
{{- fail "ui.api.auth.oidc.enabled=true requires ui.api.auth.oidc.issuerUrl to be set (e.g. https://keycloak.example.com/realms/acko)." -}}
{{- end -}}
{{- end -}}
{{- end }}

{{- define "aerospike-ce-kubernetes-operator.validate.webOidcClientId" -}}
{{- if eq (include "aerospike-ce-kubernetes-operator.web.oidc.enabled" .) "true" -}}
{{- if not .Values.ui.web.auth.oidc.clientId -}}
{{- fail "ui.web.auth.oidc.enabled=true requires ui.web.auth.oidc.clientId to be set (the SPA's public client ID registered in the IdP)." -}}
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
{{- end }}
