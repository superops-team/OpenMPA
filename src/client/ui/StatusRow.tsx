import type { ReactNode } from 'react';

export type StatusTone =
  'neutral' | 'success' | 'warning' | 'destructive' | 'progress';

export function StatusRow({
  icon,
  title,
  description,
  status,
  tone = 'neutral',
  action,
}: {
  icon: ReactNode;
  title: string;
  description?: string;
  status: string;
  tone?: StatusTone;
  action?: ReactNode;
}) {
  return (
    <div className="ui-status-row" data-tone={tone}>
      <span className="ui-status-row-icon">{icon}</span>
      <span className="ui-status-row-copy">
        <strong>{title}</strong>
        {description && <small title={description}>{description}</small>}
      </span>
      <span className="ui-status-row-state">{status}</span>
      {action}
    </div>
  );
}
