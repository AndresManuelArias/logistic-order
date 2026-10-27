
---
name: architecture-design
description: Metodología para analizar requerimientos y producir arquitecturas de software, diagramas C4, modelos de datos, contratos API, ADR y planes de implementación.
---

# Skill: Diseño de arquitectura de software

## Objetivo

Transformar requerimientos de negocio en una arquitectura técnica trazable,
con decisiones justificadas y un plan de implementación verificable.

## Fase 1: Descubrimiento

- Identificar actores, objetivos y límites del sistema.
- Extraer requerimientos funcionales.
- Identificar atributos de calidad: seguridad, rendimiento, disponibilidad,
  mantenibilidad, escalabilidad y observabilidad.
- Registrar restricciones tecnológicas, presupuestarias y operativas.
- Clasificar cada dato como confirmado, supuesto o pendiente.
- Preguntar por los datos críticos que falten.

## Fase 2: Diseño de alto nivel

- Definir el contexto y los sistemas externos.
- Comparar monolito modular, microservicios y otras opciones pertinentes.
- Elegir la arquitectura más sencilla que satisfaga los requerimientos.
- Definir responsabilidades y límites de los módulos.
- Mostrar las comunicaciones síncronas y asíncronas.
- Crear diagramas C4 de contexto y contenedores en Mermaid.

## Fase 3: Diseño detallado

- Definir módulos, interfaces y dependencias.
- Diseñar endpoints REST, métodos HTTP, solicitudes y respuestas.
- Especificar autenticación, autorización, validaciones y errores.
- Diseñar entidades, relaciones, índices y restricciones de la base de datos.
- Definir transacciones, idempotencia y consistencia cuando correspondan.
- Documentar integraciones externas y eventos de dominio si son necesarios.

## Fase 4: Seguridad y operación

- Identificar amenazas, datos sensibles y límites de confianza.
- Definir gestión de secretos y permisos de mínimo privilegio.
- Establecer logs, métricas, trazas y alertas.
- Definir backups, recuperación ante desastres y objetivos de disponibilidad
  cuando los requerimientos lo justifiquen.
- Identificar cuellos de botella, dependencias críticas y costes operativos.

## Fase 5: Decisiones arquitectónicas

Para cada decisión importante, documentar:

- Contexto y problema.
- Opciones consideradas.
- Decisión elegida.
- Justificación y consecuencias.
- Riesgos y condiciones que podrían obligar a revisarla.

## Fase 6: Plan de implementación

- Dividir el trabajo en fases y entregables pequeños.
- Definir dependencias entre módulos.
- Añadir pruebas unitarias, integración y aceptación.
- Establecer criterios de aceptación medibles.
- Identificar riesgos, preguntas abiertas y tareas pendientes.

## Reglas de calidad

- No introducir tecnologías por moda.
- No elegir microservicios sin una justificación concreta.
- No asumir volúmenes, latencias o disponibilidad que no se hayan especificado.
- Explicitar los supuestos y separar las decisiones provisionales de las confirmadas.
- Mantener trazabilidad entre requerimientos, componentes y pruebas.
- No modificar código de la aplicación: esta skill está orientada al diseño.