# ==============================================================================
# AI Provider - Terraform Outputs
# ==============================================================================
# This file defines all output values from the Terraform configuration.
# These outputs can be used by other configurations, CI/CD pipelines,
# or for reference by operators and developers.
# ==============================================================================

# ==============================================================================
# General Outputs
# ==============================================================================

output "project_name" {
  description = "Project name"
  value       = var.project_name
}

output "environment" {
  description = "Environment name"
  value       = var.environment
}

output "aws_region" {
  description = "AWS region"
  value       = var.aws_region
}

output "aws_account_id" {
  description = "AWS account ID"
  value       = data.aws_caller_identity.current.account_id
}

# ==============================================================================
# VPC Outputs
# ==============================================================================

output "vpc_id" {
  description = "VPC ID"
  value       = module.vpc.vpc_id
}

output "vpc_cidr_block" {
  description = "VPC CIDR block"
  value       = module.vpc.vpc_cidr_block
}

output "vpc_arn" {
  description = "VPC ARN"
  value       = module.vpc.vpc_arn
}

output "public_subnet_ids" {
  description = "List of public subnet IDs"
  value       = module.vpc.public_subnet_ids
}

output "private_subnet_ids" {
  description = "List of private subnet IDs"
  value       = module.vpc.private_subnet_ids
}

output "database_subnet_ids" {
  description = "List of database subnet IDs"
  value       = module.vpc.database_subnet_ids
}

output "public_subnet_cidrs" {
  description = "List of public subnet CIDR blocks"
  value       = module.vpc.public_subnet_cidrs
}

output "private_subnet_cidrs" {
  description = "List of private subnet CIDR blocks"
  value       = module.vpc.private_subnet_cidrs
}

output "database_subnet_cidrs" {
  description = "List of database subnet CIDR blocks"
  value       = module.vpc.database_subnet_cidrs
}

output "nat_gateway_public_ips" {
  description = "Public IPs of NAT Gateways"
  value       = module.vpc.nat_gateway_public_ips
}

output "internet_gateway_id" {
  description = "Internet Gateway ID"
  value       = module.vpc.internet_gateway_id
}

output "vpc_flow_logs_log_group_arn" {
  description = "ARN of VPC Flow Logs CloudWatch Log Group"
  value       = var.enable_flow_logs ? aws_cloudwatch_log_group.flow_logs[0].arn : ""
}

# ==============================================================================
# EKS Cluster Outputs
# ==============================================================================

output "eks_cluster_id" {
  description = "EKS cluster ID"
  value       = module.eks.cluster_id
}

output "eks_cluster_arn" {
  description = "EKS cluster ARN"
  value       = module.eks.cluster_arn
}

output "eks_cluster_name" {
  description = "EKS cluster name"
  value       = module.eks.cluster_name
}

output "eks_cluster_endpoint" {
  description = "EKS cluster endpoint"
  value       = module.eks.cluster_endpoint
}

output "eks_cluster_version" {
  description = "EKS cluster Kubernetes version"
  value       = module.eks.cluster_version
}

output "eks_cluster_certificate_authority_data" {
  description = "Base64 encoded certificate data for cluster authentication"
  value       = module.eks.cluster_certificate_authority_data
  sensitive   = true
}

output "eks_cluster_oidc_issuer_url" {
  description = "EKS cluster OIDC issuer URL"
  value       = module.eks.cluster_oidc_issuer_url
}

output "eks_cluster_oidc_provider_arn" {
  description = "EKS cluster OIDC provider ARN"
  value       = module.eks.oidc_provider_arn
}

output "eks_cluster_primary_security_group_id" {
  description = "EKS cluster primary security group ID"
  value       = module.eks.cluster_primary_security_group_id
}

output "eks_cluster_security_group_id" {
  description = "EKS cluster additional security group ID"
  value       = module.eks.cluster_security_group_id
}

output "eks_node_security_group_id" {
  description = "EKS node security group ID"
  value       = module.eks.node_security_group_id
}

output "eks_cluster_role_arn" {
  description = "EKS cluster IAM role ARN"
  value       = module.eks.cluster_role_arn
}

output "eks_node_role_arn" {
  description = "EKS node IAM role ARN"
  value       = module.eks.node_role_arn
}

output "eks_node_groups" {
  description = "Map of EKS node group attributes"
  value       = module.eks.eks_node_groups
}

output "eks_kubeconfig_command" {
  description = "Command to update kubeconfig for EKS cluster"
  value       = "aws eks update-kubeconfig --name ${module.eks.cluster_name} --region ${var.aws_region}"
}

output "eks_openid_connect_provider_url" {
  description = "OpenID Connect provider URL for IRSA"
  value       = module.eks.cluster_oidc_issuer_url
}

output "eks_openid_connect_provider_arn" {
  description = "OpenID Connect provider ARN for IRSA"
  value       = module.eks.oidc_provider_arn
}

# ==============================================================================
# RDS Outputs
# ==============================================================================

output "rds_instance_id" {
  description = "RDS instance identifier"
  value       = var.rds_enabled ? module.rds.db_instance_id : ""
}

output "rds_instance_arn" {
  description = "RDS instance ARN"
  value       = var.rds_enabled ? module.rds.db_instance_arn : ""
}

output "rds_instance_endpoint" {
  description = "RDS instance endpoint (host:port)"
  value       = var.rds_enabled ? module.rds.db_instance_endpoint : ""
}

output "rds_instance_address" {
  description = "RDS instance address (host)"
  value       = var.rds_enabled ? module.rds.db_instance_address : ""
}

output "rds_instance_port" {
  description = "RDS instance port"
  value       = var.rds_enabled ? module.rds.db_instance_port : 0
}

output "rds_database_name" {
  description = "RDS database name"
  value       = var.rds_enabled ? var.rds_database_name : ""
}

output "rds_master_username" {
  description = "RDS master username"
  value       = var.rds_enabled ? var.rds_username : ""
  sensitive   = true
}

output "rds_security_group_id" {
  description = "RDS security group ID"
  value       = var.rds_enabled ? module.rds.security_group_id : ""
}

output "rds_db_subnet_group_name" {
  description = "RDS DB subnet group name"
  value       = var.rds_enabled ? module.rds.db_subnet_group_name : ""
}

output "rds_parameter_group_name" {
  description = "RDS parameter group name"
  value       = var.rds_enabled ? module.rds.db_parameter_group_name : ""
}

output "rds_credentials_secret_arn" {
  description = "ARN of Secrets Manager secret containing DB credentials"
  value       = var.rds_enabled && var.secrets_manager_enabled ? aws_secretsmanager_secret.db_credentials[0].arn : ""
}

output "rds_credentials_secret_name" {
  description = "Name of Secrets Manager secret containing DB credentials"
  value       = var.rds_enabled && var.secrets_manager_enabled ? aws_secretsmanager_secret.db_credentials[0].name : ""
}

output "rds_jdbc_url" {
  description = "JDBC connection URL for RDS"
  value       = var.rds_enabled ? "jdbc:postgresql://${module.rds.db_instance_address}:${module.rds.db_instance_port}/${var.rds_database_name}" : ""
  sensitive   = true
}

output "rds_proxy_endpoint" {
  description = "RDS Proxy endpoint (if enabled)"
  value       = var.rds_enabled && var.rds_enable_proxy ? module.rds.db_proxy_endpoint : ""
}

# ==============================================================================
# ElastiCache Outputs
# ==============================================================================

output "elasticache_cluster_id" {
  description = "ElastiCache cluster ID"
  value       = var.elasticache_enabled ? module.elasticache.cluster_id : ""
}

output "elasticache_cluster_arn" {
  description = "ElastiCache cluster ARN"
  value       = var.elasticache_enabled ? module.elasticache.cluster_arn : ""
}

output "elasticache_primary_endpoint" {
  description = "ElastiCache primary endpoint"
  value       = var.elasticache_enabled ? module.elasticache.primary_endpoint : ""
}

output "elasticache_reader_endpoint" {
  description = "ElastiCache reader endpoint"
  value       = var.elasticache_enabled ? module.elasticache.reader_endpoint : ""
}

output "elasticache_configuration_endpoint" {
  description = "ElastiCache configuration endpoint"
  value       = var.elasticache_enabled ? module.elasticache.configuration_endpoint : ""
}

output "elasticache_port" {
  description = "ElastiCache port"
  value       = 6379
}

output "elasticache_security_group_id" {
  description = "ElastiCache security group ID"
  value       = var.elasticache_enabled ? module.elasticache.security_group_id : ""
}

output "elasticache_subnet_group_name" {
  description = "ElastiCache subnet group name"
  value       = var.elasticache_enabled ? module.elasticache.subnet_group_name : ""
}

output "elasticache_parameter_group_name" {
  description = "ElastiCache parameter group name"
  value       = var.elasticache_enabled ? module.elasticache.parameter_group_name : ""
}

output "elasticache_auth_token_secret_arn" {
  description = "ARN of Secrets Manager secret containing Redis auth token"
  value       = var.elasticache_enabled && var.secrets_manager_enabled ? aws_secretsmanager_secret.redis_auth[0].arn : ""
}

output "elasticache_auth_token_secret_name" {
  description = "Name of Secrets Manager secret containing Redis auth token"
  value       = var.elasticache_enabled && var.secrets_manager_enabled ? aws_secretsmanager_secret.redis_auth[0].name : ""
}

output "elasticache_connection_url" {
  description = "Redis connection URL (without auth)"
  value       = var.elasticache_enabled ? "redis://${module.elasticache.primary_endpoint}:6379" : ""
}

# ==============================================================================
# S3 Outputs
# ==============================================================================

output "s3_bucket_models_name" {
  description = "S3 bucket name for models"
  value       = module.s3.bucket_models_name
}

output "s3_bucket_models_arn" {
  description = "S3 bucket ARN for models"
  value       = module.s3.bucket_models_arn
}

output "s3_bucket_cache_name" {
  description = "S3 bucket name for cache"
  value       = module.s3.bucket_cache_name
}

output "s3_bucket_cache_arn" {
  description = "S3 bucket ARN for cache"
  value       = module.s3.bucket_cache_arn
}

output "s3_bucket_backups_name" {
  description = "S3 bucket name for backups"
  value       = module.s3.bucket_backups_name
}

output "s3_bucket_backups_arn" {
  description = "S3 bucket ARN for backups"
  value       = module.s3.bucket_backups_arn
}

output "s3_bucket_logs_name" {
  description = "S3 bucket name for logs"
  value       = module.s3.bucket_logs_name
}

output "s3_bucket_logs_arn" {
  description = "S3 bucket ARN for logs"
  value       = module.s3.bucket_logs_arn
}

output "s3_bucket_artifacts_name" {
  description = "S3 bucket name for artifacts"
  value       = module.s3.bucket_artifacts_name
}

output "s3_bucket_artifacts_arn" {
  description = "S3 bucket ARN for artifacts"
  value       = module.s3.bucket_artifacts_arn
}

output "s3_bucket_terraform_state_name" {
  description = "S3 bucket name for Terraform state"
  value       = module.s3.bucket_terraform_state_name
}

output "s3_bucket_terraform_state_arn" {
  description = "S3 bucket ARN for Terraform state"
  value       = module.s3.bucket_terraform_state_arn
}

output "dynamodb_table_terraform_locks_name" {
  description = "DynamoDB table name for Terraform state locking"
  value       = module.s3.dynamodb_table_terraform_locks_name
}

output "s3_kms_key_arn" {
  description = "KMS key ARN for S3 encryption"
  value       = module.s3.kms_key_arn
}

output "s3_kms_key_id" {
  description = "KMS key ID for S3 encryption"
  value       = module.s3.kms_key_id
}

output "s3_bucket_backups_dr_name" {
  description = "S3 bucket name for backups (DR region)"
  value       = var.enable_cross_region_replication ? module.s3.bucket_backups_dr_name : ""
}

output "s3_bucket_backups_dr_arn" {
  description = "S3 bucket ARN for backups (DR region)"
  value       = var.enable_cross_region_replication ? module.s3.bucket_backups_dr_arn : ""
}

# ==============================================================================
# IAM Outputs
# ==============================================================================

output "iam_application_role_arn" {
  description = "ARN of the application IAM role"
  value       = module.iam.application_role_arn
}

output "iam_application_role_name" {
  description = "Name of the application IAM role"
  value       = module.iam.application_role_name
}

output "iam_s3_access_role_arn" {
  description = "ARN of the S3 access IAM role"
  value       = module.iam.s3_access_role_arn
}

output "iam_cloudwatch_monitoring_role_arn" {
  description = "ARN of the CloudWatch monitoring IAM role"
  value       = module.iam.cloudwatch_monitoring_role_arn
}

output "iam_backup_role_arn" {
  description = "ARN of the backup IAM role"
  value       = module.iam.backup_role_arn
}

output "iam_lambda_role_arn" {
  description = "ARN of the Lambda IAM role"
  value       = module.iam.lambda_role_arn
}

output "iam_eks_lb_controller_role_arn" {
  description = "ARN of the EKS Load Balancer Controller IAM role"
  value       = var.eks_enable_irsa ? module.iam.eks_lb_controller_role_arn : ""
}

output "iam_eks_external_dns_role_arn" {
  description = "ARN of the EKS External DNS IAM role"
  value       = var.eks_enable_irsa && var.enable_route53 ? module.iam.eks_external_dns_role_arn : ""
}

output "iam_eks_cluster_autoscaler_role_arn" {
  description = "ARN of the EKS Cluster Autoscaler IAM role"
  value       = var.eks_enable_irsa && var.enable_cluster_autoscaler ? module.iam.eks_cluster_autoscaler_role_arn : ""
}

output "iam_eks_external_secrets_role_arn" {
  description = "ARN of the EKS External Secrets IAM role"
  value       = var.eks_enable_irsa ? module.iam.eks_external_secrets_role_arn : ""
}

output "iam_eks_node_instance_profile_name" {
  description = "Name of the EKS node instance profile"
  value       = module.iam.eks_node_instance_profile_name
}

output "iam_administrator_group_name" {
  description = "Name of the administrator IAM group"
  value       = var.create_admin_group ? module.iam.administrator_group_name : ""
}

output "iam_developer_group_name" {
  description = "Name of the developer IAM group"
  value       = var.create_developer_group ? module.iam.developer_group_name : ""
}

output "iam_readonly_group_name" {
  description = "Name of the read-only IAM group"
  value       = var.create_readonly_group ? module.iam.readonly_group_name : ""
}

# ==============================================================================
# Security Outputs
# ==============================================================================

output "kms_key_s3_arn" {
  description = "ARN of the S3 KMS key"
  value       = aws_kms_key.s3.arn
}

output "kms_key_s3_id" {
  description = "ID of the S3 KMS key"
  value       = aws_kms_key.s3.key_id
}

output "kms_key_rds_arn" {
  description = "ARN of the RDS KMS key"
  value       = var.rds_enabled && var.rds_storage_encrypted && var.rds_kms_key_arn == "" ? aws_kms_key.rds[0].arn : ""
}

output "kms_key_elasticache_arn" {
  description = "ARN of the ElastiCache KMS key"
  value       = var.elasticache_enabled && var.elasticache_at_rest_encryption_enabled && var.elasticache_kms_key_arn == "" ? aws_kms_key.elasticache[0].arn : ""
}

output "kms_key_eks_arn" {
  description = "ARN of the EKS KMS key"
  value       = var.eks_cluster_encryption_config.enabled ? aws_kms_key.eks[0].arn : ""
}

# ==============================================================================
# CloudWatch Outputs
# ==============================================================================

output "cloudwatch_log_group_eks_name" {
  description = "Name of the EKS CloudWatch log group"
  value       = aws_cloudwatch_log_group.eks_cluster.name
}

output "cloudwatch_log_group_eks_arn" {
  description = "ARN of the EKS CloudWatch log group"
  value       = aws_cloudwatch_log_group.eks_cluster.arn
}

output "cloudwatch_dashboard_name" {
  description = "Name of the CloudWatch dashboard"
  value       = var.enable_cloudwatch_dashboard ? aws_cloudwatch_dashboard.main[0].dashboard_name : ""
}

# ==============================================================================
# Application Configuration Outputs
# ==============================================================================

output "application_database_url" {
  description = "Database URL for application configuration"
  value       = var.rds_enabled ? "postgresql://${var.rds_username}:<password>@${module.rds.db_instance_address}:${module.rds.db_instance_port}/${var.rds_database_name}" : ""
  sensitive   = true
}

output "application_redis_url" {
  description = "Redis URL for application configuration"
  value       = var.elasticache_enabled ? "redis://:<auth-token>@${module.elasticache.primary_endpoint}:6379" : ""
  sensitive   = true
}

output "application_s3_models_bucket" {
  description = "S3 bucket name for models (application config)"
  value       = module.s3.bucket_models_name
}

output "application_s3_cache_bucket" {
  description = "S3 bucket name for cache (application config)"
  value       = module.s3.bucket_cache_name
}

output "application_s3_backups_bucket" {
  description = "S3 bucket name for backups (application config)"
  value       = module.s3.bucket_backups_name
}

# ==============================================================================
# Kubernetes Configuration Outputs
# ==============================================================================

output "kubernetes_config_map_name" {
  description = "Name of the Kubernetes ConfigMap for application configuration"
  value       = "ai-provider-config"
}

output "kubernetes_namespace" {
  description = "Kubernetes namespace for deployment"
  value       = "ai-provider"
}

output "kubernetes_service_account_name" {
  description = "Kubernetes service account name for IRSA"
  value       = "ai-provider-service-account"
}

# ==============================================================================
# DNS and Domain Outputs
# ==============================================================================

output "route53_zone_id" {
  description = "Route53 hosted zone ID"
  value       = var.enable_route53 && var.route53_zone_id != "" ? var.route53_zone_id : ""
}

output "domain_name" {
  description = "Domain name for the application"
  value       = var.domain_name
}

output "acm_certificate_arn" {
  description = "ARN of the ACM certificate"
  value       = var.acm_certificate_arn
}

# ==============================================================================
# Monitoring and Alerting Outputs
# ==============================================================================

output "cloudwatch_alarms_enabled" {
  description = "Whether CloudWatch alarms are enabled"
  value       = var.enable_cloudwatch_alarms
}

output "sns_topic_arn" {
  description = "ARN of the SNS topic for alerts"
  value       = var.enable_cloudwatch_alarms && var.cloudwatch_alarm_actions != [] ? var.cloudwatch_alarm_actions[0] : ""
}

# ==============================================================================
# Backup and DR Outputs
# ==============================================================================

output "backup_enabled" {
  description = "Whether AWS Backup is enabled"
  value       = var.enable_backup_plan
}

output "backup_vault_name" {
  description = "Name of the AWS Backup vault"
  value       = var.enable_backup_plan ? "${local.name_prefix}-backup-vault" : ""
}

output "cross_region_replication_enabled" {
  description = "Whether cross-region replication is enabled"
  value       = var.enable_cross_region_replication
}

output "dr_region" {
  description = "Disaster recovery region"
  value       = var.enable_cross_region_replication ? var.dr_region : ""
}

# ==============================================================================
# Cost Management Outputs
# ==============================================================================

output "cost_explorer_enabled" {
  description = "Whether Cost Explorer is enabled"
  value       = var.enable_cost_explorer
}

output "budget_amount" {
  description = "Monthly budget amount"
  value       = var.enable_cost_explorer ? var.budget_amount : 0
}

# ==============================================================================
# Security and Compliance Outputs
# ==============================================================================

output "guardduty_enabled" {
  description = "Whether Amazon GuardDuty is enabled"
  value       = var.enable_guardduty
}

output "security_hub_enabled" {
  description = "Whether AWS Security Hub is enabled"
  value       = var.enable_security_hub
}

output "config_enabled" {
  description = "Whether AWS Config is enabled"
  value       = var.enable_config
}

output "cloudtrail_enabled" {
  description = "Whether AWS CloudTrail is enabled"
  value       = var.enable_cloudtrail
}

# ==============================================================================
# Environment Configuration
# ==============================================================================

output "environment_config" {
  description = "Environment configuration for applications"
  value = {
    # General
    project_name     = var.project_name
    environment      = var.environment
    region           = var.aws_region

    # VPC
    vpc_id           = module.vpc.vpc_id
    vpc_cidr         = module.vpc.vpc_cidr_block

    # EKS
    eks_cluster_name = module.eks.cluster_name
    eks_endpoint     = module.eks.cluster_endpoint

    # Database
    db_host          = var.rds_enabled ? module.rds.db_instance_address : ""
    db_port          = var.rds_enabled ? module.rds.db_instance_port : 0
    db_name          = var.rds_enabled ? var.rds_database_name : ""
    db_secret_arn    = var.rds_enabled && var.secrets_manager_enabled ? aws_secretsmanager_secret.db_credentials[0].arn : ""

    # Redis
    redis_host       = var.elasticache_enabled ? module.elasticache.primary_endpoint : ""
    redis_port       = 6379
    redis_secret_arn = var.elasticache_enabled && var.secrets_manager_enabled ? aws_secretsmanager_secret.redis_auth[0].arn : ""

    # S3
    models_bucket    = module.s3.bucket_models_name
    cache_bucket     = module.s3.bucket_cache_name
    backups_bucket   = module.s3.bucket_backups_name
    logs_bucket      = module.s3.bucket_logs_name
    artifacts_bucket = module.s3.bucket_artifacts_name

    # Security
    kms_key_arn      = aws_kms_key.s3.arn
  }
  sensitive = true
}

# ==============================================================================
# Deployment Information
# ==============================================================================

output "deployment_info" {
  description = "Deployment information and commands"
  value = {
    # EKS
    configure_kubectl = "aws eks update-kubeconfig --name ${module.eks.cluster_name} --region ${var.aws_region}"
    get_nodes         = "kubectl get nodes"
    get_pods          = "kubectl get pods -A"

    # Database
    connect_to_db     = var.rds_enabled ? "psql -h ${module.rds.db_instance_address} -p ${module.rds.db_instance_port} -U ${var.rds_username} -d ${var.rds_database_name}" : ""

    # Redis
    connect_to_redis  = var.elasticache_enabled ? "redis-cli -h ${module.elasticache.primary_endpoint} -p 6379" : ""

    # S3
    list_models       = "aws s3 ls s3://${module.s3.bucket_models_name}/"
    list_backups      = "aws s3 ls s3://${module.s3.bucket_backups_name}/"

    # Terraform
    terraform_init    = "terraform init"
    terraform_plan    = "terraform plan"
    terraform_apply   = "terraform apply"
    terraform_destroy = "terraform destroy"
  }
}

# ==============================================================================
# Tags
# ==============================================================================

output "common_tags" {
  description = "Common tags applied to all resources"
  value       = local.merged_tags
}

# ==============================================================================
# Summary Output
# ==============================================================================

output "infrastructure_summary" {
  description = "Summary of deployed infrastructure"
  value = <<-EOT

    ========================================================================
    AI Provider Infrastructure Summary
    ========================================================================

    Project:        ${var.project_name}
    Environment:    ${var.environment}
    Region:         ${var.aws_region}
    Account ID:     ${data.aws_caller_identity.current.account_id}

    ------------------------------------------------------------------------
    Network
    ------------------------------------------------------------------------
    VPC ID:         ${module.vpc.vpc_id}
    VPC CIDR:       ${module.vpc.vpc_cidr_block}
    Public Subnets: ${length(module.vpc.public_subnet_ids)}
    Private Subnets: ${length(module.vpc.private_subnet_ids)}
    Database Subnets: ${length(module.vpc.database_subnet_ids)}

    ------------------------------------------------------------------------
    Kubernetes
    ------------------------------------------------------------------------
    Cluster Name:   ${module.eks.cluster_name}
    Cluster Version: ${module.eks.cluster_version}
    Cluster ARN:    ${module.eks.cluster_arn}
    Node Groups:    ${length(var.eks_node_groups)}

    Configure kubectl:
    aws eks update-kubeconfig --name ${module.eks.cluster_name} --region ${var.aws_region}

    ------------------------------------------------------------------------
    Database (RDS)
    ------------------------------------------------------------------------
    Enabled:        ${var.rds_enabled}
    Endpoint:       ${var.rds_enabled ? module.rds.db_instance_endpoint : "N/A"}
    Database:       ${var.rds_enabled ? var.rds_database_name : "N/A"}
    Multi-AZ:       ${var.rds_enabled && var.rds_multi_az ? "Yes" : "No"}

    ------------------------------------------------------------------------
    Cache (ElastiCache)
    ------------------------------------------------------------------------
    Enabled:        ${var.elasticache_enabled}
    Primary:        ${var.elasticache_enabled ? module.elasticache.primary_endpoint : "N/A"}
    Reader:         ${var.elasticache_enabled ? module.elasticache.reader_endpoint : "N/A"}
    Node Groups:    ${var.elasticache_enabled ? var.elasticache_num_node_groups : 0}

    ------------------------------------------------------------------------
    Storage (S3)
    ------------------------------------------------------------------------
    Models Bucket:      ${module.s3.bucket_models_name}
    Cache Bucket:       ${module.s3.bucket_cache_name}
    Backups Bucket:     ${module.s3.bucket_backups_name}
    Logs Bucket:        ${module.s3.bucket_logs_name}
    Artifacts Bucket:   ${module.s3.bucket_artifacts_name}
    Terraform State:    ${module.s3.bucket_terraform_state_name}

    ------------------------------------------------------------------------
    Security
    ------------------------------------------------------------------------
    KMS Key (S3):       ${aws_kms_key.s3.arn}
    Secrets Manager:    ${var.secrets_manager_enabled ? "Enabled" : "Disabled"}
    GuardDuty:          ${var.enable_guardduty ? "Enabled" : "Disabled"}
    Security Hub:       ${var.enable_security_hub ? "Enabled" : "Disabled"}

    ------------------------------------------------------------------------
    Monitoring
    ------------------------------------------------------------------------
    CloudWatch Alarms:  ${var.enable_cloudwatch_alarms ? "Enabled" : "Disabled"}
    CloudWatch Dashboard: ${var.enable_cloudwatch_dashboard ? "Enabled" : "Disabled"}

    ------------------------------------------------------------------------
    Disaster Recovery
    ------------------------------------------------------------------------
    Cross-Region Replication: ${var.enable_cross_region_replication ? "Enabled" : "Disabled"}
    DR Region:          ${var.enable_cross_region_replication ? var.dr_region : "N/A"}
    AWS Backup:         ${var.enable_backup_plan ? "Enabled" : "Disabled"}

    ------------------------------------------------------------------------
    Cost Management
    ------------------------------------------------------------------------
    Cost Explorer:      ${var.enable_cost_explorer ? "Enabled" : "Disabled"}
    Monthly Budget:     ${var.enable_cost_explorer ? "$${var.budget_amount}" : "N/A"}

    ========================================================================
    EOT
}
