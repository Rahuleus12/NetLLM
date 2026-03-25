# AI Provider Helm Chart

## Overview

This Helm chart deploys the AI Provider Platform - a comprehensive local AI model management system - onto a Kubernetes cluster. The chart provides production-ready configurations with high availability, security, monitoring, and scalability features.

## Features

- **Complete Platform Deployment**: Deploys all AI Provider components including API, database, cache, and monitoring
- **High Availability**: Supports multiple replicas with pod anti-affinity and pod disruption budgets
- **Auto-scaling**: Horizontal Pod Autoscaler (HPA) for automatic scaling based on load
- **Security**: Network policies, RBAC, secrets management, and TLS support
- **Monitoring**: Integrated Prometheus ServiceMonitors and Grafana dashboards
- **GitOps Ready**: Compatible with ArgoCD and Flux for GitOps workflows
- **Multi-environment**: Configurable for development, staging, and production environments

## Prerequisites

### Required

- Kubernetes 1.24+
- Helm 3.8+
- PersistentVolume provisioner support in the underlying infrastructure
- `kubectl` configured to communicate with your cluster

### Recommended

- Prometheus Operator (for monitoring)
- NGINX Ingress Controller (for external access)
- cert-manager (for TLS certificates)
- GPU nodes (for inference acceleration)

### Hardware Requirements

**Minimum (Development)**:
- CPU: 4 cores
- Memory: 8GB RAM
- Storage: 100GB

**Recommended (Production)**:
- CPU: 16+ cores
- Memory: 64GB+ RAM
- Storage: 1TB+ SSD
- GPU: NVIDIA GPU (optional, for acceleration)

## Installation

### Quick Start

```bash
# Add the Helm repository (if published)
helm repo add ai-provider https://charts.ai-provider.io
helm repo update

# Install with default values
helm install ai-provider ai-provider/ai-provider \
  --namespace ai-provider \
  --create-namespace
```

### Install from Source

```bash
# Clone the repository
git clone https://github.com/ai-provider/ai-provider.git
cd ai-provider/deployments/kubernetes/helm

# Install with custom values
helm install ai-provider ./ai-provider \
  --namespace ai-provider \
  --create-namespace \
  -f my-values.yaml
```

### Install with Custom Configuration

```bash
# Create a custom values file
cat > my-values.yaml <<EOF
replicaCount: 3

image:
  repository: ai-provider
  tag: "1.0.0"
  pullPolicy: IfNotPresent

ingress:
  enabled: true
  hostname: api.ai-provider.example.com
  tls:
    enabled: true
    secretName: ai-provider-tls

postgresql:
  auth:
    password: "my-secure-password"
    postgresPassword: "my-secure-root-password"

redis:
  auth:
    password: "my-secure-redis-password"
EOF

# Install with custom values
helm install ai-provider ./ai-provider \
  --namespace ai-provider \
  --create-namespace \
  -f my-values.yaml
```

### Dry Run and Validation

```bash
# Dry run to see what will be deployed
helm install ai-provider ./ai-provider \
  --namespace ai-provider \
  --dry-run --debug

# Validate the rendered templates
helm template ai-provider ./ai-provider \
  --namespace ai-provider \
  -f my-values.yaml > rendered.yaml

# Apply with kubectl for review
kubectl apply -f rendered.yaml --dry-run=client
```

## Configuration

### Basic Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `replicaCount` | Number of API replicas | `3` |
| `image.repository` | Image repository | `ai-provider` |
| `image.tag` | Image tag | `1.0.0` |
| `image.pullPolicy` | Image pull policy | `IfNotPresent` |
| `namespace` | Namespace to deploy into | `ai-provider` |

### Resource Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `resources.requests.cpu` | CPU request | `500m` |
| `resources.requests.memory` | Memory request | `1Gi` |
| `resources.limits.cpu` | CPU limit | `2000m` |
| `resources.limits.memory` | Memory limit | `4Gi` |

### Ingress Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `ingress.enabled` | Enable ingress | `true` |
| `ingress.className` | Ingress class name | `nginx` |
| `ingress.hostname` | Hostname for the service | `api.ai-provider.example.com` |
| `ingress.tls.enabled` | Enable TLS | `true` |
| `ingress.tls.secretName` | TLS secret name | `ai-provider-tls` |
| `ingress.annotations` | Ingress annotations | `{}` |

### Database Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `postgresql.enabled` | Deploy PostgreSQL | `true` |
| `postgresql.auth.username` | Database username | `ai_admin` |
| `postgresql.auth.password` | Database password | `change_me` |
| `postgresql.auth.database` | Database name | `ai_provider` |
| `postgresql.primary.persistence.size` | PVC size | `20Gi` |

### Redis Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `redis.enabled` | Deploy Redis | `true` |
| `redis.auth.password` | Redis password | `change_me` |
| `redis.master.persistence.size` | PVC size | `10Gi` |

### Storage Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `persistence.models.enabled` | Enable model storage | `true` |
| `persistence.models.size` | Model storage size | `500Gi` |
| `persistence.models.storageClass` | Storage class | `fast-ssd` |
| `persistence.cache.enabled` | Enable cache storage | `true` |
| `persistence.cache.size` | Cache storage size | `100Gi` |

### Security Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `security.enabled` | Enable security features | `true` |
| `security.auth.jwtSecret` | JWT signing secret | `change_me` |
| `security.tls.enabled` | Enable TLS | `true` |
| `networkPolicy.enabled` | Enable network policies | `true` |
| `podSecurityContext.runAsNonRoot` | Run as non-root | `true` |

### Monitoring Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `monitoring.enabled` | Enable monitoring | `true` |
| `monitoring.serviceMonitor.enabled` | Enable ServiceMonitor | `true` |
| `monitoring.prometheusRules.enabled` | Enable PrometheusRules | `true` |
| `monitoring.grafana.dashboards.enabled` | Enable Grafana dashboards | `true` |

### Auto-scaling Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `autoscaling.enabled` | Enable HPA | `true` |
| `autoscaling.minReplicas` | Minimum replicas | `3` |
| `autoscaling.maxReplicas` | Maximum replicas | `10` |
| `autoscaling.targetCPUUtilizationPercentage` | Target CPU % | `70` |
| `autoscaling.targetMemoryUtilizationPercentage` | Target Memory % | `80` |

## Advanced Configuration

### GPU Support

```yaml
# Enable GPU support for inference acceleration
gpu:
  enabled: true
  type: nvidia
  count: 1
  memory: 8Gi

# Node affinity for GPU nodes
nodeSelector:
  nvidia.com/gpu.present: "true"
```

### High Availability

```yaml
# Production HA configuration
replicaCount: 5

podDisruptionBudget:
  enabled: true
  minAvailable: 3

podAntiAffinity:
  type: hard
  topologyKey: kubernetes.io/hostname

topologySpreadConstraints:
  - maxSkew: 1
    topologyKey: topology.kubernetes.io/zone
    whenUnsatisfiable: DoNotSchedule
```

### External Database

```yaml
# Use external PostgreSQL instance
postgresql:
  enabled: false

externalDatabase:
  host: postgres.external.example.com
  port: 5432
  database: ai_provider
  username: ai_admin
  password: my-password
  existingSecret: db-credentials
  existingSecretPasswordKey: password
```

### External Redis

```yaml
# Use external Redis instance
redis:
  enabled: false

externalRedis:
  host: redis.external.example.com
  port: 6379
  password: my-password
  existingSecret: redis-credentials
  existingSecretPasswordKey: password
```

### Inference Configuration

```yaml
# Configure inference engine
inference:
  enabled: true
  maxConcurrent: 10
  timeout: 300
  batchSize: 8
  gpuEnabled: true
  gpuMemoryFraction: 0.8
  cpuThreads: 8
```

### Backup Configuration

```yaml
# Configure automated backups
backup:
  enabled: true
  schedule: "0 2 * * *"  # Daily at 2 AM
  retention: 30  # days
  compression: gzip
  storage:
    size: 200Gi
    storageClass: standard-hdd
```

## Environment-Specific Deployments

### Development

```yaml
# values-dev.yaml
replicaCount: 1

resources:
  requests:
    cpu: 250m
    memory: 512Mi
  limits:
    cpu: 500m
    memory: 1Gi

ingress:
  enabled: false

autoscaling:
  enabled: false

monitoring:
  enabled: false
```

```bash
helm install ai-provider ./ai-provider \
  --namespace ai-provider-dev \
  -f values-dev.yaml
```

### Staging

```yaml
# values-staging.yaml
replicaCount: 2

resources:
  requests:
    cpu: 500m
    memory: 1Gi
  limits:
    cpu: 1000m
    memory: 2Gi

ingress:
  enabled: true
  hostname: api.staging.ai-provider.example.com
  tls:
    enabled: true

autoscaling:
  enabled: true
  minReplicas: 2
  maxReplicas: 5
```

```bash
helm install ai-provider ./ai-provider \
  --namespace ai-provider-staging \
  -f values-staging.yaml
```

### Production

```yaml
# values-prod.yaml
replicaCount: 5

resources:
  requests:
    cpu: 1000m
    memory: 2Gi
  limits:
    cpu: 4000m
    memory: 8Gi

ingress:
  enabled: true
  hostname: api.ai-provider.example.com
  tls:
    enabled: true
    certManager: true

autoscaling:
  enabled: true
  minReplicas: 5
  maxReplicas: 20

podDisruptionBudget:
  enabled: true
  minAvailable: 3

networkPolicy:
  enabled: true

monitoring:
  enabled: true
```

```bash
helm install ai-provider ./ai-provider \
  --namespace ai-provider \
  -f values-prod.yaml
```

## Upgrading

### Upgrade the Release

```bash
# Update the repository
helm repo update

# Upgrade to a new version
helm upgrade ai-provider ai-provider/ai-provider \
  --namespace ai-provider \
  -f my-values.yaml

# Dry run to see changes
helm upgrade ai-provider ai-provider/ai-provider \
  --namespace ai-provider \
  --dry-run
```

### Rollback

```bash
# List release history
helm history ai-provider --namespace ai-provider

# Rollback to previous version
helm rollback ai-provider --namespace ai-provider

# Rollback to specific revision
helm rollback ai-provider 2 --namespace ai-provider
```

## Uninstallation

```bash
# Uninstall the release
helm uninstall ai-provider --namespace ai-provider

# Delete the namespace (optional)
kubectl delete namespace ai-provider

# Delete PVCs (optional - WARNING: data loss)
kubectl delete pvc -l app.kubernetes.io/name=ai-provider -n ai-provider
```

## Post-Installation

### Verify Installation

```bash
# Check pod status
kubectl get pods -n ai-provider

# Check services
kubectl get svc -n ai-provider

# Check ingress
kubectl get ingress -n ai-provider

# View logs
kubectl logs -f deployment/ai-provider -n ai-provider

# Port forward for local access
kubectl port-forward svc/ai-provider-service 8080:8080 -n ai-provider
```

### Access the API

```bash
# Get the API endpoint
export API_URL=$(kubectl get ingress ai-provider-ingress -n ai-provider -o jsonpath='{.spec.rules[0].host}')

# Health check
curl https://$API_URL/health

# API endpoint
curl https://$API_URL/api/v1/models
```

### Create Initial Admin User

```bash
# Get admin credentials from secret
kubectl get secret ai-provider-admin-credentials -n ai-provider -o jsonpath='{.data.ADMIN_USERNAME}' | base64 -d
kubectl get secret ai-provider-admin-credentials -n ai-provider -o jsonpath='{.data.ADMIN_PASSWORD}' | base64 -d

# Login
curl -X POST https://$API_URL/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"<password>"}'
```

## Security Considerations

### Secrets Management

1. **Production Secrets**: Use external secret management (HashiCorp Vault, AWS Secrets Manager, Azure Key Vault)
2. **Change Default Passwords**: Always change default passwords before production deployment
3. **Secret Encryption**: Enable encryption at rest for Kubernetes secrets
4. **RBAC**: Review and customize RBAC permissions based on your security requirements

### Network Security

1. **Network Policies**: Enable network policies to restrict pod-to-pod communication
2. **TLS**: Always enable TLS for production deployments
3. **Ingress Security**: Configure appropriate ingress annotations for security headers
4. **Firewall Rules**: Configure cloud firewall rules to restrict access

### Pod Security

1. **Security Context**: The chart configures security contexts by default
2. **Non-root Containers**: All containers run as non-root by default
3. **Read-only Filesystem**: Containers use read-only root filesystem where possible
4. **Capability Dropping**: All unnecessary Linux capabilities are dropped

## Troubleshooting

### Common Issues

#### Pods Not Starting

```bash
# Check pod events
kubectl describe pod <pod-name> -n ai-provider

# Check pod logs
kubectl logs <pod-name> -n ai-provider

# Check resource constraints
kubectl top pods -n ai-provider
kubectl describe resourcequota -n ai-provider
```

#### Database Connection Issues

```bash
# Test database connectivity
kubectl run pg-test --rm -it --image=postgres:15 --restart=Never -- \
  psql postgresql://ai_admin:<password>@postgres:5432/ai_provider

# Check PostgreSQL logs
kubectl logs statefulset/postgres -n ai-provider
```

#### Ingress Not Working

```bash
# Check ingress controller logs
kubectl logs -n ingress-nginx deployment/ingress-nginx-controller

# Verify ingress configuration
kubectl describe ingress ai-provider-ingress -n ai-provider

# Check DNS resolution
nslookup api.ai-provider.example.com
```

### Debug Mode

```bash
# Enable debug logging
helm upgrade ai-provider ./ai-provider \
  --namespace ai-provider \
  --set config.logLevel=debug \
  --reuse-values

# View debug logs
kubectl logs -f deployment/ai-provider -n ai-provider
```

### Getting Help

- **Documentation**: [https://docs.ai-provider.io](https://docs.ai-provider.io)
- **GitHub Issues**: [https://github.com/ai-provider/ai-provider/issues](https://github.com/ai-provider/ai-provider/issues)
- **Community Slack**: [https://slack.ai-provider.io](https://slack.ai-provider.io)
- **Email Support**: support@ai-provider.io

## Contributing

We welcome contributions! Please see our [Contributing Guide](https://github.com/ai-provider/ai-provider/blob/main/CONTRIBUTING.md) for details.

## License

This Helm chart is licensed under the Apache License 2.0. See [LICENSE](https://github.com/ai-provider/ai-provider/blob/main/LICENSE) for details.

## Changelog

### v1.0.0 (2025-03-18)

- Initial release
- Full AI Provider platform deployment
- High availability support
- Monitoring and alerting integration
- Security hardening
- GPU support
- Multi-environment configurations

---

**Maintained by the AI Provider Team**