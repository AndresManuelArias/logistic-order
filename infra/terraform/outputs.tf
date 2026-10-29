output "alb_dns_name" {
  description = "DNS público del Application Load Balancer."
  value       = aws_lb.main.dns_name
}

output "api_url" {
  description = "URL base de la API (HTTPS si hay certificado)."
  value       = var.certificate_arn == "" ? "http://${aws_lb.main.dns_name}" : "https://${aws_lb.main.dns_name}"
}

output "ecs_cluster_name" {
  description = "Cluster ECS donde corre el backend."
  value       = aws_ecs_cluster.main.name
}

output "ecs_service_name" {
  description = "Servicio ECS del backend."
  value       = aws_ecs_service.backend.name
}

output "rds_endpoint" {
  description = "Endpoint privado de RDS (sin credenciales)."
  value       = aws_db_instance.main.endpoint
}

output "log_group_name" {
  description = "Log group de CloudWatch del backend."
  value       = aws_cloudwatch_log_group.backend.name
}

output "secret_arns" {
  description = "ARNs de Secrets Manager consumidos por la tarea."
  value = {
    database_url = aws_secretsmanager_secret.database_url.arn
    jwt_secret   = aws_secretsmanager_secret.jwt_secret.arn
    ai_api_key   = aws_secretsmanager_secret.ai_api_key.arn
  }
}
