import {
  useEffect,
  useId,
  useRef,
  type KeyboardEvent,
  type ReactNode,
} from 'react';
import { createPortal } from 'react-dom';
import { X } from 'lucide-react';
import { IconButton } from './Button';

const focusable =
  'button:not([disabled]), input:not([disabled]), textarea:not([disabled]), select:not([disabled]), [href], [tabindex]:not([tabindex="-1"])';

export function ModalDialog({
  title,
  description,
  onClose,
  children,
  footer,
  size = 'wide',
  closeLabel,
}: {
  title: string;
  description?: string;
  onClose: () => void;
  children: ReactNode;
  footer?: ReactNode;
  size?: 'default' | 'wide';
  closeLabel?: string;
}) {
  const card = useRef<HTMLElement>(null);
  const titleId = useId();
  const descriptionId = useId();
  useEffect(() => {
    const node = card.current;
    if (!node) return;
    const previous = document.activeElement;
    const preferred = node.querySelector<HTMLElement>('[autofocus]');
    const first =
      preferred ?? node.querySelector<HTMLElement>(focusable) ?? node;
    requestAnimationFrame(() => first.focus());
    return () => {
      if (previous instanceof HTMLElement && previous.isConnected)
        previous.focus();
    };
  }, []);
  const handleKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key === 'Escape') {
      event.preventDefault();
      onClose();
      return;
    }
    if (event.key !== 'Tab' || !card.current) return;
    const items = [
      ...card.current.querySelectorAll<HTMLElement>(focusable),
    ].filter((item) => item.offsetParent !== null);
    if (!items.length) {
      event.preventDefault();
      card.current.focus();
      return;
    }
    const first = items[0];
    const last = items.at(-1)!;
    if (event.shiftKey && document.activeElement === first) {
      event.preventDefault();
      last.focus();
    } else if (!event.shiftKey && document.activeElement === last) {
      event.preventDefault();
      first.focus();
    }
  };
  const modal = (
    <div
      className="ui-modal-layer"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) onClose();
      }}
      onKeyDown={handleKeyDown}
    >
      <section
        ref={card}
        className="ui-modal-dialog desktop-app template-app"
        data-size={size}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        aria-describedby={description ? descriptionId : undefined}
        tabIndex={-1}
      >
        <div className="ui-modal-card">
          <header className="ui-modal-header">
            <div>
              <h2 id={titleId}>{title}</h2>
              {description && <p id={descriptionId}>{description}</p>}
            </div>
            <IconButton
              label={closeLabel ?? `Close ${title.toLowerCase()}`}
              variant="ghost"
              onClick={onClose}
            >
              <X aria-hidden="true" />
            </IconButton>
          </header>
          <div className="ui-modal-body">{children}</div>
          {footer && <footer className="ui-modal-footer">{footer}</footer>}
        </div>
      </section>
    </div>
  );
  return typeof document !== 'undefined' && document.body
    ? createPortal(modal, document.body)
    : modal;
}
