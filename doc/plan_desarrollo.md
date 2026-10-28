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

### ADR-09 — Geodatos: PostGIS para todas las coordenadas

- PostgreSQL se extiende con **PostGIS**; toda columna geográfica usa `geometry(Point, 4326)` (WGS84, estándar GPS).
- **`orders.destination`** almacena el punto exacto donde debe llegar la orden (y `orders.origin` el de salida), con **índices GiST**.
- Consultas espaciales con `ST_DWithin(::geography)`, `ST_Distance_Sphere`, `ST_Contains` (geocercas), en lugar de cálculos sobre columnas `lat`/`lon` sueltas.
- TimescaleDB y PostGIS conviven en la misma hypertable (`telemetry.points.location`).
- Revisar si: aparece un caso de uso que exija geometrías complejas (polígonos de ruta) no cubierto por los tipos ya elegidos.

---

### ADR-10 — Documentación de API: OpenAPI 3 (Swagger) como fuente de verdad

- **Todo endpoint REST** del backend se documenta con una especificación **OpenAPI 3** en `backend/api/openapi.yaml`.
- **Ubicación (decisión F2):** la spec vive en `backend/api/` (y no en `doc/`) para poder embebirla en el binario con `go:embed` (Go no permite `..` en embed): así Swagger se sirve siempre, sin depender de ficheros externos en la imagen. El fichero sigue siendo el contrato revisable en el repositorio y la copia en `doc/` no existe: **un solo canónico**.
- El servidor la sirve en runtime: **Swagger UI** en `/swagger/index.html` y el spec en JSON en `/api/v1/openapi.json`.
- El spec es la **fuente de verdad de los contratos**: el frontend y la app móvil generan/validan sus clientes a partir de él (generación de tipos con `openapi-typescript` / `openapi-generator`), y `qa` valida que la implementación cumple el spec.
- **Criterio:** un endpoint sin entrada/salida documentada en el spec está incompleto ( Definition of Done ). El spec se actualiza en la misma tarea que cambia el endpoint.
- WebSockets se documentan con canales y payloads en la sección `webhooks`/descripción del spec (OpenAPI no tiene soporte nativo de WS).

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
│   ├── stores/            # domain / usecase / delivery (alta de tienda, paso propio)
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

> **Documento completo del modelo de datos:** [`doc/arquitectura/database/modelo_datos.md`](arquitectura/database/modelo_datos.md) (diagrama PlantUML: `modelo_datos.puml` / `modelo_datos.png`).

**Extensiones habilitadas:** `postgis` (todas las coordenadas como `geometry(Point, 4326)`) y `timescaledb` (hypertable). **PostGIS es obligatorio para todo lo geográfico** (ADR-09).

**Esquema `app` (PostgreSQL + PostGIS):**

| Tabla | Columnas clave | Notas |
|---|---|---|
| `users` | id, email, password_hash, role (`PROVIDER`\|`STORE`\|`DRIVER`), status, created_at | RF-01 |
| `stores` | id, name, owner_user_id, address, **location geometry(Point,4326)** | tienda de barrio (PostGIS) |
| `vehicles` | id, plate, provider_id, status, **last_known_location geometry(Point,4326)** | flota B2B (PostGIS) |
| `drivers` | id, user_id, vehicle_id | conductor vinculado |
| `orders` | id, external_ref, store_id, customer_id?, status (`CREATED`\|`IN_TRANSIT`\|`DELIVERED`\|`CANCELLED`), assigned_vehicle_id, assigned_driver_id, **origin geometry(Point,4326)**, **destination geometry(Point,4326)**, destination_address, distance_meters, placed_at, picked_up_at, delivered_at, created_at | RF-05, RF-06 · **PostGIS: `destination` es el punto de llega de la orden** |
| `order_events` | id, order_id, status, actor_user_id, occurred_at, payload jsonb | auditoría de transiciones |
| `guest_tokens` | id, order_id, token_hash, expires_at, revoked | RF-02 cliente final |
| `sync_receipts` | id, idempotency_key UNIQUE, device_id, processed_at | RF-09 idempotencia del bulk sync |

**Esquema `telemetry` (TimescaleDB):**

| Objeto | Definición |
|---|---|
| `telemetry.points` (hypertable) | time TIMESTAMPTZ, vehicle_id, order_id?, **location geometry(Point,4326)**, speed, accuracy, source (`DEVICE`\|`BULK_SYNC`), device_seq BIGINT — PK compuesta `(time, vehicle_id, device_seq)` |
| `telemetry.stops` (**continuous aggregate** de TimescaleDB, refresco 1 min) | paradas: vehículo detenido ≥ N minutos dentro de un radio (ej. 30 m), derivadas de `telemetry.points` — alimenta al agente IA (RF-10) y KPIs |
| Retención | política `drop_chunks` configurable (ej. 90 días) |

**Reglas de integridad:**
- Transición de estado de orden solo vía máquina de estados en `usecase` (evita saltos Creado → Entregado).
- Escritura de telemetría con `ON CONFLICT`/`device_seq` para tolerar duplicados (RNF-06).
- `order_events` es append-only (nunca UPDATE).

---

## 6. Contrato API (v1) — resumen

> **Requerimiento explícito: todos los endpoints deben documentarse con Swagger (OpenAPI 3).**
> Fuente de verdad: `backend/api/openapi.yaml` (OpenAPI 3.1). El backend la sirve como **Swagger UI** en `GET /swagger/index.html` y como spec JSON en `GET /api/v1/openapi.json`. Este resumen es solo el alcance mínimo; **el spec define el contrato real**.

**Documentación interactiva:**
- `GET /swagger/index.html` → Swagger UI (probar endpoints desde el navegador con autorización `Bearer`).
- `GET /api/v1/openapi.json` → spec machine-readable para generar clientes (web y móvil) y para pruebas de contrato en `qa`.

**Auth**
- `POST /api/v1/auth/register` (solo `STORE`/`DRIVER`; el campo `store` es **opcional**: si viene, la tienda nace con el usuario; si no, se da de alta en el paso propio)
- `POST /api/v1/auth/login` → tokens
- `POST /api/v1/auth/refresh`
- `POST /api/v1/auth/guest-token` (rol STORE/PROVIDER) → token de invitado para `order_id`
- `POST /api/v1/auth/logout`

**Tiendas** (rol STORE)
- `POST /api/v1/stores` → alta de tienda en un paso propio del registro (409 si ya existe una)
- `GET /api/v1/stores/me` → tienda del usuario (404 si el alta sigue pendiente)

**Órdenes**
- `POST /api/v1/orders` (idempotency; payload con `origin {lat,lon}` y **`destination {lat,lon}`** o dirección geocodificada) · `GET /api/v1/orders` (filtros: status, store, vehicle, rango de fechas, **radio en metros alrededor de un punto**) · `GET /api/v1/orders/{id}`
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

### Estado de ejecución

| Fase | Estado | Evidencia |
|---|---|---|
| **F1** | ✅ **Completada (2026-10-28)** | `docker compose up` OK; `/healthz` y `/readyz` 200; 4 migraciones aplicadas; hypertable + continuous aggregate + retención verificadas |
| **F2** | ✅ **Completada (2026-10-28)** | Módulos `auth`, `orders` y `stores` + migraciones `0005_auth` y `0006_store_owner_unique`; JWT HS256 con refresh rotativo (hash SHA-256); RBAC de rol y de datos server-side (scope vacío = 403); máquina de estados con `order_events` transaccional; alta de tienda en paso propio del registro (`POST /api/v1/stores`, `GET /api/v1/stores/me`; `store` opcional en register — R11 resuelto); spec OpenAPI 3.1 embebida (`/swagger/index.html`, `/api/v1/openapi.json|yaml`); seed (1 proveedor, 2 tiendas, 3 conductores, 3 vehículos, 10 órdenes georreferenciadas); `go test -race ./...` OK y e2e `scripts/e2e_f2.sh` 72/72 casos OK |
| F3 | ✅ **Completada (2026-10-28)** | Módulo `telemetry` (ingesta individual y bulk idempotente por `Idempotency-Key` + `sync_receipts`, `latest`/`track` con RBAC y circuit breaker); bus interno `shared/eventbus` (Go channels, descarte contabilizado); hub `shared/websocket` con suscripciones validadas, revalidación de concesiones (`WS_GRANT_TTL`), cierre por expiración de token (`4401`) y por consumidor lento (`1008`); `POST /telemetry/points|bulk`, `GET /vehicles/{id}/latest|track` y `GET /ws` documentados en OpenAPI; seed con 90 puntos demo (30 por vehículo, validado con `curl` sobre `track`); `go test -race ./...` OK (16 paquetes) y e2e `scripts/e2e_f3.sh` 63/63 casos OK con latencia WS de **4,3 ms** (RNF-04 < 500 ms) |
| F4 | ✅ **Completada (2026-10-28)** | Monorepo `web/` con `@tiendas/tracking-core` (cliente OpenAPI, refresh single-flight, `useTracking` con backoff/dedupe) y `@tiendas/tracking-ui` (mapa SVG accesible, tarjeta de orden, progreso); portal Next.js 16 con login, `/proveedor`, `/tienda` (dual), `/conductor` y `/seguimiento/{id}#t=…`; BFF same-origin con refresh en cookie `httpOnly`; `GET /api/v1/vehicles` para los paneles; 58 pruebas, typecheck/lint/build OK. App móvil Expo SDK 54 + RN 0.81 + WatermelonDB con GPS en segundo plano, cola write-ahead idempotente (`Idempotency-Key`), transiciones reconciliadas por `GET`, POD local y UI de conductor; 99 pruebas, typecheck y bundle Metro OK; idempotencia validada contra el backend real. Compilación nativa, GPS y permisos reales no validados por falta de emulador/Android SDK |
| F5 | ⬜ Pendiente | — |
| F6 | ⬜ Pendiente | — |

> **Orden estricto:** una fase no arranca hasta que sus dependencias estén entregadas. Cada tarea indica el **agente responsable** (definidos en `.opencode/agents/`). El frontend y el móvil pueden **iniciar en paralelo** contra contratos mock (OpenAPI de F2).

### F1 — Preparación y esqueleto ✅ COMPLETADA (2026-10-28)
- **Objetivo:** entorno ejecutable de punta a punta con `docker compose up`.
- **Resultado:** ✅ `docker compose up -d --build` levanta db (TimescaleDB+PostGIS), backend y web; `/healthz` y `/readyz` responden 200; 4 migraciones aplicadas; `telemetry.points` hypertable, `telemetry.stops` continuous aggregate, retención 90 días y vista `stop_intervals` verificadas. Pruebas de idempotencia, append-only y PostGIS ejecutadas con éxito.
- **Agentes:** `devops` (principal), `backend-go` (apoyo).
- **Tareas:**
  1. Estructura del monolito Go (§4.1) + `go.mod` + logger + config por env.
  2. `docker-compose.yml`: `backend`, `timescaledb`, `web` (placeholder), migraciones al arrancar.
  3. Migraciones iniciales: extensiones `postgis` + `timescaledb`, esquemas `app` y `telemetry`, índices GiST, hypertable + retención, continuous aggregate `telemetry.stops` + política de refresco.
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
  5. Seed de datos demo (1 proveedor, 2 tiendas con `location`, 3 conductores, 10 órdenes **con `origin` y `destination` georreferenciadas**).
  6. **Swagger/OpenAPI:** especificación `backend/api/openapi.yaml` (OpenAPI 3.1) con todos los endpoints de F2, servida como Swagger UI en `/swagger/index.html` y spec JSON en `/api/v1/openapi.json`.
  7. **Alta de tienda en paso propio (R11):** módulo `stores` con `POST /api/v1/stores` y `GET /api/v1/stores/me`; `store` pasa a opcional en el registro y "una tienda por dueño" se protege con índice único (migración `0006`).
- **Criterio de aceptación:** un usuario de un rol **nunca** puede leer recursos de otro rol (probado); transiciones inválidas → 409.
- **Pruebas:** unitarias de dominio y casos de uso; integración API del happy path y de denegación por rol.

### F3 — Telemetría, WebSockets y filtrado por rol en tiempo real ✅ COMPLETADA (2026-10-28)
- **Objetivo:** RF-03, RF-04, RNF-03, RNF-04.
- **Depende de:** F1, F2.
- **Agentes:** `backend-go` (principal), `security` (revisión de scope de canales).
- **Tareas:**
  1. Ingesta `POST /telemetry/points` y `/bulk` con idempotencia (`device_seq`, `sync_receipts`).
  2. Escritura por lotes en la hypertable (batch/CopyFrom).
  3. Hub WebSocket con suscripciones validadas contra el scope del token; publicación en canales por rol/orden.
  4. Circuit Breaker (`gobreaker`) en el acceso a la base de datos de los caminos de lectura.
  5. Traza histórica y `latest` con RBAC.
- **Implementación (evidencia):**
  - **Ingesta por lotes:** `jsonb_to_recordset` expande todo el lote en una sola sentencia `INSERT … ON CONFLICT DO NOTHING RETURNING`; solo se publican las muestras confirmadas como nuevas. El bulk guarda el recibo y los puntos en una única transacción (todo o nada).
  - **Idempotencia:** PK `(time, vehicle_id, device_seq)` para reintentos del dispositivo; `Idempotency-Key` (8–128 chars) obligatoria en el bulk. Reintentar devuelve `already_processed: true` sin reinsertar ni republicar.
  - **Bus interno:** `shared/eventbus` pub/sub no bloqueante (buffer 256, descartes contabilizados en `Dropped()`), sin acoplar módulos.
  - **Hub WS:** protocolo `subscribe|unsubscribe|ping` → `subscribed|unsubscribed|event|error|pong`; canales `fleet:provider`, `fleet:store`, `order` y `vehicle`; autorización server-side al suscribirse y revalidación cada `WS_GRANT_TTL` (60 s); límites 4 KiB, 32 canales y 256 en cola; `CheckOrigin` con `WS_ALLOWED_ORIGINS`; cierre `4401` al expirar el token.
  - **RBAC de telemetría:** `DRIVER` solo su vehículo (deriva `vehicle_id` si falta), `PROVIDER` su flota, `STORE` vehículos de sus órdenes (vista dual), invitado denegado en REST y limitado a `order:<id>` en WS. Vehículo ajeno/inexistente → `403` (anti-enumeración); orden no asignada → `422`.
  - **Resiliencia:** circuit breaker (`sony/gobreaker`) en `latest`/`track`; circuito abierto → `503 CIRCUIT_OPEN` + `Retry-After`. El middleware de logging implementa `http.Hijacker` para no romper el upgrade WS.
  - **Contrato:** OpenAPI 3.1 con los 4 endpoints REST, schemas de telemetría y extensión `x-websocket` (canales, protocolo, límites y códigos de cierre); el test de contrato exige además que todos los `$ref` internos resuelvan.
  - **Seed:** 90 puntos demo (**30 por vehículo**, una muestra/min durante los últimos 30 min) idempotentes para que `latest`, `track` y el WS tengan una traza continua que dibujar sin depender de la app móvil. Re-ejecutar el seed no duplica (`device_seq` 100…129 dentro de una ventana de 2 h).
- **Criterio de aceptación:** un cliente suscrito a un canal **ajeno** es rechazado; latencia de publicación < 500 ms (RNF-04) medida en local.
  - **Cumplido:** e2e rechaza canal ajeno en WS (`FORBIDDEN_SCOPE`) y REST (`403`); latencia medida extremo a extremo (POST → evento en el socket) **4,3 ms** (6,8 ms en la última corrida con la traza ampliada).
  - **`track` con datos reales:** `GET /api/v1/vehicles/{id}/track` devuelve la traza demo ordenada de más reciente a más antiguo (`count` = nº de puntos, con `lat`, `lon`, `speed_kph`, `heading`, `source` y `time`); ventana sin datos → `404`, rango invertido o > 7 días → `422`, vehículo ajeno → `403`.
- **Pruebas:** unitarias del hub (suscripción válida, canal ajeno, canal inválido, unsubscribe, expiración de token, handshake sin auth), del bus, del circuit breaker, de canales y de los casos de uso (RBAC, duplicados, bulk idempotente, rangos); e2e `scripts/e2e_f3.sh` 63/63 (incluye `count > 0` y campos obligatorios en `track`).

### F4 — Frontend: librerías, portal multi-rol y app móvil offline-first
- **Objetivo:** RF-02 (UI), RF-07, RF-08, RF-09, RNF-02.
- **Depende de:** F2 y F3 para integración (puede arrancar antes con mocks).
- **Agentes:** `frontend-react` (portal + librerías), `mobile-react-native` (app).
- **Resultado web (2026-10-28):** ✅ monorepo `web/` con workspaces `@tiendas/portal`, `@tiendas/tracking-core` y `@tiendas/tracking-ui`; tipos generados desde `backend/api/openapi.yaml` (`npm run gen:api`); BFF `/api/session/*` y proxy `/api/backend/*`; paneles de proveedor, tienda y conductor y vista de invitado con datos reales; `GET /api/v1/vehicles` añadido con RBAC; se corrigió el DTO público de `TokenPair` (antes serializaba el usuario de dominio con `PasswordHash` en PascalCase); 58 pruebas (18 core + 26 UI + 14 portal), typecheck/lint/build limpios e imagen Docker validada con login, orden creada/transicionada, enlace de invitado y WS de flota. Ver [`web/README.md`](../web/README.md).
- **Resultado móvil (2026-10-28):** ✅ app Expo SDK 54 + RN 0.81 en [`mobile/`](../mobile/README.md) con Clean Architecture (`domain`/`ports`/`application`/`infrastructure`/`ui`). GPS en segundo plano (`expo-location` + `expo-task-manager`, foreground service Android), captura 15 s/25 m y filtro de precisión; persistencia WatermelonDB/SQLite con write-ahead de lote y `Idempotency-Key`; cola de transiciones con reconciliación por `GET` ante fallos ambiguos/409; detección NetInfo, backoff con jitter, cuarentena sin borrar datos y contador de pendientes; tokens en SecureStore y refresh single-flight; datos aislados por `owner_id`; POD local (el contrato no tiene endpoint). 99 pruebas, `tsc --noEmit`, `expo install --check`, `expo export` y `expo config` en verde. Idempotencia verificada contra el backend real: mismo lote con la misma clave → `already_processed: true`; mismos puntos con clave nueva → `duplicates: 2`, sin inserción. **No validado:** compilación nativa, dispositivo/emulador, GPS y permisos reales, cámara y consumo de batería (entorno sin Android SDK/Xcode).
- **Tareas web:**
  1. ✅ Monorepo con `@tiendas/tracking-core` (hook `useTracking(channel)`, reconexión con backoff, estado de conexión) y `@tiendas/tracking-ui` (mapa con capa de vehículos, tarjeta de orden, barra de progreso).
  2. ✅ Portal Next.js: login, panel Proveedor (flota global + mapa), panel Tendero (dual entrada/salida), vista pública de orden por token de invitado, panel de chat IA (contrato provisional F5).
  3. ✅ RBAC en rutas y render condicional (la autoridad sigue siendo el servidor).
  4. ✅ Generar/validar los tipos y cliente HTTP desde `backend/api/openapi.yaml` (ADR-10), sin duplicar definiciones de DTOs a mano.
- **Tareas móvil:**
  1. ✅ Login del conductor, lista de órdenes del día, detalle con prueba de entrega (evidencia local, sin endpoint POD).
  2. ✅ Servicio de geolocalización en segundo plano → cola `PointSyncService` sobre WatermelonDB/SQLite, con captura headless.
  3. ✅ Detección de red (NetInfo) → Bulk Sync con `Idempotency-Key` write-ahead; badge «X puntos pendientes» y estados online/offline/sincronizando.
  4. ✅ Transiciones de orden offline con cola persistente, orden estricto y reconciliación por `GET`.
  5. ✅ Cliente API tipado generado desde `backend/api/openapi.yaml` (ADR-10) para login, órdenes y bulk sync.
- **Criterio de aceptación:** modo avión → los puntos se acumulan localmente; al reactivar la red se sincronizan **sin duplicar** (verificado en backend). El portal renderiza las tres vistas con datos reales. Aceptación móvil cubierta por la prueba de integración offline→reconexión y por la verificación real de idempotencia del endpoint bulk; queda pendiente repetirla desde un dispositivo.
- **Pruebas:** unitarias de `tracking-core`; 99 pruebas móviles de dominio, cola/sync, sesión, órdenes, POD e integración offline; bundle Metro validado. E2E con Detox y validación de accesibilidad nativa pendientes de emulador.

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
  5. **Validación de contrato:** verificar con **Spectral** (lint del spec) o un test de contrato propio que `backend/api/openapi.yaml` coincide con las respuestas reales del servidor (spec ↔ implementación sincronizados).
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
- [ ] **Endpoint documentado en `backend/api/openapi.yaml` (Swagger) con request/response y códigos de error; Swagger UI se actualiza sin errores de validación.**
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
| R2 | Spec OpenAPI desincronizándose de la implementación | **Mitigado en F2/F3** | Spec creada y embebida en el binario; test `internal/shared/openapi` exige que cubra todos los endpoints F2/F3 y que todos los `$ref` internos resuelvan. Mantenerla en cada tarea de endpoint (ADR-10) y validar en F6 con Spectral |
| R3 | Motor LLM y proveedor de mapas sin definir | **Decisión pendiente** | Afecta coste, claves y latencia; configurable por env |
| R4 | Volumen real de vehículos y frecuencia de GPS desconocida | **Supuesto** | El plan asume cientos de vehículos (RNF-06); no hay cifras confirmadas |
| R5 | Retención y coste de TimescaleDB no definidos | Pendiente | Definir política de retención antes de producción |
| R6 | WebSockets detrás de reverse proxy/load balancer | Riesgo | Exigir soporte de upgrade (sticky/WS-aware) al desplegar. Mitigado en F3: el middleware HTTP respeta `http.Hijacker` (upgrade OK a través del logging) |
| R7 | La arquitectura de microservicios documentada no se implementa en esta fase | **ADR-01** | Documentar en el README el camino de extracción futura |
| R8 | No hay definición de notificaciones (email/SMS) a tenderos/clientes | Fuera de alcance | Confirmar si se incluye o se excluye explícitamente |
| R11 | Registro `STORE` crea la fila de `stores` junto al usuario (supuesto F2) | **Resuelto (2026-10-28)** | El usuario pidió el alta de tienda como **paso propio**: `POST /api/v1/stores` (rol STORE, 409 si ya tiene) y `GET /api/v1/stores/me`, con `store` ahora **opcional** en `POST /auth/register` (si viene, nace junto al usuario en la misma transacción). Una tienda por dueño protegida por índice único (migración `0006_store_owner_unique`) |
| R10 | Swagger spec desincronizado de la implementación | Riesgo | Mitigado por ADR-10: el spec se actualiza en la misma tarea del endpoint + validación de contrato en F6 |
| R12 | Autenticación del handshake WS en navegadores | **Resuelto (2026-10-28)** | Los navegadores no pueden fijar `Authorization` en el handshake: `/ws` acepta `?access_token=` además del header. El token sigue siendo el mismo access token JWT, se valida en servidor y la sesión se cierra al expirar (`4401`). Los clientes nativos usan el header |
| R9 | Origen de la geocodificación de `orders.destination` sin definir | **Pendiente** | ¿El cliente envía lat/lon o dirección? Definir quién geocodifica (cliente, backend o proveedor externo) — ver `doc/arquitectura/database/modelo_datos.md` §6 |

---

## 11. Validación Final (checklist de arquitectura)

- [ ] Todos los RF-01…RF-10 tienen componente, endpoint y prueba asignados (§2.1, §8).
- [ ] Todos los RNF-01…RNF-09 tienen fase y evidencia de verificación (§2.2, §8).
- [ ] Ningún componente sin responsabilidad definida (§4, §6).
- [ ] El aislamiento de datos por rol está implementado y probado server-side.
- [ ] El camino offline→online del móvil no duplica datos (idempotencia extremo a extremo).
- [ ] Las decisiones ADR-01…ADR-10 están reflejadas en el código o documentadas como pendientes.
- [ ] Todas las columnas geográficas usan `geometry(Point,4326)` con índice GiST (PostGIS) y `orders.destination` está poblada en las órdenes de seed.
- [ ] `backend/api/openapi.yaml` cubre el 100 % de los endpoints REST y Swagger UI carga sin errores de validación (ADR-10).
- [ ] Web y móvil consumen los tipos/contratos generados del spec OpenAPI.
- [ ] El README incluye decisiones de arquitectura y auditoría del uso de la IA.
