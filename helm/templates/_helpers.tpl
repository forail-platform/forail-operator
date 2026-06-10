{{- define "forail-operator.labels" -}}
app.kubernetes.io/name: forail-operator
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: forail-platform
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version }}
{{- end }}

{{- define "forail-operator.selectorLabels" -}}
app.kubernetes.io/name: forail-operator
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{- define "forail-operator.serviceAccountName" -}}
{{- default "forail-operator" .Values.serviceAccount.name }}
{{- end }}

{{- define "forail-operator.tokenSecretName" -}}
{{- if .Values.forail.existingSecret -}}
{{ .Values.forail.existingSecret }}
{{- else -}}
forail-operator-credentials
{{- end -}}
{{- end }}
