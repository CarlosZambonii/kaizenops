{{/*
Nome base do chart (sem o release), usado para compor o nome dos recursos.
*/}}
{{- define "kaizenops.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Nome completo do release, usado como prefixo de todos os recursos do chart.
*/}}
{{- define "kaizenops.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
String de versão do chart, usada no label helm.sh/chart.
*/}}
{{- define "kaizenops.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Labels comuns aplicados a todos os recursos do chart.
*/}}
{{- define "kaizenops.labels" -}}
helm.sh/chart: {{ include "kaizenops.chart" . }}
{{ include "kaizenops.selectorLabels" . }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{/*
Selector labels comuns (sem o componente).
*/}}
{{- define "kaizenops.selectorLabels" -}}
app.kubernetes.io/name: {{ include "kaizenops.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{/*
Labels completos de um componente específico (collector, spc ou ml).
Uso: {{ include "kaizenops.componentLabels" (dict "ctx" $ "component" "collector") }}
*/}}
{{- define "kaizenops.componentLabels" -}}
{{ include "kaizenops.labels" .ctx }}
app.kubernetes.io/component: {{ .component }}
{{- end -}}

{{/*
Selector labels de um componente específico (collector, spc ou ml).
Uso: {{ include "kaizenops.componentSelectorLabels" (dict "ctx" $ "component" "collector") }}
*/}}
{{- define "kaizenops.componentSelectorLabels" -}}
{{ include "kaizenops.selectorLabels" .ctx }}
app.kubernetes.io/component: {{ .component }}
{{- end -}}
