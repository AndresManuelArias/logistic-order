---
name: mobile-react-native-development
description: "Use para desarrollar la app React Native del conductor de Tiendas-Track con GPS, modo offline-first, WatermelonDB, cola de sincronización, idempotencia, permisos y CI móvil."
---

# Desarrollo Mobile React Native

## Objetivo

Crear una aplicación de conductor resiliente que conserve datos en modo offline y los sincronice sin duplicarlos ni perderlos.

## Cuando usar

- Implementar captura GPS, actualización de estados o órdenes.
- Crear persistencia local, cola, reintentos o sincronización por lotes.
- Gestionar conectividad, permisos de ubicación, batería o ciclo de vida.
- Depurar pérdida de datos, duplicados o inconsistencias de sincronización.
- Preparar empaquetado o CI/CD móvil.

## Procedimiento

1. Leer requisitos, contratos de API y modelo de sincronización aprobado.
2. Diseñar modelos locales separando dominio, persistencia local, transporte remoto y UI.
3. Asignar IDs idempotentes a coordenadas y operaciones locales.
4. Persistir primero los datos locales antes de intentar sincronización remota.
5. Mantener registros pendientes hasta recibir confirmación inequívoca del servidor.
6. Sincronizar por lotes con reintentos acotados y deduplicación.
7. Gestionar permisos, información de estado y errores de plataforma de forma comprensible.
8. Considerar consumo de batería, frecuencia GPS y almacenamiento disponible.
9. Añadir pruebas unitarias, de persistencia y de transiciones offline/reconexión.
10. Ejecutar chequeos disponibles para plataformas objetivo y documentar pruebas de hardware pendientes.

## Criterios de calidad

- No se eliminan datos locales por un error temporal de red o servidor.
- Cada operación remota tolera reenvíos y duplicados.
- Las claves secretas no se escriben en código ni almacenamiento inseguro.
- El almacenamiento, conectividad y API usan adaptadores reemplazables.
- Se requiere autorización real del backend aunque la UI oculte una acción.

## Entregables esperados

- Funcionalidad móvil implementada y probada con datos simulados.
- Modelo local y estrategia de sincronización documentados cuando sean nuevos.
- Pruebas relacionadas ejecutadas.
- Limitaciones de plataforma o hardware declaradas.
