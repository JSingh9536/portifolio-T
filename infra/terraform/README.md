# infra/terraform (AWS)

This deploys the stack to AWS:
- A VPC with 2 AZs, public and private subnets, and a NAT gateway.
- ECS Fargate (arm64), with one service per app.
- An HTTPS ALB with path routing.
- RDS Postgres 16.
- Secrets Manager and ECR.
- A GitHub OIDC deploy role.

```bash
terraform init -backend-config="bucket=<state-bucket>" -backend-config="key=fleet/staging.tfstate" -backend-config="region=us-west-2"
terraform plan -var-file=staging.tfvars
```

Notes:
- **TimescaleDB.** RDS doesn't offer the TimescaleDB extension, so ingest runs there in plain-table mode. To use Timescale Cloud and get hypertables, compression and retention, set `external_database_url_secret_arn`.
- **mission-control** is limited to one task (`maximum_percent = 100`) until its queue moves to Postgres.
- **API keys.** The secret containers are created by Terraform, but their values are set out of band, so they never land in state.
