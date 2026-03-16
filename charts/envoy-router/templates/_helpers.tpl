{{- define "envoy-router.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "envoy-router.fullname" -}}
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

{{- define "envoy-router.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "envoy-router.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{- define "envoy-router.gatewayNamespace" -}}
{{- default .Release.Namespace .Values.gateway.namespace }}
{{- end }}

{{- define "envoy-router.operatorGatewayNamespace" -}}
{{- default .Release.Namespace .Values.operator.gatewayNamespace }}
{{- end }}
