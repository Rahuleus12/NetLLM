# ==============================================================================
# AI Provider - ElastiCache Redis Configuration
# ==============================================================================
# This file defines the ElastiCache Redis cluster infrastructure for the
# AI Provider application with production-grade configurations.
# ==============================================================================

# ==============================================================================
# Data Sources
# ==============================================================================

data "aws_region" "current" {}

data "aws_caller_identity" "current" {}

# ==============================================================================
# Random Password Generation for Redis Auth Token
# ==============================================================================

resource "random_password" "redis_auth_token" {
  count = var.elasticache_enabled && var.elasticache_auth_token == "" ? 1 : 0

  length           = 64
  special          = true
  override_special = "!#$%&*()-_=+[]{}<>:?"
  min_special      = 4
  min_upper        = 4
  min_lower        = 4
  min_numeric      = 4
}

# ==============================================================================
# ElastiCache Subnet Group
# ==============================================================================

resource "aws_elasticache_subnet_group" "main" {
  count = var.elasticache_enabled ? 1 : 0

  name        = "${local.name_prefix}-cache-subnet-group"
  description = "ElastiCache subnet group for ${var.project_name} ${var.environment}"
  subnet_ids  = module.vpc.database_subnet_ids

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-cache-subnet-group"
  })
}

# ==============================================================================
# ElastiCache Parameter Group
# ==============================================================================

resource "aws_elasticache_parameter_group" "main" {
  count = var.elasticache_enabled ? 1 : 0

  name        = "${local.name_prefix}-redis-params"
  family      = var.elasticache_parameter_group_family
  description = "Custom Redis parameter group for ${var.project_name}"

  # Memory management
  parameter {
    name  = "maxmemory-policy"
    value = "allkeys-lru"
  }

  # Timeout settings
  parameter {
    name  = "timeout"
    value = "300"
  }

  # TCP keepalive
  parameter {
    name  = "tcp-keepalive"
    value = "60"
  }

  # Maxmemory samples for LRU algorithm
  parameter {
    name  = "maxmemory-samples"
    value = "10"
  }

  # Lazy expiration
  parameter {
    name  = "lazy-expire"
    value = "yes"
  }

  # Lazy deletion
  parameter {
    name  = "lazyfree-lazy-eviction"
    value = "yes"
  }

  parameter {
    name  = "lazyfree-lazy-expire"
    value = "yes"
  }

  parameter {
    name  = "lazyfree-lazy-server-del"
    value = "yes"
  }

  # Replication settings
  parameter {
    name  = "replica-serve-stale-data"
    value = "yes"
  }

  parameter {
    name  = "replica-read-only"
    value = "yes"
  }

  # Client output buffer
  parameter {
    name  = "client-output-buffer-limit"
    value = "normal 0 0 0 slave 268435456 67108864 60 pubsub 33554432 8388608 60"
  }

  # Dynamic parameters
  dynamic "parameter" {
    for_each = var.elasticache_parameters
    content {
      name  = parameter.value.name
      value = parameter.value.value
    }
  }

  lifecycle {
    create_before_destroy = true
  }

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-redis-params"
  })
}

# ==============================================================================
# Security Group for ElastiCache
# ==============================================================================

resource "aws_security_group" "elasticache" {
  count = var.elasticache_enabled ? 1 : 0

  name        = "${local.name_prefix}-elasticache-sg"
  description = "Security group for ElastiCache Redis - ${var.project_name}"
  vpc_id      = module.vpc.vpc_id

  # Redis port
  ingress {
    description     = "Redis from EKS cluster"
    from_port       = 6379
    to_port         = 6379
    protocol        = "tcp"
    security_groups = [module.eks.cluster_primary_security_group_id]
  }

  ingress {
    description     = "Redis from private subnets"
    from_port       = 6379
    to_port         = 6379
    protocol        = "tcp"
    cidr_blocks     = [var.vpc_cidr]
  }

  # Redis Cluster bus port (for cluster mode)
  ingress {
    description     = "Redis cluster bus from EKS cluster"
    from_port       = 6379
    to_port         = 6380
    protocol        = "tcp"
    security_groups = [module.eks.cluster_primary_security_group_id]
  }

  # Allow all outbound traffic
  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-elasticache-sg"
  })

  lifecycle {
    create_before_destroy = true
  }
}

# ==============================================================================
# KMS Key for ElastiCache Encryption
# ==============================================================================

resource "aws_kms_key" "elasticache" {
  count = var.elasticache_enabled && var.elasticache_at_rest_encryption_enabled && var.elasticache_kms_key_arn == "" ? 1 : 0

  description             = "KMS key for ElastiCache encryption - ${var.project_name} ${var.environment}"
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
        Sid    = "Allow ElastiCache to use the key"
        Effect = "Allow"
        Principal = {
          Service = [
            "elasticache.amazonaws.com",
            "rds.amazonaws.com"
          ]
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
    Name = "${local.name_prefix}-elasticache-key"
  })
}

resource "aws_kms_alias" "elasticache" {
  count = var.elasticache_enabled && var.elasticache_at_rest_encryption_enabled && var.elasticache_kms_key_arn == "" ? 1 : 0

  name          = "alias/${local.name_prefix}-elasticache-key"
  target_key_id = aws_kms_key.elasticache[0].key_id
}

# ==============================================================================
# ElastiCache Replication Group (Cluster Mode Enabled)
# ==============================================================================

resource "aws_elasticache_replication_group" "main" {
  count = var.elasticache_enabled ? 1 : 0

  replication_group_id          = "${local.name_prefix}-redis"
  replication_group_description = "Redis cluster for ${var.project_name} ${var.environment}"

  # Engine configuration
  engine               = "redis"
  engine_version       = var.elasticache_engine_version
  node_type            = var.elasticache_node_type
  num_cache_clusters   = var.elasticache_num_cache_clusters
  parameter_group_name = aws_elasticache_parameter_group.main[0].name

  # Cluster mode configuration
  num_node_groups         = var.elasticache_num_node_groups
  replicas_per_node_group = var.elasticache_replicas_per_node_group

  # Network configuration
  subnet_group_name  = aws_elasticache_subnet_group.main[0].name
  security_group_ids = [aws_security_group.elasticache[0].id]

  # High availability
  automatic_failover_enabled = var.elasticache_automatic_failover_enabled
  multi_az_enabled          = var.elasticache_multi_az_enabled

  # Encryption
  at_rest_encryption_enabled = var.elasticache_at_rest_encryption_enabled
  transit_encryption_enabled = var.elasticache_transit_encryption_enabled
  auth_token                 = var.elasticache_auth_token != "" ? var.elasticache_auth_token : random_password.redis_auth_token[0].result
  kms_key_id                 = var.elasticache_at_rest_encryption_enabled ? (var.elasticache_kms_key_arn != "" ? var.elasticache_kms_key_arn : aws_kms_key.elasticache[0].arn) : null

  # Snapshot configuration
  snapshot_retention_limit = var.elasticache_snapshot_retention_limit
  snapshot_window         = var.elasticache_snapshot_window
  snapshot_name           = null

  # Maintenance
  maintenance_window = var.elasticache_maintenance_window

  # Notifications
  notification_topic_arn = var.elasticache_notification_topic_arn != "" ? var.elasticache_notification_topic_arn : null

  # Tags
  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-redis"
  })

  # Lifecycle
  lifecycle {
    ignore_changes = [
      num_cache_clusters,
      num_node_groups,
      replicas_per_node_group
    ]
  }

  # Dependencies
  depends_on = [
    aws_elasticache_subnet_group.main,
    aws_elasticache_parameter_group.main,
    aws_security_group.elasticache
  ]
}

# ==============================================================================
# Secrets Manager - Redis Auth Token
# ==============================================================================

resource "aws_secretsmanager_secret" "redis_auth" {
  count = var.elasticache_enabled && var.secrets_manager_enabled ? 1 : 0

  name                    = local.redis_secret_name
  description             = "Redis auth token for ${var.project_name} ${var.environment}"
  recovery_window_in_days = 7

  kms_key_id = var.secrets_kms_key_id != "" ? var.secrets_kms_key_id : null

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-redis-auth"
  })
}

resource "aws_secretsmanager_secret_version" "redis_auth" {
  count = var.elasticache_enabled && var.secrets_manager_enabled ? 1 : 0

  secret_id = aws_secretsmanager_secret.redis_auth[0].id
  secret_string = jsonencode({
    auth_token       = var.elasticache_auth_token != "" ? var.elasticache_auth_token : random_password.redis_auth_token[0].result
    primary_endpoint = aws_elasticache_replication_group.main[0].primary_endpoint_address
    reader_endpoint  = aws_elasticache_replication_group.main[0].reader_endpoint_address
    port             = 6379
    connection_url   = "redis://:${var.elasticache_auth_token != "" ? var.elasticache_auth_token : random_password.redis_auth_token[0].result}@${aws_elasticache_replication_group.main[0].primary_endpoint_address}:6379"
  })
}

# ==============================================================================
# CloudWatch Alarms for ElastiCache
# ==============================================================================

resource "aws_cloudwatch_metric_alarm" "elasticache_cpu" {
  count = var.elasticache_enabled && var.enable_cloudwatch_alarms ? 1 : 0

  alarm_name          = "${local.name_prefix}-elasticache-cpu-utilization"
  comparison_operator = "GreaterThanThreshold"
  evaluation_periods  = "3"
  metric_name         = "CPUUtilization"
  namespace           = "AWS/ElastiCache"
  period              = "300"
  statistic           = "Average"
  threshold           = var.alarm_cpu_threshold
  alarm_description   = "This metric monitors ElastiCache CPU utilization"
  alarm_actions       = var.cloudwatch_alarm_actions
  ok_actions          = var.cloudwatch_alarm_actions

  dimensions = {
    CacheClusterId = aws_elasticache_replication_group.main[0].id
  }

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-elasticache-cpu-alarm"
  })
}

resource "aws_cloudwatch_metric_alarm" "elasticache_memory" {
  count = var.elasticache_enabled && var.enable_cloudwatch_alarms ? 1 : 0

  alarm_name          = "${local.name_prefix}-elasticache-freeable-memory"
  comparison_operator = "LessThanThreshold"
  evaluation_periods  = "3"
  metric_name         = "FreeableMemory"
  namespace           = "AWS/ElastiCache"
  period              = "300"
  statistic           = "Average"
  threshold           = 134217728  # 128MB in bytes
  alarm_description   = "This metric monitors ElastiCache freeable memory"
  alarm_actions       = var.cloudwatch_alarm_actions
  ok_actions          = var.cloudwatch_alarm_actions

  dimensions = {
    CacheClusterId = aws_elasticache_replication_group.main[0].id
  }

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-elasticache-memory-alarm"
  })
}

resource "aws_cloudwatch_metric_alarm" "elasticache_connections" {
  count = var.elasticache_enabled && var.enable_cloudwatch_alarms ? 1 : 0

  alarm_name          = "${local.name_prefix}-elasticache-connections"
  comparison_operator = "GreaterThanThreshold"
  evaluation_periods  = "3"
  metric_name         = "CurrConnections"
  namespace           = "AWS/ElastiCache"
  period              = "300"
  statistic           = "Average"
  threshold           = var.alarm_elasticache_connections_threshold
  alarm_description   = "This metric monitors ElastiCache connections"
  alarm_actions       = var.cloudwatch_alarm_actions
  ok_actions          = var.cloudwatch_alarm_actions

  dimensions = {
    CacheClusterId = aws_elasticache_replication_group.main[0].id
  }

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-elasticache-connections-alarm"
  })
}

resource "aws_cloudwatch_metric_alarm" "elasticache_evictions" {
  count = var.elasticache_enabled && var.enable_cloudwatch_alarms ? 1 : 0

  alarm_name          = "${local.name_prefix}-elasticache-evictions"
  comparison_operator = "GreaterThanThreshold"
  evaluation_periods  = "3"
  metric_name         = "Evictions"
  namespace           = "AWS/ElastiCache"
  period              = "300"
  statistic           = "Average"
  threshold           = 1000
  alarm_description   = "This metric monitors ElastiCache evictions"
  alarm_actions       = var.cloudwatch_alarm_actions
  ok_actions          = var.cloudwatch_alarm_actions

  dimensions = {
    CacheClusterId = aws_elasticache_replication_group.main[0].id
  }

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-elasticache-evictions-alarm"
  })
}

resource "aws_cloudwatch_metric_alarm" "elasticache_cache_hit_rate" {
  count = var.elasticache_enabled && var.enable_cloudwatch_alarms ? 1 : 0

  alarm_name          = "${local.name_prefix}-elasticache-cache-hit-rate"
  comparison_operator = "LessThanThreshold"
  evaluation_periods  = "3"

  metric_query {
    id          = "hit_rate"
    expression  = "(cache_hits / (cache_hits + cache_misses)) * 100"
    label       = "Cache Hit Rate"
    return_data = true
  }

  metric_query {
    id = "cache_hits"
    metric {
      metric_name = "CacheHits"
      namespace   = "AWS/ElastiCache"
      period      = "300"
      stat        = "Sum"
      dimensions = {
        CacheClusterId = aws_elasticache_replication_group.main[0].id
      }
    }
  }

  metric_query {
    id = "cache_misses"
    metric {
      metric_name = "CacheMisses"
      namespace   = "AWS/ElastiCache"
      period      = "300"
      stat        = "Sum"
      dimensions = {
        CacheClusterId = aws_elasticache_replication_group.main[0].id
      }
    }
  }

  threshold         = 80  # Alert if hit rate below 80%
  alarm_description = "This metric monitors ElastiCache cache hit rate"
  alarm_actions     = var.cloudwatch_alarm_actions
  ok_actions        = var.cloudwatch_alarm_actions

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-elasticache-hit-rate-alarm"
  })
}

resource "aws_cloudwatch_metric_alarm" "elasticache_replication_lag" {
  count = var.elasticache_enabled && var.enable_cloudwatch_alarms && var.elasticache_num_node_groups > 1 ? 1 : 0

  alarm_name          = "${local.name_prefix}-elasticache-replication-lag"
  comparison_operator = "GreaterThanThreshold"
  evaluation_periods  = "3"
  metric_name         = "ReplicationLag"
  namespace           = "AWS/ElastiCache"
  period              = "60"
  statistic           = "Average"
  threshold           = 5  # 5 seconds
  alarm_description   = "This metric monitors ElastiCache replication lag"
  alarm_actions       = var.cloudwatch_alarm_actions
  ok_actions          = var.cloudwatch_alarm_actions

  dimensions = {
    CacheClusterId = aws_elasticache_replication_group.main[0].id
  }

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-elasticache-replication-lag-alarm"
  })
}

resource "aws_cloudwatch_metric_alarm" "elasticache_swap_usage" {
  count = var.elasticache_enabled && var.enable_cloudwatch_alarms ? 1 : 0

  alarm_name          = "${local.name_prefix}-elasticache-swap-usage"
  comparison_operator = "GreaterThanThreshold"
  evaluation_periods  = "3"
  metric_name         = "SwapUsage"
  namespace           = "AWS/ElastiCache"
  period              = "300"
  statistic           = "Average"
  threshold           = 67108864  # 64MB in bytes
  alarm_description   = "This metric monitors ElastiCache swap usage"
  alarm_actions       = var.cloudwatch_alarm_actions
  ok_actions          = var.cloudwatch_alarm_actions

  dimensions = {
    CacheClusterId = aws_elasticache_replication_group.main[0].id
  }

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-elasticache-swap-usage-alarm"
  })
}

# ==============================================================================
# ElastiCache Global Replication Group (Optional - for Cross-Region DR)
# ==============================================================================

# resource "aws_elasticache_global_replication_group" "main" {
#   count = var.elasticache_enabled && var.enable_cross_region_replication ? 1 : 0
#
#   global_replication_group_id_suffix = "${local.name_prefix}-global"
#   primary_replication_group_id       = aws_elasticache_replication_group.main[0].id
#
#   tags = merge(local.merged_tags, {
#     Name = "${local.name_prefix}-global-redis"
#   })
# }

# ==============================================================================
# Outputs
# ==============================================================================

output "elasticache_cluster_id" {
  description = "ElastiCache cluster ID"
  value       = var.elasticache_enabled ? aws_elasticache_replication_group.main[0].id : ""
}

output "elasticache_cluster_arn" {
  description = "ElastiCache cluster ARN"
  value       = var.elasticache_enabled ? aws_elasticache_replication_group.main[0].arn : ""
}

output "elasticache_primary_endpoint" {
  description = "ElastiCache primary endpoint"
  value       = var.elasticache_enabled ? aws_elasticache_replication_group.main[0].primary_endpoint_address : ""
}

output "elasticache_reader_endpoint" {
  description = "ElastiCache reader endpoint"
  value       = var.elasticache_enabled ? aws_elasticache_replication_group.main[0].reader_endpoint_address : ""
}

output "elasticache_configuration_endpoint" {
  description = "ElastiCache configuration endpoint"
  value       = var.elasticache_enabled ? aws_elasticache_replication_group.main[0].configuration_endpoint_address : ""
}

output "elasticache_port" {
  description = "ElastiCache port"
  value       = 6379
}

output "elasticache_security_group_id" {
  description = "ElastiCache security group ID"
  value       = var.elasticache_enabled ? aws_security_group.elasticache[0].id : ""
}

output "elasticache_subnet_group_name" {
  description = "ElastiCache subnet group name"
  value       = var.elasticache_enabled ? aws_elasticache_subnet_group.main[0].name : ""
}

output "elasticache_parameter_group_name" {
  description = "ElastiCache parameter group name"
  value       = var.elasticache_enabled ? aws_elasticache_parameter_group.main[0].name : ""
}

output "elasticache_auth_token_secret_arn" {
  description = "ARN of Secrets Manager secret containing Redis auth token"
  value       = var.elasticache_enabled && var.secrets_manager_enabled ? aws_secretsmanager_secret.redis_auth[0].arn : ""
}

output "elasticache_connection_string" {
  description = "Redis connection string"
  value       = var.elasticache_enabled ? "redis://${aws_elasticache_replication_group.main[0].primary_endpoint_address}:6379" : ""
  sensitive   = true
}
