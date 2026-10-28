export interface LoadingPanelProps {
  label?: string
}

export function LoadingPanel({ label = 'Cargando…' }: LoadingPanelProps) {
  return (
    <div className="tt-state tt-state--loading" role="status" aria-live="polite">
      <span className="tt-spinner" aria-hidden="true" />
      <span>{label}</span>
    </div>
  )
}
