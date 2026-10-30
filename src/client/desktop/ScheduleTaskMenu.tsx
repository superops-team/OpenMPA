import { useCallback, useEffect, useId, useRef, useState } from 'react';
import { Copy, History, MoreHorizontal, Pencil, Trash2 } from 'lucide-react';
import { Button } from '../ui';
import { useMessages } from './locale';
import type { ScheduleVM } from './models';

export function ScheduleTaskMenu({
  task,
  busy,
  onOpen,
  onEdit,
  onCopy,
  onDelete,
}: {
  task: ScheduleVM;
  busy: boolean;
  onOpen(): void;
  onEdit(): void;
  onCopy(): void;
  onDelete(): void;
}) {
  const m = useMessages();
  const id = useId();
  const trigger = useRef<HTMLButtonElement>(null);
  const menu = useRef<HTMLDivElement>(null);
  const [open, setOpen] = useState(false);
  const positionMenu = useCallback(() => {
    if (!menu.current || !trigger.current) return;
    const anchor = trigger.current.getBoundingClientRect();
    const { width, height } = menu.current.getBoundingClientRect();
    const inset = 8;
    const below = anchor.bottom + 4;
    const top =
      below + height <= window.innerHeight - inset
        ? below
        : anchor.top - height - 4;
    menu.current.style.top = `${Math.max(inset, Math.min(top, window.innerHeight - height - inset))}px`;
    menu.current.style.left = `${Math.max(inset, Math.min(anchor.right - width, window.innerWidth - width - inset))}px`;
  }, []);
  useEffect(() => {
    if (!open) return;
    window.addEventListener('resize', positionMenu);
    window.addEventListener('scroll', positionMenu, true);
    return () => {
      window.removeEventListener('resize', positionMenu);
      window.removeEventListener('scroll', positionMenu, true);
    };
  }, [open, positionMenu]);
  const choose = (action: () => void) => {
    menu.current?.hidePopover();
    action();
  };
  return (
    <>
      <Button
        ref={trigger}
        size="icon-md"
        aria-label={`${m.scheduleActions()} · ${task.name}`}
        title={`${m.scheduleActions()} · ${task.name}`}
        disabled={busy}
        popoverTarget={id}
        aria-haspopup="menu"
        aria-expanded={open}
        aria-controls={id}
      >
        <MoreHorizontal size={16} />
      </Button>
      <div
        id={id}
        ref={menu}
        popover="auto"
        className="schedule-menu"
        role="menu"
        aria-label={`${m.scheduleActions()} · ${task.name}`}
        onToggle={(event) => {
          const isOpen = event.newState === 'open';
          setOpen(isOpen);
          if (isOpen) {
            positionMenu();
            menu.current?.querySelector<HTMLButtonElement>('button')?.focus();
          }
        }}
        onKeyDown={(event) => {
          if (!['ArrowUp', 'ArrowDown', 'Home', 'End'].includes(event.key))
            return;
          event.preventDefault();
          const buttons = [...event.currentTarget.querySelectorAll('button')];
          const current = buttons.indexOf(
            document.activeElement as HTMLButtonElement,
          );
          const next =
            event.key === 'Home'
              ? 0
              : event.key === 'End'
                ? buttons.length - 1
                : (current +
                    (event.key === 'ArrowDown' ? 1 : -1) +
                    buttons.length) %
                  buttons.length;
          buttons[next]?.focus();
        }}
      >
        <Button
          variant="ghost"
          role="menuitem"
          disabled={busy}
          leading={<History size={14} aria-hidden="true" />}
          onClick={() => choose(onOpen)}
        >
          {m.scheduleHistory()}
        </Button>
        <Button
          variant="ghost"
          role="menuitem"
          disabled={busy}
          onClick={() => choose(onEdit)}
          leading={<Pencil size={14} aria-hidden="true" />}
        >
          {m.desktopEdit()}
        </Button>
        <Button
          variant="ghost"
          role="menuitem"
          disabled={busy}
          onClick={() => choose(onCopy)}
          leading={<Copy size={14} aria-hidden="true" />}
        >
          {m.scheduleCopy()}
        </Button>
        <Button
          variant="ghost"
          className="schedule-menu-delete"
          role="menuitem"
          disabled={busy}
          onClick={() => choose(onDelete)}
          leading={<Trash2 size={14} aria-hidden="true" />}
        >
          {m.delete()}
        </Button>
      </div>
    </>
  );
}
