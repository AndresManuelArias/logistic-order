terraform {
  required_version = ">= 1.6.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.60"
    }
  }

  # Estado remoto recomendado en producción (descomentar y crear el bucket):
  # backend "s3" {
  #   bucket         = "tiendas-track-tfstate"
  #   key            = "f6/terraform.tfstate"
  #   region         = "us-east-1"
  #   dynamodb_table = "tiendas-track-tflock"
  #   encrypt        = true
  # }
}
