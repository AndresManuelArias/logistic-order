#!/usr/bin/env bash
# Envía ubicaciones simuladas para ABC123 y permite observarlas en el WebSocket del portal.
# Requiere backend, base de datos y usuario seed levantados.
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
EMAIL="${EMAIL:-proveedor@tiendas.dev}"
PASSWORD="${PASSWORD:-Proveedor123}"
PLATE="${PLATE:-ABC123}"
COUNT="${COUNT:-12}"
INTERVAL_SECONDS="${INTERVAL_SECONDS:-2}"

json_value() {
  python3 -c 'import json,sys; print(json.load(sys.stdin)[sys.argv[1]])' "$1"
}

command -v curl >/dev/null || { echo "Falta curl" >&2; exit 1; }
command -v python3 >/dev/null || { echo "Falta python3" >&2; exit 1; }

login_file=$(mktemp)
vehicles_file=$(mktemp)
response_file=$(mktemp)
trap 'rm -f "$login_file" "$vehicles_file" "$response_file"' EXIT

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
vehicles_status=$(curl -sS -o "$vehicles_file" -w '%{http_code}' \
  "$BASE_URL/api/v1/vehicles?limit=200" \
  -H "Authorization: Bearer $access_token")
if [[ "$vehicles_status" != "200" ]]; then
  echo "No se pudo consultar la flota (HTTP $vehicles_status)." >&2
  exit 1
fi

vehicle_id=$(python3 -c '
import json, sys
page = json.load(sys.stdin)
plate = sys.argv[1]
items = page.get("items", []) if isinstance(page, dict) else page
for vehicle in items:
    if vehicle.get("plate") == plate:
        print(vehicle["id"])
        break
' "$PLATE" < "$vehicles_file")
if [[ -z "$vehicle_id" ]]; then
  echo "No se encontró el vehículo $PLATE en la flota del proveedor." >&2
  exit 1
fi

echo "Simulando $PLATE ($vehicle_id)"
echo "Abre http://localhost:3000/proveedor antes de continuar."

# Trayecto aproximado sobre Bogotá desde Universidad Nacional hacia el norte.
start_lat=4.6382
start_lon=-74.0838
for ((i = 0; i < COUNT; i++)); do
  lat=$(python3 -c 'print(f"{float(__import__("sys").argv[1]) + float(__import__("sys").argv[2]) * int(__import__("sys").argv[3]):.6f}")' "$start_lat" 0.000045 "$i")
  lon=$(python3 -c 'print(f"{float(__import__("sys").argv[1]) + float(__import__("sys").argv[2]) * int(__import__("sys").argv[3]):.6f}")' "$start_lon" 0.000025 "$i")
  time=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  seq=$((2000000 + i))
  heading=$((90 + (i * 5) % 180))

  status=$(curl -sS -o "$response_file" -w '%{http_code}' \
    -X POST "$BASE_URL/api/v1/telemetry/points" \
    -H 'Content-Type: application/json' \
    -H "Authorization: Bearer $access_token" \
    -d "{\"vehicle_id\":\"$vehicle_id\",\"time\":\"$time\",\"lat\":$lat,\"lon\":$lon,\"speed_kph\":32.5,\"heading\":$heading,\"device_seq\":$seq}")

  if [[ "$status" != "201" ]]; then
    echo "[$((i + 1))/$COUNT] HTTP $status: $(cat "$response_file")" >&2
    exit 1
  fi
  echo "[$((i + 1))/$COUNT] $time lat=$lat lon=$lon"

  if (( i < COUNT - 1 )); then
    sleep "$INTERVAL_SECONDS"
  fi
done

echo "Se enviaron $COUNT ubicaciones. Verifica los eventos del vehículo en el navegador."
