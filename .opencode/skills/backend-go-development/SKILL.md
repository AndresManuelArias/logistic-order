---
name: backend-go-development
description: "Use para implementar, depurar o revisar el backend Go de Tiendas-Track con Clean Architecture, DDD, APIs REST, WebSockets, JWT, RBAC, PostgreSQL y TimescaleDB."
---

# Desarrollo Backend Go

## Objetivo

Implementar cambios seguros, trazables y mantenibles en el monolito modular Go sin introducir acoplamientos distribuidos innecesarios.

## Cuando usar

- Crear o modificar módulos, casos de uso, endpoints, repositorios o migraciones.
- Implementar autenticación JWT, autorización RBAC, telemetría o WebSockets.
- Diseñar sincronización masiva, idempotencia y eventos internos.
- Depurar errores de persistencia, concurrencia o integración.

## Procedimiento

1. Leer requisitos, arquitectura aprobada, ADR y contratos afectados.
2. Localizar la responsabilidad en `internal/modules` y respetar sus límites.
3. Implementar de dentro hacia fuera: entidad y reglas, casos de uso, puertos, adaptadores y infraestructura.
4. Mantener reglas de negocio independientes de HTTP, SQL, JWT y proveedores externos.
5. Validar entrada, autorización, errores y estado transaccional en el límite apropiado.
6. Añadir claves idempotentes y deduplicación en ingestión y sincronización.
7. Escribir pruebas unitarias cercanas al dominio y pruebas de integración en persistencia/API.
8. Ejecutar formato, análisis estático y la suite específica del cambio.

## Criterios de calidad

- No se permiten imports ascendentes desde dominio hacia infraestructura.
- Los módulos se comunican mediante interfaces, puertos o eventos internos, nunca mediante implementaciones concretas ajenas.
- Las migraciones protegen integridad con restricciones y índices justificados.
- Los handlers HTTP no contienen lógica de negocio relevante.
- Las operaciones de concurrencia, contexto y cancelación se manejan correctamente.
- No se introducen Kafka, microservicios o brokers sin una decisión aprobada.

## Entregables esperados

- Código implementado dentro de los límites acordados.
- Pruebas de regresión relevantes.
- Migraciones o contratos actualizados cuando corresponda.
- Resumen con archivos afectados, validaciones ejecutadas y supuestos pendientes.
