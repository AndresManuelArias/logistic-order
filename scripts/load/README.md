# Pruebas de carga F6 (k6)

Escenario de carga de Tiendas-Track para validar RNF-04 (entrega en tiempo real)
y RNF-06 (100+ vehículos concurrentes). Mide ingesta GPS, duplicados
idempotentes, errores inyectados y latencia de entrega WebSocket.

## Precondiciones

1. Stack local levantado y migrado:
   ```bash
   make db-up
   ```
2. Backend en `:8082` (para no chocar con un servidor en `:8080`) con el rate
   limiting de telemetría **desactivado** para medir capacidad:
   ```bash
   cd backend
   HTTP_ADDR=:8082 RATE_LIMIT_TELEMETRY_PER_MIN=0 RATE_LIMIT_LOGIN_PER_MIN=0 \
     go run ./cmd/server
   ```
   Los límites por defecto (120/min y 10/min) son de producción; con 100 VUs
   la prueba recibiría `429` y no mediría la ingesta. El limitador se valida en
   sus pruebas unitarias (`go test ./internal/shared/ratelimit/...`).
3. Flota suficiente. Con 3 vehículos demo se reutilizan, pero para 100+
   vehículos distintos siembra flota dedicada:
   ```bash
   make seed-load          # equivale a scripts/load/seed_load_vehicles.sh 100
   scripts/load/seed_load_vehicles.sh 150
   ```
   El script es idempotente (placas `LOAD-###`) y no toca datos existentes.
   Limpieza: `scripts/load/seed_load_vehicles.sh --clean`.

## Ejecución

Con Docker (no requiere k6 instalado):

```bash
docker run --rm --network host -i grafana/k6 run - \
  < scripts/load/k6_f6.js
```

Parámetros por entorno:

```bash
docker run --rm --network host -i \
  -e BASE_URL=http://localhost:8082 \
  -e VUS=100 -e DURATION=2m \
  -e WS_VUS=20 -e WS_DURATION=2m \
  -e AUTH_EMAIL=proveedor@tiendas.dev \
  -e AUTH_PASSWORD=Proveedor123 \
  grafana/k6 run - < scripts/load/k6_f6.js
```

Smoke test corto (validación rápida del script y del entorno):

```bash
docker run --rm --network host -i \
  -e VUS=5 -e DURATION=10s -e WS_VUS=2 -e WS_DURATION=10s \
  grafana/k6 run - < scripts/load/k6_f6.js
```

> En macOS/Windows, `--network host` no permite llegar a `localhost` del host.
> Usa `BASE_URL=http://host.docker.internal:8082` y publica el puerto.

## Qué mide

- **ingest** (`constant-vus`): cada VU reporta un punto GPS por iteración.
  - 5% reenvía **exactamente** el mismo payload `(time, vehicle_id,
    device_seq)`: el servidor debe responder `201` con `duplicate: true`.
  - 1% inyecta `lat: 999`: debe responder `422`, nunca `5xx`.
- **ws**: suscribe `vehicle:<id>`, inyecta un punto y mide el tiempo hasta el
  evento `event` del canal (`ws_event_latency`).

## Umbrales (criterio de aceptación)

| Métrica | Umbral | Requisito |
|---|---|---|
| `http_req_duration{kind:ingest_valid}` p95 | < 500 ms | RNF-06 |
| `ws_event_latency` p95 | < 500 ms | RNF-04 |
| `unexpected_5xx` rate | < 1% | resiliencia |
| `checks{kind:duplicate}` rate | > 95% | idempotencia |
| `checks{kind:invalid}` rate | > 95% | validación de borde |

Un 429 por rate limiting cuenta como respuesta no exitosa y no como 5xx; si
aparece, revisa la configuración del backend antes de interpretar el p95.

## Limitaciones

- La carga se genera desde una sola máquina: el resultado mezcla coste del
  cliente k6 y del backend. Para capacidad real, distribuir con k6 Cloud o
  varios runners.
- El token se comparte entre VUs (un solo proveedor); es intencional para
  medir el camino autenticado. Un despliegue con rate limit por usuario
  requiere desactivarlo o sembrar credenciales por VU.
- `ws_event_latency` mide desde el `subscribed` hasta el evento del mismo
  vehículo; no valida orden ni pérdida de eventos bajo reconexión.
- La base de datos local (TimescaleDB en Docker) no representa el rendimiento
  de producción; los umbrales validan el diseño, no capacity planning.
