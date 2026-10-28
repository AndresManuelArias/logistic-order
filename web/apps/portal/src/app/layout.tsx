import type { Metadata, Viewport } from 'next'
import '@tiendas/tracking-ui/styles.css'
import './globals.css'
import { AuthProvider } from '@/components/auth-provider'

export const metadata: Metadata = {
  title: {
    default: 'Tiendas-Track · Visibilidad logística',
    template: '%s · Tiendas-Track',
  },
  description:
    'Portal multi-rol de Tiendas-Track: flota del proveedor, vista dual del tendero y seguimiento de pedidos en tiempo real.',
  robots: { index: false, follow: false },
}

export const viewport: Viewport = {
  themeColor: '#0b1f3a',
  width: 'device-width',
  initialScale: 1,
}

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="es">
      <body>
        <AuthProvider>{children}</AuthProvider>
      </body>
    </html>
  )
}
