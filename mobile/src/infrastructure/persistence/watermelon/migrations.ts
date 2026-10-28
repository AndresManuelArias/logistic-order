import { schemaMigrations } from '@nozbe/watermelondb/Schema/migrations'

/**
 * Migraciones locales. La v1 no tiene pasos. Para v2 añadir:
 * `{ toVersion: 2, steps: [...] }` y subir `schema.version`.
 */
export const migrations = schemaMigrations({ migrations: [] })
