#!/usr/bin/env bash
# E2E F2 — Auth, RBAC y órdenes contra http://localhost:8080
# Sale con código != 0 si algún caso falla. Salida: .diag/e2e.txt
set -u
BASE="${BASE:-http://localhost:8080}"
FAIL=0
J=.diag/jars; mkdir -p "$J"

say()  { printf '%s\n' "$*"; }
chk()  { # chk <nombre> <esperado> <obtenido>
  if [ "$2" = "$3" ]; then say "OK   $1 -> $3"; else say "FAIL $1 -> esperado $2, obtenido $3"; FAIL=1; fi
}
post() { curl -sS -o "$2" -w '%{http_code}' -X POST "$BASE$1" -H 'Content-Type: application/json' -d "$3"; }
# postAuth: igual que post pero con Authorization: Bearer.
postAuth() { curl -sS -o "$3" -w '%{http_code}' -X POST "$BASE$1" -H 'Content-Type: application/json' -H "Authorization: Bearer $4" -d "$2"; }
get()  { curl -sS -o "$2" -w '%{http_code}' -X GET  "$BASE$1" -H "Authorization: Bearer ${3:-}"; }
patch() { curl -sS -o "$2" -w '%{http_code}' -X PATCH "$BASE$1" -H 'Content-Type: application/json' -H "Authorization: Bearer ${4:-}" -d "$3"; }
jqr()  { grep -o "\"$2\"[^,}]*" "$1" | head -1 | sed 's/.*: *"\?//; s/"$//'; }

say "=== F2 E2E: $(date -Is) ==="

# Limpieza idempotente de corridas previas (borra órdenes y usuarios E2E).
# Un segundo proveedor con su propio vehículo: permite probar el 403 real
# de "asignar vehículo de otra flota" (con un solo proveedor todo es propia).
bootstrap_prov2() {
  docker compose exec -T db psql -U tiendas -d tiendas -q <<'SQL' >/dev/null 2>&1 || true
INSERT INTO app.users (email, password_hash, full_name, role)
SELECT 'prov2@tiendas.dev', '$2y$10$0e0XjVW1bT6k3r6yJ0m0YOKk5o7Xn4Bq2v1q1J8p0S9m8k5l4d6iS', 'Proveedor 2', 'PROVIDER'
WHERE NOT EXISTS (SELECT 1 FROM app.users WHERE email='prov2@tiendas.dev');
INSERT INTO app.vehicles (plate, provider_id, status)
SELECT 'XYZ999', u.id, 'ACTIVE' FROM app.users u
WHERE u.email='prov2@tiendas.dev'
  AND NOT EXISTS (SELECT 1 FROM app.vehicles v WHERE v.plate='XYZ999');
INSERT INTO app.drivers (user_id, vehicle_id, license_number)
SELECT u.id, v.id, 'LIC-999999' FROM app.users u, app.vehicles v
WHERE u.email='prov2@tiendas.dev' AND v.plate='XYZ999'
  AND NOT EXISTS (SELECT 1 FROM app.drivers d WHERE d.vehicle_id=v.id);
SQL
}
bootstrap_prov2

cleanup_e2e() {
  docker compose exec -T db psql -U tiendas -d tiendas -q <<'SQL' >/dev/null 2>&1 || true
DELETE FROM app.order_events WHERE order_id IN (SELECT id FROM app.orders WHERE external_ref LIKE 'E2E-%');
DELETE FROM app.orders WHERE external_ref LIKE 'E2E-%';
DELETE FROM app.stores WHERE owner_user_id IN (SELECT id FROM app.users WHERE email LIKE 'e2e.%@tiendas.dev');
DELETE FROM app.drivers WHERE user_id IN (SELECT id FROM app.users WHERE email LIKE 'e2e.%@tiendas.dev');
DELETE FROM app.refresh_tokens WHERE user_id IN (SELECT id FROM app.users WHERE email LIKE 'e2e.%@tiendas.dev');
DELETE FROM app.users WHERE email LIKE 'e2e.%@tiendas.dev';
-- Reinicia estados de órdenes seed que un run previo pudo mover.
UPDATE app.orders SET status='CREATED', assigned_vehicle_id=NULL, assigned_driver_id=NULL,
       picked_up_at=NULL, delivered_at=NULL
 WHERE external_ref IN ('PED-1001','PED-1005');
SQL
}
cleanup_e2e

# --- 1. Operabilidad y documentación (ADR-10) -----------------------------
chk "GET /healthz" 200 "$(curl -sS -o /dev/null -w '%{http_code}' "$BASE/healthz")"
chk "GET /readyz" 200 "$(curl -sS -o /dev/null -w '%{http_code}' "$BASE/readyz")"
chk "GET /api/v1/openapi.json" 200 "$(curl -sS -o /dev/null -w '%{http_code}' "$BASE/api/v1/openapi.json")"
chk "GET /api/v1/openapi.yaml" 200 "$(curl -sS -o /dev/null -w '%{http_code}' "$BASE/api/v1/openapi.yaml")"
chk "GET /swagger/index.html" 200 "$(curl -sS -o /dev/null -w '%{http_code}' "$BASE/swagger/index.html")"

# --- 2. Registro (solo STORE/DRIVER) --------------------------------------
chk "register STORE con store" 201 "$(post /api/v1/auth/register "$J/reg_store.json" '{"email":"e2e.store@tiendas.dev","password":"Segura12345","full_name":"E2E Store","role":"STORE","store":{"name":"E2E Tienda","address":"Cra 1 # 2-3","location":{"lat":4.6550,"lon":-74.0632}}}')"
chk "register STORE sin store -> 422" 422 "$(post /api/v1/auth/register "$J/reg_nostore.json" '{"email":"e2e.nostore@tiendas.dev","password":"Segura12345","role":"STORE"}')"
chk "register duplicado -> 409" 409 "$(post /api/v1/auth/register "$J/reg_dup.json" '{"email":"e2e.store@tiendas.dev","password":"Segura12345","role":"STORE","store":{"name":"Dup","location":{"lat":4.65,"lon":-74.05}}}')"
chk "register PROVIDER -> 422" 422 "$(post /api/v1/auth/register "$J/reg_prov.json" '{"email":"e2e.prov@tiendas.dev","password":"Segura12345","role":"PROVIDER"}')"
chk "register rol desconocido -> 422" 422 "$(post /api/v1/auth/register "$J/reg_bad.json" '{"email":"e2e.bad@tiendas.dev","password":"Segura12345","role":"HACKER"}')"
chk "register password corta -> 422" 422 "$(post /api/v1/auth/register "$J/reg_weak.json" '{"email":"e2e.weak@tiendas.dev","password":"corta","role":"DRIVER"}')"
chk "register email inválido -> 422" 422 "$(post /api/v1/auth/register "$J/reg_mail.json" '{"email":"no-es-email","password":"Segura12345","role":"DRIVER"}')"
chk "register DRIVER" 201 "$(post /api/v1/auth/register "$J/reg_drv.json" '{"email":"e2e.driver@tiendas.dev","password":"Segura12345","full_name":"E2E Driver","role":"DRIVER"}')"

# --- 3. Login + par de tokens --------------------------------------------
chk "login OK" 200 "$(post /api/v1/auth/login "$J/login.json" '{"email":"e2e.store@tiendas.dev","password":"Segura12345"}')"
STORE_AT=$(jqr "$J/login.json" access_token); STORE_RT=$(jqr "$J/login.json" refresh_token)
chk "login password mala -> 401" 401 "$(post /api/v1/auth/login "$J/login_bad.json" '{"email":"e2e.store@tiendas.dev","password":"otra-clave"}')"
chk "login usuario inexistente -> 401" 401 "$(post /api/v1/auth/login "$J/login_no.json" '{"email":"nadie@tiendas.dev","password":"Segura12345"}')"
[ -n "$STORE_AT" ] && [ -n "$STORE_RT" ] && say "OK   par de tokens emitido" || { say "FAIL sin tokens"; FAIL=1; }

# Usuario seed proveedor (rol que no se auto-registra)
chk "login PROVIDER seed" 200 "$(post /api/v1/auth/login "$J/login_prov.json" '{"email":"proveedor@tiendas.dev","password":"Proveedor123"}')"
PROV_AT=$(jqr "$J/login_prov.json" access_token)
chk "login tienda seed" 200 "$(post /api/v1/auth/login "$J/login_t1.json" '{"email":"tienda1@tiendas.dev","password":"Tienda12345"}')"
SEEDSTORE_AT=$(jqr "$J/login_t1.json" access_token)
chk "login conductor seed 1" 200 "$(post /api/v1/auth/login "$J/login_d1.json" '{"email":"conductor1@tiendas.dev","password":"Conductor123"}')"
DRV_AT=$(jqr "$J/login_d1.json" access_token)
chk "login conductor seed 2" 200 "$(post /api/v1/auth/login "$J/login_d2.json" '{"email":"conductor2@tiendas.dev","password":"Conductor123"}')"
DRV2_AT=$(jqr "$J/login_d2.json" access_token)

# --- 4. /me y tokens protegidos ------------------------------------------
chk "GET /auth/me con token" 200 "$(get /api/v1/auth/me "$J/me.json" "$STORE_AT")"
chk "GET /auth/me sin token -> 401" 401 "$(get /api/v1/auth/me "$J/me_no.json" "")"
chk "GET /auth/me token basura -> 401" 401 "$(curl -sS -o "$J/me_bad.json" -w '%{http_code}' "$BASE/api/v1/auth/me" -H 'Authorization: Bearer no.es.jwt')"

# --- 5. Rotación de refresh y logout --------------------------------------
chk "refresh OK" 200 "$(post /api/v1/auth/refresh "$J/ref1.json" "{\"refresh_token\":\"$STORE_RT\"}")"
NEW_RT=$(jqr "$J/ref1.json" refresh_token)
chk "reutilizar refresh viejo -> 401" 401 "$(post /api/v1/auth/refresh "$J/ref2.json" "{\"refresh_token\":\"$STORE_RT\"}")"
chk "refresh rotado OK" 200 "$(post /api/v1/auth/refresh "$J/ref3.json" "{\"refresh_token\":\"$NEW_RT\"}")"
FINAL_RT=$(jqr "$J/ref3.json" refresh_token); FINAL_AT=$(jqr "$J/ref3.json" access_token)
chk "logout OK -> 204" 204 "$(curl -sS -o "$J/logout.json" -w '%{http_code}' -X POST "$BASE/api/v1/auth/logout" -H 'Content-Type: application/json' -d "{\"refresh_token\":\"$FINAL_RT\"}")"
chk "refresh tras logout -> 401" 401 "$(post /api/v1/auth/refresh "$J/ref4.json" "{\"refresh_token\":\"$FINAL_RT\"}")"

# --- 6. Creación de órdenes (STORE) y aislamiento -------------------------
NEW_AT=$FINAL_AT
chk "POST /orders (tienda propia)" 201 "$(postAuth /api/v1/orders '{"external_ref":"E2E-1001","origin":{"lat":4.6550,"lon":-74.0632},"destination":{"lat":4.6578,"lon":-74.0561},"destination_address":"Cra 7 # 72-40"}' "$J/order1.json" "$NEW_AT")"
OID=$(jqr "$J/order1.json" id)
say "     orden creada: $OID"
chk "POST /orders store_id ajeno -> 403" 403 "$(postAuth /api/v1/orders '{"external_ref":"E2E-1002","store_id":"00000000-0000-0000-0000-000000000001","origin":{"lat":4.65,"lon":-74.05},"destination":{"lat":4.66,"lon":-74.06}}' "$J/order2.json" "$NEW_AT")"
chk "POST /orders external_ref duplicado -> 409" 409 "$(postAuth /api/v1/orders '{"external_ref":"E2E-1001","origin":{"lat":4.65,"lon":-74.05},"destination":{"lat":4.66,"lon":-74.06}}' "$J/order3.json" "$NEW_AT")"
chk "POST /orders coordenada nula -> 422" 422 "$(postAuth /api/v1/orders '{"external_ref":"E2E-1003","origin":{"lat":0,"lon":0},"destination":{"lat":4.66,"lon":-74.06}}' "$J/order4.json" "$NEW_AT")"
chk "POST /orders sin token -> 401" 401 "$(curl -sS -o "$J/order5.json" -w '%{http_code}' -X POST "$BASE/api/v1/orders" -H 'Content-Type: application/json' -d '{}')"
chk "POST /orders rol DRIVER -> 403" 403 "$(curl -sS -o "$J/order6.json" -w '%{http_code}' -X POST "$BASE/api/v1/orders" -H 'Content-Type: application/json' -H "Authorization: Bearer $DRV_AT" -d '{"external_ref":"E2E-1004","origin":{"lat":4.65,"lon":-74.05},"destination":{"lat":4.66,"lon":-74.06}}')"

# --- 7. RBAC de datos en lectura ------------------------------------------
chk "tienda dueña ve su orden" 200 "$(get "/api/v1/orders/$OID" "$J/g1.json" "$NEW_AT")"
chk "rol ajeno ve orden -> 403" 403 "$(get "/api/v1/orders/$OID" "$J/g2.json" "$DRV2_AT")"
chk "conductor2 no ve orden del conductor1 -> 403" 403 "$(get "/api/v1/orders/$OID" "$J/g3.json" "$DRV2_AT")"
chk "list orders PROVIDER" 200 "$(get "/api/v1/orders" "$J/l1.json" "$PROV_AT")"
chk "list orders status inválido -> 422" 422 "$(get "/api/v1/orders?status=PAID" "$J/l2.json" "$PROV_AT")"
chk "list orders sin token -> 401" 401 "$(get "/api/v1/orders" "$J/l3.json" "")"

# ORD-1002 (PED-1002) está IN_TRANSIT asignado al conductor1.
O_TRANSIT=$(docker compose exec -T db psql -U tiendas -d tiendas -Atc "SELECT id FROM app.orders WHERE external_ref='PED-1002'" 2>/dev/null | grep -Eo '[0-9a-f-]{36}' | head -1)
chk "conductor asignado ve su orden" 200 "$(get "/api/v1/orders/$O_TRANSIT" "$J/g4.json" "$DRV_AT")"

# --- 8. Máquina de estados -------------------------------------------------
chk "CREATED -> IN_TRANSIT" 200 "$(patch "/api/v1/orders/$OID/status" "$J/s1.json" '{"status":"IN_TRANSIT"}' "$NEW_AT")"
chk "IN_TRANSIT -> DELIVERED" 200 "$(patch "/api/v1/orders/$OID/status" "$J/s2.json" '{"status":"DELIVERED"}' "$NEW_AT")"
chk "DELIVERED -> IN_TRANSIT (409)" 409 "$(patch "/api/v1/orders/$OID/status" "$J/s3.json" '{"status":"IN_TRANSIT"}' "$NEW_AT")"
chk "estado desconocido (422)" 422 "$(patch "/api/v1/orders/$OID/status" "$J/s4.json" '{"status":"PAID"}' "$NEW_AT")"
chk "transición sin token (401)" 401 "$(curl -sS -o "$J/s5.json" -w '%{http_code}' -X PATCH "$BASE/api/v1/orders/$OID/status" -H 'Content-Type: application/json' -d '{"status":"CANCELLED"}')"

# PED-1001 está CREATED y pertenece a la tienda1: probamos la cancelación.
O_CREATED=$(docker compose exec -T db psql -U tiendas -d tiendas -Atc "SELECT id FROM app.orders WHERE external_ref='PED-1001'" 2>/dev/null | grep -Eo '[0-9a-f-]{36}' | head -1)
chk "CREATED -> CANCELLED" 200 "$(patch "/api/v1/orders/$O_CREATED/status" "$J/s6.json" '{"status":"CANCELLED"}' "$SEEDSTORE_AT")"
chk "CANCELLED terminal (409)" 409 "$(patch "/api/v1/orders/$O_CREATED/status" "$J/s7.json" '{"status":"CREATED"}' "$SEEDSTORE_AT")"
chk "rol ajeno transiciona -> 403" 403 "$(patch "/api/v1/orders/$O_CREATED/status" "$J/s8.json" '{"status":"CANCELLED"}' "$DRV_AT")"

# --- 9. Asignación de flota (PROVIDER) -------------------------------------
VEH=$(docker compose exec -T db psql -U tiendas -d tiendas -Atc "SELECT id FROM app.vehicles WHERE plate='ABC123'" 2>/dev/null | grep -Eo '[0-9a-f-]{36}' | head -1)
DRV=$(docker compose exec -T db psql -U tiendas -d tiendas -Atc "SELECT d.id FROM app.drivers d JOIN app.vehicles v ON v.id=d.vehicle_id WHERE v.plate='ABC123'" 2>/dev/null | grep -Eo '[0-9a-f-]{36}' | head -1)
O_ASSIGN=$(docker compose exec -T db psql -U tiendas -d tiendas -Atc "SELECT id FROM app.orders WHERE external_ref='PED-1005'" 2>/dev/null | grep -Eo '[0-9a-f-]{36}' | head -1)
chk "asignación PROVIDER (auth)" 200 "$(curl -sS -o "$J/a1.json" -w '%{http_code}' -X POST "$BASE/api/v1/orders/$O_ASSIGN/assignment" -H 'Content-Type: application/json' -H "Authorization: Bearer $PROV_AT" -d "{\"vehicle_id\":\"$VEH\",\"driver_id\":\"$DRV\"}")"
chk "asignación sin rol -> 403" 403 "$(curl -sS -o "$J/a2.json" -w '%{http_code}' -X POST "$BASE/api/v1/orders/$O_ASSIGN/assignment" -H 'Content-Type: application/json' -H "Authorization: Bearer $SEEDSTORE_AT" -d "{\"vehicle_id\":\"$VEH\",\"driver_id\":\"$DRV\"}")"
VEH_AJENO=$(docker compose exec -T db psql -U tiendas -d tiendas -Atc "SELECT id FROM app.vehicles WHERE plate='XYZ999'" 2>/dev/null | grep -Eo '[0-9a-f-]{36}' | head -1)
DRV_AJENO=$(docker compose exec -T db psql -U tiendas -d tiendas -Atc "SELECT d.id FROM app.drivers d JOIN app.vehicles v ON v.id=d.vehicle_id WHERE v.plate='XYZ999'" 2>/dev/null | tr -d '\r\n')
chk "asignar vehículo de otra flota -> 403" 403 "$(curl -sS -o "$J/a3.json" -w '%{http_code}' -X POST "$BASE/api/v1/orders/$O_ASSIGN/assignment" -H 'Content-Type: application/json' -H "Authorization: Bearer $PROV_AT" -d "{\"vehicle_id\":\"$VEH_AJENO\",\"driver_id\":\"$DRV_AJENO\"}")"

# --- 10. Token de invitado (scope order:<id>) ------------------------------
chk "guest-token (tienda dueña, auth)" 201 "$(curl -sS -o "$J/gt1.json" -w '%{http_code}' -X POST "$BASE/api/v1/auth/guest-token" -H 'Content-Type: application/json' -H "Authorization: Bearer $SEEDSTORE_AT" -d "{\"order_id\":\"$O_ASSIGN\"}")"
GUEST=$(jqr "$J/gt1.json" token)
chk "invitado ve SOLO su orden" 200 "$(get "/api/v1/orders/$O_ASSIGN" "$J/go1.json" "$GUEST")"
chk "invitado no ve otra orden -> 403" 403 "$(get "/api/v1/orders/$OID" "$J/go2.json" "$GUEST")"
chk "invitado no lista -> 403" 403 "$(get "/api/v1/orders" "$J/go3.json" "$GUEST")"
chk "invitado no transiciona -> 403" 403 "$(curl -sS -o "$J/go4.json" -w '%{http_code}' -X PATCH "$BASE/api/v1/orders/$O_ASSIGN/status" -H 'Content-Type: application/json' -H "Authorization: Bearer $GUEST" -d '{"status":"CANCELLED"}')"
chk "guest-token rol DRIVER -> 403" 403 "$(curl -sS -o "$J/gt2.json" -w '%{http_code}' -X POST "$BASE/api/v1/auth/guest-token" -H 'Content-Type: application/json' -H "Authorization: Bearer $DRV_AT" -d "{\"order_id\":\"$OID\"}")"
chk "guest-token orden ajena -> 403" 403 "$(curl -sS -o "$J/gt3.json" -w '%{http_code}' -X POST "$BASE/api/v1/auth/guest-token" -H 'Content-Type: application/json' -H "Authorization: Bearer $SEEDSTORE_AT" -d '{"order_id":"00000000-0000-0000-0000-000000000009"}')"

say ""
if [ "$FAIL" -eq 0 ]; then say "=== RESULTADO: TODOS LOS CASOS OK ==="; else say "=== RESULTADO: HAY FALLOS ==="; fi
exit $FAIL
