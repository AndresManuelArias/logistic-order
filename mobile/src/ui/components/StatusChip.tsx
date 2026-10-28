import React from 'react'
import { StyleSheet, Text, View } from 'react-native'

import type { OrderStatus } from '../../contracts/types'
import { colors, radius, spacing, typography } from '../theme'

const LABELS: Record<OrderStatus, { label: string; color: string; background: string }> = {
  CREATED: { label: 'Creada', color: '#fbbf24', background: 'rgba(245, 158, 11, 0.14)' },
  IN_TRANSIT: { label: 'En ruta', color: '#93c5fd', background: 'rgba(59, 130, 246, 0.16)' },
  DELIVERED: { label: 'Entregada', color: '#86efac', background: 'rgba(34, 197, 94, 0.16)' },
  CANCELLED: { label: 'Cancelada', color: '#fca5a5', background: 'rgba(239, 68, 68, 0.16)' },
}

export function StatusChip({ status }: { status: OrderStatus }) {
  const config = LABELS[status]
  return (
    <View style={[styles.chip, { backgroundColor: config.background }]}>
      <Text style={[styles.label, { color: config.color }]}>{config.label}</Text>
    </View>
  )
}

const styles = StyleSheet.create({
  chip: {
    alignSelf: 'flex-start',
    paddingHorizontal: spacing.md,
    paddingVertical: spacing.xs + 2,
    borderRadius: radius.pill,
  },
  label: {
    fontSize: typography.small,
    fontWeight: '700',
    letterSpacing: 0.3,
  },
})
