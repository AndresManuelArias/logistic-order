# Tiendas-Track — App del conductor (F4 móvil)

Aplicación React Native **offline-first** para conductores. Captura la ruta por
GPS, guarda cada muestra en el dispositivo, encola los cambios de estado de las
órdenes y los sincroniza por lotes cuando hay red, sin duplicar datos.

> **Estado:** lógica, UI y empaquetado JS implementados; `typecheck`, 99 pruebas
> y bundle de Metro (`expo export`) en verde. **No se pudo compilar ni ejecutar
> en un dispositivo/emulador** porque el entorno no tiene Android SDK, Xcode ni
> hardware conectado. Ver [Limitaciones](#limitaciones-y-supuestos).

---

## 1. Arranque rápido

> **Requisitos:** Node.js **20.19.4+** (recomendado 22 LTS; hay `.nvmrc`, ejecuta
> `nvm use`) y npm 10+. `npm ci` instala exactamente las versiones del lockfile.

```bash
cd mobile
nvm use                     # Node 22 (si usas nvm)
npm ci                      # instalación reproducible desde package-lock.json
cp .env.example .env        # ajusta EXPO_PUBLIC_API_URL

npm run typecheck           # tsc --noEmit
npm test                    # Vitest (lógica pura, sin nativo)

npx expo prebuild --clean   # genera android/ e ios/ (no se versionan)
npm run android             # requiere emulador/dispositivo + Android SDK
# npm run ios               # requiere macOS + Xcode
```

`npm start` levanta Metro en modo dev client. **Expo Go no sirve para validar
GPS en segundo plano** (limitación de Expo Go); usa un development build.

### Android: SDK, emulador y JDK

`expo run:android` necesita el SDK de Android y `adb`. Si faltan, el error es
`Failed to resolve the Android SDK path` y `spawn adb ENOENT`; no es un problema
del proyecto.

```bash
# Desde la raíz del repo: exporta ANDROID_HOME, PATH, GRADLE_USER_HOME y JDK 17.
# Si el SDK vive fuera de $HOME (disco pequeño), indica la base:
export ANDROID_TOOLCHAINS=/ruta/con/espacio/toolchains
source scripts/android-env.sh

sdkmanager --licenses
sdkmanager "platform-tools" "platforms;android-36" "build-tools;36.0.0" \
           "emulator" "system-images;android-36;google_apis;x86_64"

avdmanager create avd -n tiendas36 -k "system-images;android-36;google_apis;x86_64" -d pixel_7
emulator -avd tiendas36 &
adb devices          # debe listar el emulador como "device"

cd mobile && npm run android
```

- `compileSdk`/`targetSdk` = **36** y `minSdk` = **24** (los fija Expo SDK 54).
- Gradle 8.14 + React Native 0.81: usa **JDK 17** (el script lo detecta con
  sdkman o `/usr/lib/jvm/java-17-*`).
- El SDK ocupa ~7-9 GB entre platform, build-tools, emulador e imagen del
  sistema; reserva ese espacio antes de instalarlo.
- El emulador Android mapea `10.0.2.2` al loopback del host: es lo que permite
  que `EXPO_PUBLIC_API_URL=http://10.0.2.2:8080` llegue al backend local.
- Si `avdmanager create avd` termina imprimiendo `null` y no crea nada (pasa con
  imágenes descargadas a mano para API 36, porque el zip no trae `package.xml`),
  el emulador **sí** acepta el AVD creado manualmente:
  - Registrar el paquete en `system-images/android-36/google_apis/x86_64/package.xml`
    (`sysImgDetailsType`, api-level 36, tag `google_apis`, abi `x86_64`).
  - Crear `$ANDROID_AVD_HOME/tiendas36.ini` (`path` + `target=android-36`) y
    `tiendas36.avd/config.ini` (`abi.type=x86_64`, `image.sysdir.1=system-images/android-36/google_apis/x86_64/`,
    `hw.device.name=pixel_7`, RAM/CPU/densidad).
  - Comprobar con `emulator -list-avds`; `avdmanager list avd` puede seguir
    fallando sin impedir el arranque.

### URL del backend

| Entorno | `EXPO_PUBLIC_API_URL` |
|---|---|
| Emulador Android | `http://10.0.2.2:8080` |
| Simulador iOS | `http://localhost:8080` |
| Dispositivo físico | `http://<IP-LAN-de-tu-máquina>:8080` |

`EXPO_PUBLIC_*` se sustituye en build-time: cámbiala en `.env` y reinicia Metro.
No pongas secretos en estas variables (acaban en el bundle).

> En producción la URL **debe** ser `https://`. Android bloquea tráfico en claro
> desde API 28 salvo que se declare `usesCleartextTraffic`; las direcciones
> `http://` de la tabla son sólo para desarrollo local con emulador o LAN.
> La app nunca desactiva la validación TLS.

Credenciales demo del seed: `conductor1@tiendas.dev` / `Conductor123`.

---

## 2. Arquitectura

```
mobile/
├── App.tsx                       # arranque, splash/error, providers
├── index.ts                      # registerRootComponent
└── src/
    ├── composition/              # composition root (¡no es expo-router!)
    │   ├── services.ts           # singleton de dependencias + sesión + coordinador
    │   └── background-task.ts    # registra la tarea GPS en ámbito de módulo
    ├── contracts/                # tipos generados desde OpenAPI (ADR-10)
    ├── domain/                   # reglas puras: punto, orden, sync, POD, errores
    ├── ports/                    # interfaces (auth, GPS, cola, conectividad…)
    ├── application/
    │   ├── auth/                 # SessionService (login, refresh single-flight)
    │   ├── telemetry/            # RecordGpsSample (validar + persistir)
    │   ├── sync/                 # PointSync, TransitionSync, Coordinator, backoff
    │   ├── orders/               # OrderService (caché + overlay optimista)
    │   └── delivery/             # DeliveryService (POD local + transición)
    ├── infrastructure/
    │   ├── api/                  # HttpClient + endpoints tipados
    │   ├── connectivity/         # NetInfo
    │   ├── device/               # expo-location + TaskManager
    │   ├── files/                # copia de fotos a Documents/
    │   ├── logging/              # logger sin coordenadas ni tokens
    │   ├── persistence/watermelon/  # esquema, modelos y adaptadores
    │   └── security/             # SecureStore (Keychain/Keystore)
    ├── ui/                       # pantallas, navegación y componentes
    ├── shared/                   # IDs e idempotency keys
    ├── integration/              # prueba offline → reconexión
    └── testing/                  # dobles en memoria para Vitest
```

Regla de dependencia: `ui → application → domain` y `infrastructure` implementa
los `ports`. La lógica de sincronización no importa React Native ni WatermelonDB,
por eso puede probarse con Vitest sin emulador.

---

## 3. Modelo local (WatermelonDB / SQLite)

Esquema v1 en `src/infrastructure/persistence/watermelon/schema.ts`.

| Tabla | Propósito |
|---|---|
| `app_meta` | `device_id` y `device_seq` monótono por instalación |
| `local_points` | Muestras GPS con estado `pending/batched/synced/rejected` |
| `sync_batches` | Lote write-ahead con su `Idempotency-Key` y backoff |
| `orders` | Caché del DTO `Order` (JSON) + columnas indexadas `status`, `placed_at` |
| `order_transitions` | Cola de cambios de estado (`pending/applied/rejected`) |
| `delivery_proofs` | Evidencia POD local (foto copiada a `Documents/`) |

Decisiones:

- Las órdenes guardan el JSON completo del contrato para no migrar la tabla si
  evoluciona el DTO; `status` y `placed_at` se duplican para filtrar/ordenar.
- Los IDs de fila son propios (UUID del servidor para órdenes, IDs locales para
  puntos/transiciones) y se asignan con `record._raw.id`.
- Todos los datos llevan `owner_id` (el `sub` del conductor). Al cerrar sesión no
  se consulta ninguna tabla, así que no se sirve información de otra cuenta.

---

## 4. Sincronización offline-first

### 4.1 Puntos GPS

1. `RecordGpsSample` valida coordenadas, timestamp (≤ 5 min en el futuro),
   velocidad, rumbo y precisión (≤ 100 m).
2. Normaliza y redondea (~0,1 m) y asigna `device_seq` monótono.
3. **Persiste en SQLite antes de cualquier intento de red.**
4. `PointSyncService` toma hasta 200 puntos y crea un lote con su
   `Idempotency-Key`, **también persistido antes de enviar**.
5. Sólo puede existir **un lote abierto por conductor**. Un reintento reutiliza
   la misma clave y el mismo lote, aunque la app se haya reiniciado.
6. El backend responde `accepted`, `duplicates`, `already_processed`. Un
   `already_processed: true` se considera éxito y cierra el lote.
7. Tras confirmar el servidor, los puntos pasan a `synced`. Nunca se borran
   antes de la confirmación.

Clave de deduplicación del backend: `(time, vehicle_id, device_seq)`. El
`Idempotency-Key` cubre además el lote completo.

### 4.2 Transiciones de orden

`PATCH /api/v1/orders/{id}/status` **no acepta** `Idempotency-Key`; la
idempotencia se resuelve releyendo el servidor:

- Si un intento anterior fue ambiguo (timeout/5xx), antes de reenviar se hace
  `GET /orders/{id}`. Si ya está en el estado objetivo → aplicada.
- Un `409` se reconcilia con `GET`: si el servidor ya está en el objetivo se da
  por aplicada; si no, se marca rechazada (el estado cambió fuera de la app).
- Las transiciones se envían **en orden de encolado** y de una en una, para no
  adelantar `DELIVERED` sobre `IN_TRANSIT`.
- Offline, la UI muestra el estado optimista y una marca «cambio pendiente».

### 4.3 Política de errores

| Respuesta | Acción | ¿Se conservan los datos? |
|---|---|---|
| Red / timeout / 5xx / 429 | Reintento con backoff exponencial + jitter (5 s → 5 min; respeta `Retry-After`) | Sí |
| 401 / 403 / 404 | Pausa sin quemar reintentos; 401 pide reautenticar | Sí |
| 400 / 409 / 422 | Cuarentena del lote (`rejected`) sin borrar | Sí |
| 2xx (incl. `already_processed`) | Cierre y limpieza de la cola | — |

Un conductor sin vehículo asignado recibe `403`: la cola se pausa y los puntos
permanecen guardados hasta que el servidor permita el envío.

### 4.4 Disparadores

- Arranque y login.
- Cambio de conectividad (NetInfo).
- Cada 30 s en primer plano (`SYNC_POLICY.tickMs`).
- Tras capturar una muestra o encolar una transición.
- Al volver del segundo plano.
- Pull-to-refresh y reintento manual.

`SyncCoordinator` es **single-flight**: dos disparadores simultáneos comparten
la misma promesa y no crean dos lotes con claves distintas.

---

## 5. GPS, batería y permisos

- Frecuencia: **15 s o 25 m**, precisión `High`, filtro de precisión 100 m.
  Son ~4 muestras/min, suficiente para la traza sin vaciar la batería.
- **Por qué `High` y no `Balanced`:** en Android, `Balanced` se traduce a
  `PRIORITY_BALANCED_POWER_ACCURACY`, que FusedLocationProvider resuelve con
  localización por red (`Nlp`), sin pedir GPS. Si el equipo no tiene ese
  proveedor (emulador sin servicios de red de Google, dispositivo sin datos) no
  llega **ninguna** muestra aunque el GPS esté disponible. Con `High`
  (`PRIORITY_HIGH_ACCURACY`) la petición incluye `GpsBatch[requested=true]`.
- **Segundo plano:** `expo-location` + `expo-task-manager` con foreground service
  en Android (notificación persistente) y `UIBackgroundModes: location` en iOS.
- Si el usuario sólo concede «mientras uso la app», se usa `watchPositionAsync` y
  se **pausa** al pasar a segundo plano (`AppState`), reanudándose al volver.
- El seguimiento **no se activa solo**: la tarjeta GPS pide permisos de forma
  explícita y explica el impacto. El botón «Detener» apaga la tarea.
- Permisos declarados en `app.json`:
  - iOS: ubicación en uso y siempre, cámara y fotos.
  - Android: `ACCESS_FINE/COARSE/BACKGROUND_LOCATION`, `FOREGROUND_SERVICE`,
    `FOREGROUND_SERVICE_LOCATION`, `POST_NOTIFICATIONS`, `CAMERA`.
- La tarea se registra al importar `src/composition/background-task.ts`, antes de
  montar React, para que funcione en arranque headless.
- En Android 13+ la notificación del servicio requiere permiso de
  notificaciones; si el usuario lo niega, el servicio puede seguir activo pero
  sin notification visible (comportamiento del sistema).

---

## 6. Autenticación y seguridad

- Access token **en memoria** + refresh token rotativo en **SecureStore**
  (Keychain/Keystore); nunca AsyncStorage ni SQLite.
- `keychainAccessible: AFTER_FIRST_UNLOCK` para permitir la sincronización en
  segundo plano tras el primer desbloqueo.
- Refresh **single-flight**: varias peticiones concurrentes comparten una sola
  rotación; evita invalidar el refresh token por doble uso.
- La petición de refresh se envía con `auth: false`: no resuelve el access token
  ni reintenta tras un 401. Si lo hiciera, `getAccessToken` → `ensureAccessToken`
  volvería a pedir el refresh en curso y formaría una cadena de promesas
  autorreferente (`Maximum call stack size exceeded (native stack depth)`) que
  bloquea toda sincronización hasta reiniciar la app. Cubierto por
  `src/infrastructure/api/http.test.ts`.
- Un fallo de red al renovar **no cierra la sesión**: la app sigue operando
  offline y reintenta al reconectar.
- Un refresh revocado (`401/403`) limpia la sesión y pide login; los datos
  pendientes permanecen en el dispositivo.
- El cliente HTTP añade el bearer y traduce el sobre
  `{ "error": { "code", "message" } }` del backend a errores tipados.
- No se registran coordenadas, tokens ni datos personales en logs.

---

## 7. Contratos y generación de tipos

- Tipos generados con `openapi-typescript` desde `backend/api/openapi.yaml`
  (ADR-10): `src/contracts/generated/openapi.ts` **no se edita a mano**.
- `npm run gen:api` regenera; `npm run check:api` falla si hay diferencias.
- Endpoints usados (todos existentes en el contrato):
  - `POST /api/v1/auth/login`, `/refresh`, `/logout`, `GET /auth/me`
  - `GET /api/v1/orders`, `GET /orders/{id}`, `PATCH /orders/{id}/status`
  - `POST /api/v1/telemetry/bulk` con `Idempotency-Key`
- No se inventaron endpoints ni reglas de negocio.

### Prueba de entrega (POD)

El contrato F2–F4 **no expone un endpoint de prueba de entrega**. La app guarda
receptor, notas, timestamp y foto en el dispositivo (`upload_state =
pending_backend`) y **no finge una subida**. La transición `DELIVERED` sí viaja
por la cola normal. Cuando exista endpoint POD bastará con añadir un puerto y un
adaptador; el dominio ya está separado.

### Órdenes «del día»

`GET /api/v1/orders` no filtra por fecha. La app descarga la página (100) y
selecciona en cliente: activas (`CREATED`, `IN_TRANSIT`) + finalizadas creadas
hoy (hora local). Es una decisión de presentación, no una regla del backend.

---

## 8. Interfaz

| Pantalla | Contenido |
|---|---|
| Login | Correo/contraseña, errores accionables, aviso de sesión caducada |
| Órdenes | Nombre, contador «X puntos pendientes», banner online/offline, tarjeta GPS, lista del día con estado y cambio pendiente, pull-to-refresh, logout |
| Detalle | Datos de la orden, estado optimista, acción `Iniciar entrega` / `Confirmar entrega`, formulario POD (receptor, notas, cámara/galería), evidencias guardadas |

Componentes destacados: `PendingBadge`, `OfflineBanner`, `StatusChip`, `GpsCard`,
`RuntimeEffects` (timer + `AppState`). `useSyncExternalStore` conecta la UI con
`SyncStatusStore` y `SessionService` sin polling.

---

## 9. Pruebas y validación

```bash
npm test          # 99 pruebas, 13 archivos
npm run typecheck # sin errores
```

| Área | Cobertura |
|---|---|
| Dominio | validación/normalización GPS, máquina de estados, órdenes del día |
| Sync de puntos | lotes, límites, backoff, `Retry-After`, 401/403, cuarentena |
| Idempotencia | fallo ambiguo + reinicio reutiliza la misma clave; sin duplicados |
| Transiciones | orden, 409 reconciliado, cambio externo, reintentos |
| Coordinador | single-flight, offline, reconexión, sesión expirada, contadores |
| Sesión | refresh single-flight, revocación, fallo de red, logout best-effort |
| Órdenes | caché-first, overlay optimista, dedupe, transición ilegal, aislamiento |
| POD | validación, copia de foto, persistencia local, fallo de transición |
| Integración | modo avión → reconexión; fallo ambiguo; aislamiento por conductor |

Validaciones ejecutadas en este entorno:

- `npx tsc --noEmit` ✅
- `npx vitest run` → **99/99** ✅
- `npx expo install --check` → dependencias alineadas con SDK 54 ✅
- `npx expo export --platform android` → bundle Metro/Hermes correcto ✅
- `npx expo config --type public` → configuración válida ✅

**No ejecutado:** compilación nativa, instalación en dispositivo, GPS real,
permisos reales, cámara, ciclo de vida en segundo plano y consumo de batería.

---

## 10. Limitaciones y supuestos

1. **Sin endpoint POD**: la evidencia queda sólo en el dispositivo. Se muestra
   explícitamente en la UI.
2. **Sin filtro de fecha en órdenes**: «del día» se calcula en cliente.
3. **Sin emulador/hardware**: no se validó compilación nativa ni comportamiento
   real de GPS, permisos, cámara o battery drain.
4. **WatermelonDB 0.28 exige arquitectura antigua**: `newArchEnabled: false` y
   `jsi: false`. No actualizar RN/SDK sin revisar compatibilidad.
5. **Expo Go no soporta GPS en segundo plano**; se necesita dev client/prebuild.
6. **La app no envía `vehicle_id` ni `order_id`** en las muestras: el backend
   deriva el vehículo del conductor. Si no hay vehículo asignado responde `403`
   y la cola se pausa (los datos no se pierden).
7. **Reloj del dispositivo**: muestras con más de 5 min de adelanto se descartan
   (límite del backend). No hay corrección automática de hora.
8. **Cuarentena sin UI de reintento**: los puntos rechazados se cuentan y se
   conservan, pero no hay botón para reenviarlos; requiere revisión/depuración.
9. **Reinicio tras reboot**: la tarea se re-registra al abrir la app. No se
   garantiza reanudación automática del servicio tras reiniciar el teléfono.
10. **Multicuenta en el mismo dispositivo**: los datos se separan por
    `owner_id`, pero las fotos POD de una cuenta anterior permanecen en
    `Documents/` (no se borran al cerrar sesión).
11. **Sin mapa ni navegación turn-by-turn**: se abre el destino en Google Maps.
12. **`src/composition`** se llama así a propósito para no confundirse con la
    convención de rutas `src/app` de Expo Router.

---

## 11. Solución de problemas

| Síntoma | Causa probable | Acción |
|---|---|---|
| «Sin conexión» con red | `EXPO_PUBLIC_API_URL` incorrecta o firewall | Probar `curl http://<host>:8080/healthz` desde el dispositivo/emulador |
| La cola no avanza y error 403 | Conductor sin vehículo asignado | Asignar vehículo/orden en el backend; los puntos se conservan |
| «La sesión caducó» | Refresh revocado o expirado | Volver a iniciar sesión; la cola local se mantiene |
| «Maximum call stack size exceeded (native stack depth)» al caducar el token | La petición de refresh resolvía el access token y se llamaba a sí misma | Ya corregido (`auth: false` en `/auth/*`); si reaparece, revisar `HttpClient.request` y añadir el caso a `http.test.ts` |
| No aparece el permiso «Siempre» | iOS exige conceder primero «Al usar la app» y luego volver a solicitar | Ajustes → Ubicación → Siempre |
| El GPS no avanza en Expo Go | Limitación de Expo Go | Usar `expo prebuild` + dev client |
| GPS activo pero 0 puntos | La petición es de red y no de GPS | `adb shell dumpsys activity service com.google.android.gms/com.google.android.location.internal.GoogleLocationManagerService \| grep -E "tiendas\|HIGH_ACCURACY\|GpsBatch"`; debe aparecer `HIGH_ACCURACY` y `GpsBatch[requested=true]` |
| App no arranca («base local») | SQLite corrupta o migración fallida | Reabrir; si persiste, reinstalar (se pierde la cola local) |
| Bundle falla tras cambiar `.env` | Metro cachea variables build-time | Reiniciar Metro con `--clear` |

---

## 12. Referencias

- Plan y criterio F4: [`doc/plan_desarrollo.md`](../doc/plan_desarrollo.md)
- Contrato canónico: [`backend/api/openapi.yaml`](../backend/api/openapi.yaml)
- Portal web F4: [`web/README.md`](../web/README.md)
- Configuración: `app.json`, `src/config/env.ts`, `src/config/policies.ts`
