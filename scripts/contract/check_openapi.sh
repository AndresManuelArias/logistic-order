#!/usr/bin/env bash
# Lint del contrato OpenAPI de Tiendas-Track (F6).
#
# Uso: scripts/contract/check_openapi.sh
#
# Ejecuta:
#   1. Pruebas Go del paquete openapi (estructura, $ref y endpoints obligatorios).
#   2. Spectral (si npx y red están disponibles) con severidad de fallo "error".
#
# En CI se ejecuta desde la raíz del repositorio. No requiere Node instalado
# aparte: `npx --yes` descarga la CLI fijada en la versión 6.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SPEC="${ROOT}/backend/api/openapi.yaml"

if [[ ! -f "$SPEC" ]]; then
  echo "No existe el contrato: $SPEC" >&2
  exit 1
fi

echo "==> Pruebas Go del contrato embebido"
(cd "$ROOT/backend" && go test ./internal/shared/openapi/...)

echo "==> Spectral lint (OpenAPI)"
if ! command -v npx >/dev/null 2>&1; then
  echo "npx no está disponible; se omite Spectral (solo se ejecutaron las pruebas Go)." >&2
  exit 0
fi

npx --yes @stoplight/spectral-cli@6 lint \
  --ruleset "${ROOT}/.spectral.yaml" \
  --fail-severity=error \
  "$SPEC"

echo "Contrato OpenAPI válido."
