locals {
  use_rds = var.external_database_url_secret_arn == ""
}

resource "random_password" "db" {
  count   = local.use_rds ? 1 : 0
  length  = 32
  special = false
}

resource "aws_db_subnet_group" "main" {
  count      = local.use_rds ? 1 : 0
  name       = local.name
  subnet_ids = aws_subnet.private[*].id
}

resource "aws_security_group" "db" {
  name   = "${local.name}-db"
  vpc_id = aws_vpc.main.id
  ingress {
    from_port       = 5432
    to_port         = 5432
    protocol        = "tcp"
    security_groups = [aws_security_group.tasks.id]
  }
}

resource "aws_db_instance" "telemetry" {
  count                        = local.use_rds ? 1 : 0
  identifier                   = "${local.name}-telemetry"
  engine                       = "postgres"
  engine_version               = "16"
  instance_class               = var.db_instance_class
  allocated_storage            = 50
  max_allocated_storage        = 500
  storage_encrypted            = true
  db_name                      = "telemetry"
  username                     = "fleet"
  password                     = random_password.db[0].result
  db_subnet_group_name         = aws_db_subnet_group.main[0].name
  vpc_security_group_ids       = [aws_security_group.db.id]
  backup_retention_period      = 7
  multi_az                     = var.environment == "production"
  deletion_protection          = var.environment == "production"
  skip_final_snapshot          = var.environment != "production"
  final_snapshot_identifier    = "${local.name}-telemetry-final"
  performance_insights_enabled = true
}

resource "aws_secretsmanager_secret" "database_url" {
  count = local.use_rds ? 1 : 0
  name  = "${local.name}/database-url"
}

resource "aws_secretsmanager_secret_version" "database_url" {
  count     = local.use_rds ? 1 : 0
  secret_id = aws_secretsmanager_secret.database_url[0].id
  secret_string = format("postgres://fleet:%s@%s/telemetry?sslmode=require",
  random_password.db[0].result, aws_db_instance.telemetry[0].endpoint)
}

locals {
  database_url_secret_arn = local.use_rds ? aws_secretsmanager_secret.database_url[0].arn : var.external_database_url_secret_arn
}
