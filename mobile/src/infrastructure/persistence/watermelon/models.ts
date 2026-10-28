import { Model } from '@nozbe/watermelondb'
import { field } from '@nozbe/watermelondb/decorators'

/** Clave/valor interno: `device_id`, `device_seq`. */
export class AppMetaModel extends Model {
  static table = 'app_meta'

  @field('value') value!: string
}

export class LocalPointModel extends Model {
  static table = 'local_points'

  @field('owner_id') ownerId!: string
  @field('device_seq') deviceSeq!: number
  @field('recorded_at') recordedAt!: number
  @field('lat') lat!: number
  @field('lon') lon!: number
  @field('speed_kph') speedKph!: number | null
  @field('accuracy_m') accuracyM!: number | null
  @field('heading') heading!: number | null
  @field('vehicle_id') vehicleId!: string | null
  @field('order_id') orderId!: string | null
  @field('sync_state') syncState!: string
  @field('batch_id') batchId!: string | null
  @field('attempts') attempts!: number
  @field('last_error') lastError!: string | null
  @field('created_at') createdAt!: number
  @field('synced_at') syncedAt!: number | null
  @field('rejected_at') rejectedAt!: number | null
}

export class SyncBatchModel extends Model {
  static table = 'sync_batches'

  @field('owner_id') ownerId!: string
  @field('idempotency_key') idempotencyKey!: string
  @field('point_ids') pointIds!: string
  @field('created_at') createdAt!: number
  @field('attempts') attempts!: number
  @field('last_attempt_at') lastAttemptAt!: number | null
  @field('next_attempt_at') nextAttemptAt!: number
  @field('last_error') lastError!: string | null
  @field('state') state!: string
  @field('closed_at') closedAt!: number | null
}

export class OrderModel extends Model {
  static table = 'orders'

  @field('owner_id') ownerId!: string
  @field('status') status!: string
  @field('placed_at') placedAt!: number
  @field('cached_at') cachedAt!: number
  @field('payload') payload!: string
}

export class OrderTransitionModel extends Model {
  static table = 'order_transitions'

  @field('owner_id') ownerId!: string
  @field('order_id') orderId!: string
  @field('expected_status') expectedStatus!: string
  @field('target_status') targetStatus!: string
  @field('idempotency_key') idempotencyKey!: string
  @field('requested_at') requestedAt!: number
  @field('attempts') attempts!: number
  @field('next_attempt_at') nextAttemptAt!: number
  @field('last_error') lastError!: string | null
  @field('state') state!: string
  @field('resolved_at') resolvedAt!: number | null
  @field('resolution_code') resolutionCode!: string | null
}

export class DeliveryProofModel extends Model {
  static table = 'delivery_proofs'

  @field('owner_id') ownerId!: string
  @field('order_id') orderId!: string
  @field('receiver_name') receiverName!: string
  @field('notes') notes!: string | null
  @field('photo_uri') photoUri!: string | null
  @field('captured_at') capturedAt!: number
  @field('upload_state') uploadState!: string
}
