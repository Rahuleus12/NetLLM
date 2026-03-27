# ==============================================================================
# AI Provider - CloudWatch Monitoring Configuration
# ==============================================================================
# This file defines comprehensive CloudWatch monitoring including:
# - CloudWatch Dashboards
# - CloudWatch Alarms for all services
# - SNS Topics for notifications
# - CloudWatch Log Groups
# - Anomaly Detection
# - Composite Alarms
# ==============================================================================

# ==============================================================================
# Data Sources
# ==============================================================================

data "aws_caller_identity" "current" {}

data "aws_region" "current" {}

# ==============================================================================
# SNS Topics for Alarms
# ==============================================================================

# SNS topic for critical alerts
resource "aws_sns_topic" "critical_alerts" {
  name = "${local.name_prefix}-critical-alerts"

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-critical-alerts"
  })
}

# SNS topic for warning alerts
resource "aws_sns_topic" "warning_alerts" {
  name = "${local.name_prefix}-warning-alerts"

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-warning-alerts"
  })
}

# SNS topic for informational alerts
resource "aws_sns_topic" "info_alerts" {
  name = "${local.name_prefix}-info-alerts"

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-info-alerts"
  })
}

# SNS topic subscriptions
resource "aws_sns_topic_subscription" "critical_email" {
  count = length(var.budget_alert_emails) > 0 ? 1 : 0

  topic_arn = aws_sns_topic.critical_alerts.arn
  protocol  = "email"
  endpoint  = var.budget_alert_emails[0]
}

resource "aws_sns_topic_subscription" "warning_email" {
  count = length(var.budget_alert_emails) > 0 ? 1 : 0

  topic_arn = aws_sns_topic.warning_alerts.arn
  protocol  = "email"
  endpoint  = var.budget_alert_emails[0]
}

# ==============================================================================
# CloudWatch Log Groups
# ==============================================================================

# Log group for application logs
resource "aws_cloudwatch_log_group" "application" {
  name              = "/aws/ecs/${local.name_prefix}/application"
  retention_in_days = var.cloudwatch_log_retention_days

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-application-logs"
  })
}

# Log group for EKS cluster logs
resource "aws_cloudwatch_log_group" "eks_cluster" {
  name              = "/aws/eks/${local.eks_cluster_name}/cluster"
  retention_in_days = var.eks_cluster_log_retention_days

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-eks-cluster-logs"
  })
}

# Log group for Lambda logs
resource "aws_cloudwatch_log_group" "lambda" {
  name              = "/aws/lambda/${local.name_prefix}"
  retention_in_days = var.cloudwatch_log_retention_days

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-lambda-logs"
  })
}

# Log group for API Gateway logs
resource "aws_cloudwatch_log_group" "api_gateway" {
  name              = "/aws/apigateway/${local.name_prefix}"
  retention_in_days = var.cloudwatch_log_retention_days

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-api-gateway-logs"
  })
}

# Log group for authentication logs
resource "aws_cloudwatch_log_group" "auth" {
  name              = "/aws/${local.name_prefix}/auth"
  retention_in_days = var.cloudwatch_log_retention_days

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-auth-logs"
  })
}

# Log group for audit logs
resource "aws_cloudwatch_log_group" "audit" {
  name              = "/aws/${local.name_prefix}/audit"
  retention_in_days = 90  # Keep audit logs longer

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-audit-logs"
  })
}

# ==============================================================================
# CloudWatch Dashboard
# ==============================================================================

resource "aws_cloudwatch_dashboard" "main" {
  count = var.enable_cloudwatch_dashboard ? 1 : 0

  dashboard_name = local.dashboard_name

  dashboard_body = jsonencode({
    widgets = [
      # EKS Cluster Overview
      {
        type   = "metric"
        x      = 0
        y      = 0
        width  = 12
        height = 6

        properties = {
          title = "EKS Cluster - CPU & Memory"
          view  = "timeSeries"
          stacked = false
          metrics = [
            ["AWS/ContainerInsights", "cluster_cpu_utilization", "ClusterName", local.eks_cluster_name, { stat = "Average", period = 300 }],
            [".", "cluster_memory_utilization", ".", ".", { stat = "Average", period = 300 }]
          ]
          region = data.aws_region.current.name
          title  = "EKS Cluster Utilization"
        }
      },
      # RDS Overview
      {
        type   = "metric"
        x      = 12
        y      = 0
        width  = 12
        height = 6

        properties = {
          title = "RDS - Performance"
          view  = "timeSeries"
          stacked = false
          metrics = [
            ["AWS/RDS", "CPUUtilization", "DBInstanceIdentifier", "${local.name_prefix}-postgres", { stat = "Average", period = 300 }],
            [".", "FreeableMemory", ".", ".", { stat = "Average", period = 300 }],
            [".", "ReadIOPS", ".", ".", { stat = "Average", period = 300 }],
            [".", "WriteIOPS", ".", ".", { stat = "Average", period = 300 }]
          ]
          region = data.aws_region.current.name
        }
      },
      # ElastiCache Overview
      {
        type   = "metric"
        x      = 0
        y      = 6
        width  = 12
        height = 6

        properties = {
          title = "ElastiCache - Redis Performance"
          view  = "timeSeries"
          stacked = false
          metrics = [
            ["AWS/ElastiCache", "CPUUtilization", "CacheClusterId", "${local.name_prefix}-redis", { stat = "Average", period = 300 }],
            [".", "FreeableMemory", ".", ".", { stat = "Average", period = 300 }],
            [".", "CacheHits", ".", ".", { stat = "Sum", period = 300 }],
            [".", "CacheMisses", ".", ".", { stat = "Sum", period = 300 }]
          ]
          region = data.aws_region.current.name
        }
      },
      # Application Performance
      {
        type   = "metric"
        x      = 12
        y      = 6
        width  = 12
        height = 6

        properties = {
          title = "Application - Request Latency"
          view  = "timeSeries"
          stacked = false
          metrics = [
            ["AIProvider/Application", "RequestLatency", { stat = "p95", period = 300 }],
            [".", "RequestLatency", { stat = "p99", period = 300 }],
            [".", "ErrorRate", { stat = "Average", period = 300 }]
          ]
          region = data.aws_region.current.name
        }
      },
      # EKS Node Count
      {
        type   = "metric"
        x      = 0
        y      = 12
        width  = 6
        height = 6

        properties = {
          title = "EKS - Node Count"
          view  = "timeSeries"
          stacked = false
          metrics = [
            ["AWS/ContainerInsights", "node_cpu_utilization", "ClusterName", local.eks_cluster_name, { stat = "Average", period = 300 }]
          ]
          region = data.aws_region.current.name
        }
      },
      # Pod Count
      {
        type   = "metric"
        x      = 6
        y      = 12
        width  = 6
        height = 6

        properties = {
          title = "EKS - Pod Count"
          view  = "timeSeries"
          stacked = false
          metrics = [
            ["AWS/ContainerInsights", "pod_number_of_running_pods", "ClusterName", local.eks_cluster_name, { stat = "Average", period = 300 }]
          ]
          region = data.aws_region.current.name
        }
      },
      # S3 Bucket Size
      {
        type   = "metric"
        x      = 12
        y      = 12
        width  = 6
        height = 6

        properties = {
          title = "S3 - Bucket Sizes"
          view  = "timeSeries"
          stacked = false
          metrics = [
            ["AWS/S3", "BucketSizeBytes", "BucketName", aws_s3_bucket.models.bucket, "StorageType", "StandardStorage", { stat = "Average", period = 86400 }],
            [".", ".", ".", aws_s3_bucket.cache.bucket, ".", ".", { stat = "Average", period = 86400 }],
            [".", ".", ".", aws_s3_bucket.backups.bucket, ".", ".", { stat = "Average", period = 86400 }]
          ]
          region = data.aws_region.current.name
        }
      },
      # Network Traffic
      {
        type   = "metric"
        x      = 18
        y      = 12
        width  = 6
        height = 6

        properties = {
          title = "Network - Traffic"
          view  = "timeSeries"
          stacked = false
          metrics = [
            ["AWS/NetworkELB", "ProcessedBytes", { stat = "Sum", period = 300 }],
            [".", "NewFlowCount", { stat = "Sum", period = 300 }]
          ]
          region = data.aws_region.current.name
        }
      },
      # Log Insights - Error Count
      {
        type   = "log"
        x      = 0
        y      = 18
        width  = 24
        height = 6

        properties = {
          title  = "Recent Errors"
          view   = "table"
          region = data.aws_region.current.name
          logGroupNames = [
            aws_cloudwatch_log_group.application.name,
            aws_cloudwatch_log_group.eks_cluster.name
          ]
          query = "fields @timestamp, @message | filter @message like /ERROR/ | sort @timestamp desc | limit 20"
        }
      }
    ]
  })
}

# ==============================================================================
# EKS CloudWatch Alarms
# ==============================================================================

# EKS Cluster CPU Utilization
resource "aws_cloudwatch_metric_alarm" "eks_cpu_high" {
  count = var.enable_cloudwatch_alarms ? 1 : 0

  alarm_name          = "${local.name_prefix}-eks-cpu-high"
  comparison_operator = "GreaterThanThreshold"
  evaluation_periods  = "3"
  metric_name         = "cluster_cpu_utilization"
  namespace           = "AWS/ContainerInsights"
  period              = "300"
  statistic           = "Average"
  threshold           = var.alarm_cpu_threshold
  alarm_description   = "EKS cluster CPU utilization is high"
  alarm_actions       = [aws_sns_topic.critical_alerts.arn]
  ok_actions          = [aws_sns_topic.info_alerts.arn]

  dimensions = {
    ClusterName = local.eks_cluster_name
  }

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-eks-cpu-high-alarm"
  })
}

# EKS Cluster Memory Utilization
resource "aws_cloudwatch_metric_alarm" "eks_memory_high" {
  count = var.enable_cloudwatch_alarms ? 1 : 0

  alarm_name          = "${local.name_prefix}-eks-memory-high"
  comparison_operator = "GreaterThanThreshold"
  evaluation_periods  = "3"
  metric_name         = "cluster_memory_utilization"
  namespace           = "AWS/ContainerInsights"
  period              = "300"
  statistic           = "Average"
  threshold           = var.alarm_memory_threshold
  alarm_description   = "EKS cluster memory utilization is high"
  alarm_actions       = [aws_sns_topic.critical_alerts.arn]
  ok_actions          = [aws_sns_topic.info_alerts.arn]

  dimensions = {
    ClusterName = local.eks_cluster_name
  }

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-eks-memory-high-alarm"
  })
}

# EKS Node Status
resource "aws_cloudwatch_metric_alarm" "eks_node_status" {
  count = var.enable_cloudwatch_alarms ? 1 : 0

  alarm_name          = "${local.name_prefix}-eks-node-status"
  comparison_operator = "LessThanThreshold"
  evaluation_periods  = "2"
  metric_name         = "cluster_node_count"
  namespace           = "AWS/ContainerInsights"
  period              = "300"
  statistic           = "Average"
  threshold           = "2"
  alarm_description   = "EKS cluster has less than 2 nodes running"
  alarm_actions       = [aws_sns_topic.critical_alerts.arn]
  ok_actions          = [aws_sns_topic.info_alerts.arn]

  dimensions = {
    ClusterName = local.eks_cluster_name
  }

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-eks-node-status-alarm"
  })
}

# ==============================================================================
# Application CloudWatch Alarms
# ==============================================================================

# Application Error Rate
resource "aws_cloudwatch_metric_alarm" "app_error_rate" {
  count = var.enable_cloudwatch_alarms ? 1 : 0

  alarm_name          = "${local.name_prefix}-app-error-rate"
  comparison_operator = "GreaterThanThreshold"
  evaluation_periods  = "3"
  threshold           = 5  # 5% error rate

  metric_query {
    id          = "error_rate"
    expression  = "(errors / requests) * 100"
    label       = "Error Rate (%)"
    return_data = true
  }

  metric_query {
    id = "errors"
    metric {
      metric_name = "ErrorCount"
      namespace   = "AIProvider/Application"
      period      = "300"
      stat        = "Sum"
    }
  }

  metric_query {
    id = "requests"
    metric {
      metric_name = "RequestCount"
      namespace   = "AIProvider/Application"
      period      = "300"
      stat        = "Sum"
    }
  }

  alarm_description = "Application error rate is above 5%"
  alarm_actions     = [aws_sns_topic.critical_alerts.arn]
  ok_actions        = [aws_sns_topic.info_alerts.arn]

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-app-error-rate-alarm"
  })
}

# Application Latency P95
resource "aws_cloudwatch_metric_alarm" "app_latency_p95" {
  count = var.enable_cloudwatch_alarms ? 1 : 0

  alarm_name          = "${local.name_prefix}-app-latency-p95"
  comparison_operator = "GreaterThanThreshold"
  evaluation_periods  = "3"
  metric_name         = "RequestLatency"
  namespace           = "AIProvider/Application"
  period              = "300"
  statistic           = "p95"
  threshold           = 2000  # 2 seconds
  alarm_description   = "Application P95 latency is above 2 seconds"
  alarm_actions       = [aws_sns_topic.warning_alerts.arn]
  ok_actions          = [aws_sns_topic.info_alerts.arn]

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-app-latency-p95-alarm"
  })
}

# Application 5XX Errors
resource "aws_cloudwatch_metric_alarm" "app_5xx_errors" {
  count = var.enable_cloudwatch_alarms ? 1 : 0

  alarm_name          = "${local.name_prefix}-app-5xx-errors"
  comparison_operator = "GreaterThanThreshold"
  evaluation_periods  = "2"
  metric_name         = "5XXError"
  namespace           = "AIProvider/Application"
  period              = "300"
  statistic           = "Sum"
  threshold           = 10
  alarm_description   = "Application is returning more than 10 5XX errors per 5 minutes"
  alarm_actions       = [aws_sns_topic.critical_alerts.arn]
  ok_actions          = [aws_sns_topic.info_alerts.arn]

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-app-5xx-errors-alarm"
  })
}

# ==============================================================================
# Composite Alarms
# ==============================================================================

# Composite alarm for overall system health
resource "aws_cloudwatch_composite_alarm" "system_health" {
  count = var.enable_cloudwatch_alarms ? 1 : 0

  alarm_name        = "${local.name_prefix}-system-health"
  alarm_description = "Composite alarm for overall system health"

  actions_enabled = true
  alarm_actions   = [aws_sns_topic.critical_alerts.arn]
  ok_actions      = [aws_sns_topic.info_alerts.arn]

  alarm_rule = jsonencode({
    "Or" = [
      {
        "Alarm" = {
          "AlarmName" = aws_cloudwatch_metric_alarm.eks_cpu_high[0].alarm_name
          "Region"    = data.aws_region.current.name
        }
      },
      {
        "Alarm" = {
          "AlarmName" = aws_cloudwatch_metric_alarm.eks_memory_high[0].alarm_name
          "Region"    = data.aws_region.current.name
        }
      },
      {
        "Alarm" = {
          "AlarmName" = aws_cloudwatch_metric_alarm.rds_cpu[0].alarm_name
          "Region"    = data.aws_region.current.name
        }
      },
      {
        "Alarm" = {
          "AlarmName" = aws_cloudwatch_metric_alarm.elasticache_cpu[0].alarm_name
          "Region"    = data.aws_region.current.name
        }
      }
    ]
  })

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-system-health-composite-alarm"
  })
}

# ==============================================================================
# Anomaly Detection Alarms
# ==============================================================================

# Anomaly detection for application traffic
resource "aws_cloudwatch_metric_alarm" "app_traffic_anomaly" {
  count = var.enable_cloudwatch_alarms ? 1 : 0

  alarm_name          = "${local.name_prefix}-app-traffic-anomaly"
  comparison_operator = "GreaterThanUpperThreshold"
  evaluation_periods  = "2"
  threshold_metric_id = "e1"

  metric_query {
    id          = "e1"
    expression  = "ANOMALY_DETECTION_BAND(m1, 2)"
    label       = "RequestCount (Expected)"
    return_data = true
  }

  metric_query {
    id = "m1"
    metric {
      metric_name = "RequestCount"
      namespace   = "AIProvider/Application"
      period      = "300"
      stat        = "Sum"
    }
  }

  alarm_description = "Application traffic is anomalous"
  alarm_actions     = [aws_sns_topic.warning_alerts.arn]
  ok_actions        = [aws_sns_topic.info_alerts.arn]

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-app-traffic-anomaly-alarm"
  })
}

# Anomaly detection for database connections
resource "aws_cloudwatch_metric_alarm" "db_connections_anomaly" {
  count = var.enable_cloudwatch_alarms && var.rds_enabled ? 1 : 0

  alarm_name          = "${local.name_prefix}-db-connections-anomaly"
  comparison_operator = "GreaterThanUpperThreshold"
  evaluation_periods  = "2"
  threshold_metric_id = "e1"

  metric_query {
    id          = "e1"
    expression  = "ANOMALY_DETECTION_BAND(m1, 2)"
    label       = "DatabaseConnections (Expected)"
    return_data = true
  }

  metric_query {
    id = "m1"
    metric {
      metric_name = "DatabaseConnections"
      namespace   = "AWS/RDS"
      period      = "300"
      stat        = "Average"
      dimensions = {
        DBInstanceIdentifier = "${local.name_prefix}-postgres"
      }
    }
  }

  alarm_description = "Database connections are anomalous"
  alarm_actions     = [aws_sns_topic.warning_alerts.arn]
  ok_actions        = [aws_sns_topic.info_alerts.arn]

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-db-connections-anomaly-alarm"
  })
}

# ==============================================================================
# Budget Alarms
# ==============================================================================

# Budget for monthly costs
resource "aws_budgets_budget" "monthly" {
  count = var.enable_cost_explorer ? 1 : 0

  name         = "${local.name_prefix}-monthly-budget"
  budget_type  = "COST"
  limit_amount = var.budget_amount
  limit_unit   = "USD"
  time_unit    = "MONTHLY"

  # Cost filters
  cost_filter {
    name = "Service"
    values = [
      "Amazon Elastic Compute Cloud - Compute",
      "Amazon Relational Database Service",
      "Amazon ElastiCache",
      "Amazon Simple Storage Service",
      "Amazon Elastic Kubernetes Service"
    ]
  }

  # Notifications
  dynamic "notification" {
    for_each = var.budget_threshold_percentages
    content {
      comparison_operator        = "GREATER_THAN"
      threshold                  = notification.value
      threshold_type             = "PERCENTAGE"
      notification_type          = "ACTUAL"
      subscriber_email_addresses = var.budget_alert_emails
    }
  }

  # Forecast notifications
  notification {
    comparison_operator        = "GREATER_THAN"
    threshold                  = 100
    threshold_type             = "PERCENTAGE"
    notification_type          = "FORECASTED"
    subscriber_email_addresses = var.budget_alert_emails
  }

  tags = local.merged_tags
}

# ==============================================================================
# Metric Filters for Log Groups
# ==============================================================================

# Metric filter for application errors
resource "aws_cloudwatch_log_metric_filter" "app_errors" {
  name           = "${local.name_prefix}-app-errors"
  log_group_name = aws_cloudwatch_log_group.application.name
  pattern        = "[timestamp, level, message, ERROR=*]"

  metric_transformation {
    name      = "ErrorCount"
    namespace = "AIProvider/Application"
    value     = "1"
  }
}

# Metric filter for application warnings
resource "aws_cloudwatch_log_metric_filter" "app_warnings" {
  name           = "${local.name_prefix}-app-warnings"
  log_group_name = aws_cloudwatch_log_group.application.name
  pattern        = "[timestamp, level, message, WARN=*]"

  metric_transformation {
    name      = "WarningCount"
    namespace = "AIProvider/Application"
    value     = "1"
  }
}

# Metric filter for authentication failures
resource "aws_cloudwatch_log_metric_filter" "auth_failures" {
  name           = "${local.name_prefix}-auth-failures"
  log_group_name = aws_cloudwatch_log_group.auth.name
  pattern        = "[timestamp, level, message, AUTH_FAILED=*]"

  metric_transformation {
    name      = "AuthFailureCount"
    namespace = "AIProvider/Application"
    value     = "1"
  }
}

# ==============================================================================
# CloudWatch Agent Configuration (for EKS)
# ==============================================================================

resource "aws_cloudwatch_log_group" "cloudwatch_agent" {
  name              = "/aws/containerinsights/${local.eks_cluster_name}/performance"
  retention_in_days = var.cloudwatch_log_retention_days

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-cloudwatch-agent-logs"
  })
}

# ==============================================================================
# Synthetics Canaries (Optional)
# ==============================================================================

# Canary for health check endpoint
resource "aws_synthetics_canary" "health_check" {
  count = var.enable_synthetics ? 1 : 0

  name                 = "${local.name_prefix}-health-check"
  artifact_s3_location = "s3://${aws_s3_bucket.logs.bucket}/synthetics/"
  execution_role_arn   = aws_iam_role.synthetics[0].arn
  runtime_version      = "synthetics-nodejs-puppeteer-3.6"
  start_canary         = true

  schedule {
    expression = "rate(5 minutes)"
  }

  block_devices {
    drive_name = "/dev/xvda"
    size_in_gb = 10
  }

  run_config {
    timeout_in_seconds = 60
  }

  step {
    name = "HealthCheck"
    url  = "https://${var.domain_name}/health"
  }

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-health-check-canary"
  })
}

# IAM role for Synthetics
resource "aws_iam_role" "synthetics" {
  count = var.enable_synthetics ? 1 : 0

  name = "${local.name_prefix}-synthetics-role"

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

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-synthetics-role"
  })
}

resource "aws_iam_role_policy" "synthetics" {
  count = var.enable_synthetics ? 1 : 0

  name = "${local.name_prefix}-synthetics-policy"
  role = aws_iam_role.synthetics[0].id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "s3:PutObject",
          "s3:GetBucketLocation"
        ]
        Resource = [
          aws_s3_bucket.logs.arn,
          "${aws_s3_bucket.logs.arn}/*"
        ]
      },
      {
        Effect = "Allow"
        Action = [
          "logs:CreateLogStream",
          "logs:PutLogEvents",
          "logs:CreateLogGroup"
        ]
        Resource = "arn:aws:logs:*:*:*"
      }
    ]
  })
}

# ==============================================================================
# ServiceLens (X-Ray Integration)
# ==============================================================================

# Enable X-Ray tracing for EKS
resource "aws_xray_sampling_rule" "main" {
  count = var.enable_xray_tracing ? 1 : 0

  rule_name      = "${local.name_prefix}-sampling-rule"
  priority       = 1000
  version        = 1
  reservoir_size = 1
  fixed_rate     = 0.1
  service_name   = "*"
  service_type   = "*"
  host           = "*"
  http_method    = "*"
  url_path       = "*"
  resource_arn   = "*"

  tags = local.merged_tags
}

# ==============================================================================
# Contributor Insights
# ==============================================================================

# Contributor insights for RDS
resource "aws_cloudwatch_contributor_insights_rule" "rds" {
  count = var.rds_enabled && var.enable_contributor_insights ? 1 : 0

  name           = "${local.name_prefix}-rds-contributor-insights"
  schema_string  = jsonencode({
    Schema = {
      Name = "ContributorInsightsRule"
      Version = 1
    }
    AggregateOn = "Sum"
    Contribution = {
      Filters = [
        {
          Match = "db.load.avg"
        }
      ]
      Keys = [
        {
          Name = "query"
          Type = "String"
        }
      ]
    }
    LogGroupNames = [
      aws_cloudwatch_log_group.eks_cluster.name
    ]
  })

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-rds-contributor-insights"
  })
}

# ==============================================================================
# Alarm Summary Dashboard Widget
# ==============================================================================

resource "aws_cloudwatch_dashboard" "alarms" {
  count = var.enable_cloudwatch_dashboard ? 1 : 0

  dashboard_name = "${local.dashboard_name}-alarms"

  dashboard_body = jsonencode({
    widgets = [
      {
        type   = "alarm"
        x      = 0
        y      = 0
        width  = 24
        height = 6

        properties = {
          title = "Alarms Status"
          alarms = [
            aws_cloudwatch_metric_alarm.eks_cpu_high[0].alarm_name,
            aws_cloudwatch_metric_alarm.eks_memory_high[0].alarm_name,
            aws_cloudwatch_metric_alarm.app_error_rate[0].alarm_name,
            aws_cloudwatch_metric_alarm.app_latency_p95[0].alarm_name
          ]
        }
      }
    ]
  })
}

# ==============================================================================
# Outputs
# ==============================================================================

output "cloudwatch_log_group_application_name" {
  description = "Name of the application CloudWatch log group"
  value       = aws_cloudwatch_log_group.application.name
}

output "cloudwatch_log_group_eks_cluster_name" {
  description = "Name of the EKS cluster CloudWatch log group"
  value       = aws_cloudwatch_log_group.eks_cluster.name
}

output "cloudwatch_dashboard_name" {
  description = "Name of the CloudWatch dashboard"
  value       = var.enable_cloudwatch_dashboard ? aws_cloudwatch_dashboard.main[0].dashboard_name : ""
}

output "sns_topic_critical_alerts_arn" {
  description = "ARN of the SNS topic for critical alerts"
  value       = aws_sns_topic.critical_alerts.arn
}

output "sns_topic_warning_alerts_arn" {
  description = "ARN of the SNS topic for warning alerts"
  value       = aws_sns_topic.warning_alerts.arn
}

output "sns_topic_info_alerts_arn" {
  description = "ARN of the SNS topic for informational alerts"
  value       = aws_sns_topic.info_alerts.arn
}

output "cloudwatch_alarm_eks_cpu_high_arn" {
  description = "ARN of the EKS CPU high alarm"
  value       = var.enable_cloudwatch_alarms ? aws_cloudwatch_metric_alarm.eks_cpu_high[0].arn : ""
}

output "cloudwatch_alarm_eks_memory_high_arn" {
  description = "ARN of the EKS memory high alarm"
  value       = var.enable_cloudwatch_alarms ? aws_cloudwatch_metric_alarm.eks_memory_high[0].arn : ""
}

output "cloudwatch_alarm_app_error_rate_arn" {
  description = "ARN of the application error rate alarm"
  value       = var.enable_cloudwatch_alarms ? aws_cloudwatch_metric_alarm.app_error_rate[0].arn : ""
}

output "budget_monthly_id" {
  description = "ID of the monthly budget"
  value       = var.enable_cost_explorer ? aws_budgets_budget.monthly[0].id : ""
}

output "synthetics_canary_health_check_id" {
  description = "ID of the health check canary"
  value       = var.enable_synthetics ? aws_synthetics_canary.health_check[0].id : ""
}
