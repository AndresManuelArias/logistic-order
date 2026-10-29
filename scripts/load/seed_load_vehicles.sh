#!/usr/bin/env bash
# Siembra vehículos adicionales para la prueba de carga F6 (RNF-06).
#
# Uso:
#   scripts/load/seed_load_vehicles.sh [N]        # por defecto 100
#   scripts/load/seed_load_vehicles.sh --clean    # elimina solo los LOAD-*
#
# Requisitos: contenedor PostgreSQL de docker compose en marcha (`make db-up`).
# Es idempotente: las placas LOAD-001..LOAD-N se insertan con ON CONFLICT.
# No borra datos existentes; --clean elimina únicamente vehículos LOAD-*.
set -euo pipefail

COMPOSE="${COMPOSE:-docker compose}"
DB_SERVICE="${DB_SERVICE:-db}"
POSTGRES_USER="${POSTGRES_USER:-tiendas}"
POSTGRES_DB="${POSTGRES_DB:-tiendas}"

if [[ "${1:-}" == "--clean" ]]; then
  echo "Eliminando vehículos de carga (LOAD-*)..."
  $COMPOSE exec -T "$DB_SERVICE" psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" <<'SQL'
-- Los puntos de telemetría no tienen FK al vehículo (hypertable): se limpian
-- explícitamente para no dejar series huérfanas de la prueba de carga.
DELETE FROM telemetry.points
WHERE vehicle_id IN (SELECT id FROM app.vehicles WHERE plate LIKE 'LOAD-%');
DELETE FROM app.vehicles WHERE plate LIKE 'LOAD-%';
SQL
  echo "Listo. Los vehículos LOAD-* fueron eliminados."
  exit 0
fi

N="${1:-100}"
if ! [[ "$N" =~ ^[0-9]+$ ]] || [[ "$N" -lt 1 ]] || [[ "$N" -gt 1000 ]]; then
  echo "N debe ser un entero entre 1 y 1000" >&2
  exit 1
fi

echo "Sembrando $N vehículos LOAD-* para el proveedor demo..."

$COMPOSE exec -T "$DB_SERVICE" psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" \
  -v n="$N" <<'SQL'
WITH provider AS (
  SELECT id FROM app.users WHERE email = 'proveedor@tiendas.dev' AND role = 'PROVIDER'
),
series AS (
  SELECT generate_series(1, :n) AS i
)
INSERT INTO app.vehicles (plate, provider_id, status)
SELECT 'LOAD-' || lpad(i::text, 3, '0'), provider.id, 'ACTIVE'
FROM series CROSS JOIN provider
ON CONFLICT (plate) DO NOTHING;

SELECT count(*) AS total_load FROM app.vehicles WHERE plate LIKE 'LOAD-%';
SQL

echo "Vehículos de carga listos. Limpieza: scripts/load/seed_load_vehicles.sh --clean"
