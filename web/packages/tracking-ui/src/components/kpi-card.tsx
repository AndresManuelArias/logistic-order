import type { ReactNode } from 'react'

export interface KpiCardProps {
  label: string
  value: ReactNode
  hint?: string
  tone?: 'live' | 'pending' | 'offline' | 'danger' | 'neutral'
}

export function KpiCard({ label, value, hint, tone = 'neutral' }: KpiCardProps) {
  return (
    <div className={`tt-kpi tt-kpi--${tone}`}>
      <span className="tt-kpi__label">{label}</span>
      <span className="tt-kpi__value">{value}</span>
      {hint ? <span className="tt-kpi__hint">{hint}</span> : null}
    </div>
  )
}
