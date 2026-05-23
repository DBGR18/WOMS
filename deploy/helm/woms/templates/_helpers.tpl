{{- define "woms.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "woms.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name (include "woms.name" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}

{{- define "woms.labels" -}}
app.kubernetes.io/name: {{ include "woms.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{- define "woms.image" -}}
{{- $registry := .registry | default "" | trimSuffix "/" -}}
{{- if $registry -}}
{{- printf "%s/%s:%s" $registry .repository .tag -}}
{{- else -}}
{{- printf "%s:%s" .repository .tag -}}
{{- end -}}
{{- end -}}

{{- define "woms.externalScheme" -}}
{{- ternary "https" "http" .Values.ingress.tls.enabled -}}
{{- end -}}

{{- define "woms.grafanaExternalPath" -}}
{{- $path := default "/grafana" .Values.monitoring.grafana.externalPath | trimSuffix "/" -}}
{{- if hasPrefix "/" $path -}}
{{- $path -}}
{{- else -}}
{{- printf "/%s" $path -}}
{{- end -}}
{{- end -}}

{{- define "woms.grafanaRootUrl" -}}
{{- if .Values.monitoring.grafana.env.rootUrl -}}
{{- .Values.monitoring.grafana.env.rootUrl -}}
{{- else -}}
{{- printf "%s://%s%s/" (include "woms.externalScheme" .) .Values.ingress.host (include "woms.grafanaExternalPath" .) -}}
{{- end -}}
{{- end -}}
