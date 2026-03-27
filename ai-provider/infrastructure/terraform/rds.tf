# ==============================================================================
# AI Provider - RDS PostgreSQL Database Configuration
# ==============================================================================
# This file defines the RDS PostgreSQL database infrastructure for the
# AI Provider application with production-grade configurations.
# ==============================================================================

# ==============================================================================
# Data Sources
# ==============================================================================

data "aws_db_instance" "selected" {
  db_instance_identifier = aws_db_instance.main.id
}

# ==============================================================================
# Random Password Generation
# ==============================================================================

resource "random_password" "db_master_password" {
  count = var.rds_enabled && var.rds_master_password == "" ? 1 : 0

  length           = 32
  special          = true
  override_special = "!#$%&*()-_=+[]{}<>:?"
  min_special      = 2
  min_upper        = 2
  min_lower        = 2
  min_numeric      = 2
}

# ==============================================================================
# DB Subnet Group
# ==============================================================================

resource "aws_db_subnet_group" "main" {
  count = var.rds_enabled ? 1 : 0

  name        = "${local.name_prefix}-db-subnet-group"
  description = "Database subnet group for ${var.project_name} ${var.environment}"
  subnet_ids  = module.vpc.database_subnet_ids

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-db-subnet-group"
  })
}

# ==============================================================================
# DB Parameter Group
# ==============================================================================

resource "aws_db_parameter_group" "main" {
  count = var.rds_enabled ? 1 : 0

  name        = "${local.name_prefix}-postgres-params"
  family      = var.rds_parameter_group_family
  description = "Custom PostgreSQL parameter group for ${var.project_name}"

  # Performance optimization parameters
  parameter {
    name  = "max_connections"
    value = "500"
  }

  parameter {
    name  = "shared_buffers"
    value = "{DBInstanceClassMemory/4}"
  }

  parameter {
    name  = "work_mem"
    value = "262144"  # 256MB in KB
  }

  parameter {
    name  = "maintenance_work_mem"
    value = "524288"  # 512MB in KB
  }

  parameter {
    name  = "effective_cache_size"
    value = "{DBInstanceClassMemory*3/4}"
  }

  parameter {
    name  = "random_page_cost"
    value = "1.1"
  }

  parameter {
    name  = "effective_io_concurrency"
    value = "200"
  }

  # WAL and checkpoint optimization
  parameter {
    name  = "wal_buffers"
    value = "16384"  # 16MB in 8KB pages
  }

  parameter {
    name  = "checkpoint_completion_target"
    value = "0.9"
  }

  parameter {
    name  = "max_wal_size"
    value = "2048"  # 2GB in MB
  }

  parameter {
    name  = "min_wal_size"
    value = "1024"  # 1GB in MB
  }

  # Query optimization
  parameter {
    name  = "default_statistics_target"
    value = "100"
  }

  parameter {
    name  = "constraint_exclusion"
    value = "partition"
  }

  # Logging parameters
  parameter {
    name  = "log_min_duration_statement"
    value = "1000"  # Log queries taking > 1 second
  }

  parameter {
    name  = "log_checkpoints"
    value = "1"
  }

  parameter {
    name  = "log_connections"
    value = "1"
  }

  parameter {
    name  = "log_disconnections"
    value = "1"
  }

  parameter {
    name  = "log_lock_waits"
    value = "1"
  }

  parameter {
    name  = "log_temp_files"
    value = "0"
  }

  parameter {
    name  = "log_autovacuum_min_duration"
    value = "0"
  }

  # Autovacuum optimization
  parameter {
    name  = "autovacuum"
    value = "1"
  }

  parameter {
    name  = "autovacuum_max_workers"
    value = "3"
  }

  parameter {
    name  = "autovacuum_naptime"
    value = "30"  # 30 seconds
  }

  parameter {
    name  = "autovacuum_vacuum_threshold"
    value = "50"
  }

  parameter {
    name  = "autovacuum_analyze_threshold"
    value = "50"
  }

  parameter {
    name  = "autovacuum_vacuum_scale_factor"
    value = "0.05"
  }

  parameter {
    name  = "autovacuum_analyze_scale_factor"
    value = "0.02"
  }

  parameter {
    name  = "autovacuum_vacuum_cost_limit"
    value = "2000"
  }

  # Statement timeout
  parameter {
    name  = "statement_timeout"
    value = "300000"  # 5 minutes in milliseconds
  }

  parameter {
    name  = "lock_timeout"
    value = "30000"  # 30 seconds in milliseconds
  }

  # SSL/TLS
  parameter {
    name  = "ssl"
    value = "1"
  }

  # Timezone
  parameter {
    name  = "timezone"
    value = "UTC"
  }

  # Dynamic parameters (can be changed without reboot)
  dynamic "parameter" {
    for_each = var.rds_parameters
    content {
      name  = parameter.value.name
      value = parameter.value.value
    }
  }

  lifecycle {
    create_before_destroy = true
  }

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-postgres-params"
  })
}

# ==============================================================================
# KMS Key for RDS Encryption
# ==============================================================================

resource "aws_kms_key" "rds" {
  count = var.rds_enabled && var.rds_storage_encrypted && var.rds_kms_key_arn == "" ? 1 : 0

  description             = "KMS key for RDS encryption - ${var.project_name} ${var.environment}"
  deletion_window_in_days = 30
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
        Sid    = "Allow RDS to use the key"
        Effect = "Allow"
        Principal = {
          Service = "rds.amazonaws.com"
        }
        Action = [
          "kms:Decrypt",
          "kms:DescribeKey",
          "kms:Encrypt",
          "kms:GenerateDataKey*",
          "kms:ReEncrypt*"
        ]
        Resource = "*"
        Condition = {
          StringEquals = {
            "kms:CallerAccount" = data.aws_caller_identity.current.account_id
          }
        }
      }
    ]
  })

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-rds-key"
  })
}

resource "aws_kms_alias" "rds" {
  count = var.rds_enabled && var.rds_storage_encrypted && var.rds_kms_key_arn == "" ? 1 : 0

  name          = "alias/${local.name_prefix}-rds-key"
  target_key_id = aws_kms_key.rds[0].key_id
}

# ==============================================================================
# Security Group for RDS
# ==============================================================================

resource "aws_security_group" "rds" {
  count = var.rds_enabled ? 1 : 0

  name        = "${local.name_prefix}-rds-sg"
  description = "Security group for RDS PostgreSQL - ${var.project_name}"
  vpc_id      = module.vpc.vpc_id

  # PostgreSQL port
  ingress {
    description     = "PostgreSQL from EKS cluster"
    from_port       = 5432
    to_port         = 5432
    protocol        = "tcp"
    security_groups = [module.eks.cluster_primary_security_group_id]
  }

  ingress {
    description     = "PostgreSQL from private subnets"
    from_port       = 5432
    to_port         = 5432
    protocol        = "tcp"
    cidr_blocks     = [var.vpc_cidr]
  }

  # Allow all outbound traffic
  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-rds-sg"
  })

  lifecycle {
    create_before_destroy = true
  }
}

# ==============================================================================
# RDS Instance
# ==============================================================================

resource "aws_db_instance" "main" {
  count = var.rds_enabled ? 1 : 0

  identifier = "${local.name_prefix}-postgres"

  # Engine configuration
  engine         = "postgres"
  engine_version = var.rds_engine_version
  instance_class = var.rds_instance_class

  # Storage configuration
  allocated_storage     = var.rds_allocated_storage
  max_allocated_storage = var.rds_max_allocated_storage
  storage_type          = var.rds_storage_type
  storage_encrypted     = var.rds_storage_encrypted
  kms_key_id           = var.rds_kms_key_arn != "" ? var.rds_kms_key_arn : (var.rds_storage_encrypted ? aws_kms_key.rds[0].arn : null)

  # Database credentials
  db_name  = var.rds_database_name
  username = var.rds_username
  password = var.rds_master_password != "" ? var.rds_master_password : random_password.db_master_password[0].result

  # Network configuration
  db_subnet_group_name   = aws_db_subnet_group.main[0].name
  vpc_security_group_ids = [aws_security_group.rds[0].id]
  port                   = 5432

  # Multi-AZ and high availability
  multi_az               = var.rds_multi_az
  availability_zone      = var.rds_multi_az ? null : data.aws_availability_zones.available.names[0]

  # Parameter and option groups
  parameter_group_name   = aws_db_parameter_group.main[0].name
  option_group_name      = null  # Use default

  # Backup configuration
  backup_retention_period = var.rds_backup_retention_period
  backup_window          = var.rds_backup_window
  skip_final_snapshot    = var.rds_skip_final_snapshot
  final_snapshot_identifier = var.rds_skip_final_snapshot ? null : "${local.name_prefix}-final-snapshot-${formatdate("YYYYMMDD-HHmmss", timestamp())}"

  # Maintenance
  maintenance_window         = var.rds_maintenance_window
  auto_minor_version_upgrade = true

  # Deletion protection
  deletion_protection = var.rds_deletion_protection

  # Performance Insights
  performance_insights_enabled          = var.rds_performance_insights_enabled
  performance_insights_retention_period = var.rds_performance_insights_retention_period
  performance_insights_kms_key_id      = var.rds_storage_encrypted ? (var.rds_kms_key_arn != "" ? var.rds_kms_key_arn : aws_kms_key.rds[0].arn) : null

  # Monitoring
  enabled_cloudwatch_logs_exports = ["postgresql", "upgrade"]
  monitoring_interval             = 60  # Enhanced monitoring every 60 seconds
  monitoring_role_arn            = aws_iam_role.rds_monitoring[0].arn

  # Timezone
  timezone = "UTC"

  # Character set
  character_set_name = "UTF8"

  # IAM database authentication
  iam_database_authentication_enabled = true

  # Tags
  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-postgres"
  })

  # Lifecycle
  lifecycle {
    ignore_changes = [
      allocated_storage,  # Allow autoscaling
      max_allocated_storage,
      snapshot_identifier,
      final_snapshot_identifier
    ]
  }

  # Dependencies
  depends_on = [
    aws_db_subnet_group.main,
    aws_db_parameter_group.main,
    aws_security_group.rds,
    aws_iam_role.rds_monitoring
  ]
}

# ==============================================================================
# IAM Role for RDS Enhanced Monitoring
# ==============================================================================

resource "aws_iam_role" "rds_monitoring" {
  count = var.rds_enabled ? 1 : 0

  name        = "${local.name_prefix}-rds-monitoring-role"
  description = "IAM role for RDS Enhanced Monitoring"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Principal = {
          Service = "monitoring.rds.amazonaws.com"
        }
      }
    ]
  })

  permissions_boundary = var.iam_permissions_boundary != "" ? var.iam_permissions_boundary : null
  path                 = var.iam_path

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-rds-monitoring-role"
  })
}

resource "aws_iam_role_policy_attachment" "rds_monitoring" {
  count = var.rds_enabled ? 1 : 0

  role       = aws_iam_role.rds_monitoring[0].name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonRDSEnhancedMonitoringRole"
}

# ==============================================================================
# Secrets Manager - Database Credentials
# ==============================================================================

resource "aws_secretsmanager_secret" "db_credentials" {
  count = var.rds_enabled && var.secrets_manager_enabled ? 1 : 0

  name                    = local.db_secret_name
  description             = "Database credentials for ${var.project_name} ${var.environment}"
  recovery_window_in_days = 7

  kms_key_id = var.secrets_kms_key_id != "" ? var.secrets_kms_key_id : null

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-db-credentials"
  })
}

resource "aws_secretsmanager_secret_version" "db_credentials" {
  count = var.rds_enabled && var.secrets_manager_enabled ? 1 : 0

  secret_id = aws_secretsmanager_secret.db_credentials[0].id
  secret_string = jsonencode({
    username = var.rds_username
    password = var.rds_master_password != "" ? var.rds_master_password : random_password.db_master_password[0].result
    host     = aws_db_instance.main[0].address
    port     = aws_db_instance.main[0].port
    database = var.rds_database_name
    jdbc_url = "jdbc:postgresql://${aws_db_instance.main[0].address}:${aws_db_instance.main[0].port}/${var.rds_database_name}"
    conn_str = "postgresql://${var.rds_username}:${var.rds_master_password != "" ? var.rds_master_password : random_password.db_master_password[0].result}@${aws_db_instance.main[0].address}:${aws_db_instance.main[0].port}/${var.rds_database_name}"
  })
}

# ==============================================================================
# CloudWatch Alarms for RDS
# ==============================================================================

resource "aws_cloudwatch_metric_alarm" "rds_cpu" {
  count = var.rds_enabled && var.enable_cloudwatch_alarms ? 1 : 0

  alarm_name          = "${local.name_prefix}-rds-cpu-utilization"
  comparison_operator = "GreaterThanThreshold"
  evaluation_periods  = "3"
  metric_name         = "CPUUtilization"
  namespace           = "AWS/RDS"
  period              = "300"
  statistic           = "Average"
  threshold           = var.alarm_cpu_threshold
  alarm_description   = "This metric monitors RDS CPU utilization"
  alarm_actions       = var.cloudwatch_alarm_actions
  ok_actions          = var.cloudwatch_alarm_actions

  dimensions = {
    DBInstanceIdentifier = aws_db_instance.main[0].id
  }

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-rds-cpu-alarm"
  })
}

resource "aws_cloudwatch_metric_alarm" "rds_memory" {
  count = var.rds_enabled && var.enable_cloudwatch_alarms ? 1 : 0

  alarm_name          = "${local.name_prefix}-rds-freeable-memory"
  comparison_operator = "LessThanThreshold"
  evaluation_periods  = "3"
  metric_name         = "FreeableMemory"
  namespace           = "AWS/RDS"
  period              = "300"
  statistic           = "Average"
  threshold           = 268435456  # 256MB in bytes
  alarm_description   = "This metric monitors RDS freeable memory"
  alarm_actions       = var.cloudwatch_alarm_actions
  ok_actions          = var.cloudwatch_alarm_actions

  dimensions = {
    DBInstanceIdentifier = aws_db_instance.main[0].id
  }

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-rds-memory-alarm"
  })
}

resource "aws_cloudwatch_metric_alarm" "rds_storage" {
  count = var.rds_enabled && var.enable_cloudwatch_alarms ? 1 : 0

  alarm_name          = "${local.name_prefix}-rds-storage-space"
  comparison_operator = "LessThanThreshold"
  evaluation_periods  = "3"
  metric_name         = "FreeStorageSpace"
  namespace           = "AWS/RDS"
  period              = "300"
  statistic           = "Average"
  threshold           = 5368709120  # 5GB in bytes
  alarm_description   = "This metric monitors RDS free storage space"
  alarm_actions       = var.cloudwatch_alarm_actions
  ok_actions          = var.cloudwatch_alarm_actions

  dimensions = {
    DBInstanceIdentifier = aws_db_instance.main[0].id
  }

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-rds-storage-alarm"
  })
}

resource "aws_cloudwatch_metric_alarm" "rds_connections" {
  count = var.rds_enabled && var.enable_cloudwatch_alarms ? 1 : 0

  alarm_name          = "${local.name_prefix}-rds-db-connections"
  comparison_operator = "GreaterThanThreshold"
  evaluation_periods  = "3"
  metric_name         = "DatabaseConnections"
  namespace           = "AWS/RDS"
  period              = "300"
  statistic           = "Average"
  threshold           = var.alarm_rds_connections_threshold
  alarm_description   = "This metric monitors RDS database connections"
  alarm_actions       = var.cloudwatch_alarm_actions
  ok_actions          = var.cloudwatch_alarm_actions

  dimensions = {
    DBInstanceIdentifier = aws_db_instance.main[0].id
  }

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-rds-connections-alarm"
  })
}

resource "aws_cloudwatch_metric_alarm" "rds_read_latency" {
  count = var.rds_enabled && var.enable_cloudwatch_alarms ? 1 : 0

  alarm_name          = "${local.name_prefix}-rds-read-latency"
  comparison_operator = "GreaterThanThreshold"
  evaluation_periods  = "3"
  metric_name         = "ReadLatency"
  namespace           = "AWS/RDS"
  period              = "300"
  statistic           = "Average"
  threshold           = 0.1  # 100ms
  alarm_description   = "This metric monitors RDS read latency"
  alarm_actions       = var.cloudwatch_alarm_actions
  ok_actions          = var.cloudwatch_alarm_actions

  dimensions = {
    DBInstanceIdentifier = aws_db_instance.main[0].id
  }

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-rds-read-latency-alarm"
  })
}

resource "aws_cloudwatch_metric_alarm" "rds_write_latency" {
  count = var.rds_enabled && var.enable_cloudwatch_alarms ? 1 : 0

  alarm_name          = "${local.name_prefix}-rds-write-latency"
  comparison_operator = "GreaterThanThreshold"
  evaluation_periods  = "3"
  metric_name         = "WriteLatency"
  namespace           = "AWS/RDS"
  period              = "300"
  statistic           = "Average"
  threshold           = 0.1  # 100ms
  alarm_description   = "This metric monitors RDS write latency"
  alarm_actions       = var.cloudwatch_alarm_actions
  ok_actions          = var.cloudwatch_alarm_actions

  dimensions = {
    DBInstanceIdentifier = aws_db_instance.main[0].id
  }

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-rds-write-latency-alarm"
  })
}

# ==============================================================================
# RDS Proxy (Optional - for connection pooling)
# ==============================================================================

resource "aws_db_proxy" "main" {
  count = var.rds_enabled && var.rds_enable_proxy ? 1 : 0

  name                   = "${local.name_prefix}-db-proxy"
  engine_family          = "POSTGRESQL"
  require_tls            = true
  idle_client_timeout    = 1800
  debug_logging          = false

  auth {
    auth_scheme = "SECRETS"
    description = "PostgreSQL authentication via Secrets Manager"
    iam_auth    = "DISABLED"
    secret_arn  = aws_secretsmanager_secret.db_credentials[0].arn
  }

  role_arn = aws_iam_role.rds_proxy[0].arn

  vpc_subnet_ids = module.vpc.private_subnet_ids
  vpc_security_group_ids = [aws_security_group.rds[0].id]

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-db-proxy"
  })
}

resource "aws_db_proxy_default_target_group" "main" {
  count = var.rds_enabled && var.rds_enable_proxy ? 1 : 0

  db_proxy_name = aws_db_proxy.main[0].name

  connection_pool_config {
    connection_borrow_timeout = 120
    max_connections_percent   = 100
    max_idle_connections_percent = 50
    session_pinning_filters   = []
  }
}

resource "aws_db_proxy_target" "main" {
  count = var.rds_enabled && var.rds_enable_proxy ? 1 : 0

  db_proxy_name          = aws_db_proxy.main[0].name
  target_group_name      = aws_db_proxy_default_target_group.main[0].name
  db_instance_identifier = aws_db_instance.main[0].id
}

# ==============================================================================
# IAM Role for RDS Proxy
# ==============================================================================

resource "aws_iam_role" "rds_proxy" {
  count = var.rds_enabled && var.rds_enable_proxy ? 1 : 0

  name        = "${local.name_prefix}-rds-proxy-role"
  description = "IAM role for RDS Proxy"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Principal = {
          Service = "rds.amazonaws.com"
        }
      }
    ]
  })

  permissions_boundary = var.iam_permissions_boundary != "" ? var.iam_permissions_boundary : null
  path                 = var.iam_path

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-rds-proxy-role"
  })
}

resource "aws_iam_role_policy" "rds_proxy" {
  count = var.rds_enabled && var.rds_enable_proxy ? 1 : 0

  name = "${local.name_prefix}-rds-proxy-policy"
  role = aws_iam_role.rds_proxy[0].id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "secretsmanager:GetSecretValue",
          "secretsmanager:DescribeSecret"
        ]
        Resource = aws_secretsmanager_secret.db_credentials[0].arn
      },
      {
        Effect = "Allow"
        Action = [
          "kms:Decrypt"
        ]
        Resource = var.rds_storage_encrypted ? (var.rds_kms_key_arn != "" ? var.rds_kms_key_arn : aws_kms_key.rds[0].arn) : "*"
      }
    ]
  })
}

# ==============================================================================
# Outputs
# ==============================================================================

output "rds_instance_endpoint" {
  description = "RDS instance endpoint"
  value       = var.rds_enabled ? aws_db_instance.main[0].endpoint : ""
}

output "rds_instance_address" {
  description = "RDS instance address"
  value       = var.rds_enabled ? aws_db_instance.main[0].address : ""
}

output "rds_instance_port" {
  description = "RDS instance port"
  value       = var.rds_enabled ? aws_db_instance.main[0].port : 0
}

output "rds_instance_id" {
  description = "RDS instance identifier"
  value       = var.rds_enabled ? aws_db_instance.main[0].id : ""
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
  value       = var.rds_enabled ? aws_security_group.rds[0].id : ""
}

output "rds_db_subnet_group_name" {
  description = "RDS DB subnet group name"
  value       = var.rds_enabled ? aws_db_subnet_group.main[0].name : ""
}

output "rds_parameter_group_name" {
  description = "RDS parameter group name"
  value       = var.rds_enabled ? aws_db_parameter_group.main[0].name : ""
}

output "rds_credentials_secret_arn" {
  description = "ARN of Secrets Manager secret containing DB credentials"
  value       = var.rds_enabled && var.secrets_manager_enabled ? aws_secretsmanager_secret.db_credentials[0].arn : ""
}

output "rds_proxy_endpoint" {
  description = "RDS Proxy endpoint"
  value       = var.rds_enabled && var.rds_enable_proxy ? aws_db_proxy.main[0].endpoint : ""
}
