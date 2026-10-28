import type { Metadata } from 'next'
import { ProviderDashboard } from '@/components/dashboards/provider-dashboard'
import { RequireRole } from '@/components/require-role'

export const metadata: Metadata = {
  title: 'Proveedor',
}

export default function ProviderPage() {
  return (
    <RequireRole roles={['PROVIDER']}>
      <ProviderDashboard />
    </RequireRole>
  )
}
