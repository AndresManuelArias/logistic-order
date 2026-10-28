import path from 'node:path'
import type { NextConfig } from 'next'

// Portal multi-rol (F4). El backend Go se consume a través del BFF
// (/api/backend/*) para no exponer CORS ni tokens en el navegador.
const nextConfig: NextConfig = {
  output: 'standalone',
  outputFileTracingRoot: path.join(process.cwd(), '../..'),
  reactStrictMode: true,
  poweredByHeader: false,
  transpilePackages: ['@tiendas/tracking-core', '@tiendas/tracking-ui'],
  async headers() {
    return [
      {
        source: '/:path*',
        headers: [
          { key: 'X-Content-Type-Options', value: 'nosniff' },
          { key: 'X-Frame-Options', value: 'DENY' },
          { key: 'Referrer-Policy', value: 'no-referrer' },
          { key: 'Permissions-Policy', value: 'geolocation=(), microphone=(), camera=()' },
        ],
      },
    ]
  },
}

export default nextConfig
