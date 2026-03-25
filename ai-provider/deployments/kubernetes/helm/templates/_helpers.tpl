{{/*
Expand the name of the chart.
*/}}
{{- define "ai-provider.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "ai-provider.fullname" -}}
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

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "ai-provider.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "ai-provider.labels" -}}
helm.sh/chart: {{ include "ai-provider.chart" . }}
{{ include "ai-provider.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: ai-provider-platform
{{- with .Values.global.labels }}
{{ toYaml . }}
{{- end }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "ai-provider.selectorLabels" -}}
app.kubernetes.io/name: {{ include "ai-provider.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Common annotations
*/}}
{{- define "ai-provider.annotations" -}}
{{- with .Values.global.annotations }}
{{ toYaml . }}
{{- end }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "ai-provider.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "ai-provider.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Return the proper image name
*/}}
{{- define "ai-provider.image" -}}
{{- $registryName := .Values.global.imageRegistry | default .Values.image.registry -}}
{{- $repositoryName := .Values.image.repository -}}
{{- $tag := .Values.image.tag | default .Chart.AppVersion | toString -}}
{{- if $registryName }}
{{- printf "%s/%s:%s" $registryName $repositoryName $tag -}}
{{- else }}
{{- printf "%s:%s" $repositoryName $tag -}}
{{- end }}
{{- end }}

{{/*
Return the proper PostgreSQL image name
*/}}
{{- define "ai-provider.postgresql.image" -}}
{{- $registryName := .Values.global.imageRegistry | default .Values.postgresql.image.registry -}}
{{- $repositoryName := .Values.postgresql.image.repository -}}
{{- $tag := .Values.postgresql.image.tag | toString -}}
{{- if $registryName }}
{{- printf "%s/%s:%s" $registryName $repositoryName $tag -}}
{{- else }}
{{- printf "%s:%s" $repositoryName $tag -}}
{{- end }}
{{- end }}

{{/*
Return the proper Redis image name
*/}}
{{- define "ai-provider.redis.image" -}}
{{- $registryName := .Values.global.imageRegistry | default .Values.redis.image.registry -}}
{{- $repositoryName := .Values.redis.image.repository -}}
{{- $tag := .Values.redis.image.tag | toString -}}
{{- if $registryName }}
{{- printf "%s/%s:%s" $registryName $repositoryName $tag -}}
{{- else }}
{{- printf "%s:%s" $repositoryName $tag -}}
{{- end }}
{{- end }}

{{/*
Return the appropriate apiVersion for ingress
*/}}
{{- define "ai-provider.ingress.apiVersion" -}}
{{- if semverCompare ">=1.19-0" .Capabilities.KubeVersion.GitVersion -}}
networking.k8s.io/v1
{{- else if semverCompare ">=1.14-0" .Capabilities.KubeVersion.GitVersion -}}
networking.k8s.io/v1beta1
{{- else -}}
extensions/v1beta1
{{- end }}
{{- end }}

{{/*
Return the appropriate apiVersion for networkpolicy
*/}}
{{- define "ai-provider.networkPolicy.apiVersion" -}}
{{- if semverCompare ">=1.19-0" .Capabilities.KubeVersion.GitVersion -}}
networking.k8s.io/v1
{{- else -}}
networking.k8s.io/v1beta1
{{- end }}
{{- end }}

{{/*
Return the appropriate apiVersion for RBAC
*/}}
{{- define "ai-provider.rbac.apiVersion" -}}
{{- if semverCompare ">=1.17-0" .Capabilities.KubeVersion.GitVersion -}}
rbac.authorization.k8s.io/v1
{{- else -}}
rbac.authorization.k8s.io/v1beta1
{{- end }}
{{- end }}

{{/*
Return the appropriate apiVersion for HPA
*/}}
{{- define "ai-provider.hpa.apiVersion" -}}
{{- if semverCompare ">=1.23-0" .Capabilities.KubeVersion.GitVersion -}}
autoscaling/v2
{{- else -}}
autoscaling/v2beta2
{{- end }}
{{- end }}

{{/*
Return the appropriate apiVersion for PDB
*/}}
{{- define "ai-provider.pdb.apiVersion" -}}
{{- if semverCompare ">=1.21-0" .Capabilities.KubeVersion.GitVersion -}}
policy/v1
{{- else -}}
policy/v1beta1
{{- end }}
{{- end }}

{{/*
Create a fully qualified PostgreSQL name.
*/}}
{{- define "ai-provider.postgresql.fullname" -}}
{{- if .Values.postgresql.enabled }}
{{- printf "%s-%s" (include "ai-provider.fullname" .) "postgresql" | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s" .Values.externalDatabase.host }}
{{- end }}
{{- end }}

{{/*
Create a fully qualified Redis name.
*/}}
{{- define "ai-provider.redis.fullname" -}}
{{- if .Values.redis.enabled }}
{{- printf "%s-%s" (include "ai-provider.fullname" .) "redis" | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s" .Values.externalRedis.host }}
{{- end }}
{{- end }}

{{/*
Get the PostgreSQL host
*/}}
{{- define "ai-provider.postgresql.host" -}}
{{- if .Values.postgresql.enabled }}
{{- include "ai-provider.postgresql.fullname" . }}
{{- else }}
{{- .Values.externalDatabase.host }}
{{- end }}
{{- end }}

{{/*
Get the Redis host
*/}}
{{- define "ai-provider.redis.host" -}}
{{- if .Values.redis.enabled }}
{{- include "ai-provider.redis.fullname" . }}
{{- else }}
{{- .Values.externalRedis.host }}
{{- end }}
{{- end }}

{{/*
Get the PostgreSQL port
*/}}
{{- define "ai-provider.postgresql.port" -}}
{{- if .Values.postgresql.enabled }}
{{- 5432 }}
{{- else }}
{{- .Values.externalDatabase.port }}
{{- end }}
{{- end }}

{{/*
Get the Redis port
*/}}
{{- define "ai-provider.redis.port" -}}
{{- if .Values.redis.enabled }}
{{- 6379 }}
{{- else }}
{{- .Values.externalRedis.port }}
{{- end }}
{{- end }}

{{/*
Get the PostgreSQL database name
*/}}
{{- define "ai-provider.postgresql.database" -}}
{{- if .Values.postgresql.enabled }}
{{- .Values.postgresql.auth.database }}
{{- else }}
{{- .Values.externalDatabase.database }}
{{- end }}
{{- end }}

{{/*
Get the PostgreSQL username
*/}}
{{- define "ai-provider.postgresql.username" -}}
{{- if .Values.postgresql.enabled }}
{{- .Values.postgresql.auth.username }}
{{- else }}
{{- .Values.externalDatabase.username }}
{{- end }}
{{- end }}

{{/*
Return the PostgreSQL secret name
*/}}
{{- define "ai-provider.postgresql.secretName" -}}
{{- if .Values.postgresql.enabled }}
{{- printf "%s-%s" (include "ai-provider.fullname" .) "postgresql" }}
{{- else if .Values.externalDatabase.existingSecret }}
{{- .Values.externalDatabase.existingSecret }}
{{- else }}
{{- printf "%s-%s" (include "ai-provider.fullname" .) "external-database" }}
{{- end }}
{{- end }}

{{/*
Return the Redis secret name
*/}}
{{- define "ai-provider.redis.secretName" -}}
{{- if .Values.redis.enabled }}
{{- printf "%s-%s" (include "ai-provider.fullname" .) "redis" }}
{{- else if .Values.externalRedis.existingSecret }}
{{- .Values.externalRedis.existingSecret }}
{{- else }}
{{- printf "%s-%s" (include "ai-provider.fullname" .) "external-redis" }}
{{- end }}
{{- end }}

{{/*
Return the JWT secret name
*/}}
{{- define "ai-provider.jwt.secretName" -}}
{{- if .Values.security.jwt.existingSecret }}
{{- .Values.security.jwt.existingSecret }}
{{- else }}
{{- printf "%s-%s" (include "ai-provider.fullname" .) "jwt" }}
{{- end }}
{{- end }}

{{/*
Return the TLS secret name
*/}}
{{- define "ai-provider.tls.secretName" -}}
{{- if .Values.ingress.tls.existingSecret }}
{{- .Values.ingress.tls.existingSecret }}
{{- else }}
{{- printf "%s-%s" (include "ai-provider.fullname" .) "tls" }}
{{- end }}
{{- end }}

{{/*
Determine secret name for image pull
*/}}
{{- define "ai-provider.imagePullSecrets" -}}
{{- if .Values.global.imagePullSecrets }}
imagePullSecrets:
{{- range .Values.global.imagePullSecrets }}
  - name: {{ . }}
{{- end }}
{{- else if .Values.imagePullSecrets }}
imagePullSecrets:
{{- range .Values.imagePullSecrets }}
  - name: {{ . }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create annotations for prometheus scraping
*/}}
{{- define "ai-provider.prometheus.annotations" -}}
{{- if .Values.monitoring.serviceMonitor.enabled }}
prometheus.io/scrape: "true"
prometheus.io/port: {{ .Values.service.metricsPort | quote }}
prometheus.io/path: "/metrics"
{{- end }}
{{- end }}

{{/*
Pod labels - adds extra labels specific to pods
*/}}
{{- define "ai-provider.podLabels" -}}
{{ include "ai-provider.selectorLabels" . }}
{{- with .Values.podLabels }}
{{ toYaml . }}
{{- end }}
{{- end }}

{{/*
Pod annotations - adds extra annotations specific to pods
*/}}
{{- define "ai-provider.podAnnotations" -}}
{{ include "ai-provider.prometheus.annotations" . }}
{{- with .Values.podAnnotations }}
{{ toYaml . }}
{{- end }}
{{- end }}

{{/*
Resource limits and requests
*/}}
{{- define "ai-provider.resources" -}}
limits:
  cpu: {{ .Values.resources.limits.cpu }}
  memory: {{ .Values.resources.limits.memory }}
requests:
  cpu: {{ .Values.resources.requests.cpu }}
  memory: {{ .Values.resources.requests.memory }}
{{- end }}

{{/*
Node selector
*/}}
{{- define "ai-provider.nodeSelector" -}}
{{- with .Values.nodeSelector }}
nodeSelector:
{{ toYaml . }}
{{- end }}
{{- end }}

{{/*
Tolerations
*/}}
{{- define "ai-provider.tolerations" -}}
{{- with .Values.tolerations }}
tolerations:
{{ toYaml . }}
{{- end }}
{{- end }}

{{/*
Affinity
*/}}
{{- define "ai-provider.affinity" -}}
{{- with .Values.affinity }}
affinity:
{{ toYaml . }}
{{- end }}
{{- end }}

{{/*
Security context for pods
*/}}
{{- define "ai-provider.podSecurityContext" -}}
{{- with .Values.podSecurityContext }}
securityContext:
{{ toYaml . }}
{{- end }}
{{- end }}

{{/*
Security context for containers
*/}}
{{- define "ai-provider.securityContext" -}}
{{- with .Values.securityContext }}
securityContext:
{{ toYaml . }}
{{- end }}
{{- end }}

{{/*
Volume mounts for main container
*/}}
{{- define "ai-provider.volumeMounts" -}}
- name: config
  mountPath: /app/config
  readOnly: true
- name: model-storage
  mountPath: {{ .Values.modelStorage.path }}
  subPath: models
- name: model-storage
  mountPath: {{ .Values.modelStorage.cachePath }}
  subPath: cache
- name: temp-storage
  mountPath: /tmp
- name: logs
  mountPath: /var/log/ai-provider
- name: audit-logs
  mountPath: /var/log/ai-provider/audit
- name: backup-storage
  mountPath: /backups
  readOnly: true
- name: plugins
  mountPath: /plugins
  readOnly: true
{{- with .Values.extraVolumeMounts }}
{{ toYaml . }}
{{- end }}
{{- end }}

{{/*
Volumes for pod
*/}}
{{- define "ai-provider.volumes" -}}
- name: config
  configMap:
    name: {{ include "ai-provider.fullname" . }}-config
- name: model-storage
  persistentVolumeClaim:
    claimName: {{ include "ai-provider.fullname" . }}-models-pvc
- name: temp-storage
  emptyDir:
    sizeLimit: 10Gi
- name: logs
  emptyDir:
    sizeLimit: 1Gi
- name: audit-logs
  persistentVolumeClaim:
    claimName: {{ include "ai-provider.fullname" . }}-audit-pvc
- name: backup-storage
  persistentVolumeClaim:
    claimName: {{ include "ai-provider.fullname" . }}-backup-pvc
- name: plugins
  configMap:
    name: {{ include "ai-provider.fullname" . }}-plugins
    optional: true
{{- with .Values.extraVolumes }}
{{ toYaml . }}
{{- end }}
{{- end }}

{{/*
Environment variables from configmap
*/}}
{{- define "ai-provider.envFromConfigMap" -}}
- configMapRef:
    name: {{ include "ai-provider.fullname" . }}-config
{{- end }}

{{/*
Environment variables from secrets
*/}}
{{- define "ai-provider.envFromSecrets" -}}
- secretRef:
    name: {{ include "ai-provider.postgresql.secretName" . }}
- secretRef:
    name: {{ include "ai-provider.redis.secretName" . }}
- secretRef:
    name: {{ include "ai-provider.jwt.secretName" . }}
{{- end }}

{{/*
Environment variables for database
*/}}
{{- define "ai-provider.databaseEnvVars" -}}
- name: DB_HOST
  value: {{ include "ai-provider.postgresql.host" . }}
- name: DB_PORT
  value: {{ include "ai-provider.postgresql.port" . | quote }}
- name: DB_NAME
  value: {{ include "ai-provider.postgresql.database" . }}
- name: DB_USERNAME
  valueFrom:
    secretKeyRef:
      name: {{ include "ai-provider.postgresql.secretName" . }}
      key: username
- name: DB_PASSWORD
  valueFrom:
    secretKeyRef:
      name: {{ include "ai-provider.postgresql.secretName" . }}
      key: password
{{- end }}

{{/*
Environment variables for redis
*/}}
{{- define "ai-provider.redisEnvVars" -}}
- name: REDIS_HOST
  value: {{ include "ai-provider.redis.host" . }}
- name: REDIS_PORT
  value: {{ include "ai-provider.redis.port" . | quote }}
- name: REDIS_PASSWORD
  valueFrom:
    secretKeyRef:
      name: {{ include "ai-provider.redis.secretName" . }}
      key: password
{{- end }}

{{/*
Environment variables for application
*/}}
{{- define "ai-provider.appEnvVars" -}}
- name: POD_NAME
  valueFrom:
    fieldRef:
      fieldPath: metadata.name
- name: POD_NAMESPACE
  valueFrom:
    fieldRef:
      fieldPath: metadata.namespace
- name: POD_IP
  valueFrom:
    fieldRef:
      fieldPath: status.podIP
- name: NODE_NAME
  valueFrom:
    fieldRef:
      fieldPath: spec.nodeName
{{- end }}

{{/*
Liveness probe
*/}}
{{- define "ai-provider.livenessProbe" -}}
{{- if .Values.probes.liveness.enabled }}
livenessProbe:
  httpGet:
    path: {{ .Values.probes.liveness.path }}
    port: http
  initialDelaySeconds: {{ .Values.probes.liveness.initialDelaySeconds }}
  periodSeconds: {{ .Values.probes.liveness.periodSeconds }}
  timeoutSeconds: {{ .Values.probes.liveness.timeoutSeconds }}
  failureThreshold: {{ .Values.probes.liveness.failureThreshold }}
  successThreshold: {{ .Values.probes.liveness.successThreshold }}
{{- end }}
{{- end }}

{{/*
Readiness probe
*/}}
{{- define "ai-provider.readinessProbe" -}}
{{- if .Values.probes.readiness.enabled }}
readinessProbe:
  httpGet:
    path: {{ .Values.probes.readiness.path }}
    port: http
  initialDelaySeconds: {{ .Values.probes.readiness.initialDelaySeconds }}
  periodSeconds: {{ .Values.probes.readiness.periodSeconds }}
  timeoutSeconds: {{ .Values.probes.readiness.timeoutSeconds }}
  failureThreshold: {{ .Values.probes.readiness.failureThreshold }}
  successThreshold: {{ .Values.probes.readiness.successThreshold }}
{{- end }}
{{- end }}

{{/*
Startup probe
*/}}
{{- define "ai-provider.startupProbe" -}}
{{- if .Values.probes.startup.enabled }}
startupProbe:
  httpGet:
    path: {{ .Values.probes.startup.path }}
    port: http
  initialDelaySeconds: {{ .Values.probes.startup.initialDelaySeconds }}
  periodSeconds: {{ .Values.probes.startup.periodSeconds }}
  timeoutSeconds: {{ .Values.probes.startup.timeoutSeconds }}
  failureThreshold: {{ .Values.probes.startup.failureThreshold }}
  successThreshold: {{ .Values.probes.startup.successThreshold }}
{{- end }}
{{- end }}

{{/*
Init containers
*/}}
{{- define "ai-provider.initContainers" -}}
{{- if .Values.initContainers.waitForDb.enabled }}
- name: wait-for-db
  image: {{ .Values.initContainers.waitForDb.image }}
  command: ['sh', '-c', 'until nc -z {{ include "ai-provider.postgresql.host" . }} {{ include "ai-provider.postgresql.port" . }}; do echo waiting for database; sleep 2; done']
  resources:
    limits:
      cpu: 50m
      memory: 32Mi
    requests:
      cpu: 10m
      memory: 16Mi
{{- end }}
{{- if .Values.initContainers.waitForRedis.enabled }}
- name: wait-for-redis
  image: {{ .Values.initContainers.waitForRedis.image }}
  command: ['sh', '-c', 'until nc -z {{ include "ai-provider.redis.host" . }} {{ include "ai-provider.redis.port" . }}; do echo waiting for redis; sleep 2; done']
  resources:
    limits:
      cpu: 50m
      memory: 32Mi
    requests:
      cpu: 10m
      memory: 16Mi
{{- end }}
{{- with .Values.extraInitContainers }}
{{ toYaml . }}
{{- end }}
{{- end }}

{{/*
Storage class name
*/}}
{{- define "ai-provider.storageClass" -}}
{{- if .Values.global.storageClass }}
storageClassName: {{ .Values.global.storageClass }}
{{- else if .Values.persistence.storageClass }}
storageClassName: {{ .Values.persistence.storageClass }}
{{- end }}
{{- end }}
