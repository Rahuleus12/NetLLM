# Terraform Provider Configuration for AI Provider Infrastructure
# This file defines the required providers and their configurations

terraform {
  required_version = ">= 1.5.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }

    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "~> 2.23"
    }

    helm = {
      source  = "hashicorp/helm"
      version = "~> 2.11"
    }

    kubectl = {
      source  = "gavinbunney/kubectl"
      version = "~> 1.14"
    }

    random = {
      source  = "hashicorp/random"
      version = "~> 3.5"
    }

    tls = {
      source  = "hashicorp/tls"
      version = "~> 4.0"
    }

    cloudinit = {
      source  = "hashicorp/cloudinit"
      version = "~> 2.3"
    }

    postgresql = {
      source  = "cyrilgdn/postgresql"
      version = "~> 1.21"
    }

    redis = {
      source  = "redis/redis"
      version = "~> 0.2"
    }
  }

  # Backend configuration for remote state management
  # Uncomment and configure based on your preferred backend

  # backend "s3" {
  #   bucket         = "ai-provider-terraform-state"
  #   key            = "infrastructure/terraform.tfstate"
  #   region         = "us-east-1"
  #   encrypt        = true
  #   dynamodb_table = "ai-provider-terraform-locks"
  #
  #   # Optional: Enable state locking
  #   # skip_metadata_api_check = false
  # }

  # backend "consul" {
  #   address = "consul.example.com:8500"
  #   path    = "ai-provider/terraform/state"
  #   lock    = true
  # }

  # backend "http" {
  #   address        = "https://terraform-state.example.com/state/ai-provider"
  #   lock_address   = "https://terraform-state.example.com/lock/ai-provider"
  #   unlock_address = "https://terraform-state.example.com/unlock/ai-provider"
  # }
}

# AWS Provider Configuration
provider "aws" {
  region = var.aws_region

  # Default tags applied to all resources
  default_tags {
    tags = {
      Project     = "AI-Provider"
      Environment = var.environment
      ManagedBy   = "Terraform"
      Repository  = "ai-provider"
      Owner       = var.owner
      CostCenter  = var.cost_center
    }
  }

  # Ignore tag changes for resources managed externally
  ignore_tags {
    key_prefixes = ["kubernetes.io/", "karpenter.sh/"]
  }
}

# AWS Provider for DNS (can be in different region/account)
provider "aws" {
  alias  = "dns"
  region = var.dns_region != "" ? var.dns_region : var.aws_region

  default_tags {
    tags = {
      Project     = "AI-Provider"
      Environment = var.environment
      ManagedBy   = "Terraform"
    }
  }
}

# AWS Provider for certificate manager (must be us-east-1 for CloudFront)
provider "aws" {
  alias  = "acm"
  region = "us-east-1"

  default_tags {
    tags = {
      Project     = "AI-Provider"
      Environment = var.environment
      ManagedBy   = "Terraform"
    }
  }
}

# Kubernetes Provider Configuration
provider "kubernetes" {
  host                   = module.eks.cluster_endpoint
  cluster_ca_certificate = base64decode(module.eks.cluster_certificate_authority_data)

  # Use EKS authentication
  exec {
    api_version = "client.authentication.k8s.io/v1beta1"
    command     = "aws"
    args        = ["eks", "get-token", "--cluster-name", module.eks.cluster_name]
  }
}

# Helm Provider Configuration
provider "helm" {
  kubernetes {
    host                   = module.eks.cluster_endpoint
    cluster_ca_certificate = base64decode(module.eks.cluster_certificate_authority_data)

    exec {
      api_version = "client.authentication.k8s.io/v1beta1"
      command     = "aws"
      args        = ["eks", "get-token", "--cluster-name", module.eks.cluster_name]
    }
  }

  # Debug mode (set via environment variable TF_HELM_DEBUG=true)
  debug = var.helm_debug
}

# Kubectl Provider Configuration
provider "kubectl" {
  host                   = module.eks.cluster_endpoint
  cluster_ca_certificate = base64decode(module.eks.cluster_certificate_authority_data)

  exec {
    api_version = "client.authentication.k8s.io/v1beta1"
    command     = "aws"
    args        = ["eks", "get-token", "--cluster-name", module.eks.cluster_name]
  }
}

# PostgreSQL Provider Configuration
provider "postgresql" {
  host            = module.rds.db_instance_endpoint
  port            = 5432
  database        = var.database_name
  username        = var.database_master_username
  password        = var.database_master_password
  sslmode         = "require"
  connect_timeout = 15

  # Expected schema for migrations
  expected_version = 150004
}

# Redis Provider Configuration (optional, for Redis Cloud)
# provider "redis" {
#   url = "redis://${module.elasticache.cluster_address}:6379"
# }

# Data sources for existing resources
data "aws_caller_identity" "current" {}

data "aws_region" "current" {}

data "aws_availability_zones" "available" {
  state = "available"
}

# Data source for latest EKS AMI
data "aws_ami" "eks" {
  most_recent = true
  owners      = ["amazon"]

  filter {
    name   = "name"
    values = ["amazon-eks-node-${var.eks_cluster_version}-v*"]
  }
}

# Data source for current partition (aws, aws-cn, aws-gov, etc.)
data "aws_partition" "current" {}
