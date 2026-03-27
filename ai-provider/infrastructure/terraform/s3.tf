# ==============================================================================
# AI Provider - S3 Buckets Configuration
# ==============================================================================
# This file defines S3 buckets for various purposes:
# - Models: AI model storage
# - Cache: Cache storage
# - Backups: Backup storage
# - Logs: Log storage
# - Artifacts: CI/CD artifacts
# ==============================================================================

# ==============================================================================
# Data Sources
# ==============================================================================

data "aws_caller_identity" "current" {}

data "aws_region" "current" {}

# ==============================================================================
# KMS Key for S3 Encryption
# ==============================================================================

resource "aws_kms_key" "s3" {
  description             = "KMS key for S3 bucket encryption - ${var.project_name} ${var.environment}"
  deletion_window_in_days = 30
  enable_key_rotation     = true

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid    = "EnableIAMUserPermissions"
        Effect = "Allow"
        Principal = {
          AWS = "arn:aws:iam::${data.aws_caller_identity.current.account_id}:root"
        }
        Action   = "kms:*"
        Resource = "*"
      },
      {
        Sid    = "AllowS3Service"
        Effect = "Allow"
        Principal = {
          Service = "s3.amazonaws.com"
        }
        Action = [
          "kms:Decrypt",
          "kms:GenerateDataKey*"
        ]
        Resource = "*"
        Condition = {
          StringEquals = {
            "aws:SourceAccount" = data.aws_caller_identity.current.account_id
          }
        }
      },
      {
        Sid    = "AllowEC2Service"
        Effect = "Allow"
        Principal = {
          Service = "ec2.amazonaws.com"
        }
        Action = [
          "kms:Decrypt",
          "kms:GenerateDataKey*",
          "kms:CreateGrant"
        ]
        Resource = "*"
        Condition = {
          StringEquals = {
            "aws:SourceAccount" = data.aws_caller_identity.current.account_id
          }
        }
      },
      {
        Sid    = "AllowEKSService"
        Effect = "Allow"
        Principal = {
          Service = "eks.amazonaws.com"
        }
        Action = [
          "kms:Decrypt",
          "kms:GenerateDataKey*"
        ]
        Resource = "*"
      }
    ]
  })

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-s3-key"
  })
}

resource "aws_kms_alias" "s3" {
  name          = "alias/${local.name_prefix}-s3-key"
  target_key_id = aws_kms_key.s3.key_id
}

# ==============================================================================
# S3 Buckets
# ==============================================================================

# Models bucket - for storing AI models
resource "aws_s3_bucket" "models" {
  bucket        = "${local.s3_bucket_prefix}-models-${data.aws_caller_identity.current.account_id}"
  force_destroy = false

  tags = merge(local.merged_tags, {
    Name        = "${local.name_prefix}-models"
    Purpose     = "AI model storage"
    BucketType  = "models"
  })
}

# Cache bucket - for caching
resource "aws_s3_bucket" "cache" {
  bucket        = "${local.s3_bucket_prefix}-cache-${data.aws_caller_identity.current.account_id}"
  force_destroy = false

  tags = merge(local.merged_tags, {
    Name        = "${local.name_prefix}-cache"
    Purpose     = "Cache storage"
    BucketType  = "cache"
  })
}

# Backups bucket - for backups
resource "aws_s3_bucket" "backups" {
  bucket        = "${local.s3_bucket_prefix}-backups-${data.aws_caller_identity.current.account_id}"
  force_destroy = false

  tags = merge(local.merged_tags, {
    Name        = "${local.name_prefix}-backups"
    Purpose     = "Backup storage"
    BucketType  = "backups"
  })
}

# Logs bucket - for storing logs
resource "aws_s3_bucket" "logs" {
  bucket        = "${local.s3_bucket_prefix}-logs-${data.aws_caller_identity.current.account_id}"
  force_destroy = false

  tags = merge(local.merged_tags, {
    Name        = "${local.name_prefix}-logs"
    Purpose     = "Log storage"
    BucketType  = "logs"
  })
}

# Artifacts bucket - for CI/CD artifacts
resource "aws_s3_bucket" "artifacts" {
  bucket        = "${local.s3_bucket_prefix}-artifacts-${data.aws_caller_identity.current.account_id}"
  force_destroy = false

  tags = merge(local.merged_tags, {
    Name        = "${local.name_prefix}-artifacts"
    Purpose     = "CI/CD artifacts"
    BucketType  = "artifacts"
  })
}

# Terraform state bucket
resource "aws_s3_bucket" "terraform_state" {
  bucket        = "${local.s3_bucket_prefix}-terraform-state-${data.aws_caller_identity.current.account_id}"
  force_destroy = false

  tags = merge(local.merged_tags, {
    Name        = "${local.name_prefix}-terraform-state"
    Purpose     = "Terraform state storage"
    BucketType  = "terraform"
  })
}

# ==============================================================================
# S3 Bucket Versioning
# ==============================================================================

resource "aws_s3_bucket_versioning" "models" {
  bucket = aws_s3_bucket.models.id
  versioning_configuration {
    status = var.s3_buckets.models.versioning ? "Enabled" : "Suspended"
  }
}

resource "aws_s3_bucket_versioning" "cache" {
  bucket = aws_s3_bucket.cache.id
  versioning_configuration {
    status = var.s3_buckets.cache.versioning ? "Enabled" : "Suspended"
  }
}

resource "aws_s3_bucket_versioning" "backups" {
  bucket = aws_s3_bucket.backups.id
  versioning_configuration {
    status = var.s3_buckets.backups.versioning ? "Enabled" : "Suspended"
  }
}

resource "aws_s3_bucket_versioning" "logs" {
  bucket = aws_s3_bucket.logs.id
  versioning_configuration {
    status = var.s3_buckets.logs.versioning ? "Enabled" : "Suspended"
  }
}

resource "aws_s3_bucket_versioning" "artifacts" {
  bucket = aws_s3_bucket.artifacts.id
  versioning_configuration {
    status = var.s3_buckets.artifacts.versioning ? "Enabled" : "Suspended"
  }
}

resource "aws_s3_bucket_versioning" "terraform_state" {
  bucket = aws_s3_bucket.terraform_state.id
  versioning_configuration {
    status = "Enabled"
  }
}

# ==============================================================================
# S3 Bucket Encryption
# ==============================================================================

resource "aws_s3_bucket_server_side_encryption_configuration" "models" {
  bucket = aws_s3_bucket.models.id

  rule {
    apply_server_side_encryption_by_default {
      kms_master_key_id = aws_kms_key.s3.arn
      sse_algorithm     = "aws:kms"
    }
    bucket_key_enabled = true
  }
}

resource "aws_s3_bucket_server_side_encryption_configuration" "cache" {
  bucket = aws_s3_bucket.cache.id

  rule {
    apply_server_side_encryption_by_default {
      kms_master_key_id = aws_kms_key.s3.arn
      sse_algorithm     = "aws:kms"
    }
    bucket_key_enabled = true
  }
}

resource "aws_s3_bucket_server_side_encryption_configuration" "backups" {
  bucket = aws_s3_bucket.backups.id

  rule {
    apply_server_side_encryption_by_default {
      kms_master_key_id = aws_kms_key.s3.arn
      sse_algorithm     = "aws:kms"
    }
    bucket_key_enabled = true
  }
}

resource "aws_s3_bucket_server_side_encryption_configuration" "logs" {
  bucket = aws_s3_bucket.logs.id

  rule {
    apply_server_side_encryption_by_default {
      kms_master_key_id = aws_kms_key.s3.arn
      sse_algorithm     = "aws:kms"
    }
    bucket_key_enabled = true
  }
}

resource "aws_s3_bucket_server_side_encryption_configuration" "artifacts" {
  bucket = aws_s3_bucket.artifacts.id

  rule {
    apply_server_side_encryption_by_default {
      kms_master_key_id = aws_kms_key.s3.arn
      sse_algorithm     = "aws:kms"
    }
    bucket_key_enabled = true
  }
}

resource "aws_s3_bucket_server_side_encryption_configuration" "terraform_state" {
  bucket = aws_s3_bucket.terraform_state.id

  rule {
    apply_server_side_encryption_by_default {
      kms_master_key_id = aws_kms_key.s3.arn
      sse_algorithm     = "aws:kms"
    }
    bucket_key_enabled = true
  }
}

# ==============================================================================
# S3 Bucket Public Access Block
# ==============================================================================

resource "aws_s3_bucket_public_access_block" "models" {
  bucket = aws_s3_bucket.models.id

  block_public_acls       = var.s3_buckets.models.block_public
  block_public_policy     = var.s3_buckets.models.block_public
  ignore_public_acls      = var.s3_buckets.models.block_public
  restrict_public_buckets = var.s3_buckets.models.block_public
}

resource "aws_s3_bucket_public_access_block" "cache" {
  bucket = aws_s3_bucket.cache.id

  block_public_acls       = var.s3_buckets.cache.block_public
  block_public_policy     = var.s3_buckets.cache.block_public
  ignore_public_acls      = var.s3_buckets.cache.block_public
  restrict_public_buckets = var.s3_buckets.cache.block_public
}

resource "aws_s3_bucket_public_access_block" "backups" {
  bucket = aws_s3_bucket.backups.id

  block_public_acls       = var.s3_buckets.backups.block_public
  block_public_policy     = var.s3_buckets.backups.block_public
  ignore_public_acls      = var.s3_buckets.backups.block_public
  restrict_public_buckets = var.s3_buckets.backups.block_public
}

resource "aws_s3_bucket_public_access_block" "logs" {
  bucket = aws_s3_bucket.logs.id

  block_public_acls       = var.s3_buckets.logs.block_public
  block_public_policy     = var.s3_buckets.logs.block_public
  ignore_public_acls      = var.s3_buckets.logs.block_public
  restrict_public_buckets = var.s3_buckets.logs.block_public
}

resource "aws_s3_bucket_public_access_block" "artifacts" {
  bucket = aws_s3_bucket.artifacts.id

  block_public_acls       = var.s3_buckets.artifacts.block_public
  block_public_policy     = var.s3_buckets.artifacts.block_public
  ignore_public_acls      = var.s3_buckets.artifacts.block_public
  restrict_public_buckets = var.s3_buckets.artifacts.block_public
}

resource "aws_s3_bucket_public_access_block" "terraform_state" {
  bucket = aws_s3_bucket.terraform_state.id

  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

# ==============================================================================
# S3 Bucket Lifecycle Configuration
# ==============================================================================

resource "aws_s3_bucket_lifecycle_configuration" "models" {
  count = var.s3_buckets.models.enable_lifecycle ? 1 : 0

  bucket = aws_s3_bucket.models.id

  rule {
    id     = "transition-to-ia"
    status = "Enabled"

    transition {
      days          = 90
      storage_class = "STANDARD_IA"
    }

    transition {
      days          = 180
      storage_class = "GLACIER"
    }

    expiration {
      days = 365
    }

    noncurrent_version_transition {
      noncurrent_days = 30
      storage_class   = "STANDARD_IA"
    }

    noncurrent_version_expiration {
      noncurrent_days = 90
    }
  }

  rule {
    id     = "abort-incomplete-multipart-upload"
    status = "Enabled"

    abort_incomplete_multipart_upload {
      days_after_initiation = 7
    }
  }
}

resource "aws_s3_bucket_lifecycle_configuration" "cache" {
  count = var.s3_buckets.cache.enable_lifecycle ? 1 : 0

  bucket = aws_s3_bucket.cache.id

  rule {
    id     = "expire-old-cache"
    status = "Enabled"

    expiration {
      days = 30
    }

    noncurrent_version_expiration {
      noncurrent_days = 7
    }
  }

  rule {
    id     = "abort-incomplete-multipart-upload"
    status = "Enabled"

    abort_incomplete_multipart_upload {
      days_after_initiation = 3
    }
  }
}

resource "aws_s3_bucket_lifecycle_configuration" "backups" {
  count = var.s3_buckets.backups.enable_lifecycle ? 1 : 0

  bucket = aws_s3_bucket.backups.id

  rule {
    id     = "transition-to-glacier"
    status = "Enabled"

    transition {
      days          = 30
      storage_class = "STANDARD_IA"
    }

    transition {
      days          = 90
      storage_class = "GLACIER"
    }

    transition {
      days          = 365
      storage_class = "DEEP_ARCHIVE"
    }

    expiration {
      days = 2555  # 7 years
    }

    noncurrent_version_transition {
      noncurrent_days = 30
      storage_class   = "GLACIER"
    }

    noncurrent_version_expiration {
      noncurrent_days = 90
    }
  }

  rule {
    id     = "abort-incomplete-multipart-upload"
    status = "Enabled"

    abort_incomplete_multipart_upload {
      days_after_initiation = 7
    }
  }
}

resource "aws_s3_bucket_lifecycle_configuration" "logs" {
  count = var.s3_buckets.logs.enable_lifecycle ? 1 : 0

  bucket = aws_s3_bucket.logs.id

  rule {
    id     = "expire-old-logs"
    status = "Enabled"

    transition {
      days          = 30
      storage_class = "STANDARD_IA"
    }

    transition {
      days          = 90
      storage_class = "GLACIER"
    }

    expiration {
      days = 90
    }
  }

  rule {
    id     = "abort-incomplete-multipart-upload"
    status = "Enabled"

    abort_incomplete_multipart_upload {
      days_after_initiation = 3
    }
  }
}

resource "aws_s3_bucket_lifecycle_configuration" "artifacts" {
  count = var.s3_buckets.artifacts.enable_lifecycle ? 1 : 0

  bucket = aws_s3_bucket.artifacts.id

  rule {
    id     = "expire-old-artifacts"
    status = "Enabled"

    expiration {
      days = 90
    }

    noncurrent_version_transition {
      noncurrent_days = 30
      storage_class   = "STANDARD_IA"
    }

    noncurrent_version_expiration {
      noncurrent_days = 60
    }
  }

  rule {
    id     = "abort-incomplete-multipart-upload"
    status = "Enabled"

    abort_incomplete_multipart_upload {
      days_after_initiation = 7
    }
  }
}

# ==============================================================================
# S3 Bucket Logging
# ==============================================================================

resource "aws_s3_bucket_logging" "models" {
  bucket = aws_s3_bucket.models.id

  target_bucket = aws_s3_bucket.logs.id
  target_prefix = "s3-access-logs/models/"
}

resource "aws_s3_bucket_logging" "cache" {
  bucket = aws_s3_bucket.cache.id

  target_bucket = aws_s3_bucket.logs.id
  target_prefix = "s3-access-logs/cache/"
}

resource "aws_s3_bucket_logging" "backups" {
  bucket = aws_s3_bucket.backups.id

  target_bucket = aws_s3_bucket.logs.id
  target_prefix = "s3-access-logs/backups/"
}

resource "aws_s3_bucket_logging" "artifacts" {
  bucket = aws_s3_bucket.artifacts.id

  target_bucket = aws_s3_bucket.logs.id
  target_prefix = "s3-access-logs/artifacts/"
}

# ==============================================================================
# S3 Bucket Policies
# ==============================================================================

resource "aws_s3_bucket_policy" "models" {
  bucket = aws_s3_bucket.models.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid       = "DenyIncorrectEncryptionHeader"
        Effect    = "Deny"
        Principal = "*"
        Action    = "s3:PutObject"
        Resource  = "${aws_s3_bucket.models.arn}/*"
        Condition = {
          StringNotEquals = {
            "s3:x-amz-server-side-encryption" = "aws:kms"
          }
        }
      },
      {
        Sid       = "DenyUnEncryptedObjectUploads"
        Effect    = "Deny"
        Principal = "*"
        Action    = "s3:PutObject"
        Resource  = "${aws_s3_bucket.models.arn}/*"
        Condition = {
          "Null" = {
            "s3:x-amz-server-side-encryption" = "true"
          }
        }
      },
      {
        Sid       = "AllowReadFromEKS"
        Effect    = "Allow"
        Principal = {
          AWS = module.eks.cluster_role_arn
        }
        Action = [
          "s3:GetObject",
          "s3:ListBucket"
        ]
        Resource = [
          aws_s3_bucket.models.arn,
          "${aws_s3_bucket.models.arn}/*"
        ]
      },
      {
        Sid       = "AllowWriteFromEKS"
        Effect    = "Allow"
        Principal = {
          AWS = module.eks.node_role_arn
        }
        Action = [
          "s3:PutObject",
          "s3:GetObject",
          "s3:DeleteObject",
          "s3:ListBucket"
        ]
        Resource = [
          aws_s3_bucket.models.arn,
          "${aws_s3_bucket.models.arn}/*"
        ]
      },
      {
        Sid       = "EnforceTLS"
        Effect    = "Deny"
        Principal = "*"
        Action    = "s3:*"
        Resource = [
          aws_s3_bucket.models.arn,
          "${aws_s3_bucket.models.arn}/*"
        ]
        Condition = {
          Bool = {
            "aws:SecureTransport" = "false"
          }
        }
      }
    ]
  })
}

resource "aws_s3_bucket_policy" "backups" {
  bucket = aws_s3_bucket.backups.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid       = "DenyIncorrectEncryptionHeader"
        Effect    = "Deny"
        Principal = "*"
        Action    = "s3:PutObject"
        Resource  = "${aws_s3_bucket.backups.arn}/*"
        Condition = {
          StringNotEquals = {
            "s3:x-amz-server-side-encryption" = "aws:kms"
          }
        }
      },
      {
        Sid       = "DenyUnEncryptedObjectUploads"
        Effect    = "Deny"
        Principal = "*"
        Action    = "s3:PutObject"
        Resource  = "${aws_s3_bucket.backups.arn}/*"
        Condition = {
          "Null" = {
            "s3:x-amz-server-side-encryption" = "true"
          }
        }
      },
      {
        Sid       = "AllowBackupOperations"
        Effect    = "Allow"
        Principal = {
          AWS = module.eks.node_role_arn
        }
        Action = [
          "s3:PutObject",
          "s3:GetObject",
          "s3:DeleteObject",
          "s3:ListBucket"
        ]
        Resource = [
          aws_s3_bucket.backups.arn,
          "${aws_s3_bucket.backups.arn}/*"
        ]
      },
      {
        Sid       = "EnforceTLS"
        Effect    = "Deny"
        Principal = "*"
        Action    = "s3:*"
        Resource = [
          aws_s3_bucket.backups.arn,
          "${aws_s3_bucket.backups.arn}/*"
        ]
        Condition = {
          Bool = {
            "aws:SecureTransport" = "false"
          }
        }
      }
    ]
  })
}

# ==============================================================================
# S3 Bucket CORS Configuration
# ==============================================================================

resource "aws_s3_bucket_cors_configuration" "artifacts" {
  bucket = aws_s3_bucket.artifacts.id

  cors_rule {
    allowed_headers = ["*"]
    allowed_methods = ["GET", "HEAD", "PUT", "POST", "DELETE"]
    allowed_origins = ["*"]
    expose_headers  = ["ETag", "x-amz-server-side-encryption"]
    max_age_seconds = 3000
  }
}

# ==============================================================================
# DynamoDB Table for Terraform State Locking
# ==============================================================================

resource "aws_dynamodb_table" "terraform_locks" {
  name         = "${local.name_prefix}-terraform-locks"
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "LockID"

  attribute {
    name = "LockID"
    type = "S"
  }

  tags = merge(local.merged_tags, {
    Name    = "${local.name_prefix}-terraform-locks"
    Purpose = "Terraform state locking"
  })
}

# ==============================================================================
# Cross-Region Replication (Optional)
# ==============================================================================

resource "aws_s3_bucket_replication_configuration" "backups" {
  count = var.enable_cross_region_replication ? 1 : 0

  role   = aws_iam_role.s3_replication[0].arn
  bucket = aws_s3_bucket.backups.id

  token = ""

  rule {
    id     = "replicate-to-dr"
    status = "Enabled"

    destination {
      bucket        = aws_s3_bucket.backups_dr[0].arn
      storage_class = "STANDARD_IA"

      encryption_configuration {
        replica_kms_key_id = aws_kms_key.s3_dr[0].arn
      }

      replication_time {
        status = "Enabled"
      }

      metrics {
        status = "Enabled"
      }
    }

    filter {}

    delete_marker_replication {
      status = "Disabled"
    }
  }

  depends_on = [
    aws_s3_bucket_versioning.backups
  ]
}

# DR bucket for cross-region replication
resource "aws_s3_bucket" "backups_dr" {
  count = var.enable_cross_region_replication ? 1 : 0

  provider = aws.dr

  bucket   = "${local.s3_bucket_prefix}-backups-dr-${data.aws_caller_identity.current.account_id}"
  region   = var.dr_region

  tags = merge(local.merged_tags, {
    Name        = "${local.name_prefix}-backups-dr"
    Purpose     = "Backup storage (DR)"
    BucketType  = "backups-dr"
  })
}

resource "aws_s3_bucket_versioning" "backups_dr" {
  count = var.enable_cross_region_replication ? 1 : 0

  provider = aws.dr

  bucket = aws_s3_bucket.backups_dr[0].id
  versioning_configuration {
    status = "Enabled"
  }
}

# KMS key for DR region
resource "aws_kms_key" "s3_dr" {
  count = var.enable_cross_region_replication ? 1 : 0

  provider = aws.dr

  description             = "KMS key for S3 bucket encryption (DR) - ${var.project_name} ${var.environment}"
  deletion_window_in_days = 30
  enable_key_rotation     = true

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-s3-dr-key"
  })
}

# IAM role for S3 replication
resource "aws_iam_role" "s3_replication" {
  count = var.enable_cross_region_replication ? 1 : 0

  name = "${local.name_prefix}-s3-replication-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Principal = {
          Service = "s3.amazonaws.com"
        }
      }
    ]
  })

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-s3-replication-role"
  })
}

resource "aws_iam_role_policy" "s3_replication" {
  count = var.enable_cross_region_replication ? 1 : 0

  name = "${local.name_prefix}-s3-replication-policy"
  role = aws_iam_role.s3_replication[0].id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "s3:GetReplicationConfiguration",
          "s3:ListBucket"
        ]
        Resource = [
          aws_s3_bucket.backups.arn,
          aws_s3_bucket.models.arn
        ]
      },
      {
        Effect = "Allow"
        Action = [
          "s3:GetObjectVersionForReplication",
          "s3:GetObjectVersionAcl",
          "s3:GetObjectVersionTagging"
        ]
        Resource = [
          "${aws_s3_bucket.backups.arn}/*",
          "${aws_s3_bucket.models.arn}/*"
        ]
      },
      {
        Effect = "Allow"
        Action = [
          "s3:ReplicateObject",
          "s3:ReplicateDelete",
          "s3:ReplicateTags"
        ]
        Resource = [
          "${aws_s3_bucket.backups_dr[0].arn}/*"
        ]
      },
      {
        Effect = "Allow"
        Action = [
          "kms:Decrypt",
          "kms:Encrypt",
          "kms:GenerateDataKey"
        ]
        Resource = [
          aws_kms_key.s3.arn,
          aws_kms_key.s3_dr[0].arn
        ]
      }
    ]
  })
}

# ==============================================================================
# Outputs
# ==============================================================================

output "s3_bucket_models_id" {
  description = "S3 bucket ID for models"
  value       = aws_s3_bucket.models.id
}

output "s3_bucket_models_arn" {
  description = "S3 bucket ARN for models"
  value       = aws_s3_bucket.models.arn
}

output "s3_bucket_models_name" {
  description = "S3 bucket name for models"
  value       = aws_s3_bucket.models.bucket
}

output "s3_bucket_cache_id" {
  description = "S3 bucket ID for cache"
  value       = aws_s3_bucket.cache.id
}

output "s3_bucket_cache_arn" {
  description = "S3 bucket ARN for cache"
  value       = aws_s3_bucket.cache.arn
}

output "s3_bucket_cache_name" {
  description = "S3 bucket name for cache"
  value       = aws_s3_bucket.cache.bucket
}

output "s3_bucket_backups_id" {
  description = "S3 bucket ID for backups"
  value       = aws_s3_bucket.backups.id
}

output "s3_bucket_backups_arn" {
  description = "S3 bucket ARN for backups"
  value       = aws_s3_bucket.backups.arn
}

output "s3_bucket_backups_name" {
  description = "S3 bucket name for backups"
  value       = aws_s3_bucket.backups.bucket
}

output "s3_bucket_logs_id" {
  description = "S3 bucket ID for logs"
  value       = aws_s3_bucket.logs.id
}

output "s3_bucket_logs_arn" {
  description = "S3 bucket ARN for logs"
  value       = aws_s3_bucket.logs.arn
}

output "s3_bucket_logs_name" {
  description = "S3 bucket name for logs"
  value       = aws_s3_bucket.logs.bucket
}

output "s3_bucket_artifacts_id" {
  description = "S3 bucket ID for artifacts"
  value       = aws_s3_bucket.artifacts.id
}

output "s3_bucket_artifacts_arn" {
  description = "S3 bucket ARN for artifacts"
  value       = aws_s3_bucket.artifacts.arn
}

output "s3_bucket_artifacts_name" {
  description = "S3 bucket name for artifacts"
  value       = aws_s3_bucket.artifacts.bucket
}

output "s3_bucket_terraform_state_id" {
  description = "S3 bucket ID for Terraform state"
  value       = aws_s3_bucket.terraform_state.id
}

output "s3_bucket_terraform_state_arn" {
  description = "S3 bucket ARN for Terraform state"
  value       = aws_s3_bucket.terraform_state.arn
}

output "s3_bucket_terraform_state_name" {
  description = "S3 bucket name for Terraform state"
  value       = aws_s3_bucket.terraform_state.bucket
}

output "dynamodb_table_terraform_locks_name" {
  description = "DynamoDB table name for Terraform locks"
  value       = aws_dynamodb_table.terraform_locks.name
}

output "dynamodb_table_terraform_locks_arn" {
  description = "DynamoDB table ARN for Terraform locks"
  value       = aws_dynamodb_table.terraform_locks.arn
}

output "s3_kms_key_arn" {
  description = "KMS key ARN for S3 encryption"
  value       = aws_kms_key.s3.arn
}

output "s3_kms_key_id" {
  description = "KMS key ID for S3 encryption"
  value       = aws_kms_key.s3.key_id
}

output "s3_bucket_backups_dr_id" {
  description = "S3 bucket ID for backups (DR region)"
  value       = var.enable_cross_region_replication ? aws_s3_bucket.backups_dr[0].id : ""
}

output "s3_bucket_backups_dr_arn" {
  description = "S3 bucket ARN for backups (DR region)"
  value       = var.enable_cross_region_replication ? aws_s3_bucket.backups_dr[0].arn : ""
}
