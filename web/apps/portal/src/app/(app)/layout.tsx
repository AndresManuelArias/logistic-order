import { AppShell } from '@/components/app-shell'
import type { ReactNode } from 'react'

// Layout del portal autenticado: cabecera, navegación por rol y contenido.
// Los guardas de rol viven en cada página (RequireRole).
export default function PortalLayout({ children }: { children: ReactNode }) {
  return <AppShell>{children}</AppShell>
}
