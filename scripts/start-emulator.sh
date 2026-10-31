#!/usr/bin/env bash
# Arranca el emulador Android y abre la app del conductor contra Metro.
#
#   scripts/start-emulator.sh                 # emulador + Metro + app
#   scripts/start-emulator.sh --avd pixel     # otro AVD
#   scripts/start-emulator.sh --sdk /media/datos/toolchains
#   scripts/start-emulator.sh --no-metro      # ya tienes Metro corriendo
#   HEADLESS=1 scripts/start-emulator.sh      # sin ventana (SSH / sin pantalla)
#
# Hace, en orden: resuelve el entorno Android, arranca el emulador, espera el
# boot real, arranca Metro si hace falta, instala/abre la app por deep link,
# concede permisos y deja una posición GPS inicial.
set -euo pipefail

REPO_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
MOBILE_DIR="$REPO_ROOT/mobile"
AVD=${AVD:-tiendas36}
GPU=${GPU:-swiftshader_indirect}
PORT=${PORT:-8081}
API_URL=${API_URL:-http://10.0.2.2:8080}
EMU_LOG=${EMU_LOG:-/tmp/emulator.log}
METRO_LOG=${METRO_LOG:-/tmp/metro.log}
START_METRO=1
SDK_BASE=${ANDROID_TOOLCHAINS:-}
PKG=dev.tiendas.track.driver

die() { printf '\n[error] %s\n' "$*" >&2; exit 1; }
info() { printf '[start-emulator] %s\n' "$*"; }

while [ $# -gt 0 ]; do
  case "$1" in
    --avd)      AVD="$2"; shift 2 ;;
    --sdk)      SDK_BASE="$2"; shift 2 ;;
    --gpu)      GPU="$2"; shift 2 ;;
    --port)     PORT="$2"; shift 2 ;;
    --no-metro) START_METRO=0; shift ;;
    -h|--help)  sed -n '2,12p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *)          die "opción desconocida: $1 (usa --help)" ;;
  esac
done

# El emulador aborta con mensajes de Qt/crashpad poco claros: traducimos los
# fallos típicos a una causa y una acción.
explain_emulator_failure() {
  local log=$1
  if grep -qiE "could not connect to display|Authorization required|no Qt platform plugin" "$log"; then
    printf '%s\n' \
      "No puede abrir la ventana del emulador (X11/Wayland)." \
      "  · Terminal del escritorio: export DISPLAY=:0 y export XAUTHORITY=\$XAUTHORITY" \
      "  · Por SSH o sin pantalla: HEADLESS=1 $0  (arranca con -no-window; usa adb exec-out screencap para verlo)"
  elif grep -qi "Unknown AVD name" "$log"; then
    printf '%s\n' "No encuentra el AVD: revisa ANDROID_AVD_HOME (lo exporta scripts/android-env.sh)."
  elif grep -qiE "/dev/kvm|KVM is required" "$log"; then
    printf '%s\n' "Sin acceso a /dev/kvm: añade el usuario al grupo kvm (newgrp kvm) o usa GPU=swiftshader -accel off (mucho más lento)."
  elif grep -qi "No space left" "$log"; then
    printf '%s\n' "Sin espacio en disco para el AVD."
  else
    printf 'Revisa el log completo: %s\n' "$log"
  fi
}

# ── 1. Entorno Android ────────────────────────────────────────────────────────
# El script sólo funciona con el SDK en el PATH; si no está, se busca la base
# donde vive (caso de discos pequeños: SDK fuera de $HOME).
env_ok() { ( source "$REPO_ROOT/scripts/android-env.sh" >/dev/null 2>&1 \
             && command -v adb >/dev/null && command -v emulator >/dev/null ); }

find_sdk_base() {
  local d found
  for d in "$HOME/Android/Sdk" "$HOME/Android/sdk" /opt/android-sdk /usr/lib/android-sdk; do
    [ -x "$d/emulator/emulator" ] && { dirname "$d"; return 0; }
  done
  info "buscando el SDK de Android fuera de \$HOME..." >&2
  for d in "$REPO_ROOT/.." "$HOME" /media /mnt /opt /srv; do
    [ -d "$d" ] || continue
    found=$(find "$d" -maxdepth 5 -type d -name android-sdk -print -quit 2>/dev/null || true)
    if [ -n "$found" ] && [ -x "$found/emulator/emulator" ]; then
      dirname "$found"; return 0
    fi
  done
  return 1
}

if ! env_ok; then
  if [ -z "$SDK_BASE" ]; then
    SDK_BASE=$(find_sdk_base) || die "SDK de Android no encontrado.
  Instálalo o pásale la base donde vive:
    scripts/start-emulator.sh --sdk /ruta/con/espacio/toolchains
    (debe contener android-sdk/emulator/emulator)"
  fi
  export ANDROID_TOOLCHAINS="$SDK_BASE"
  env_ok || die "ANDROID_TOOLCHAINS=$SDK_BASE no contiene android-sdk con platform-tools y emulator."
fi

# shellcheck disable=SC1091
source "$REPO_ROOT/scripts/android-env.sh" >/dev/null
info "SDK: $ANDROID_HOME"
info "AVD home: $ANDROID_AVD_HOME"

EMU_WINDOW_ARGS=(-no-snapshot -no-audio -no-boot-anim -gpu "$GPU")
if [ -n "${HEADLESS:-}" ]; then
  EMU_WINDOW_ARGS+=(-no-window)
  info "modo headless (sin ventana): verás la pantalla con adb exec-out screencap -p > pantalla.png"
elif [ -z "${DISPLAY:-}${WAYLAND_DISPLAY:-}" ]; then
  die "sin pantalla (DISPLAY vacío). Usa HEADLESS=1 $0 o abre una sesión de escritorio."
fi

emulator -list-avds 2>/dev/null | grep -qx "$AVD" || die "el AVD '$AVD' no existe.
  Lista: emulator -list-avds
  Créalo: avdmanager create avd -n $AVD -k \"system-images;android-36;google_apis;x86_64\" -d pixel_7"

# ── 2. Emulador ───────────────────────────────────────────────────────────────
serial=$(adb devices | awk 'NR>1 && $2=="device" {print $1; exit}')
if [ -n "$serial" ]; then
  info "emulador ya en marcha: $serial (no arranco otro)"
else
  info "arrancando AVD '$AVD' (log: $EMU_LOG)"
  emulator -avd "$AVD" "${EMU_WINDOW_ARGS[@]}" > "$EMU_LOG" 2>&1 &
  emu_pid=$!

  # Fallo rápido: si el proceso muere en los primeros segundos, no esperamos 6 min.
  for _ in 1 2 3 4 5 6; do
    sleep 1
    kill -0 "$emu_pid" 2>/dev/null || {
      explain_emulator_failure "$EMU_LOG" >&2
      die "el emulador terminó al arrancar (log: $EMU_LOG)"
    }
  done

  adb wait-for-device
  info "esperando el boot de Android (puede tardar ~90 s la primera vez)"
  booted=0
  for _ in $(seq 1 180); do
    if [ "$(adb shell getprop sys.boot_completed 2>/dev/null | tr -d '\r')" = "1" ]; then booted=1; break; fi
    sleep 2
  done
  [ "$booted" = "1" ] || { explain_emulator_failure "$EMU_LOG" >&2; die "Android no terminó de arrancar en 6 min (log: $EMU_LOG)"; }

  serial=$(adb devices | awk 'NR>1 && $2=="device" {print $1; exit}')
  info "emulador listo: $serial (API $(adb shell getprop ro.build.version.sdk | tr -d '\r'))"
fi

adb_dev() { adb -s "$serial" "$@"; }

# El System UI del emulador se satura con renderizado por software y saca un
# diálogo "System UI isn't responding" que tapa la app y roba el foco. Se
# desactivan esos diálogos y, si ya hay uno, se reinicia SystemUI.
adb_dev shell settings put global hide_error_dialogs 1 >/dev/null 2>&1 || true

focused_anr() { adb_dev shell dumpsys window 2>/dev/null | grep -q "Not Responding"; }
dismiss_anr() {
  focused_anr || return 0
  info "System UI no responde (ANR del emulador, no de la app): reinicio SystemUI"
  adb_dev shell am force-stop com.android.systemui >/dev/null 2>&1 || true
  sleep 8
}

APK="$MOBILE_DIR/android/app/build/outputs/apk/debug/app-debug.apk"
# `pm path` es más rápido y fiable que filtrar `pm list packages` (que puede
# venir vacío durante los primeros segundos tras el boot).
app_installed() { adb_dev shell pm path "$PKG" 2>/dev/null | tr -d '\r' | grep -q "^package:"; }

# ── 3. App instalada ──────────────────────────────────────────────────────────
installed=0
for _ in $(seq 1 15); do app_installed && { installed=1; break; }; sleep 2; done

if [ "$installed" = "1" ]; then
  info "app ya instalada: $PKG"
elif [ -f "$APK" ]; then
  info "instalando APK: $APK"
  adb_dev install -r "$APK" >/dev/null
else
  info "la app no está instalada y no hay APK compilado."
  info "compílala una vez (la primera vez tarda mucho: Gradle + NDK):"
  printf '\n    cd mobile && npm run android\n\n' >&2
  info "o instala un APK existente: adb install -r /ruta/app-debug.apk"
fi

# ── 4. Metro ──────────────────────────────────────────────────────────────────
metro_up()   { curl -sf "http://localhost:$PORT/status" >/dev/null 2>&1; }
port_busy()  { ss -ltn 2>/dev/null | awk '{print $4}' | grep -qE "[:.]$PORT$"; }

if metro_up; then
  info "Metro ya responde en :$PORT (lo reutilizo)"
elif port_busy; then
  info "el puerto :$PORT está ocupado pero Metro aún no responde; espero"
  for _ in $(seq 1 30); do metro_up && break; sleep 2; done
  metro_up || die "el puerto :$PORT está ocupado por otro proceso (ss -ltnp | grep :$PORT). Libéralo o usa --port <otro>."
elif [ "$START_METRO" = "1" ]; then
  command -v npx >/dev/null || die "npx no está en el PATH: activa Node (nvm use) o instálalo desde https://nodejs.org"
  info "arrancando Metro en :$PORT (log: $METRO_LOG)"
  ( cd "$MOBILE_DIR" && CI=1 npx expo start --dev-client --port "$PORT" > "$METRO_LOG" 2>&1 & )
  for _ in $(seq 1 90); do metro_up && break; sleep 2; done
  if ! metro_up; then
    grep -qiE "non-interactive|already.*running|port" "$METRO_LOG" 2>/dev/null && \
      printf '  → %s\n' "$(grep -m1 -iE "port .* in use|Use port|already" "$METRO_LOG" || true)" >&2
    die "Metro no respondió en :$PORT (ver $METRO_LOG)"
  fi
  info "Metro listo"
else
  info "sin Metro en :$PORT (--no-metro); la app mostrará 'Unable to load script'"
fi

# ── 5. Abrir la app contra Metro ──────────────────────────────────────────────
open_app() {
  adb_dev reverse "tcp:$PORT" "tcp:$PORT" >/dev/null
  adb_dev shell am start -a android.intent.action.VIEW \
    -d "tiendastrack://expo-development-client/?url=http%3A%2F%2Flocalhost%3A$PORT" >/dev/null
}
app_focused() { adb_dev shell dumpsys window 2>/dev/null | grep -m1 "mCurrentFocus" | grep -q "$PKG"; }

dismiss_anr
open_app
for _ in 1 2 3; do
  sleep 6
  if focused_anr; then dismiss_anr; open_app; continue; fi
  app_focused && break
done

# ── 6. Permisos y posición inicial ────────────────────────────────────────────
for p in ACCESS_FINE_LOCATION ACCESS_COARSE_LOCATION ACCESS_BACKGROUND_LOCATION POST_NOTIFICATIONS; do
  adb_dev shell pm grant "$PKG" "android.permission.$p" >/dev/null 2>&1 || true
done
adb_dev emu geo fix -74.0612 4.6480 2600 8 5 >/dev/null 2>&1 || true

# ── 7. Resumen ────────────────────────────────────────────────────────────────
if app_focused; then
  estado="app en primer plano"
else
  estado="la app no está en primer plano todavía (mira la ventana del emulador)"
fi

cat <<EOF

[listo] $estado en $serial
  backend esperado : $API_URL  (mobile/.env)
  Metro            : http://localhost:$PORT
  logs de la app   : adb logcat -s ReactNativeJS:V | grep -i tiendas
  mover GPS        : adb emu geo fix <lon> <lat> [alt] [sats] [vel]
  captura          : adb exec-out screencap -p > pantalla.png
  apagar emulador  : adb -s $serial emu kill

La primera carga del bundle tarda ~30 s y después aparece el login
(conductor1@tiendas.dev / Conductor123). Si ves "System UI isn't responding",
pulsa Wait: es el emulador, no la app. Revisa $METRO_LOG si algo falla.
EOF
