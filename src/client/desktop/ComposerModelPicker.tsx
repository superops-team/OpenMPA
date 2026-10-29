import { Check, ChevronDown, Cpu, Search } from 'lucide-react';
import { useEffect, useLayoutEffect, useId, useRef, useState } from 'react';
import type { ExecutionConfigVM } from './models';
import {
  filterModels,
  type RuntimeModelCatalog,
  type ModelOption,
} from './model-catalog';
import { type ModelSelectionMode } from './use-composer-model';
import { useMessages } from './locale';

export function ComposerModelPicker({
  config,
  busy,
  needsInitialization,
  locked,
  onApply,
  modelCatalog = [],
  catalog,
  catalogError = '',
  onRefreshCatalog,
  placement = 'above',
  catalogLoading = false,
  catalogFailed = false,
  onRetryCatalog,
  unavailable = false,
}: {
  config?: ExecutionConfigVM;
  busy: boolean;
  needsInitialization?: boolean;
  locked?: boolean;
  modelCatalog?: ModelOption[];
  catalog?: RuntimeModelCatalog;
  catalogError?: string;
  onRefreshCatalog?: () => void;
  onApply: (mode: ModelSelectionMode, model?: string) => Promise<boolean>;
  placement?: 'above' | 'below';
  catalogLoading?: boolean;
  catalogFailed?: boolean;
  onRetryCatalog?: () => void;
  unavailable?: boolean;
}) {
  const m = useMessages();
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState('');
  const root = useRef<HTMLDivElement>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  const search = useRef<HTMLInputElement>(null);
  const menuId = useId();
  modelCatalog = catalog?.models ?? modelCatalog;
  const disabled =
    busy ||
    !!locked ||
    unavailable ||
    catalogLoading ||
    catalogFailed ||
    !!catalogError ||
    catalog?.supported === false;
  onRetryCatalog = onRetryCatalog ?? onRefreshCatalog;
  catalogFailed = catalogFailed || !!catalogError;
  const label =
    config?.modelMode === 'clear'
      ? m.noModelOverride()
      : config?.model || m.runtimeDefaultModel();
  const invalidCurrent =
    !catalogLoading &&
    !catalogFailed &&
    config?.modelMode !== 'clear' &&
    !!config?.model &&
    !modelCatalog.some(({ id }) => id === config?.model);
  const models = filterModels(query, modelCatalog);
  const inheritedTarget =
    config?.modelMode === 'inherit' ? config.model : undefined;
  const defaultBlocked =
    !!inheritedTarget && !modelCatalog.some(({ id }) => id === inheritedTarget);
  useLayoutEffect(() => {
    if (open && !disabled) search.current?.focus({ preventScroll: true });
  }, [open, disabled]);
  useEffect(() => {
    if (!open || typeof document === 'undefined') return;
    const close = (event: MouseEvent) => {
      if (!root.current?.contains(event.target as Node)) setOpen(false);
    };
    document.addEventListener('mousedown', close);
    return () => document.removeEventListener('mousedown', close);
  }, [open]);
  const choose = async (mode: ModelSelectionMode, id?: string) => {
    if (mode === 'inherit' && defaultBlocked) return;
    if (await onApply(mode, id)) {
      setOpen(false);
      trigger.current?.focus({ preventScroll: true });
    }
  };
  const options: { mode: ModelSelectionMode; id?: string; label: string }[] = [
    { mode: 'inherit', label: m.runtimeDefaultModel() },
    ...models.map(({ id }) => ({ mode: 'replace' as const, id, label: id })),
    { mode: 'clear', label: m.noModelOverride() },
  ];
  return (
    <div
      className="composer-model"
      data-placement={placement}
      ref={root}
      onBlur={(event) => {
        if (
          event.relatedTarget &&
          !event.currentTarget.contains(event.relatedTarget)
        )
          setOpen(false);
      }}
      onKeyDown={(event) => {
        if (event.key === 'Escape' && open) {
          event.preventDefault();
          event.stopPropagation();
          setOpen(false);
          trigger.current?.focus({ preventScroll: true });
        }
      }}
    >
      <button
        ref={trigger}
        type="button"
        className="composer-model-trigger"
        aria-haspopup="dialog"
        aria-expanded={open}
        aria-controls={open ? menuId : undefined}
        aria-label={m.chatModelLabel(label)}
        title={locked ? m.modelLocked() : label}
        disabled={busy || unavailable}
        onClick={() => {
          setQuery('');
          setOpen((value) => !value);
        }}
      >
        <Cpu size={14} />
        <span>{label}</span>
        <ChevronDown size={12} />
      </button>
      {open && (
        <div
          id={menuId}
          className="composer-model-menu"
          role="dialog"
          aria-label={m.modelSelection()}
        >
          <label className="composer-model-search">
            <Search size={14} />
            <input
              ref={search}
              value={query}
              disabled={disabled}
              placeholder={m.searchModelPlaceholder()}
              aria-label={m.filterModels()}
              onChange={(event) => setQuery(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === 'Enter' && !event.nativeEvent?.isComposing) {
                  event.preventDefault();
                  event.stopPropagation();
                }
                if (event.key === 'ArrowDown') {
                  event.preventDefault();
                  root.current
                    ?.querySelector<HTMLButtonElement>(
                      '[role="radio"]:not(:disabled)',
                    )
                    ?.focus();
                }
              }}
            />
          </label>
          {locked && (
            <p className="composer-model-warning" role="status">
              {m.modelLocked()}
            </p>
          )}
          {invalidCurrent && (
            <p className="composer-model-warning" role="status">
              {catalog
                ? m.chatCurrentModelInvalid(config?.model || '')
                : m.modelNotListed(config?.model || '')}
            </p>
          )}
          {defaultBlocked && (
            <p className="composer-model-warning" role="status">
              {m.chatModelInvalid(inheritedTarget || '')}
            </p>
          )}
          {needsInitialization && (
            <span className="sr-only">{m.modelInitialization()}</span>
          )}
          {catalogLoading && (
            <p className="composer-model-empty" role="status">
              {m.loadingModels()}
            </p>
          )}
          {catalogFailed && (
            <div className="composer-model-warning" role="status">
              <span>{catalogError || m.modelCatalogReadFailed()}</span>
              {onRetryCatalog && (
                <button
                  type="button"
                  className="text-button"
                  onClick={onRetryCatalog}
                >
                  {m.modelCatalogRetry()}
                </button>
              )}
            </div>
          )}
          <div
            className="composer-model-list"
            role="radiogroup"
            aria-label={m.model()}
            aria-disabled={disabled || undefined}
            onKeyDown={(event) => {
              if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key))
                return;
              const buttons = [
                ...event.currentTarget.querySelectorAll<HTMLButtonElement>(
                  '[role="radio"]:not(:disabled)',
                ),
              ];
              if (!buttons.length) return;
              event.preventDefault();
              const index = buttons.indexOf(event.target as HTMLButtonElement);
              const next =
                event.key === 'Home'
                  ? 0
                  : event.key === 'End'
                    ? buttons.length - 1
                    : (index +
                        (event.key === 'ArrowUp' ? -1 : 1) +
                        buttons.length) %
                      buttons.length;
              buttons[next]?.focus({ preventScroll: true });
              buttons[next]?.scrollIntoView?.({ block: 'nearest' });
            }}
          >
            {options.map((option) => {
              const selected =
                config?.modelMode === option.mode &&
                (option.mode !== 'replace' || config?.model === option.id);
              return (
                <button
                  key={option.id || option.mode}
                  type="button"
                  role="radio"
                  aria-checked={selected}
                  aria-pressed={selected}
                  className="composer-model-option"
                  disabled={
                    disabled || (option.mode === 'inherit' && defaultBlocked)
                  }
                  onMouseDown={(event) => event.preventDefault()}
                  onClick={() => void choose(option.mode, option.id)}
                >
                  <span>
                    <strong>{option.label}</strong>
                  </span>
                  <Check
                    size={14}
                    aria-hidden="true"
                    className={selected ? '' : 'model-check-hidden'}
                  />
                </button>
              );
            })}
            {!models.length && !catalogLoading && !catalogFailed && (
              <p className="composer-model-empty">
                {query.trim() ? m.noModelsMatch() : m.noAccountModels()}
              </p>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
