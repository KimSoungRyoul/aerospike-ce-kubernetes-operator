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
{{- include "aerospike-ce-kubernetes-operator.fullname" . }}-metrics-service
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
UI container image with tag.
UI is versioned independently from the operator; tag is set explicitly in values.yaml.
*/}}
{{- define "aerospike-ce-kubernetes-operator.ui.image" -}}
{{- printf "%s:%s" .Values.ui.image.repository (.Values.ui.image.tag | toString) -}}
{{- end }}
