variable "aws_region" {
  description = "Región AWS del despliegue."
  type        = string
  default     = "us-east-1"
}

variable "project" {
  description = "Prefijo de nombres de recursos."
  type        = string
  default     = "tiendas-track"
}

variable "environment" {
  description = "Entorno lógico (dev, staging, prod)."
  type        = string
  default     = "dev"

  validation {
    condition     = contains(["dev", "staging", "prod"], var.environment)
    error_message = "environment debe ser dev, staging o prod."
  }
}

variable "offline_plan" {
  description = "Valida el plan sin credenciales AWS (solo CI/local; nunca apply)."
  type        = bool
  default     = false
}

variable "vpc_cidr" {
  description = "CIDR de la VPC."
  type        = string
  default     = "10.20.0.0/16"
}

variable "az_count" {
  description = "Número de zonas de disponibilidad (2 recomendado)."
  type        = number
  default     = 2

  validation {
    condition     = var.az_count >= 2 && var.az_count <= 3
    error_message = "az_count debe estar entre 2 y 3."
  }
}

variable "backend_image" {
  description = "Imagen del backend (ECR u otro registry)."
  type        = string
  default     = "public.ecr.aws/nginx/nginx:stable-alpine"
}

variable "backend_cpu" {
  description = "CPU Fargate en unidades de 256."
  type        = number
  default     = 512
}

variable "backend_memory" {
  description = "Memoria Fargate en MiB."
  type        = number
  default     = 1024
}

variable "desired_count" {
  description = "Tareas Fargate deseadas (mínimo 2 para alta disponibilidad)."
  type        = number
  default     = 2
}

variable "db_instance_class" {
  description = "Clase de la instancia RDS."
  type        = string
  default     = "db.t4g.micro"
}

variable "db_name" {
  description = "Base de datos de la aplicación."
  type        = string
  default     = "tiendas"
}

variable "db_username" {
  description = "Usuario maestro de RDS."
  type        = string
  default     = "tiendas"
}

variable "db_password" {
  description = "Contraseña maestra de RDS. Llega por tfvars/CI, nunca por git."
  type        = string
  sensitive   = true
  default     = ""
}

variable "database_url" {
  description = "DATABASE_URL completa. Si está vacía se construye con RDS."
  type        = string
  sensitive   = true
  default     = ""
}

variable "jwt_secret" {
  description = "Secreto HS256 (>= 32 caracteres). Llega por secreto de CI."
  type        = string
  sensitive   = true
  default     = ""
}

variable "ai_api_key" {
  description = "API key del proveedor LLM. Vacía deshabilita el agente (503)."
  type        = string
  sensitive   = true
  default     = ""
}

variable "ws_allowed_origins" {
  description = "Orígenes permitidos para el handshake WebSocket."
  type        = string
  default     = "https://app.tiendas-track.example"
}

variable "certificate_arn" {
  description = "ARN del certificado ACM para HTTPS. Vacío = solo HTTP (no producción)."
  type        = string
  default     = ""
}

variable "rate_limit_login_per_min" {
  description = "Límite de login por IP y minuto (0 desactiva)."
  type        = number
  default     = 10
}

variable "rate_limit_telemetry_per_min" {
  description = "Límite de ingesta por usuario y minuto (0 desactiva)."
  type        = number
  default     = 120
}

variable "rate_limit_ai_per_min" {
  description = "Límite del agente IA por usuario y minuto (0 desactiva)."
  type        = number
  default     = 10
}
