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
| Portal (placeholder) | http://localhost:3000 |
| PostgreSQL | `localhost:5432` (`tiendas` / `tiendas_dev`) |

> Las credenciales por defecto son **solo para desarrollo local**. En cualquier otro entorno se inyectan por entorno/secretos y nunca se versionan.

## Comandos

```bash
make help      # lista de comandos
make dev       # levanta el stack (build + detached)
make down      # detiene y elimina contenedores
make logs      # logs del backend en vivo
make ps        # estado de servicios
make health    # healthz + readyz
make test      # pruebas unitarias del backend
make build     # compila el backend
make vet       # go vet
make psql      # shell psql en la base de datos
make clean     # baja el stack y BORRA los volúmenes locales
```

## Estructura

```
backend/
├── cmd/server/                  # punto de entrada
├── internal/shared/
│   ├── config/                  # configuración por entorno
│   ├── database/                # pool, migraciones
│   ├── httpserver/              # router, health checks
│   └── websocket/               # (F3)
├── internal/{auth,orders,telemetry,aiagent}/   # (F2, F3, F5)
├── migrations/                  # SQL embebido y versionado
└── pkg/logger/                  # slog estructurado
web/                             # placeholder del portal (F4)
doc/                             # plan, arquitectura, modelo de datos
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
| F2 — Auth, RBAC y órdenes | ⬜ Pendiente |
| F3 — Telemetría y WebSockets | ⬜ Pendiente |
| F4 — Portal y app móvil | ⬜ Pendiente |
| F5 — Agente IA | ⬜ Pendiente |
| F6 — Carga, IaC y CI/CD | ⬜ Pendiente |

## Problemas comunes

- **`No space left on device`**: Docker necesita espacio en `/var/lib/docker`. Revisa `docker system df` y libera imágenes/caché no usadas.
- **Puerto ocupado**: cambia `BACKEND_PORT`, `WEB_PORT` o `DB_PORT` en `.env`.
- **BD en bucle de reinicio**: revisa `docker compose logs db`; si el volumen quedó corrupto tras un fallo de disco, `make clean` lo recrea (borra datos locales).
