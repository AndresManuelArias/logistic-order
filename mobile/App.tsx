// Debe importarse antes de montar React: registra la tarea de ubicación en el
// ámbito de módulo para que el sistema pueda despertarla en headless.
import './src/composition/background-task'

import React, { useEffect, useState } from 'react'
import { ActivityIndicator, Pressable, StyleSheet, Text, View } from 'react-native'
import { StatusBar } from 'expo-status-bar'
import { SafeAreaProvider } from 'react-native-safe-area-context'

import { getServices, type AppServices } from './src/composition/services'
import { ServicesProvider } from './src/ui/context/ServicesProvider'
import { RootNavigator } from './src/ui/navigation/RootNavigator'
import { colors, radius, spacing, typography } from './src/ui/theme'

function BootScreen({ error, onRetry }: { error: string | null; onRetry: () => void }) {
  return (
    <View style={styles.boot}>
      {error ? (
        <>
          <Text style={styles.title}>No se pudo iniciar la app</Text>
          <Text style={styles.body}>{error}</Text>
          <Text style={styles.hint}>
            Si el problema continúa, cierra la app y vuelve a abrirla. Los datos ya guardados no se
            borran.
          </Text>
          <Pressable
            onPress={onRetry}
            style={({ pressed }) => [styles.button, pressed && styles.pressed]}
            accessibilityRole="button"
          >
            <Text style={styles.buttonLabel}>Reintentar</Text>
          </Pressable>
        </>
      ) : (
        <>
          <ActivityIndicator color={colors.primary} size="large" />
          <Text style={styles.body}>Preparando la ruta…</Text>
        </>
      )}
    </View>
  )
}

export default function App() {
  const [services, setServices] = useState<AppServices | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [attempt, setAttempt] = useState(0)

  useEffect(() => {
    let cancelled = false
    setError(null)

    getServices()
      .then((ready) => {
        if (!cancelled) setServices(ready)
      })
      .catch((rawError: unknown) => {
        if (cancelled) return
        setError(
          rawError instanceof Error
            ? rawError.message
            : 'Error desconocido al inicializar la base local',
        )
      })

    return () => {
      cancelled = true
    }
  }, [attempt])

  return (
    <SafeAreaProvider>
      <StatusBar style="light" />
      {services ? (
        <ServicesProvider services={services}>
          <RootNavigator />
        </ServicesProvider>
      ) : (
        <BootScreen error={error} onRetry={() => setAttempt((value) => value + 1)} />
      )}
    </SafeAreaProvider>
  )
}

const styles = StyleSheet.create({
  boot: {
    flex: 1,
    backgroundColor: colors.background,
    alignItems: 'center',
    justifyContent: 'center',
    padding: spacing.xl,
    gap: spacing.md,
  },
  title: { color: colors.text, fontSize: typography.subtitle, fontWeight: '700' },
  body: { color: colors.muted, fontSize: typography.body, textAlign: 'center', lineHeight: 22 },
  hint: { color: colors.muted, fontSize: typography.small, textAlign: 'center', lineHeight: 19 },
  button: {
    marginTop: spacing.md,
    backgroundColor: colors.primary,
    borderRadius: radius.md,
    paddingHorizontal: spacing.xl,
    paddingVertical: spacing.md,
  },
  pressed: { opacity: 0.8 },
  buttonLabel: { color: colors.white, fontWeight: '700', fontSize: typography.body },
})
