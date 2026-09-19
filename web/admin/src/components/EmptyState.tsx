import type { ReactElement, ReactNode } from 'react';

interface EmptyStateProps {
  title: string;
  description?: string;
  action?: ReactNode;
}

export function EmptyState({ title, description, action }: EmptyStateProps): ReactElement {
  return (
    <div className="empty-state">
      <h2>{title}</h2>
      {description !== undefined ? <p>{description}</p> : null}
      {action}
    </div>
  );
}
