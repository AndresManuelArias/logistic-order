# Tiendas-Track

Módulo de logística, trazabilidad y telemetría en tiempo real multi-rol (Proveedor / Tendero / Cliente final / Conductor) para tiendas de barrio.

- **Plan de desarrollo:** [`doc/plan_desarrollo.md`](doc/plan_desarrollo.md)
- **Modelo de datos:** [`doc/arquitectura/database/modelo_datos.md`](doc/arquitectura/database/modelo_datos.md)
- **Arquitectura:** [`doc/arquitectura/`](doc/arquitectura/)

## Stack

| Componente | Tecnología |
|---|---|
| Backend | Go 1.22 (monolito modular, Clean Architecture) |
| Base de datos | PostgreSQL + TimescaleDB + PostGIS |
| Portal web | Placeholder Nginx (Next.js en F4) |
| App móvil | React Native offline-first (F4) |

## Requisitos

- Docker + Docker Compose v2
- Go 1.22 (solo para desarrollo local sin contenedores)
- Make (opcional)

## Inicio rápido

```bash
cp .env.example .env     # ajusta credenciales locales si hace falta
make dev                 # docker compose up --build -d
make health              # verifica /healthz y /readyz
```

Servicios:

| Servicio | URL |
|---|---|
| API | http://localhost:8080 (`/healthz`, `/readyz`) |
| **Swagger (OpenAPI 3.1)** | http://localhost:8080/swagger/index.html |
| Spec OpenAPI | http://localhost:8080/api/v1/openapi.json · `/api/v1/openapi.yaml` |
| Portal (placeholder) | http://localhost:3000 |
| PostgreSQL | `localhost:5432` (`tiendas` / `tiendas_dev`) |

> Las credenciales por defecto son **solo para desarrollo local**. En cualquier otro entorno se inyectan por entorno/secretos y nunca se versionan.

## Autenticación y RBAC (F2)

```bash
# 1) Login (devuelve access token de 15 min y refresh token rotativo de 30 días)
curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"tienda1@tiendas.dev","password":"Tienda12345"}'

# 2) Usar el access token
curl -s http://localhost:8080/api/v1/orders \
  -H "Authorization: Bearer $ACCESS_TOKEN"

# 3) Rotar el refresh token (el anterior queda revocado)
curl -s -X POST http://localhost:8080/api/v1/auth/refresh \
  -H 'Content-Type: application/json' -d "{\"refresh_token\":\"$REFRESH_TOKEN\"}"
```

| Rol | Quién lo tiene | Alcance de datos (server-side) |
|---|---|---|
| `PROVIDER` | Solo por seed/administración | Su flota: vehículos y órdenes asignadas a ellos |
| `STORE` | Auto-registro (crea su tienda) | Solo órdenes de su tienda |
| `DRIVER` | Auto-registro | Solo órdenes asignadas a él |
| Invitado | Token con scope `order:<id>` (48 h) | Solo `GET` de esa orden; nunca lista ni transiciona |

Se expone también `POST /api/v1/auth/guest-token` para que `STORE`/`PROVIDER`
compartan el seguimiento de una orden con el cliente final.

**Endpoints** (contrato completo en Swagger): `POST /auth/register|login|refresh|logout|guest-token`,
`GET /auth/me`, `GET|POST /orders`, `GET /orders/{id}`, `PATCH /orders/{id}/status`,
`POST /orders/{id}/assignment`.

## Datos de demostración (seed)

```bash
make seed     # idempotente: si ya existen los datos, no hace nada
```

| Rol | Email | Contraseña |
|---|---|---|
| PROVIDER | `proveedor@tiendas.dev` | `Proveedor123` |
| STORE | `tienda1@tiendas.dev` | `Tienda12345` |
| STORE | `tienda2@tiendas.dev` | `Tienda12345` |
| DRIVER | `conductor1@tiendas.dev` | `Conductor123` |
| DRIVER | `conductor2@tiendas.dev` | `Conductor123` |
| DRIVER | `conductor3@tiendas.dev` | `Conductor123` |

Crea 10 órdenes con `origin`/`destination` reales de Bogotá (PostGIS) en
distintos estados (`CREATED`, `IN_TRANSIT`, `DELIVERED`, `CANCELLED`) y sus
eventos de historial.

## Comandos

```bash
make help      # lista de comandos
make dev       # levanta el stack (build + detached)
make down      # detiene y elimina contenedores
make logs      # logs del backend en vivo
make ps        # estado de servicios
make health    # healthz + readyz
make seed      # datos demo idempotentes (ver credenciales arriba)
make swagger   # URL de la documentación OpenAPI
make test      # pruebas unitarias del backend
make build     # compila el backend
make vet       # go vet
make psql      # shell psql en la base de datos
make clean     # baja el stack y BORRA los volúmenes locales
```

Pruebas end-to-end de F2 (requiere el stack levantado):

```bash
bash scripts/e2e_f2.sh    # 60 casos: auth, RBAC, transiciones, invitado, Swagger
```

## Estructura

```
backend/
├── api/openapi.yaml               # spec OpenAPI 3.1 canónica (ADR-10), embebida
├── cmd/server/                    # punto de entrada de la API
├── cmd/seed/                      # datos de demostración (make seed)
├── internal/shared/
│   ├── config/                    # configuración por entorno (JWT_SECRET, TTLs…)
│   ├── database/                  # pool, migraciones
│   ├── httperr/                   # sobre de error { error: { code, message } }
│   ├── httpserver/                # router, health checks, montaje de módulos
│   ├── authz/                     # principal, roles, scope de invitado
│   ├── token/                     # JWT HS256 (access/refresh/guest)
│   ├── middleware/                # Authenticate, RequireRole
│   ├── openapi/                   # sirve spec + Swagger UI embebidos
│   └── websocket/                 # (F3)
├── internal/auth/                 # F2: registro, login, refresh rotativo, guest
├── internal/orders/               # F2: órdenes, máquina de estados, asignación
├── internal/seed/                 # datos demo idempotentes
├── internal/{telemetry,aiagent}/  # (F3, F5)
├── migrations/                    # SQL embebido y versionado (0001…0005)
└── pkg/logger/                    # slog estructurado
scripts/e2e_f2.sh                  # e2e de la F2 (60 casos)
web/                               # placeholder del portal (F4)
doc/                               # plan, arquitectura, modelo de datos
```

## Base de datos

Las migraciones se aplican automáticamente al arrancar el backend (`MIGRATE_ON_STARTUP=true`) y quedan registradas en `telemetry.schema_migrations`.

- Extensiones: `postgis`, `timescaledb`
- `app.*`: usuarios, tiendas, vehículos, conductores, órdenes, eventos, tokens, sync, auditoría IA
- `telemetry.points`: hypertable PostGIS (retención 90 días)
- `telemetry.stops`: continuous aggregate (refresco cada 1 min)
- `telemetry.stop_intervals`: vista de paradas reales

## Estado de las fases

| Fase | Estado |
|---|---|
| F1 — Esqueleto + Docker Compose | ✅ Completada (2026-10-28) |
| F2 — Auth, RBAC y órdenes | ✅ Completada (2026-10-28) |
| F3 — Telemetría y WebSockets | ⬜ Pendiente |
| F4 — Portal y app móvil | ⬜ Pendiente |
| F5 — Agente IA | ⬜ Pendiente |
| F6 — Carga, IaC y CI/CD | ⬜ Pendiente |

## Problemas comunes

- **`No space left on device`**: Docker necesita espacio en `/var/lib/docker`. Revisa `docker system df` y libera imágenes/caché no usadas.
- **Puerto ocupado**: cambia `BACKEND_PORT`, `WEB_PORT` o `DB_PORT` en `.env`.
- **BD en bucle de reinicio**: revisa `docker compose logs db`; si el volumen quedó corrupto tras un fallo de disco, `make clean` lo recrea (borra datos locales).
