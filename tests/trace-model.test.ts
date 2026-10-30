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
  expect(model.workerSteps.map((step) => step.kind)).toEqual([
    'worker.start_turn',
    'worker.binding',
  ]);
});

it('preserves authoritative counts and derives timings from recorded evidence', () => {
  const model = buildTraceModel(
    {
      ...trace,
      run: {
        status: 'success',
        startedAt: '2026-10-27T00:00:00Z',
        wallDurationMs: 95_000,
        modelRequestCount: 0,
        toolCount: 0,
      },
      steps: [
        trace.steps[0],
        {
          stepId: 'usage',
          parentStepId: 'root',
          sequence: 2,
          kind: 'usage.updated',
          title: 'Model usage',
          status: 'success',
          startedAt: '2026-10-27T00:00:00.200Z',
          durationMs: 3533,
          details: {
            inputTokens: 9174,
            outputTokens: 53,
            totalTokens: 9227,
            firstContentFrameMs: 2107,
          },
        },
        {
          stepId: 'tool-accepted',
          parentStepId: 'root',
          sequence: 3,
          kind: 'tool.accepted',
          title: 'exec_command',
          status: 'success',
          startedAt: '2026-10-27T00:00:04Z',
          durationMs: 617,
        },
        {
          stepId: 'answer',
          parentStepId: 'root',
          sequence: 4,
          kind: 'codex.turn.message_delta',
          title: 'Reply output',
          status: 'success',
          startedAt: '2026-10-27T00:00:05Z',
        },
      ],
    },
    Date.parse('2026-10-27T00:00:06Z'),
  );
  expect(model.summary.modelRequestCount).toBe(0);
  expect(model.summary.toolCount).toBe(0);
  expect(model.summary.totalTokens).toBe(9227);
  expect(model.summary.firstModelContentMs).toBe(2107);
  expect(model.summary.firstVisibleAnswerMs).toBe(5000);
  expect(
    traceStepCategory(model.steps.find((step) => step.stepId === 'usage')!),
  ).toBe('model');
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

it('uses observed step evidence for incomplete traces and respects measured zero', () => {
  const model = buildTraceModel({
    ...trace,
    run: { wallDurationMs: 0 },
    coverage: { complete: false, truncated: true },
    steps: trace.steps,
  });
  expect(model.summary.wallDurationMs).toBe(0);
  expect(model.summary.totalTokens).toBe(42);
  expect(model.summary.modelRequestCount).toBe(1);
  expect(model.summary.retryCount).toBeUndefined();
});

it('honors v1.4 aggregate coverage and does not count tooling preparation as a tool call', () => {
  const model = buildTraceModel({
    ...trace,
    traceVersion: '1.4',
    run: { status: 'success' },
    coverage: {
      complete: true,
      aggregateComplete: 'unknown',
      workerTrajectory: { status: 'unavailable' },
    },
    steps: [{ ...trace.steps[0], kind: 'tooling.refresh_check' }],
  });
  expect(model.coverage.complete).toBe(false);
  expect(model.summary.toolCount).toBeUndefined();
  expect(model.summary.modelRequestCount).toBeUndefined();
});

it('does not add multiple cumulative usage snapshots into a fake total', () => {
  const model = buildTraceModel({
    ...trace,
    run: { status: 'success' },
    steps: [100, 200].map((totalTokens, index) => ({
      ...trace.steps[0],
      stepId: `usage-${index}`,
      kind: 'codex.usage.updated',
      details: { totalTokens },
    })),
  });
  expect(model.summary.totalTokens).toBeUndefined();
  expect(model.summary.modelRequestCount).toBe(0);
});

it('treats a Runtime milestone as an occurrence, not a zero-cost operation', () => {
  const model = buildTraceModel({
    ...trace,
    steps: [
      ...trace.steps,
      {
        ...trace.steps[0],
        stepId: 'worker-event',
        kind: 'codex.turn.reasoning_delta',
        durationMs: 0,
        timing: { kind: 'milestone' },
      },
    ],
  });
  const event = model.steps.find((step) => step.stepId === 'worker-event')!;
  expect(event.duration).toBeUndefined();
  expect(event.offsetMs).toBeDefined();
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
