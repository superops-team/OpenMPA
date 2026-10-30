import { ChevronDown } from 'lucide-react';
import type { ReactNode } from 'react';

export function Disclosure({
  open,
  onToggle,
  label,
  ariaLabel = label,
  toggleLabel,
  meta,
  children,
  className = '',
}: {
  open: boolean;
  onToggle: () => void;
  label: string;
  ariaLabel?: string;
  toggleLabel?: string;
  meta?: ReactNode;
  children: ReactNode;
  className?: string;
}) {
  return (
    <section className={`ui-disclosure ${className}`.trim()}>
      <button
        type="button"
        className="ui-disclosure-trigger"
        aria-expanded={open}
        aria-label={
          toggleLabel ??
          `${open ? 'Collapse' : 'Expand'} ${ariaLabel.toLowerCase()}`
        }
        onClick={onToggle}
      >
        <span>{label}</span>
        {meta && <span className="ui-disclosure-meta">{meta}</span>}
        <ChevronDown aria-hidden="true" />
      </button>
      {open && <div className="ui-disclosure-content">{children}</div>}
    </section>
  );
}
