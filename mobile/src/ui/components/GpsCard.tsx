import React, { useCallback, useEffect, useState } from 'react'
import { ActivityIndicator, Linking, Pressable, StyleSheet, Text, View } from 'react-native'

import type { LocationTrackingState } from '../../ports/location'
import { useServices } from '../context/ServicesProvider'
import { colors, radius, spacing, typography } from '../theme'

/**
 * Control explícito del GPS. No se activa solo: el conductor debe aceptar los
 * permisos y entender el impacto en batería (especialmente «Permitir siempre»).
 */
export function GpsCard() {
  const { tracker } = useServices()
  const [state, setState] = useState<LocationTrackingState | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const refresh = useCallback(async () => {
    try {
      setState(await tracker.getState())
    } catch (rawError) {
      setError(rawError instanceof Error ? rawError.message : String(rawError))
    }
  }, [tracker])

  useEffect(() => {
    void refresh()
  }, [refresh])

  const start = useCallback(async () => {
    setBusy(true)
    setError(null)
    try {
      setState(await tracker.start())
    } catch (rawError) {
      setError(
        rawError instanceof Error
          ? rawError.message
          : 'No se pudo iniciar el seguimiento GPS en este dispositivo',
      )
    } finally {
      setBusy(false)
    }
  }, [tracker])

  const stop = useCallback(async () => {
    setBusy(true)
    setError(null)
    try {
      setState(await tracker.stop())
    } catch (rawError) {
      setError(rawError instanceof Error ? rawError.message : String(rawError))
    } finally {
      setBusy(false)
    }
  }, [tracker])

  const running = state?.mode === 'background' || state?.mode === 'foreground'
  const permissionDenied = state?.foreground === 'denied' || state?.background === 'denied'

  return (
    <View style={styles.card}>
      <View style={styles.header}>
        <Text style={styles.title}>Seguimiento GPS</Text>
        <View style={[styles.dot, running ? styles.dotOn : styles.dotOff]} />
      </View>

      <Text style={styles.message}>{state?.message ?? 'Comprobando permisos de ubicación…'}</Text>

      {state?.mode === 'foreground' ? (
        <Text style={styles.warning}>
          Sin permiso «Permitir siempre» la ruta se pausa cuando la app queda en segundo plano.
        </Text>
      ) : null}

      {error ? <Text style={styles.error}>{error}</Text> : null}

      <View style={styles.actions}>
        {running ? (
          <Pressable
            onPress={() => void stop()}
            disabled={busy}
            style={({ pressed }) => [styles.button, styles.secondary, pressed && styles.pressed]}
          >
            {busy ? (
              <ActivityIndicator color={colors.text} />
            ) : (
              <Text style={styles.secondaryLabel}>Detener</Text>
            )}
          </Pressable>
        ) : (
          <Pressable
            onPress={() => void start()}
            disabled={busy}
            style={({ pressed }) => [styles.button, styles.primary, pressed && styles.pressed]}
          >
            {busy ? (
              <ActivityIndicator color={colors.white} />
            ) : (
              <Text style={styles.primaryLabel}>Activar seguimiento</Text>
            )}
          </Pressable>
        )}

        {permissionDenied ? (
          <Pressable
            onPress={() => void Linking.openSettings()}
            style={({ pressed }) => [styles.button, styles.ghost, pressed && styles.pressed]}
          >
            <Text style={styles.secondaryLabel}>Abrir ajustes</Text>
          </Pressable>
        ) : null}
      </View>

      <Text style={styles.hint}>
        Se registra una muestra cada 15 s o 25 m. En segundo plano verás una notificación
        mientras la ruta esté activa.
      </Text>
    </View>
  )
}

const styles = StyleSheet.create({
  card: {
    backgroundColor: colors.surface,
    borderRadius: radius.lg,
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: colors.border,
    padding: spacing.lg,
    gap: spacing.sm,
  },
  header: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
  },
  title: {
    color: colors.text,
    fontSize: typography.subtitle,
    fontWeight: '700',
  },
  dot: {
    width: 10,
    height: 10,
    borderRadius: radius.pill,
  },
  dotOn: { backgroundColor: colors.success },
  dotOff: { backgroundColor: colors.muted },
  message: {
    color: colors.muted,
    fontSize: typography.small,
    lineHeight: 19,
  },
  warning: {
    color: '#fbbf24',
    fontSize: typography.small,
    lineHeight: 19,
  },
  error: {
    color: '#fca5a5',
    fontSize: typography.small,
  },
  actions: {
    flexDirection: 'row',
    flexWrap: 'wrap',
    gap: spacing.sm,
    marginTop: spacing.xs,
  },
  button: {
    minHeight: 42,
    paddingHorizontal: spacing.lg,
    borderRadius: radius.md,
    alignItems: 'center',
    justifyContent: 'center',
  },
  primary: { backgroundColor: colors.primary },
  secondary: {
    backgroundColor: colors.surfaceAlt,
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: colors.border,
  },
  ghost: {
    backgroundColor: 'transparent',
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: colors.border,
  },
  pressed: { opacity: 0.75 },
  primaryLabel: { color: colors.white, fontWeight: '700', fontSize: typography.body },
  secondaryLabel: { color: colors.text, fontWeight: '600', fontSize: typography.body },
  hint: {
    color: colors.muted,
    fontSize: typography.tiny,
    lineHeight: 16,
    marginTop: spacing.xs,
  },
})
