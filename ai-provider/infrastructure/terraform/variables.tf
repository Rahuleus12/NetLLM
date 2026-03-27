# ==============================================================================
# AI Provider - Terraform Variables
# ==============================================================================
# This file defines all input variables for the AI Provider infrastructure
# deployment on AWS. Variables are organized by component for clarity.
# ==============================================================================

# ==============================================================================
# General Configuration
# ==============================================================================

variable "project_name" {
  description = "Name of the project (used in resource naming)"
  type        = string
  default     = "ai-provider"

  validation {
    condition     = can(regex("^[a-z0-9-]+$", var.project_name))
    error_message = "Project name must contain only lowercase letters, numbers, and hyphens."
  }
}

variable "environment" {
  description = "Environment name (e.g., dev, staging, prod)"
  type        = string
  default     = "dev"

  validation {
    condition     = contains(["dev", "staging", "prod"], var.environment)
    error_message = "Environment must be one of: dev, staging, prod."
  }
}

variable "aws_region" {
  description = "AWS region to deploy resources"
  type        = string
  default     = "us-east-1"
}

variable "aws_profile" {
  description = "AWS CLI profile to use for authentication"
  type        = string
  default     = "default"
}

variable "tags" {
  description = "Common tags to apply to all resources"
  type        = map(string)
  default     = {}
}

# ==============================================================================
# VPC Configuration
# ==============================================================================

variable "vpc_cidr" {
  description = "CIDR block for VPC"
  type        = string
  default     = "10.0.0.0/16"

  validation {
    condition     = can(cidrhost(var.vpc_cidr, 0))
    error_message = "VPC CIDR must be a valid CIDR block."
  }
}

variable "vpc_enable_dns_hostnames" {
  description = "Enable DNS hostnames in VPC"
  type        = bool
  default     = true
}

variable "vpc_enable_dns_support" {
  description = "Enable DNS support in VPC"
  type        = bool
  default     = true
}

variable "availability_zones" {
  description = "List of availability zones to use"
  type        = list(string)
  default     = ["us-east-1a", "us-east-1b", "us-east-1c"]
}

variable "public_subnet_cidrs" {
  description = "CIDR blocks for public subnets"
  type        = list(string)
  default     = ["10.0.1.0/24", "10.0.2.0/24", "10.0.3.0/24"]
}

variable "private_subnet_cidrs" {
  description = "CIDR blocks for private subnets"
  type        = list(string)
  default     = ["10.0.10.0/24", "10.0.11.0/24", "10.0.12.0/24"]
}

variable "database_subnet_cidrs" {
  description = "CIDR blocks for database subnets"
  type        = list(string)
  default     = ["10.0.20.0/24", "10.0.21.0/24", "10.0.22.0/24"]
}

variable "enable_nat_gateway" {
  description = "Enable NAT Gateway for private subnets"
  type        = bool
  default     = true
}

variable "single_nat_gateway" {
  description = "Use a single NAT Gateway for all availability zones (cost savings)"
  type        = bool
  default     = false
}

variable "enable_vpn_gateway" {
  description = "Enable VPN Gateway"
  type        = bool
  default     = false
}

variable "enable_flow_logs" {
  description = "Enable VPC flow logs"
  type        = bool
  default     = true
}

variable "flow_logs_retention_days" {
  description = "Number of days to retain VPC flow logs"
  type        = number
  default     = 30
}

# ==============================================================================
# EKS Configuration
# ==============================================================================

variable "eks_cluster_name" {
  description = "Name of the EKS cluster"
  type        = string
  default     = ""
}

variable "eks_cluster_version" {
  description = "Kubernetes version for EKS cluster"
  type        = string
  default     = "1.28"

  validation {
    condition     = can(regex("^1\\.(2[5-9]|[3-9][0-9])$", var.eks_cluster_version))
    error_message = "EKS cluster version must be 1.25 or higher."
  }
}

variable "eks_cluster_enabled_log_types" {
  description = "List of EKS cluster log types to enable"
  type        = list(string)
  default     = ["api", "audit", "authenticator", "controllerManager", "scheduler"]
}

variable "eks_cluster_log_retention_days" {
  description = "Number of days to retain EKS cluster logs"
  type        = number
  default     = 30
}

variable "eks_endpoint_private_access" {
  description = "Enable private access to EKS cluster endpoint"
  type        = bool
  default     = true
}

variable "eks_endpoint_public_access" {
  description = "Enable public access to EKS cluster endpoint"
  type        = bool
  default     = true
}

variable "eks_public_access_cidrs" {
  description = "List of CIDR blocks allowed to access EKS public endpoint"
  type        = list(string)
  default     = ["0.0.0.0/0"]
}

variable "eks_enable_irsa" {
  description = "Enable IAM Roles for Service Accounts (IRSA)"
  type        = bool
  default     = true
}

variable "eks_cluster_encryption_config" {
  description = "Configuration for EKS cluster encryption"
  type = object({
    enabled        = bool
    resources      = list(string)
    kms_key_arn    = optional(string)
  })
  default = {
    enabled   = true
    resources = ["secrets"]
  }
}

variable "eks_node_groups" {
  description = "Map of EKS node group configurations"
  type = map(object({
    instance_types         = list(string)
    capacity_type          = string
    disk_size              = number
    desired_size           = number
    min_size               = number
    max_size               = number
    max_unavailable        = number
    labels                 = map(string)
    taints                 = map(string)
    enable_monitoring      = bool
    enable_spot_instances  = bool
    spot_instance_pools    = number
  }))
  default = {
    general = {
      instance_types        = ["m5.large"]
      capacity_type         = "ON_DEMAND"
      disk_size             = 100
      desired_size          = 3
      min_size              = 2
      max_size              = 10
      max_unavailable       = 1
      labels                = {}
      taints                = {}
      enable_monitoring     = true
      enable_spot_instances = false
      spot_instance_pools   = 2
    }
    gpu = {
      instance_types        = ["g4dn.xlarge"]
      capacity_type         = "ON_DEMAND"
      disk_size             = 200
      desired_size          = 2
      min_size              = 1
      max_size              = 5
      max_unavailable       = 1
      labels = {
        "nvidia.com/gpu" = "true"
      }
      taints = {
        "nvidia.com/gpu" = "true:NoSchedule"
      }
      enable_monitoring     = true
      enable_spot_instances = false
      spot_instance_pools   = 2
    }
  }
}

variable "eks_fargate_profiles" {
  description = "Map of Fargate profile configurations"
  type = map(object({
    subnet_ids    = list(string)
    selectors     = list(object({
      namespace   = string
      labels      = map(string)
    }))
  }))
  default = {}
}

variable "eks_addons" {
  description = "List of EKS addons to enable"
  type = list(object({
    name                 = string
    version              = string
    configuration_values = optional(string)
  }))
  default = [
    {
      name    = "vpc-cni"
      version = "v1.14.0-eksbuild.1"
    },
    {
      name    = "coredns"
      version = "v1.10.1-eksbuild.1"
    },
    {
      name    = "kube-proxy"
      version = "v1.28.1-eksbuild.1"
    }
  ]
}

# ==============================================================================
# RDS Configuration
# ==============================================================================

variable "rds_enabled" {
  description = "Enable RDS PostgreSQL instance"
  type        = bool
  default     = true
}

variable "rds_instance_class" {
  description = "Instance class for RDS PostgreSQL"
  type        = string
  default     = "db.r6g.large"
}

variable "rds_engine_version" {
  description = "PostgreSQL engine version"
  type        = string
  default     = "15.4"
}

variable "rds_allocated_storage" {
  description = "Allocated storage in GB"
  type        = number
  default     = 100
}

variable "rds_max_allocated_storage" {
  description = "Maximum allocated storage for autoscaling (GB)"
  type        = number
  default     = 1000
}

variable "rds_storage_type" {
  description = "Storage type (gp2, gp3, io1)"
  type        = string
  default     = "gp3"
}

variable "rds_storage_encrypted" {
  description = "Enable storage encryption"
  type        = bool
  default     = true
}

variable "rds_multi_az" {
  description = "Enable Multi-AZ deployment"
  type        = bool
  default     = true
}

variable "rds_backup_retention_period" {
  description = "Backup retention period in days"
  type        = number
  default     = 30
}

variable "rds_backup_window" {
  description = "Preferred backup window (UTC)"
  type        = string
  default     = "03:00-04:00"
}

variable "rds_maintenance_window" {
  description = "Preferred maintenance window (UTC)"
  type        = string
  default     = "sun:04:00-sun:05:00"
}

variable "rds_deletion_protection" {
  description = "Enable deletion protection"
  type        = bool
  default     = true
}

variable "rds_skip_final_snapshot" {
  description = "Skip final snapshot on deletion"
  type        = bool
  default     = false
}

variable "rds_performance_insights_enabled" {
  description = "Enable Performance Insights"
  type        = bool
  default     = true
}

variable "rds_performance_insights_retention_period" {
  description = "Performance Insights retention period in days"
  type        = number
  default     = 7
}

variable "rds_parameter_group_family" {
  description = "Parameter group family for PostgreSQL"
  type        = string
  default     = "postgres15"
}

variable "rds_parameters" {
  description = "List of RDS parameters to set"
  type = list(object({
    name  = string
    value = string
  }))
  default = [
    { name = "max_connections", value = "500" },
    { name = "shared_buffers", value = "{DBInstanceClassMemory/4}" },
    { name = "work_mem", value = "262144" },
    { name = "effective_cache_size", value = "{DBInstanceClassMemory*3/4}" },
    { name = "random_page_cost", value = "1.1" },
    { name = "log_min_duration_statement", value = "1000" }
  ]
}

variable "rds_username" {
  description = "Master username for RDS"
  type        = string
  default     = "ai_provider_admin"
  sensitive   = true
}

variable "rds_database_name" {
  description = "Database name"
  type        = string
  default     = "ai_provider"
}

# ==============================================================================
# ElastiCache Configuration
# ==============================================================================

variable "elasticache_enabled" {
  description = "Enable ElastiCache Redis cluster"
  type        = bool
  default     = true
}

variable "elasticache_node_type" {
  description = "Node type for ElastiCache Redis"
  type        = string
  default     = "cache.r6g.large"
}

variable "elasticache_engine_version" {
  description = "Redis engine version"
  type        = string
  default     = "7.0"
}

variable "elasticache_parameter_group_family" {
  description = "Parameter group family for Redis"
  type        = string
  default     = "redis7"
}

variable "elasticache_num_cache_clusters" {
  description = "Number of cache clusters"
  type        = number
  default     = 2
}

variable "elasticache_num_node_groups" {
  description = "Number of node groups (shards)"
  type        = number
  default     = 2
}

variable "elasticache_replicas_per_node_group" {
  description = "Number of replicas per node group"
  type        = number
  default     = 1
}

variable "elasticache_automatic_failover_enabled" {
  description = "Enable automatic failover"
  type        = bool
  default     = true
}

variable "elasticache_multi_az_enabled" {
  description = "Enable Multi-AZ"
  type        = bool
  default     = true
}

variable "elasticache_at_rest_encryption_enabled" {
  description = "Enable at-rest encryption"
  type        = bool
  default     = true
}

variable "elasticache_transit_encryption_enabled" {
  description = "Enable transit encryption"
  type        = bool
  default     = true
}

variable "elasticache_auth_token" {
  description = "Auth token for Redis"
  type        = string
  default     = ""
  sensitive   = true
}

variable "elasticache_snapshot_retention_limit" {
  description = "Number of snapshots to retain"
  type        = number
  default     = 30
}

variable "elasticache_snapshot_window" {
  description = "Preferred snapshot window (UTC)"
  type        = string
  default     = "02:00-03:00"
}

variable "elasticache_maintenance_window" {
  description = "Preferred maintenance window (UTC)"
  type        = string
  default     = "sun:03:00-sun:04:00"
}

variable "elasticache_parameters" {
  description = "List of ElastiCache parameters to set"
  type = list(object({
    name  = string
    value = string
  }))
  default = [
    { name = "maxmemory-policy", value = "allkeys-lru" },
    { name = "timeout", value = "300" },
    { name = "tcp-keepalive", value = "60" },
    { name = "maxmemory-samples", value = "10" }
  ]
}

# ==============================================================================
# S3 Configuration
# ==============================================================================

variable "s3_bucket_prefix" {
  description = "Prefix for S3 bucket names"
  type        = string
  default     = ""
}

variable "s3_enable_versioning" {
  description = "Enable versioning for S3 buckets"
  type        = bool
  default     = true
}

variable "s3_enable_encryption" {
  description = "Enable server-side encryption for S3 buckets"
  type        = bool
  default     = true
}

variable "s3_block_public_access" {
  description = "Block all public access to S3 buckets"
  type        = bool
  default     = true
}

variable "s3_lifecycle_rules" {
  description = "Lifecycle rules for S3 buckets"
  type = list(object({
    id      = string
    enabled = bool
    prefix  = string
    transitions = list(object({
      days          = number
      storage_class = string
    }))
    expiration = object({
      days = number
    })
  }))
  default = [
    {
      id      = "transition-to-ia"
      enabled = true
      prefix  = ""
      transitions = [
        {
          days          = 90
          storage_class = "STANDARD_IA"
        },
        {
          days          = 180
          storage_class = "GLACIER"
        }
      ]
      expiration = {
        days = 365
      }
    }
  ]
}

variable "s3_buckets" {
  description = "Map of S3 bucket configurations"
  type = map(object({
    purpose           = string
    versioning        = bool
    encryption        = bool
    block_public      = bool
    enable_lifecycle  = bool
  }))
  default = {
    models = {
      purpose          = "AI model storage"
      versioning       = true
      encryption       = true
      block_public     = true
      enable_lifecycle = true
    }
    cache = {
      purpose          = "Cache storage"
      versioning       = false
      encryption       = true
      block_public     = true
      enable_lifecycle = true
    }
    backups = {
      purpose          = "Backup storage"
      versioning       = true
      encryption       = true
      block_public     = true
      enable_lifecycle = true
    }
    logs = {
      purpose          = "Log storage"
      versioning       = false
      encryption       = true
      block_public     = true
      enable_lifecycle = true
    }
    artifacts = {
      purpose          = "CI/CD artifacts"
      versioning       = true
      encryption       = true
      block_public     = true
      enable_lifecycle = true
    }
  }
}

# ==============================================================================
# IAM Configuration
# ==============================================================================

variable "iam_path" {
  description = "Path for IAM resources"
  type        = string
  default     = "/"
}

variable "iam_permissions_boundary" {
  description = "ARN of permissions boundary to apply to roles"
  type        = string
  default     = ""
}

variable "eks_pod_iam_policies" {
  description = "Map of IAM policies for EKS pods"
  type = map(object({
    description = string
    policy      = string
  }))
  default = {}
}

# ==============================================================================
# CloudWatch Monitoring Configuration
# ==============================================================================

variable "cloudwatch_log_retention_days" {
  description = "Number of days to retain CloudWatch logs"
  type        = number
  default     = 30
}

variable "enable_cloudwatch_alarms" {
  description = "Enable CloudWatch alarms"
  type        = bool
  default     = true
}

variable "cloudwatch_alarm_actions" {
  description = "List of ARNs for alarm actions (SNS topics)"
  type        = list(string)
  default     = []
}

variable "alarm_cpu_threshold" {
  description = "CPU usage threshold for alarms (percentage)"
  type        = number
  default     = 80
}

variable "alarm_memory_threshold" {
  description = "Memory usage threshold for alarms (percentage)"
  type        = number
  default     = 80
}

variable "alarm_disk_threshold" {
  description = "Disk usage threshold for alarms (percentage)"
  type        = number
  default     = 85
}

variable "alarm_rds_connections_threshold" {
  description = "RDS connections threshold for alarms"
  type        = number
  default     = 400
}

variable "alarm_elasticache_connections_threshold" {
  description = "ElastiCache connections threshold for alarms"
  type        = number
  default     = 10000
}

variable "enable_cloudwatch_dashboard" {
  description = "Enable CloudWatch dashboard"
  type        = bool
  default     = true
}

variable "dashboard_name" {
  description = "Name of CloudWatch dashboard"
  type        = string
  default     = ""
}

# ==============================================================================
# Security Configuration
# ==============================================================================

variable "enable_waf" {
  description = "Enable AWS WAF for load balancer"
  type        = bool
  default     = false
}

variable "enable_shield_advanced" {
  description = "Enable AWS Shield Advanced"
  type        = bool
  default     = false
}

variable "enable_guardduty" {
  description = "Enable Amazon GuardDuty"
  type        = bool
  default     = true
}

variable "enable_security_hub" {
  description = "Enable AWS Security Hub"
  type        = bool
  default     = true
}

variable "enable_config" {
  description = "Enable AWS Config"
  type        = bool
  default     = true
}

variable "enable_cloudtrail" {
  description = "Enable AWS CloudTrail"
  type        = bool
  default     = true
}

variable "allowed_ssh_cidrs" {
  description = "List of CIDR blocks allowed SSH access"
  type        = list(string)
  default     = []
}

variable "allowed_api_cidrs" {
  description = "List of CIDR blocks allowed API access"
  type        = list(string)
  default     = ["0.0.0.0/0"]
}

# ==============================================================================
# Cost Management Configuration
# ==============================================================================

variable "enable_cost_explorer" {
  description = "Enable AWS Cost Explorer"
  type        = bool
  default     = true
}

variable "budget_amount" {
  description = "Monthly budget amount in USD"
  type        = number
  default     = 5000
}

variable "budget_alert_emails" {
  description = "List of email addresses for budget alerts"
  type        = list(string)
  default     = []
}

variable "budget_threshold_percentages" {
  description = "List of budget threshold percentages for alerts"
  type        = list(number)
  default     = [50, 75, 90, 100]
}

# ==============================================================================
# DNS and Domain Configuration
# ==============================================================================

variable "enable_route53" {
  description = "Enable Route53 DNS records"
  type        = bool
  default     = true
}

variable "domain_name" {
  description = "Domain name for the application"
  type        = string
  default     = ""
}

variable "route53_zone_id" {
  description = "Route53 hosted zone ID"
  type        = string
  default     = ""
}

variable "acm_certificate_arn" {
  description = "ARN of ACM certificate for HTTPS"
  type        = string
  default     = ""
}

variable "create_acm_certificate" {
  description = "Create new ACM certificate"
  type        = bool
  default     = false
}

# ==============================================================================
# Secrets Manager Configuration
# ==============================================================================

variable "secrets_manager_enabled" {
  description = "Enable AWS Secrets Manager"
  type        = bool
  default     = true
}

variable "secrets_kms_key_id" {
  description = "KMS key ID for encrypting secrets"
  type        = string
  default     = ""
}

variable "secrets_rotation_days" {
  description = "Number of days between automatic secret rotation"
  type        = number
  default     = 30
}

# ==============================================================================
# Auto Scaling Configuration
# ==============================================================================

variable "enable_cluster_autoscaler" {
  description = "Enable Kubernetes Cluster Autoscaler"
  type        = bool
  default     = true
}

variable "cluster_autoscaler_min_nodes" {
  description = "Minimum number of nodes in cluster"
  type        = number
  default     = 3
}

variable "cluster_autoscaler_max_nodes" {
  description = "Maximum number of nodes in cluster"
  type        = number
  default     = 20
}

# ==============================================================================
# Disaster Recovery Configuration
# ==============================================================================

variable "enable_cross_region_replication" {
  description = "Enable cross-region replication for S3 buckets"
  type        = bool
  default     = false
}

variable "dr_region" {
  description = "Disaster recovery region"
  type        = string
  default     = "us-west-2"
}

variable "enable_backup_plan" {
  description = "Enable AWS Backup plan"
  type        = bool
  default     = true
}

variable "backup_plan_schedule" {
  description = "Backup schedule in cron format"
  type        = string
  default     = "cron(0 5 ? * * *)"  # Daily at 5 AM UTC
}

variable "backup_retention_days" {
  description = "Number of days to retain backups"
  type        = number
  default     = 90
}

# ==============================================================================
# Developer Tools Configuration
# ==============================================================================

variable "enable_eksctl" {
  description = "Generate eksctl configuration"
  type        = bool
  default     = true
}

variable "enable_kubectl_config" {
  description = "Generate kubectl configuration"
  type        = bool
  default     = true
}

# ==============================================================================
# Additional Tags
# ==============================================================================

variable "common_tags" {
  description = "Common tags applied to all resources"
  type        = map(string)
  default = {
    ManagedBy   = "Terraform"
    Project     = "AI-Provider"
    Owner       = "DevOps"
    CostCenter  = "Engineering"
  }
}

variable "environment_tags" {
  description = "Environment-specific tags"
  type        = map(string)
  default = {
    Environment = "dev"
  }
}

# ==============================================================================
# Locals for Computed Values
# ==============================================================================

locals {
  # Naming convention: {project}-{environment}-{resource}
  name_prefix = "${var.project_name}-${var.environment}"

  # Common tags merged with environment tags
  merged_tags = merge(
    var.common_tags,
    var.environment_tags,
    var.tags,
    {
      Environment = var.environment
      Project     = var.project_name
    }
  )

  # EKS cluster name
  eks_cluster_name = var.eks_cluster_name != "" ? var.eks_cluster_name : "${local.name_prefix}-cluster"

  # Dashboard name
  dashboard_name = var.dashboard_name != "" ? var.dashboard_name : "${local.name_prefix}-dashboard"

  # S3 bucket prefix
  s3_bucket_prefix = var.s3_bucket_prefix != "" ? var.s3_bucket_prefix : local.name_prefix

  # Database credentials secret name
  db_secret_name = "${local.name_prefix}-db-credentials"

  # Redis auth token secret name
  redis_secret_name = "${local.name_prefix}-redis-auth"

  # KMS key alias
  kms_key_alias = "alias/${local.name_prefix}-key"
}
