# Infraestructura AWS (F6)

Terraform mínimo para desplegar el backend de Tiendas-Track en AWS:

- **Red**: VPC con 2 subredes públicas (ALB) y 2 privadas (ECS/RDS), IGW y NAT.
- **Base de datos**: RDS PostgreSQL 16 cifrado, backups de 7 días, sin acceso público.
- **Servicio**: ECS Fargate (2 tareas por defecto) detrás de un ALB con health
  check `/healthz` y circuit breaker de despliegue.
- **Secretos**: Secrets Manager para `DATABASE_URL`, `JWT_SECRET` y `AI_API_KEY`.
- **Observabilidad**: CloudWatch Logs con retención 7/30 días y Container Insights.
- **IAM**: execution role con la política gestionada de ECS + lectura de sus tres
  secretos; task role sin permisos adicionales.

## Limitación importante: TimescaleDB

**Amazon RDS no soporta la extensión TimescaleDB.** La aplicación usa
hypertables, `time_bucket` y agregados continuos sobre `telemetry.*`, por lo que
este stack es un despliegue de referencia del plano de aplicación, no un
entorno de producción completo.

Rutas válidas para producción:

1. **Timescale Cloud** (recomendada): aprovisionar el servicio allí y pasar su
   cadena en `database_url`; Terraform seguiría gestionando red, ECS, ALB y
   secretos. RDS puede usarse para datos de aplicación o retirarse.
2. **PostgreSQL autogestionado en EC2** con `timescale/timescaledb-ha:pg16` y
   PostGIS (misma imagen que `docker-compose.yml`), con backups y parcheo
   propios.

Mientras no se resuelva, el backend fallará al aplicar las migraciones de
telemetría porque `CREATE EXTENSION timescaledb` no existe en RDS. No ocultar
este error: es la señal de que falta el plano temporal.

## Uso

```bash
cd infra/terraform
terraform init
terraform fmt -check -recursive
terraform validate

# Plan real (requiere credenciales AWS y variables sensibles)
export TF_VAR_db_password='...'
export TF_VAR_jwt_secret="$(openssl rand -hex 32)"
terraform plan -out=tfplan

# Validación offline sin credenciales (CI)
terraform plan -refresh=false -input=false \
  -var offline_plan=true \
  -var db_password=offline-dummy \
  -var jwt_secret=offline-dummy-secret-at-least-32-chars \
  -var ai_api_key=offline-dummy
```

`offline_plan=true` desactiva la validación de credenciales/región del provider
AWS. **Solo sirve para validar el grafo de recursos**; un `apply` con ese modo
falla y nunca debe ejecutarse.

## Variables y secretos

| Variable | Obligatoria | Descripción |
|---|---|---|
| `db_password` | sí (apply) | Contraseña maestra de RDS. `sensitive`. |
| `jwt_secret` | sí (apply) | HS256 >= 32 caracteres. `sensitive`. |
| `ai_api_key` | no | Sin clave el agente responde `503 AI_NOT_CONFIGURED`. |
| `backend_image` | sí | Imagen ECR publicada por el pipeline. |
| `certificate_arn` | no | Si se define, crea listener HTTPS y redirige HTTP→HTTPS. |
| `database_url` | no | Sobrescribe la URL construida (p. ej. Timescale Cloud). |

Pasa secretos con `TF_VAR_*` desde el gestor de secretos del CI, nunca en
`terraform.tfvars` versionado. El estado contiene la contraseña de RDS y la
`DATABASE_URL`; usa backend S3 cifrado y bloqueo (ver `versions.tf`) y restringe
el acceso al bucket.

## Coste aproximado (us-east-1, dev)

| Recurso | Estimado/mes |
|---|---|
| ALB | ~16 USD |
| NAT Gateway + EIP | ~33 USD |
| ECS Fargate 2 × (0.5 vCPU, 1 GiB) | ~15 USD |
| RDS `db.t4g.micro` + 20 GiB gp3 | ~15 USD |
| Secrets Manager (3 secretos) | ~1.2 USD |
| CloudWatch Logs | ~1–5 USD |
| **Total** | **~80–90 USD** |

Para abaratar: un solo NAT ya está aplicado; bajar `desired_count` a 1 en dev
(no recomendado en prod) o usar RDS bajo demanda solo en horas de trabajo.

## Operación

- **Despliegue**: el pipeline construye y publica la imagen en ECR, actualiza
  `backend_image` y ejecuta `terraform apply`. El circuit breaker de ECS revierte
  si las tareas no llegan a healthy.
- **Rollback**: `terraform apply` con la imagen anterior o `aws ecs
  update-service --task-definition <revisión>`.
- **Backups**: RDS retiene 7 días. Timescale Cloud/EC2 requiere política propia.
- **Rotación**: actualizar el secreto y forzar un nuevo despliegue de ECS.
- **Migraciones**: el backend las aplica al arrancar (`MIGRATE_ON_STARTUP=true`).
  En producción conviene ejecutarlas como tarea one-off antes del despliegue.

## Estado actual (limitaciones del entorno)

- Validado: `terraform fmt -check`, `terraform init -backend=false`,
  `terraform validate` y `terraform plan` con `offline_plan=true` (36 recursos).
- No validado: `apply` real, porque el entorno no tiene credenciales AWS.
- No incluido: certificado ACM, Route53, WAF, autoescalado y alarmas. Son
  extensiones razonables para producción, fuera del alcance mínimo de F6.
