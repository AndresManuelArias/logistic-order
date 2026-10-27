---
name: frontend-react-development
description: "Use para desarrollar el portal web React o Next.js de Tiendas-Track, incluyendo vistas multi-rol, RBAC, REST, WebSockets, accesibilidad, monorepos y componentes compartidos."
---

# Desarrollo Frontend React

## Objetivo

Construir una interfaz multi-rol mantenible que refleje contratos aprobados y aplique de forma segura los permisos de proveedor, tendero y cliente final.

## Cuando usar

- Crear vistas, rutas, formularios, estados o componentes del portal.
- Integrar APIs REST o canales WebSocket/SSE.
- Implementar navegación por rol y expiración de sesión.
- Extraer lógica o componentes a paquetes compartidos.
- Depurar autorización, accesibilidad, estado o integración de tiempo real.

## Procedimiento

1. Leer requisitos, contratos HTTP/WebSocket y diseño aprobado.
2. Identificar actores, permisos, rutas y datos visibles para cada rol.
3. Separar dominio, casos de uso, puertos/adaptadores y presentación.
4. Centralizar cliente HTTP, tipos, manejo de errores y renovación de sesión.
5. Implementar estados de carga, vacío, error, sesión caducada y acceso denegado.
6. Suscribirse a WebSockets/SSE con reconexión, limpieza y protección contra actualizaciones obsoletas.
7. Usar HTML semántico, teclado, etiquetas, contraste y responsive como parte de la implementación.
8. Añadir pruebas de componentes, interacción, permisos y accesibilidad.
9. Ejecutar typecheck, lint, formato y la suite específica disponible.

## Criterios de calidad

- Las restricciones visuales complementan, pero nunca sustituyen, la autorización del backend.
- Los secretos nunca se incluyen ni persisten en el cliente.
- No hay suscripciones duplicadas ni listeners sin limpieza.
- Los contratos no se inventan para ocultar discrepancias con backend.
- Las vistas reutilizables están desacopladas de detalles de transporte cuando sea posible.

## Entregables esperados

- Componentes y rutas implementados.
- Pruebas de casos principales y permisos.
- Contratos o documentación relacionada actualizados.
- Validaciones ejecutadas y limitaciones conocidas.
