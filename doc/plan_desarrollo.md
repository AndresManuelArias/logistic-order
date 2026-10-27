# Plan de Desarrollo — Tiendas-Track (Logística, Trazabilidad y Telemetría Multi-Rol)

> **Documento maestro para agentes de IA.**
> Cualquier agente que vaya a implementar, revisar o probar una parte del sistema **debe leer este documento completo antes de empezar** y respetar las decisiones aquí registradas. Si encuentra una contradicción con otro documento, este plan prevalece y debe escalarse al rol `architect`.

- **Versión:** 1.0
- **Fecha:** 2026-10-27
- **Estado:** Propuesta pendiente de aprobación del usuario
- **Fuentes:** `doc/requeriment/*`, `doc/arquitectura/*` (monolito modular, microservicios, UML casos de uso, estructuras de carpetas)

---

## 1. Resumen Ejecutivo

**Tiendas-Track** es un módulo de logística multi-rol para una plataforma POS dirigida a tiendas de barrio (tenderos) en Latinoamérica. Permite:

1. **Trazabilidad en tiempo real** de la flota que lleva mercancía a las tiendas (B2B) y de la flota que reparte pedidos de las tiendas a sus clientes finales (B2C).
2. **Gestión del ciclo de vida de órdenes** (Creado → En Tránsito → Entregado / Cancelado) con asociación a vehículo y conductor.
3. **App móvil offline-first** del conductor/domiciliario que captura GPS sin conexión y sincroniza en bloque al reconectar.
4. **Portal web multi-rol** (Proveedor / Tendero / Cliente Final) construido sobre librerías reutilizables (`@tiendas/tracking-core`, `@tiendas/tracking-ui`).
5. **Agente operativo de IA** que traduce preguntas en lenguaje natural a consultas analíticas sobre la flota.

**Problema que resuelve:** los tenderos y proveedores no tienen visibilidad confiable de dónde está su mercancía ni de sus repartos en curso; la información vive en llamadas, hojas de cálculo o grupos de mensajería, sin historial ni métricas.

**Restricción clave de entrega:** prueba técnica de nivel senior con ventana de 8–12 horas usando agentes de IA, por lo que el plan prioriza verticalidades demostrables sobre cobertura exhaustiva.

---

## 2. Requerimientos y Trazabilidad

### 2.1 Requerimientos Funcionales (fuente: `doc/requeriment/requerimientos_funcionales_no_funcionales.md`)

| ID | Requerimiento | Módulo dueño | Fase |
|---|---|---|---|
| RF-01 | Autenticación única con credenciales seguras y JWT | `auth` | F2 |
| RF-02 | RBAC con aislamiento de datos: Proveedor (flota global), Tendero (vista dual entrada/salida), Cliente Final (token de invitado temporal a su orden) | `auth` + `telemetry` + `orders` | F3 |
| RF-03 | Ingesta de coordenadas (lat, lon, velocidad, timestamp) de vehículos/app | `telemetry` | F3 |
| RF-04 | Transmisión en tiempo real vía WebSockets/SSE | `telemetry` + `shared/websocket` | F3 |
| RF-05 | Ciclo de vida de órdenes: Creado, En Tránsito, Entregado, Cancelado | `orders` | F2 |
| RF-06 | Asociación de orden → vehículo y conductor | `orders` | F2 |
| RF-07 | Captura GPS automática en la app del conductor | móvil `tracking` | F4 |
| RF-08 | Persistencia local sin conexión (SQLite/WatermelonDB) | móvil `tracking` | F4 |
| RF-09 | Bulk Sync automático al recuperar red | móvil `tracking` | F4 |
| RF-10 | Agente IA con consultas en lenguaje natural sobre la flota | `aiagent` | F5 |

### 2.2 Requerimientos No Funcionales

| ID | Requerimiento | Impacto en el diseño | Fase |
|---|---|---|---|
| RNF-01 | Clean Architecture y DDD en backend Go y frontends | Estructura de carpetas obligatoria (§4) | F1 |
| RNF-02 | Monorepo/librerías `@tiendas/tracking-core` y `@tiendas/tracking-ui` | Estructura de monorepo web (§4.2) | F3 |
| RNF-03 | TimescaleDB para telemetría de alta frecuencia | Hypertable + retención (§5) | F1 |
| RNF-04 | Latencia de WebSockets < 500 ms en condiciones normales | Hub en memoria, sin round-trips a BD por mensaje | F3 |
| RNF-05 | Circuit Breakers (`sony/gobreaker`) en comunicaciones internas/externas | `shared/resilience` | F5 |
| RNF-06 | Pruebas de carga con k6 (cientos de vehículos, duplicados y errores controlados) | scripts `load/` | F6 |
| RNF-07 | Docker Compose para todo el entorno local | `docker-compose.yml` raíz | F1 |
| RNF-08 | IaC (Terraform o AWS CDK) | `infra/` | F6 |
| RNF-09 | CI/CD móvil con Fastlane / GitHub Actions | `.github/workflows` + `fastlane/` | F6 |

### 2.3 Actores y casos de uso (fuente: `doc/arquitectura/uml/casos_de_uso.md`)

- **Proveedor B2B:** autenticar → monitorear flota global → consultar IA.
- **Tendero:** autenticar → monitorear flotas de entrada y salida (dual) → consultar IA.
- **Cliente Final:** visualizar ubicación de su orden específica (token de invitado, sin login completo).
- **Conductor:** autenticar → enviar GPS en tiempo real → (extend) sincronización en bloque al volver la red.
- **Agente de IA:** procesar lenguaje natural → traducir a consulta analítica.

---

## 3. Decisiones Arquitectónicas (ADR)

### ADR-01 — Backend: Monolito Modular en Go (NO microservicios en esta fase)

- **Contexto:** existen dos documentos de arquitectura (`doc/arquitectura/monolito_modular/` y `doc/arquitectura/microservicios/`).
- **Decisión:** implementar el **Monolito Modular** (`doc/arquitectura/monolito_modular/arquitectura_monolito_modular.md`).
- **Justificación:** la ventana de 8–12 h, el entorno local y el objetivo de MVP hacen inviable operar Kafka, Gateway y múltiples servicios. Los módulos aislados por interfaces permiten extraerlos a microservicios después (la arquitectura de microservicios queda como **estado futuro**).
- **Consecuencia:** el bus de eventos interno se resuelve con **Go Channels / dispatcher en memoria** (patrón pub/sub interno), no con Kafka.
- **Revisar si:** el volumen real supera una instancia o se requiere despliegue independiente por módulo.

### ADR-02 — Persistencia: TimescaleDB (PostgreSQL + extensión) como única instancia

- Los datos relacionales (usuarios, tiendas, órdenes) y las series de tiempo (telemetría) conviven en **una misma base** con esquemas separados (`app` y `telemetry`). Simplifica transacciones (orden + asignación de flota) y el entorno Docker.
- La telemetría se modela como **hypertable** con política de retención y agregados por intervalo para las consultas del agente IA.

### ADR-03 — Tiempo real: WebSockets (protocolo primario), SSE como fallback

- Hub central en `internal/shared/websocket` con suscripciones **filtradas por rol y scope** (flota del proveedor / flotas del tendero / orden del cliente final).
- El filtro de autorización se aplica **al momento de suscribirse** y en cada publicación, nunca se delega al cliente.

### ADR-04 — Seguridad: JWT con claims de rol y scope + token de invitado para cliente final

- Access token corto + refresh token rotativo.
- El Cliente Final no usa credenciales: recibe un **token de invitado firmado con `scope: order:<id>` y expiración corta**, que solo da derecho a suscribirse al canal de esa orden.
- Toda consulta pasa por middleware de autorización; el filtrado de datos es **siempre server-side** (nunca confiar en filtros del frontend).

### ADR-05 — Frontend: monorepo con librerías reutilizables

- Monorepo (Turborepo o npm workspaces) con `@tiendas/tracking-core` (hooks de conexión WS, estado de orden, reintentos, auth por rol) y `@tiendas/tracking-ui` (mapas Leaflet/Mapbox, tarjetas de estado, barras de progreso). El portal web es un consumidor más de estas librerías.

### ADR-06 — Móvil: offline-first con WatermelonDB y cola de sincronización

- Cola `SyncQueueManager` persistente con **idempotency key** por registro para que el Bulk Sync pueda reintentarse sin duplicar coordenadas ni eventos de entrega en el backend.

### ADR-07 — Agente IA: capa de herramientas sobre endpoints internos + vistas SQL

- El agente **no** ejecuta SQL arbitrario escrito libremente por el modelo. Expone un **catálogo de herramientas/consultas parametrizadas** (vistas SQL predefinidas y endpoints internos) que el modelo selecciona y rellena con parámetros. Reduce el riesgo de inyección SQL y el coste de tokens.
- El motor LLM es configurable por variable de entorno (`LLM_PROVIDER`, `LLM_API_KEY`); sin credenciales, el agente responde con un error controlado y el resto del sistema sigue funcionando.

### ADR-08 — Estilo de API: REST/JSON versionada en `/api/v1` + WebSocket en `/ws`

- REST para comandos y consultas puntuales; WebSocket para push. `Idempotency-Key` en creación de órdenes y en ingesta por lote de telemetría.

---

## 4. Estructura del Repositorio

### 4.1 Backend (monolito Go) — fuente: `doc/arquitectura/monolito_modular/estructura_carpetas_backend.md`

```
backend/
├── cmd/server/main.go
├── internal/
│   ├── shared/
│   │   ├── database/      # conexión, migraciones, hypertable TimescaleDB
│   │   ├── middleware/    # JWT, RBAC, logger, recovery, rate-limit, CORS
│   │   ├── websocket/     # hub, suscripciones filtradas por rol
│   │   └── resilience/    # circuit breakers (gobreaker)
│   ├── auth/              # domain / usecase / delivery
│   ├── orders/            # domain / usecase / delivery
│   ├── telemetry/         # domain / usecase / delivery (ingesta + consulta)
│   └── aiagent/           # domain / usecase / delivery (herramientas + chat)
├── pkg/logger/
├── .env.example
├── Dockerfile
└── go.mod
```

**Regla de dependencias (Clean Architecture):** `delivery → usecase → domain`. `domain` no importa nada de infraestructura. La infraestructura implementa interfaces definidas en `domain`.

### 4.2 Frontend web (monorepo) — **estructura propuesta, pendiente de confirmación**

> ⚠️ **Pendiente (R1):** `doc/arquitectura/monolito_modular/estructura_carpeta_portal_web_multi_rol.md` contiene **por error la estructura de la app móvil duplicada** (idéntica a `estructura_carpetas_app_conductor.md`). Mientras se corrige, se usa la siguiente propuesta:

```
apps/web/                    # Portal Next.js multi-rol
├── src/app/                 # rutas: /login, /proveedor, /tendero, /orden/[id]
├── src/modules/             # auth, orders, tracking, aiagent (presentation)
└── src/core/                # api client, ws client, auth provider
packages/
├── tracking-core/           # @tiendas/tracking-core (hooks, estado, WS)
└── tracking-ui/             # @tiendas/tracking-ui (mapas, tarjetas, progreso)
```

### 4.3 App móvil del conductor — fuente: `doc/arquitectura/monolito_modular/estructura_carpetas_app_conductor.md`

```
mobile/src/
├── app/            # navigation, providers, App.tsx
├── core/           # api, database (WatermelonDB), network (NetInfo), storage
└── modules/
    ├── auth/       # domain / data / presentation
    ├── orders/     # domain / data / presentation
    └── tracking/   # domain / data (queue/SyncQueueManager) / presentation
```

---

## 5. Modelo de Datos (esquemas)

**Esquema `app` (PostgreSQL):**

| Tabla | Columnas clave | Notas |
|---|---|---|
| `users` | id, email, password_hash, role (`PROVIDER`\|`STORE`\|`DRIVER`), status, created_at | RF-01 |
| `stores` | id, name, owner_user_id, address, geo | tienda de barrio |
| `vehicles` | id, plate, provider_id, status | flota B2B |
| `drivers` | id, user_id, vehicle_id | conductor vinculado |
| `orders` | id, external_ref, store_id, customer_id?, status (`CREATED`\|`IN_TRANSIT`\|`DELIVERED`\|`CANCELLED`), assigned_vehicle_id, assigned_driver_id, created_at, delivered_at | RF-05, RF-06 |
| `order_events` | id, order_id, status, actor_user_id, occurred_at, payload jsonb | auditoría de transiciones |
| `guest_tokens` | id, order_id, token_hash, expires_at, revoked | RF-02 cliente final |
| `sync_receipts` | id, idempotency_key UNIQUE, device_id, processed_at | RF-09 idempotencia del bulk sync |

**Esquema `telemetry` (TimescaleDB):**

| Objeto | Definición |
|---|---|
| `telemetry.points` (hypertable) | time TIMESTAMPTZ, vehicle_id, order_id?, lat, lon, speed, accuracy, source (`DEVICE`\|`BULK_SYNC`), device_seq BIGINT — PK compuesta `(time, vehicle_id, device_seq)` |
| `telemetry.stops` (vista/agregado) | paradas: vehículo detenido > N minutos en un radio — alimenta al agente IA |
| Retención | política `drop_chunks` configurable (ej. 90 días) |

**Reglas de integridad:**
- Transición de estado de orden solo vía máquina de estados en `usecase` (evita saltos Creado → Entregado).
- Escritura de telemetría con `ON CONFLICT`/`device_seq` para tolerar duplicados (RNF-06).
- `order_events` es append-only (nunca UPDATE).

---

## 6. Contrato API (v1) — resumen

> Especificación detallada a producir en F2 como `doc/api/openapi.yaml`. Este resumen es el alcance mínimo.

**Auth**
- `POST /api/v1/auth/login` → tokens
- `POST /api/v1/auth/refresh`
- `POST /api/v1/auth/guest-token` (rol STORE/PROVIDER) → token de invitado para `order_id`
- `POST /api/v1/auth/logout`

**Órdenes**
- `POST /api/v1/orders` (idempotency) · `GET /api/v1/orders` (filtros: status, store, vehicle, rango de fechas) · `GET /api/v1/orders/{id}`
- `PATCH /api/v1/orders/{id}/status` (transiciones validadas)
- `POST /api/v1/orders/{id}/assignment` (vehículo + conductor)

**Telemetría**
- `POST /api/v1/telemetry/points` (ingesta individual, app)
- `POST /api/v1/telemetry/bulk` (Bulk Sync con `Idempotency-Key`)
- `GET /api/v1/vehicles/{id}/track?from=&to=` (traza histórica, RBAC)
- `GET /api/v1/vehicles/{id}/latest`

**Tiempo real**
- `GET /ws` (upgrade WebSocket) → mensajes `SUBSCRIBE`/`UNSUBSCRIBE` sobre canales: `fleet:provider:{id}`, `fleet:store:{id}`, `order:{id}`. El servidor valida el scope del JWT antes de suscribir.

**Agente IA**
- `POST /api/v1/ai/query` `{ "question": "..." }` → `{ "answer", "tool", "params", "rows?" }`

**Errores de negocio (formato consistente):**
`{ "error": { "code": "ORDER_INVALID_TRANSITION", "message": "...", "details": {} } }`
Códigos: `401 UNAUTHENTICATED` · `403 FORBIDDEN_SCOPE` · `404 NOT_FOUND` · `409 CONFLICT/IDEMPOTENCY` · `422 VALIDATION` · `429 RATE_LIMITED` · `503 CIRCUIT_OPEN`.

---

## 7. Plan de Implementación por Fases

> **Orden estricto:** una fase no arranca hasta que sus dependencias estén entregadas. Cada tarea indica el **agente responsable** (definidos en `.opencode/agents/`). El frontend y el móvil pueden **iniciar en paralelo** contra contratos mock (OpenAPI de F2).

### F1 — Preparación y esqueleto
- **Objetivo:** entorno ejecutable de punta a punta con `docker compose up`.
- **Agentes:** `devops` (principal), `backend-go` (apoyo).
- **Tareas:**
  1. Estructura del monolito Go (§4.1) + `go.mod` + logger + config por env.
  2. `docker-compose.yml`: `backend`, `timescaledb`, `web` (placeholder), migraciones al arrancar.
  3. Migraciones iniciales: esquemas `app` y `telemetry`, hypertable + retención.
  4. Healthcheck `GET /healthz`.
- **Entregables:** repo compila, `make dev` levanta el stack, migraciones aplicadas.
- **Criterio de aceptación:** `docker compose up` sin errores y `curl /healthz` → 200.
- **Pruebas:** smoke test de arranque en CI.

### F2 — Auth, RBAC y ciclo de vida de órdenes
- **Objetivo:** RF-01, RF-02 (base), RF-05.
- **Depende de:** F1.
- **Agentes:** `backend-go` (principal), `security` (revisión).
- **Tareas:**
  1. Módulo `auth`: registro/login, hashing (bcrypt/argon2), JWT + refresh rotativo, claims `{sub, role, store_id?, provider_id?, scope?}`.
  2. Middleware RBAC + `RequireRole` + filtrado de datos por rol en repositorios.
  3. Módulo `orders`: CRUD, máquina de estados, `order_events`, asignación de flota (RF-06).
  4. Token de invitado para Cliente Final, con expiración y scope `order:<id>`.
  5. Seed de datos demo (1 proveedor, 2 tiendas, 3 conductores, 10 órdenes).
  6. Contrato `doc/api/openapi.yaml` con lo implementado.
- **Criterio de aceptación:** un usuario de un rol **nunca** puede leer recursos de otro rol (probado); transiciones inválidas → 409.
- **Pruebas:** unitarias de dominio y casos de uso; integración API del happy path y de denegación por rol.

### F3 — Telemetría, WebSockets y filtrado por rol en tiempo real
- **Objetivo:** RF-03, RF-04, RNF-03, RNF-04.
- **Depende de:** F1, F2.
- **Agentes:** `backend-go` (principal), `security` (revisión de scope de canales).
- **Tareas:**
  1. Ingesta `POST /telemetry/points` y `/bulk` con idempotencia (`device_seq`, `sync_receipts`).
  2. Escritura por lotes en la hypertable (batch/CopyFrom).
  3. Hub WebSocket con suscripciones validadas contra el scope del token; publicación en canales por rol/orden.
  4. Circuit Breaker (`gobreaker`) en el acceso a la base de datos de los caminos de lectura.
  5. Traza histórica y `latest` con RBAC.
- **Criterio de aceptación:** un cliente suscrito a un canal **ajeno** es rechazado; latencia de publicación < 500 ms (RNF-04) medida en local.
- **Pruebas:** unitarias del hub, integración WS (suscripción válida/inválida), prueba de duplicados.

### F4 — Frontend: librerías, portal multi-rol y app móvil offline-first
- **Objetivo:** RF-02 (UI), RF-07, RF-08, RF-09, RNF-02.
- **Depende de:** F2 y F3 para integración (puede arrancar antes con mocks).
- **Agentes:** `frontend-react` (portal + librerías), `mobile-react-native` (app).
- **Tareas web:**
  1. Monorepo con `@tiendas/tracking-core` (hook `useTracking(channel)`, reconexión con backoff, estado de conexión) y `@tiendas/tracking-ui` (mapa con capa de vehículos, tarjeta de orden, barra de progreso).
  2. Portal Next.js: login, panel Proveedor (flota global + mapa), panel Tendero (dual entrada/salida), vista pública de orden por token de invitado, panel de chat IA.
  3. RBAC en rutas y render condicional (la autoridad sigue siendo el servidor).
- **Tareas móvil:**
  1. Login del conductor, lista de órdenes del día, detalle con prueba de entrega.
  2. Servicio de geolocalización en segundo plano → cola `SyncQueueManager` (WatermelonDB/SQLite).
  3. Detección de red (NetInfo) → Bulk Sync con idempotency key; barra de estado "X puntos pendientes".
  4. Transiciones de orden offline con cola de eventos.
- **Criterio de aceptación:** modo avión → los puntos se acumulan localmente; al reactivar la red se sincronizan **sin duplicar** (verificado en backend). El portal renderiza las tres vistas con datos reales.
- **Pruebas:** unitarias de `tracking-core`, E2E del flujo de sync (Playwright/Detox), verificación básica de accesibilidad (contraste, teclado).

### F5 — Agente IA operativo + resiliencia transversal
- **Objetivo:** RF-10, RNF-05.
- **Depende de:** F3.
- **Agentes:** `backend-go` (herramientas + endpoints), `architect` (diseño del catálogo de herramientas).
- **Tareas:**
  1. Catálogo de herramientas parametrizadas: `get_stopped_vehicles(minutes, area?)`, `get_order_status(order_id)`, `get_fleet_eta(store_id)`, `get_late_deliveries(range)`.
  2. Integración LLM configurable (proveedor por env); prompt de sistema con reglas (no inventar datos, citar la herramienta usada).
  3. Endpoint `POST /ai/query` con auditoría de cada consulta (pregunta, herramienta, params, latencia).
  4. Circuit Breaker en la llamada al proveedor LLM (si falla → 503 controlado).
- **Criterio de aceptación:** "¿Qué vehículos llevan detenidos más de 20 minutos?" devuelve filas reales con la herramienta correcta; sin API key → error controlado, no 500 inesperado.

### F6 — Pruebas de carga, IaC, CI/CD y documentación
- **Objetivo:** RNF-06, RNF-08, RNF-09 + documentación final.
- **Depende de:** F3–F5.
- **Agentes:** `qa`, `devops`, `security`.
- **Tareas:**
  1. Script **k6**: 100+ vehículos concurrentes, 5% duplicados, errores inyectados; umbrales p95 de latencia de ingesta y de WS.
  2. Terraform (o CDK) mínimo: red, base de datos, servicio, secretos.
  3. GitHub Actions: lint + test backend, lint + build web, build móvil con Fastlane.
  4. Auditoría de seguridad (checklist §9.4) + README con decisiones de arquitectura y **auditoría del uso de la IA** (requisito de evaluación).
- **Criterio de aceptación:** pipeline verde; k6 cumple umbrales; `terraform plan` sin errores.

---

## 8. Matriz de Trazabilidad (resumen)

| Fase | RF cubiertos | RNF cubiertos | Agentes |
|---|---|---|---|
| F1 | — | RNF-01, RNF-03 (base), RNF-07 | devops, backend-go |
| F2 | RF-01, RF-02 (base), RF-05, RF-06 | RNF-01 | backend-go, security |
| F3 | RF-03, RF-04 | RNF-03, RNF-04 | backend-go, security |
| F4 | RF-02 (UI), RF-07, RF-08, RF-09 | RNF-02 | frontend-react, mobile-react-native |
| F5 | RF-10 | RNF-05 | backend-go, architect |
| F6 | — | RNF-06, RNF-08, RNF-09 | qa, devops, security |

---

## 9. Reglas para los Agentes

### 9.1 Antes de empezar
1. Lee este documento completo y el de tu rol en `doc/arquitectura/`.
2. Lee los RF/RNF que te corresponden en `doc/requeriment/`.
3. Si algo no está definido aquí, **pregunta o registra un supuesto**; no inventes reglas de negocio.

### 9.2 Convenciones obligatorias
- **Idioma:** elegir español o inglés por repo y mantenerlo de forma consistente en código y documentación.
- **Nombres:** estados en mayúsculas (`IN_TRANSIT`), endpoints en plural (`/api/v1/orders`), códigos de error en `SCREAMING_SNAKE`.
- **Commits:** convención convencional (`feat:`, `fix:`, `test:`, `chore:`).
- **Configuración:** nunca secretos en el repo; todo por variables de entorno con `.env.example` actualizado.
- **Sin dependencias nuevas** sin justificarlas (superficie de seguridad y tiempo de entrega).

### 9.3 Definition of Done (todas las tareas)
- [ ] Compila/lint sin errores.
- [ ] Pruebas unitarias del camino feliz y del camino de error/denegación.
- [ ] Respeta la arquitectura de capas de su módulo (sin `usecase` importando frameworks, sin `domain` importando infraestructura).
- [ ] Manejo de errores tipado y códigos HTTP correctos.
- [ ] Documentación afectada actualizada.
- [ ] Si toca seguridad: revisión del agente `security`.

### 9.4 Seguridad (checklist permanente)
- Autorización **server-side** en cada endpoint y en cada suscripción WebSocket.
- Validación de entrada en el borde (DTOs) y parámetros del agente IA acotados a un catálogo (sin SQL concatenable).
- Rate limiting en login, ingesta de telemetría y endpoint IA.
- Tokens de invitado: expiración corta, scope mínimo, revocables.
- Auditoría de accesos y de consultas del agente IA.
- No exponer datos de un rol en respuestas de otro (revisar proyecciones SELECT y JSON devuelto).

### 9.5 Calidad
- Backend Go: `go test ./...`, `go vet`, race detector en tests del hub de concurrencia.
- Web: `eslint` + `tsc`, tests de `tracking-core`, una prueba E2E del flujo principal por vista.
- Móvil: pruebas unitarias del `SyncQueueManager` (incluye reintento con duplicados).
- Carga: k6 en F6 con los umbrales definidos.

---

## 10. Riesgos y Preguntas Pendientes

| # | Tema | Tipo | Nota |
|---|---|---|---|
| R1 | `estructura_carpeta_portal_web_multi_rol.md` está duplicado con la estructura móvil | **Doc defectuoso** | Corregir o reemplazar por la §4.2 |
| R2 | No hay definición de `openapi.yaml` ni modelo de datos oficial | Pendiente | Producirlo en F2 como contrato fuente |
| R3 | Motor LLM y proveedor de mapas sin definir | **Decisión pendiente** | Afecta coste, claves y latencia; configurable por env |
| R4 | Volumen real de vehículos y frecuencia de GPS desconocida | **Supuesto** | El plan asume cientos de vehículos (RNF-06); no hay cifras confirmadas |
| R5 | Retención y coste de TimescaleDB no definidos | Pendiente | Definir política de retención antes de producción |
| R6 | WebSockets detrás de reverse proxy/load balancer | Riesgo | Exigir soporte de upgrade (sticky/WS-aware) al desplegar |
| R7 | La arquitectura de microservicios documentada no se implementa en esta fase | **ADR-01** | Documentar en el README el camino de extracción futura |
| R8 | No hay definición de notificaciones (email/SMS) a tenderos/clientes | Fuera de alcance | Confirmar si se incluye o se excluye explícitamente |

---

## 11. Validación Final (checklist de arquitectura)

- [ ] Todos los RF-01…RF-10 tienen componente, endpoint y prueba asignados (§2.1, §8).
- [ ] Todos los RNF-01…RNF-09 tienen fase y evidencia de verificación (§2.2, §8).
- [ ] Ningún componente sin responsabilidad definida (§4, §6).
- [ ] El aislamiento de datos por rol está implementado y probado server-side.
- [ ] El camino offline→online del móvil no duplica datos (idempotencia extremo a extremo).
- [ ] Las decisiones ADR-01…ADR-08 están reflejadas en el código o documentadas como pendientes.
- [ ] El README incluye decisiones de arquitectura y auditoría del uso de la IA.
