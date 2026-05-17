{{- define "tracing.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "tracing.fullname" -}}
{{- printf "%s-%s" .Release.Name (include "tracing.name" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}
