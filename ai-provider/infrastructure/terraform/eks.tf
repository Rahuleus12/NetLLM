# ==============================================================================
# AI Provider - EKS Cluster Configuration
# ==============================================================================
# This file defines the EKS cluster configuration including:
# - EKS cluster with control plane
# - Managed node groups (general and GPU)
# - IAM roles for service accounts (IRSA)
# - Cluster encryption
# - Add-ons and extensions
# ==============================================================================

# ==============================================================================
# Data Sources
# ==============================================================================

# Get current AWS caller identity
data "aws_caller_identity" "current" {}

# Get current region
data "aws_region" "current" {}

# Get available availability zones
data "aws_availability_zones" "available" {
  state = "available"
}

# Get latest EKS optimized AMI
data "aws_ami" "eks" {
  most_recent = true
  owners      = ["amazon"]

  filter {
    name   = "name"
    values = ["amazon-eks-node-${var.eks_cluster_version}-v*"]
  }
}

# Get latest EKS GPU optimized AMI
data "aws_ami" "eks_gpu" {
  most_recent = true
  owners      = ["amazon"]

  filter {
    name   = "name"
    values = ["amazon-eks-gpu-node-${var.eks_cluster_version}-v*"]
  }
}

# ==============================================================================
# KMS Key for EKS Encryption
# ==============================================================================

# KMS key for EKS secrets encryption
resource "aws_kms_key" "eks" {
  count = var.eks_cluster_encryption_config.enabled ? 1 : 0

  description             = "KMS key for EKS cluster ${local.eks_cluster_name} encryption"
  deletion_window_in_days = 7
  enable_key_rotation     = true

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid    = "Enable IAM User Permissions"
        Effect = "Allow"
        Principal = {
          AWS = "arn:aws:iam::${data.aws_caller_identity.current.account_id}:root"
        }
        Action   = "kms:*"
        Resource = "*"
      },
      {
        Sid    = "Allow EKS to use the key"
        Effect = "Allow"
        Principal = {
          Service = "eks.amazonaws.com"
        }
        Action = [
          "kms:Encrypt",
          "kms:Decrypt",
          "kms:ReEncrypt*",
          "kms:GenerateDataKey*",
          "kms:DescribeKey"
        ]
        Resource = "*"
      }
    ]
  })

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-eks-encryption-key"
  })
}

# KMS alias for easier reference
resource "aws_kms_alias" "eks" {
  count = var.eks_cluster_encryption_config.enabled ? 1 : 0

  name          = "alias/${local.name_prefix}-eks"
  target_key_id = aws_kms_key.eks[0].key_id
}

# ==============================================================================
# IAM Roles for EKS Cluster
# ==============================================================================

# IAM role for EKS cluster
resource "aws_iam_role" "eks_cluster" {
  name = "${local.name_prefix}-eks-cluster-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Principal = {
          Service = "eks.amazonaws.com"
        }
      }
    ]
  })

  path = var.iam_path

  permissions_boundary = var.iam_permissions_boundary != "" ? var.iam_permissions_boundary : null

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-eks-cluster-role"
  })
}

# Attach AmazonEKSClusterPolicy
resource "aws_iam_role_policy_attachment" "eks_cluster_policy" {
  policy_arn = "arn:aws:iam::aws:policy/AmazonEKSClusterPolicy"
  role       = aws_iam_role.eks_cluster.name
}

# Attach AmazonEKSVPCResourceController
resource "aws_iam_role_policy_attachment" "eks_vpc_resource_controller" {
  policy_arn = "arn:aws:iam::aws:policy/AmazonEKSVPCResourceController"
  role       = aws_iam_role.eks_cluster.name
}

# Attach AmazonEKSServicePolicy (optional, for older versions)
resource "aws_iam_role_policy_attachment" "eks_service_policy" {
  policy_arn = "arn:aws:iam::aws:policy/AmazonEKSServicePolicy"
  role       = aws_iam_role.eks_cluster.name
}

# Additional IAM policy for EKS cluster
resource "aws_iam_role_policy" "eks_cluster_additional" {
  name = "${local.name_prefix}-eks-cluster-additional"
  role = aws_iam_role.eks_cluster.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "ec2:DescribeInstances",
          "ec2:DescribeRouteTables",
          "ec2:DescribeSecurityGroups",
          "ec2:DescribeSubnets",
          "ec2:DescribeVpcs",
          "ecr:GetAuthorizationToken",
          "ecr:BatchCheckLayerAvailability",
          "ecr:GetDownloadUrlForLayer",
          "ecr:BatchGetImage"
        ]
        Resource = "*"
      }
    ]
  })
}

# ==============================================================================
# EKS Cluster Security Group
# ==============================================================================

# Security group for EKS cluster
resource "aws_security_group" "eks_cluster" {
  name        = "${local.name_prefix}-eks-cluster-sg"
  description = "Security group for EKS cluster control plane"
  vpc_id      = module.vpc.vpc_id

  # Egress: Allow all outbound traffic
  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
    description = "Allow all outbound traffic"
  }

  # Ingress: Allow worker nodes to communicate with cluster API
  ingress {
    from_port = 443
    to_port   = 443
    protocol  = "tcp"
    self      = true
    description = "Allow worker nodes to communicate with cluster API"
  }

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-eks-cluster-sg"
  })
}

# Security group rule: Allow nodes to communicate with cluster API
resource "aws_security_group_rule" "cluster_ingress_nodes" {
  type                     = "ingress"
  from_port                = 443
  to_port                  = 443
  protocol                 = "tcp"
  security_group_id        = aws_security_group.eks_cluster.id
  source_security_group_id = aws_security_group.eks_nodes.id
  description              = "Allow worker nodes to communicate with cluster API"
}

# ==============================================================================
# EKS Cluster
# ==============================================================================

# EKS cluster
resource "aws_eks_cluster" "main" {
  name     = local.eks_cluster_name
  version  = var.eks_cluster_version
  role_arn = aws_iam_role.eks_cluster.arn

  # Enable EKS cluster to use VPC resources
  vpc_config {
    subnet_ids              = module.vpc.private_subnet_ids
    endpoint_private_access = var.eks_endpoint_private_access
    endpoint_public_access  = var.eks_endpoint_public_access
    public_access_cidrs     = var.eks_public_access_cidrs
    security_group_ids      = [aws_security_group.eks_cluster.id]
  }

  # Encryption configuration
  dynamic "encryption_config" {
    for_each = var.eks_cluster_encryption_config.enabled ? [1] : []
    content {
      provider {
        key_arn = aws_kms_key.eks[0].arn
      }
      resources = var.eks_cluster_encryption_config.resources
    }
  }

  # Kubernetes network configuration
  kubernetes_network_config {
    service_ipv4_cidr = "172.20.0.0/16"
  }

  # Enable EKS cluster logging
  enabled_cluster_log_types = var.eks_cluster_enabled_log_types

  tags = merge(local.merged_tags, {
    Name = local.eks_cluster_name
  })

  # Ensure IAM role permissions are created before cluster
  depends_on = [
    aws_iam_role_policy_attachment.eks_cluster_policy,
    aws_iam_role_policy_attachment.eks_vpc_resource_controller,
    aws_cloudwatch_log_group.eks_cluster
  ]

  # Enable OIDC provider for IRSA
  # Note: This is automatically enabled in EKS 1.21+ and cannot be disabled
}

# CloudWatch log group for EKS cluster logs
resource "aws_cloudwatch_log_group" "eks_cluster" {
  name              = "/aws/eks/${local.eks_cluster_name}/cluster"
  retention_in_days = var.eks_cluster_log_retention_days

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-eks-logs"
  })
}

# ==============================================================================
# OIDC Provider for IAM Roles for Service Accounts (IRSA)
# ==============================================================================

# OIDC provider for IRSA
data "aws_iam_openid_connect_provider" "eks" {
  url = aws_eks_cluster.main.identity[0].oidc[0].issuer
}

# IAM role for OIDC provider
resource "aws_iam_role" "eks_oidc_provider" {
  count = var.eks_enable_irsa ? 1 : 0

  name = "${local.name_prefix}-eks-oidc-provider-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Principal = {
          Federated = aws_eks_cluster.main.identity[0].oidc[0].issuer
        }
        Action = "sts:AssumeRoleWithWebIdentity"
        Condition = {
          StringEquals = {
            "${replace(aws_eks_cluster.main.identity[0].oidc[0].issuer, "https://", "")}:sub" = "system:serviceaccount:*"
          }
        }
      }
    ]
  })

  path = var.iam_path

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-eks-oidc-provider-role"
  })
}

# ==============================================================================
# IAM Roles for Node Groups
# ==============================================================================

# IAM role for EKS node group
resource "aws_iam_role" "eks_nodes" {
  name = "${local.name_prefix}-eks-node-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Principal = {
          Service = "ec2.amazonaws.com"
        }
      }
    ]
  })

  path = var.iam_path

  permissions_boundary = var.iam_permissions_boundary != "" ? var.iam_permissions_boundary : null

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-eks-node-role"
  })
}

# Attach AmazonEKSWorkerNodePolicy
resource "aws_iam_role_policy_attachment" "eks_worker_node_policy" {
  policy_arn = "arn:aws:iam::aws:policy/AmazonEKSWorkerNodePolicy"
  role       = aws_iam_role.eks_nodes.name
}

# Attach AmazonEKS_CNI_Policies
resource "aws_iam_role_policy_attachment" "eks_cni_policy" {
  policy_arn = "arn:aws:iam::aws:policy/AmazonEKS_CNI_Policy"
  role       = aws_iam_role.eks_nodes.name
}

# Attach AmazonEC2ContainerRegistryReadOnly
resource "aws_iam_role_policy_attachment" "eks_ecr_readonly" {
  policy_arn = "arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly"
  role       = aws_iam_role.eks_nodes.name
}

# Attach AmazonSSMManagedInstanceCore for SSM access
resource "aws_iam_role_policy_attachment" "eks_ssm_managed_instance" {
  policy_arn = "arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore"
  role       = aws_iam_role.eks_nodes.name
}

# Additional IAM policy for EKS nodes
resource "aws_iam_role_policy" "eks_nodes_additional" {
  name = "${local.name_prefix}-eks-nodes-additional"
  role = aws_iam_role.eks_nodes.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "ec2:Describe*",
          "ecr:GetAuthorizationToken",
          "ecr:BatchCheckLayerAvailability",
          "ecr:GetDownloadUrlForLayer",
          "ecr:BatchGetImage",
          "s3:GetObject",
          "s3:ListBucket",
          "kms:Decrypt",
          "kms:GenerateDataKey"
        ]
        Resource = "*"
      }
    ]
  })
}

# ==============================================================================
# Security Group for EKS Nodes
# ==============================================================================

# Security group for EKS nodes
resource "aws_security_group" "eks_nodes" {
  name        = "${local.name_prefix}-eks-nodes-sg"
  description = "Security group for EKS worker nodes"
  vpc_id      = module.vpc.vpc_id

  # Egress: Allow all outbound traffic
  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
    description = "Allow all outbound traffic"
  }

  # Ingress: Allow nodes to communicate with each other
  ingress {
    from_port = 0
    to_port   = 65535
    protocol  = "-1"
    self      = true
    description = "Allow nodes to communicate with each other"
  }

  # Ingress: Allow cluster API to communicate with nodes
  ingress {
    from_port                = 1025
    to_port                  = 65535
    protocol                 = "tcp"
    source_security_group_id = aws_security_group.eks_cluster.id
    description              = "Allow cluster API to communicate with nodes"
  }

  # Ingress: Allow HTTPS traffic from load balancer
  ingress {
    from_port       = 443
    to_port         = 443
    protocol        = "tcp"
    security_groups = [aws_security_group.eks_cluster.id]
    description     = "Allow HTTPS traffic from cluster"
  }

  tags = merge(local.merged_tags, {
    Name                                           = "${local.name_prefix}-eks-nodes-sg"
    "kubernetes.io/cluster/${local.eks_cluster_name}" = "owned"
    "k8s.io/cluster-autoscaler/enabled"            = "true"
    "k8s.io/cluster-autoscaler/${local.eks_cluster_name}" = "owned"
  })
}

# Security group rule: Allow nodes to access cluster API
resource "aws_security_group_rule" "nodes_ingress_cluster" {
  type                     = "ingress"
  from_port                = 443
  to_port                  = 443
  protocol                 = "tcp"
  security_group_id        = aws_security_group.eks_nodes.id
  source_security_group_id = aws_security_group.eks_cluster.id
  description              = "Allow nodes to access cluster API"
}

# ==============================================================================
# EKS Managed Node Groups
# ==============================================================================

# Create node groups based on configuration
resource "aws_eks_node_group" "main" {
  for_each = var.eks_node_groups

  cluster_name    = aws_eks_cluster.main.name
  node_group_name = "${local.name_prefix}-${each.key}"
  node_role_arn   = aws_iam_role.eks_nodes.arn
  subnet_ids      = module.vpc.private_subnet_ids

  # Instance configuration
  instance_types = each.value.instance_types
  capacity_type  = each.value.capacity_type
  disk_size      = each.value.disk_size

  # Scaling configuration
  scaling_config {
    desired_size = each.value.desired_size
    min_size     = each.value.min_size
    max_size     = each.value.max_size
  }

  # Update configuration
  update_config {
    max_unavailable = each.value.max_unavailable
  }

  # Labels
  labels = merge(each.value.labels, {
    Environment = var.environment
    NodeGroup   = each.key
  })

  # Taints
  dynamic "taint" {
    for_each = each.value.taints
    content {
      key    = taint.key
      value  = taint.value
      effect = "NO_SCHEDULE"
    }
  }

  # Tags
  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-${each.key}-node-group"
    NodeGroupType = each.key
  })

  # Ensure IAM role is created before node group
  depends_on = [
    aws_iam_role_policy_attachment.eks_worker_node_policy,
    aws_iam_role_policy_attachment.eks_cni_policy,
    aws_iam_role_policy_attachment.eks_ecr_readonly
  ]

  lifecycle {
    ignore_changes = [scaling_config[0].desired_size]
  }
}

# ==============================================================================
# EKS Add-ons
# ==============================================================================

# VPC CNI add-on
resource "aws_eks_addon" "vpc_cni" {
  cluster_name                = aws_eks_cluster.main.name
  addon_name                  = "vpc-cni"
  addon_version               = coalesce(
    [for addon in var.eks_addons : addon.version if addon.name == "vpc-cni"][0],
    "v1.14.0-eksbuild.1"
  )
  service_account_role_arn   = aws_iam_role.eks_nodes.arn
  resolve_conflicts_on_create = "OVERWRITE"
  resolve_conflicts_on_update = "OVERWRITE"

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-vpc-cni-addon"
  })

  depends_on = [aws_eks_node_group.main]
}

# CoreDNS add-on
resource "aws_eks_addon" "coredns" {
  cluster_name                = aws_eks_cluster.main.name
  addon_name                  = "coredns"
  addon_version               = coalesce(
    [for addon in var.eks_addons : addon.version if addon.name == "coredns"][0],
    "v1.10.1-eksbuild.1"
  )
  service_account_role_arn   = aws_iam_role.eks_nodes.arn
  resolve_conflicts_on_create = "OVERWRITE"
  resolve_conflicts_on_update = "OVERWRITE"

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-coredns-addon"
  })

  depends_on = [aws_eks_node_group.main]
}

# Kube-proxy add-on
resource "aws_eks_addon" "kube_proxy" {
  cluster_name                = aws_eks_cluster.main.name
  addon_name                  = "kube-proxy"
  addon_version               = coalesce(
    [for addon in var.eks_addons : addon.version if addon.name == "kube-proxy"][0],
    "v1.28.1-eksbuild.1"
  )
  service_account_role_arn   = aws_iam_role.eks_nodes.arn
  resolve_conflicts_on_create = "OVERWRITE"
  resolve_conflicts_on_update = "OVERWRITE"

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-kube-proxy-addon"
  })

  depends_on = [aws_eks_node_group.main]
}

# AWS EBS CSI Driver add-on
resource "aws_eks_addon" "ebs_csi_driver" {
  count = contains([for addon in var.eks_addons : addon.name], "aws-ebs-csi-driver") ? 1 : 0

  cluster_name                = aws_eks_cluster.main.name
  addon_name                  = "aws-ebs-csi-driver"
  addon_version               = coalesce(
    [for addon in var.eks_addons : addon.version if addon.name == "aws-ebs-csi-driver"][0],
    "v1.24.0-eksbuild.1"
  )
  service_account_role_arn   = aws_iam_role.eks_nodes.arn
  resolve_conflicts_on_create = "OVERWRITE"
  resolve_conflicts_on_update = "OVERWRITE"

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-ebs-csi-driver-addon"
  })

  depends_on = [aws_eks_node_group.main]
}

# ==============================================================================
# Fargate Profiles
# ==============================================================================

# Create Fargate profiles if configured
resource "aws_eks_fargate_profile" "main" {
  for_each = var.eks_fargate_profiles

  cluster_name           = aws_eks_cluster.main.name
  fargate_profile_name   = "${local.name_prefix}-${each.key}"
  pod_execution_role_arn = aws_iam_role.eks_fargate[each.key].arn
  subnet_ids             = coalesce(each.value.subnet_ids, module.vpc.private_subnet_ids)

  dynamic "selector" {
    for_each = each.value.selectors
    content {
      namespace = selector.value.namespace
      labels    = selector.value.labels
    }
  }

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-${each.key}-fargate-profile"
  })

  depends_on = [aws_eks_cluster.main]
}

# IAM role for Fargate profiles
resource "aws_iam_role" "eks_fargate" {
  for_each = var.eks_fargate_profiles

  name = "${local.name_prefix}-fargate-${each.key}-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Principal = {
          Service = "eks-fargate-pods.amazonaws.com"
        }
      }
    ]
  })

  path = var.iam_path

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-fargate-${each.key}-role"
  })
}

# Attach AmazonEKSFargatePodExecutionRolePolicy
resource "aws_iam_role_policy_attachment" "eks_fargate" {
  for_each = var.eks_fargate_profiles

  policy_arn = "arn:aws:iam::aws:policy/AmazonEKSFargatePodExecutionRolePolicy"
  role       = aws_iam_role.eks_fargate[each.key].name
}

# ==============================================================================
# Cluster Autoscaler IAM Role (Optional)
# ==============================================================================

# IAM role for cluster autoscaler
resource "aws_iam_role" "cluster_autoscaler" {
  count = var.enable_cluster_autoscaler ? 1 : 0

  name = "${local.name_prefix}-cluster-autoscaler-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Principal = {
          Federated = aws_eks_cluster.main.identity[0].oidc[0].issuer
        }
        Action = "sts:AssumeRoleWithWebIdentity"
        Condition = {
          StringEquals = {
            "${replace(aws_eks_cluster.main.identity[0].oidc[0].issuer, "https://", "")}:sub" = "system:serviceaccount:kube-system:cluster-autoscaler"
          }
        }
      }
    ]
  })

  path = var.iam_path

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-cluster-autoscaler-role"
  })
}

# IAM policy for cluster autoscaler
resource "aws_iam_role_policy" "cluster_autoscaler" {
  count = var.enable_cluster_autoscaler ? 1 : 0

  name = "${local.name_prefix}-cluster-autoscaler-policy"
  role = aws_iam_role.cluster_autoscaler[0].id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "autoscaling:DescribeAutoScalingGroups",
          "autoscaling:DescribeAutoScalingInstances",
          "autoscaling:DescribeLaunchConfigurations",
          "autoscaling:DescribeTags",
          "autoscaling:SetDesiredCapacity",
          "autoscaling:TerminateInstanceInAutoScalingGroup",
          "ec2:DescribeLaunchTemplateVersions"
        ]
        Resource = "*"
      }
    ]
  })
}

# ==============================================================================
# Outputs
# ==============================================================================

output "eks_cluster_id" {
  description = "EKS cluster ID"
  value       = aws_eks_cluster.main.id
}

output "eks_cluster_arn" {
  description = "EKS cluster ARN"
  value       = aws_eks_cluster.main.arn
}

output "eks_cluster_name" {
  description = "EKS cluster name"
  value       = aws_eks_cluster.main.name
}

output "eks_cluster_endpoint" {
  description = "EKS cluster endpoint"
  value       = aws_eks_cluster.main.endpoint
}

output "eks_cluster_version" {
  description = "EKS cluster version"
  value       = aws_eks_cluster.main.version
}

output "eks_cluster_certificate_authority_data" {
  description = "EKS cluster certificate authority data"
  value       = aws_eks_cluster.main.certificate_authority[0].data
  sensitive   = true
}

output "eks_cluster_oidc_issuer_url" {
  description = "EKS cluster OIDC issuer URL"
  value       = aws_eks_cluster.main.identity[0].oidc[0].issuer
}

output "eks_cluster_primary_security_group_id" {
  description = "EKS cluster primary security group ID"
  value       = aws_eks_cluster.main.vpc_config[0].cluster_security_group_id
}

output "eks_node_groups" {
  description = "EKS node groups"
  value       = aws_eks_node_group.main
}

output "eks_node_security_group_id" {
  description = "EKS node security group ID"
  value       = aws_security_group.eks_nodes.id
}

output "eks_cluster_security_group_id" {
  description = "EKS cluster security group ID"
  value       = aws_security_group.eks_cluster.id
}

output "eks_node_role_arn" {
  description = "EKS node IAM role ARN"
  value       = aws_iam_role.eks_nodes.arn
}

output "eks_cluster_role_arn" {
  description = "EKS cluster IAM role ARN"
  value       = aws_iam_role.eks_cluster.arn
}

output "eks_kubeconfig_command" {
  description = "Command to update kubeconfig"
  value       = "aws eks update-kubeconfig --name ${aws_eks_cluster.main.name} --region ${data.aws_region.current.name}"
}

output "eks_kubeconfig" {
  description = "kubectl configuration for EKS cluster"
  value = templatefile("${path.module}/templates/kubeconfig.tpl", {
    cluster_name    = aws_eks_cluster.main.name
    cluster_endpoint = aws_eks_cluster.main.endpoint
    cluster_ca      = aws_eks_cluster.main.certificate_authority[0].data
    region          = data.aws_region.current.name
  })
  sensitive = true
}

# ==============================================================================
# EKS Module Usage Example
# ==============================================================================

# To use this EKS configuration, you need to:
#
# 1. Ensure VPC module is available and configured
# 2. Configure variables in variables.tf
# 3. Run terraform init, plan, apply
#
# After deployment:
#
# 1. Configure kubectl:
#    aws eks update-kubeconfig --name ${aws_eks_cluster.main.name} --region ${data.aws_region.current.name}
#
# 2. Verify cluster:
#    kubectl get nodes
#    kubectl get pods -A
#
# 3. Deploy workloads:
#    kubectl apply -f your-deployment.yaml
#
# 4. Install additional tools (optional):
#    - Cluster Autoscaler
#    - Metrics Server
#    - AWS Load Balancer Controller
#    - EBS CSI Driver
#    - Cluster Autoscaler
#
# Security best practices:
#
# 1. Enable private endpoint access for production
# 2. Restrict public endpoint access to specific CIDR blocks
# 3. Enable encryption at rest
# 4. Use IAM roles for service accounts (IRSA)
# 5. Regularly update cluster version
# 6. Enable cluster logging
# 7. Use security groups to control access
# 8. Implement network policies
# 9. Use pod security policies or standards
# 10. Regularly review and audit cluster access
