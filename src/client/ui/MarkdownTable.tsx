import type { ReactNode } from 'react';

export function MarkdownTable({ children }: { children: ReactNode }) {
  return (
    <div
      className="ui-markdown-table"
      role="region"
      aria-label="Scrollable Markdown table"
      tabIndex={0}
    >
      <table>{children}</table>
    </div>
  );
}
