import React from 'react'
import { StyleSheet, Text, View } from 'react-native'

import { colors, radius, spacing, typography } from '../theme'

export interface PendingBadgeProps {
  pendingPoints: number
  pendingTransitions: number
  rejectedPoints: number
  syncing: boolean
}

/**
 * Contador exigido por F4: «X puntos pendientes». Incluye los cambios de orden
 * para que el conductor sepa que también están en cola.
 */
export function PendingBadge({
  pendingPoints,
  pendingTransitions,
  rejectedPoints,
  syncing,
}: PendingBadgeProps) {
  const hasQueue = pendingPoints > 0 || pendingTransitions > 0 || rejectedPoints > 0

  if (!hasQueue && !syncing) {
    return (
      <View style={[styles.badge, styles.clean]}>
        <Text style={[styles.value, styles.cleanText]}>Todo sincronizado</Text>
      </View>
    )
  }

  return (
    <View style={styles.row}>
      <View style={styles.badge}>
        <Text style={styles.value}>
          {pendingPoints} {pendingPoints === 1 ? 'punto pendiente' : 'puntos pendientes'}
        </Text>
      </View>
      {pendingTransitions > 0 ? (
        <View style={styles.badge}>
          <Text style={styles.value}>
            {pendingTransitions} {pendingTransitions === 1 ? 'cambio' : 'cambios'}
          </Text>
        </View>
      ) : null}
      {rejectedPoints > 0 ? (
        <View style={[styles.badge, styles.rejected]}>
          <Text style={[styles.value, styles.rejectedText]}>{rejectedPoints} rechazados</Text>
        </View>
      ) : null}
      {syncing ? (
        <View style={[styles.badge, styles.syncing]}>
          <Text style={[styles.value, styles.syncingText]}>Sincronizando…</Text>
        </View>
      ) : null}
    </View>
  )
}

const styles = StyleSheet.create({
  row: {
    flexDirection: 'row',
    flexWrap: 'wrap',
    gap: spacing.sm,
  },
  badge: {
    backgroundColor: colors.surfaceAlt,
    borderRadius: radius.pill,
    paddingHorizontal: spacing.md,
    paddingVertical: spacing.xs + 2,
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: colors.border,
  },
  value: {
    color: colors.text,
    fontSize: typography.small,
    fontWeight: '600',
  },
  clean: {
    backgroundColor: 'rgba(34, 197, 94, 0.12)',
    borderColor: 'rgba(34, 197, 94, 0.4)',
  },
  cleanText: {
    color: '#86efac',
  },
  rejected: {
    backgroundColor: 'rgba(239, 68, 68, 0.12)',
    borderColor: 'rgba(239, 68, 68, 0.4)',
  },
  rejectedText: {
    color: '#fca5a5',
  },
  syncing: {
    backgroundColor: 'rgba(59, 130, 246, 0.14)',
    borderColor: 'rgba(59, 130, 246, 0.4)',
  },
  syncingText: {
    color: '#93c5fd',
  },
})
