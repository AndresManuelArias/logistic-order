---
name: quality-assurance
description: "Use para diseñar o ejecutar pruebas unitarias, integración, API, E2E, accesibilidad, carga con k6, resiliencia, sincronización offline, WebSockets y RBAC de Tiendas-Track."
---

# Garantía de Calidad

## Objetivo

Validar requisitos con pruebas reproducibles y priorizadas por riesgo funcional, seguridad y pérdida de datos.

## Cuando usar

- Crear o revisar la estrategia de pruebas de un módulo.
- Escribir pruebas unitarias, integración, contrato o E2E.
- Validar RBAC, duplicados, telemetría, WebSockets o sincronización offline.
- Diseñar escenarios de carga o resiliencia con k6.
- Investigar y clasificar fallos.

## Procedimiento

1. Extraer los requerimientos y criterios de aceptación aplicables.
2. Crear una matriz breve de trazabilidad requisito-caso-riesgo.
3. Seleccionar los niveles mínimos de prueba que cubran el cambio.
4. Definir precondiciones, datos, acciones, resultado esperado y limpieza.
5. Priorizar autorización, integridad, duplicación, pérdida de datos y seguridad.
6. Mantener pruebas deterministas y sin dependencias innecesarias.
7. En carga, fijar escenarios, concurrencia, duración y métricas antes de ejecutar.
8. Ejecutar primero la selección específica y ampliar solo ante resultados relevantes.
9. Separar defectos de producto, fallos ambientales y limitaciones de herramienta.

## Escenarios específicos

- RBAC cruzado entre proveedor, tendero y cliente final.
- Coordenadas duplicadas, malformadas, fuera de orden y sin conexión.
- Sincronización parcial, reinicio de aplicación y reintento.
- WebSocket desconectado, reconectado y actualizado con datos antiguos.
- Carga concurrente de vehículos con tasa controlada de duplicados y errores.
- Degradación o caída de dependencias y circuit breakers.

## Criterios de calidad

- Las pruebas representan requisitos confirmados y los supuestos están identificados.
- Los fixtures no contienen secretos ni datos personales innecesarios.
- Cada fallo incluye impacto, evidencia, reproducibilidad y alcance.
- No se reporta una validación como ejecutada si solo fue diseñada.
- Las métricas de carga tienen umbrales acordados o se presentan como pendientes.

## Entregables esperados

- Casos o suites implementadas.
- Resultado detallado de la ejecución seleccionada.
- Matriz de trazabilidad actualizada cuando sea útil.
- Riesgos, limitaciones y próximos casos pendientes.
