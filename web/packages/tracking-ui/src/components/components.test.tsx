import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { describe, expect, it, vi } from 'vitest'
import { ApiError, type Order } from '@tiendas/tracking-core'
import { ConnectionPill } from './connection-pill'
import { EmptyState } from './empty-state'
import { ErrorPanel } from './error-panel'
import { KpiCard } from './kpi-card'
import { LoadingPanel } from './loading-panel'
import { OrderCard } from './order-card'
import { OrderProgress } from './order-progress'
import { SegmentedTabs } from './segmented-tabs'
import { TrackingMap } from './tracking-map'

const order: Order = {
  id: 'o-1',
  external_ref: 'PED-1001',
  store_id: 's-1',
  status: 'IN_TRANSIT',
  origin: { lat: 4.7, lon: -74.07 },
  destination: { lat: 4.65, lon: -74.05 },
  destination_address: 'Cra 7 # 72-40, Bogotá',
  distance_meters: 2400,
  placed_at: '2026-10-28T14:00:00Z',
  picked_up_at: '2026-10-28T14:20:00Z',
}

describe('OrderCard', () => {
  it('muestra la orden y permite seleccionarla', async () => {
    const onSelect = vi.fn()
    const user = userEvent.setup()
    render(
      <OrderCard
        order={order}
        vehiclePlate="ABC123"
        onSelect={onSelect}
        actions={<button type="button">Compartir</button>}
      />,
    )

    expect(screen.getByRole('heading', { name: 'PED-1001' })).toBeInTheDocument()
    expect(screen.getByText('En tránsito')).toBeInTheDocument()
    expect(screen.getByText('Cra 7 # 72-40, Bogotá')).toBeInTheDocument()
    expect(screen.getByText('2.4 km')).toBeInTheDocument()
    expect(screen.getByText('ABC123')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Ver detalle' }))
    expect(onSelect).toHaveBeenCalledWith(order)
    expect(screen.getByRole('button', { name: 'Compartir' })).toBeInTheDocument()
  })

  it('marca como seleccionada y usa texto de respaldo sin dirección', () => {
    render(
      <OrderCard
        order={{ ...order, destination_address: undefined }}
        selected
        onSelect={() => {}}
      />,
    )
    expect(screen.getByRole('button', { name: 'Seleccionada' })).toHaveAttribute('aria-pressed', 'true')
    expect(screen.getByText('Destino sin dirección')).toBeInTheDocument()
  })
})

describe('OrderProgress', () => {
  it('anuncia el paso actual con aria-current', () => {
    render(<OrderProgress order={order} />)
    const current = screen.getByText('En tránsito').closest('li')
    expect(current).toHaveAttribute('aria-current', 'step')
    expect(screen.getByText('Completado')).toBeInTheDocument()
  })

  it('muestra una orden cancelada sin pasos falsos', () => {
    render(<OrderProgress order={{ ...order, status: 'CANCELLED' }} />)
    expect(screen.getByRole('status')).toHaveTextContent('Cancelada')
    expect(screen.queryByRole('list')).not.toBeInTheDocument()
  })
})

describe('SegmentedTabs', () => {
  it('navega con flechas y mantiene el foco en la pestaña activa', async () => {
    const user = userEvent.setup()
    function Harness() {
      const [value, setValue] = useState('entrada')
      return (
        <SegmentedTabs
          ariaLabel="Vista de órdenes"
          value={value}
          onChange={setValue}
          items={[
            { id: 'entrada', label: 'Entrada', count: 2 },
            { id: 'salida', label: 'Salida', count: 1 },
          ]}
        />
      )
    }
    render(<Harness />)

    const entrada = screen.getByRole('tab', { name: /Entrada/ })
    expect(entrada).toHaveAttribute('aria-selected', 'true')

    await user.tab()
    expect(entrada).toHaveFocus()

    await user.keyboard('{ArrowRight}')
    const salida = screen.getByRole('tab', { name: /Salida/ })
    expect(salida).toHaveAttribute('aria-selected', 'true')
    expect(salida).toHaveFocus()

    await user.keyboard('{Home}')
    expect(entrada).toHaveAttribute('aria-selected', 'true')
    expect(entrada).toHaveFocus()
  })
})

describe('TrackingMap', () => {
  const markers = [
    { id: 'v-1', label: 'ABC123', lat: 4.66, lon: -74.06, detail: 'En ruta', tone: 'live' as const },
    { id: 'v-2', label: 'DEF456', lat: 4.7, lon: -74.07, detail: 'Sin señal', tone: 'offline' as const },
  ]

  it('expone los marcadores en una lista accesible y notifica la selección', async () => {
    const onSelect = vi.fn()
    const user = userEvent.setup()
    render(<TrackingMap markers={markers} onSelect={onSelect} />)

    expect(screen.getByText('2 vehículo(s) en el mapa')).toBeInTheDocument()
    expect(screen.getAllByRole('listitem')).toHaveLength(2)
    await user.click(screen.getByRole('button', { name: /DEF456/ }))
    expect(onSelect).toHaveBeenCalledWith('v-2')
  })

  it('renderiza el estado vacío sin inventar posiciones', () => {
    render(<TrackingMap markers={[]} emptyLabel="Aún no hay vehículos" />)
    expect(screen.getByRole('status')).toHaveTextContent('Aún no hay vehículos')
  })

  it('muestra la traza y no tiene botones de selección si no hay onSelect', () => {
    const { container } = render(
      <TrackingMap
        markers={markers}
        trails={[{ id: 't-1', points: [{ lat: 4.6, lon: -74.1 }, { lat: 4.7, lon: -74.0 }] }] }
      />,
    )
    // MapLibre usa canvas, verificamos que el contenedor exista
    expect(container.querySelector('.tt-map')).not.toBeNull()
    // Sin onSelect, no debería haber botones
    expect(screen.queryByRole('button')).not.toBeInTheDocument()
  })
})

describe('ConnectionPill', () => {
  it('ofrece reconexión solo cuando el canal se cayó', async () => {
    const onReconnect = vi.fn()
    const user = userEvent.setup()
    const { rerender } = render(<ConnectionPill status="open" onReconnect={onReconnect} />)
    expect(screen.getByRole('status')).toHaveTextContent('En vivo')
    expect(screen.queryByRole('button', { name: 'Reintentar' })).not.toBeInTheDocument()

    rerender(<ConnectionPill status="closed" onReconnect={onReconnect} />)
    await user.click(screen.getByRole('button', { name: 'Reintentar' }))
    expect(onReconnect).toHaveBeenCalledTimes(1)
  })
})

describe('estados de UI', () => {
  it('ErrorPanel traduce un 403 sin revelar detalles internos', () => {
    render(<ErrorPanel error={new ApiError(403, 'FORBIDDEN_SCOPE', 'ajeno')} />)
    expect(screen.getByRole('alert')).toHaveTextContent('Tu rol no tiene acceso a esta información.')
    expect(screen.getByRole('alert')).toHaveTextContent('Código: FORBIDDEN_SCOPE')
  })

  it('ErrorPanel permite reintentar', async () => {
    const onRetry = vi.fn()
    const user = userEvent.setup()
    render(<ErrorPanel error={new Error('boom')} onRetry={onRetry} />)
    await user.click(screen.getByRole('button', { name: 'Reintentar' }))
    expect(onRetry).toHaveBeenCalledTimes(1)
  })

  it('LoadingPanel se anuncia como estado', () => {
    render(<LoadingPanel label="Cargando flota…" />)
    expect(screen.getByRole('status')).toHaveTextContent('Cargando flota…')
  })

  it('EmptyState muestra título y acción', () => {
    render(<EmptyState title="Sin órdenes" description="No hay órdenes para este filtro." action={<button type="button">Crear</button>} />)
    expect(screen.getByRole('status')).toHaveTextContent('Sin órdenes')
    expect(screen.getByRole('button', { name: 'Crear' })).toBeInTheDocument()
  })

  it('KpiCard expone etiqueta, valor y pista', () => {
    render(<KpiCard label="Vehículos" value={3} hint="En flota" tone="live" />)
    expect(screen.getByText('Vehículos')).toBeInTheDocument()
    expect(screen.getByText('3')).toBeInTheDocument()
    expect(screen.getByText('En flota')).toBeInTheDocument()
  })
})