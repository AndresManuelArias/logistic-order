import type { Metadata } from 'next'
import { GuestTracking } from '@/components/guest-tracking'

export const metadata: Metadata = {
  title: 'Seguimiento de tu pedido',
  robots: { index: false, follow: false },
}

// La vista es cliente porque el token de invitado viaja en el fragmento de la
// URL y nunca llega al servidor del portal.
export default function TrackingPage() {
  return <GuestTracking />
}
