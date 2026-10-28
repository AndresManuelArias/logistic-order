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
| Portal web | Next.js 16 + React 19 (monorepo `web/`, F4) |
| App móvil | React Native offline-first (F4, pendiente) |

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

### Backend local sin Docker

Para inicializar PostgreSQL + TimescaleDB + PostGIS y ejecutar únicamente el backend en la máquina local:

```bash
cp .env.example .env   # primera vez; ajusta las variables si hace falta
make run-local         # levanta la base de datos y ejecuta go run ./cmd/server
```

El backend quedará disponible en `http://localhost:8080`. Para iniciar solo la base de datos, ejecuta `make db-up`.

Servicios:

| Servicio | URL |
|---|---|
| API | http://localhost:8080 (`/healthz`, `/readyz`) |
| **Swagger (OpenAPI 3.1)** | http://localhost:8080/swagger/index.html |
| Spec OpenAPI | http://localhost:8080/api/v1/openapi.json · `/api/v1/openapi.yaml` |
| Portal web (multi-rol) | http://localhost:3000 |
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
| `STORE` | Auto-registro (alta de tienda en paso propio) | Solo órdenes de su tienda; sin tienda todavía no tiene alcance (403) |
| `DRIVER` | Auto-registro | Solo órdenes asignadas a él |
| Invitado | Token con scope `order:<id>` (48 h) | Solo `GET` de esa orden; nunca lista ni transiciona |

Se expone también `POST /api/v1/auth/guest-token` para que `STORE`/`PROVIDER`
compartan el seguimiento de una orden con el cliente final.

**Endpoints** (contrato completo en Swagger): `POST /auth/register|login|refresh|logout|guest-token`,
`GET /auth/me`, `GET|POST /orders`, `GET /orders/{id}`, `PATCH /orders/{id}/status`,
`POST /orders/{id}/assignment`, `POST /stores`, `GET /stores/me`.

### Alta de tienda (paso propio del registro)

El campo `store` de `POST /auth/register` es **opcional**: la tienda puede nacer
con el usuario o darse de alta después. Solo existe **una tienda por dueño**
(protegida por índice único en BD):

```bash
# 1) Registrarse como tendero (sin tienda todavía)
curl -s -X POST http://localhost:8080/api/v1/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"email":"tendero@ejemplo.com","password":"Segura12345","full_name":"Tendero","role":"STORE"}'

# 2) Login y alta de la tienda (rol STORE)
curl -s -X POST http://localhost:8080/api/v1/stores \
  -H 'Content-Type: application/json' -H "Authorization: Bearer $ACCESS_TOKEN" \
  -d '{"name":"La Esquina","address":"Cra 7 # 72-40","location":{"lat":4.6550,"lon":-74.0632}}'

# 3) Consultar la tienda propia (404 si el alta sigue pendiente)
curl -s http://localhost:8080/api/v1/stores/me -H "Authorization: Bearer $ACCESS_TOKEN"
```

Respuestas: alta repetida → `409`; coordenadas inválidas o `(0,0)` → `422`;
rol distinto de `STORE` → `403`.

## Telemetría y tiempo real (F3)

### Ingesta de posiciones (RF-03, RF-09)

```bash
# Muestra individual. Un DRIVER puede omitir vehicle_id: se deriva de su perfil.
curl -s -X POST http://localhost:8080/api/v1/telemetry/points \
  -H 'Content-Type: application/json' -H "Authorization: Bearer $DRIVER_TOKEN" \
  -d '{"time":"2026-10-28T12:00:00Z","lat":4.6612,"lon":-74.0742,"speed_kph":28.5,"device_seq":1}'

# Bulk Sync offline-first: el header Idempotency-Key es obligatorio (8–128 chars).
curl -s -X POST http://localhost:8080/api/v1/telemetry/bulk \
  -H 'Content-Type: application/json' -H "Authorization: Bearer $DRIVER_TOKEN" \
  -H 'Idempotency-Key: device-8f3a-batch-0001' \
  -d '{"device_id":"device-8f3a","points":[
        {"vehicle_id":"<uuid>","time":"2026-10-28T11:59:00Z","lat":4.65,"lon":-74.07,"device_seq":1},
        {"vehicle_id":"<uuid>","time":"2026-10-28T12:00:00Z","lat":4.66,"lon":-74.08,"device_seq":2}]}'
```

- La PK `(time, vehicle_id, device_seq)` deduplica: repetir una muestra responde
  `201` con `duplicate: true` y **no** reemite el evento.
- El recibo del bulk (`app.sync_receipts`) y las muestras se guardan en **una
  transacción**. Repetir la `Idempotency-Key` devuelve `already_processed: true`
  sin insertar ni republicar; los puntos repetidos de un lote nuevo se informan
  en `duplicates`.
- Si se envía `order_id`, debe existir y estar asignada a ese vehículo (`422`).
- Solo se publican en WebSocket las muestras que la BD confirmó como nuevas.

### Consultas con RBAC

| Endpoint | PROVIDER | DRIVER | STORE | Invitado |
|---|---|---|---|---|
| `GET /api/v1/vehicles/{id}/latest` | su flota | su vehículo | vehículos de sus órdenes | `403` |
| `GET /api/v1/vehicles/{id}/track?from=&to=&limit=` | su flota | su vehículo | vehículos de sus órdenes | `403` |
| `POST /api/v1/telemetry/points` · `/bulk` | su flota | su vehículo | `403` | `403` |

- `track`: `from`/`to` obligatorios (RFC3339), rango máximo 7 días, `limit`
  máximo 5000 (1000 por defecto).
- Un vehículo ajeno o inexistente responde `403` (anti-enumeración).
- Las lecturas pasan por un **circuit breaker** (`CB_FAILURE_THRESHOLD`,
  `CB_TIMEOUT`): si la BD está degradada responden `503 CIRCUIT_OPEN` con
  `Retry-After`.

### Ejemplo que funciona: traza de un vehículo (`track`)

Copia y pega este bloque en la raíz del repo (stack levantado con `make up` y
seed aplicado). Obtiene el token, el `VEHICLE_ID` de `ABC123` y las últimas 2
horas de traza demo:

```bash
# 1) Token de proveedor.
ACCESS_TOKEN=$(curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"proveedor@tiendas.dev","password":"Proveedor123"}' \
  | sed -E 's/.*"access_token":"([^"]+)".*/\1/')

# 2) Un vehículo de su flota (ABC123 lo crea el seed).
VEHICLE_ID=$(docker compose exec -T db psql -U tiendas -d tiendas -tAc \
  "SELECT v.id FROM app.vehicles v JOIN app.users u ON u.id = v.provider_id
   WHERE u.email='proveedor@tiendas.dev' AND v.plate='ABC123'")

# 3) from/to son obligatorios en RFC3339 (aquí, las últimas 2 horas).
FROM=$(date -u -d '-2 hours' +%Y-%m-%dT%H:%M:%SZ)
TO=$(date -u -d '+1 minute' +%Y-%m-%dT%H:%M:%SZ)

curl -s "http://localhost:8080/api/v1/vehicles/$VEHICLE_ID/track?from=$FROM&to=$TO&limit=100" \
  -H "Authorization: Bearer $ACCESS_TOKEN"
```

Respuesta `200` de la corrida de ejemplo (recortada a 2 de los 30 puntos; llegan
**del más reciente al más antiguo**):

```json
{
  "vehicle_id": "e1cf4001-75f0-45fa-94e4-484b2335213f",
  "from": "2026-10-28T17:43:47Z",
  "to": "2026-10-28T19:44:47Z",
  "count": 30,
  "points": [
    {
      "vehicle_id": "e1cf4001-75f0-45fa-94e4-484b2335213f",
      "device_seq": 100,
      "lat": 4.6483,
      "lon": -74.072,
      "speed_kph": 18,
      "accuracy_m": 5.5,
      "heading": 45,
      "source": "DEVICE",
      "time": "2026-10-28T19:15:00Z",
      "received_at": "2026-10-28T19:15:13.742092Z"
    },
    {
      "vehicle_id": "e1cf4001-75f0-45fa-94e4-484b2335213f",
      "device_seq": 101,
      "lat": 4.6485834870838945,
      "lon": -74.07141912212128,
      "speed_kph": 21.5,
      "accuracy_m": 5.5,
      "heading": 48,
      "source": "DEVICE",
      "time": "2026-10-28T19:14:00Z",
      "received_at": "2026-10-28T19:15:13.742092Z"
    }
  ]
}
```

- `count` es el número de puntos devueltos (`limit` 1000 por defecto, 5000
  máximo) y coincide con `len(points)`.
- En macOS reemplaza el cálculo de fechas por
  `FROM=$(date -u -v-2H +%Y-%m-%dT%H:%M:%SZ)` y
  `TO=$(date -u -v+1M +%Y-%m-%dT%H:%M:%SZ)`.
- Si la ventana no tiene muestras → `404`; `from`/`to` mal formados, invertidos
  o con rango > 7 días → `422`; vehículo de otra flota → `403`.
- ¿Vacío porque el seed fue hace más de 2 h? Vuelve a ejecutar `make seed` para
  refrescar la traza demo.

### WebSocket `/ws` (RF-04, ADR-03)

```bash
# Navegadores: token en query (no pueden fijar headers en el handshake).
ws://localhost:8080/ws?access_token=$ACCESS_TOKEN
# Clientes nativos: Authorization: Bearer <token>
```

| Canal | Quién puede suscribirse |
|---|---|
| `fleet:provider:{provider_id}` | `PROVIDER` dueño de esa flota |
| `fleet:store:{store_id}` | `STORE` dueño de esa tienda (vista dual) |
| `order:{order_id}` | `STORE` dueño, `DRIVER` asignado, `PROVIDER` de la flota y el invitado con `scope: order:<id>` |
| `vehicle:{vehicle_id}` | `PROVIDER` de la flota, `DRIVER` asignado, `STORE` con una orden de ese vehículo |

Protocolo JSON (mensajes de cliente ≤ 4 KiB):

```jsonc
// cliente → servidor
{"action":"subscribe","channel":"vehicle:<uuid>"}
{"action":"unsubscribe","channel":"vehicle:<uuid>"}
{"action":"ping"}

// servidor → cliente
{"type":"subscribed","channel":"vehicle:<uuid>"}
{"type":"unsubscribed","channel":"vehicle:<uuid>"}
{"type":"pong"}
{"type":"error","code":"FORBIDDEN_SCOPE","message":"el token no autoriza el canal ..."}
{"type":"event","channel":"vehicle:<uuid>","at":"...","data":{"vehicle_id":"...","lat":4.65,"lon":-74.07,"time":"...","source":"DEVICE"}}
```

- Suscribirse a un canal ajeno devuelve `FORBIDDEN_SCOPE` sin cerrar la conexión.
- Las concesiones se revalidan cada `WS_GRANT_TTL` (60 s por defecto).
- La sesión se cierra con `4401` al expirar el token y con `1008` si el cliente
  no consume su cola (32 canales y cola de 256 por conexión).
- Orígenes permitidos: `WS_ALLOWED_ORIGINS` (coma-separados); vacío = solo
  clientes sin `Origin` (apps nativas) y mismo host.
- Latencia medida en el e2e local: **~4 ms** (presupuesto RNF-04: 500 ms).

## Portal web multi-rol (F4)

El portal vive en [`web/`](web/README.md) como monorepo npm:

- `@tiendas/tracking-core`: tipos generados desde OpenAPI, cliente HTTP con
  refresh single-flight, hook `useTracking` con reconexión/backoff, dedupe y
  estado de conexión.
- `@tiendas/tracking-ui`: mapa SVG accesible (sin proveedor de tiles, R3),
  tarjeta de orden, barra de progreso, badges, estados de carga/error/vacío.
- `apps/portal`: Next.js 16 + React 19 con las vistas por rol y el BFF.

| Vista | Ruta | Rol |
|---|---|---|
| Flota y traza en vivo | `/proveedor` | `PROVIDER` |
| Vista dual entrada/salida, órdenes y enlaces | `/tienda` | `STORE` |
| Órdenes asignadas (respaldo del móvil) | `/conductor` | `DRIVER` |
| Seguimiento público | `/seguimiento/{orderId}#t=…` | Invitado |

La sesión usa access token **en memoria** y refresh token en cookie `httpOnly`
(`path=/api/session`). Las llamadas REST pasan por el proxy same-origin
`/api/backend/api/v1/*`, así que el backend no necesita CORS. El WebSocket se
abre directo contra `NEXT_PUBLIC_WS_URL` con el token en query (R12).

```bash
cd web
npm install
npm run gen:api       # tipos desde backend/api/openapi.yaml
npm run dev           # http://localhost:3000
npm run typecheck && npm run lint && npm test && npm run build
```

Con Docker, `make dev` ya construye el portal (`web/Dockerfile` multi-stage,
`output: standalone`) y lo publica en `http://localhost:3000`.

### Enlace de seguimiento del cliente final

```bash
# 1) Login de la tienda y datos de una orden propia.
STORE_TOKEN=$(curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"tienda1@tiendas.dev","password":"Tienda12345"}' | jq -r .access_token)
ORDER_ID=$(curl -s http://localhost:8080/api/v1/orders -H "Authorization: Bearer $STORE_TOKEN" \
  | jq -r '.items[0].id')

# 2) Emitir token de invitado (48 h por defecto).
GUEST_TOKEN=$(curl -s -X POST http://localhost:8080/api/v1/auth/guest-token \
  -H "Authorization: Bearer $STORE_TOKEN" -H 'Content-Type: application/json' \
  -d "{\"order_id\":\"$ORDER_ID\"}" | jq -r .token)

# 3) Abrir el enlace en el navegador. El token va en el fragmento (#), no en el servidor.
echo "http://localhost:3000/seguimiento/$ORDER_ID#t=$GUEST_TOKEN"
```

El invitado solo puede `GET` esa orden y suscribirse a `order:{orderId}`; la
lista de flota y cualquier otra orden responden `403`.

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
eventos de historial. Además carga una **traza demo de 30 puntos por vehículo**
(una muestra por minuto durante los últimos 30 minutos) para que `latest`,
`track` y el WebSocket tengan datos que dibujar; si el vehículo ya tiene una
traza reciente, no se duplica.

Los vehículos `ABC123`, `DEF456` y `GHI789` quedan con 30 muestras demo cada
uno (más las que ya existieran), y `track` las devuelve de la más reciente a la
más antigua. Para probarlo paso a paso (token, `VEHICLE_ID` y `from`/`to`) usa
el **ejemplo que funciona** de la sección «Telemetría y tiempo real (F3)».

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
make e2e-f2    # e2e F2 contra el stack local (72 casos)
make e2e-f3    # e2e F3: telemetría, RBAC y WebSockets (60 casos)
make test-web  # pruebas del portal + tracking-core + tracking-ui (58)
make lint-web  # typecheck + lint + build de producción del portal
make clean     # baja el stack y BORRA los volúmenes locales
```

### Levantar solo la base de datos

Para desarrollo o pruebas que no requieren el backend completo:

```bash
docker compose up db -d       # levanta solo PostgreSQL + TimescaleDB + PostGIS
docker compose logs db -f     # ver logs de la base de datos
docker compose exec db pg_isready   # verificar que la BD está healthy
```

**Verificación:**

```bash
docker compose ps
# NAME                 IMAGE                           STATUS
# tiendas-track-db-1   timescale/timescaledb-ha:pg16   Up About a minute (healthy)

docker compose exec db psql -U ${POSTGRES_USER:-tiendas} -d ${POSTGRES_DB:-tiendas} -c "\\dt"
# Lista las tablas si la BD está lista
```

> Conéctate a PostgreSQL directamente dentro del contenedor Docker. El nombre
> generado por Compose es `tiendas-track-db-1`, según el proyecto definido en
> `docker-compose.yml`:
>
> ```bash
> docker exec -it tiendas-track-db-1 psql -U tiendas -d tiendas
> ```
>
> Dentro de `psql`, lista los esquemas y las tablas de la aplicación:
>
> ```sql
> \dn                 -- esquemas disponibles
> \dt app.*           -- tablas de la aplicación
> \dt telemetry.*     -- tablas de telemetría
> \dt *.*             -- todas las tablas visibles
> \q                  -- salir
> ```
>
> Para ejecutar una consulta y salir inmediatamente, utiliza `-c`:
>
> ```bash
> docker exec -i tiendas-track-db-1 \
>   psql -U tiendas -d tiendas -c "\dt app.*"
> ```
>
> Como alternativa, Compose encuentra el contenedor sin usar su nombre generado:
>
> ```bash
> docker compose exec db psql -U tiendas -d tiendas
> ```
>
> Si cambiaste las credenciales de `.env`, reemplaza `tiendas` por los valores
> de `POSTGRES_USER` y `POSTGRES_DB` correspondientes.
>
> La aplicación usa los esquemas `app` y `telemetry`; `psql -c "\dt"` solo
> muestra las tablas de `public`, por lo que puede parecer que la base está vacía.

Pruebas end-to-end de F2 (requiere el stack levantado):

```bash
bash scripts/e2e_f2.sh    # 72 casos: auth, RBAC, tiendas, transiciones, invitado, Swagger
```

## Estructura

```
backend/
├── api/openapi.yaml               # spec OpenAPI 3.1 canónica (ADR-10), embebida
├── cmd/server/                    # punto de entrada de la API
├── cmd/seed/                      # datos de demostración (make seed)
├── cmd/wsprobe/                   # sonda de integración WS usada por el e2e F3
├── internal/shared/
│   ├── config/                    # configuración por entorno (JWT, WS, circuit breaker)
│   ├── database/                  # pool, migraciones
│   ├── httperr/                   # sobre de error { error: { code, message } }
│   ├── httpserver/                # router, health checks, montaje de módulos
│   ├── authz/                     # principal, roles, scope de invitado y canales WS
│   ├── token/                     # JWT HS256 (access/refresh/guest)
│   ├── middleware/                # Authenticate, RequireRole
│   ├── eventbus/                  # pub/sub en memoria (Go channels, no bloqueante)
│   ├── resilience/                # circuit breaker (gobreaker) de lecturas
│   ├── websocket/                 # hub /ws: concesiones, fan-out, límites
│   └── openapi/                   # sirve spec + Swagger UI embebidos
├── internal/auth/                 # F2: registro, login, refresh rotativo, guest
├── internal/orders/               # F2: órdenes, máquina de estados, asignación
├── internal/stores/               # F2: alta de tienda (paso propio) y consulta /me
├── internal/telemetry/            # F3: ingesta, bulk idempotente, latest/track, canales
├── internal/seed/                 # datos demo idempotentes (incluye telemetría)
├── internal/aiagent/              # F5
├── migrations/                    # SQL embebido y versionado (0001…0006)
└── pkg/logger/                    # slog estructurado
scripts/e2e_f2.sh                  # e2e de la F2 (72 casos)
scripts/e2e_f3.sh                  # e2e de la F3 (60 casos)
web/                               # portal Next.js + librerías compartidas (F4)
├── apps/portal/                   # rutas, BFF y paneles por rol
├── packages/tracking-core/        # cliente OpenAPI, WS y hook useTracking
└── packages/tracking-ui/          # componentes y estilos compartidos
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
| F3 — Telemetría y WebSockets | ✅ Completada (2026-10-28) |
| F4 — Portal y app móvil | 🟨 **Portal completado (2026-10-28); app móvil pendiente** |
| F5 — Agente IA | ⬜ Pendiente |
| F6 — Carga, IaC y CI/CD | ⬜ Pendiente |

## Problemas comunes

- **`No space left on device`**: Docker necesita espacio en `/var/lib/docker`. Revisa `docker system df` y libera imágenes/caché no usadas.
- **Puerto ocupado**: cambia `BACKEND_PORT`, `WEB_PORT` o `DB_PORT` en `.env`.
- **BD en bucle de reinicio**: revisa `docker compose logs db`; si el volumen quedó corrupto tras un fallo de disco, `make clean` lo recrea (borra datos locales).
