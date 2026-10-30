import type { ReactNode } from 'react';

export function MarkdownTable({
  children,
  label = 'Scrollable Markdown table',
}: {
  children: ReactNode;
  label?: string;
}) {
  return (
    <div
      className="ui-markdown-table"
      role="region"
      aria-label={label}
      tabIndex={0}
    >
      <table>{children}</table>
    </div>
  );
}
