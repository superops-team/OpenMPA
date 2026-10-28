import { Check, Clipboard, WrapText } from 'lucide-react';
import { useState } from 'react';
import { IconButton } from './Button';

const languageNames: Record<string, string> = {
  js: 'JavaScript',
  javascript: 'JavaScript',
  jsx: 'JSX',
  ts: 'TypeScript',
  typescript: 'TypeScript',
  tsx: 'TSX',
  json: 'JSON',
  bash: 'Shell',
  sh: 'Shell',
  shell: 'Shell',
  go: 'Go',
  python: 'Python',
  py: 'Python',
  text: 'Plain text',
};

async function copyText(value: string) {
  if (navigator.clipboard?.writeText) {
    await navigator.clipboard.writeText(value);
    return;
  }
  const textarea = document.createElement('textarea');
  textarea.value = value;
  textarea.style.position = 'fixed';
  textarea.style.opacity = '0';
  document.body.append(textarea);
  textarea.select();
  const copied = document.execCommand('copy');
  textarea.remove();
  if (!copied) throw new Error('Clipboard unavailable');
}

export function CodeBlock({
  code,
  language = 'text',
}: {
  code: string;
  language?: string;
}) {
  const [wrapped, setWrapped] = useState(false);
  const [copyState, setCopyState] = useState<'idle' | 'copied' | 'failed'>(
    'idle',
  );
  const label = languageNames[language.toLowerCase()] || language.toUpperCase();
  return (
    <section className="ui-code-block">
      <header>
        <span>{label}</span>
        <div>
          <IconButton
            label={wrapped ? 'Do not wrap code lines' : 'Wrap code lines'}
            size="icon-sm"
            variant="ghost"
            aria-pressed={wrapped}
            onClick={() => setWrapped((current) => !current)}
          >
            <WrapText aria-hidden="true" />
          </IconButton>
          <IconButton
            label="Copy code"
            size="icon-sm"
            variant="ghost"
            onClick={() => {
              void copyText(code)
                .then(() => {
                  setCopyState('copied');
                  window.setTimeout(() => setCopyState('idle'), 1200);
                })
                .catch(() => setCopyState('failed'));
            }}
          >
            {copyState === 'copied' ? (
              <Check aria-hidden="true" />
            ) : (
              <Clipboard aria-hidden="true" />
            )}
          </IconButton>
        </div>
      </header>
      <pre data-wrap={wrapped || undefined}>
        <code className={`language-${language}`}>{code}</code>
      </pre>
      <span className="sr-only" role="status">
        {copyState === 'copied'
          ? 'Code copied'
          : copyState === 'failed'
            ? 'Unable to copy code'
            : ''}
      </span>
    </section>
  );
}
