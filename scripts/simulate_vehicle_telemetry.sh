#!/usr/bin/env bash
# Envía ubicaciones simuladas para TODOS los vehículos de la flota del proveedor.
# Cada vehículo corre en un subproceso paralelo con un retraso de 2 s entre puntos.
# Requiere backend, base de datos y usuario seed levantados.
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
EMAIL="${EMAIL:-proveedor@tiendas.dev}"
PASSWORD="${PASSWORD:-Proveedor123}"
COUNT="${COUNT:-12}"
DELAY_SECONDS=2   # fijo: 2 segundos entre cada punto enviado

json_value() {
  python3 -c 'import json,sys; print(json.load(sys.stdin)[sys.argv[1]])' "$1"
}

command -v curl    >/dev/null || { echo "Falta curl"    >&2; exit 1; }
command -v python3 >/dev/null || { echo "Falta python3" >&2; exit 1; }

# ── Login ──────────────────────────────────────────────────────────────────────
login_file=$(mktemp)
trap 'rm -f "$login_file"' EXIT

login_status=$(curl -sS -o "$login_file" -w '%{http_code}' \
  -X POST "$BASE_URL/api/v1/auth/login" \
  -H 'Content-Type: application/json' \
  -d "{\"email\":\"$EMAIL\",\"password\":\"$PASSWORD\"}")

if [[ "$login_status" != "200" ]]; then
  echo "No se pudo iniciar sesión como proveedor (HTTP $login_status)." >&2
  cat "$login_file" >&2
  exit 1
fi

access_token=$(json_value access_token < "$login_file")

# ── Obtener todos los vehículos de la flota ───────────────────────────────────
vehicles_file=$(mktemp)
trap 'rm -f "$login_file" "$vehicles_file"' EXIT

vehicles_status=$(curl -sS -o "$vehicles_file" -w '%{http_code}' \
  "$BASE_URL/api/v1/vehicles?limit=200" \
  -H "Authorization: Bearer $access_token")

if [[ "$vehicles_status" != "200" ]]; then
  echo "No se pudo consultar la flota (HTTP $vehicles_status)." >&2
  cat "$vehicles_file" >&2
  exit 1
fi

# Extrae pares "plate:id" para todos los vehículos de la respuesta.
mapfile -t vehicle_pairs < <(python3 - "$vehicles_file" <<'PYEOF'
import json, sys

with open(sys.argv[1]) as f:
    page = json.load(f)

items = page.get("items", []) if isinstance(page, dict) else page
if not items:
    print("La flota no tiene vehículos.", file=sys.stderr)
    sys.exit(1)

for v in items:
    print(f"{v['plate']}:{v['id']}")
PYEOF
)

if [[ ${#vehicle_pairs[@]} -eq 0 ]]; then
  echo "La flota del proveedor no tiene vehículos." >&2
  exit 1
fi

echo "Flota encontrada: ${#vehicle_pairs[@]} vehículo(s)"
for pair in "${vehicle_pairs[@]}"; do
  echo "  • ${pair%%:*}"
done
echo "Puntos por vehículo: $COUNT  |  Delay entre puntos: ${DELAY_SECONDS}s"
echo "Abre http://localhost:3000/proveedor para ver los eventos en vivo."
echo "─────────────────────────────────────────────────────────────────"

# ── Función de simulación por vehículo (corre en subproceso) ─────────────────
simulate_vehicle() {
  local plate="$1"
  local vehicle_id="$2"
  local token="$3"
  local count="$4"
  local delay="$5"

  # Punto de inicio: cada vehículo arranca en una coordenada levemente distinta
  # para que las trazas no se superpongan en el mapa.
  local idx="$6"   # índice 0-based dentro de la flota
  local start_lat
  local start_lon
  start_lat=$(python3 -c "print(f'{4.6382 + $idx * 0.0120:.6f}')")
  start_lon=$(python3 -c "print(f'{-74.0838 + $idx * 0.0090:.6f}')")

  local response_file
  response_file=$(mktemp)
  # shellcheck disable=SC2064
  trap "rm -f '$response_file'" EXIT

  for ((i = 0; i < count; i++)); do
    local lat lon time seq heading status
    lat=$(python3 -c "print(f'{$start_lat + $i * 0.000045:.6f}')")
    lon=$(python3 -c "print(f'{$start_lon + $i * 0.000025:.6f}')")
    time=$(date -u +%Y-%m-%dT%H:%M:%SZ)
    # device_seq único por vehículo: base en el índice de flota para evitar
    # colisiones si dos vehículos emiten el mismo segundo.
    seq=$(( 3000000 + idx * 100000 + i ))
    heading=$(( (45 + i * 5) % 360 ))

    status=$(curl -sS -o "$response_file" -w '%{http_code}' \
      -X POST "$BASE_URL/api/v1/telemetry/points" \
      -H 'Content-Type: application/json' \
      -H "Authorization: Bearer $token" \
      -d "{\"vehicle_id\":\"$vehicle_id\",\"time\":\"$time\",\"lat\":$lat,\"lon\":$lon,\"speed_kph\":32.5,\"heading\":$heading,\"device_seq\":$seq}")

    if [[ "$status" == "201" ]]; then
      echo "[$plate] [$((i + 1))/$count] $time  lat=$lat lon=$lon"
    else
      echo "[$plate] [$((i + 1))/$count] HTTP $status: $(cat "$response_file")" >&2
    fi

    if (( i < count - 1 )); then
      sleep "$delay"
    fi
  done

  echo "[$plate] Simulación completada ($count puntos enviados)."
}

export -f simulate_vehicle

# ── Lanzar un subproceso por vehículo ────────────────────────────────────────
pids=()
for idx in "${!vehicle_pairs[@]}"; do
  pair="${vehicle_pairs[$idx]}"
  plate="${pair%%:*}"
  vid="${pair##*:}"
  simulate_vehicle "$plate" "$vid" "$access_token" "$COUNT" "$DELAY_SECONDS" "$idx" &
  pids+=($!)
done

# Esperar a que todos los subprocesos terminen.
failed=0
for pid in "${pids[@]}"; do
  if ! wait "$pid"; then
    failed=$(( failed + 1 ))
  fi
done

echo "─────────────────────────────────────────────────────────────────"
if (( failed > 0 )); then
  echo "Simulación terminada con $failed error(s). Revisa la salida de arriba." >&2
  exit 1
else
  echo "Simulación completada. Se enviaron $COUNT puntos por cada uno de ${#vehicle_pairs[@]} vehículo(s)."
fi
