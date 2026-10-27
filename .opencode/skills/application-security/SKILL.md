---
name: application-security
description: "Use para revisar y corregir seguridad en Tiendas-Track: JWT, RBAC, secretos, amenazas, privacidad de ubicación, APIs, aplicación móvil, Docker, IaC y CI/CD."
---

# Seguridad de Aplicaciones

## Objetivo

Identificar amenazas y vulnerabilidades confirmables, priorizarlas por riesgo y recomendar correcciones verificables con impacto mínimo.

## Cuando usar

- Revisar autenticación JWT o renovación de sesión.
- Validar permisos y aislamiento por roles.
- Analizar APIs, WebSockets, GPS y sincronización masiva.
- Revisar secretos, infraestructura, dependencias o pipelines.
- Evaluar fuga de datos entre organizaciones o usuarios.

## Procedimiento

1. Identificar activos, actores, datos sensibles, límites de confianza y sistemas externos.
2. Recorrer flujos de autenticación, autorización, GPS, órdenes y enlaces temporales.
3. Formular escenarios de amenaza con atacante, punto de entrada, objetivo y impacto.
4. Revisar código y configuración buscando evidencia concreta.
5. Clasificar hallazgos por impacto, explotabilidad, alcance y confianza.
6. No convertir sospechas o recomendaciones preventivas en vulnerabilidades confirmadas.
7. Documentar ubicación, evidencia, severidad y corrección mínima.
8. Añadir pruebas de regresión de seguridad.
9. Si se solicita remediación, aplicar solo cambios autorizados y validar el flujo afectado.

## Checklist prioritario

- Revocación y expiración de JWT.
- Autorización horizontal y vertical por rol.
- Alcance mínimo y expiración de enlaces temporales al cliente final.
- Protección de coordenadas históricas y actuales.
- Validación de entrada, idempotencia y abuso masivo.
- Secretos fuera de código, logs, imágenes y clientes.
- Permisos mínimos en cloud, contenedores y CI/CD.
- Registro de auditoría sin datos sensibles innecesarios.

## Criterios de calidad

- La autorización crítica se aplica en backend; el cliente solo mejora la experiencia.
- Los hallazgos contienen una explicación reproducible y no solo una etiqueta de OWASP.
- Los datos sensibles nunca se muestran innecesariamente en informes o fixtures.
- Las correcciones evitan regresiones funcionales y mantienen compatibilidad.

## Entregables esperados

- Resumen ejecutivo de riesgos.
- Lista priorizada con evidencia y ubicación.
- Pruebas o verificaciones de remediación propuestas.
- Correcciones implementadas solo dentro del alcance solicitado.
