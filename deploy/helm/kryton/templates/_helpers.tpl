{{- define "kryton.name" -}}kryton{{- end }}
{{- define "kryton.fullname" -}}{{ .Release.Name }}{{- end }}
{{- define "kryton.serviceAccountName" -}}{{- if .Values.serviceAccount.name -}}{{ .Values.serviceAccount.name }}{{- else -}}{{ include "kryton.fullname" . }}{{- end -}}{{- end }}
{{- define "kryton.labels" -}}
app.kubernetes.io/name: {{ include "kryton.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}
