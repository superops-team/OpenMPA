import { useEffect, useId, useRef, useState } from 'react';
import { Check, ChevronDown, RefreshCw } from 'lucide-react';
import { Button } from '../ui';
import { renderMessage, useMessages, type MessageText } from './locale';
import { studioFailure } from './studio-services';
import type { MpaLifecycleServices } from './lifecycle-models';

type Bucket = { name: string; region: string };
export function MpaTosBucketPicker({
  services,
  epoch,
  region,
  value,
  disabled,
  onChange,
}: {
  services: Pick<MpaLifecycleServices, 'tosBuckets'>;
  epoch: number;
  region: string;
  value: string;
  disabled: boolean;
  onChange(value: string): void;
}) {
  const m = useMessages();
  const identity = `${epoch}:${region}`;
  const current = useRef(identity);
  current.current = identity;
  const generation = useRef(0);
  const root = useRef<HTMLFieldSetElement>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  const search = useRef<HTMLInputElement>(null);
  const labelId = useId();
  const listId = useId();
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState('');
  const [state, setState] = useState<{
    identity: string;
    rows: Bucket[];
    loading: boolean;
    error?: MessageText;
  }>({ identity, rows: [], loading: true });
  const load = async () => {
    const id = ++generation.current;
    setState((previous) => ({
      identity,
      rows: previous.identity === identity ? previous.rows : [],
      loading: true,
    }));
    try {
      const rows = await services.tosBuckets(epoch, region);
      if (id === generation.current && identity === current.current)
        setState({ identity, rows, loading: false });
    } catch (error) {
      if (id === generation.current && identity === current.current)
        setState({
          identity,
          rows: [],
          loading: false,
          error: studioFailure(error),
        });
    }
  };
  useEffect(() => {
    setQuery('');
    setOpen(false);
    void load();
    return () => {
      generation.current++;
    };
  }, [services, epoch, region]);
  useEffect(() => {
    if (disabled) setOpen(false);
  }, [disabled]);
  useEffect(() => {
    if (!open || typeof document === 'undefined') return;
    const outside = (event: MouseEvent) => {
      if (!root.current?.contains(event.target as Node)) setOpen(false);
    };
    const escape = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        event.preventDefault();
        event.stopPropagation();
        setOpen(false);
        trigger.current?.focus();
      }
    };
    document.addEventListener('mousedown', outside);
    document.addEventListener('keydown', escape, true);
    return () => {
      document.removeEventListener('mousedown', outside);
      document.removeEventListener('keydown', escape, true);
    };
  }, [open]);
  const close = () => {
    setOpen(false);
    trigger.current?.focus();
  };
  const rows = state.identity === identity ? state.rows : [];
  const loading = state.identity !== identity || state.loading;
  const error = state.identity === identity ? state.error : undefined;
  const visible = rows.filter((row) =>
    row.name.toLowerCase().includes(query.trim().toLowerCase()),
  );
  const savedMissing = value && !rows.some((row) => row.name === value);
  const blocked = disabled || loading || Boolean(error);
  useEffect(() => {
    if (open && !blocked) search.current?.focus();
  }, [open, blocked]);
  return (
    <fieldset
      className="mpa-tos-storage mpa-fields"
      ref={root}
      disabled={disabled}
      onBlur={(event) => {
        // Removing a focused clear/option button can report a null target.
        // Close on an actual focus move outside; pointer dismissal handles
        // outside clicks, including non-focusable elements.
        if (
          event.relatedTarget &&
          !event.currentTarget.contains(event.relatedTarget as Node)
        )
          setOpen(false);
      }}
      onKeyDown={(event) => {
        if (event.key === 'Escape' && open) {
          event.preventDefault();
          event.stopPropagation();
          close();
        }
        if (
          !open ||
          !['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)
        )
          return;
        const options = Array.from(
          root.current?.querySelectorAll<HTMLButtonElement>(
            '[role="option"]:not([disabled])',
          ) || [],
        );
        if (!options.length) return;
        if (
          event.target === search.current &&
          !['ArrowDown', 'ArrowUp'].includes(event.key)
        )
          return;
        const index = options.indexOf(event.target as HTMLButtonElement);
        const next =
          event.key === 'Home'
            ? 0
            : event.key === 'End'
              ? options.length - 1
              : event.key === 'ArrowDown'
                ? (index + 1) % options.length
                : (index <= 0 ? options.length : index) - 1;
        event.preventDefault();
        options[next]?.focus();
      }}
    >
      <legend>{m.mpaTosTitle()}</legend>
      <p>{m.mpaTosDescription()}</p>
      <div className="mpa-bucket-field">
        <span id={labelId}>{m.mpaTosBucket()}</span>
        <button
          type="button"
          ref={trigger}
          className="mpa-resource-trigger"
          role="combobox"
          aria-labelledby={labelId}
          aria-expanded={open}
          aria-controls={open ? listId : undefined}
          aria-haspopup="listbox"
          disabled={blocked}
          data-placeholder={!value || undefined}
          title={value || undefined}
          onClick={() => setOpen((previous) => !previous)}
          onKeyDown={(event) => {
            if (['ArrowDown', 'ArrowUp'].includes(event.key) && !open) {
              event.preventDefault();
              setOpen(true);
            }
          }}
        >
          <span>
            {value || (loading ? m.mpaTosLoading() : m.mpaTosSelectExisting())}
          </span>
          <ChevronDown size={16} aria-hidden="true" />
        </button>
        {open && (
          <div className="mpa-resource-panel">
            <div className="mpa-bucket-toolbar">
              <label>
                {m.mpaTosSearch()}
                <input
                  ref={search}
                  type="search"
                  autoComplete="off"
                  autoCapitalize="none"
                  autoCorrect="off"
                  spellCheck={false}
                  disabled={blocked}
                  value={query}
                  onChange={(event) => setQuery(event.target.value)}
                />
              </label>
              <Button
                variant="ghost"
                leading={<RefreshCw size={15} aria-hidden="true" />}
                disabled={blocked}
                aria-busy={loading}
                onClick={() => void load()}
              >
                {m.mpaTosRefresh()}
              </Button>
            </div>
            <div
              id={listId}
              role="listbox"
              aria-labelledby={labelId}
              className="mpa-resource-options"
            >
              {visible.map((row) => (
                <button
                  type="button"
                  role="option"
                  value={row.name}
                  key={row.name}
                  aria-selected={value === row.name}
                  disabled={blocked}
                  onClick={() => {
                    if (!blocked) {
                      onChange(row.name);
                      close();
                    }
                  }}
                >
                  <span>{row.name}</span>
                  {value === row.name && <Check size={15} aria-hidden="true" />}
                </button>
              ))}
            </div>
            {!loading && !error && visible.length === 0 && (
              <p className="mpa-resource-empty">
                {query.trim() ? m.mpaTosNoMatches() : m.mpaTosEmpty()}
              </p>
            )}
          </div>
        )}
      </div>
      {loading && (
        <p role="status" className="mpa-tos-meta">
          {m.mpaTosLoading()}
        </p>
      )}
      {error ? (
        <div role="alert" className="mpa-tos-error">
          <span>{renderMessage(error, m)}</span>
          <Button
            variant="ghost"
            disabled={disabled || loading}
            onClick={() => void load()}
          >
            {m.mpaTosRefresh()}
          </Button>
        </div>
      ) : (
        !loading && (
          <p role="status" className="mpa-tos-meta">
            {m.mpaTosLoaded(region, String(rows.length), String(rows.length))}
          </p>
        )
      )}
      {!loading && !error && rows.length === 0 && (
        <p className="mpa-tos-meta">{m.mpaTosEmpty()}</p>
      )}
      {!loading && !error && savedMissing && (
        <p className="mpa-tos-meta">{m.mpaTosSaved()}</p>
      )}
      {value && (
        <div>
          <Button
            variant="outline"
            disabled={disabled}
            onClick={() => {
              onChange('');
              close();
            }}
          >
            {m.mpaTosClear()}
          </Button>
        </div>
      )}
      <p>{m.mpaTosPermissions()}</p>
    </fieldset>
  );
}
