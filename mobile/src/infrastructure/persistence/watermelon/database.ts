import { Database } from '@nozbe/watermelondb'
import SQLiteAdapter from '@nozbe/watermelondb/adapters/sqlite'

import { migrations } from './migrations'
import {
  AppMetaModel,
  DeliveryProofModel,
  LocalPointModel,
  OrderModel,
  OrderTransitionModel,
  SyncBatchModel,
} from './models'
import { DATABASE_NAME, schema } from './schema'

/**
 * SQLite síncrono (JSI) está desactivado a propósito: la app fija la
 * arquitectura antigua de React Native (WatermelonDB 0.28) y el adaptador por
 * bridge es el más estable en headless/background. La frecuencia (1 muestra
 * cada 15 s) no justifica el riesgo.
 */
export function createDatabase(onSetUpError?: (error: Error) => void): Database {
  const adapter = new SQLiteAdapter({
    dbName: DATABASE_NAME,
    schema,
    migrations,
    jsi: false,
    onSetUpError: (error) => {
      // No se silencia: una BD corrupta debe poder reiniciarse desde la UI.
      console.error('[watermelon] error al inicializar la base local', error)
      onSetUpError?.(error)
    },
  })

  return new Database({
    adapter,
    modelClasses: [
      AppMetaModel,
      LocalPointModel,
      SyncBatchModel,
      OrderModel,
      OrderTransitionModel,
      DeliveryProofModel,
    ],
  })
}
