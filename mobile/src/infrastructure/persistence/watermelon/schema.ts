import { appSchema, tableSchema } from '@nozbe/watermelondb'

export const DATABASE_NAME = 'tiendas_track'

/**
 * Esquema local v1.
 *
 * - `orders` guarda el JSON del contrato OpenAPI (ADR-10) más columnas
 *   indexadas para filtrar; así una evolución del DTO no obliga a migrar.
 * - `local_points` y `sync_batches` implementan el write-ahead de idempotencia.
 * - `order_transitions` es la cola offline de cambios de estado.
 * - `app_meta` guarda `device_id`/`device_seq` (monótono por instalación).
 */
export const schema = appSchema({
  version: 1,
  tables: [
    tableSchema({
      name: 'app_meta',
      columns: [{ name: 'value', type: 'string' }],
    }),
    tableSchema({
      name: 'local_points',
      columns: [
        { name: 'owner_id', type: 'string', isIndexed: true },
        { name: 'device_seq', type: 'number', isIndexed: true },
        { name: 'recorded_at', type: 'number' },
        { name: 'lat', type: 'number' },
        { name: 'lon', type: 'number' },
        { name: 'speed_kph', type: 'number', isOptional: true },
        { name: 'accuracy_m', type: 'number', isOptional: true },
        { name: 'heading', type: 'number', isOptional: true },
        { name: 'vehicle_id', type: 'string', isOptional: true },
        { name: 'order_id', type: 'string', isOptional: true },
        { name: 'sync_state', type: 'string', isIndexed: true },
        { name: 'batch_id', type: 'string', isOptional: true, isIndexed: true },
        { name: 'attempts', type: 'number' },
        { name: 'last_error', type: 'string', isOptional: true },
        { name: 'created_at', type: 'number' },
        { name: 'synced_at', type: 'number', isOptional: true },
        { name: 'rejected_at', type: 'number', isOptional: true },
      ],
    }),
    tableSchema({
      name: 'sync_batches',
      columns: [
        { name: 'owner_id', type: 'string', isIndexed: true },
        { name: 'idempotency_key', type: 'string' },
        { name: 'point_ids', type: 'string' },
        { name: 'created_at', type: 'number' },
        { name: 'attempts', type: 'number' },
        { name: 'last_attempt_at', type: 'number', isOptional: true },
        { name: 'next_attempt_at', type: 'number' },
        { name: 'last_error', type: 'string', isOptional: true },
        { name: 'state', type: 'string', isIndexed: true },
        { name: 'closed_at', type: 'number', isOptional: true },
      ],
    }),
    tableSchema({
      name: 'orders',
      columns: [
        { name: 'owner_id', type: 'string', isIndexed: true },
        { name: 'status', type: 'string', isIndexed: true },
        { name: 'placed_at', type: 'number', isIndexed: true },
        { name: 'cached_at', type: 'number' },
        { name: 'payload', type: 'string' },
      ],
    }),
    tableSchema({
      name: 'order_transitions',
      columns: [
        { name: 'owner_id', type: 'string', isIndexed: true },
        { name: 'order_id', type: 'string', isIndexed: true },
        { name: 'expected_status', type: 'string' },
        { name: 'target_status', type: 'string' },
        { name: 'idempotency_key', type: 'string' },
        { name: 'requested_at', type: 'number' },
        { name: 'attempts', type: 'number' },
        { name: 'next_attempt_at', type: 'number' },
        { name: 'last_error', type: 'string', isOptional: true },
        { name: 'state', type: 'string', isIndexed: true },
        { name: 'resolved_at', type: 'number', isOptional: true },
        { name: 'resolution_code', type: 'string', isOptional: true },
      ],
    }),
    tableSchema({
      name: 'delivery_proofs',
      columns: [
        { name: 'owner_id', type: 'string', isIndexed: true },
        { name: 'order_id', type: 'string', isIndexed: true },
        { name: 'receiver_name', type: 'string' },
        { name: 'notes', type: 'string', isOptional: true },
        { name: 'photo_uri', type: 'string', isOptional: true },
        { name: 'captured_at', type: 'number' },
        { name: 'upload_state', type: 'string' },
      ],
    }),
  ],
})
