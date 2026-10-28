#!/usr/bin/env bash
# E2E F3 — Telemetría, WebSockets y RBAC en tiempo real (RF-03, RF-04, RF-09).
# Requiere el stack levantado (docker compose up -d db backend) y el seed cargado.
# Sale con código != 0 si algún caso falla. Salida: .diag/e2e_f3.txt
set -u
BASE="${BASE:-http://localhost:8080}"
WS="${WS:-ws://localhost:8080/ws}"
FAIL=0
J=.diag/jars_f3; mkdir -p "$J"

say() { printf '%s\n' "$*"; }
chk() { # chk <nombre> <esperado> <obtenido>
  if [ "$2" = "$3" ]; then say "OK   $1 -> $3"; else say "FAIL $1 -> esperado $2, obtenido $3"; FAIL=1; fi
}
has() { # has <nombre> <fichero> <texto>
  if grep -q -- "$3" "$2" 2>/dev/null; then say "OK   $1"; else say "FAIL $1 -> no contiene $3"; FAIL=1; fi
  }
post()        { curl -sS -o "$2" -w '%{http_code}' -X POST "$BASE$1" -H 'Content-Type: application/json' -d "$3"; }
# postAuthTo <path> <salida> <body> <token>
postAuthTo()  { curl -sS -o "$2" -w '%{http_code}' -X POST "$BASE$1" -H 'Content-Type: application/json' -H "Authorization: Bearer $4" -d "$3"; }
get()      { curl -sS -o "$2" -w '%{http_code}' -X GET "$BASE$1" -H "Authorization: Bearer ${3:-}"; }
jqr()      { grep -o "\"$2\"[^,}]*" "$1" | head -1 | sed 's/.*: *"\?//; s/"$//'; }
psqlq()    { docker compose exec -T db psql -U tiendas -d tiendas -tA -v ON_ERROR_STOP=1 -c "$1"; }

# Sonda WS (paquete main separado del servidor).
wsexpect() { # wsexpect <nombre> <texto-esperado> <args...>
  local name="$1" want="$2"; shift 2
  local rc
  ( cd backend && GOTOOLCHAIN=local go run ./cmd/wsprobe "$@" ) > "$J/ws.out" 2>&1
  rc=$?
  if [ "$rc" -eq 0 ] && grep -q -- "$want" "$J/ws.out"; then
    say "OK   $name -> $(head -1 "$J/ws.out")"
  else
    say "FAIL $name -> rc=$rc salida=$(head -1 "$J/ws.out")"; FAIL=1
  fi
}

say "=== F3 E2E: $(date -Is) ==="

# --- 0. Stack y datos base -------------------------------------------------
if ! curl -fsS -o /dev/null "$BASE/healthz"; then
  say "FAIL backend no responde en $BASE"; exit 1
fi

cleanup_e2e() {
  docker compose exec -T db psql -U tiendas -d tiendas -q <<'SQL' >/dev/null 2>&1 || true
-- Solo datos creados por la suite (device_seq >= 900000, claves e2e-f3-*).
DELETE FROM telemetry.points WHERE device_seq >= 900000;
DELETE FROM app.sync_receipts WHERE idempotency_key LIKE 'e2e-f3-%';
DELETE FROM app.vehicles WHERE plate = 'F3X999';
DELETE FROM app.users WHERE email = 'provf3@tiendas.dev';
SQL
}
cleanup_e2e

# Segundo proveedor con vehículo propio: permite probar el 403 real entre
# flotas (un proveedor no puede ingerir ni leer vehículos ajenos).
docker compose exec -T db psql -U tiendas -d tiendas -q <<'SQL' >/dev/null 2>&1 || true
INSERT INTO app.users (email, password_hash, full_name, role)
SELECT 'provf3@tiendas.dev', '$2y$10$0e0XjVW1bT6k3r6yJ0m0YOKk5o7Xn4Bq2v1q1J8p0S9m8k5l4d6iS', 'Proveedor F3', 'PROVIDER'
WHERE NOT EXISTS (SELECT 1 FROM app.users WHERE email = 'provf3@tiendas.dev');
INSERT INTO app.vehicles (plate, provider_id, status)
SELECT 'F3X999', u.id, 'ACTIVE' FROM app.users u
WHERE u.email = 'provf3@tiendas.dev'
  AND NOT EXISTS (SELECT 1 FROM app.vehicles v WHERE v.plate = 'F3X999');
SQL

DRV1_VEH=$(psqlq "SELECT d.vehicle_id::text FROM app.drivers d JOIN app.users u ON u.id = d.user_id WHERE u.email='conductor1@tiendas.dev'")
DRV2_VEH=$(psqlq "SELECT d.vehicle_id::text FROM app.drivers d JOIN app.users u ON u.id = d.user_id WHERE u.email='conductor2@tiendas.dev'")
FOREIGN_VEH=$(psqlq "SELECT id::text FROM app.vehicles WHERE plate='F3X999'")
ORDER_DRV1=$(psqlq "SELECT id::text FROM app.orders WHERE assigned_vehicle_id='$DRV1_VEH'::uuid AND status='IN_TRANSIT' ORDER BY placed_at DESC LIMIT 1")
ORDER_OTHER=$(psqlq "SELECT id::text FROM app.orders WHERE assigned_vehicle_id='$DRV2_VEH'::uuid ORDER BY placed_at DESC LIMIT 1")
[ -n "$DRV1_VEH" ] && [ -n "$DRV2_VEH" ] && [ -n "$FOREIGN_VEH" ] && [ -n "$ORDER_DRV1" ] && [ -n "$ORDER_OTHER" ] \
  && say "OK   datos base (vehículos y órdenes)" \
  || { say "FAIL faltan datos base: ¿ejecutaste make seed?"; exit 1; }

# --- 1. Contrato OpenAPI (ADR-10) -----------------------------------------
curl -sS "$BASE/api/v1/openapi.json" -o "$J/openapi.json"
for p in /api/v1/telemetry/points /api/v1/telemetry/bulk "/api/v1/vehicles/{id}/latest" "/api/v1/vehicles/{id}/track" "/ws" x-websocket; do
  has "openapi documenta $p" "$J/openapi.json" "$p"
done

# --- 2. Tokens -------------------------------------------------------------
post /api/v1/auth/login "$J/login_prov.json" '{"email":"proveedor@tiendas.dev","password":"Proveedor123"}' >/dev/null
PROV_AT=$(jqr "$J/login_prov.json" access_token)
post /api/v1/auth/login "$J/login_drv1.json" '{"email":"conductor1@tiendas.dev","password":"Conductor123"}' >/dev/null
DRV1_AT=$(jqr "$J/login_drv1.json" access_token)
post /api/v1/auth/login "$J/login_drv2.json" '{"email":"conductor2@tiendas.dev","password":"Conductor123"}' >/dev/null
DRV2_AT=$(jqr "$J/login_drv2.json" access_token)
post /api/v1/auth/login "$J/login_store.json" '{"email":"tienda1@tiendas.dev","password":"Tienda12345"}' >/dev/null
STORE_AT=$(jqr "$J/login_store.json" access_token)
[ -n "$PROV_AT" ] && [ -n "$DRV1_AT" ] && [ -n "$DRV2_AT" ] && [ -n "$STORE_AT" ] \
  && say "OK   tokens de proveedor, conductores y tienda" \
  || { say "FAIL no se pudieron emitir tokens"; exit 1; }

chk "guest-token de la tienda sobre su orden" 201 \
  "$(postAuthTo /api/v1/auth/guest-token "$J/guest.json" "{\"order_id\":\"$ORDER_DRV1\"}" "$STORE_AT")"
GUEST_AT=$(jqr "$J/guest.json" token)
[ -n "$GUEST_AT" ] && say "OK   token de invitado emitido" || { say "FAIL sin token de invitado"; FAIL=1; }

# --- 3. Ingesta individual (RF-03, RBAC) ----------------------------------
TS=$(date -u -d '-1 minute' +%Y-%m-%dT%H:%M:%SZ)
point_body() { # point_body <vehicle> <seq> [order]
  local order_json=""
  [ -n "${3:-}" ] && order_json=",\"order_id\":\"$3\""
  printf '{"vehicle_id":"%s"%s,"time":"%s","lat":4.6612,"lon":-74.0742,"speed_kph":28.5,"heading":90,"device_seq":%s}' \
    "$1" "$order_json" "$TS" "$2"
}
POINT_DRV1=$(point_body "$DRV1_VEH" 900001 "$ORDER_DRV1")

chk "points sin token -> 401" 401 \
  "$(curl -sS -o "$J/p_no.json" -w '%{http_code}' -X POST "$BASE/api/v1/telemetry/points" -H 'Content-Type: application/json' -d "$POINT_DRV1")"
chk "points rol STORE -> 403" 403 \
  "$(postAuthTo /api/v1/telemetry/points "$J/p_store.json" "$POINT_DRV1" "$STORE_AT")"
chk "points DRIVER vehículo ajeno -> 403" 403 \
  "$(postAuthTo /api/v1/telemetry/points "$J/p_other.json" "$(point_body "$DRV2_VEH" 900002)" "$DRV1_AT")"
chk "points PROVIDER flota ajena -> 403" 403 \
  "$(postAuthTo /api/v1/telemetry/points "$J/p_foreign.json" "$(point_body "$FOREIGN_VEH" 900003)" "$PROV_AT")"
chk "points lat fuera de rango -> 422" 422 \
  "$(postAuthTo /api/v1/telemetry/points "$J/p_lat.json" '{"time":"'"$TS"'","lat":91,"lon":-74.07,"device_seq":900004}' "$DRV1_AT")"
chk "points orden no asignada al vehículo -> 422" 422 \
  "$(postAuthTo /api/v1/telemetry/points "$J/p_order.json" "$(point_body "$DRV1_VEH" 900005 "$ORDER_OTHER")" "$DRV1_AT")"

chk "points DRIVER propio vehículo -> 201" 201 \
  "$(postAuthTo /api/v1/telemetry/points "$J/p_ok.json" "$POINT_DRV1" "$DRV1_AT")"
has "points respuesta con duplicate=false" "$J/p_ok.json" '"duplicate":false'
chk "points reintento idempotente -> 201" 201 \
  "$(postAuthTo /api/v1/telemetry/points "$J/p_dup.json" "$POINT_DRV1" "$DRV1_AT")"
has "points reintento duplicate=true" "$J/p_dup.json" '"duplicate":true'

chk "points DRIVER sin vehicle_id (derivado) -> 201" 201 \
  "$(postAuthTo /api/v1/telemetry/points "$J/p_derived.json" '{"time":"'"$TS"'","lat":4.6620,"lon":-74.0730,"device_seq":900006}' "$DRV1_AT")"
has "points derivado al vehículo del conductor" "$J/p_derived.json" "$DRV1_VEH"
chk "points PROVIDER su flota -> 201" 201 \
  "$(postAuthTo /api/v1/telemetry/points "$J/p_prov.json" "$(point_body "$DRV1_VEH" 900007)" "$PROV_AT")"
chk "points invitado -> 403" 403 \
  "$(curl -sS -o "$J/p_guest.json" -w '%{http_code}' -X POST "$BASE/api/v1/telemetry/points" \
      -H 'Content-Type: application/json' -H "Authorization: Bearer $GUEST_AT" -d "$POINT_DRV1")"

# --- 4. Bulk Sync idempotente (RF-09) -------------------------------------
bulk_body() { # bulk_body <seq-base> <n>
  local base="$1" n="$2" out=""
  for i in $(seq 1 "$n"); do
    [ "$i" -gt 1 ] && out="$out,"
    out="$out{\"vehicle_id\":\"$DRV1_VEH\",\"time\":\"$(date -u -d "-$((i+2)) minute" +%Y-%m-%dT%H:%M:%SZ)\",\"lat\":4.66$i,\"lon\":-74.07$i,\"device_seq\":$((base+i))}"
  done
  printf '{"device_id":"e2e-device","points":[%s]}' "$out"
}
BULK1=$(bulk_body 900100 3)

chk "bulk sin Idempotency-Key -> 422" 422 \
  "$(postAuthTo /api/v1/telemetry/bulk "$J/b_no_key.json" "$BULK1" "$DRV1_AT")"
chk "bulk STORE -> 403" 403 \
  "$(curl -sS -o "$J/b_store.json" -w '%{http_code}' -X POST "$BASE/api/v1/telemetry/bulk" \
      -H 'Content-Type: application/json' -H 'Idempotency-Key: e2e-f3-bulk-store' \
      -H "Authorization: Bearer $STORE_AT" -d "$BULK1")"
chk "bulk punto de otra flota -> 403" 403 \
  "$(curl -sS -o "$J/b_foreign.json" -w '%{http_code}' -X POST "$BASE/api/v1/telemetry/bulk" \
      -H 'Content-Type: application/json' -H 'Idempotency-Key: e2e-f3-bulk-foreign' \
      -H "Authorization: Bearer $DRV1_AT" \
      -d '{"device_id":"e2e-device","points":[{"vehicle_id":"'"$DRV2_VEH"'","time":"'"$TS"'","lat":4.6,"lon":-74.0,"device_seq":900200}]}')"
chk "bulk OK -> 200" 200 \
  "$(curl -sS -o "$J/b_ok.json" -w '%{http_code}' -X POST "$BASE/api/v1/telemetry/bulk" \
      -H 'Content-Type: application/json' -H 'Idempotency-Key: e2e-f3-bulk-001' \
      -H "Authorization: Bearer $DRV1_AT" -d "$BULK1")"
has "bulk accepted=3" "$J/b_ok.json" '"accepted":3'
has "bulk no está ya procesado" "$J/b_ok.json" '"already_processed":false'
chk "bulk mismo Idempotency-Key -> 200" 200 \
  "$(curl -sS -o "$J/b_replay.json" -w '%{http_code}' -X POST "$BASE/api/v1/telemetry/bulk" \
      -H 'Content-Type: application/json' -H 'Idempotency-Key: e2e-f3-bulk-001' \
      -H "Authorization: Bearer $DRV1_AT" -d "$BULK1")"
has "bulk replay ya procesado" "$J/b_replay.json" '"already_processed":true'
has "bulk replay accepted=0" "$J/b_replay.json" '"accepted":0'

chk "bulk nuevo key con puntos repetidos -> 200" 200 \
  "$(curl -sS -o "$J/b_partial.json" -w '%{http_code}' -X POST "$BASE/api/v1/telemetry/bulk" \
      -H 'Content-Type: application/json' -H 'Idempotency-Key: e2e-f3-bulk-002' \
      -H "Authorization: Bearer $DRV1_AT" -d "$BULK1")"
has "bulk duplicados contabilizados" "$J/b_partial.json" '"duplicates":3'

python3 - "$TS" "$DRV1_VEH" > "$J/bulk_big.json" <<'PY'
import json, sys
ts, veh = sys.argv[1], sys.argv[2]
print(json.dumps({"device_id": "e2e-device", "points": [
    {"vehicle_id": veh, "time": ts, "lat": 4.65, "lon": -74.07, "device_seq": 910000 + i}
    for i in range(1001)
]}))
PY
chk "bulk > 1000 puntos -> 422" 422 \
  "$(curl -sS -o "$J/b_big.json" -w '%{http_code}' -X POST "$BASE/api/v1/telemetry/bulk" \
      -H 'Content-Type: application/json' -H 'Idempotency-Key: e2e-f3-bulk-big' \
      -H "Authorization: Bearer $DRV1_AT" --data-binary @"$J/bulk_big.json")"

# --- 5. Lecturas con RBAC (latest / track) --------------------------------
chk "latest sin token -> 401" 401 "$(get "/api/v1/vehicles/$DRV1_VEH/latest" "$J/l_no.json" "")"
chk "latest PROVIDER su flota -> 200" 200 "$(get "/api/v1/vehicles/$DRV1_VEH/latest" "$J/l_prov.json" "$PROV_AT")"
chk "latest DRIVER su vehículo -> 200" 200 "$(get "/api/v1/vehicles/$DRV1_VEH/latest" "$J/l_drv.json" "$DRV1_AT")"
chk "latest DRIVER vehículo ajeno -> 403" 403 "$(get "/api/v1/vehicles/$DRV2_VEH/latest" "$J/l_drv2.json" "$DRV1_AT")"
chk "latest STORE vehículo de su orden -> 200" 200 "$(get "/api/v1/vehicles/$DRV1_VEH/latest" "$J/l_store.json" "$STORE_AT")"
chk "latest STORE vehículo ajeno -> 403" 403 "$(get "/api/v1/vehicles/$DRV2_VEH/latest" "$J/l_store2.json" "$STORE_AT")"
chk "latest invitado -> 403" 403 "$(get "/api/v1/vehicles/$DRV1_VEH/latest" "$J/l_guest.json" "$GUEST_AT")"
chk "latest PROVIDER flota ajena -> 403" 403 "$(get "/api/v1/vehicles/$FOREIGN_VEH/latest" "$J/l_foreign.json" "$PROV_AT")"
chk "latest vehículo inexistente -> 403 (anti-enumeración)" 403 \
  "$(get "/api/v1/vehicles/00000000-0000-4000-8000-000000000000/latest" "$J/l_404.json" "$DRV1_AT")"

FROM=$(date -u -d '-2 hours' +%Y-%m-%dT%H:%M:%SZ)
TO=$(date -u -d '+1 minute' +%Y-%m-%dT%H:%M:%SZ)
chk "track sin from/to -> 422" 422 "$(get "/api/v1/vehicles/$DRV1_VEH/track" "$J/t_no.json" "$DRV1_AT")"
chk "track rango invertido -> 422" 422 "$(get "/api/v1/vehicles/$DRV1_VEH/track?from=$TO&to=$FROM" "$J/t_bad.json" "$DRV1_AT")"
chk "track rango > 7 días -> 422" 422 \
  "$(get "/api/v1/vehicles/$DRV1_VEH/track?from=2026-01-01T00:00:00Z&to=2026-02-01T00:00:00Z" "$J/t_big.json" "$DRV1_AT")"
chk "track DRIVER su vehículo -> 200" 200 \
  "$(get "/api/v1/vehicles/$DRV1_VEH/track?from=$FROM&to=$TO&limit=50" "$J/t_ok.json" "$DRV1_AT")"
has "track devuelve puntos" "$J/t_ok.json" '"count":'
chk "track DRIVER vehículo ajeno -> 403" 403 \
  "$(get "/api/v1/vehicles/$DRV2_VEH/track?from=$FROM&to=$TO" "$J/t_deny.json" "$DRV1_AT")"

# --- 6. WebSocket (RF-04, ADR-03) -----------------------------------------
chk "ws sin token -> 401" 401 \
  "$(curl -sS -o /dev/null -w '%{http_code}' -H 'Connection: Upgrade' -H 'Upgrade: websocket' \
      -H 'Sec-WebSocket-Version: 13' -H 'Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==' "$BASE/ws")"

wsexpect "ws suscripción válida a vehículo propio" "subscribed channel=vehicle" \
  -url "$WS" -token "$DRV1_AT" -channel "vehicle:$DRV1_VEH"
wsexpect "ws token por query (navegadores)" "subscribed channel=order" \
  -url "$WS" -token "$STORE_AT" -token-in-query -channel "order:$ORDER_DRV1"
wsexpect "ws canal inválido" "expect-error=INVALID_CHANNEL" \
  -url "$WS" -token "$DRV1_AT" -channel "order:no-es-uuid" -expect-error INVALID_CHANNEL
wsexpect "ws DRIVER canal de vehículo ajeno -> FORBIDDEN_SCOPE" "expect-error=FORBIDDEN_SCOPE" \
  -url "$WS" -token "$DRV1_AT" -channel "vehicle:$DRV2_VEH" -expect-error FORBIDDEN_SCOPE
wsexpect "ws PROVIDER flota ajena -> FORBIDDEN_SCOPE" "expect-error=FORBIDDEN_SCOPE" \
  -url "$WS" -token "$PROV_AT" -channel "fleet:provider:$(psqlq "SELECT id::text FROM app.users WHERE email='provf3@tiendas.dev'")" \
  -expect-error FORBIDDEN_SCOPE
wsexpect "ws invitado solo su orden" "subscribed channel=order" \
  -url "$WS" -token "$GUEST_AT" -channel "order:$ORDER_DRV1"
wsexpect "ws invitado orden ajena -> FORBIDDEN_SCOPE" "expect-error=FORBIDDEN_SCOPE" \
  -url "$WS" -token "$GUEST_AT" -channel "order:$ORDER_OTHER" -expect-error FORBIDDEN_SCOPE

# Latencia de publicación: el probe se suscribe, ingesta y mide hasta el evento.
TS_WS=$(date -u +%Y-%m-%dT%H:%M:%SZ)
WS_BODY='{"vehicle_id":"'"$DRV1_VEH"'","time":"'"$TS_WS"'","lat":4.6630,"lon":-74.0720,"speed_kph":31,"device_seq":900300}'
wsexpect "ws evento en tiempo real (RNF-04 < 500 ms)" "latency_ms=" \
  -url "$WS" -token "$PROV_AT" -channel "vehicle:$DRV1_VEH" \
  -ingest-url "$BASE/api/v1/telemetry/points" -ingest-token "$PROV_AT" -ingest-body "$WS_BODY"
grep -o 'latency_ms=[0-9.]*' "$J/ws.out" | head -1 | sed 's/^/INFO latencia WS /'

# --- 7. Limpieza ----------------------------------------------------------
cleanup_e2e

say "=== F3 E2E finalizado: $(date -Is) ==="
if [ "$FAIL" -eq 0 ]; then say "RESULTADO: OK"; else say "RESULTADO: FALLOS"; fi
exit "$FAIL"
