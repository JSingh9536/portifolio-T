# API keys are created empty-valued here and populated out of band
# (aws secretsmanager put-secret-value) so they never land in state.
resource "aws_secretsmanager_secret" "operator_keys" { name = "${local.name}/operator-keys" }
resource "aws_secretsmanager_secret" "robot_key" { name = "${local.name}/robot-key" }
# The dashboard's server-side proxy key; must also appear in operator-keys.
resource "aws_secretsmanager_secret" "dashboard_operator_key" { name = "${local.name}/dashboard-operator-key" }
