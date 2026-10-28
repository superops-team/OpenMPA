import {
  AlertTriangle,
  BarChart3,
  CheckCircle2,
  ChevronDown,
  ChevronRight,
  Clock3,
  List,
  RefreshCw,
  Search,
  X,
} from 'lucide-react';
import { useEffect, useMemo, useRef, useState } from 'react';
import { IconButton } from '../ui';
import { useTraceResource } from './use-trace-resource';
import type { ConnectionVM, DesktopServices, SessionVM } from './models';
import {
  buildTraceModel,
  filterTraceTree,
  flattenTraceTree,
  formatTraceCount,
  formatTraceDuration,
  traceStepForTool,
  traceStatusLabel,
  traceStepCategory,
  traceStepTitle,
  type TraceStep,
} from './trace-model';
import { TraceStepDetail } from './TraceStepDetail';

const categories = [
  'request',
  'preparation',
  'model',
  'tool',
  'sandbox',
  'worker',
  'file',
  'plan',
  'terminal',
  'error',
  'runtime',
];

function shortId(value: string) {
  return value.length > 22 ? `${value.slice(0, 10)}…${value.slice(-8)}` : value;
}

export function TracePanel({
  services,
  connection,
  session,
  invocationIds,
  currentInvocationId,
  focus,
  onClose,
}: {
  services: DesktopServices;
  connection?: ConnectionVM;
  session?: SessionVM;
  invocationIds: string[];
  currentInvocationId?: string;
  focus?: { invocationId: string; toolId?: string };
  onClose: () => void;
}) {
  const [selected, setSelected] = useState(
    focus?.invocationId || currentInvocationId || invocationIds.at(-1) || '',
  );
  const { trace, loading, error, reload } = useTraceResource({
    read: services.trace.read,
    epoch: connection?.epoch,
    sessionId: session?.id,
    invocationId: selected,
  });
  const [query, setQuery] = useState('');
  const [status, setStatus] = useState('');
  const [category, setCategory] = useState('');
  const [mode, setMode] = useState<'timeline' | 'tree'>('timeline');
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const [selectedStep, setSelectedStep] = useState<string>();
  const lastTraceKey = useRef('');
  const previousCurrent = useRef(currentInvocationId);
  const previousFocus = useRef(focus);
  const dialog = useRef<HTMLDialogElement>(null);
  useEffect(() => {
    const node = dialog.current;
    if (!node) return;
    const previous = document.activeElement;
    node.showModal();
    return () => {
      node.close();
      if (previous instanceof HTMLElement && previous.isConnected)
        previous.focus();
    };
  }, []);
  const model = useMemo(
    () => (trace ? buildTraceModel(trace) : undefined),
    [trace],
  );
  const filteredRoots = useMemo(
    () =>
      model ? filterTraceTree(model.roots, { query, status, category }) : [],
    [model, query, status, category],
  );
  const visible = useMemo(
    () =>
      flattenTraceTree(
        filteredRoots,
        query || status || category
          ? new Set(model?.steps.map((step) => step.stepId))
          : expanded,
      ),
    [filteredRoots, expanded, query, status, category, model],
  );
  const selectedStepData = useMemo(
    () => model?.steps.find((step) => step.stepId === selectedStep),
    [model, selectedStep],
  );

  useEffect(() => {
    const next =
      focus?.invocationId || currentInvocationId || invocationIds.at(-1) || '';
    setSelected((previous) => {
      if (focus && focus !== previousFocus.current) return focus.invocationId;
      if (!next) return '';
      if (!previous || !invocationIds.includes(previous)) return next;
      if (
        currentInvocationId &&
        previousCurrent.current !== currentInvocationId &&
        previous === previousCurrent.current
      )
        return currentInvocationId;
      return previous;
    });
    previousCurrent.current = currentInvocationId;
    previousFocus.current = focus;
  }, [currentInvocationId, focus, invocationIds]);

  useEffect(() => {
    if (!model) return;
    const key = `${model.trace.sessionId}:${model.trace.invocationId}`;
    if (lastTraceKey.current === key) return;
    lastTraceKey.current = key;
    setExpanded(new Set(model.roots.map((step) => step.stepId)));
    setSelectedStep(
      model.steps.find((step) => step.status === 'failed')?.stepId ||
        model.roots[0]?.stepId,
    );
  }, [model]);

  useEffect(() => {
    if (!model || model.trace.invocationId !== focus?.invocationId) return;
    const step = focus.toolId
      ? traceStepForTool(model, focus.toolId)
      : undefined;
    setSelectedStep(step?.stepId ?? model.roots[0]?.stepId);
    if (step) {
      setExpanded((previous) => {
        const next = new Set(previous);
        let parent = step.parentStepId;
        while (parent && !next.has(parent)) {
          next.add(parent);
          parent = model.steps.find(
            (item) => item.stepId === parent,
          )?.parentStepId;
        }
        return next;
      });
    }
  }, [focus?.invocationId, focus?.toolId, model]);

  const toggle = (stepId: string) => {
    setExpanded((previous) => {
      const next = new Set(previous);
      if (next.has(stepId)) next.delete(stepId);
      else next.add(stepId);
      return next;
    });
  };

  const allExpanded = () =>
    setExpanded(new Set(model?.steps.map((step) => step.stepId) || []));
  const allCollapsed = () => setExpanded(new Set());

  const timelineLanes = useMemo(() => {
    const rows = filteredRoots.flatMap(function flatten(item): TraceStep[] {
      return [item, ...item.children.flatMap(flatten)];
    });
    const groups = new Map<string, TraceStep[]>();
    for (const row of rows) {
      const lane = traceStepCategory(row);
      const list = groups.get(lane) || [];
      list.push(row);
      groups.set(lane, list);
    }
    return [...groups.entries()];
  }, [filteredRoots]);

  return (
    <dialog
      ref={dialog}
      className="trace-panel"
      aria-label="Turn trace"
      onCancel={(event) => {
        event.preventDefault();
        onClose();
      }}
    >
      <header className="trace-panel__header">
        <div className="trace-panel__title">
          <BarChart3 size={18} />
          <div>
            <strong>Turn trace</strong>
            <span>
              {session?.title || 'Conversation'} ·{' '}
              {connection?.runtimeId || 'No Runtime'}
            </span>
          </div>
        </div>
        <div className="trace-panel__actions">
          <IconButton
            className="trace-icon-button"
            label="Refresh trace"
            onClick={reload}
            disabled={!selected || loading}
          >
            <RefreshCw size={15} className={loading ? 'trace-spin' : ''} />
          </IconButton>
          <IconButton
            className="trace-icon-button"
            label="Close trace"
            onClick={onClose}
          >
            <X size={16} />
          </IconButton>
        </div>
      </header>

      <div className="trace-panel__controls">
        <label className="trace-turn">
          <span>Turn</span>
          <select
            value={selected}
            onChange={(event) => setSelected(event.target.value)}
            disabled={!invocationIds.length}
          >
            {invocationIds.length ? (
              invocationIds.map((id, index) => (
                <option key={id} value={id}>
                  {id === currentInvocationId ? 'Current' : `Turn ${index + 1}`}{' '}
                  · {shortId(id)}
                </option>
              ))
            ) : (
              <option value="">Waiting for an accepted turn</option>
            )}
          </select>
        </label>
        <div
          className="trace-validation"
          role="status"
          data-state={
            error ? 'error' : loading ? 'loading' : model ? 'ready' : 'idle'
          }
        >
          {error ? (
            <AlertTriangle size={14} />
          ) : loading ? (
            <Clock3 size={14} />
          ) : (
            <CheckCircle2 size={14} />
          )}
          <span>
            {error
              ? 'Trace validation failed'
              : loading
                ? 'Validating trace'
                : model
                  ? `Trace validated · ${model.summary.status}`
                  : 'Trace validation idle'}
          </span>
        </div>
      </div>

      {error && (
        <div className="trace-banner trace-banner--error" role="alert">
          <AlertTriangle size={15} />
          <span>{error}</span>
        </div>
      )}
      {!error && model && (
        <div
          className={`trace-banner ${
            model.coverage.truncated || !model.coverage.complete
              ? 'trace-banner--warning'
              : 'trace-banner--ok'
          }`}
        >
          {model.coverage.truncated || !model.coverage.complete ? (
            <AlertTriangle size={15} />
          ) : (
            <CheckCircle2 size={15} />
          )}
          <span>
            {model.coverage.truncated
              ? `Trace truncated · returned ${model.coverage.returnedCount} of ${model.coverage.recordCount}`
              : model.coverage.active
                ? 'Trace collecting · the current turn is still running'
                : model.coverage.complete
                  ? `Trace complete · ${model.coverage.recordCount} steps`
                  : `Trace partial · ${model.coverage.recordCount} steps`}
            {model.coverage.missingKinds.length
              ? ` · missing ${model.coverage.missingKinds.join(', ')}`
              : ''}
            {` · ${model.trace.traceVersion}`}
          </span>
        </div>
      )}

      {!error && model && model.steps.length > 0 && (
        <>
          <div className="trace-summary" aria-label="Trace summary">
            {[
              ['Duration', formatTraceDuration(model.summary.wallDurationMs)],
              ['Queue', formatTraceDuration(model.summary.queueDurationMs)],
              [
                'First model content',
                formatTraceDuration(model.summary.firstModelContentMs),
              ],
              [
                'First visible answer',
                formatTraceDuration(model.summary.firstVisibleAnswerMs),
              ],
              [
                'Model calls',
                formatTraceCount(model.summary.modelRequestCount),
              ],
              ['Tools', formatTraceCount(model.summary.toolCount)],
              ['Tokens', formatTraceCount(model.summary.totalTokens)],
              ['Retries', formatTraceCount(model.summary.retryCount)],
            ].map(([label, value]) => (
              <div key={label} className="trace-summary__item">
                <span>{label}</span>
                <strong>{value}</strong>
              </div>
            ))}
          </div>
          {model.summary.slowestStep && (
            <div className="trace-slowest">
              <span>Slowest step</span>
              <strong>{model.summary.slowestStep.title}</strong>
              <em>
                {formatTraceDuration(model.summary.slowestStep.durationMs)}
              </em>
            </div>
          )}
          <div className="trace-toolbar">
            <label className="trace-search">
              <Search size={14} />
              <input
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                placeholder="Search steps, errors or IDs"
                aria-label="Search trace"
              />
            </label>
            <select
              value={status}
              onChange={(event) => setStatus(event.target.value)}
              aria-label="Filter trace status"
            >
              <option value="">All statuses</option>
              <option value="success">Success</option>
              <option value="failed">Failed</option>
              <option value="running">Running</option>
              <option value="timeout">Timeout</option>
              <option value="not_observed">Not observed</option>
              <option value="cancelled">Cancelled</option>
            </select>
            <select
              value={category}
              onChange={(event) => setCategory(event.target.value)}
              aria-label="Filter trace category"
            >
              <option value="">All categories</option>
              {categories.map((item) => (
                <option key={item} value={item}>
                  {item}
                </option>
              ))}
            </select>
            <div
              className="trace-view-toggle"
              role="group"
              aria-label="Trace view"
            >
              <button
                type="button"
                className={mode === 'timeline' ? 'active' : ''}
                onClick={() => setMode('timeline')}
              >
                Timeline
              </button>
              <button
                type="button"
                className={mode === 'tree' ? 'active' : ''}
                onClick={() => setMode('tree')}
              >
                Tree
              </button>
            </div>
            {mode === 'tree' && (
              <div className="trace-view-toggle">
                <button type="button" onClick={allExpanded}>
                  Expand
                </button>
                <button type="button" onClick={allCollapsed}>
                  Collapse
                </button>
              </div>
            )}
          </div>

          <div className="trace-body">
            <div className="trace-ledger">
              {mode === 'timeline' ? (
                timelineLanes.length ? (
                  timelineLanes.map(([lane, rows]) => (
                    <section key={lane} className="trace-lane">
                      <header>
                        <strong>{lane}</strong>
                        <span>{rows.length}</span>
                      </header>
                      {rows.map((step) => (
                        <button
                          key={step.stepId}
                          type="button"
                          className={`trace-row ${step.status} ${
                            selectedStep === step.stepId ? 'selected' : ''
                          }`}
                          onClick={() => setSelectedStep(step.stepId)}
                          aria-pressed={selectedStep === step.stepId}
                        >
                          <span
                            className="trace-row__name"
                            style={{ paddingLeft: step.depth * 10 }}
                          >
                            <strong title={traceStepTitle(step)}>
                              {traceStepTitle(step)}
                            </strong>
                            <small>{step.kind}</small>
                          </span>
                          <span className="trace-row__track">
                            {step.offsetMs !== undefined &&
                            step.widthPercent !== undefined &&
                            model.summary.wallDurationMs ? (
                              <i
                                style={{
                                  left: `${Math.min(100, (step.offsetMs / model.summary.wallDurationMs) * 100)}%`,
                                  width: `${Math.min(step.widthPercent, Math.max(0, 100 - (step.offsetMs / model.summary.wallDurationMs) * 100))}%`,
                                }}
                              />
                            ) : (
                              <em>Timing unavailable</em>
                            )}
                          </span>
                          <span className={`trace-status ${step.status}`}>
                            {traceStatusLabel(step.status)}
                          </span>
                          <span className="trace-row__duration">
                            {formatTraceDuration(step.duration)}
                          </span>
                        </button>
                      ))}
                    </section>
                  ))
                ) : (
                  <div className="trace-empty">
                    No steps match the current filters.
                  </div>
                )
              ) : visible.length ? (
                visible.map((step) => (
                  <div
                    key={step.stepId}
                    className={`trace-tree-row ${step.status} ${
                      selectedStep === step.stepId ? 'selected' : ''
                    }`}
                    style={{ paddingLeft: 8 + step.depth * 14 }}
                  >
                    {step.children.length > 0 ? (
                      <button
                        type="button"
                        className="trace-fold"
                        aria-label={
                          expanded.has(step.stepId)
                            ? 'Collapse step'
                            : 'Expand step'
                        }
                        aria-expanded={expanded.has(step.stepId)}
                        onClick={(event) => {
                          event.stopPropagation();
                          toggle(step.stepId);
                        }}
                      >
                        {step.children.length ? (
                          expanded.has(step.stepId) ? (
                            <ChevronDown size={13} />
                          ) : (
                            <ChevronRight size={13} />
                          )
                        ) : (
                          <span />
                        )}
                      </button>
                    ) : (
                      <span className="trace-fold" aria-hidden="true" />
                    )}
                    <button
                      className="trace-tree-select"
                      type="button"
                      onClick={() => setSelectedStep(step.stepId)}
                      aria-pressed={selectedStep === step.stepId}
                    >
                      <span className="trace-tree-row__name">
                        <strong>{traceStepTitle(step)}</strong>
                        <small>{step.kind}</small>
                      </span>
                      <span className={`trace-status ${step.status}`}>
                        {traceStatusLabel(step.status)}
                      </span>
                      <span className="trace-row__duration">
                        {formatTraceDuration(step.duration)}
                      </span>
                    </button>
                  </div>
                ))
              ) : (
                <div className="trace-empty">
                  No steps match the current filters.
                </div>
              )}
            </div>
            {selectedStepData && (
              <TraceStepDetail
                step={selectedStepData}
                onClose={() => setSelectedStep(undefined)}
              />
            )}
          </div>
        </>
      )}

      {!error && (!model || model.steps.length === 0) && (
        <div className="trace-empty trace-empty--large">
          <List size={22} />
          <strong>
            {loading
              ? 'Validating the current turn trace…'
              : selected
                ? model
                  ? 'Trace returned no steps for this turn.'
                  : 'Trace is not available for this turn.'
                : 'Waiting for an accepted turn.'}
          </strong>
          <span>
            {selected
              ? 'The Runtime returned an empty or unavailable trace for this invocation.'
              : 'Send a message first; trace validation starts automatically when an invocation exists.'}
          </span>
        </div>
      )}
    </dialog>
  );
}
