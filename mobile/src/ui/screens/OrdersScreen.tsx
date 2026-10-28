import React, { useCallback, useEffect, useMemo, useState } from 'react'
import {
  ActivityIndicator,
  Alert,
  Pressable,
  RefreshControl,
  ScrollView,
  StyleSheet,
  Text,
  View,
} from 'react-native'
import { useFocusEffect } from '@react-navigation/native'
import type { NativeStackScreenProps } from '@react-navigation/native-stack'
import { SafeAreaView } from 'react-native-safe-area-context'

import { selectDayOrders } from '../../domain/order'
import type { OrderView } from '../../application/orders/order-service'
import { useOwnerId, useServices, useSession, useSyncStatus } from '../context/ServicesProvider'
import { describeError } from '../errors'
import { GpsCard } from '../components/GpsCard'
import { OfflineBanner } from '../components/OfflineBanner'
import { PendingBadge } from '../components/PendingBadge'
import { StatusChip } from '../components/StatusChip'
import { colors, radius, spacing, typography } from '../theme'
import type { RootStackParamList } from '../navigation/types'

type Props = NativeStackScreenProps<RootStackParamList, 'Orders'>

function formatTime(value: string): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '—'
  return date.toLocaleTimeString('es-CO', { hour: '2-digit', minute: '2-digit' })
}

function formatDistance(meters: number | undefined): string | null {
  if (typeof meters !== 'number' || !Number.isFinite(meters)) return null
  return meters >= 1000 ? `${(meters / 1000).toFixed(1)} km` : `${Math.round(meters)} m`
}

export function OrdersScreen({ navigation }: Props) {
  const { orders, coordinator, session } = useServices()
  const snapshot = useSession()
  const ownerId = useOwnerId()
  const status = useSyncStatus()
  const [views, setViews] = useState<OrderView[]>([])
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const readLocal = useCallback(async () => {
    if (!ownerId) return
    try {
      setViews(await orders.listCached(ownerId))
    } catch (rawError) {
      setError(describeError(rawError))
    }
  }, [orders, ownerId])

  const reload = useCallback(async () => {
    if (!ownerId) return
    setRefreshing(true)
    setError(null)
    try {
      const result = await orders.load(ownerId)
      setViews(result.views)
      if (result.error) setError(describeError(result.error))
    } catch (rawError) {
      setError(describeError(rawError))
      await readLocal()
    } finally {
      setRefreshing(false)
      setLoading(false)
    }
  }, [orders, ownerId, readLocal])

  // Al volver del detalle se relee la caché (incluye transiciones pendientes).
  useFocusEffect(
    useCallback(() => {
      void readLocal()
      void reload()
    }, [readLocal, reload]),
  )

  // Refresca la vista local cuando el coordinador aplica o encola cambios.
  useEffect(() => {
    void readLocal()
  }, [readLocal, status.lastSyncedAt, status.pendingTransitions, status.pendingPoints])

  const dayViews = useMemo(() => {
    const byId = new Map(views.map((view) => [view.order.id, view]))
    return selectDayOrders(
      views.map((view) => view.order),
      new Date(),
    )
      .map((order) => byId.get(order.id))
      .filter((view): view is OrderView => view !== undefined)
  }, [views])

  const confirmLogout = useCallback(() => {
    Alert.alert(
      'Cerrar sesión',
      'Los datos pendientes permanecen en este dispositivo y se enviarán cuando vuelvas a entrar con la misma cuenta.',
      [
        { text: 'Cancelar', style: 'cancel' },
        { text: 'Salir', style: 'destructive', onPress: () => void session.logout() },
      ],
    )
  }, [session])

  const reauthenticate = useCallback(() => {
    void session.logout()
  }, [session])

  const manualSync = useCallback(() => {
    void coordinator.syncNow('manual')
  }, [coordinator])

  // El error visible puede venir de la sincronización o de la descarga de
  // órdenes: reintentar debe cubrir ambos caminos.
  const retryAll = useCallback(() => {
    manualSync()
    void reload()
  }, [manualSync, reload])

  if (!ownerId) {
    return (
      <SafeAreaView style={styles.safe}>
        <View style={styles.centered}>
          <Text style={styles.muted}>Sesión no disponible.</Text>
        </View>
      </SafeAreaView>
    )
  }

  return (
    <SafeAreaView style={styles.safe} edges={['top', 'left', 'right']}>
      <ScrollView
        contentContainerStyle={styles.container}
        refreshControl={
          <RefreshControl
            refreshing={refreshing}
            onRefresh={() => void reload()}
            tintColor={colors.primary}
            colors={[colors.primary]}
          />
        }
      >
        <View style={styles.header}>
          <View style={styles.headerText}>
            <Text style={styles.eyebrow}>Ruta de hoy</Text>
            <Text style={styles.name} numberOfLines={1}>
              {snapshot.user?.full_name ?? 'Conductor'}
            </Text>
          </View>
          <Pressable
            onPress={confirmLogout}
            style={({ pressed }) => [styles.logout, pressed && styles.pressed]}
            accessibilityRole="button"
            accessibilityLabel="Cerrar sesión"
          >
            <Text style={styles.logoutLabel}>Salir</Text>
          </Pressable>
        </View>

        <PendingBadge
          pendingPoints={status.pendingPoints}
          pendingTransitions={status.pendingTransitions}
          rejectedPoints={status.rejectedPoints}
          syncing={status.syncing}
        />

        {status.sessionExpired ? (
          <View style={styles.sessionCard}>
            <Text style={styles.sessionTitle}>La sesión caducó</Text>
            <Text style={styles.muted}>
              Tus datos siguen guardados. Vuelve a iniciar sesión para reanudar la sincronización.
            </Text>
            <Pressable
              onPress={reauthenticate}
              style={({ pressed }) => [styles.primaryButton, pressed && styles.pressed]}
            >
              <Text style={styles.primaryLabel}>Iniciar sesión de nuevo</Text>
            </Pressable>
          </View>
        ) : null}

        <OfflineBanner
          online={status.online}
          syncing={status.syncing}
          pendingPoints={status.pendingPoints}
          lastError={status.lastError}
        />

        <GpsCard />

        <View style={styles.sectionHeader}>
          <Text style={styles.sectionTitle}>Órdenes del día</Text>
          <Text style={styles.count}>{dayViews.length}</Text>
        </View>

        {error ? (
          <Pressable onPress={retryAll} style={styles.errorCard}>
            <Text style={styles.errorTitle}>No se pudieron actualizar las órdenes</Text>
            <Text style={styles.errorBody}>{error} Toca para reintentar.</Text>
          </Pressable>
        ) : null}

        {loading ? (
          <View style={styles.centered}>
            <ActivityIndicator color={colors.primary} />
            <Text style={styles.muted}>Cargando órdenes…</Text>
          </View>
        ) : dayViews.length === 0 ? (
          <View style={styles.empty}>
            <Text style={styles.emptyTitle}>Sin órdenes para hoy</Text>
            <Text style={styles.muted}>
              {status.online
                ? 'Desliza hacia abajo para actualizar.'
                : 'Cuando recuperes conexión se descargarán tus órdenes.'}
            </Text>
          </View>
        ) : (
          <View style={styles.list}>
            {dayViews.map((view) => {
              const distance = formatDistance(view.order.distance_meters)
              const pending = view.pendingTransition !== null
              return (
                <Pressable
                  key={view.order.id}
                  onPress={() => navigation.navigate('OrderDetail', { orderId: view.order.id })}
                  style={({ pressed }) => [styles.card, pressed && styles.cardPressed]}
                  accessibilityRole="button"
                  accessibilityLabel={`Orden ${view.order.external_ref}, ${view.effectiveStatus}`}
                >
                  <View style={styles.cardHeader}>
                    <Text style={styles.ref}>{view.order.external_ref}</Text>
                    <StatusChip status={view.effectiveStatus} />
                  </View>
                  <Text style={styles.address} numberOfLines={2}>
                    {view.order.destination_address ?? 'Destino sin dirección'}
                  </Text>
                  <View style={styles.cardFooter}>
                    <Text style={styles.meta}>Creada {formatTime(view.order.placed_at)}</Text>
                    {distance ? <Text style={styles.meta}>{distance}</Text> : null}
                    {pending ? (
                      <Text style={styles.pending}>
                        Cambio pendiente{view.pendingTransition?.attempts ? ' · reintentando' : ''}
                      </Text>
                    ) : null}
                  </View>
                </Pressable>
              )
            })}
          </View>
        )}

        <Text style={styles.footer}>
          Los datos se guardan primero en el dispositivo y se envían por lotes al recuperar
          conexión.
        </Text>
      </ScrollView>
    </SafeAreaView>
  )
}

const styles = StyleSheet.create({
  safe: { flex: 1, backgroundColor: colors.background },
  container: {
    padding: spacing.lg,
    gap: spacing.md,
    paddingBottom: spacing.xxl,
  },
  header: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
  },
  headerText: { gap: 2, flex: 1, marginRight: spacing.md },
  eyebrow: {
    color: colors.muted,
    fontSize: typography.small,
    textTransform: 'uppercase',
    letterSpacing: 1,
  },
  name: { color: colors.text, fontSize: typography.title, fontWeight: '800' },
  logout: {
    paddingHorizontal: spacing.md,
    paddingVertical: spacing.sm,
    borderRadius: radius.md,
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: colors.border,
  },
  logoutLabel: { color: colors.muted, fontSize: typography.small, fontWeight: '600' },
  pressed: { opacity: 0.7 },
  sessionCard: {
    backgroundColor: 'rgba(239, 68, 68, 0.1)',
    borderColor: 'rgba(239, 68, 68, 0.4)',
    borderWidth: StyleSheet.hairlineWidth,
    borderRadius: radius.md,
    padding: spacing.md,
    gap: spacing.sm,
  },
  sessionTitle: { color: '#fca5a5', fontWeight: '700', fontSize: typography.body },
  primaryButton: {
    alignSelf: 'flex-start',
    backgroundColor: colors.primary,
    borderRadius: radius.md,
    paddingHorizontal: spacing.lg,
    paddingVertical: spacing.sm + 2,
  },
  primaryLabel: { color: colors.white, fontWeight: '700', fontSize: typography.small },
  sectionHeader: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    marginTop: spacing.sm,
  },
  sectionTitle: { color: colors.text, fontSize: typography.subtitle, fontWeight: '700' },
  count: {
    color: colors.muted,
    fontSize: typography.small,
    fontWeight: '700',
    backgroundColor: colors.surfaceAlt,
    paddingHorizontal: spacing.sm + 2,
    paddingVertical: 2,
    borderRadius: radius.pill,
    overflow: 'hidden',
  },
  errorCard: {
    backgroundColor: 'rgba(239, 68, 68, 0.1)',
    borderColor: 'rgba(239, 68, 68, 0.35)',
    borderWidth: StyleSheet.hairlineWidth,
    borderRadius: radius.md,
    padding: spacing.md,
    gap: spacing.xs,
  },
  errorTitle: { color: '#fca5a5', fontWeight: '700', fontSize: typography.small },
  errorBody: { color: colors.muted, fontSize: typography.small, lineHeight: 18 },
  centered: { alignItems: 'center', gap: spacing.sm, paddingVertical: spacing.xl },
  muted: { color: colors.muted, fontSize: typography.small, lineHeight: 19 },
  empty: {
    backgroundColor: colors.surface,
    borderRadius: radius.lg,
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: colors.border,
    padding: spacing.xl,
    alignItems: 'center',
    gap: spacing.sm,
  },
  emptyTitle: { color: colors.text, fontWeight: '700', fontSize: typography.body },
  list: { gap: spacing.md },
  card: {
    backgroundColor: colors.surface,
    borderRadius: radius.lg,
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: colors.border,
    padding: spacing.lg,
    gap: spacing.sm,
  },
  cardPressed: { opacity: 0.8, borderColor: colors.primary },
  cardHeader: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    gap: spacing.sm,
  },
  ref: { color: colors.text, fontSize: typography.subtitle, fontWeight: '800' },
  address: { color: colors.muted, fontSize: typography.small, lineHeight: 19 },
  cardFooter: {
    flexDirection: 'row',
    flexWrap: 'wrap',
    alignItems: 'center',
    gap: spacing.md,
  },
  meta: { color: colors.muted, fontSize: typography.tiny },
  pending: { color: '#fbbf24', fontSize: typography.tiny, fontWeight: '700' },
  footer: {
    color: colors.muted,
    fontSize: typography.tiny,
    textAlign: 'center',
    marginTop: spacing.md,
    lineHeight: 16,
  },
})
