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

{{/*
Per-process-kind names. Every resource a process kind owns derives its name
here, so a new process kind adds templates rather than editing shared ones.
*/}}
{{- define "sqlwarden.api.fullname" -}}
{{- printf "%s-api" (include "sqlwarden.fullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "sqlwarden.connector.fullname" -}}
{{- printf "%s-connector" (include "sqlwarden.fullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "sqlwarden.migration.fullname" -}}
{{- printf "%s-migrate" (include "sqlwarden.fullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "sqlwarden.api.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- include "sqlwarden.api.fullname" . -}}
{{- else -}}
default
{{- end -}}
{{- end -}}

{{- define "sqlwarden.connector.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- include "sqlwarden.connector.fullname" . -}}
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
Connector address shared by both process kinds: the api process dials it and
the connector process is reachable at it.
*/}}
{{- define "sqlwarden.connectorAddress" -}}
{{- printf "%s:%v" (include "sqlwarden.connector.fullname" .) .Values.connector.listenPort -}}
{{- end -}}

{{/*
Environment shared by every SQLWarden container. Secrets are referenced, never
inlined, and db.automigrate is pinned off: migrations belong to the migration
Job, and a serving replica rejects automigrate at startup anyway.
*/}}
{{- define "sqlwarden.commonEnv" -}}
- name: DB_DRIVER
  valueFrom:
    configMapKeyRef:
      name: {{ include "sqlwarden.configMapName" . }}
      key: DB_DRIVER
- name: BASE_URL
  valueFrom:
    configMapKeyRef:
      name: {{ include "sqlwarden.configMapName" . }}
      key: BASE_URL
- name: ACCESS_MODE
  valueFrom:
    configMapKeyRef:
      name: {{ include "sqlwarden.configMapName" . }}
      key: ACCESS_MODE
- name: LOG_FORMAT
  valueFrom:
    configMapKeyRef:
      name: {{ include "sqlwarden.configMapName" . }}
      key: LOG_FORMAT
- name: SHUTDOWN_TIMEOUT
  valueFrom:
    configMapKeyRef:
      name: {{ include "sqlwarden.configMapName" . }}
      key: SHUTDOWN_TIMEOUT
- name: DB_MIGRATION_TIMEOUT
  valueFrom:
    configMapKeyRef:
      name: {{ include "sqlwarden.configMapName" . }}
      key: DB_MIGRATION_TIMEOUT
- name: FILES_ROOT_DIR
  valueFrom:
    configMapKeyRef:
      name: {{ include "sqlwarden.configMapName" . }}
      key: FILES_ROOT_DIR
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
- name: CONNECTOR_GRANT_SIGNING_KEY
  valueFrom:
    secretKeyRef:
      name: {{ include "sqlwarden.secretName" . }}
      key: connector_grant_signing_key
- name: CONNECTOR_ADDRESS
  value: {{ include "sqlwarden.connectorAddress" . | quote }}
- name: CONNECTOR_REPLICAS
  value: {{ .Values.connector.replicas | quote }}
{{- with .Values.config.extraEnv }}
{{ toYaml . }}
{{- end }}
{{- end -}}

{{/*
Guards that turn an unsupported topology into a render failure with the same
reason the application would report at startup.
*/}}
{{- define "sqlwarden.validateTopology" -}}
{{- if gt (int .Values.connector.replicas) 1 -}}
{{- fail "connector.replicas must be 1: the static session directory resolves a single connector address, so extra replicas would own live sessions nothing can route to. The connector process enforces the same rule at startup." -}}
{{- end -}}
{{- if and .Values.api.enabled (not .Values.connector.enabled) -}}
{{- fail "connector.enabled must be true while api.enabled is true: an api process delegates every target-database session to the connector process." -}}
{{- end -}}
{{- if and (gt (int .Values.api.replicas) 1) (not .Values.api.files.existingClaim) -}}
{{- fail "api.files.existingClaim is required when api.replicas is above 1: workspace file content lives on a filesystem backend that every api replica must share." -}}
{{- end -}}
{{- if and .Values.secrets.create (not .Values.secrets.existingSecret) -}}
{{- range $key := list "cookieSecretKey" "jwtSecretKey" "encryptionKey" "connectorGrantSigningKey" -}}
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
{{- if ne .Values.database.driver "postgres" -}}
{{- fail "database.driver must be postgres for the split api/connector chart: SQLite cannot be shared safely by separate Deployments and the migration Job." -}}
{{- end -}}
{{- if and .Values.api.enabled (lt (int .Values.api.replicas) 1) -}}
{{- fail "api.replicas must be at least 1 when api.enabled is true." -}}
{{- end -}}
{{- end -}}
