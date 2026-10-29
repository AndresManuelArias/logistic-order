// Tipos derivados del contrato OpenAPI (ADR-10). No se declaran DTOs a mano:
// `npm run gen:api` regenera este archivo desde backend/api/openapi.yaml.
import type { components, paths } from './generated/openapi'

export type Schemas = components['schemas']
export type Role = Schemas['Role']
export type User = Schemas['User']
export type TokenPair = Schemas['TokenPair']
export type Store = Schemas['Store']
export type Order = Schemas['Order']
export type OrderStatus = Schemas['OrderStatus']
export type Vehicle = Schemas['Vehicle']
export type TelemetryPoint = Schemas['TelemetryPoint']
export type PointInput = Schemas['TelemetryPointInput']
export type BulkSyncResponse = Schemas['BulkSyncResponse']
export type TrackResponse = Schemas['TrackResponse']
export type CreateStoreInput = Schemas['CreateStoreRequest']
export type CreateOrderInput = Schemas['CreateOrderRequest']

export type OrderPage =
  paths['/api/v1/orders']['get']['responses'][200]['content']['application/json']

export type GuestTokenResponse =
  paths['/api/v1/auth/guest-token']['post']['responses'][201]['content']['application/json']

export type VehiclePage =
  paths['/api/v1/vehicles']['get']['responses'][200]['content']['application/json']

export type StorePage =
  paths['/api/v1/stores']['get']['responses'][200]['content']['application/json']

// Driver no está en el OpenAPI generado (módulo nuevo); se declara aquí de forma manual.
export interface Driver {
  id: string
  user_id: string
  full_name: string
  license_number: string
  vehicle_id: string
}

export interface DriverPage {
  items: Driver[]
  total: number
}
