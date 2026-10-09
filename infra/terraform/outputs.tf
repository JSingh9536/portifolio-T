output "url" { value = "https://${aws_lb.main.dns_name}" }
output "ecr_repositories" { value = { for k, r in aws_ecr_repository.svc : k => r.repository_url } }
output "ci_role_arn" { value = aws_iam_role.ci.arn }
output "database_mode" { value = local.use_rds ? "rds-postgres (plain tables)" : "external (TimescaleDB)" }
