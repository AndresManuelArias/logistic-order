import React, { useCallback, useState } from 'react'
import {
  ActivityIndicator,
  KeyboardAvoidingView,
  Platform,
  Pressable,
  ScrollView,
  StyleSheet,
  Text,
  TextInput,
  View,
} from 'react-native'
import { SafeAreaView } from 'react-native-safe-area-context'

import { API_BASE_URL } from '../../config/env'
import { useServices, useSession } from '../context/ServicesProvider'
import { describeError } from '../errors'
import { colors, radius, spacing, typography } from '../theme'

export function LoginScreen() {
  const { session } = useServices()
  const snapshot = useSession()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const submit = useCallback(async () => {
    if (loading) return
    if (email.trim().length === 0 || password.length === 0) {
      setError('Escribe tu correo y contraseña')
      return
    }
    setLoading(true)
    setError(null)
    try {
      await session.login(email, password)
    } catch (rawError) {
      setError(describeError(rawError))
    } finally {
      setLoading(false)
    }
  }, [email, loading, password, session])

  return (
    <SafeAreaView style={styles.safe}>
      <KeyboardAvoidingView
        style={styles.flex}
        behavior={Platform.OS === 'ios' ? 'padding' : undefined}
      >
        <ScrollView
          contentContainerStyle={styles.container}
          keyboardShouldPersistTaps="handled"
          showsVerticalScrollIndicator={false}
        >
          <View style={styles.brand}>
            <View style={styles.logo}>
              <Text style={styles.logoText}>TT</Text>
            </View>
            <Text style={styles.title}>Tiendas-Track</Text>
            <Text style={styles.subtitle}>App del conductor</Text>
          </View>

          {snapshot.status === 'expired' ? (
            <View style={styles.notice}>
              <Text style={styles.noticeText}>
                Tu sesión caducó. Vuelve a iniciar sesión; los datos pendientes siguen en este
                dispositivo.
              </Text>
            </View>
          ) : null}

          <View style={styles.card}>
            <Text style={styles.label}>Correo</Text>
            <TextInput
              value={email}
              onChangeText={setEmail}
              placeholder="conductor@tiendas.dev"
              placeholderTextColor={colors.muted}
              autoCapitalize="none"
              autoCorrect={false}
              keyboardType="email-address"
              textContentType="username"
              editable={!loading}
              style={styles.input}
              returnKeyType="next"
            />

            <Text style={styles.label}>Contraseña</Text>
            <TextInput
              value={password}
              onChangeText={setPassword}
              placeholder="••••••••"
              placeholderTextColor={colors.muted}
              autoCapitalize="none"
              autoCorrect={false}
              secureTextEntry
              textContentType="password"
              editable={!loading}
              style={styles.input}
              returnKeyType="go"
              onSubmitEditing={() => void submit()}
            />

            {error ? <Text style={styles.error}>{error}</Text> : null}

            <Pressable
              onPress={() => void submit()}
              disabled={loading}
              style={({ pressed }) => [
                styles.submit,
                (pressed || loading) && styles.submitPressed,
              ]}
              accessibilityRole="button"
              accessibilityLabel="Iniciar sesión"
            >
              {loading ? (
                <ActivityIndicator color={colors.white} />
              ) : (
                <Text style={styles.submitLabel}>Iniciar sesión</Text>
              )}
            </Pressable>
          </View>

          <Text style={styles.hint}>
            Sólo cuentas de conductor. La ruta, las órdenes y la prueba de entrega se guardan en el
            dispositivo cuando no hay conexión.
          </Text>
          <Text style={styles.server}>Servidor: {API_BASE_URL}</Text>
        </ScrollView>
      </KeyboardAvoidingView>
    </SafeAreaView>
  )
}

const styles = StyleSheet.create({
  safe: { flex: 1, backgroundColor: colors.background },
  flex: { flex: 1 },
  container: {
    flexGrow: 1,
    justifyContent: 'center',
    padding: spacing.xl,
    gap: spacing.xl,
  },
  brand: { alignItems: 'center', gap: spacing.sm },
  logo: {
    width: 64,
    height: 64,
    borderRadius: radius.lg,
    backgroundColor: colors.primary,
    alignItems: 'center',
    justifyContent: 'center',
  },
  logoText: { color: colors.white, fontWeight: '800', fontSize: 26 },
  title: { color: colors.text, fontSize: 28, fontWeight: '800' },
  subtitle: { color: colors.muted, fontSize: typography.body },
  notice: {
    backgroundColor: 'rgba(245, 158, 11, 0.12)',
    borderColor: 'rgba(245, 158, 11, 0.45)',
    borderWidth: StyleSheet.hairlineWidth,
    borderRadius: radius.md,
    padding: spacing.md,
  },
  noticeText: { color: '#fbbf24', fontSize: typography.small, lineHeight: 19 },
  card: {
    backgroundColor: colors.surface,
    borderRadius: radius.lg,
    padding: spacing.lg,
    gap: spacing.sm,
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: colors.border,
  },
  label: {
    color: colors.muted,
    fontSize: typography.small,
    fontWeight: '600',
    marginTop: spacing.xs,
  },
  input: {
    backgroundColor: colors.surfaceAlt,
    borderRadius: radius.md,
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: colors.border,
    color: colors.text,
    paddingHorizontal: spacing.md,
    paddingVertical: spacing.md,
    fontSize: typography.body,
  },
  error: { color: '#fca5a5', fontSize: typography.small, marginTop: spacing.xs },
  submit: {
    marginTop: spacing.md,
    backgroundColor: colors.primary,
    borderRadius: radius.md,
    minHeight: 48,
    alignItems: 'center',
    justifyContent: 'center',
  },
  submitPressed: { opacity: 0.8 },
  submitLabel: { color: colors.white, fontWeight: '700', fontSize: typography.body },
  hint: {
    color: colors.muted,
    fontSize: typography.small,
    textAlign: 'center',
    lineHeight: 19,
  },
  server: { color: colors.border, fontSize: typography.tiny, textAlign: 'center' },
})
