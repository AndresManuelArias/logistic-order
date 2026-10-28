import type { Metadata } from 'next'
import { DriverDashboard } from '@/components/dashboards/driver-dashboard'
import { RequireRole } from '@/components/require-role'

export const metadata: Metadata = {
  title: 'Mis entregas',
}

export default function DriverPage() {
  return (
    <RequireRole roles={['DRIVER']}>
      <DriverDashboard />
    </RequireRole>
  )
}
