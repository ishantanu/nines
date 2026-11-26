{{- define "nines.fullname" -}}
{{- if contains "nines" .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-nines" .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}

{{- define "nines.operatorName" -}}
{{- printf "%s-operator" (include "nines.fullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "nines.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" }}
app.kubernetes.io/part-of: nines
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{- define "nines.selector" -}}
app.kubernetes.io/name: nines
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "nines.operatorSelector" -}}
app.kubernetes.io/name: nines-operator
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "nines.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "nines.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{- define "nines.credentialsSecret" -}}
{{- default (printf "%s-credentials" (include "nines.fullname" .)) .Values.credentials.existingSecret -}}
{{- end -}}

{{/* An image reference: repository, then the digest or tag (default appVersion). */}}
{{- define "nines.image" -}}
{{- $image := index . 0 -}}
{{- $root := index . 1 -}}
{{- if $image.digest -}}
{{- printf "%s@%s" $image.repository $image.digest -}}
{{- else -}}
{{- printf "%s:%s" $image.repository (default $root.Chart.AppVersion $image.tag) -}}
{{- end -}}
{{- end -}}

{{- define "nines.publicURL" -}}
{{- if .Values.publicURL -}}
{{- .Values.publicURL | trimSuffix "/" -}}
{{- else if .Values.ingress.enabled -}}
{{- printf "%s://%s" (ternary "https" "http" (ne .Values.ingress.tlsSecretName "")) .Values.ingress.host -}}
{{- else -}}
http://127.0.0.1:8080
{{- end -}}
{{- end -}}

{{- define "nines.restrictedContainer" -}}
allowPrivilegeEscalation: false
readOnlyRootFilesystem: true
capabilities:
  drop: [ALL]
{{- end -}}

{{- define "nines.restrictedPod" -}}
runAsNonRoot: true
runAsUser: 65532
runAsGroup: 65532
fsGroup: 65532
seccompProfile:
  type: RuntimeDefault
{{- end -}}
