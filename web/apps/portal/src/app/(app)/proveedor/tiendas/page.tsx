import type { Metadata } from 'next'
import { ProviderStores } from '@/components/provider-stores'
import { RequireRole } from '@/components/require-role'

export const metadata: Metadata = {
  title: 'Tiendas | Proveedor',
}

export default function ProviderStoresPage() {
  return (
    <RequireRole roles={['PROVIDER']}>
      <ProviderStores />
    </RequireRole>
  )
}
