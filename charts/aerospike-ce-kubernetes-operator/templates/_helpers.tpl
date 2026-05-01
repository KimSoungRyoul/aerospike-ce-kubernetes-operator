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

`ui.enabled` is the master switch (false → no UI resources at all). When
ui.enabled is true, ui.api.enabled / ui.web.enabled act as independent
sub-toggles so an operator can deploy API-only (when an external UI talks
to the FastAPI backend) or Web-only (when ui.web.env.apiUrl points to an
external API instance).

Defaults: both api.enabled and web.enabled are true, so the existing
chart 0.2.x deployment behavior is preserved exactly when only ui.enabled
is set in user values.
*/}}
{{- define "aerospike-ce-kubernetes-operator.ui.api.enabled" -}}
{{- and .Values.ui.enabled .Values.ui.api.enabled -}}
{{- end }}

{{- define "aerospike-ce-kubernetes-operator.ui.web.enabled" -}}
{{- and .Values.ui.enabled .Values.ui.web.enabled -}}
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
