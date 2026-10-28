import type { Role } from '../api/types'

// Reglas de navegación por rol (la autorización real vive en el backend).
const HOME_BY_ROLE: Record<Role, string> = {
  PROVIDER: '/proveedor',
  STORE: '/tienda',
  DRIVER: '/conductor',
}

export const ROLE_LABEL: Record<Role, string> = {
  PROVIDER: 'Proveedor',
  STORE: 'Tendero',
  DRIVER: 'Conductor',
}

export function homeForRole(role: Role): string {
  return HOME_BY_ROLE[role] ?? '/login'
}

export function isRoleAllowed(role: Role, allowed: readonly Role[]): boolean {
  return allowed.includes(role)
}
