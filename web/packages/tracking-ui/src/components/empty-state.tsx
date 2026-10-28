import type { ReactNode } from 'react'

export interface EmptyStateProps {
  title: string
  description?: string
  action?: ReactNode
}

export function EmptyState({ title, description, action }: EmptyStateProps) {
  return (
    <div className="tt-state tt-state--empty" role="status">
      <p className="tt-state__title">{title}</p>
      {description ? <p className="tt-state__text">{description}</p> : null}
      {action ? <div className="tt-state__action">{action}</div> : null}
    </div>
  )
}
