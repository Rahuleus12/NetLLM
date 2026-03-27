# AI Provider - Terraform Infrastructure

[![Terraform](https://img.shields.io/badge/Terraform-1.5%2B-623CE4?logo=terraform)](https://terraform.io)
[![AWS](https://img.shields.io/badge/AWS-5.0%2B-FF9900?logo=amazon-aws)](https://aws.amazon.com)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

Production-grade Terraform infrastructure for the AI Provider platform on AWS. This repository contains complete Infrastructure as Code (IaC) for deploying a scalable, secure, and highly available AI model management platform.

## 📋 Table of Contents

- [Architecture](#architecture)
- [Features](#features)
- [Prerequisites](#prerequisites)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [Deployment](#deployment)
- [Post-Deployment](#post-deployment)
- [Security](#security)
- [Cost Estimation](#cost-estimation)
- [Troubleshooting](#troubleshooting)
- [Contributing](#contributing)

## 🏗 Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                              AWS Cloud                                    │
│  ┌───────────────────────────────────────────────────────────────────┐  │
│  │                            VPC (10.0.0.0/16)                       │  │
│  │                                                                    │  │
│  │  ┌─────────────────────┐      ┌─────────────────────┐            │  │
│  │  │  Public Subnets     │      │  Private Subnets    │            │  │
│  │  │  (10.0.1-3.0/24)    │      │  (10.0.10-12.0/24)  │            │  │
│  │  │                     │      │                     │            │  │
│  │  │  ┌───────────────┐  │      │  ┌───────────────┐  │            │  │
│  │  │  │  NAT Gateway  │  │      │  │ EKS Cluster   │  │            │  │
│  │  │  │  (x3 HA)      │◄─┼──────┼──│ (Kubernetes)  │  │            │  │
│  │  │  └───────────────┘  │      │  │               │  │            │  │
│  │  │                     │      │  │ ┌───────────┐ │  │            │  │
│  │  │  ┌───────────────┐  │      │  │ │API Server │ │  │            │  │
│  │  │  │  ALB/Ingress  │  │      │  │ │Workers    │ │  │            │  │
│  │  │  │  (Public)     │  │      │  │ │GPU Nodes  │ │  │            │  │
│  │  │  └───────────────┘  │      │  │ └───────────┘ │  │            │  │
│  │  └─────────────────────┘      │  └───────────────┘  │            │  │
│  │                               │                     │            │  │
│  │  ┌────────────────────────────┴─────────────────────┴──────────┐ │  │
│  │  │              Database Subnets (10.0.20-22.0/24)              │ │  │
│  │  │                                                               │ │  │
│  │  │  ┌─────────────────┐         ┌─────────────────┐            │ │  │
│  │  │  │  RDS PostgreSQL │         │  ElastiCache    │            │ │  │
│  │  │  │  (Multi-AZ)     │         │  Redis Cluster  │            │ │  │
│  │  │  │  Primary + Standby│       │  (Cluster Mode) │            │ │  │
│  │  │  └─────────────────┘         └─────────────────┘            │ │  │
│  │  └───────────────────────────────────────────────────────────────┘ │  │
│  └────────────────────────────────────────────────────────────────────┘  │
│                                                                          │
│  ┌───────────────────────────────────────────────────────────────────┐  │
│  │                         AWS Services                               │  │
│  │                                                                    │  │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐         │  │
│  │  │   S3     │  │ Secrets  │  │CloudWatch│  │   KMS    │         │  │
│  │  │ Buckets  │  │ Manager  │  │ Monitor  │  │  Keys    │         │  │
│  │  └──────────┘  └──────────┘  └──────────┘  └──────────┘         │  │
│  │                                                                    │  │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐         │  │
│  │  │   IAM    │  │Route 53  │  │   ACM    │  │GuardDuty │         │  │
│  │  │  Roles   │  │   DNS    │  │   TLS    │  │ Security │         │  │
│  │  └──────────┘  └──────────┘  └──────────┘  └──────────┘         │  │
│  └────────────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────────┘
```

### Components

| Component | Service | Purpose | HA |
|-----------|---------|---------|-----|
| **Compute** | EKS 1.28+ | Kubernetes cluster for AI workloads | ✅ |
| **Database** | RDS PostgreSQL 15.4 | Primary data store | ✅ Multi-AZ |
| **Cache** | ElastiCache Redis 7.0 | Caching and session storage | ✅ Cluster Mode |
| **Storage** | S3 | Models, cache, backups, logs | ✅ Cross-Region Replication |
| **Network** | VPC, NAT GW, ALB | Network infrastructure | ✅ 3 AZs |
| **Security** | KMS, IAM, SG | Encryption and access control | ✅ |
| **Monitoring** | CloudWatch | Logs, metrics, alerts | ✅ |

## ✨ Features

### 🚀 Core Infrastructure
- **EKS Cluster**: Managed Kubernetes with managed node groups
- **GPU Support**: NVIDIA GPU nodes for AI model inference
- **Auto-scaling**: Cluster autoscaler and horizontal pod autoscaling
- **Multi-AZ**: High availability across 3 availability zones

### 🗄️ Data Layer
- **RDS PostgreSQL**: Multi-AZ deployment with automated backups
- **ElastiCache Redis**: Cluster mode with automatic failover
- **S3 Buckets**: Multiple buckets for different use cases
- **Cross-Region Replication**: Disaster recovery support

### 🔒 Security
- **Encryption at Rest**: KMS encryption for all data stores
- **Encryption in Transit**: TLS for all communications
- **Network Security**: Security groups, NACLs, VPC endpoints
- **IAM Roles**: Least privilege access with IRSA support
- **Secrets Management**: AWS Secrets Manager integration

### 📊 Monitoring & Operations
- **CloudWatch Dashboards**: Comprehensive monitoring dashboards
- **CloudWatch Alarms**: Automated alerting for critical metrics
- **VPC Flow Logs**: Network traffic monitoring
- **Cost Management**: Budget alerts and cost tracking

### 🔄 GitOps Ready
- **ArgoCD Compatible**: Pre-configured for GitOps workflows
- **Flux Support**: Alternative GitOps tooling support
- **Terraform State**: Remote state with S3 and DynamoDB locking

## 📋 Prerequisites

### Required Tools

| Tool | Version | Purpose |
|------|---------|---------|
| [Terraform](https://terraform.io/downloads) | >= 1.5.0 | Infrastructure provisioning |
| [AWS CLI](https://aws.amazon.com/cli/) | >= 2.0 | AWS interaction |
| [kubectl](https://kubernetes.io/docs/tasks/tools/) | >= 1.28.0 | Kubernetes management |
| [Helm](https://helm.sh/docs/intro/install/) | >= 3.8.0 | Package management |

### AWS Requirements

- **AWS Account**: With appropriate permissions
- **IAM User/Role**: With administrator or PowerUser access
- **Service Quotas**: Ensure sufficient quotas for:
  - EC2 Instances (vCPU limits)
  - RDS Instances
  - ElastiCache Nodes
  - EKS Clusters
  - VPCs and NAT Gateways

### Knowledge Requirements

- Basic Terraform knowledge
- AWS services understanding
- Kubernetes fundamentals
- Networking concepts (VPC, subnets, routing)

## 🚀 Quick Start

### 1. Clone and Initialize

```bash
# Clone the repository
git clone <repository-url>
cd ai-provider/infrastructure/terraform

# Initialize Terraform
terraform init
```

### 2. Configure Variables

```bash
# Copy example variables
cp terraform.tfvars.example terraform.tfvars

# Edit variables
vim terraform.tfvars
```

### 3. Deploy Infrastructure

```bash
# Plan deployment
terraform plan -out=tfplan

# Apply configuration
terraform apply tfplan
```

### 4. Configure kubectl

```bash
# Get kubectl configuration command from outputs
terraform output eks_kubeconfig_command

# Configure kubectl
aws eks update-kubeconfig --name <cluster-name> --region <region>

# Verify connection
kubectl get nodes
```

## ⚙️ Configuration

### Environment-Specific Configurations

#### Development
```hcl
environment = "dev"

# Cost-optimized settings
eks_node_groups = {
  general = {
    instance_types = ["m5.large"]
    desired_size   = 2
    min_size       = 1
    max_size       = 5
  }
}

rds_multi_az                    = false
elasticache_num_node_groups     = 1
enable_nat_gateway              = true
single_nat_gateway              = true
```

#### Staging
```hcl
environment = "staging"

# Balanced settings
eks_node_groups = {
  general = {
    instance_types = ["m5.xlarge"]
    desired_size   = 3
    min_size       = 2
    max_size       = 10
  }
}

rds_multi_az                    = true
elasticache_num_node_groups     = 2
enable_nat_gateway              = true
single_nat_gateway              = false
```

#### Production
```hcl
environment = "prod"

# High availability settings
eks_node_groups = {
  general = {
    instance_types = ["m5.2xlarge"]
    desired_size   = 5
    min_size       = 3
    max_size       = 20
  }
  gpu = {
    instance_types = ["g4dn.xlarge"]
    desired_size   = 3
    min_size       = 2
    max_size       = 10
  }
}

rds_multi_az                    = true
elasticache_num_node_groups     = 3
enable_nat_gateway              = true
single_nat_gateway              = false
enable_cross_region_replication = true
```

### Key Variables

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `project_name` | Project name for resource naming | `ai-provider` | No |
| `environment` | Environment (dev/staging/prod) | `dev` | No |
| `aws_region` | AWS region for deployment | `us-east-1` | No |
| `vpc_cidr` | CIDR block for VPC | `10.0.0.0/16` | No |
| `eks_cluster_version` | Kubernetes version | `1.28` | No |
| `rds_instance_class` | RDS instance type | `db.r6g.large` | No |
| `elasticache_node_type` | ElastiCache node type | `cache.r6g.large` | No |

### Complete Variable Reference

See [variables.tf](variables.tf) for all available configuration options with descriptions and defaults.

## 🚢 Deployment

### Step-by-Step Deployment

#### 1. Backend Configuration (Optional)

For remote state management, configure a backend:

```hcl
# In versions.tf, uncomment and configure:
terraform {
  backend "s3" {
    bucket         = "your-terraform-state-bucket"
    key            = "ai-provider/terraform.tfstate"
    region         = "us-east-1"
    encrypt        = true
    dynamodb_table = "terraform-locks"
  }
}
```

#### 2. Initialize

```bash
terraform init
```

#### 3. Validate Configuration

```bash
terraform validate
```

#### 4. Plan Deployment

```bash
# Review the execution plan
terraform plan -out=tfplan

# Save plan for review
terraform show tfplan > plan.txt
```

#### 5. Apply Configuration

```bash
# Apply the plan
terraform apply tfplan

# Or apply directly (will show plan first)
terraform apply
```

#### 6. Save Outputs

```bash
# Save outputs to file
terraform output -json > outputs.json

# View infrastructure summary
terraform output infrastructure_summary
```

### Deployment Times

| Component | Estimated Time |
|-----------|---------------|
| VPC & Networking | 5-10 minutes |
| EKS Cluster | 15-20 minutes |
| RDS PostgreSQL | 10-15 minutes |
| ElastiCache Redis | 8-12 minutes |
| S3 & IAM | 2-5 minutes |
| **Total** | **40-60 minutes** |

### Targeted Deployment

Deploy specific components:

```bash
# Deploy only VPC
terraform apply -target=module.vpc

# Deploy only EKS
terraform apply -target=module.eks

# Deploy only RDS
terraform apply -target=module.rds
```

## 🔄 Post-Deployment

### 1. Configure kubectl

```bash
# Get cluster name
CLUSTER_NAME=$(terraform output -raw eks_cluster_name)
REGION=$(terraform output -raw aws_region)

# Update kubeconfig
aws eks update-kubeconfig --name $CLUSTER_NAME --region $REGION

# Verify cluster access
kubectl cluster-info
kubectl get nodes
```

### 2. Install Essential Add-ons

```bash
# Install AWS Load Balancer Controller
helm repo add eks-charts https://aws.github.io/eks-charts
helm install aws-load-balancer-controller eks-charts/aws-load-balancer-controller \
  -n kube-system \
  --set clusterName=$CLUSTER_NAME \
  --set serviceAccount.create=false \
  --set serviceAccount.name=aws-load-balancer-controller

# Install Cluster Autoscaler
kubectl apply -f - <<EOF
apiVersion: v1
kind: ServiceAccount
metadata:
  name: cluster-autoscaler
  namespace: kube-system
  annotations:
    eks.amazonaws.com/role-arn: $(terraform output -raw iam_eks_cluster_autoscaler_role_arn)
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: cluster-autoscaler
  namespace: kube-system
spec:
  selector:
    matchLabels:
      app: cluster-autoscaler
  template:
    metadata:
      labels:
        app: cluster-autoscaler
    spec:
      serviceAccountName: cluster-autoscaler
      containers:
      - image: k8s.gcr.io/autoscaling/cluster-autoscaler:v1.28.0
        name: cluster-autoscaler
        command:
        - ./cluster-autoscaler
        - --v=4
        - --stderrthreshold=info
        - --cloud-provider=aws
        - --skip-nodes-with-local-storage=false
        - --expander=least-waste
        - --node-group-auto-discovery=asg:tag=k8s.io/cluster-autoscaler/enabled,k8s.io/cluster-autoscaler/$CLUSTER_NAME
        resources:
          limits:
            cpu: 100m
            memory: 600Mi
          requests:
            cpu: 100m
            memory: 600Mi
EOF

# Install Metrics Server
kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml
```

### 3. Deploy AI Provider Application

```bash
# Create namespace
kubectl create namespace ai-provider

# Create secrets from AWS Secrets Manager
kubectl create secret generic db-credentials \
  --from-literal=username=$(terraform output -raw rds_master_username) \
  --from-literal=password=<password> \
  --namespace=ai-provider

# Deploy application
kubectl apply -f ../../deployments/kubernetes/manifests/
```

### 4. Verify Deployment

```bash
# Check pod status
kubectl get pods -n ai-provider

# Check services
kubectl get services -n ai-provider

# Check ingress
kubectl get ingress -n ai-provider

# View application logs
kubectl logs -f deployment/ai-provider -n ai-provider
```

### 5. Configure DNS (Optional)

If using Route53:

```bash
# Get load balancer hostname
LB_HOSTNAME=$(kubectl get ingress -n ai-provider ai-provider-ingress -o jsonpath='{.status.loadBalancer.ingress[0].hostname}')

# Create DNS record
aws route53 change-resource-record-sets \
  --hosted-zone-id <zone-id> \
  --change-batch '{
    "Changes": [{
      "Action": "CREATE",
      "ResourceRecordSet": {
        "Name": "api.yourdomain.com",
        "Type": "CNAME",
        "TTL": 300,
        "ResourceRecords": [{"Value": "'$LB_HOSTNAME'"}]
      }
    }]
  }'
```

## 🔒 Security

### Security Features Implemented

✅ **Network Security**
- VPC with private subnets for workloads
- Security groups with least privilege access
- Network ACLs for additional layer of security
- VPC Flow Logs for network monitoring
- VPC endpoints for AWS services

✅ **Encryption**
- KMS encryption for all data at rest
- TLS encryption for data in transit
- Encrypted Kubernetes secrets
- RDS encryption at rest and in transit
- ElastiCache encryption at rest and in transit

✅ **Access Control**
- IAM roles with least privilege
- IRSA (IAM Roles for Service Accounts)
- RBAC for Kubernetes cluster
- Security groups restricting access

✅ **Monitoring & Compliance**
- CloudTrail for API auditing
- GuardDuty for threat detection
- Security Hub for compliance
- CloudWatch for monitoring and alerting

### Security Best Practices

1. **Rotate Credentials Regularly**
   ```bash
   # Rotate RDS master password
   aws rds modify-db-instance \
     --db-instance-identifier <instance-id> \
     --master-user-password <new-password>
   
   # Update Secrets Manager
   aws secretsmanager update-secret \
     --secret-id <secret-arn> \
     --secret-string <new-secret>
   ```

2. **Review IAM Policies**
   ```bash
   # List all IAM roles
   terraform output | grep iam_.*_role_arn
   
   # Review policies attached
   aws iam list-attached-role-policies --role-name <role-name>
   ```

3. **Enable Additional Security Services**
   ```hcl
   # In terraform.tfvars
   enable_guardduty     = true
   enable_security_hub  = true
   enable_config        = true
   enable_cloudtrail    = true
   ```

4. **Regular Security Updates**
   ```bash
   # Update EKS cluster version
   terraform apply -var="eks_cluster_version=1.29"
   
   # Update node groups
   kubectl drain <node-name> --ignore-daemonsets --delete-emptydir-data
   ```

### Security Checklist

- [ ] All S3 buckets have encryption enabled
- [ ] RDS has encryption at rest enabled
- [ ] ElastiCache has encryption enabled
- [ ] EKS cluster has secrets encryption enabled
- [ ] Security groups follow least privilege
- [ ] No security groups allow 0.0.0.0/0 ingress
- [ ] IAM policies follow least privilege
- [ ] CloudTrail is enabled
- [ ] GuardDuty is enabled
- [ ] Security Hub is enabled
- [ ] VPC Flow Logs are enabled
- [ ] Regular security audits scheduled

## 💰 Cost Estimation

### Estimated Monthly Costs (us-east-1)

| Component | Configuration | Monthly Cost |
|-----------|---------------|--------------|
| **EKS Cluster** | 1 cluster | $73.00 |
| **EC2 Nodes (General)** | 3x m5.large | $263.52 |
| **EC2 Nodes (GPU)** | 2x g4dn.xlarge | $405.84 |
| **RDS PostgreSQL** | db.r6g.large Multi-AZ | $290.00 |
| **ElastiCache Redis** | 2x cache.r6g.large | $186.00 |
| **NAT Gateways** | 3x NAT Gateway | $97.20 |
| **S3 Storage** | 500GB models, 100GB cache, 200GB backups | $25.50 |
| **CloudWatch** | Logs, metrics, dashboards | $50.00 |
| **Data Transfer** | ~500GB/month | $45.00 |
| **Other** | KMS, Secrets Manager, etc. | $30.00 |
| **Total (Production)** | | **~$1,466/month** |

### Cost Optimization Tips

1. **Development Environment**
   ```hcl
   # Use single NAT Gateway
   single_nat_gateway = true
   
   # Disable Multi-AZ for RDS
   rds_multi_az = false
   
   # Smaller instance types
   rds_instance_class      = "db.t3.large"
   elasticache_node_type   = "cache.t3.medium"
   eks_node_groups = {
     general = {
       instance_types = ["t3.large"]
       desired_size   = 2
     }
   }
   ```
   **Estimated Dev Cost: ~$400-500/month**

2. **Use Spot Instances**
   ```hcl
   eks_node_groups = {
     general = {
       capacity_type         = "SPOT"
       enable_spot_instances = true
     }
   }
   ```
   **Savings: 60-70% on compute costs**

3. **Reserved Instances**
   - Purchase 1-year or 3-year reserved instances for stable workloads
   - Savings: 30-60% depending on term

4. **Right-Sizing**
   - Monitor resource utilization with CloudWatch
   - Adjust instance types based on actual usage
   - Use Kubernetes resource requests/limits effectively

5. **Storage Optimization**
   - Use S3 lifecycle policies to transition to cheaper storage tiers
   - Implement intelligent tiering for frequently accessed data
   - Regular cleanup of unused resources

### Cost Monitoring

```bash
# Enable Cost Explorer
terraform apply -var="enable_cost_explorer=true"

# Set budget alerts
terraform apply -var="budget_amount=1500" \
                -var="budget_alert_emails=["team@example.com"]"

# View current costs
aws ce get-cost-and-usage \
  --time-period Start=2024-01-01,End=2024-01-31 \
  --granularity MONTHLY \
  --metrics BlendedCost
```

## 🔧 Troubleshooting

### Common Issues

#### 1. Terraform State Lock

**Problem**: State file is locked

**Solution**:
```bash
# Force unlock (use with caution)
terraform force-unlock <lock-id>

# Or wait for the lock to be released automatically
```

#### 2. EKS Cluster Authentication

**Problem**: Unable to connect to EKS cluster

**Solution**:
```bash
# Update kubeconfig
aws eks update-kubeconfig --name <cluster-name> --region <region>

# Verify AWS credentials
aws sts get-caller-identity

# Check cluster status
aws eks describe-cluster --name <cluster-name> --region <region>
```

#### 3. RDS Connection Issues

**Problem**: Cannot connect to RDS instance

**Solution**:
```bash
# Check security group rules
aws ec2 describe-security-groups --group-ids <sg-id>

# Verify subnet group
aws rds describe-db-subnet-groups --db-subnet-group-name <name>

# Test connectivity from EKS pod
kubectl run psql-test --image=postgres:15 --rm -it --restart=Never -- \
  psql -h <rds-endpoint> -U <username> -d <database>
```

#### 4. ElastiCache Connection

**Problem**: Cannot connect to Redis

**Solution**:
```bash
# Check cluster status
aws elasticache describe-replication-groups \
  --replication-group-id <cluster-id>

# Verify security group
aws ec2 describe-security-groups --group-ids <sg-id>

# Test connection
kubectl run redis-test --image=redis:7 --rm -it --restart=Never -- \
  redis-cli -h <redis-endpoint> -p 6379 ping
```

#### 5. Node Group Scaling

**Problem**: Nodes not scaling properly

**Solution**:
```bash
# Check cluster autoscaler logs
kubectl logs -n kube-system deployment/cluster-autoscaler

# Verify node group tags
aws eks describe-nodegroup \
  --cluster-name <cluster-name> \
  --nodegroup-name <nodegroup-name>

# Check ASG
aws autoscaling describe-auto-scaling-groups \
  --auto-scaling-group-name <asg-name>
```

### Useful Commands

```bash
# View Terraform state
terraform state list

# Show specific resource
terraform state show module.eks.aws_eks_cluster.main

# Refresh state
terraform refresh

# Import existing resource
terraform import aws_vpc.main vpc-12345678

# Taint resource for recreation
terraform taint module.rds.aws_db_instance.main

# View outputs
terraform output

# Graph dependencies
terraform graph | dot -Tpng > graph.png
```

### Debugging

```bash
# Enable debug logging
export TF_LOG=DEBUG
terraform apply

# Save debug log
export TF_LOG_PATH=terraform.log
terraform apply

# Kubernetes debugging
kubectl get events --all-namespaces --sort-by='.lastTimestamp'
kubectl describe pod <pod-name> -n ai-provider
kubectl logs <pod-name> -n ai-provider --previous
```

### Getting Help

1. **Check Documentation**: Review this README and inline code comments
2. **Terraform Docs**: https://terraform.io/docs/providers/aws/
3. **AWS Documentation**: https://docs.aws.amazon.com/
4. **Kubernetes Docs**: https://kubernetes.io/docs/
5. **Issue Tracker**: Open an issue in the repository

## 🤝 Contributing

### Development Setup

```bash
# Clone repository
git clone <repository-url>
cd ai-provider/infrastructure/terraform

# Install pre-commit hooks
pip install pre-commit
pre-commit install

# Run validation
terraform validate
terraform fmt -check

# Run security scan
tfsec .
```

### Contribution Guidelines

1. **Fork the repository** and create your branch from `main`
2. **Make changes** following the existing code style
3. **Test changes** in a development environment
4. **Update documentation** if needed
5. **Submit pull request** with clear description

### Code Standards

- Follow [Terraform best practices](https://terraform.io/docs/extend/best-practices/)
- Use consistent naming conventions
- Add descriptive comments
- Validate all configurations
- Test in multiple environments

### Testing

```bash
# Validate syntax
terraform validate

# Format check
terraform fmt -check

# Security scan
tfsec .

# Cost estimation
terraform plan -out=tfplan && tc show tfplan
```

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](../../LICENSE) file for details.

## 🙏 Acknowledgments

- [Terraform AWS Provider](https://github.com/hashicorp/terraform-provider-aws)
- [Amazon EKS Best Practices](https://aws.github.io/aws-eks-best-practices/)
- [Kubernetes Documentation](https://kubernetes.io/docs/)
- [AWS Well-Architected Framework](https://aws.amazon.com/architecture/well-architected/)

## 📞 Support

- **Documentation**: [Wiki](../../wiki)
- **Issues**: [GitHub Issues](../../issues)
- **Discussions**: [GitHub Discussions](../../discussions)

---

**Maintained by the AI Provider Platform Team**

Made with ❤️ by the community