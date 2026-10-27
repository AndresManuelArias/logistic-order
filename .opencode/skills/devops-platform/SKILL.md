---
name: devops-platform
description: "Use para configurar Docker Compose, contenedores, infraestructura como código, CI/CD, Fastlane, GitHub Actions, secretos, observabilidad, backups y despliegues de Tiendas-Track."
---

# Plataforma DevOps

## Objetivo

Mantener entornos reproducibles, seguros y verificables desde el desarrollo local hasta la entrega automatizada.

## Cuando usar

- Crear o modificar Dockerfiles o Docker Compose.
- Proveer infraestructura con Terraform o AWS CDK.
- Configurar CI/CD para backend, portal o aplicación móvil.
- Gestionar entornos, secretos, health checks o observabilidad.
- Diseñar estrategias de despliegue, backup y recuperación.

## Procedimiento

1. Revisar arquitectura, dependencias, proveedores y restricciones confirmadas.
2. Definir variables de entorno, servicios, volúmenes, puertos y dependencias explícitas.
3. Implementar configuraciones mínimas para el entorno solicitado.
4. Aplicar mínimo privilegio en IAM, contenedores, runners y secretos.
5. Evitar secretos reales en archivos, logs, imágenes o salida de comandos.
6. Añadir health checks, logs estructurados y métricas necesarias.
7. Validar sintaxis, arranque local, conectividad y rutas críticas.
8. Documentar comandos, variables requeridas, rollback y responsabilidades.
9. No declarar infraestructura terminada sin ejecutar una prueba disponible.

## Criterios de calidad

- La configuración es reproducible y parametrizada por entorno.
- Los artefactos tienen versiones identificables y permiten rollback cuando corresponda.
- Docker Compose no intenta reemplazar una plataforma cloud sin justificación.
- Las credenciales se consumen desde secretos administrados.
- Los objetivos operativos no se inventan sin requerimiento o acuerdo documentado.

## Entregables esperados

- Archivos de contenedores, IaC o pipelines implementados.
- Validación de sintaxis, arranque o despliegue realizado.
- Variables y secretos requeridos documentados sin valores sensibles.
- Riesgos de costo, disponibilidad o proveedor registrados.
