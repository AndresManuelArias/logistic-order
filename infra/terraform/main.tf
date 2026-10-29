###############################################################################
# Red: VPC con 2 subredes públicas (ALB) y 2 privadas (ECS/RDS).
###############################################################################

locals {
  name = "${var.project}-${var.environment}"
  azs  = [for i in range(var.az_count) : "${var.aws_region}${element(["a", "b", "c"], i)}"]

  database_url_value = var.database_url != "" ? var.database_url : format(
    "postgres://%s:%s@%s:5432/%s?sslmode=require",
    var.db_username, var.db_password, aws_db_instance.main.address, var.db_name,
  )

  container_environment = [
    { name = "APP_ENV", value = var.environment },
    { name = "HTTP_ADDR", value = ":8080" },
    { name = "LOG_FORMAT", value = "json" },
    { name = "LOG_LEVEL", value = "info" },
    { name = "MIGRATE_ON_STARTUP", value = "true" },
    { name = "WS_ALLOWED_ORIGINS", value = var.ws_allowed_origins },
    { name = "RATE_LIMIT_ENABLED", value = "true" },
    { name = "RATE_LIMIT_TRUST_PROXY", value = "true" },
    { name = "RATE_LIMIT_LOGIN_PER_MIN", value = tostring(var.rate_limit_login_per_min) },
    { name = "RATE_LIMIT_TELEMETRY_PER_MIN", value = tostring(var.rate_limit_telemetry_per_min) },
    { name = "RATE_LIMIT_AI_PER_MIN", value = tostring(var.rate_limit_ai_per_min) },
  ]
}

resource "aws_vpc" "main" {
  cidr_block           = var.vpc_cidr
  enable_dns_support   = true
  enable_dns_hostnames = true

  tags = { Name = "${local.name}-vpc" }
}

resource "aws_internet_gateway" "main" {
  vpc_id = aws_vpc.main.id
  tags   = { Name = "${local.name}-igw" }
}

resource "aws_subnet" "public" {
  count                   = var.az_count
  vpc_id                  = aws_vpc.main.id
  cidr_block              = cidrsubnet(var.vpc_cidr, 8, count.index)
  availability_zone       = local.azs[count.index]
  map_public_ip_on_launch = true

  tags = { Name = "${local.name}-public-${count.index}", Tier = "public" }
}

resource "aws_subnet" "private" {
  count             = var.az_count
  vpc_id            = aws_vpc.main.id
  cidr_block        = cidrsubnet(var.vpc_cidr, 8, count.index + 10)
  availability_zone = local.azs[count.index]

  tags = { Name = "${local.name}-private-${count.index}", Tier = "private" }
}

resource "aws_route_table" "public" {
  vpc_id = aws_vpc.main.id

  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.main.id
  }

  tags = { Name = "${local.name}-public" }
}

resource "aws_route_table_association" "public" {
  count          = var.az_count
  subnet_id      = aws_subnet.public[count.index].id
  route_table_id = aws_route_table.public.id
}

# Una sola NAT para abaratar el entorno de referencia. En producción: una por AZ.
resource "aws_eip" "nat" {
  domain = "vpc"
  tags   = { Name = "${local.name}-nat" }
}

resource "aws_nat_gateway" "main" {
  allocation_id = aws_eip.nat.id
  subnet_id     = aws_subnet.public[0].id
  tags          = { Name = "${local.name}-nat" }

  depends_on = [aws_internet_gateway.main]
}

resource "aws_route_table" "private" {
  vpc_id = aws_vpc.main.id

  route {
    cidr_block     = "0.0.0.0/0"
    nat_gateway_id = aws_nat_gateway.main.id
  }

  tags = { Name = "${local.name}-private" }
}

resource "aws_route_table_association" "private" {
  count          = var.az_count
  subnet_id      = aws_subnet.private[count.index].id
  route_table_id = aws_route_table.private.id
}

###############################################################################
# Security groups: solo el ALB expone tráfico; ECS y RDS quedan privados.
###############################################################################

resource "aws_security_group" "alb" {
  name        = "${local.name}-alb"
  description = "Ingreso HTTP/HTTPS desde Internet al ALB"
  vpc_id      = aws_vpc.main.id

  ingress {
    description = "HTTP"
    from_port   = 80
    to_port     = 80
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  ingress {
    description = "HTTPS"
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = { Name = "${local.name}-alb" }
}

resource "aws_security_group" "ecs" {
  name        = "${local.name}-ecs"
  description = "Backend Fargate: solo recibe del ALB"
  vpc_id      = aws_vpc.main.id

  ingress {
    description     = "API desde ALB"
    from_port       = 8080
    to_port         = 8080
    protocol        = "tcp"
    security_groups = [aws_security_group.alb.id]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = { Name = "${local.name}-ecs" }
}

resource "aws_security_group" "rds" {
  name        = "${local.name}-rds"
  description = "PostgreSQL: solo desde las tareas ECS"
  vpc_id      = aws_vpc.main.id

  ingress {
    description     = "PostgreSQL desde ECS"
    from_port       = 5432
    to_port         = 5432
    protocol        = "tcp"
    security_groups = [aws_security_group.ecs.id]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = { Name = "${local.name}-rds" }
}

###############################################################################
# Base de datos: RDS PostgreSQL 16 (datos de aplicación).
#
# LIMITACIÓN CONOCIDA: RDS no soporta la extensión TimescaleDB. El plano de
# series temporales (telemetría) requiere Timescale Cloud o PostgreSQL
# autogestionado con timescale/timescaledb-ha. Ver infra/README.md.
###############################################################################

resource "aws_db_subnet_group" "main" {
  name       = "${local.name}-db"
  subnet_ids = aws_subnet.private[*].id
  tags       = { Name = "${local.name}-db" }
}

resource "aws_db_instance" "main" {
  identifier = "${local.name}-postgres"

  engine         = "postgres"
  engine_version = "16"
  instance_class = var.db_instance_class

  db_name  = var.db_name
  username = var.db_username
  password = var.db_password
  port     = 5432

  allocated_storage     = 20
  max_allocated_storage = 100
  storage_type          = "gp3"
  storage_encrypted     = true

  db_subnet_group_name   = aws_db_subnet_group.main.name
  vpc_security_group_ids = [aws_security_group.rds.id]
  publicly_accessible    = false
  multi_az               = false

  backup_retention_period      = 7
  auto_minor_version_upgrade   = true
  deletion_protection          = var.environment == "prod"
  skip_final_snapshot          = var.environment != "prod"
  final_snapshot_identifier    = var.environment == "prod" ? "${local.name}-final" : null
  performance_insights_enabled = true

  tags = { Name = "${local.name}-postgres" }
}

###############################################################################
# Secretos: DATABASE_URL, JWT_SECRET y AI_API_KEY salen de Secrets Manager.
###############################################################################

resource "aws_secretsmanager_secret" "database_url" {
  name                    = "${local.name}/database_url"
  recovery_window_in_days = 0
  tags                    = { Name = "${local.name}-database-url" }
}

resource "aws_secretsmanager_secret_version" "database_url" {
  secret_id     = aws_secretsmanager_secret.database_url.id
  secret_string = local.database_url_value
}

resource "aws_secretsmanager_secret" "jwt_secret" {
  name                    = "${local.name}/jwt_secret"
  recovery_window_in_days = 0
  tags                    = { Name = "${local.name}-jwt-secret" }
}

resource "aws_secretsmanager_secret_version" "jwt_secret" {
  secret_id     = aws_secretsmanager_secret.jwt_secret.id
  secret_string = var.jwt_secret
}

resource "aws_secretsmanager_secret" "ai_api_key" {
  name                    = "${local.name}/ai_api_key"
  recovery_window_in_days = 0
  tags                    = { Name = "${local.name}-ai-api-key" }
}

resource "aws_secretsmanager_secret_version" "ai_api_key" {
  secret_id     = aws_secretsmanager_secret.ai_api_key.id
  secret_string = var.ai_api_key
}

###############################################################################
# Observabilidad: logs estructurados del backend en CloudWatch.
###############################################################################

resource "aws_cloudwatch_log_group" "backend" {
  name              = "/ecs/${local.name}"
  retention_in_days = var.environment == "prod" ? 30 : 7
  tags              = { Name = "${local.name}-logs" }
}

###############################################################################
# IAM: execution role (pull de imagen, logs y secretos) + task role vacío.
###############################################################################

resource "aws_iam_role" "execution" {
  name = "${local.name}-ecs-execution"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "ecs-tasks.amazonaws.com" }
      Action    = "sts:AssumeRole"
    }]
  })

  tags = { Name = "${local.name}-ecs-execution" }
}

resource "aws_iam_role_policy_attachment" "execution_managed" {
  role       = aws_iam_role.execution.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

resource "aws_iam_role_policy" "execution_secrets" {
  name = "${local.name}-read-secrets"
  role = aws_iam_role.execution.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Action = ["secretsmanager:GetSecretValue"]
      Resource = [
        aws_secretsmanager_secret.database_url.arn,
        aws_secretsmanager_secret.jwt_secret.arn,
        aws_secretsmanager_secret.ai_api_key.arn,
      ]
    }]
  })
}

resource "aws_iam_role" "task" {
  name = "${local.name}-ecs-task"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "ecs-tasks.amazonaws.com" }
      Action    = "sts:AssumeRole"
    }]
  })

  tags = { Name = "${local.name}-ecs-task" }
}

###############################################################################
# Cómputo: ECS Fargate + ALB.
###############################################################################

resource "aws_ecs_cluster" "main" {
  name = local.name

  setting {
    name  = "containerInsights"
    value = "enabled"
  }

  tags = { Name = local.name }
}

resource "aws_ecs_task_definition" "backend" {
  family                   = "${local.name}-backend"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = var.backend_cpu
  memory                   = var.backend_memory
  execution_role_arn       = aws_iam_role.execution.arn
  task_role_arn            = aws_iam_role.task.arn

  container_definitions = jsonencode([{
    name      = "backend"
    image     = var.backend_image
    essential = true

    portMappings = [{
      containerPort = 8080
      protocol      = "tcp"
    }]

    environment = local.container_environment

    secrets = [
      { name = "DATABASE_URL", valueFrom = aws_secretsmanager_secret.database_url.arn },
      { name = "JWT_SECRET", valueFrom = aws_secretsmanager_secret.jwt_secret.arn },
      { name = "AI_API_KEY", valueFrom = aws_secretsmanager_secret.ai_api_key.arn },
    ]

    logConfiguration = {
      logDriver = "awslogs"
      options = {
        "awslogs-group"         = aws_cloudwatch_log_group.backend.name
        "awslogs-region"        = var.aws_region
        "awslogs-stream-prefix" = "backend"
      }
    }

    healthCheck = {
      command     = ["CMD-SHELL", "wget -qO- http://127.0.0.1:8080/healthz || exit 1"]
      interval    = 30
      timeout     = 5
      retries     = 3
      startPeriod = 30
    }
  }])

  tags = { Name = "${local.name}-backend" }
}

resource "aws_lb" "main" {
  name               = substr("${local.name}-alb", 0, 32)
  internal           = false
  load_balancer_type = "application"
  security_groups    = [aws_security_group.alb.id]
  subnets            = aws_subnet.public[*].id

  tags = { Name = "${local.name}-alb" }
}

resource "aws_lb_target_group" "backend" {
  name        = substr("${local.name}-tg", 0, 32)
  port        = 8080
  protocol    = "HTTP"
  vpc_id      = aws_vpc.main.id
  target_type = "ip"

  health_check {
    enabled             = true
    path                = "/healthz"
    healthy_threshold   = 2
    unhealthy_threshold = 3
    interval            = 30
    timeout             = 5
    matcher             = "200"
  }

  deregistration_delay = 15
  tags                 = { Name = "${local.name}-tg" }
}

resource "aws_lb_listener" "http" {
  load_balancer_arn = aws_lb.main.arn
  port              = 80
  protocol          = "HTTP"

  default_action {
    type             = var.certificate_arn == "" ? "forward" : "redirect"
    target_group_arn = var.certificate_arn == "" ? aws_lb_target_group.backend.arn : null

    dynamic "redirect" {
      for_each = var.certificate_arn == "" ? [] : [1]
      content {
        port        = "443"
        protocol    = "HTTPS"
        status_code = "HTTP_301"
      }
    }
  }
}

resource "aws_lb_listener" "https" {
  count             = var.certificate_arn == "" ? 0 : 1
  load_balancer_arn = aws_lb.main.arn
  port              = 443
  protocol          = "HTTPS"
  ssl_policy        = "ELBSecurityPolicy-TLS13-1-2-2021-06"
  certificate_arn   = var.certificate_arn

  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.backend.arn
  }
}

resource "aws_ecs_service" "backend" {
  name            = "${local.name}-backend"
  cluster         = aws_ecs_cluster.main.id
  task_definition = aws_ecs_task_definition.backend.arn
  desired_count   = var.desired_count
  launch_type     = "FARGATE"

  network_configuration {
    subnets          = aws_subnet.private[*].id
    security_groups  = [aws_security_group.ecs.id]
    assign_public_ip = false
  }

  load_balancer {
    target_group_arn = aws_lb_target_group.backend.arn
    container_name   = "backend"
    container_port   = 8080
  }

  health_check_grace_period_seconds = 60

  deployment_circuit_breaker {
    enable   = true
    rollback = true
  }

  depends_on = [aws_lb_listener.http, aws_iam_role_policy.execution_secrets]

  tags = { Name = "${local.name}-backend" }
}
