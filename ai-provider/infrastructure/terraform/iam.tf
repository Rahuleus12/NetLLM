# ==============================================================================
# AI Provider - IAM Roles and Policies
# ==============================================================================
# This file defines IAM roles, policies, and instance profiles for the
# AI Provider infrastructure following the principle of least privilege.
# ==============================================================================

# ==============================================================================
# Data Sources
# ==============================================================================

data "aws_caller_identity" "current" {}

data "aws_partition" "current" {}

data "aws_region" "current" {}

# ==============================================================================
# IAM Policy Documents - Common
# ==============================================================================

# Trust policy for EC2 services
data "aws_iam_policy_document" "ec2_assume_role" {
  statement {
    sid     = "EC2AssumeRole"
    effect  = "Allow"
    actions = ["sts:AssumeRole"]

    principals {
      type        = "Service"
      identifiers = ["ec2.amazonaws.com"]
    }
  }
}

# Trust policy for EKS services
data "aws_iam_policy_document" "eks_assume_role" {
  statement {
    sid     = "EKSAssumeRole"
    effect  = "Allow"
    actions = ["sts:AssumeRole"]

    principals {
      type        = "Service"
      identifiers = ["eks.amazonaws.com"]
    }
  }
}

# Trust policy for Lambda services
data "aws_iam_policy_document" "lambda_assume_role" {
  statement {
    sid     = "LambdaAssumeRole"
    effect  = "Allow"
    actions = ["sts:AssumeRole"]

    principals {
      type        = "Service"
      identifiers = ["lambda.amazonaws.com"]
    }
  }
}

# Trust policy for ECS services
data "aws_iam_policy_document" "ecs_assume_role" {
  statement {
    sid     = "ECSAssumeRole"
    effect  = "Allow"
    actions = ["sts:AssumeRole"]

    principals {
      type        = "Service"
      identifiers = ["ecs-tasks.amazonaws.com"]
    }
  }
}

# Trust policy for ElastiCache
data "aws_iam_policy_document" "elasticache_assume_role" {
  statement {
    sid     = "ElastiCacheAssumeRole"
    effect  = "Allow"
    actions = ["sts:AssumeRole"]

    principals {
      type        = "Service"
      identifiers = ["elasticache.amazonaws.com"]
    }
  }
}

# ==============================================================================
# IAM Role - Application
# ==============================================================================

# IAM role for the main application
resource "aws_iam_role" "application" {
  name = "${local.name_prefix}-application-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Principal = {
          Service = "ecs-tasks.amazonaws.com"
        }
        Action = "sts:AssumeRole"
      }
    ]
  })

  path                 = var.iam_path
  permissions_boundary = var.iam_permissions_boundary != "" ? var.iam_permissions_boundary : null

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-application-role"
  })
}

# Policy for application to access AWS services
resource "aws_iam_role_policy" "application" {
  name = "${local.name_prefix}-application-policy"
  role = aws_iam_role.application.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid    = "S3Access"
        Effect = "Allow"
        Action = [
          "s3:GetObject",
          "s3:PutObject",
          "s3:DeleteObject",
          "s3:ListBucket",
          "s3:GetBucketLocation"
        ]
        Resource = [
          aws_s3_bucket.models.arn,
          "${aws_s3_bucket.models.arn}/*",
          aws_s3_bucket.cache.arn,
          "${aws_s3_bucket.cache.arn}/*",
          aws_s3_bucket.artifacts.arn,
          "${aws_s3_bucket.artifacts.arn}/*"
        ]
      },
      {
        Sid    = "SecretsManagerAccess"
        Effect = "Allow"
        Action = [
          "secretsmanager:GetSecretValue",
          "secretsmanager:DescribeSecret"
        ]
        Resource = [
          aws_secretsmanager_secret.db_credentials[0].arn,
          aws_secretsmanager_secret.redis_auth[0].arn
        ]
      },
      {
        Sid    = "KMSAccess"
        Effect = "Allow"
        Action = [
          "kms:Decrypt",
          "kms:GenerateDataKey",
          "kms:Encrypt"
        ]
        Resource = var.secrets_kms_key_id != "" ? var.secrets_kms_key_id : "*"
      },
      {
        Sid    = "CloudWatchLogs"
        Effect = "Allow"
        Action = [
          "logs:CreateLogGroup",
          "logs:CreateLogStream",
          "logs:PutLogEvents",
          "logs:DescribeLogStreams"
        ]
        Resource = "arn:aws:logs:*:*:*"
      },
      {
        Sid    = "CloudWatchMetrics"
        Effect = "Allow"
        Action = [
          "cloudwatch:PutMetricData"
        ]
        Resource = "*"
        Condition = {
          StringEquals = {
            "cloudwatch:namespace" = "AIProvider/Application"
          }
        }
      }
    ]
  })
}

# Attach basic execution role
resource "aws_iam_role_policy_attachment" "application_basic" {
  role       = aws_iam_role.application.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

# ==============================================================================
# IAM Role - S3 Access
# ==============================================================================

# IAM role for S3 access
resource "aws_iam_role" "s3_access" {
  name = "${local.name_prefix}-s3-access-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Principal = {
          Service = [
            "ec2.amazonaws.com",
            "ecs-tasks.amazonaws.com",
            "lambda.amazonaws.com"
          ]
        }
        Action = "sts:AssumeRole"
      }
    ]
  })

  path                 = var.iam_path
  permissions_boundary = var.iam_permissions_boundary != "" ? var.iam_permissions_boundary : null

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-s3-access-role"
  })
}

# Policy for S3 access
resource "aws_iam_role_policy" "s3_access" {
  name = "${local.name_prefix}-s3-access-policy"
  role = aws_iam_role.s3_access.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid    = "S3FullAccess"
        Effect = "Allow"
        Action = [
          "s3:GetObject",
          "s3:PutObject",
          "s3:DeleteObject",
          "s3:ListBucket",
          "s3:GetBucketLocation",
          "s3:GetBucketVersioning",
          "s3:GetBucketEncryption",
          "s3:GetObjectVersion"
        ]
        Resource = [
          aws_s3_bucket.models.arn,
          "${aws_s3_bucket.models.arn}/*",
          aws_s3_bucket.cache.arn,
          "${aws_s3_bucket.cache.arn}/*",
          aws_s3_bucket.backups.arn,
          "${aws_s3_bucket.backups.arn}/*",
          aws_s3_bucket.logs.arn,
          "${aws_s3_bucket.logs.arn}/*",
          aws_s3_bucket.artifacts.arn,
          "${aws_s3_bucket.artifacts.arn}/*"
        ]
      }
    ]
  })
}

# ==============================================================================
# IAM Role - CloudWatch Monitoring
# ==============================================================================

# IAM role for CloudWatch monitoring
resource "aws_iam_role" "cloudwatch_monitoring" {
  name = "${local.name_prefix}-cloudwatch-monitoring-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Principal = {
          Service = [
            "cloudwatch.amazonaws.com",
            "logs.amazonaws.com",
            "ecs-tasks.amazonaws.com"
          ]
        }
        Action = "sts:AssumeRole"
      }
    ]
  })

  path                 = var.iam_path
  permissions_boundary = var.iam_permissions_boundary != "" ? var.iam_permissions_boundary : null

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-cloudwatch-monitoring-role"
  })
}

# Policy for CloudWatch monitoring
resource "aws_iam_role_policy" "cloudwatch_monitoring" {
  name = "${local.name_prefix}-cloudwatch-monitoring-policy"
  role = aws_iam_role.cloudwatch_monitoring.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid    = "CloudWatchAccess"
        Effect = "Allow"
        Action = [
          "cloudwatch:PutMetricData",
          "cloudwatch:GetMetricData",
          "cloudwatch:ListMetrics",
          "cloudwatch:DescribeAlarms",
          "cloudwatch:PutDashboard",
          "cloudwatch:GetDashboard",
          "cloudwatch:ListDashboards"
        ]
        Resource = "*"
      },
      {
        Sid    = "LogsAccess"
        Effect = "Allow"
        Action = [
          "logs:CreateLogGroup",
          "logs:CreateLogStream",
          "logs:PutLogEvents",
          "logs:DescribeLogGroups",
          "logs:DescribeLogStreams",
          "logs:GetLogEvents",
          "logs:FilterLogEvents"
        ]
        Resource = "*"
      }
    ]
  })
}

# ==============================================================================
# IAM Role - Backup and Restore
# ==============================================================================

# IAM role for backup operations
resource "aws_iam_role" "backup" {
  name = "${local.name_prefix}-backup-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Principal = {
          Service = "backup.amazonaws.com"
        }
        Action = "sts:AssumeRole"
      }
    ]
  })

  path                 = var.iam_path
  permissions_boundary = var.iam_permissions_boundary != "" ? var.iam_permissions_boundary : null

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-backup-role"
  })
}

# Policy for backup operations
resource "aws_iam_role_policy" "backup" {
  name = "${local.name_prefix}-backup-policy"
  role = aws_iam_role.backup.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid    = "BackupAccess"
        Effect = "Allow"
        Action = [
          "backup:CreateBackupPlan",
          "backup:CreateBackupSelection",
          "backup:CreateBackupVault",
          "backup:DeleteBackupPlan",
          "backup:DeleteBackupSelection",
          "backup:DeleteBackupVault",
          "backup:DescribeBackupJob",
          "backup:DescribeBackupPlan",
          "backup:DescribeBackupSelection",
          "backup:DescribeBackupVault",
          "backup:DescribeProtectedResource",
          "backup:DescribeRecoveryPoint",
          "backup:GetBackupPlan",
          "backup:GetBackupSelection",
          "backup:GetBackupVaultAccessPolicy",
          "backup:GetBackupVaultNotifications",
          "backup:GetRecoveryPointRestoreMetadata",
          "backup:GetSupportedResourceTypes",
          "backup:ListBackupJobs",
          "backup:ListBackupPlanTemplates",
          "backup:ListBackupPlans",
          "backup:ListBackupSelections",
          "backup:ListBackupVaults",
          "backup:ListProtectedResources",
          "backup:ListRecoveryPoints",
          "backup:ListTags",
          "backup:PutBackupVaultAccessPolicy",
          "backup:PutBackupVaultNotifications",
          "backup:StartBackupJob",
          "backup:StopBackupJob",
          "backup:TagResource",
          "backup:UntagResource",
          "backup:UpdateBackupPlan"
        ]
        Resource = "*"
      },
      {
        Sid    = "S3BackupAccess"
        Effect = "Allow"
        Action = [
          "s3:GetObject",
          "s3:PutObject",
          "s3:DeleteObject",
          "s3:ListBucket",
          "s3:GetBucketVersioning"
        ]
        Resource = [
          aws_s3_bucket.backups.arn,
          "${aws_s3_bucket.backups.arn}/*"
        ]
      }
    ]
  })
}

# Attach AWS Backup service role policy
resource "aws_iam_role_policy_attachment" "backup_service" {
  role       = aws_iam_role.backup.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AWSBackupServiceRolePolicyForBackup"
}

# Attach AWS Backup restore role policy
resource "aws_iam_role_policy_attachment" "backup_restore" {
  role       = aws_iam_role.backup.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AWSBackupServiceRolePolicyForRestores"
}

# ==============================================================================
# IAM Role - Lambda Functions
# ==============================================================================

# IAM role for Lambda functions
resource "aws_iam_role" "lambda" {
  name = "${local.name_prefix}-lambda-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Principal = {
          Service = "lambda.amazonaws.com"
        }
        Action = "sts:AssumeRole"
      }
    ]
  })

  path                 = var.iam_path
  permissions_boundary = var.iam_permissions_boundary != "" ? var.iam_permissions_boundary : null

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-lambda-role"
  })
}

# Policy for Lambda functions
resource "aws_iam_role_policy" "lambda" {
  name = "${local.name_prefix}-lambda-policy"
  role = aws_iam_role.lambda.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid    = "LambdaBasicExecution"
        Effect = "Allow"
        Action = [
          "logs:CreateLogGroup",
          "logs:CreateLogStream",
          "logs:PutLogEvents"
        ]
        Resource = "arn:aws:logs:*:*:*"
      },
      {
        Sid    = "VPCAccess"
        Effect = "Allow"
        Action = [
          "ec2:CreateNetworkInterface",
          "ec2:DescribeNetworkInterfaces",
          "ec2:DeleteNetworkInterface"
        ]
        Resource = "*"
      },
      {
        Sid    = "SecretsAccess"
        Effect = "Allow"
        Action = [
          "secretsmanager:GetSecretValue"
        ]
        Resource = [
          aws_secretsmanager_secret.db_credentials[0].arn,
          aws_secretsmanager_secret.redis_auth[0].arn
        ]
      }
    ]
  })
}

# Attach AWS Lambda basic execution role
resource "aws_iam_role_policy_attachment" "lambda_basic" {
  role       = aws_iam_role.lambda.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"
}

# Attach AWS Lambda VPC access execution role
resource "aws_iam_role_policy_attachment" "lambda_vpc" {
  role       = aws_iam_role.lambda.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AWSLambdaVPCAccessExecutionRole"
}

# ==============================================================================
# IAM Role - EKS Pod Execution (IRSA)
# ==============================================================================

# IAM role for EKS pod execution (AWS Load Balancer Controller)
resource "aws_iam_role" "eks_load_balancer_controller" {
  count = var.eks_enable_irsa ? 1 : 0

  name = "${local.name_prefix}-eks-lb-controller-role"

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
            "${replace(aws_eks_cluster.main.identity[0].oidc[0].issuer, "https://", "")}:sub" = "system:serviceaccount:kube-system:aws-load-balancer-controller"
          }
        }
      }
    ]
  })

  path                 = var.iam_path
  permissions_boundary = var.iam_permissions_boundary != "" ? var.iam_permissions_boundary : null

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-eks-lb-controller-role"
  })
}

# Attach AWS Load Balancer Controller policy
resource "aws_iam_role_policy_attachment" "eks_load_balancer_controller" {
  count = var.eks_enable_irsa ? 1 : 0

  role       = aws_iam_role.eks_load_balancer_controller[0].name
  policy_arn = "arn:aws:iam::aws:policy/AWSLoadBalancerControllerIAMPolicy"
}

# ==============================================================================
# IAM Role - EKS External DNS
# ==============================================================================

# IAM role for External DNS
resource "aws_iam_role" "eks_external_dns" {
  count = var.eks_enable_irsa && var.enable_route53 ? 1 : 0

  name = "${local.name_prefix}-eks-external-dns-role"

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
            "${replace(aws_eks_cluster.main.identity[0].oidc[0].issuer, "https://", "")}:sub" = "system:serviceaccount:kube-system:external-dns"
          }
        }
      }
    ]
  })

  path                 = var.iam_path
  permissions_boundary = var.iam_permissions_boundary != "" ? var.iam_permissions_boundary : null

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-eks-external-dns-role"
  })
}

# Policy for External DNS
resource "aws_iam_role_policy" "eks_external_dns" {
  count = var.eks_enable_irsa && var.enable_route53 ? 1 : 0

  name = "${local.name_prefix}-eks-external-dns-policy"
  role = aws_iam_role.eks_external_dns[0].id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "route53:ChangeResourceRecordSets",
          "route53:ListHostedZones",
          "route53:ListResourceRecordSets"
        ]
        Resource = "*"
      }
    ]
  })
}

# ==============================================================================
# IAM Role - EKS Cluster Autoscaler
# ==============================================================================

# IAM role for Cluster Autoscaler
resource "aws_iam_role" "eks_cluster_autoscaler" {
  count = var.eks_enable_irsa && var.enable_cluster_autoscaler ? 1 : 0

  name = "${local.name_prefix}-eks-cluster-autoscaler-role"

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

  path                 = var.iam_path
  permissions_boundary = var.iam_permissions_boundary != "" ? var.iam_permissions_boundary : null

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-eks-cluster-autoscaler-role"
  })
}

# Policy for Cluster Autoscaler
resource "aws_iam_role_policy" "eks_cluster_autoscaler" {
  count = var.eks_enable_irsa && var.enable_cluster_autoscaler ? 1 : 0

  name = "${local.name_prefix}-eks-cluster-autoscaler-policy"
  role = aws_iam_role.eks_cluster_autoscaler[0].id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "autoscaling:DescribeAutoScalingGroups",
          "autoscaling:DescribeAutoScalingInstances",
          "autoscaling:DescribeLaunchConfigurations",
          "autoscaling:DescribeScalingActivities",
          "autoscaling:DescribeTags",
          "ec2:DescribeLaunchTemplateVersions",
          "autoscaling:SetDesiredCapacity",
          "autoscaling:TerminateInstanceInAutoScalingGroup",
          "ec2:DescribeInstanceTypes",
          "ec2:DescribeInstances"
        ]
        Resource = "*"
      }
    ]
  })
}

# ==============================================================================
# IAM Role - EKS External Secrets
# ==============================================================================

# IAM role for External Secrets operator
resource "aws_iam_role" "eks_external_secrets" {
  count = var.eks_enable_irsa ? 1 : 0

  name = "${local.name_prefix}-eks-external-secrets-role"

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
            "${replace(aws_eks_cluster.main.identity[0].oidc[0].issuer, "https://", "")}:sub" = "system:serviceaccount:kube-system:external-secrets"
          }
        }
      }
    ]
  })

  path                 = var.iam_path
  permissions_boundary = var.iam_permissions_boundary != "" ? var.iam_permissions_boundary : null

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-eks-external-secrets-role"
  })
}

# Policy for External Secrets
resource "aws_iam_role_policy" "eks_external_secrets" {
  count = var.eks_enable_irsa ? 1 : 0

  name = "${local.name_prefix}-eks-external-secrets-policy"
  role = aws_iam_role.eks_external_secrets[0].id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "secretsmanager:GetResourcePolicy",
          "secretsmanager:GetSecretValue",
          "secretsmanager:DescribeSecret",
          "secretsmanager:ListSecretVersionIds"
        ]
        Resource = [
          aws_secretsmanager_secret.db_credentials[0].arn,
          aws_secretsmanager_secret.redis_auth[0].arn
        ]
      },
      {
        Effect = "Allow"
        Action = [
          "kms:Decrypt"
        ]
        Resource = var.secrets_kms_key_id != "" ? var.secrets_kms_key_id : "*"
      }
    ]
  })
}

# ==============================================================================
# IAM Instance Profile - EKS Nodes
# ==============================================================================

# IAM instance profile for EKS nodes
resource "aws_iam_instance_profile" "eks_nodes" {
  name = "${local.name_prefix}-eks-node-profile"
  role = aws_iam_role.eks_nodes.name
  path = var.iam_path

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-eks-node-profile"
  })
}

# ==============================================================================
# IAM Group - Administrators
# ==============================================================================

# IAM group for administrators
resource "aws_iam_group" "administrators" {
  count = var.create_admin_group ? 1 : 0

  name = "${local.name_prefix}-administrators"
  path = var.iam_path
}

# Attach administrator access policy
resource "aws_iam_group_policy_attachment" "administrators" {
  count = var.create_admin_group ? 1 : 0

  group      = aws_iam_group.administrators[0].name
  policy_arn = "arn:aws:iam::aws:policy/AdministratorAccess"
}

# ==============================================================================
# IAM Group - Developers
# ==============================================================================

# IAM group for developers
resource "aws_iam_group" "developers" {
  count = var.create_developer_group ? 1 : 0

  name = "${local.name_prefix}-developers"
  path = var.iam_path
}

# Policy for developers
resource "aws_iam_group_policy" "developers" {
  count = var.create_developer_group ? 1 : 0

  name  = "${local.name_prefix}-developers-policy"
  group = aws_iam_group.developers[0].name

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid    = "DeveloperAccess"
        Effect = "Allow"
        Action = [
          "eks:DescribeCluster",
          "eks:ListClusters",
          "eks:AccessKubernetesApi",
          "ecr:GetAuthorizationToken",
          "ecr:BatchCheckLayerAvailability",
          "ecr:GetDownloadUrlForLayer",
          "ecr:BatchGetImage",
          "ecr:PutImage",
          "ecr:InitiateLayerUpload",
          "ecr:UploadLayerPart",
          "ecr:CompleteLayerUpload",
          "logs:DescribeLogGroups",
          "logs:DescribeLogStreams",
          "logs:GetLogEvents",
          "cloudwatch:DescribeAlarms",
          "cloudwatch:GetMetricData",
          "cloudwatch:ListMetrics"
        ]
        Resource = "*"
      }
    ]
  })
}

# ==============================================================================
# IAM Group - Read-Only
# ==============================================================================

# IAM group for read-only access
resource "aws_iam_group" "read_only" {
  count = var.create_readonly_group ? 1 : 0

  name = "${local.name_prefix}-readonly"
  path = var.iam_path
}

# Attach read-only access policy
resource "aws_iam_group_policy_attachment" "read_only" {
  count = var.create_readonly_group ? 1 : 0

  group      = aws_iam_group.read_only[0].name
  policy_arn = "arn:aws:iam::aws:policy/ReadOnlyAccess"
}

# ==============================================================================
# IAM Policy - Cost Management
# ==============================================================================

# IAM policy for cost management
resource "aws_iam_policy" "cost_management" {
  count = var.enable_cost_explorer ? 1 : 0

  name        = "${local.name_prefix}-cost-management-policy"
  description = "Policy for cost management and budget monitoring"

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid    = "CostExplorerAccess"
        Effect = "Allow"
        Action = [
          "ce:GetCostAndUsage",
          "ce:GetDimensionValues",
          "ce:GetReservationCoverage",
          "ce:GetReservationPurchaseRecommendation",
          "ce:GetReservationUtilization",
          "ce:GetRightsizingRecommendation",
          "ce:GetSavingsPlansCoverage",
          "ce:GetSavingsPlansPurchaseRecommendation",
          "ce:GetSavingsPlansUtilization",
          "ce:GetSavingsPlansUtilizationDetails",
          "ce:ListCostCategoryDefinitions"
        ]
        Resource = "*"
      },
      {
        Sid    = "BudgetsAccess"
        Effect = "Allow"
        Action = [
          "budgets:DescribeBudgets",
          "budgets:DescribeBudgetActionsForAccount",
          "budgets:DescribeBudgetActionsForBudget",
          "budgets:ViewBudget"
        ]
        Resource = "*"
      }
    ]
  })

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-cost-management-policy"
  })
}

# ==============================================================================
# IAM Policy - Security Audit
# ==============================================================================

# IAM policy for security audit
resource "aws_iam_policy" "security_audit" {
  count = var.enable_security_hub ? 1 : 0

  name        = "${local.name_prefix}-security-audit-policy"
  description = "Policy for security auditing and compliance"

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid    = "SecurityAuditAccess"
        Effect = "Allow"
        Action = [
          "guardduty:Get*",
          "guardduty:List*",
          "securityhub:Get*",
          "securityhub:List*",
          "config:Get*",
          "config:List*",
          "config:Describe*",
          "cloudtrail:DescribeTrails",
          "cloudtrail:GetTrailStatus",
          "cloudtrail:LookupEvents",
          "kms:DescribeKey",
          "kms:GetKeyPolicy",
          "kms:GetKeyRotationStatus",
          "kms:ListAliases",
          "kms:ListKeys",
          "iam:Get*",
          "iam:List*",
          "iam:GenerateCredentialReport",
          "iam:GenerateServiceLastAccessedDetails",
          "iam:GetCredentialReport",
          "iam:GetServiceLastAccessedDetails"
        ]
        Resource = "*"
      }
    ]
  })

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-security-audit-policy"
  })
}

# ==============================================================================
# Outputs
# ==============================================================================

output "iam_application_role_arn" {
  description = "ARN of the application IAM role"
  value       = aws_iam_role.application.arn
}

output "iam_s3_access_role_arn" {
  description = "ARN of the S3 access IAM role"
  value       = aws_iam_role.s3_access.arn
}

output "iam_cloudwatch_monitoring_role_arn" {
  description = "ARN of the CloudWatch monitoring IAM role"
  value       = aws_iam_role.cloudwatch_monitoring.arn
}

output "iam_backup_role_arn" {
  description = "ARN of the backup IAM role"
  value       = aws_iam_role.backup.arn
}

output "iam_lambda_role_arn" {
  description = "ARN of the Lambda IAM role"
  value       = aws_iam_role.lambda.arn
}

output "iam_eks_lb_controller_role_arn" {
  description = "ARN of the EKS Load Balancer Controller IAM role"
  value       = var.eks_enable_irsa ? aws_iam_role.eks_load_balancer_controller[0].arn : ""
}

output "iam_eks_external_dns_role_arn" {
  description = "ARN of the EKS External DNS IAM role"
  value       = var.eks_enable_irsa && var.enable_route53 ? aws_iam_role.eks_external_dns[0].arn : ""
}

output "iam_eks_cluster_autoscaler_role_arn" {
  description = "ARN of the EKS Cluster Autoscaler IAM role"
  value       = var.eks_enable_irsa && var.enable_cluster_autoscaler ? aws_iam_role.eks_cluster_autoscaler[0].arn : ""
}

output "iam_eks_external_secrets_role_arn" {
  description = "ARN of the EKS External Secrets IAM role"
  value       = var.eks_enable_irsa ? aws_iam_role.eks_external_secrets[0].arn : ""
}

output "iam_eks_node_instance_profile_name" {
  description = "Name of the EKS node instance profile"
  value       = aws_iam_instance_profile.eks_nodes.name
}

output "iam_administrator_group_name" {
  description = "Name of the administrator IAM group"
  value       = var.create_admin_group ? aws_iam_group.administrators[0].name : ""
}

output "iam_developer_group_name" {
  description = "Name of the developer IAM group"
  value       = var.create_developer_group ? aws_iam_group.developers[0].name : ""
}

output "iam_readonly_group_name" {
  description = "Name of the read-only IAM group"
  value       = var.create_readonly_group ? aws_iam_group.read_only[0].name : ""
}

output "iam_cost_management_policy_arn" {
  description = "ARN of the cost management IAM policy"
  value       = var.enable_cost_explorer ? aws_iam_policy.cost_management[0].arn : ""
}

output "iam_security_audit_policy_arn" {
  description = "ARN of the security audit IAM policy"
  value       = var.enable_security_hub ? aws_iam_policy.security_audit[0].arn : ""
}
