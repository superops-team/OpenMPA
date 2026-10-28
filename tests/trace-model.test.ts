import { expect, it } from 'vitest';
import {
  buildTraceModel,
  filterTraceTree,
  formatTraceDuration,
  safeTraceDetails,
  traceStepCategory,
  traceStepForTool,
} from '../src/client/desktop/trace-model';
import type { TraceVM } from '../src/client/desktop/models';

const trace: TraceVM = {
  traceVersion: '1.2',
  sessionId: 'session-1',
  invocationId: 'inv-1',
  run: {
    status: 'success',
    startedAt: '2026-10-27T00:00:00Z',
    wallDurationMs: 1000,
    queueDurationMs: 100,
    firstModelContentMs: 300,
    usage: { totalTokens: 42 },
  },
  coverage: {
    complete: true,
    active: false,
    truncated: false,
    recordCount: 3,
    returnedCount: 3,
    metrics: { wallTiming: 'measured' },
  },
  steps: [
    {
      stepId: 'root',
      sequence: 1,
      kind: 'run.total',
      title: 'Turn',
      status: 'success',
      startedAt: '2026-10-27T00:00:00Z',
      endedAt: '2026-10-27T00:00:01Z',
      durationMs: 1000,
    },
    {
      stepId: 'model',
      parentStepId: 'root',
      sequence: 2,
      kind: 'model.request',
      title: 'Model request',
      status: 'success',
      startedAt: '2026-10-27T00:00:00.100Z',
      endedAt: '2026-10-27T00:00:00.500Z',
      durationMs: 400,
      details: { usage: { inputTokens: 20, outputTokens: 22 } },
    },
    {
      stepId: 'tool',
      parentStepId: 'root',
      sequence: 3,
      kind: 'tool.call',
      title: 'Shell',
      status: 'failed',
      startedAt: '2026-10-27T00:00:00.500Z',
      endedAt: '2026-10-27T00:00:00.700Z',
      durationMs: 200,
    },
  ],
};

it('builds the summary, hierarchy and timeline from a trace payload', () => {
  const model = buildTraceModel(trace, Date.parse('2026-10-27T00:00:02Z'));
  expect(model.summary.status).toBe('success');
  expect(model.summary.wallDurationMs).toBe(1000);
  expect(model.summary.modelRequestCount).toBe(1);
  expect(model.summary.toolCount).toBe(1);
  expect(model.summary.failedToolCount).toBe(1);
  expect(model.summary.totalTokens).toBe(42);
  expect(model.roots).toHaveLength(1);
  expect(model.roots[0].children).toHaveLength(2);
  expect(model.steps.find((step) => step.stepId === 'model')?.depth).toBe(1);
  expect(model.steps.find((step) => step.stepId === 'model')?.offsetMs).toBe(
    100,
  );
  expect(
    model.steps.find((step) => step.stepId === 'model')?.widthPercent,
  ).toBe(40);
});

it('filters a tree while keeping the parent of a matching child', () => {
  const model = buildTraceModel(trace);
  const filtered = filterTraceTree(model.roots, { status: 'failed' });
  expect(filtered).toHaveLength(1);
  expect(filtered[0].stepId).toBe('root');
  expect(filtered[0].children).toHaveLength(1);
  expect(filtered[0].children[0].stepId).toBe('tool');
});

it('projects worker lifecycle and binding steps into the worker lane', () => {
  const model = buildTraceModel({
    ...trace,
    steps: [
      ...trace.steps,
      {
        stepId: 'worker-start',
        parentStepId: 'root',
        sequence: 4,
        kind: 'worker.start_turn',
        title: 'Worker turn started',
        status: 'success',
        startedAt: '2026-10-27T00:00:00.710Z',
        durationMs: 50,
      },
      {
        stepId: 'worker-binding',
        parentStepId: 'root',
        sequence: 5,
        kind: 'worker.binding',
        title: 'Worker binding',
        status: 'running',
        startedAt: '2026-10-27T00:00:00.760Z',
        category: 'worker',
        correlation: { worker: { sessionId: 'worker-session' } },
      },
    ],
  });
  const worker = model.steps.filter(
    (step) => traceStepCategory(step) === 'worker',
  );
  expect(worker.map((step) => step.kind)).toEqual([
    'worker.start_turn',
    'worker.binding',
  ]);
  const filtered = filterTraceTree(model.roots, { category: 'worker' });
  expect(filtered[0].children.map((step) => step.kind)).toEqual([
    'worker.start_turn',
    'worker.binding',
  ]);
});

it('formats durations and redacts sensitive detail keys', () => {
  expect(formatTraceDuration(450)).toBe('450ms');
  expect(formatTraceDuration(1500)).toBe('1.5s');
  expect(formatTraceDuration(65_000)).toBe('1m 5s');
  expect(formatTraceDuration(undefined)).toBe('Unavailable');
  expect(
    safeTraceDetails({
      command: 'ls',
      authorization: 'Bearer secret',
      nested: { accessToken: 'secret', output: 'ok' },
    }),
  ).toEqual({ command: 'ls', nested: { output: 'ok' } });
});

it('focuses a tool only through an exact correlation id', () => {
  const model = buildTraceModel({
    ...trace,
    steps: trace.steps.map((step) =>
      step.stepId === 'tool'
        ? { ...step, correlation: { toolId: 'tool-1' } }
        : step,
    ),
  });
  expect(traceStepForTool(model, 'tool-1')?.stepId).toBe('tool');
  expect(traceStepForTool(model, 'Shell')).toBeUndefined();
  expect(traceStepForTool(model, 'missing')).toBeUndefined();
});

it('does not double count token totals, cached input or other numeric usage fields', () => {
  const model = buildTraceModel({
    ...trace,
    run: { status: 'success' },
    steps: trace.steps.map((step) =>
      step.stepId === 'model'
        ? {
            ...step,
            details: {
              usage: {
                totalTokens: 42,
                inputTokens: 20,
                outputTokens: 22,
                cachedInputTokens: 10,
              },
            },
          }
        : step,
    ),
  });
  expect(model.summary.totalTokens).toBe(42);
});

it('keeps absent metrics unknown for incomplete traces and respects measured zero', () => {
  const model = buildTraceModel({
    ...trace,
    run: { wallDurationMs: 0 },
    coverage: { complete: false, truncated: true },
    steps: trace.steps,
  });
  expect(model.summary.wallDurationMs).toBe(0);
  expect(model.summary.totalTokens).toBeUndefined();
  expect(model.summary.modelRequestCount).toBeUndefined();
  expect(model.summary.retryCount).toBeUndefined();
});

it('does not extend a finished step to the current time when end timing is absent', () => {
  const model = buildTraceModel(
    {
      ...trace,
      steps: [{ ...trace.steps[1], durationMs: undefined, endedAt: undefined }],
    },
    Date.parse('2026-10-28T00:00:00Z'),
  );
  expect(model.steps[0].duration).toBeUndefined();
});
