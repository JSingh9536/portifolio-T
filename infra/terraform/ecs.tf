locals {
  services = {
    telemetry-ingest = {
      port          = 8080
      cpu           = 512
      memory        = 1024
      desired_count = 2
      path_patterns = ["/v1/telemetry", "/v1/sites/*", "/v1/robots/*/series", "/v1/stream"]
      priority      = 10
      health_path   = "/healthz"
      environment   = { CORS_ORIGIN = "https://${aws_lb.main.dns_name}" }
      secrets       = { DATABASE_URL = local.database_url_secret_arn }
    }
    mission-control = {
      port = 3001
      cpu  = 256
      # In-memory command queue: exactly one replica until it moves to Postgres.
      memory        = 512
      desired_count = 1
      path_patterns = ["/v1/commands*", "/v1/robots", "/v1/robots/*/commands*", "/v1/audit"]
      priority      = 20
      health_path   = "/healthz"
      environment   = { CORS_ORIGIN = "https://${aws_lb.main.dns_name}" }
      secrets = {
        OPERATOR_KEYS = aws_secretsmanager_secret.operator_keys.arn
        ROBOT_KEY     = aws_secretsmanager_secret.robot_key.arn
      }
    }
    site-dashboard = {
      port          = 3000
      cpu           = 256
      memory        = 512
      desired_count = 2
      path_patterns = ["/*"]
      priority      = 100
      health_path   = "/"
      environment = {
        NUXT_PUBLIC_INGEST_URL = "https://${aws_lb.main.dns_name}"
        # Server-side proxy target; reaches mission-control through the ALB.
        NUXT_CONTROL_URL = "https://${aws_lb.main.dns_name}"
      }
      secrets = { NUXT_OPERATOR_KEY = aws_secretsmanager_secret.dashboard_operator_key.arn }
    }
  }
}

resource "aws_ecr_repository" "svc" {
  for_each             = local.services
  name                 = "fleet/${each.key}"
  image_tag_mutability = "IMMUTABLE"
  image_scanning_configuration { scan_on_push = true }
}

resource "aws_ecr_lifecycle_policy" "svc" {
  for_each   = aws_ecr_repository.svc
  repository = each.value.name
  policy = jsonencode({
    rules = [{
      rulePriority = 1
      description  = "keep last 30 images"
      selection    = { tagStatus = "any", countType = "imageCountMoreThan", countNumber = 30 }
      action       = { type = "expire" }
    }]
  })
}

resource "aws_ecs_cluster" "main" {
  name = local.name
  setting {
    name  = "containerInsights"
    value = "enabled"
  }
}

resource "aws_cloudwatch_log_group" "svc" {
  for_each          = local.services
  name              = "/ecs/${local.name}/${each.key}"
  retention_in_days = 30
}

resource "aws_security_group" "tasks" {
  name   = "${local.name}-tasks"
  vpc_id = aws_vpc.main.id
  ingress {
    from_port       = 0
    to_port         = 65535
    protocol        = "tcp"
    security_groups = [aws_security_group.alb.id]
  }
  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_ecs_task_definition" "svc" {
  for_each                 = local.services
  family                   = "${local.name}-${each.key}"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = each.value.cpu
  memory                   = each.value.memory
  execution_role_arn       = aws_iam_role.execution.arn
  task_role_arn            = aws_iam_role.task.arn
  runtime_platform {
    cpu_architecture        = "ARM64"
    operating_system_family = "LINUX"
  }
  container_definitions = jsonencode([{
    name         = each.key
    image        = "${aws_ecr_repository.svc[each.key].repository_url}:${var.image_tag}"
    essential    = true
    portMappings = [{ containerPort = each.value.port }]
    environment = concat(
      [for k, v in each.value.environment : { name = k, value = v }],
      [{ name = "PORT", value = tostring(each.value.port) }, { name = "ADDR", value = ":${each.value.port}" }],
    )
    secrets = [for k, arn in each.value.secrets : { name = k, valueFrom = arn }]
    logConfiguration = {
      logDriver = "awslogs"
      options = {
        awslogs-group         = aws_cloudwatch_log_group.svc[each.key].name
        awslogs-region        = var.region
        awslogs-stream-prefix = each.key
      }
    }
  }])
}

resource "aws_ecs_service" "svc" {
  for_each        = local.services
  name            = each.key
  cluster         = aws_ecs_cluster.main.id
  task_definition = aws_ecs_task_definition.svc[each.key].arn
  desired_count   = each.value.desired_count
  launch_type     = "FARGATE"
  # mission-control must never run two copies, even mid-deploy.
  deployment_maximum_percent         = each.key == "mission-control" ? 100 : 200
  deployment_minimum_healthy_percent = each.key == "mission-control" ? 0 : 50
  deployment_circuit_breaker {
    enable   = true
    rollback = true
  }
  network_configuration {
    subnets         = aws_subnet.private[*].id
    security_groups = [aws_security_group.tasks.id]
  }
  load_balancer {
    target_group_arn = aws_lb_target_group.svc[each.key].arn
    container_name   = each.key
    container_port   = each.value.port
  }
  depends_on = [aws_lb_listener.https]
}
