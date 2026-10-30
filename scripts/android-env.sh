#!/usr/bin/env bash
# Entorno Android para compilar y ejecutar la app móvil.
#
#   source scripts/android-env.sh
#
# Sólo exporta variables; no instala nada ni modifica el proyecto.
#
# Variables que respeta:
#   ANDROID_HOME        SDK ya instalado (tiene prioridad)
#   ANDROID_TOOLCHAINS  base donde viven SDK/caché/AVD cuando no están en $HOME
#                       (útil si el disco de $HOME es pequeño)
#
# Ejemplo en una máquina con el SDK fuera de $HOME:
#   export ANDROID_TOOLCHAINS=/media/datos/toolchains
#   source scripts/android-env.sh

set -u

_find_sdk() {
  local c
  if [ -n "${ANDROID_HOME:-}" ] && [ -x "$ANDROID_HOME/platform-tools/adb" ]; then
    printf '%s' "$ANDROID_HOME"; return 0
  fi
  for c in "$HOME/Android/Sdk" "$HOME/Android/sdk" /opt/android-sdk "${ANDROID_SDK_ROOT:-}"; do
    [ -n "$c" ] && [ -x "$c/platform-tools/adb" ] && { printf '%s' "$c"; return 0; }
  done
  if [ -n "${ANDROID_TOOLCHAINS:-}" ] && [ -x "$ANDROID_TOOLCHAINS/android-sdk/platform-tools/adb" ]; then
    printf '%s' "$ANDROID_TOOLCHAINS/android-sdk"; return 0
  fi
  return 1
}

_find_jdk17() {
  local d
  for d in "$HOME/.sdkman/candidates/java"/17* /usr/lib/jvm/java-17-*; do
    [ -x "$d/bin/javac" ] && { printf '%s' "$d"; return 0; }
  done
  return 1
}

SDK="$(_find_sdk || true)"
if [ -z "$SDK" ]; then
  echo "android-env: SDK de Android no encontrado." >&2
  echo "  Instálalo o exporta ANDROID_HOME/ANDROID_TOOLCHAINS." >&2
  return 1 2>/dev/null || exit 1
fi

export ANDROID_HOME="$SDK"
export ANDROID_SDK_ROOT="$SDK"
export PATH="$SDK/platform-tools:$SDK/cmdline-tools/latest/bin:$SDK/emulator:$PATH"

# Fuera de $HOME, Gradle y los AVD también deben salir de $HOME (disco pequeño).
if [ -n "${ANDROID_TOOLCHAINS:-}" ]; then
  export ANDROID_USER_HOME="${ANDROID_USER_HOME:-$ANDROID_TOOLCHAINS/android-home}"
  export GRADLE_USER_HOME="${GRADLE_USER_HOME:-$ANDROID_TOOLCHAINS/gradle}"
  mkdir -p "$ANDROID_USER_HOME" "$GRADLE_USER_HOME"
fi

# React Native 0.81 / Gradle 8.14 se validan con JDK 17.
if [ -z "${JAVA_HOME:-}" ]; then
  JDK="$(_find_jdk17 || true)"
  [ -n "$JDK" ] && export JAVA_HOME="$JDK"
fi

echo "ANDROID_HOME=$ANDROID_HOME"
echo "ANDROID_USER_HOME=${ANDROID_USER_HOME:-$HOME/.android}"
echo "GRADLE_USER_HOME=${GRADLE_USER_HOME:-$HOME/.gradle}"
echo "JAVA_HOME=${JAVA_HOME:-<system java>}"
command -v adb >/dev/null && echo "adb=$(command -v adb)"
command -v emulator >/dev/null && echo "emulator=$(command -v emulator)"
