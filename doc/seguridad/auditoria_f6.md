# Auditoría de seguridad — F6

Fecha: 2026-10-29
Alcance: backend Go, contrato OpenAPI, portal web, app móvil, Docker, Terraform y CI/CD.
Checklist de referencia: `doc/plan_desarrollo.md` §9.4.

## Metodología

- Revisión de código de los caminos de autenticación, autorización y datos.
- Pruebas: `go vet`, `go test -race`, `govulncheck`, `npm audit`, Spectral y
  `terraform validate/plan`.
- Clasificación: **Crítico/Alto/Medio/Bajo/Informativo**; estado **Corregido**,
  **Mitigado**, **Aceptado** o **Abierto**.
- No se inventan hallazgos: cada uno referencia archivo y comportamiento
  verificable. Los supuestos se marcan como tales.

## Resumen

| # | Severidad | Hallazgo | Estado |
|---|---|---|---|
| 1 | Alta | Sin rate limiting en login, telemetría e IA | Corregido (F6) |
| 2 | Alta | Fuga de alcance STORE en `get_stopped_vehicles` | Corregido (F5) |
| 3 | Alta | `PasswordHash` expuesto en el DTO de autenticación | Corregido (F4) |
| 4 | Media | Token de invitado con TTL 48 h y sin revocación efectiva | Aceptado con recomendación |
| 5 | Media | Rate limiting en memoria: el límite se multiplica por réplica | Aceptado con recomendación |
| 6 | Media | Dependencias Go vulnerables (pgx, jwt, x/text) | Corregido (F6) |
| 7 | Media | 15 vulnerabilidades npm en dependencias transitivas de Expo | Abierto (upstream) |
| 8 | Baja | Contrato OpenAPI no válido para Spectral | Corregido (F6) |
| 9 | Baja | `access_token` de WebSocket en query string | Aceptado con recomendación |
| 10 | Informativo | Estado de Terraform contiene la contraseña de RDS | Aceptado con recomendación |

## Hallazgos

### 1. Ausencia de rate limiting (Alta) — Corregido

**Evidencia previa:** no existía ningún limitador; `backend/internal/shared/middleware/`
solo contenía autenticación/RBAC. Login, `POST /api/v1/telemetry/points`,
`/bulk` y `/api/v1/ai/query` aceptaban peticiones ilimitadas.

**Impacto:** fuerza bruta de credenciales, agotamiento de CPU/BD y coste
descontrolado del proveedor LLM.

**Corrección (F6):**
- Nuevo paquete `backend/internal/shared/ratelimit` (token bucket thread-safe,
  sin dependencias, con recolección de claves inactivas).
- Middleware `rateLimit` en `backend/internal/shared/httpserver/router.go`:
  - login por IP,
  - telemetría e IA por `principal.UserID` (fallback IP),
  - respuesta `429 RATE_LIMITED` con `Retry-After`.
- Configuración `RATE_LIMIT_*` en `backend/internal/shared/config/config.go`,
  `.env.example`, `docker-compose.yml` y Terraform.
- Pruebas: `internal/shared/ratelimit/limiter_test.go` y
  `internal/shared/httpserver/ratelimit_test.go`.
- Contrato: respuesta reutilizable `TooManyRequests` documentada en login,
  points, bulk y ai/query.

**Verificación:** `go test ./internal/shared/ratelimit/... ./internal/shared/httpserver/...`
y prueba de contrato `TestSpecRateLimitDocumentado`.

**Residual:** ver hallazgo 5.

### 2. Fuga de alcance en el agente IA para rol STORE (Alta) — Corregido

**Evidencia:** `get_stopped_vehicles` filtraba paradas por vehículo y ámbito de
proveedor, pero no comprobaba que la parada perteneciera a una orden de la
tienda cuando el principal era `STORE`. Corregido en
`backend/internal/aiagent/infrastructure/postgres/analytics_repo.go` con el
`EXISTS` sobre `app.orders` de la tienda.

**Impacto:** un tendero podía inferir paradas de vehículos ajenos.

**Verificación:** pruebas del repositorio/usecase del agente y E2E F5 con SQL
real; ver `doc/arquitectura/monolito_modular/f5_agente_ia.md`.

### 3. `PasswordHash` en la respuesta de autenticación (Alta) — Corregido

**Evidencia:** `domain.User` incluye `PasswordHash` y el par de tokens lo
transportaba; el DTO HTTP (`toTokenPairResponse`) lo proyecta a campos públicos.
La regresión está cubierta por
`backend/internal/auth/delivery/http_test.go::TestToTokenPairResponseMatchesContract`,
que falla si aparece el hash o los nombres de campo del dominio.

**Impacto evitado:** compromiso de credenciales almacenadas.

**Recomendación adicional:** añadir `json:"-"` a `PasswordHash` en el dominio
como defensa en profundidad si algún día se serializa directamente. No se aplica
ahora porque el dominio no se expone y el test ya lo protege; el cambio es
trivial y de bajo riesgo.

### 4. Token de invitado: TTL 48 h y revocación no efectiva (Media) — Aceptado

**Evidencia:** `GUEST_TOKEN_TTL` por defecto 48 h
(`backend/internal/shared/config/config.go`). El repositorio
`app.guest_tokens` guarda el hash y expone `Revoke`
(`backend/internal/auth/domain/repository.go`), pero **no hay endpoint que lo
invoque** ni el middleware consulta el estado del token: la validación es JWT
stateless.

**Impacto:** un enlace de seguimiento filtrado es válido hasta 48 h y no puede
revocarse antes de expirar. El scope está limitado a `order:<id>` y las rutas
permitidas son de solo lectura, lo que reduce el impacto.

**Recomendación (fuera del alcance F6):**
1. Bajar `GUEST_TOKEN_TTL` a 2–8 h.
2. Añadir `POST /api/v1/auth/guest-token/revoke` (propietario de la orden) y
   consultar `revoked_at` en el middleware de autenticación (implica acceso a BD
   por request o una caché).
3. Rotar el token al cambiar el estado de la orden a `DELIVERED`.

### 5. Rate limiting en memoria (Media) — Aceptado

**Evidencia:** `backend/internal/shared/ratelimit/limiter.go` mantiene los
cubos en el proceso. Con `desired_count = 2` en ECS, el límite efectivo de login
es ~2× el configurado.

**Impacto:** un atacante distribuido puede aprovechar el margen entre réplicas.

**Recomendación:** regla rate-based en AWS WAF delante del ALB (preferida, sin
estado compartido en la app) o un backend Redis. Cualquiera de las dos requiere
una decisión arquitectónica; no se introduce un almacén distribuido sin ADR.

### 6. Dependencias Go vulnerables (Media) — Corregido

**Evidencia (`govulncheck` antes):**
- `github.com/jackc/pgx/v5 v5.5.5` → GO-2026-5004 (inyección SQL por confusión
  de placeholders), corregido en v5.9.2.
- `github.com/golang-jwt/jwt/v5 v5.2.1` → GO-2025-3553 (consumo excesivo de
  memoria al parsear cabeceras), corregido en v5.2.2.
- `golang.org/x/text v0.14.0` → GO-2026-6629 y GO-2026-5970, corregido en v0.41.0.

**Corrección:** actualizadas las tres dependencias; `go.mod` pasa a Go 1.25
(requisito de pgx v5.9.2), Dockerfile a `golang:1.25-alpine` y CI/Makefile a
`GOTOOLCHAIN=auto`.

**Verificación:** `govulncheck ./...` → *No vulnerabilities found* (0 en código
llamado). Quedan 22 vulnerabilidades en módulos requeridos no alcanzables por el
código; se revisarán al actualizar dependencias.

**Residual:** revisar `npm audit` del portal y de la app móvil en cada PR.

### 7. Vulnerabilidades npm en la app móvil (Media) — Abierto

**Evidencia:** `npm audit --omit=dev` en `mobile/` reporta 15 vulnerabilidades
(8 low, 5 moderate, 2 high), todas en `postcss` transitivo de Expo. El fix
automático propone `expo@51`, un downgrade incompatible con el SDK 54 actual.

**Impacto:** `postcss` procesa CSS en build, no en runtime del conductor; el
riesgo de explotación en producción es bajo. No se aplica el downgrade porque
rompería la app.

**Recomendación:** seguir actualizando Expo cuando publique una versión con
`postcss` corregido; el job `security` del CI lo reporta sin bloquear.

**Verificación:** `cd web && npm audit --omit=dev` → 0 vulnerabilidades.

### 8. Contrato OpenAPI no válido para Spectral (Baja) — Corregido

**Evidencia:** `examples` del header `Idempotency-Key` era un array (inválido
para OpenAPI 3.1) y la regla `operation-success-response` no reconoce `101` en
`/ws`. Corregido en `backend/api/openapi.yaml` y `.spectral.yaml`.

**Verificación:** `scripts/contract/check_openapi.sh` → *Contrato OpenAPI válido*.

### 9. Token de WebSocket en query string (Baja) — Aceptado

**Evidencia:** `backend/internal/telemetry/delivery/http.go` acepta
`?access_token=` para navegadores que no pueden fijar cabeceras en el handshake.
El logger del backend registra `path` sin query
(`backend/internal/shared/httpserver/router.go`), pero los access logs del ALB
o de un proxy sí pueden registrar la URL completa.

**Mitigación existente:** access tokens de 15 min y sesión WS cerrada al expirar.

**Recomendación:** desactivar query-string logging en el ALB o usar un ticket WS
de un solo uso (endpoint que emite un token efímero). Requiere cambio de
contrato; no se hace en F6.

### 10. Estado de Terraform con secretos (Informativo) — Aceptado

**Evidencia:** `infra/terraform/main.tf` construye `DATABASE_URL` con
`db_password`; ambos quedan en el state. Las variables están marcadas
`sensitive`, lo que evita mostrarlas en consola, pero no las cifra en el state.

**Recomendación:** backend S3 con cifrado y bloqueo (plantilla en
`infra/terraform/versions.tf`), bucket restringido, y rotación de la contraseña.
Alternativa: `manage_master_user_password` de RDS y actualizar el secreto de la
aplicación fuera de Terraform.

## Checklist §9.4

| Control | Estado | Evidencia |
|---|---|---|
| Autorización server-side en endpoints | ✅ | `middleware.RequireRole` + filtros de scope en usecases; pruebas E2E F2/F3 |
| Autorización en suscripciones WS | ✅ | `Hub` autoriza cada canal con el principal; canal ajeno → error, sin eventos |
| Validación de entrada en el borde | ✅ | DTOs y `Validate` de dominio; límites de tamaño de body |
| Catálogo cerrado en el agente IA | ✅ | 4 herramientas; sin SQL libre ni ejecución de texto del modelo |
| Rate limiting login/telemetría/IA | ✅ | Hallazgo 1 |
| Tokens de invitado: scope mínimo | ✅ | `order:<id>`, solo lectura |
| Tokens de invitado: expiración corta | ⚠️ | 48 h; ver hallazgo 4 |
| Tokens de invitado: revocables | ⚠️ | Repositorio con `Revoke`, sin endpoint ni comprobación; hallazgo 4 |
| Auditoría de accesos | ✅ | Logs estructurados con `trace_id`, método, ruta, estado y latencia |
| Auditoría de consultas IA | ✅ | `app.ai_query_log` (pregunta, herramientas, argumentos, estado, modelo, latencia) |
| No exponer datos entre roles | ✅ | Proyecciones DTO + scope; regresión `PasswordHash` y hallazgo 2 |
| Secretos fuera del repo | ✅ | `.env` ignorado, JWT ≥ 32 caracteres, Secrets Manager en Terraform |
| Contenedores sin root | ✅ | Backend `USER app`; web `USER nextjs` |
| Dependencias escaneadas | ✅ | `govulncheck` 0; `npm audit` web 0, móvil con residual upstream |

## Riesgos residuales

1. Revocación de tokens de invitado (hallazgo 4).
2. Límite por réplica (hallazgo 5) hasta añadir WAF/Redis.
3. `postcss` transitivo de Expo (hallazgo 7).
4. Sin WAF, sin mTLS interno y sin cifrado de extremo a extremo más allá de TLS
   del ALB; fuera del alcance mínimo de F6.
5. Sin pruebas de penetración externas ni revisión de configuración de AWS con
   credenciales reales (el entorno no las tiene).
