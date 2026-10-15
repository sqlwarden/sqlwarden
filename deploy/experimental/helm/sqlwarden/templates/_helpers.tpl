{{/* Chart name, overridable per release. */}}
{{- define "sqlwarden.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "sqlwarden.fullname" -}}
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

{{- define "sqlwarden.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "sqlwarden.labels" -}}
helm.sh/chart: {{ include "sqlwarden.chart" . }}
{{ include "sqlwarden.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{- define "sqlwarden.selectorLabels" -}}
app.kubernetes.io/name: {{ include "sqlwarden.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{/* Names of the per-workload resources. */}}
{{- define "sqlwarden.all.fullname" -}}
{{- printf "%s-all" (include "sqlwarden.fullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "sqlwarden.api.fullname" -}}
{{- printf "%s-api" (include "sqlwarden.fullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "sqlwarden.jobs.fullname" -}}
{{- printf "%s-jobs" (include "sqlwarden.fullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "sqlwarden.migration.fullname" -}}
{{- printf "%s-migrate" (include "sqlwarden.fullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
The Deployment that serves HTTP API traffic: the all-in-one Deployment in mode
all, the api Deployment in mode split. The component name is also the key of
its values block and the component label the Service selects.
*/}}
{{- define "sqlwarden.serving.component" -}}
{{- if eq .Values.mode "split" -}}api{{- else -}}all{{- end -}}
{{- end -}}

{{- define "sqlwarden.serving.fullname" -}}
{{- if eq .Values.mode "split" -}}
{{- include "sqlwarden.api.fullname" . -}}
{{- else -}}
{{- include "sqlwarden.all.fullname" . -}}
{{- end -}}
{{- end -}}

{{- define "sqlwarden.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- include "sqlwarden.fullname" . -}}
{{- else -}}
default
{{- end -}}
{{- end -}}

{{- define "sqlwarden.migration.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- include "sqlwarden.migration.fullname" . -}}
{{- else -}}
default
{{- end -}}
{{- end -}}

{{- define "sqlwarden.configMapName" -}}
{{- include "sqlwarden.fullname" . -}}
{{- end -}}

{{- define "sqlwarden.secretName" -}}
{{- if .Values.secrets.existingSecret -}}
{{- .Values.secrets.existingSecret -}}
{{- else -}}
{{- include "sqlwarden.fullname" . -}}
{{- end -}}
{{- end -}}

{{- define "sqlwarden.databaseSecretName" -}}
{{- if .Values.database.existingSecret -}}
{{- .Values.database.existingSecret -}}
{{- else if and .Values.secrets.create (not .Values.secrets.existingSecret) -}}
{{- include "sqlwarden.secretName" . -}}
{{- else -}}
{{- printf "%s-database" (include "sqlwarden.fullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}

{{- define "sqlwarden.image" -}}
{{- printf "%s:%s" .Values.image.repository (default .Chart.AppVersion .Values.image.tag) -}}
{{- end -}}

{{/*
Environment shared by every SQLWarden container. Secrets are referenced from
Secrets, never inlined and never placed in the ConfigMap.
*/}}
{{- define "sqlwarden.commonEnv" -}}
{{- range $key := list "DB_DRIVER" "BASE_URL" "PROFILE" "LOG_FORMAT" "SHUTDOWN_TIMEOUT" "DB_MIGRATION_TIMEOUT" "FILES_ROOT_DIR" }}
- name: {{ $key }}
  valueFrom:
    configMapKeyRef:
      name: {{ include "sqlwarden.configMapName" $ }}
      key: {{ $key }}
{{- end }}
{{- if .Values.config.trustedProxies }}
- name: SERVER_TRUSTED_PROXIES
  valueFrom:
    configMapKeyRef:
      name: {{ include "sqlwarden.configMapName" . }}
      key: SERVER_TRUSTED_PROXIES
{{- end }}
- name: DB_DSN
  valueFrom:
    secretKeyRef:
      name: {{ include "sqlwarden.databaseSecretName" . }}
      key: {{ .Values.database.dsnKey }}
- name: COOKIE_SECRET_KEY
  valueFrom:
    secretKeyRef:
      name: {{ include "sqlwarden.secretName" . }}
      key: cookie_secret_key
- name: JWT_SECRET_KEY
  valueFrom:
    secretKeyRef:
      name: {{ include "sqlwarden.secretName" . }}
      key: jwt_secret_key
- name: ENCRYPTION_KEY
  valueFrom:
    secretKeyRef:
      name: {{ include "sqlwarden.secretName" . }}
      key: encryption_key
{{- with .Values.config.extraEnv }}
{{ toYaml . }}
{{- end }}
{{- end -}}

{{/*
Guards that turn an unsupported topology into a render failure with the same
reason the application would report at startup.
*/}}
{{- define "sqlwarden.validateTopology" -}}
{{- if not (has .Values.mode (list "all" "split")) -}}
{{- fail "mode must be all or split." -}}
{{- end -}}
{{- if not (has .Values.database.driver (list "postgres" "sqlite")) -}}
{{- fail "database.driver must be postgres or sqlite." -}}
{{- end -}}
{{- if and (eq .Values.mode "split") (ne .Values.database.driver "postgres") -}}
{{- fail "database.driver must be postgres when mode is split: SQLite cannot be shared by separate Deployments and the migration Job." -}}
{{- end -}}
{{- if eq .Values.database.driver "sqlite" -}}
{{- if gt (int .Values.all.replicaCount) 1 -}}
{{- fail "all.replicaCount must be 1 when database.driver is sqlite: the database file is owned by one process." -}}
{{- end -}}
{{- if .Values.migration.enabled -}}
{{- fail "migration.enabled must be false when database.driver is sqlite: the migration Job cannot share the database file with the serving pod." -}}
{{- end -}}
{{- end -}}
{{- if eq .Values.mode "split" -}}
{{- if not .Values.storage.existingClaim -}}
{{- fail "storage.existingClaim (ReadWriteMany) is required when mode is split: the api and jobs pods must see the same workspace file volume." -}}
{{- end -}}
{{- else if and (gt (int .Values.all.replicaCount) 1) (not .Values.storage.existingClaim) -}}
{{- fail "storage.existingClaim is required when all.replicaCount is above 1: workspace file content lives on a filesystem every replica must share." -}}
{{- end -}}
{{- if and (eq .Values.mode "split") (gt (int .Values.api.replicaCount) 1) (not .Values.storage.existingClaim) -}}
{{- fail "storage.existingClaim is required when api.replicaCount is above 1." -}}
{{- end -}}
{{- if and .Values.secrets.create (not .Values.secrets.existingSecret) -}}
{{- range $key := list "cookieSecretKey" "jwtSecretKey" "encryptionKey" -}}
{{- if not (index $.Values.secrets $key) -}}
{{- fail (printf "secrets.%s is required when secrets.create is true; set it or point secrets.existingSecret at a Secret you manage." $key) -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- if and (not .Values.secrets.create) (not .Values.secrets.existingSecret) -}}
{{- fail "secrets.existingSecret is required when secrets.create is false." -}}
{{- end -}}
{{- if and (not .Values.database.existingSecret) (not .Values.database.dsn) -}}
{{- fail "database.dsn or database.existingSecret is required." -}}
{{- end -}}
{{- end -}}
