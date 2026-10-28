import { Check, ChevronDown, Cpu, Search, X } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
import type { ExecutionConfigVM } from './models';
import { filterModels, isKnownModel } from './model-catalog';
import {
  modelSelectionLabel,
  type ModelSelectionMode,
} from './use-composer-model';

export function ComposerModelPicker({
  config,
  busy,
  needsInitialization,
  locked,
  onApply,
}: {
  config?: ExecutionConfigVM;
  busy: boolean;
  needsInitialization?: boolean;
  locked?: boolean;
  onApply: (mode: ModelSelectionMode, model?: string) => Promise<boolean>;
}) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState('');
  const root = useRef<HTMLDivElement>(null);
  const canApply = !busy && !locked;
  const selectionDisabled = !canApply;
  const label = modelSelectionLabel(config);
  const invalidCurrent = !!config?.model && !isKnownModel(config.model);
  const models = filterModels(query);
  const currentModelLabel =
    config?.modelMode === 'replace'
      ? config.model || 'Selected model'
      : config?.modelMode === 'clear'
        ? 'No model override'
        : 'Runtime default';
  useEffect(() => {
    if (!open || typeof document === 'undefined') return;
    const close = (event: MouseEvent) => {
      if (!root.current?.contains(event.target as Node)) setOpen(false);
    };
    document.addEventListener('mousedown', close);
    return () => document.removeEventListener('mousedown', close);
  }, [open]);
  const choose = (mode: ModelSelectionMode, model?: string) => {
    void onApply(mode, model).then((saved) => {
      if (saved) setOpen(false);
    });
  };
  return (
    <div
      className="composer-model"
      ref={root}
      onKeyDown={(event) => {
        if (event.key === 'Escape') setOpen(false);
      }}
    >
      <button
        type="button"
        className="composer-model-trigger"
        aria-haspopup="true"
        aria-expanded={open}
        aria-label={`Model: ${label}`}
        title={
          locked
            ? 'Wait for the current turn to finish before changing the next turn model.'
            : canApply
              ? 'Choose a Runtime catalog model for future turns'
              : 'Loading the conversation model configuration.'
        }
        disabled={busy}
        onClick={() => {
          setQuery('');
          setOpen((value) => !value);
        }}
      >
        <Cpu size={13} />
        <span>{label}</span>
        <ChevronDown size={12} />
      </button>
      {open && (
        <div
          className="composer-model-menu"
          role="group"
          aria-label="Model selection"
        >
          <div className="composer-model-head">
            <div>
              <p className="composer-model-heading">
                Model for upcoming messages
              </p>
              <p className="composer-model-note">
                Applies only to new messages. Earlier turns keep the model they
                already used.
              </p>
            </div>
            <button
              type="button"
              className="composer-model-close"
              aria-label="Close model selector"
              onClick={() => setOpen(false)}
            >
              <X size={13} />
            </button>
          </div>
          {invalidCurrent && (
            <p className="composer-model-warning" role="status">
              Current model {config?.model} is not in the Runtime catalog.
              Choose a valid model before the next turn.
            </p>
          )}
          {locked && (
            <div className="composer-model-warning" role="status">
              <span>
                The current turn is still running or awaiting outcome
                verification. Finish it before changing the model for the next
                turn.
              </span>
            </div>
          )}
          {!locked && needsInitialization && (
            <p className="composer-model-note" role="status">
              This legacy conversation has not initialized its next-turn model
              configuration yet. Your first selection will initialize it; no
              existing turn is changed or replayed.
            </p>
          )}
          <label className="composer-model-search">
            <Search size={13} />
            <input
              autoFocus={!selectionDisabled}
              value={query}
              disabled={selectionDisabled}
              placeholder="Search by name or family"
              aria-label="Filter models"
              onChange={(event) => setQuery(event.target.value)}
            />
          </label>
          <div
            className={`composer-model-list ${
              selectionDisabled ? 'is-disabled' : ''
            }`}
            role="radiogroup"
            aria-disabled={selectionDisabled || undefined}
          >
            <button
              type="button"
              role="radio"
              aria-pressed={config?.modelMode === 'inherit'}
              aria-checked={config?.modelMode === 'inherit'}
              className="composer-model-option"
              disabled={selectionDisabled}
              onClick={() => choose('inherit')}
            >
              <span className="composer-model-radio" aria-hidden="true" />
              <span>
                <strong>Runtime default</strong>
                <small>
                  {config?.modelMode === 'inherit' && config.model
                    ? config.model
                    : 'Use the assistant profile default'}
                </small>
              </span>
              {config?.modelMode === 'inherit' && <Check size={13} />}
            </button>
            {models.map((model) => (
              <button
                key={model.id}
                type="button"
                role="radio"
                aria-pressed={
                  config?.modelMode === 'replace' && config.model === model.id
                }
                aria-checked={
                  config?.modelMode === 'replace' && config.model === model.id
                }
                className="composer-model-option"
                disabled={selectionDisabled}
                onClick={() => choose('replace', model.id)}
              >
                <span className="composer-model-radio" aria-hidden="true" />
                <span>
                  <strong>{model.id}</strong>
                  <small>
                    {model.family} ·{' '}
                    {(model.contextWindow / 1_000_000).toLocaleString(
                      undefined,
                      { maximumFractionDigits: 1 },
                    )}
                    M context window
                  </small>
                </span>
                {config?.modelMode === 'replace' &&
                  config.model === model.id && <Check size={13} />}
              </button>
            ))}
            {!models.length && (
              <p className="composer-model-empty">
                No models match the filter.
              </p>
            )}
            <button
              type="button"
              role="radio"
              aria-pressed={config?.modelMode === 'clear'}
              aria-checked={config?.modelMode === 'clear'}
              className="composer-model-option"
              disabled={selectionDisabled}
              onClick={() => choose('clear')}
            >
              <span className="composer-model-radio" aria-hidden="true" />
              <span>
                <strong>No model override</strong>
                <small>Clear this conversation override</small>
              </span>
              {config?.modelMode === 'clear' && <Check size={13} />}
            </button>
          </div>
          <p className="composer-model-footnote">
            Current choice: {currentModelLabel}. The model list is a Runtime
            catalog snapshot.
          </p>
        </div>
      )}
    </div>
  );
}
