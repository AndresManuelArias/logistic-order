import React, { useCallback, useEffect, useState } from 'react'
import {
  ActivityIndicator,
  Alert,
  Image,
  KeyboardAvoidingView,
  Linking,
  Platform,
  Pressable,
  RefreshControl,
  ScrollView,
  StyleSheet,
  Text,
  TextInput,
  View,
} from 'react-native'
import { useHeaderHeight } from '@react-navigation/elements'
import type { NativeStackScreenProps } from '@react-navigation/native-stack'
import * as ImagePicker from 'expo-image-picker'
import { SafeAreaView } from 'react-native-safe-area-context'

import type { OrderStatus } from '../../contracts/types'
import type { OrderView } from '../../application/orders/order-service'
import type { DeliveryProofRecord } from '../../ports/proofs'
import { DELIVERY_PROOF_UPLOAD_SUPPORTED } from '../../domain/delivery-proof'
import { useOwnerId, useServices, useSyncStatus } from '../context/ServicesProvider'
import { describeError } from '../errors'
import { StatusChip } from '../components/StatusChip'
import { colors, radius, spacing, typography } from '../theme'
import type { RootStackParamList } from '../navigation/types'

type Props = NativeStackScreenProps<RootStackParamList, 'OrderDetail'>

function formatDateTime(value: string): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '—'
  return date.toLocaleString('es-CO', {
    day: '2-digit',
    month: 'short',
    hour: '2-digit',
    minute: '2-digit',
  })
}

function formatDistance(meters: number | undefined): string {
  if (typeof meters !== 'number' || !Number.isFinite(meters)) return '—'
  return meters >= 1000 ? `${(meters / 1000).toFixed(1)} km` : `${Math.round(meters)} m`
}

export function OrderDetailScreen({ route }: Props) {
  const { orderId } = route.params
  const { orders, delivery, coordinator, session } = useServices()
  const ownerId = useOwnerId()
  const status = useSyncStatus()
  const headerHeight = useHeaderHeight()

  const [view, setView] = useState<OrderView | null>(null)
  const [proofs, setProofs] = useState<DeliveryProofRecord[]>([])
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const [showDelivery, setShowDelivery] = useState(false)
  const [receiverName, setReceiverName] = useState('')
  const [notes, setNotes] = useState('')
  const [photoUri, setPhotoUri] = useState<string | null>(null)
  const [deliveryError, setDeliveryError] = useState<string | null>(null)

  const load = useCallback(
    async (remote: boolean) => {
      if (!ownerId) return
      try {
        if (remote) {
          const result = await orders.refreshOne(ownerId, orderId)
          setView(result.view)
          if (result.error) setError(describeError(result.error))
        } else {
          setView(await orders.get(ownerId, orderId))
        }
        const [localView, localProofs] = await Promise.all([
          orders.get(ownerId, orderId),
          delivery.listProofs(ownerId, orderId),
        ])
        setView(localView)
        setProofs(localProofs)
      } catch (rawError) {
        setError(describeError(rawError))
      } finally {
        setLoading(false)
      }
    },
    [delivery, orderId, orders, ownerId],
  )

  useEffect(() => {
    void load(true)
  }, [load])

  // Los cambios aplicados por el coordinador se reflejan sin salir de la vista.
  useEffect(() => {
    void load(false)
  }, [load, status.lastSyncedAt, status.pendingTransitions])

  const refresh = useCallback(async () => {
    setRefreshing(true)
    setError(null)
    await load(true)
    setRefreshing(false)
  }, [load])

  const runTransition = useCallback(
    async (target: OrderStatus) => {
      if (!ownerId) return
      setBusy(true)
      setError(null)
      try {
        const result = await orders.requestTransition(ownerId, orderId, target)
        if (result.transition) await coordinator.syncNow('transition')
        await load(false)
      } catch (rawError) {
        setError(describeError(rawError))
      } finally {
        setBusy(false)
      }
    },
    [coordinator, load, orderId, orders, ownerId],
  )

  const confirmAction = useCallback(() => {
    if (!view) return
    const action = orders.nextAction(view)
    if (!action) return

    if (action.target === 'DELIVERED') {
      setShowDelivery(true)
      setDeliveryError(null)
      return
    }

    Alert.alert(action.label, action.confirmation, [
      { text: 'Cancelar', style: 'cancel' },
      { text: 'Confirmar', onPress: () => void runTransition(action.target) },
    ])
  }, [orders, runTransition, view])

  const takePhoto = useCallback(async (source: 'camera' | 'library') => {
    setDeliveryError(null)
    try {
      const permission =
        source === 'camera'
          ? await ImagePicker.requestCameraPermissionsAsync()
          : await ImagePicker.requestMediaLibraryPermissionsAsync()

      if (!permission.granted) {
        setDeliveryError(
          source === 'camera'
            ? 'Permiso de cámara denegado. Puedes elegir una foto de la galería.'
            : 'Permiso de fotos denegado. Actívalo en los ajustes del sistema.',
        )
        return
      }

      const result =
        source === 'camera'
          ? await ImagePicker.launchCameraAsync({ quality: 0.6 })
          : await ImagePicker.launchImageLibraryAsync({ mediaTypes: ['images'], quality: 0.6 })

      if (!result.canceled) {
        setPhotoUri(result.assets[0]?.uri ?? null)
      }
    } catch (rawError) {
      setDeliveryError(describeError(rawError))
    }
  }, [])

  const submitDelivery = useCallback(async () => {
    if (!ownerId) return
    setBusy(true)
    setDeliveryError(null)
    try {
      const result = await delivery.completeDelivery(ownerId, {
        orderId,
        receiverName,
        notes,
        photoUri,
      })

      if (!result.ok) {
        setDeliveryError(result.message)
        if (result.proof) {
          setProofs(await delivery.listProofs(ownerId, orderId))
        }
        await load(false)
        return
      }

      setShowDelivery(false)
      setReceiverName('')
      setNotes('')
      setPhotoUri(null)
      await coordinator.syncNow('transition')
      await load(false)
    } catch (rawError) {
      setDeliveryError(describeError(rawError))
    } finally {
      setBusy(false)
    }
  }, [
    coordinator,
    delivery,
    load,
    notes,
    orderId,
    ownerId,
    photoUri,
    receiverName,
  ])

  const openMaps = useCallback(() => {
    const destination = view?.order.destination
    if (!destination) return
    const url = `https://www.google.com/maps/search/?api=1&query=${destination.lat},${destination.lon}`
    void Linking.openURL(url)
  }, [view])

  if (loading) {
    return (
      <SafeAreaView style={styles.safe}>
        <View style={styles.centered}>
          <ActivityIndicator color={colors.primary} />
          <Text style={styles.muted}>Cargando orden…</Text>
        </View>
      </SafeAreaView>
    )
  }

  if (!view) {
    return (
      <SafeAreaView style={styles.safe}>
        <ScrollView contentContainerStyle={styles.container}>
          <View style={styles.empty}>
            <Text style={styles.emptyTitle}>Orden no disponible en este dispositivo</Text>
            <Text style={styles.muted}>
              Conéctate a la red y actualiza para descargar los datos de la orden.
            </Text>
            {error ? <Text style={styles.errorText}>{error}</Text> : null}
            <Pressable
              onPress={() => void refresh()}
              style={({ pressed }) => [styles.primaryButton, pressed && styles.pressed]}
            >
              <Text style={styles.primaryLabel}>Reintentar</Text>
            </Pressable>
          </View>
        </ScrollView>
      </SafeAreaView>
    )
  }

  const action = orders.nextAction(view)
  const terminal = view.effectiveStatus === 'DELIVERED' || view.effectiveStatus === 'CANCELLED'
  const pending = view.pendingTransition

  return (
    <SafeAreaView style={styles.safe} edges={['left', 'right', 'bottom']}>
      <KeyboardAvoidingView
        style={styles.flex}
        behavior={Platform.OS === 'ios' ? 'padding' : undefined}
        keyboardVerticalOffset={headerHeight}
      >
        <ScrollView
          contentContainerStyle={styles.container}
          keyboardShouldPersistTaps="handled"
          refreshControl={
            <RefreshControl
              refreshing={refreshing}
              onRefresh={() => void refresh()}
              tintColor={colors.primary}
              colors={[colors.primary]}
            />
          }
        >
          {status.sessionExpired ? (
            <View style={styles.warningCard}>
              <Text style={styles.warningTitle}>La sesión caducó</Text>
              <Text style={styles.muted}>
                Los cambios quedan en cola. Vuelve a iniciar sesión para enviarlos.
              </Text>
              <Pressable
                onPress={() => void session.logout()}
                style={({ pressed }) => [styles.primaryButton, pressed && styles.pressed]}
              >
                <Text style={styles.primaryLabel}>Iniciar sesión de nuevo</Text>
              </Pressable>
            </View>
          ) : null}

          <View style={styles.card}>
            <View style={styles.cardHeader}>
              <Text style={styles.ref}>{view.order.external_ref}</Text>
              <StatusChip status={view.effectiveStatus} />
            </View>

            {pending ? (
              <View style={styles.pendingBanner}>
                <Text style={styles.pendingTitle}>
                  Cambio pendiente de sincronizar → {pending.targetStatus}
                </Text>
                <Text style={styles.muted}>
                  {pending.attempts > 0
                    ? `Reintentando (${pending.attempts}). Se enviará automáticamente.`
                    : 'Se enviará automáticamente al recuperar conexión.'}
                </Text>
              </View>
            ) : null}

            <View style={styles.infoRow}>
              <Text style={styles.infoLabel}>Destino</Text>
              <Text style={styles.infoValue}>{view.order.destination_address ?? '—'}</Text>
            </View>
            <View style={styles.infoRow}>
              <Text style={styles.infoLabel}>Creada</Text>
              <Text style={styles.infoValue}>{formatDateTime(view.order.placed_at)}</Text>
            </View>
            <View style={styles.infoRow}>
              <Text style={styles.infoLabel}>Distancia</Text>
              <Text style={styles.infoValue}>{formatDistance(view.order.distance_meters)}</Text>
            </View>
            <View style={styles.infoRow}>
              <Text style={styles.infoLabel}>Coordenadas</Text>
              <Text style={styles.infoValue}>
                {view.order.destination.lat.toFixed(5)}, {view.order.destination.lon.toFixed(5)}
              </Text>
            </View>

            <Pressable
              onPress={openMaps}
              style={({ pressed }) => [styles.linkButton, pressed && styles.pressed]}
            >
              <Text style={styles.linkLabel}>Abrir destino en mapas</Text>
            </Pressable>
          </View>

          {error ? <Text style={styles.errorText}>{error}</Text> : null}

          {action ? (
            <Pressable
              onPress={confirmAction}
              disabled={busy}
              style={({ pressed }) => [
                styles.primaryButton,
                styles.actionButton,
                (pressed || busy) && styles.pressed,
              ]}
            >
              {busy ? (
                <ActivityIndicator color={colors.white} />
              ) : (
                <Text style={styles.primaryLabel}>{action.label}</Text>
              )}
            </Pressable>
          ) : (
            <View style={styles.terminalCard}>
              <Text style={styles.terminalTitle}>
                {terminal ? 'Esta entrega ya está cerrada' : 'Sin acciones disponibles'}
              </Text>
              <Text style={styles.muted}>
                {view.effectiveStatus === 'DELIVERED'
                  ? 'La orden figura como entregada.'
                  : view.effectiveStatus === 'CANCELLED'
                    ? 'La orden fue cancelada.'
                    : 'Actualiza la orden para ver si hay una acción disponible.'}
              </Text>
            </View>
          )}

          {showDelivery ? (
            <View style={styles.card}>
              <Text style={styles.sectionTitle}>Prueba de entrega</Text>
              <Text style={styles.muted}>
                {DELIVERY_PROOF_UPLOAD_SUPPORTED
                  ? 'La evidencia se enviará al servidor.'
                  : 'La evidencia se guarda sólo en este dispositivo: el backend actual no expone un endpoint de prueba de entrega.'}
              </Text>

              <Text style={styles.label}>Nombre de quien recibe *</Text>
              <TextInput
                value={receiverName}
                onChangeText={setReceiverName}
                placeholder="Ej. Ana Pérez"
                placeholderTextColor={colors.muted}
                style={styles.input}
                editable={!busy}
                maxLength={120}
              />

              <Text style={styles.label}>Notas (opcional)</Text>
              <TextInput
                value={notes}
                onChangeText={setNotes}
                placeholder="Portería, observaciones…"
                placeholderTextColor={colors.muted}
                style={[styles.input, styles.textArea]}
                editable={!busy}
                multiline
                numberOfLines={3}
                maxLength={500}
              />

              <View style={styles.photoActions}>
                <Pressable
                  onPress={() => void takePhoto('camera')}
                  disabled={busy}
                  style={({ pressed }) => [styles.secondaryButton, pressed && styles.pressed]}
                >
                  <Text style={styles.secondaryLabel}>Tomar foto</Text>
                </Pressable>
                <Pressable
                  onPress={() => void takePhoto('library')}
                  disabled={busy}
                  style={({ pressed }) => [styles.secondaryButton, pressed && styles.pressed]}
                >
                  <Text style={styles.secondaryLabel}>Elegir de galería</Text>
                </Pressable>
                {photoUri ? (
                  <Pressable
                    onPress={() => setPhotoUri(null)}
                    disabled={busy}
                    style={({ pressed }) => [styles.secondaryButton, pressed && styles.pressed]}
                  >
                    <Text style={styles.secondaryLabel}>Quitar foto</Text>
                  </Pressable>
                ) : null}
              </View>

              {photoUri ? <Image source={{ uri: photoUri }} style={styles.photo} /> : null}

              {deliveryError ? <Text style={styles.errorText}>{deliveryError}</Text> : null}

              <Pressable
                onPress={() => void submitDelivery()}
                disabled={busy || receiverName.trim().length === 0}
                style={({ pressed }) => [
                  styles.primaryButton,
                  (pressed || busy || receiverName.trim().length === 0) && styles.disabled,
                ]}
              >
                {busy ? (
                  <ActivityIndicator color={colors.white} />
                ) : (
                  <Text style={styles.primaryLabel}>Confirmar entrega</Text>
                )}
              </Pressable>

              <Pressable
                onPress={() => {
                  setShowDelivery(false)
                  setDeliveryError(null)
                }}
                disabled={busy}
                style={({ pressed }) => [styles.linkButton, pressed && styles.pressed]}
              >
                <Text style={styles.linkLabel}>Cancelar</Text>
              </Pressable>
            </View>
          ) : null}

          {proofs.length > 0 ? (
            <View style={styles.card}>
              <Text style={styles.sectionTitle}>Evidencias guardadas</Text>
              {proofs.map((proof) => (
                <View key={proof.id} style={styles.proofRow}>
                  <Text style={styles.proofName}>{proof.receiverName}</Text>
                  <Text style={styles.muted}>
                    {formatDateTime(new Date(proof.capturedAt).toISOString())}
                    {proof.notes ? ` · ${proof.notes}` : ''}
                  </Text>
                  <Text style={styles.proofState}>
                    {proof.photoUri ? 'Con foto · ' : ''}Sólo en este dispositivo (pendiente de
                    backend)
                  </Text>
                </View>
              ))}
            </View>
          ) : null}

          <Text style={styles.footer}>
            Los cambios de estado se aplican de forma optimista y se confirman con el servidor.
          </Text>
        </ScrollView>
      </KeyboardAvoidingView>
    </SafeAreaView>
  )
}

const styles = StyleSheet.create({
  safe: { flex: 1, backgroundColor: colors.background },
  flex: { flex: 1 },
  container: { padding: spacing.lg, gap: spacing.md, paddingBottom: spacing.xxl },
  centered: { flex: 1, alignItems: 'center', justifyContent: 'center', gap: spacing.sm },
  muted: { color: colors.muted, fontSize: typography.small, lineHeight: 19 },
  card: {
    backgroundColor: colors.surface,
    borderRadius: radius.lg,
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: colors.border,
    padding: spacing.lg,
    gap: spacing.md,
  },
  cardHeader: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    gap: spacing.sm,
  },
  ref: { color: colors.text, fontSize: typography.title, fontWeight: '800' },
  pendingBanner: {
    backgroundColor: 'rgba(245, 158, 11, 0.1)',
    borderColor: 'rgba(245, 158, 11, 0.4)',
    borderWidth: StyleSheet.hairlineWidth,
    borderRadius: radius.md,
    padding: spacing.md,
    gap: spacing.xs,
  },
  pendingTitle: { color: '#fbbf24', fontWeight: '700', fontSize: typography.small },
  warningCard: {
    backgroundColor: 'rgba(239, 68, 68, 0.1)',
    borderColor: 'rgba(239, 68, 68, 0.4)',
    borderWidth: StyleSheet.hairlineWidth,
    borderRadius: radius.md,
    padding: spacing.md,
    gap: spacing.sm,
  },
  warningTitle: { color: '#fca5a5', fontWeight: '700', fontSize: typography.body },
  infoRow: { flexDirection: 'row', gap: spacing.md, alignItems: 'flex-start' },
  infoLabel: {
    color: colors.muted,
    fontSize: typography.small,
    width: 92,
    fontWeight: '600',
  },
  infoValue: { color: colors.text, fontSize: typography.small, flex: 1, lineHeight: 19 },
  linkButton: {
    alignSelf: 'flex-start',
    paddingVertical: spacing.sm,
    paddingHorizontal: spacing.md,
    borderRadius: radius.md,
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: colors.border,
    backgroundColor: colors.surfaceAlt,
  },
  linkLabel: { color: '#93c5fd', fontSize: typography.small, fontWeight: '600' },
  primaryButton: {
    backgroundColor: colors.primary,
    borderRadius: radius.md,
    minHeight: 48,
    paddingHorizontal: spacing.lg,
    alignItems: 'center',
    justifyContent: 'center',
  },
  actionButton: { marginTop: spacing.xs },
  primaryLabel: { color: colors.white, fontWeight: '700', fontSize: typography.body },
  disabled: { opacity: 0.5 },
  pressed: { opacity: 0.75 },
  secondaryButton: {
    backgroundColor: colors.surfaceAlt,
    borderRadius: radius.md,
    paddingHorizontal: spacing.md,
    paddingVertical: spacing.sm + 2,
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: colors.border,
  },
  secondaryLabel: { color: colors.text, fontSize: typography.small, fontWeight: '600' },
  photoActions: { flexDirection: 'row', flexWrap: 'wrap', gap: spacing.sm },
  photo: { width: '100%', height: 180, borderRadius: radius.md, backgroundColor: colors.surfaceAlt },
  label: { color: colors.muted, fontSize: typography.small, fontWeight: '600' },
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
  textArea: { minHeight: 84, textAlignVertical: 'top' },
  sectionTitle: { color: colors.text, fontSize: typography.subtitle, fontWeight: '700' },
  errorText: { color: '#fca5a5', fontSize: typography.small, lineHeight: 19 },
  terminalCard: {
    backgroundColor: colors.surface,
    borderRadius: radius.lg,
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: colors.border,
    padding: spacing.lg,
    gap: spacing.xs,
  },
  terminalTitle: { color: colors.text, fontWeight: '700', fontSize: typography.body },
  empty: {
    backgroundColor: colors.surface,
    borderRadius: radius.lg,
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: colors.border,
    padding: spacing.xl,
    alignItems: 'center',
    gap: spacing.sm,
  },
  emptyTitle: { color: colors.text, fontWeight: '700', fontSize: typography.body, textAlign: 'center' },
  proofRow: { gap: 2, paddingVertical: spacing.sm, borderTopWidth: StyleSheet.hairlineWidth, borderTopColor: colors.border },
  proofName: { color: colors.text, fontWeight: '700', fontSize: typography.small },
  proofState: { color: '#fbbf24', fontSize: typography.tiny },
  footer: {
    color: colors.muted,
    fontSize: typography.tiny,
    textAlign: 'center',
    lineHeight: 16,
  },
})
