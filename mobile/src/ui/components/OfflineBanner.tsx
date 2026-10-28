import React from 'react'
import { StyleSheet, Text, View } from 'react-native'

import { colors, radius, spacing, typography } from '../theme'

export interface OfflineBannerProps {
  online: boolean
  syncing: boolean
  pendingPoints: number
  lastError: string | null
}

/**
 * Estado de red siempre visible: el conductor debe saber que sus datos siguen
 * a salvo en el dispositivo aunque no haya cobertura.
 */
export function OfflineBanner({ online, syncing, pendingPoints, lastError }: OfflineBannerProps) {
  if (!online) {
    return (
      <View style={[styles.banner, styles.offline]}>
        <Text style={styles.title}>Sin conexión</Text>
        <Text style={styles.body}>
          Los puntos GPS y los cambios se guardan en el dispositivo
          {pendingPoints > 0 ? ` (${pendingPoints} en cola)` : ''} y se enviarán al reconectar.
        </Text>
      </View>
    )
  }

  if (syncing) {
    return (
      <View style={[styles.banner, styles.syncing]}>
        <Text style={styles.title}>Sincronizando…</Text>
        <Text style={styles.body}>Enviando los datos acumulados al servidor.</Text>
      </View>
    )
  }

  if (lastError) {
    return (
      <View style={[styles.banner, styles.error]}>
        <Text style={styles.title}>No se pudo sincronizar</Text>
        <Text style={styles.body}>{lastError}</Text>
      </View>
    )
  }

  return null
}

const styles = StyleSheet.create({
  banner: {
    borderRadius: radius.md,
    padding: spacing.md,
    borderWidth: StyleSheet.hairlineWidth,
    gap: spacing.xs,
  },
  offline: {
    backgroundColor: 'rgba(245, 158, 11, 0.12)',
    borderColor: 'rgba(245, 158, 11, 0.45)',
  },
  syncing: {
    backgroundColor: 'rgba(59, 130, 246, 0.12)',
    borderColor: 'rgba(59, 130, 246, 0.45)',
  },
  error: {
    backgroundColor: 'rgba(239, 68, 68, 0.12)',
    borderColor: 'rgba(239, 68, 68, 0.45)',
  },
  title: {
    color: colors.text,
    fontSize: typography.body,
    fontWeight: '700',
  },
  body: {
    color: colors.muted,
    fontSize: typography.small,
    lineHeight: 19,
  },
})
