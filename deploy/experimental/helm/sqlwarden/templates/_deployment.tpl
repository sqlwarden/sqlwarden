{{/*
One SQLWarden Deployment. Arguments: root (the chart context), kind (the
process kind, also the component label), name (the resource name), and cfg (the
values block for the workload).
*/}}
{{- define "sqlwarden.deployment" -}}
{{- $root := .root -}}
{{- $kind := .kind -}}
{{- $cfg := .cfg -}}
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ .name }}
  labels:
    {{- include "sqlwarden.labels" $root | nindent 4 }}
    app.kubernetes.io/component: {{ $kind }}
spec:
  replicas: {{ $cfg.replicaCount }}
  selector:
    matchLabels:
      {{- include "sqlwarden.selectorLabels" $root | nindent 6 }}
      app.kubernetes.io/component: {{ $kind }}
  template:
    metadata:
      labels:
        {{- include "sqlwarden.selectorLabels" $root | nindent 8 }}
        app.kubernetes.io/component: {{ $kind }}
        {{- with $cfg.podLabels }}
        {{- toYaml . | nindent 8 }}
        {{- end }}
      annotations:
        checksum/config: {{ include (print $root.Template.BasePath "/configmap.yaml") $root | sha256sum }}
        {{- with $cfg.podAnnotations }}
        {{- toYaml . | nindent 8 }}
        {{- end }}
    spec:
      serviceAccountName: {{ include "sqlwarden.serviceAccountName" $root }}
      automountServiceAccountToken: false
      terminationGracePeriodSeconds: {{ $root.Values.config.terminationGracePeriodSeconds }}
      {{- with $root.Values.imagePullSecrets }}
      imagePullSecrets:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      securityContext:
        {{- toYaml $root.Values.podSecurityContext | nindent 8 }}
      {{- with $cfg.nodeSelector }}
      nodeSelector:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with $cfg.tolerations }}
      tolerations:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with $cfg.affinity }}
      affinity:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      containers:
        - name: {{ $kind }}
          image: {{ include "sqlwarden.image" $root }}
          imagePullPolicy: {{ $root.Values.image.pullPolicy }}
          securityContext:
            {{- toYaml $root.Values.securityContext | nindent 12 }}
          ports:
            - name: http
              containerPort: {{ $cfg.httpPort }}
              protocol: TCP
          env:
            - name: PROCESS_KINDS
              value: {{ $kind }}
            - name: HTTP_PORT
              value: {{ $cfg.httpPort | quote }}
            {{- if and (eq $kind "all") (not $root.Values.migration.enabled) }}
            - name: DB_AUTOMIGRATE
              value: "true"
            {{- else }}
            - name: DB_AUTOMIGRATE
              value: "false"
            {{- end }}
            {{- include "sqlwarden.commonEnv" $root | nindent 12 }}
            {{- with $cfg.extraEnv }}
            {{- toYaml . | nindent 12 }}
            {{- end }}
          startupProbe:
            httpGet:
              path: /readyz
              port: http
            periodSeconds: {{ $cfg.probes.startup.periodSeconds }}
            failureThreshold: {{ $cfg.probes.startup.failureThreshold }}
          livenessProbe:
            httpGet:
              path: /livez
              port: http
            periodSeconds: {{ $cfg.probes.liveness.periodSeconds }}
            timeoutSeconds: {{ $cfg.probes.liveness.timeoutSeconds }}
            failureThreshold: {{ $cfg.probes.liveness.failureThreshold }}
          readinessProbe:
            httpGet:
              path: /readyz
              port: http
            periodSeconds: {{ $cfg.probes.readiness.periodSeconds }}
            timeoutSeconds: {{ $cfg.probes.readiness.timeoutSeconds }}
            failureThreshold: {{ $cfg.probes.readiness.failureThreshold }}
          resources:
            {{- toYaml $cfg.resources | nindent 12 }}
          volumeMounts:
            - name: data
              mountPath: {{ $root.Values.storage.mountPath }}
            - name: tmp
              mountPath: /tmp
      volumes:
        - name: data
          {{- if $root.Values.storage.existingClaim }}
          persistentVolumeClaim:
            claimName: {{ $root.Values.storage.existingClaim }}
          {{- else }}
          emptyDir: {}
          {{- end }}
        - name: tmp
          emptyDir: {}
{{- end -}}
