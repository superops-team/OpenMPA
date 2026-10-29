import type { Messages } from './generated/locales';
import type { TraceStepVM, TraceVM } from './models';

export interface TraceStep extends TraceStepVM {
  children: TraceStep[];
  depth: number;
  duration?: number;
  offsetMs?: number;
  widthPercent?: number;
  filterMatch?: boolean;
}

export interface TraceSummary {
  status: string;
  wallDurationMs?: number;
  queueDurationMs?: number;
  firstModelContentMs?: number;
  firstVisibleAnswerMs?: number;
  modelRequestCount?: number;
  toolCount?: number;
  failedToolCount?: number;
  retryCount?: number;
  totalTokens?: number;
  slowestStep?: { stepId: string; title: string; durationMs: number };
}

export interface TraceCoverage {
  complete: boolean;
  active: boolean;
  truncated: boolean;
  recordCount: number;
  returnedCount: number;
  missingKinds: string[];
  metrics: Record<string, string>;
}

export interface TraceModel {
  trace: TraceVM;
  run: Record<string, unknown>;
  coverage: TraceCoverage;
  summary: TraceSummary;
  roots: TraceStep[];
  steps: TraceStep[];
  workerSteps: TraceStep[];
}

export interface TraceFilters {
  query?: string;
  status?: string;
  category?: string;
}

function numberValue(value: unknown): number | undefined {
  return typeof value === 'number' && Number.isFinite(value) && value >= 0
    ? value
    : undefined;
}
function stringValue(value: unknown): string | undefined {
  return typeof value === 'string' && value ? value : undefined;
}
function booleanValue(value: unknown): boolean | undefined {
  return typeof value === 'boolean' ? value : undefined;
}
function objectValue(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : {};
}

export function formatTraceDuration(value?: number | null, m?: Messages) {
  if (value === undefined || value === null || !Number.isFinite(value))
    return m?.valueUnavailable() ?? 'Unavailable';
  if (value < 1000) {
    const milliseconds = String(Math.max(0, Math.round(value)));
    return m?.durationMilliseconds(milliseconds) ?? `${milliseconds}ms`;
  }
  if (value < 60_000) {
    const seconds = (value / 1000).toFixed(1);
    return m?.durationSeconds(seconds) ?? `${seconds}s`;
  }
  const roundedSeconds = Math.round(value / 1000);
  const minutes = Math.floor(roundedSeconds / 60);
  const seconds = roundedSeconds % 60;
  return (
    m?.durationMinutesSeconds(String(minutes), String(seconds)) ??
    `${minutes}m ${seconds}s`
  );
}

export function formatTraceCount(value?: number | null, m?: Messages) {
  return value === undefined || value === null || !Number.isFinite(value)
    ? (m?.valueUnavailable() ?? 'Unavailable')
    : String(value);
}

export function traceStatusLabel(status: string, m?: Messages) {
  const labels: Record<string, string> = {
    running: m?.desktopRunning() ?? 'Running',
    success: m?.desktopSuccess() ?? 'Success',
    failed: m?.desktopFailed() ?? 'Failed',
    cancelled: m?.desktopCancelled() ?? 'Cancelled',
    timeout: m?.desktopTimeout() ?? 'Timeout',
    skipped: m?.statusSkipped() ?? 'Skipped',
    not_observed: m?.desktopNotObserved() ?? 'Not observed',
    login_required: m?.desktopLoginRequired() ?? 'Login required',
    denied: m?.statusDenied() ?? 'Denied',
  };
  return (
    (Object.hasOwn(labels, status) ? labels[status] : status) ||
    (m?.desktopUnknown() ?? 'Unknown')
  );
}

export function traceCategoryLabel(category: string, m?: Messages) {
  if (!m) return category;
  const labels: Record<string, string> = {
    request: m.traceCategoryRequest(),
    preparation: m.traceCategoryPreparation(),
    model: m.desktopModel(),
    tool: m.desktopTool(),
    sandbox: m.traceCategorySandbox(),
    worker: m.desktopWorker(),
    file: m.traceCategoryFile(),
    plan: m.traceCategoryPlan(),
    terminal: m.traceCategoryTerminal(),
    error: m.traceCategoryError(),
    runtime: m.runtime(),
  };
  return Object.hasOwn(labels, category) ? labels[category] : category;
}

export function traceStepCategory(step: TraceStepVM) {
  if (step.category) return step.category;
  const kind = step.kind.toLowerCase();
  if (kind.includes('usage') || kind.includes('model')) return 'model';
  if (
    kind.includes('worker') ||
    kind.includes('sandbox') ||
    kind.startsWith('codex.')
  )
    return 'worker';
  if (/^tool[._]/.test(kind) || kind.includes('command')) return 'tool';
  if (kind.includes('file')) return 'file';
  if (kind.includes('queue') || kind.includes('request')) return 'request';
  if (kind.includes('terminal') || kind.includes('run.total'))
    return 'terminal';
  if (kind.includes('error') || step.status === 'failed') return 'error';
  return 'runtime';
}

export function traceStepTitle(step: TraceStepVM, m?: Messages) {
  const workerTitles: Record<string, { title: string; defaults: string[] }> = {
    'codex.turn.reasoning_delta': {
      title: m?.traceWorkerReasoning() ?? 'Worker reasoning',
      defaults: ['Worker reasoning', 'Worker turn'],
    },
    'codex.turn.message_delta': {
      title: m?.traceWorkerResponse() ?? 'Worker response',
      defaults: ['Worker response', 'Worker response output'],
    },
    'codex.usage.updated': {
      title: m?.traceWorkerUsage() ?? 'Worker usage',
      defaults: ['Worker usage', 'Worker model usage'],
    },
    'codex.turn.completed': {
      title: m?.traceWorkerCompleted() ?? 'Worker completed',
      defaults: ['Worker completed', 'Worker turn completed'],
    },
  };
  if (Object.hasOwn(workerTitles, step.kind)) {
    const worker = workerTitles[step.kind];
    if (
      !step.title ||
      step.title === step.kind ||
      worker.defaults.includes(step.title)
    )
      return worker.title;
  }
  if (
    /^(commandExecution|exec_command|run_command|bash|shell)$/i.test(step.title)
  ) {
    const details = objectValue(step.details);
    const command =
      stringValue(details.toolSubject) ||
      stringValue(objectValue(details.safeCommandSummary).commandPreview);
    return command
      ? (m?.traceRunCommand(command) ?? `Run ${command}`)
      : (m?.desktopRunCommand() ?? 'Run command');
  }
  if (step.title) return step.title;
  return step.kind || step.stepId;
}

function stepDuration(step: TraceStepVM, now: number) {
  if (step.timing?.kind === 'milestone') return undefined;
  if (step.durationMs !== undefined && Number.isFinite(step.durationMs))
    return Math.max(0, step.durationMs);
  const start = Date.parse(step.startedAt);
  if (!Number.isFinite(start)) return undefined;
  if (!step.endedAt && step.status !== 'running') return undefined;
  const end = step.endedAt ? Date.parse(step.endedAt) : now;
  if (!Number.isFinite(end)) return undefined;
  return Math.max(0, end - start);
}

function buildStep(step: TraceStepVM, now: number, depth: number): TraceStep {
  return {
    ...step,
    children: [],
    depth,
    duration: stepDuration(step, now),
  };
}

function numberFromObject(value: unknown, keys: string[]): number | undefined {
  const object = objectValue(value);
  for (const key of keys) {
    const number = numberValue(object[key]);
    if (number !== undefined) return number;
  }
  return undefined;
}

function usageTotal(value: unknown): number | undefined {
  const usage = objectValue(value);
  const total = numberFromObject(usage, [
    'totalTokens',
    'total_tokens',
    'totalTokenCount',
    'total_token_count',
  ]);
  if (total !== undefined) return total;
  const input = numberFromObject(usage, [
    'inputTokens',
    'input_tokens',
    'promptTokens',
    'prompt_tokens',
    'inputTokenCount',
    'input_token_count',
  ]);
  const output = numberFromObject(usage, [
    'outputTokens',
    'output_tokens',
    'completionTokens',
    'completion_tokens',
    'outputTokenCount',
    'output_token_count',
  ]);
  return input !== undefined && output !== undefined
    ? input + output
    : undefined;
}

function detailObjects(step: TraceStepVM): Record<string, unknown>[] {
  const details = objectValue(step.details);
  return [
    details,
    objectValue(details.usage),
    objectValue(details.payload),
    objectValue(details.metrics),
    objectValue(step.timing),
  ].filter((item) => Object.keys(item).length > 0);
}

function stepMetric(step: TraceStepVM, keys: string[]): number | undefined {
  for (const object of detailObjects(step)) {
    const value = numberFromObject(object, keys);
    if (value !== undefined) return value;
  }
  return undefined;
}

function stepUsageTotal(step: TraceStepVM): number | undefined {
  for (const object of detailObjects(step)) {
    const total = usageTotal(object);
    if (total !== undefined) return total;
  }
  return undefined;
}

function isModelStep(step: TraceStepVM) {
  if (step.kind === 'run.total' || step.kind === 'trace.phase') return false;
  const kind = step.kind.toLowerCase();
  if (traceStepCategory(step) === 'model') return true;
  return (
    kind.includes('usage') ||
    kind.includes('model_proxy') ||
    stepUsageTotal(step) !== undefined
  );
}

function isToolStep(step: TraceStepVM) {
  if (step.kind === 'run.total' || step.kind === 'trace.phase') return false;
  const kind = step.kind.toLowerCase();
  const category = traceStepCategory(step);
  return (
    category === 'tool' ||
    /(?:^|\.)tool[._]/.test(kind) ||
    kind.includes('commandexecution') ||
    kind.includes('exec_command')
  );
}

function isWorkerStep(step: TraceStepVM) {
  const category = traceStepCategory(step);
  return category === 'worker' || category === 'sandbox';
}

function meaningfulCount(
  reported: number | undefined,
  observed: number,
  complete: boolean,
) {
  if (reported !== undefined) return reported;
  if (observed > 0) return observed;
  return complete ? observed : undefined;
}

function minDefined(values: Array<number | undefined>) {
  const numbers = values.filter(
    (value): value is number => value !== undefined,
  );
  return numbers.length ? Math.min(...numbers) : undefined;
}

function deriveSummary(
  steps: TraceStepVM[],
  run: Record<string, unknown>,
  complete: boolean,
  now: number,
) {
  const root = steps.find((step) => step.kind === 'run.total');
  const modelSteps = steps.filter(isModelStep);
  const toolSteps = steps.filter(isToolStep);
  const retrySteps = steps.filter((step) =>
    step.kind.toLowerCase().includes('retry'),
  );
  let totalTokens = numberValue(run.totalTokens) ?? usageTotal(run.usage);
  if (totalTokens === undefined && modelSteps.length > 0) {
    const totals = modelSteps
      .map(stepUsageTotal)
      .filter((count): count is number => count !== undefined);
    // A usage.updated event may be a cumulative snapshot. Multiple snapshots
    // cannot be added without a request-level accounting contract.
    if (totals.length === 1) totalTokens = totals[0];
  }
  const firstModelContent =
    numberValue(run.firstModelContentMs) ??
    numberValue(run.firstContentMs) ??
    minDefined(
      modelSteps.map((step) =>
        stepMetric(step, [
          'firstModelContentMs',
          'firstContentMs',
          'firstContentFrameMs',
          'first_content_frame_ms',
        ]),
      ),
    );
  const traceStart = Date.parse(
    stringValue(run.startedAt) || root?.startedAt || steps[0]?.startedAt || '',
  );
  const firstAnswerOffset = minDefined(
    steps
      .filter((step) => {
        const kind = step.kind.toLowerCase();
        return (
          kind.includes('message_delta') ||
          kind.includes('answer') ||
          kind.includes('visible_answer') ||
          kind.includes('reply')
        );
      })
      .map((step) => {
        const started = Date.parse(step.startedAt);
        return Number.isFinite(traceStart) && Number.isFinite(started)
          ? Math.max(0, started - traceStart)
          : undefined;
      }),
  );
  const slowest = steps
    .filter(
      (step) =>
        step.kind !== 'run.total' &&
        step.kind !== 'trace.phase' &&
        step.status !== 'not_observed',
    )
    .reduce<{ stepId: string; title: string; durationMs: number } | undefined>(
      (best, step) => {
        const duration = stepDuration(step, now);
        if (duration === undefined) return best;
        return !best || duration > best.durationMs
          ? {
              stepId: step.stepId,
              title: traceStepTitle(step),
              durationMs: duration,
            }
          : best;
      },
      undefined,
    );
  return {
    status:
      stringValue(run.status) ||
      root?.status ||
      steps.at(-1)?.status ||
      'not_observed',
    wallDurationMs:
      numberValue(run.wallDurationMs) ??
      (root ? stepDuration(root, now) : undefined),
    queueDurationMs: numberValue(run.queueDurationMs),
    firstModelContentMs: firstModelContent,
    firstVisibleAnswerMs:
      numberValue(run.firstVisibleAnswerMs) ?? firstAnswerOffset,
    modelRequestCount: meaningfulCount(
      numberValue(run.modelRequestCount),
      modelSteps.filter((step) => !step.kind.toLowerCase().includes('usage'))
        .length,
      complete,
    ),
    toolCount: meaningfulCount(
      numberValue(run.toolCount),
      toolSteps.length,
      complete,
    ),
    failedToolCount: meaningfulCount(
      numberValue(run.failedToolCount),
      toolSteps.filter((step) => step.status === 'failed').length,
      complete,
    ),
    retryCount: meaningfulCount(
      numberValue(run.retryCount),
      retrySteps.length,
      complete,
    ),
    totalTokens,
    slowestStep:
      (objectValue(run.slowestStep).stepId &&
      numberValue(objectValue(run.slowestStep).durationMs) !== undefined
        ? {
            stepId: String(objectValue(run.slowestStep).stepId),
            title: String(objectValue(run.slowestStep).title || 'Step'),
            durationMs:
              numberValue(objectValue(run.slowestStep).durationMs) ?? 0,
          }
        : undefined) || slowest,
  };
}

function deriveCoverage(
  steps: TraceStepVM[],
  raw: Record<string, unknown>,
): TraceCoverage {
  const metrics = Object.fromEntries(
    Object.entries(objectValue(raw.metrics)).map(([key, value]) => [
      key,
      String(value),
    ]),
  );
  return {
    complete:
      raw.aggregateComplete !== undefined
        ? raw.aggregateComplete === true
        : (booleanValue(raw.complete) ?? false),
    active:
      booleanValue(raw.active) ??
      steps.some((step) => step.status === 'running'),
    truncated: booleanValue(raw.truncated) ?? false,
    recordCount: numberValue(raw.recordCount) ?? steps.length,
    returnedCount: numberValue(raw.returnedCount) ?? steps.length,
    missingKinds: Array.isArray(raw.missingKinds)
      ? raw.missingKinds.map(String)
      : [],
    metrics,
  };
}

export function buildTraceModel(trace: TraceVM, now = Date.now()): TraceModel {
  const run = objectValue(trace.run);
  const rawCoverage = objectValue(trace.coverage);
  const sorted = [...trace.steps].sort(
    (left, right) => left.sequence - right.sequence,
  );
  const nodes = new Map<string, TraceStep>();
  for (const step of sorted) nodes.set(step.stepId, buildStep(step, now, 0));
  const roots: TraceStep[] = [];
  for (const node of nodes.values()) {
    const parent = node.parentStepId ? nodes.get(node.parentStepId) : undefined;
    if (parent && parent !== node) {
      parent.children.push(node);
    } else {
      roots.push(node);
    }
  }
  const assignDepth = (items: TraceStep[], depth: number) => {
    for (const item of items) {
      item.depth = depth;
      assignDepth(item.children, depth + 1);
    }
  };
  assignDepth(roots, 0);
  const all = roots.flatMap(function flatten(item): TraceStep[] {
    return [item, ...item.children.flatMap(flatten)];
  });
  const coverage = deriveCoverage(sorted, rawCoverage);
  const summary = deriveSummary(
    sorted,
    run,
    coverage.complete && !coverage.truncated,
    now,
  );
  const wall = summary.wallDurationMs;
  const start = Date.parse(
    stringValue(run.startedAt) || sorted[0]?.startedAt || '',
  );
  if (Number.isFinite(start) && wall && wall > 0) {
    for (const step of all) {
      const stepStart = Date.parse(step.startedAt);
      if (!Number.isFinite(stepStart)) continue;
      step.offsetMs = Math.max(0, stepStart - start);
      if (step.duration !== undefined) {
        step.widthPercent = Math.max(
          0.8,
          Math.min(100, (step.duration / wall) * 100),
        );
      }
    }
  }
  return {
    trace,
    run,
    coverage,
    summary,
    roots,
    steps: all,
    workerSteps: all.filter(isWorkerStep),
  };
}

export function flattenTraceTree(roots: TraceStep[], expanded: Set<string>) {
  const out: TraceStep[] = [];
  const visit = (items: TraceStep[]) => {
    for (const item of items) {
      out.push(item);
      if (item.children.length && expanded.has(item.stepId))
        visit(item.children);
    }
  };
  visit(roots);
  return out;
}

export function traceStepForTool(
  model: TraceModel,
  toolId: string,
): TraceStep | undefined {
  return model.steps.find(
    (step) => objectValue(step.correlation).toolId === toolId,
  );
}

export function filterTraceTree(
  roots: TraceStep[],
  filters: TraceFilters,
): TraceStep[] {
  const query = (filters.query || '').trim().toLowerCase();
  const matches = (step: TraceStep) => {
    const haystack =
      `${step.stepId} ${step.title} ${step.kind} ${step.status}`.toLowerCase();
    return (
      (!query || haystack.includes(query)) &&
      (!filters.status || step.status === filters.status) &&
      (!filters.category || traceStepCategory(step) === filters.category)
    );
  };
  const visit = (items: TraceStep[]): TraceStep[] => {
    const out: TraceStep[] = [];
    for (const item of items) {
      const children = visit(item.children);
      const match = matches(item);
      if (match || children.length) {
        out.push({ ...item, children, filterMatch: match });
      }
    }
    return out;
  };
  return visit(roots);
}

const sensitiveKey = /authorization|token|secret|password|api[-_]?key|cookie/i;
export function safeTraceDetails(value: unknown, depth = 0): unknown {
  if (depth > 4) return '[depth limited]';
  if (typeof value === 'string')
    return value.length > 2000 ? `${value.slice(0, 2000)}…` : value;
  if (Array.isArray(value))
    return value.slice(0, 50).map((item) => safeTraceDetails(item, depth + 1));
  if (value && typeof value === 'object') {
    const out: Record<string, unknown> = {};
    for (const [key, item] of Object.entries(
      value as Record<string, unknown>,
    )) {
      if (sensitiveKey.test(key)) continue;
      out[key] = safeTraceDetails(item, depth + 1);
    }
    return out;
  }
  return value;
}
