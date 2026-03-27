# ==============================================================================
# AI Provider - VPC and Networking Configuration
# ==============================================================================
# This file creates a production-grade VPC with:
# - Public, private, and database subnets across multiple AZs
# - NAT Gateways for internet access from private subnets
# - VPC endpoints for AWS services
# - Network ACLs and security groups
# - VPC Flow Logs for network monitoring
# ==============================================================================

# ==============================================================================
# Data Sources
# ==============================================================================

data "aws_availability_zones" "available" {
  state = "available"
}

data "aws_region" "current" {}

# ==============================================================================
# VPC
# ==============================================================================

resource "aws_vpc" "main" {
  cidr_block           = var.vpc_cidr
  enable_dns_hostnames = var.vpc_enable_dns_hostnames
  enable_dns_support   = var.vpc_enable_dns_support

  # Instance tenancy (default or dedicated)
  instance_tenancy = "default"

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-vpc"
    Type = "main"
  })
}

# ==============================================================================
# Internet Gateway
# ==============================================================================

resource "aws_internet_gateway" "main" {
  vpc_id = aws_vpc.main.id

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-igw"
  })

  depends_on = [aws_vpc.main]
}

# ==============================================================================
# Subnets - Public
# ==============================================================================

resource "aws_subnet" "public" {
  count = length(var.public_subnet_cidrs)

  vpc_id                  = aws_vpc.main.id
  cidr_block              = var.public_subnet_cidrs[count.index]
  availability_zone       = data.aws_availability_zones.available.names[count.index % length(data.aws_availability_zones.available.names)]
  map_public_ip_on_launch = true

  tags = merge(local.merged_tags, {
    Name                                           = "${local.name_prefix}-public-${count.index + 1}"
    Type                                           = "public"
    "kubernetes.io/role/elb"                       = "1"
    "kubernetes.io/cluster/${local.eks_cluster_name}" = "shared"
    AvailabilityZone                               = data.aws_availability_zones.available.names[count.index % length(data.aws_availability_zones.available.names)]
  })

  depends_on = [aws_vpc.main]
}

# ==============================================================================
# Subnets - Private
# ==============================================================================

resource "aws_subnet" "private" {
  count = length(var.private_subnet_cidrs)

  vpc_id                  = aws_vpc.main.id
  cidr_block              = var.private_subnet_cidrs[count.index]
  availability_zone       = data.aws_availability_zones.available.names[count.index % length(data.aws_availability_zones.available.names)]
  map_public_ip_on_launch = false

  tags = merge(local.merged_tags, {
    Name                                           = "${local.name_prefix}-private-${count.index + 1}"
    Type                                           = "private"
    "kubernetes.io/role/internal-elb"              = "1"
    "kubernetes.io/cluster/${local.eks_cluster_name}" = "shared"
    AvailabilityZone                               = data.aws_availability_zones.available.names[count.index % length(data.aws_availability_zones.available.names)]
  })

  depends_on = [aws_vpc.main]
}

# ==============================================================================
# Subnets - Database
# ==============================================================================

resource "aws_subnet" "database" {
  count = length(var.database_subnet_cidrs)

  vpc_id                  = aws_vpc.main.id
  cidr_block              = var.database_subnet_cidrs[count.index]
  availability_zone       = data.aws_availability_zones.available.names[count.index % length(data.aws_availability_zones.available.names)]
  map_public_ip_on_launch = false

  tags = merge(local.merged_tags, {
    Name             = "${local.name_prefix}-database-${count.index + 1}"
    Type             = "database"
    AvailabilityZone = data.aws_availability_zones.available.names[count.index % length(data.aws_availability_zones.available.names)]
  })

  depends_on = [aws_vpc.main]
}

# ==============================================================================
# Database Subnet Group
# ==============================================================================

resource "aws_db_subnet_group" "main" {
  count = var.rds_enabled ? 1 : 0

  name       = "${local.name_prefix}-db-subnet-group"
  subnet_ids = aws_subnet.database[*].id

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-db-subnet-group"
  })

  depends_on = [aws_subnet.database]
}

# ==============================================================================
# ElastiCache Subnet Group
# ==============================================================================

resource "aws_elasticache_subnet_group" "main" {
  count = var.elasticache_enabled ? 1 : 0

  name       = "${local.name_prefix}-cache-subnet-group"
  subnet_ids = aws_subnet.database[*].id

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-cache-subnet-group"
  })

  depends_on = [aws_subnet.database]
}

# ==============================================================================
# Elastic IPs for NAT Gateways
# ==============================================================================

resource "aws_eip" "nat" {
  count = var.enable_nat_gateway && !var.single_nat_gateway ? length(var.public_subnet_cidrs) : var.enable_nat_gateway ? 1 : 0

  domain = "vpc"

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-nat-eip-${count.index + 1}"
  })

  depends_on = [aws_internet_gateway.main]
}

# ==============================================================================
# NAT Gateways
# ==============================================================================

resource "aws_nat_gateway" "main" {
  count = var.enable_nat_gateway && !var.single_nat_gateway ? length(var.public_subnet_cidrs) : var.enable_nat_gateway ? 1 : 0

  allocation_id = aws_eip.nat[count.index].id
  subnet_id     = aws_subnet.public[count.index].id

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-nat-${count.index + 1}"
  })

  depends_on = [aws_internet_gateway.main, aws_eip.nat]

  lifecycle {
    create_before_destroy = true
  }
}

# ==============================================================================
# Route Tables - Public
# ==============================================================================

resource "aws_route_table" "public" {
  vpc_id = aws_vpc.main.id

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-public-rt"
    Type = "public"
  })

  depends_on = [aws_vpc.main]
}

resource "aws_route" "public_internet_gateway" {
  route_table_id         = aws_route_table.public.id
  destination_cidr_block = "0.0.0.0/0"
  gateway_id             = aws_internet_gateway.main.id

  depends_on = [aws_route_table.public, aws_internet_gateway.main]
}

resource "aws_route_table_association" "public" {
  count = length(aws_subnet.public)

  subnet_id      = aws_subnet.public[count.index].id
  route_table_id = aws_route_table.public.id

  depends_on = [aws_subnet.public, aws_route_table.public]
}

# ==============================================================================
# Route Tables - Private
# ==============================================================================

resource "aws_route_table" "private" {
  count = var.enable_nat_gateway && !var.single_nat_gateway ? length(var.private_subnet_cidrs) : 1

  vpc_id = aws_vpc.main.id

  tags = merge(local.merged_tags, {
    Name = var.single_nat_gateway ? "${local.name_prefix}-private-rt" : "${local.name_prefix}-private-rt-${count.index + 1}"
    Type = "private"
  })

  depends_on = [aws_vpc.main]
}

resource "aws_route" "private_nat_gateway" {
  count = var.enable_nat_gateway && !var.single_nat_gateway ? length(var.private_subnet_cidrs) : var.enable_nat_gateway ? 1 : 0

  route_table_id         = aws_route_table.private[count.index].id
  destination_cidr_block = "0.0.0.0/0"
  nat_gateway_id         = aws_nat_gateway.main[count.index].id

  depends_on = [aws_route_table.private, aws_nat_gateway.main]
}

resource "aws_route_table_association" "private" {
  count = length(aws_subnet.private)

  subnet_id      = aws_subnet.private[count.index].id
  route_table_id = var.single_nat_gateway ? aws_route_table.private[0].id : aws_route_table.private[count.index % length(aws_route_table.private)].id

  depends_on = [aws_subnet.private, aws_route_table.private]
}

# ==============================================================================
# Route Tables - Database
# ==============================================================================

resource "aws_route_table" "database" {
  count = var.enable_nat_gateway && !var.single_nat_gateway ? length(var.database_subnet_cidrs) : 1

  vpc_id = aws_vpc.main.id

  tags = merge(local.merged_tags, {
    Name = var.single_nat_gateway ? "${local.name_prefix}-database-rt" : "${local.name_prefix}-database-rt-${count.index + 1}"
    Type = "database"
  })

  depends_on = [aws_vpc.main]
}

resource "aws_route" "database_nat_gateway" {
  count = var.enable_nat_gateway && !var.single_nat_gateway ? length(var.database_subnet_cidrs) : var.enable_nat_gateway ? 1 : 0

  route_table_id         = aws_route_table.database[count.index].id
  destination_cidr_block = "0.0.0.0/0"
  nat_gateway_id         = aws_nat_gateway.main[count.index].id

  depends_on = [aws_route_table.database, aws_nat_gateway.main]
}

resource "aws_route_table_association" "database" {
  count = length(aws_subnet.database)

  subnet_id      = aws_subnet.database[count.index].id
  route_table_id = var.single_nat_gateway ? aws_route_table.database[0].id : aws_route_table.database[count.index % length(aws_route_table.database)].id

  depends_on = [aws_subnet.database, aws_route_table.database]
}

# ==============================================================================
# VPC Flow Logs
# ==============================================================================

resource "aws_flow_log" "main" {
  count = var.enable_flow_logs ? 1 : 0

  iam_role_arn    = aws_iam_role.flow_logs[0].arn
  log_destination = aws_cloudwatch_log_group.flow_logs[0].arn
  traffic_type    = "ALL"
  vpc_id          = aws_vpc.main.id

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-flow-logs"
  })

  depends_on = [aws_vpc.main, aws_iam_role.flow_logs, aws_cloudwatch_log_group.flow_logs]
}

resource "aws_cloudwatch_log_group" "flow_logs" {
  count = var.enable_flow_logs ? 1 : 0

  name              = "/aws/vpc/${local.name_prefix}/flow-logs"
  retention_in_days = var.flow_logs_retention_days

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-flow-logs-lg"
  })
}

resource "aws_iam_role" "flow_logs" {
  count = var.enable_flow_logs ? 1 : 0

  name = "${local.name_prefix}-flow-logs-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Principal = {
          Service = "vpc-flow-logs.amazonaws.com"
        }
      }
    ]
  })

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-flow-logs-role"
  })
}

resource "aws_iam_role_policy" "flow_logs" {
  count = var.enable_flow_logs ? 1 : 0

  name = "${local.name_prefix}-flow-logs-policy"
  role = aws_iam_role.flow_logs[0].id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = [
          "logs:CreateLogGroup",
          "logs:CreateLogStream",
          "logs:PutLogEvents",
          "logs:DescribeLogGroups",
          "logs:DescribeLogStreams"
        ]
        Effect   = "Allow"
        Resource = "*"
      }
    ]
  })
}

# ==============================================================================
# VPC Endpoints - S3
# ==============================================================================

resource "aws_vpc_endpoint" "s3" {
  vpc_id            = aws_vpc.main.id
  service_name      = "com.amazonaws.${data.aws_region.current.name}.s3"
  vpc_endpoint_type = "Gateway"

  route_table_ids = concat(
    [aws_route_table.public.id],
    aws_route_table.private[*].id,
    aws_route_table.database[*].id
  )

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-s3-endpoint"
  })

  depends_on = [aws_vpc.main, aws_route_table.public, aws_route_table.private, aws_route_table.database]
}

# ==============================================================================
# VPC Endpoints - DynamoDB
# ==============================================================================

resource "aws_vpc_endpoint" "dynamodb" {
  vpc_id            = aws_vpc.main.id
  service_name      = "com.amazonaws.${data.aws_region.current.name}.dynamodb"
  vpc_endpoint_type = "Gateway"

  route_table_ids = concat(
    [aws_route_table.public.id],
    aws_route_table.private[*].id,
    aws_route_table.database[*].id
  )

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-dynamodb-endpoint"
  })

  depends_on = [aws_vpc.main, aws_route_table.public, aws_route_table.private, aws_route_table.database]
}

# ==============================================================================
# VPC Endpoints - Interface (ECR, Secrets Manager, etc.)
# ==============================================================================

resource "aws_vpc_endpoint" "interface" {
  for_each = {
    ecr_api            = "com.amazonaws.${data.aws_region.current.name}.ecr.api"
    ecr_dkr            = "com.amazonaws.${data.aws_region.current.name}.ecr.dkr"
    secretsmanager     = "com.amazonaws.${data.aws_region.current.name}.secretsmanager"
    kms                = "com.amazonaws.${data.aws_region.current.name}.kms"
    logs               = "com.amazonaws.${data.aws_region.current.name}.logs"
    monitoring         = "com.amazonaws.${data.aws_region.current.name}.monitoring"
    elasticloadbalancing = "com.amazonaws.${data.aws_region.current.name}.elasticloadbalancing"
  }

  vpc_id              = aws_vpc.main.id
  service_name        = each.value
  vpc_endpoint_type   = "Interface"
  private_dns_enabled = true

  subnet_ids = aws_subnet.private[*].id

  security_group_ids = [aws_security_group.vpc_endpoints.id]

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-${each.key}-endpoint"
  })

  depends_on = [aws_vpc.main, aws_subnet.private, aws_security_group.vpc_endpoints]
}

# ==============================================================================
# Security Group - VPC Endpoints
# ==============================================================================

resource "aws_security_group" "vpc_endpoints" {
  name        = "${local.name_prefix}-vpc-endpoints-sg"
  description = "Security group for VPC endpoints"
  vpc_id      = aws_vpc.main.id

  ingress {
    description = "HTTPS from VPC"
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = [var.vpc_cidr]
  }

  egress {
    description = "All outbound traffic"
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-vpc-endpoints-sg"
  })

  depends_on = [aws_vpc.main]
}

# ==============================================================================
# Network ACL - Public Subnets
# ==============================================================================

resource "aws_network_acl" "public" {
  vpc_id     = aws_vpc.main.id
  subnet_ids = aws_subnet.public[*].id

  # Allow all inbound traffic
  ingress {
    action     = "allow"
    from_port  = 0
    to_port    = 0
    protocol   = "-1"
    rule_no    = 100
    cidr_block = "0.0.0.0/0"
  }

  # Allow all outbound traffic
  egress {
    action     = "allow"
    from_port  = 0
    to_port    = 0
    protocol   = "-1"
    rule_no    = 100
    cidr_block = "0.0.0.0/0"
  }

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-public-nacl"
  })

  depends_on = [aws_vpc.main, aws_subnet.public]
}

# ==============================================================================
# Network ACL - Private Subnets
# ==============================================================================

resource "aws_network_acl" "private" {
  vpc_id     = aws_vpc.main.id
  subnet_ids = aws_subnet.private[*].id

  # Allow all inbound traffic from VPC
  ingress {
    action     = "allow"
    from_port  = 0
    to_port    = 0
    protocol   = "-1"
    rule_no    = 100
    cidr_block = var.vpc_cidr
  }

  # Allow all outbound traffic
  egress {
    action     = "allow"
    from_port  = 0
    to_port    = 0
    protocol   = "-1"
    rule_no    = 100
    cidr_block = "0.0.0.0/0"
  }

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-private-nacl"
  })

  depends_on = [aws_vpc.main, aws_subnet.private]
}

# ==============================================================================
# Network ACL - Database Subnets
# ==============================================================================

resource "aws_network_acl" "database" {
  vpc_id     = aws_vpc.main.id
  subnet_ids = aws_subnet.database[*].id

  # Allow all inbound traffic from VPC
  ingress {
    action     = "allow"
    from_port  = 0
    to_port    = 0
    protocol   = "-1"
    rule_no    = 100
    cidr_block = var.vpc_cidr
  }

  # Allow all outbound traffic
  egress {
    action     = "allow"
    from_port  = 0
    to_port    = 0
    protocol   = "-1"
    rule_no    = 100
    cidr_block = "0.0.0.0/0"
  }

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-database-nacl"
  })

  depends_on = [aws_vpc.main, aws_subnet.database]
}

# ==============================================================================
# VPN Gateway (Optional)
# ==============================================================================

resource "aws_vpn_gateway" "main" {
  count = var.enable_vpn_gateway ? 1 : 0

  vpc_id = aws_vpc.main.id

  tags = merge(local.merged_tags, {
    Name = "${local.name_prefix}-vpn-gateway"
  })

  depends_on = [aws_vpc.main]
}

# ==============================================================================
# VPC Peering Connection (Optional - for cross-account/cross-region)
# ==============================================================================

# resource "aws_vpc_peering_connection" "main" {
#   count = var.enable_vpc_peering ? 1 : 0
#
#   vpc_id        = aws_vpc.main.id
#   peer_vpc_id   = var.peer_vpc_id
#   peer_region   = var.peer_region
#   auto_accept   = var.auto_accept_peering
#
#   tags = merge(local.merged_tags, {
#     Name = "${local.name_prefix}-peering"
#   })
#
#   depends_on = [aws_vpc.main]
# }
