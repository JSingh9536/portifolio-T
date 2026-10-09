resource "aws_security_group" "alb" {
  name   = "${local.name}-alb"
  vpc_id = aws_vpc.main.id
  ingress {
    description = "operators, field robots, and the dashboard's own server-side proxy (via NAT)"
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = concat(var.dashboard_cidrs, var.robot_cidrs, ["${aws_eip.nat.public_ip}/32"])
  }
  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = [var.vpc_cidr]
  }
}

resource "aws_lb" "main" {
  name               = local.name
  load_balancer_type = "application"
  subnets            = aws_subnet.public[*].id
  security_groups    = [aws_security_group.alb.id]
  # SSE connections idle between samples; keep them open past the 15 s keepalive.
  idle_timeout = 120
}

resource "aws_lb_target_group" "svc" {
  for_each             = local.services
  name                 = substr("${local.name}-${each.key}", 0, 32)
  port                 = each.value.port
  protocol             = "HTTP"
  target_type          = "ip"
  vpc_id               = aws_vpc.main.id
  deregistration_delay = 15
  health_check {
    path    = each.value.health_path
    matcher = "200"
  }
}

resource "aws_lb_listener" "https" {
  load_balancer_arn = aws_lb.main.arn
  port              = 443
  protocol          = "HTTPS"
  ssl_policy        = "ELBSecurityPolicy-TLS13-1-2-2021-06"
  certificate_arn   = var.certificate_arn
  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.svc["site-dashboard"].arn
  }
}

resource "aws_lb_listener_rule" "svc" {
  for_each     = { for k, v in local.services : k => v if k != "site-dashboard" }
  listener_arn = aws_lb_listener.https.arn
  priority     = each.value.priority
  action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.svc[each.key].arn
  }
  condition {
    path_pattern { values = each.value.path_patterns }
  }
}
