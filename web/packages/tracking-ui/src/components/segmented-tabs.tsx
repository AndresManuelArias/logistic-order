'use client'

import { useRef, type KeyboardEvent } from 'react'

export interface SegmentedTab {
  id: string
  label: string
  count?: number
}

export interface SegmentedTabsProps {
  items: readonly SegmentedTab[]
  value: string
  onChange: (id: string) => void
  ariaLabel: string
}

// Ids estables para emparejar tab y panel. Cada grupo de pestañas de una
// página debe usar ids propios (p. ej. "entrada"/"salida").
export const tabDomId = (tabId: string): string => `tt-tab-${tabId}`
export const tabPanelDomId = (tabId: string): string => `tt-panel-${tabId}`

// Pestañas segmentadas con patrón ARIA tabs y navegación por teclado
// (flechas, Home y End). El foco sigue a la selección.
export function SegmentedTabs({ items, value, onChange, ariaLabel }: SegmentedTabsProps) {
  const refs = useRef<Array<HTMLButtonElement | null>>([])

  function moveTo(index: number) {
    const next = items[index]
    if (!next) return
    onChange(next.id)
    refs.current[index]?.focus()
  }

  function handleKeyDown(event: KeyboardEvent<HTMLButtonElement>, index: number) {
    let target: number | null = null
    switch (event.key) {
      case 'ArrowRight':
        target = (index + 1) % items.length
        break
      case 'ArrowLeft':
        target = (index - 1 + items.length) % items.length
        break
      case 'Home':
        target = 0
        break
      case 'End':
        target = items.length - 1
        break
      default:
        return
    }
    event.preventDefault()
    moveTo(target)
  }

  return (
    <div className="tt-tabs" role="tablist" aria-label={ariaLabel}>
      {items.map((item, index) => {
        const selected = item.id === value
        return (
          <button
            key={item.id}
            ref={(node) => {
              refs.current[index] = node
            }}
            type="button"
            role="tab"
            id={tabDomId(item.id)}
            aria-controls={tabPanelDomId(item.id)}
            aria-selected={selected}
            tabIndex={selected ? 0 : -1}
            className={`tt-tabs__tab${selected ? ' is-selected' : ''}`}
            onClick={() => onChange(item.id)}
            onKeyDown={(event) => handleKeyDown(event, index)}
          >
            <span>{item.label}</span>
            {typeof item.count === 'number' ? <span className="tt-tabs__count">{item.count}</span> : null}
          </button>
        )
      })}
    </div>
  )
}
