// Tipos derivados del contrato OpenAPI (ADR-10). No se declaran DTOs a mano:
// `npm run gen:api` regenera `generated/openapi.ts` desde
// `backend/api/openapi.yaml`.
import type { components, paths } from './generated/openapi'

export type Schemas = components['schemas']
export type Role = Schemas['Role']
export type User = Schemas['User']
export type TokenPair = Schemas['TokenPair']
export type Order = Schemas['Order']
export type OrderStatus = Schemas['OrderStatus']
export type Point = Schemas['Point']
export type TelemetryPointInput = Schemas['TelemetryPointInput']
export type TelemetryPoint = Schemas['TelemetryPoint']
export type BulkSyncResponse = Schemas['BulkSyncResponse']

export type LoginRequest =
  paths['/api/v1/auth/login']['post']['requestBody']['content']['application/json']
export type RefreshRequest =
  paths['/api/v1/auth/refresh']['post']['requestBody']['content']['application/json']
export type ChangeStatusRequest =
  paths['/api/v1/orders/{id}/status']['patch']['requestBody']['content']['application/json']

export type OrderPage =
  paths['/api/v1/orders']['get']['responses'][200]['content']['application/json']
