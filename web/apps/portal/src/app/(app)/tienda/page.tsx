import type { Metadata } from 'next'
import { StoreDashboard } from '@/components/dashboards/store-dashboard'
import { RequireRole } from '@/components/require-role'

export const metadata: Metadata = {
  title: 'Mi tienda',
}

export default function StorePage() {
  return (
    <RequireRole roles={['STORE']}>
      <StoreDashboard />
    </RequireRole>
  )
}
