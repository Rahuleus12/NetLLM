# Terraform Version Constraints and Provider Requirements
# This file defines the required versions for Terraform and all providers

terraform {
  required_version = ">= 1.5.0, < 2.0.0"

  required_providers {
    # AWS Provider - Primary cloud infrastructure
    aws = {
      source  = "hashicorp/aws"
      version = ">= 5.0.0, < 6.0.0"
    }

    # Kubernetes Provider - For managing K8s resources
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = ">= 2.23.0, < 3.0.0"
    }

    # Helm Provider - For deploying Helm charts
    helm = {
      source  = "hashicorp/helm"
      version = ">= 2.11.0, < 3.0.0"
    }

    # Random Provider - For generating random strings, passwords
    random = {
      source  = "hashicorp/random"
      version = ">= 3.5.0, < 4.0.0"
    }

    # Time Provider - For time-based resources and sleep
    time = {
      source  = "hashicorp/time"
      version = ">= 0.9.0, < 1.0.0"
    }

    # TLS Provider - For TLS certificate generation
    tls = {
      source  = "hashicorp/tls"
      version = ">= 4.0.0, < 5.0.0"
    }

    # CloudPosse null-label - For consistent resource naming
    null = {
      source  = "hashicorp/null"
      version = ">= 3.2.0, < 4.0.0"
    }

    # Local Provider - For local file operations
    local = {
      source  = "hashicorp/local"
      version = ">= 2.4.0, < 3.0.0"
    }

    # Archive Provider - For creating archives
    archive = {
      source  = "hashicorp/archive"
      version = ">= 2.4.0, < 3.0.0"
    }

    # HTTP Provider - For HTTP data sources
    http = {
      source  = "hashicorp/http"
      version = ">= 3.4.0, < 4.0.0"
    }

    # CloudInit Provider - For cloud-init configurations
    cloudinit = {
      source  = "hashicorp/cloudinit"
      version = ">= 2.3.0, < 3.0.0"
    }
  }

  # Backend configuration for remote state management
  # Uncomment and configure based on your state backend preference

  # S3 Backend (Recommended for production)
  # backend "s3" {
  #   bucket         = "ai-provider-terraform-state"
  #   key            = "infrastructure/terraform.tfstate"
  #   region         = "us-east-1"
  #   encrypt        = true
  #   dynamodb_table = "ai-provider-terraform-locks"
  #
  #   # Optional: Enable versioning
  #   # versioning    = true
  #
  #   # Optional: Specify role ARN for cross-account access
  #   # role_arn     = "arn:aws:iam::ACCOUNT_ID:role/TerraformRole"
  # }

  # Remote Backend - Terraform Cloud (Alternative)
  # backend "remote" {
  #   hostname     = "app.terraform.io"
  #   organization = "ai-provider"
  #
  #   workspaces {
  #     name = "ai-provider-infrastructure"
  #   }
  # }

  # HTTP Backend (Alternative)
  # backend "http" {
  #   address        = "https://terraform-state.example.com/state/ai-provider"
  #   lock_address   = "https://terraform-state.example.com/lock/ai-provider"
  #   unlock_address = "https://terraform-state.example.com/unlock/ai-provider"
  # }
}

# Provider Configuration Notes:
#
# 1. AWS Provider:
#    - Version 5.x is required for latest EKS and RDS features
#    - Supports all AWS services needed (EKS, RDS, ElastiCache, S3, IAM, etc.)
#
# 2. Kubernetes Provider:
#    - Version 2.23+ for latest K8s API support
#    - Used for deploying K8s resources after EKS cluster creation
#    - Configured to work with EKS cluster
#
# 3. Helm Provider:
#    - Version 2.11+ for latest Helm features
#    - Used for deploying Helm charts to EKS
#    - Supports Helm v3 charts
#
# 4. Random Provider:
#    - Used for generating random passwords, suffixes
#    - Ensures unique resource names
#
# 5. TLS Provider:
#    - Used for generating TLS certificates
#    - Supports self-signed certificates for development
#
# 6. Additional Providers:
#    - null: For null resources and triggers
#    - local: For local file operations
#    - archive: For creating zip archives
#    - http: For HTTP data sources
#    - cloudinit: For EC2 cloud-init configurations

# Version Upgrade Strategy:
#
# - Terraform: Follow HashiCorp's release schedule
#   - Minor version updates: Safe to upgrade
#   - Major version updates: Test thoroughly before upgrading
#
# - Providers: Follow semantic versioning
#   - Patch updates (x.x.Z): Safe to upgrade
#   - Minor updates (x.Y.x): Review changelog, test before upgrading
#   - Major updates (X.x.x): Breaking changes possible, thorough testing required
#
# - State Backend:
#   - Test state migration before changing backends
#   - Always backup state before major changes
#   - Use state locking to prevent concurrent modifications
