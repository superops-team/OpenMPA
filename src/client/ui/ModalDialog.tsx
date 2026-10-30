import {
  useLayoutEffect,
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
  className = '',
  closeLabel,
}: {
  title: string;
  description?: string;
  onClose: () => void;
  children: ReactNode;
  footer?: ReactNode;
  size?: 'default' | 'wide';
  className?: string;
  closeLabel?: string;
}) {
  const card = useRef<HTMLElement>(null);
  const close = useRef(onClose);
  close.current = onClose;
  const titleId = useId();
  const descriptionId = useId();
  useLayoutEffect(() => {
    const node = card.current;
    if (!node) return;
    const previous = document.activeElement;
    const preferred = [
      ...node.querySelectorAll<HTMLElement>(
        '[data-autofocus="true"], [autofocus]',
      ),
    ].find((item) => !item.matches(':disabled'));
    const background = [...document.body.children]
      .filter(
        (item): item is HTMLElement =>
          item instanceof HTMLElement && !item.contains(node),
      )
      .map((item) => ({ item, inert: item.inert }));
    for (const { item } of background) item.inert = true;
    const overflow = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    // Disabling the focused submit button during an async operation can move
    // focus to body. Escape must still dismiss the topmost dialog in that case.
    const escape = (event: globalThis.KeyboardEvent) => {
      if (
        event.key === 'Escape' &&
        !event.defaultPrevented &&
        !event.isComposing &&
        [...document.querySelectorAll('.ui-modal-dialog')].at(-1) === node
      ) {
        event.preventDefault();
        close.current();
      }
    };
    document.addEventListener('keydown', escape);
    // Resolve focus before paint. Async content must not move focus again or
    // temporarily focus the close button while the form is loading.
    (preferred ?? node).focus({ preventScroll: true });
    return () => {
      document.removeEventListener('keydown', escape);
      for (const { item, inert } of background) item.inert = inert;
      document.body.style.overflow = overflow;
      if (previous instanceof HTMLElement && previous.isConnected)
        previous.focus({ preventScroll: true });
    };
  }, []);
  const handleKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key === 'Escape' && !event.nativeEvent?.isComposing) {
      event.preventDefault();
      event.stopPropagation?.();
      onClose();
      return;
    }
    if (event.key !== 'Tab' || !card.current) return;
    const items = [
      ...card.current.querySelectorAll<HTMLElement>(focusable),
    ].filter(
      (item) => item.offsetParent !== null && !item.matches(':disabled'),
    );
    if (!items.length) {
      event.preventDefault();
      card.current.focus();
      return;
    }
    const first = items[0];
    const last = items.at(-1)!;
    if (
      event.shiftKey &&
      (document.activeElement === first ||
        document.activeElement === card.current)
    ) {
      event.preventDefault();
      last.focus();
    } else if (
      !event.shiftKey &&
      (document.activeElement === last ||
        document.activeElement === card.current)
    ) {
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
        className={`ui-modal-dialog desktop-app template-app ${className}`.trim()}
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
