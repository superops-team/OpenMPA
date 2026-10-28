import {
  Children,
  isValidElement,
  type ComponentPropsWithoutRef,
  type ReactNode,
} from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { CodeBlock, MarkdownTable } from '../ui';

function safeHTTPURL(value: string | undefined) {
  if (!value) return undefined;
  try {
    const url = new URL(value);
    return url.protocol === 'http:' || url.protocol === 'https:'
      ? value
      : undefined;
  } catch {
    return undefined;
  }
}

function textFromNode(value: ReactNode): string {
  if (typeof value === 'string' || typeof value === 'number')
    return String(value);
  if (Array.isArray(value)) return value.map(textFromNode).join('');
  if (isValidElement<{ children?: ReactNode }>(value))
    return textFromNode(value.props.children);
  return '';
}

export function AssistantMarkdown({ text }: { text: string }) {
  return (
    <div className="assistant-markdown">
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        skipHtml
        allowedElements={[
          'a',
          'blockquote',
          'br',
          'code',
          'del',
          'em',
          'h1',
          'h2',
          'h3',
          'h4',
          'h5',
          'h6',
          'hr',
          'input',
          'li',
          'ol',
          'p',
          'pre',
          'strong',
          'table',
          'tbody',
          'td',
          'th',
          'thead',
          'tr',
          'ul',
        ]}
        components={{
          a: ({ href, children }) => {
            const safe = safeHTTPURL(href);
            return safe ? (
              <a href={safe} target="_blank" rel="noopener noreferrer">
                {children}
              </a>
            ) : (
              <span>{children}</span>
            );
          },
          table: ({ children }) => <MarkdownTable>{children}</MarkdownTable>,
          pre: ({ children }) => {
            const child = Children.toArray(children)[0];
            return isValidElement<ComponentPropsWithoutRef<'code'>>(child) ? (
              <CodeBlock
                code={textFromNode(child.props.children).replace(/\n$/u, '')}
                language={
                  /language-([^ ]+)/u.exec(child.props.className || '')?.[1] ||
                  'text'
                }
              />
            ) : (
              <pre>{children}</pre>
            );
          },
          code: (props) => <code {...props} />,
        }}
      >
        {text}
      </ReactMarkdown>
    </div>
  );
}
