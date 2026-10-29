# Portal web Tiendas-Track (F4)

Monorepo npm con el portal multi-rol en Next.js y dos librerías compartidas. El
objetivo es que el portal y, en el futuro, la app móvil reutilicen los mismos
contratos y la misma lógica de tiempo real.

## Estructura

```
web/
├── apps/portal/                 # Next.js 16 (App Router) + React 19
│   ├── src/app/
│   │   ├── (app)/               # Rutas autenticadas: /proveedor, /tienda, /conductor
│   │   ├── api/session/         # BFF: login, refresh, logout (cookie httpOnly)
│   │   ├── api/backend/[...path] # Proxy same-origin hacia la API Go (sin CORS)
│   │   ├── login/               # Formulario de acceso
│   │   └── seguimiento/[orderId] # Vista pública de invitado
│   ├── src/components/          # Sesión, guardas, paneles por rol y formularios
│   └── src/hooks/               # Carga de datos y suscripción WS atada a sesión
├── packages/tracking-core/      # Dominio, cliente OpenAPI, WS con backoff, hooks
└── packages/tracking-ui/        # Componentes presentacionales + estilos tt-*
```

### Separación de responsabilidades

| Capa | Dónde | Regla |
|---|---|---|
| Dominio | `tracking-core/src/domain` | Tipos y reglas sin red ni React |
| Aplicación | `tracking-core/src/application` | Cliente de tracking, backoff, dedupe y descarte de eventos obsoletos |
| Infraestructura | `tracking-core/src/api` | Cliente HTTP tipado y generación desde OpenAPI |
| Presentación | `tracking-ui` | Componentes sin sesión ni acceso a red; acciones inyectadas por el portal |
| Composición | `apps/portal` | Rutas, BFF, formularios y decisiones por rol |

`tracking-ui` no conoce roles ni tokens: recibe datos y callbacks. Los paneles del
portal deciden qué acciones mostrar según el rol, pero **el backend siempre
revalida** el permiso y el alcance.

## Requisitos

- Node.js 22+ (probado con 24)
- npm 10+ (workspaces)
- Backend Tiendas-Track en `http://localhost:8080` (`make dev` en la raíz)

## Puesta en marcha

```bash
cd web
npm install
npm run gen:api     # regenera tipos desde ../backend/api/openapi.yaml
npm run dev         # http://localhost:3000
```

Variables de entorno del portal:

| Variable | Por defecto | Uso |
|---|---|---|
| `BACKEND_URL` | `http://localhost:8080` | URL interna que usa el BFF (servidor) |
| `NEXT_PUBLIC_WS_URL` | `ws://localhost:8080/ws` | WebSocket que abre el navegador |

`NEXT_PUBLIC_*` se incrusta en el bundle durante el build; cámbiala antes de
compilar si el backend no está en `localhost:8080`.

### Credenciales de demostración

| Rol | Email | Contraseña |
|---|---|---|
| Proveedor | `proveedor@tiendas.dev` | `Proveedor123` |
| Tendero | `tienda1@tiendas.dev` | `Tienda12345` |
| Conductor | `conductor1@tiendas.dev` | `Conductor123` |

## Diseño de sesión

1. `POST /api/session/login` valida contra Go y devuelve el **access token** al
   navegador. El **refresh token** queda en una cookie `httpOnly`, `SameSite=Lax`
   y con `path=/api/session`, fuera del alcance de JavaScript.
2. El access token vive **solo en memoria**; nunca en `localStorage`.
3. Al recargar, el portal llama a `POST /api/session/refresh` una única vez
   (single-flight incluso con Strict Mode) y reconstruye la sesión.
4. El cliente HTTP reintenta un `401` renovando el token; si la rotación falla,
   limpia la sesión y redirige al login con `?expired=1`.
5. Todas las llamadas REST pasan por `/api/backend/api/v1/*`. El proxy solo
   reenvía el prefijo `api/v1/`, valida los segmentos (defensa SSRF), reenvía
   `authorization` e `idempotency-key` y no cachea respuestas.
6. `POST /api/session/logout` revoca el refresh token y borra la cookie.

Decisiones asociadas: ADR-04 (JWT + refresh rotativo) y ADR-10 (contrato
OpenAPI como fuente de tipos).

## Rutas y RBAC

| Ruta | Rol | Contenido |
|---|---|---|
| `/` | Público | Portada con acceso por rol |
| `/login` | Público | Acceso; respeta `?next` solo dentro del panel del rol |
| `/proveedor` | `PROVIDER` | Flota, mapa en vivo, traza de 6 h, KPIs y órdenes de la flota |
| `/tienda` | `STORE` | Vista dual entrada/salida, alta de tienda, creación de órdenes, transiciones y enlace de invitado |
| `/conductor` | `DRIVER` | Vista de respaldo: órdenes asignadas y transiciones permitidas |
| `/seguimiento/{orderId}#t={token}` | Invitado | Solo esa orden; sin cuenta ni acceso a flota |

`RequireRole` evita mostrar paneles ajenos, pero es una mejora de UX: la
autorización efectiva está en cada endpoint y en cada suscripción WebSocket.

### Supuestos de la vista dual del tendero

El contrato no define una bandera "entrada/salida". Se usa:

- **Entrada:** órdenes en `CREATED` (pendientes de despacho).
- **Salida:** órdenes en `IN_TRANSIT`, `DELIVERED` o `CANCELLED`.

Está documentado como supuesto y se puede cambiar sin tocar `tracking-ui`.

## Tiempo real

`useTracking` (en `tracking-core`) encapsula:

- conexión a `NEXT_PUBLIC_WS_URL` con el token en query (R12: los navegadores no
  pueden fijar `Authorization` en el handshake);
- reconexión con backoff exponencial y jitter, estado `connecting/open/reconnecting/closed`;
- deduplicación por `vehicle_id + time + device_seq` y descarte de eventos más
  antiguos que la última posición conocida;
- cierre limpio al desmontar o cambiar de canal/token;
- cierre `4401` → callback de sesión expirada para renovar o volver al login.

Canales usados:

| Vista | Canal |
|---|---|
| Proveedor | `fleet:provider:{user.id}` |
| Tienda | `fleet:store:{store.id}` |
| Invitado | `order:{orderId}` |

## Vista pública de invitado

- El enlace se genera desde la tienda con `POST /api/v1/auth/guest-token`.
- El token viaja en el **fragmento** (`#t=…`), que el navegador no envía al
  servidor del portal ni a logs de acceso.
- Se lee con `useSyncExternalStore` (reacciona a `hashchange` sin renders en
  cascada) y solo se usa como bearer contra la API.
- La página no incluye el token en HTML ni en la URL del servidor.
- Un enlace inválido, vencido o revocado muestra un mensaje y no inventa datos.

## Mapa

No hay proveedor de tiles definido (riesgo R3). `TrackingMap` dibuja un SVG con
proyección equirectangular corregida por latitud y una **lista accesible
alternativa** con los mismos marcadores y selección por teclado. El color nunca
es el único portador de información: cada estado lleva texto.

## Datos y contrato

```bash
npm run gen:api     # openapi-typescript desde backend/api/openapi.yaml
npm run check:api   # falla si el archivo generado está desactualizado
```

No se escriben DTOs a mano: los tipos públicos salen de `openapi.ts` y se
re-exportan con alias en `tracking-core/src/api/types.ts`. `AiAnswer` se declara
en `tracking-core/src/api/client.ts` con la forma del contrato F5
(`answer`, `tool`, `tools`, `model`, `audit_id`).

## Calidad

```bash
npm run typecheck   # los 3 workspaces
npm run lint        # ESLint flat config de Next 16
npm test            # 58 pruebas (core + UI + hooks y BFF del portal)
npm run build       # build de producción (standalone)
```

Cobertura de pruebas actual:

- `tracking-core`: 18 pruebas (cliente HTTP, refresh single-flight, backoff,
  dedupe, eventos obsoletos, cierre de sesión, canales).
- `tracking-ui`: 26 pruebas (tarjeta, progreso, pestañas con teclado, mapa y
  lista accesible, estados de carga/error/vacío, badges, conexión).
- Portal: 9 pruebas (parser del token de invitado y `useApiResource`:
  cancelación, recarga, error y respuesta obsoleta).

## Docker

La imagen usa build multi-stage y `output: standalone`:

```bash
docker compose build web
docker compose up -d web
# http://localhost:3000
```

El contexto de build es `web/` (raíz de workspaces). El contenedor recibe
`BACKEND_URL=http://backend:8080`; el WebSocket público se fija como build arg
`NEXT_PUBLIC_WS_URL` (por defecto `ws://localhost:8080/ws`).

## Limitaciones conocidas

- El panel de IA llama a `POST /api/v1/ai/query` (F5). Muestra la respuesta y
  las herramientas consultadas; ante `503 AI_NOT_CONFIGURED` deshabilita el
  formulario con un mensaje explícito, y ante `CIRCUIT_OPEN`, `AI_TIMEOUT` o
  `AI_PROVIDER_ERROR` informa sin inventar datos.
- No hay endpoint de listado de conductores: la asignación vehículo-conductor no
  se inventa en la UI.
- La operación de campo del conductor (GPS en segundo plano, cola offline) es
  responsabilidad de la app móvil; `/conductor` es una vista de respaldo.
- El token de demo del seed caduca en 15 minutos; el refresh dura 30 días.
