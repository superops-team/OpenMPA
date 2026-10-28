import { useMessages } from './locale';
import { Check, ChevronDown, Cpu, Search, X } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
import type { ExecutionConfigVM } from './models';
import type { RuntimeModelCatalog } from './model-catalog';
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
  catalog = { models: [], defaultModel: '', supported: false },
  catalogLoading = false,
  catalogError = '',
  onRefreshCatalog,
}: {
  catalog?: RuntimeModelCatalog;
  catalogLoading?: boolean;
  catalogError?: string;
  onRefreshCatalog?: () => void;
  config?: ExecutionConfigVM;
  busy: boolean;
  needsInitialization?: boolean;
  locked?: boolean;
  onApply: (mode: ModelSelectionMode, model?: string) => Promise<boolean>;
}) {
  const m = useMessages();
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState('');
  const root = useRef<HTMLDivElement>(null);
  const canApply =
    !busy && !locked && !catalogLoading && !catalogError && catalog.supported;
  const selectionDisabled = !canApply;
  const defaultModel = catalog.defaultModel || catalog.models[0]?.id || '';
  const label = modelSelectionLabel(config, m, defaultModel);
  const invalidCurrent =
    catalog.supported &&
    !!config?.model &&
    !isKnownModel(config.model, catalog.models);
  const models = filterModels(query, catalog.models);
  const currentModelLabel =
    config?.model || defaultModel || m.desktopRuntimeDefault();
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
        aria-label={m.chatModelLabel(label)}
        title={
          locked
            ? m.modelWaitForTurn()
            : canApply
              ? m.modelChooseCatalog()
              : m.modelLoadingConfiguration()
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
          aria-label={m.desktopModelSelection()}
        >
          <div className="composer-model-head">
            <div>
              <p className="composer-model-heading">
                {m.runtimeModelSelectionTitle()}
              </p>
              <p className="composer-model-note">
                {m.desktopAppliesOnlyToNewMessagesEarlierTurnsKeepThe()}
              </p>
            </div>
            <button
              type="button"
              className="composer-model-close"
              aria-label={m.desktopCloseModelSelector()}
              onClick={() => setOpen(false)}
            >
              <X size={13} />
            </button>
          </div>
          {catalogLoading && (
            <p role="status" className="composer-model-note">
              {m.modelCatalogLoading()}
            </p>
          )}
          {!catalogLoading && catalogError && (
            <div className="composer-model-warning" role="alert">
              <p>{catalogError}</p>
              {onRefreshCatalog && (
                <button
                  type="button"
                  className="text-button"
                  onClick={onRefreshCatalog}
                >
                  {m.modelCatalogRetry()}
                </button>
              )}
            </div>
          )}
          {!catalogLoading && !catalogError && !catalog.supported && (
            <p role="status" className="composer-model-note">
              {m.modelCatalogUnavailable()}
            </p>
          )}
          {invalidCurrent && (
            <p className="composer-model-warning" role="status">
              {m.chatCurrentModelInvalid(config?.model || '')}
            </p>
          )}
          {locked && (
            <div className="composer-model-warning" role="status">
              <span>
                {m.desktopTheCurrentTurnIsStillRunningOrAwaitingOutcome()}
              </span>
            </div>
          )}
          {!locked && needsInitialization && (
            <p className="composer-model-note" role="status">
              {m.desktopThisLegacyConversationHasNotInitializedItsNextTurn()}
            </p>
          )}
          <label className="composer-model-search">
            <Search size={13} />
            <input
              autoFocus={!selectionDisabled}
              value={query}
              disabled={selectionDisabled}
              placeholder={m.modelSearchRuntime()}
              aria-label={m.desktopFilterModels()}
              onChange={(event) => setQuery(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === 'Enter') {
                  event.preventDefault();
                  event.stopPropagation();
                }
              }}
            />
          </label>
          <div
            className={`composer-model-list ${
              selectionDisabled ? 'is-disabled' : ''
            }`}
            role="radiogroup"
            aria-disabled={selectionDisabled || undefined}
          >
            {models.map((model) => (
              <button
                key={model.id}
                type="button"
                role="radio"
                aria-pressed={(config?.model || defaultModel) === model.id}
                aria-checked={(config?.model || defaultModel) === model.id}
                className="composer-model-option"
                disabled={selectionDisabled}
                onClick={() => choose('replace', model.id)}
              >
                <span className="composer-model-radio" aria-hidden="true" />
                <span>
                  <strong>{model.id}</strong>
                  {model.id === defaultModel && (
                    <small>{m.runtimeDefaultModelHint()}</small>
                  )}
                </span>
                {config?.modelMode === 'replace' &&
                  config.model === model.id && <Check size={13} />}
              </button>
            ))}
            {!models.length && catalog.supported && !catalogLoading && (
              <p className="composer-model-empty">
                {m.desktopNoModelsMatchTheFilter()}
              </p>
            )}
          </div>
          <p className="composer-model-footnote">
            {m.runtimeModelChoice(currentModelLabel)}
          </p>
        </div>
      )}
    </div>
  );
}
