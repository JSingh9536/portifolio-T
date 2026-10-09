variable "region" {
  type    = string
  default = "us-west-2"
}

variable "environment" {
  type    = string
  default = "staging"
  validation {
    condition     = contains(["staging", "production"], var.environment)
    error_message = "environment must be staging or production"
  }
}

variable "vpc_cidr" {
  type    = string
  default = "10.40.0.0/16"
}

variable "image_tag" {
  description = "Container tag (git SHA) deployed for every service"
  type        = string
}

variable "db_instance_class" {
  type    = string
  default = "db.t4g.medium"
}

variable "external_database_url_secret_arn" {
  description = "Optional: Secrets Manager ARN holding a Timescale Cloud DSN. When set, RDS is not created and ingest uses TimescaleDB hypertables; otherwise it runs on RDS Postgres in plain-table mode."
  type        = string
  default     = ""
}

variable "certificate_arn" {
  description = "ACM certificate for the ALB HTTPS listener"
  type        = string
}

variable "dashboard_cidrs" {
  description = "CIDRs allowed to reach the ALB (office / VPN egress)"
  type        = list(string)
}

variable "robot_cidrs" {
  description = "Egress CIDRs of the field robots' cellular APN (private APN recommended)"
  type        = list(string)
  default     = []
}
