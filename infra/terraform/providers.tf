provider "aws" {
  region = var.aws_region

  # Modo validación offline: permite `terraform plan` sin credenciales AWS.
  # NO usar para apply; solo valida sintaxis, referencias y grafo de recursos.
  skip_credentials_validation = var.offline_plan
  skip_requesting_account_id  = var.offline_plan
  skip_metadata_api_check     = var.offline_plan
  skip_region_validation      = var.offline_plan
  access_key                  = var.offline_plan ? "offline" : null
  secret_key                  = var.offline_plan ? "offline" : null

  default_tags {
    tags = {
      Project     = var.project
      Environment = var.environment
      ManagedBy   = "terraform"
      Phase       = "F6"
    }
  }
}
