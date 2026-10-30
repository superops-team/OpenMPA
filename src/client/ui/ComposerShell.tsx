import type { HTMLAttributes, ReactNode } from 'react';

export function ComposerShell({
  children,
  className = '',
  ...props
}: HTMLAttributes<HTMLDivElement>) {
  return (
    <div className={`ui-composer-shell ${className}`.trim()} {...props}>
      {children}
    </div>
  );
}

export function ComposerFooter({
  children,
  className = '',
  ...props
}: HTMLAttributes<HTMLDivElement> & { children: ReactNode }) {
  return (
    <div className={`ui-composer-footer ${className}`.trim()} {...props}>
      {children}
    </div>
  );
}
