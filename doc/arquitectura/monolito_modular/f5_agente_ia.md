# F5 — Agente IA operativo + resiliencia transversal

**Estado:** diseño aprobado para implementación (2026-10-29)
**Requisitos:** RF-10 (consultas en lenguaje natural), RNF-05 (circuit breakers)
**Decisiones relacionadas:** ADR-01 (monolito modular), ADR-07 (capa de herramientas), ADR-09 (PostGIS), ADR-10 (OpenAPI)
**Módulo:** `backend/internal/aiagent`

---

## 1. Alcance y límites

El agente interpreta preguntas en lenguaje natural y las traduce a un **catálogo
fijo de herramientas parametrizadas**. El modelo **no escribe SQL** ni ejecuta
acciones: solo elige herramienta y argumentos. El servidor valida argumentos,
aplica el scope del token y ejecuta SQL predefinido.

Fuera de alcance: escritura de datos, acciones sobre órdenes, memoria
conversacional entre peticiones, RAG documental y ejecución de código.

## 2. Estructura del módulo

```
internal/aiagent/
├── domain/                  # Tipos puros: mensajes, herramientas, auditoría, puertos
├── usecase/                 # Orquestador, registro de herramientas, prompt, auditoría
├── infrastructure/
│   ├── llm/                 # Adaptador HTTP compatible con OpenAI /chat/completions
│   └── postgres/            # Consultas analíticas scopeadas + auditoría
└── delivery/                # POST /api/v1/ai/query
```

Dependencias permitidas:

- `delivery → usecase → domain`.
- `infrastructure/* → domain` y paquetes compartidos (`authz`, `resilience`).
- `main` compone todo; ningún módulo de negocio importa `aiagent`.

El módulo **no** importa `telemetry`, `orders` ni sus adaptadores. Consulta las
vistas y tablas con su propio repositorio de solo lectura, como ya preveía la
arquitectura (`AIAgentModule --> DB : Consultas Analíticas de Flotas`).

## 3. Catálogo de herramientas

| Herramienta | Argumentos | Semántica | Scope |
|---|---|---|---|
| `get_stopped_vehicles` | `minutes` int 1–1440 (def. 20), `area` string 0–80 | Vehículos con parada **vigente**: último bucket detenido hace ≤ 3 min y duración ≥ `minutes`. `area` filtra por nombre o dirección de la tienda de la orden asociada a la parada. | Vehículos del proveedor / tienda / conductor |
| `get_order_status` | `order_id` uuid | Estado, hitos (`placed_at`, `picked_up_at`, `delivered_at`), tienda, placa y conductor. Si no existe o está fuera de scope: mismo error (anti-enumeración). | Órdenes del ámbito |
| `get_fleet_eta` | `store_id` uuid opcional | Órdenes `IN_TRANSIT` con última posición ≤ 30 min. ETA = distancia en línea recta desde la última posición al destino / velocidad (última muestra > 0; si no, 30 km/h). | Flota/órdenes del ámbito; `store_id` acota al proveedor |
| `get_late_deliveries` | `hours` int 1–720 (def. 24) | Órdenes `CREATED` o `IN_TRANSIT` con `placed_at` anterior a `now() - hours` (pendientes vencidas). | Órdenes del ámbito |

Todas devuelven como máximo 50 filas y orden estable. Los argumentos se decodifican
con `DisallowUnknownFields`, rangos validados y UUID validado antes de tocar la BD.

### SQL y scope

El scope se aplica **siempre en SQL** con tres parámetros (`provider_id`,
`store_id`, `driver_id`); un parámetro vacío se convierte en `NULL` y no coincide.
Es el mismo criterio que `telemetry/infrastructure/postgres/fleet_repo.go`:

- `PROVIDER`: `v.provider_id = $1` o, en órdenes, `v.provider_id = $1`.
- `STORE`: vehículos con órdenes de la tienda; órdenes `o.store_id = $2`.
- `DRIVER`: vehículo del conductor; órdenes `o.assigned_driver_id = $3`.

`get_stopped_vehicles` reutiliza `telemetry.stop_intervals` (agregado continuo
`telemetry.stops`). Para `STORE`, además de limitar los vehículos, exige que la
orden asociada a la parada pertenezca a la tienda: no se expone una parada
actual de otra tienda por una entrega histórica. `get_fleet_eta` usa
`ST_Distance(geography, geography)`.

## 4. Endpoint

`POST /api/v1/ai/query` — roles `PROVIDER`, `STORE`, `DRIVER`; invitados `403`.

```json
{ "question": "¿Qué vehículos llevan más de 20 minutos detenidos?" }
```

```json
{
  "answer": "ABC123 lleva 27 minutos detenido en la tienda La Esquina.",
  "tool": "get_stopped_vehicles",
  "tools": [
    {
      "name": "get_stopped_vehicles",
      "arguments": { "minutes": 20 },
      "ok": true,
      "duration_ms": 8
    }
  ],
  "model": "gpt-4o-mini",
  "audit_id": "0e0f..."
}
```

Validación: `question` recortada, 1–500 caracteres, cuerpo máximo 8 KiB.

| Situación | HTTP | Código |
|---|---|---|
| Pregunta vacía o inválida | 422 | `INVALID_QUESTION` |
| Invitado o rol sin scope | 403 | `FORBIDDEN_SCOPE` |
| Sin `AI_API_KEY` / `AI_ENABLED=false` | 503 | `AI_NOT_CONFIGURED` |
| Circuito abierto | 503 | `CIRCUIT_OPEN` + `Retry-After` |
| Proveedor LLM 4xx/5xx o respuesta ilegible | 502 | `AI_PROVIDER_ERROR` |
| Timeout del proveedor | 504 | `AI_TIMEOUT` |
| Sin respuesta final tras N pasos | 502 | `AI_INVALID_RESPONSE` |
| Error inesperado | 500 | `INTERNAL` |

## 5. Bucle del agente

1. Validar pregunta y resolver `Scope` desde el `authz.Principal`.
2. Construir mensajes `system` + `user`. El prompt prohíbe inventar datos y
   obliga a usar herramientas para cifras operativas.
3. Llamar al LLM a través del **circuit breaker** (`ai-llm`) con timeout global
   `AI_TIMEOUT`.
4. Si hay `tool_calls`: validar y ejecutar cada una (máx. 4 por paso), añadir
   mensajes `tool` con el JSON de resultado o `{"error": "..."}` y volver al paso 3.
5. Si hay contenido final: devolver respuesta.
6. Tras `AI_MAX_STEPS` (def. 4) sin contenido: `AI_INVALID_RESPONSE`.
7. Auditar **siempre** (éxito o error) con `context.WithoutCancel` y timeout de 2 s.
   Un fallo de auditoría no rompe la respuesta: se registra un warning.

Un error de herramienta no aborta la consulta: se comunica al modelo para que
responda con lo disponible o indique que no pudo obtener el dato.

## 6. Auditoría

Migración `0007_ai_audit.sql` amplía `app.ai_query_log` (ya existente):

| Columna | Tipo | Uso |
|---|---|---|
| `status` | `VARCHAR(16)` | `OK` / `ERROR` |
| `error_code` | `VARCHAR(64)` | Código estable del fallo |
| `answer` | `TEXT` | Respuesta final (truncada a 4000 caracteres) |
| `model` | `VARCHAR(80)` | Modelo usado |
| `tools` | `JSONB` | Lista de invocaciones con argumentos, resultado y duración |

Se conservan `question`, `tool`, `params` y `latency_ms`. **No** se guardan el
prompt de sistema, la API key ni cabeceras del proveedor.

## 7. Configuración

| Variable | Default | Notas |
|---|---|---|
| `AI_ENABLED` | `true` | Si es `false` → 503 controlado |
| `AI_BASE_URL` | `https://api.openai.com/v1` | Compatible con OpenAI, Groq, Ollama y vLLM |
| `AI_API_KEY` | vacío | Secreto; vacío deshabilita el agente (503) |
| `AI_MODEL` | `gpt-4o-mini` | Modelo con soporte de function calling |
| `AI_TIMEOUT` | `20s` | Timeout global de la consulta |
| `AI_MAX_STEPS` | `4` | 1–8 pasos de tool-calling |

El circuit breaker del LLM reutiliza `CB_FAILURE_THRESHOLD` y `CB_TIMEOUT`.

## 8. Seguridad

- El modelo nunca recibe SQL ni credenciales; solo definiciones de herramientas.
- El scope se resuelve server-side y se aplica en SQL; una herramienta no puede
  ampliar el ámbito del token.
- `get_order_status` responde igual para “no existe” y “fuera de scope”.
- Las respuestas de error del proveedor se truncan y no se propagan al cliente.
- La auditoría no almacena secretos; sí pregunta y respuesta (datos operativos).
- Límites: 500 caracteres de pregunta, 8 KiB de cuerpo, 4 pasos, 4 herramientas
  por paso, 50 filas por consulta, 800 tokens de salida.

## 9. Pruebas

- Unitarias del orquestador con `ChatModel` falso: herramienta correcta, args
  inválidos, herramienta desconocida, error de repositorio, error de proveedor,
  circuito abierto, máximo de pasos y auditoría en éxito/fallo.
- Unitarias del registro: defaults, rangos, UUID, `area`.
- Unitarias del adaptador LLM contra `httptest` (request/response, 5xx, JSON roto).
- Unitarias del handler: 200, 422, 403, 502, 503, 504 y `Retry-After`.
- Contrato: `openapi_test.go` exige `POST /api/v1/ai/query`.
- Criterio de aceptación sin API key: `503 AI_NOT_CONFIGURED`, nunca 500.
  Con proveedor real o mock: la pregunta de vehículos detenidos devuelve filas
  reales y `tool = get_stopped_vehicles`.

## 10. Supuestos y riesgos

- **`area`**: no hay requisito de geocercas; se interpreta como fragmento del
  nombre/dirección de la tienda asociada a la parada. Si se necesita un radio,
  se añadirá un parámetro explícito en una revisión posterior.
- **ETA**: distancia en línea recta y velocidad de crucero; no considera tráfico
  ni ruta real. Es una estimación declarada, no una promesa.
- **Retraso**: “pendiente vencida” (> `hours` sin entregar); no hay SLA definido
  en los requisitos.
- **Parada vigente**: depende del refresco del agregado continuo (1 min). El
  seed refresca el agregado tras insertar la traza demo.
- **Sin fallback determinista**: sin LLM se devuelve 503; se prioriza no simular
  inteligencia ni respuestas no auditables. El resto del sistema sigue operativo.
- **Coste/latencia**: cada consulta puede consumir hasta 4 llamadas al LLM. No
  hay rate limiting por usuario en F5; se anota para F6.

## 11. Evidencia de validación (2026-10-29)

- Migración `0007_ai_audit` aplicada por el migrador real sobre TimescaleDB
  (`telemetry.schema_migrations` incluye `0007_ai_audit`).
- Las cuatro consultas analíticas se ejecutaron contra datos reales del seed:
  `get_stopped_vehicles` devolvió `DEF456` con 24–25 minutos de parada asociada
  a «Tienda El Mercado»; `get_fleet_eta` devolvió `PED-1007`/`PED-1009` con
  distancia y velocidad; `get_late_deliveries` devolvió tres órdenes pendientes
  (42–46 h). El scope `STORE` solo devolvió la parada de su propia tienda.
- Consulta de aceptación end-to-end con un proveedor LLM simulado compatible
  con OpenAI (sin SDK): `POST /api/v1/ai/query` respondió `200` con
  `tool=get_stopped_vehicles`, `tools[0].ok=true` y `audit_id`; el registro en
  `app.ai_query_log` quedó en `status=OK` con `tools` JSONB.
- Sin `AI_API_KEY`, el mismo endpoint respondió `503 AI_NOT_CONFIGURED` (nunca
  500) y auditó `status=ERROR`, `error_code=AI_NOT_CONFIGURED`.
- `go vet ./...`, `go test ./...` y las 58 pruebas web están en verde.
