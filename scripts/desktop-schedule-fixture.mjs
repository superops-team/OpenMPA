export function installScheduleFixture() {
  const base = {
    prompt: 'Summarize the latest project updates.',
    agentId: 'main',
    enabled: true,
    version: 1,
    schedule: { type: 'Daily', time: '09:00', timezone: 'Asia/Shanghai' },
    delivery: { channel: 'Web', receiveIdType: 'chat_id', bestEffort: true },
    jitterSeconds: 60,
    timeoutSeconds: 7200,
    lastRunAt: new Date().toISOString(),
    nextRunAt: new Date(Date.now() + 86400000).toISOString(),
    lastRunStatus: 'Succeeded',
    createdAt: '2026-01-01T00:00:00Z',
  };
  const tasks = Array.from({ length: 13 }, (_, index) => ({
    ...structuredClone(base),
    id: `task-${index}`,
    name:
      index === 0
        ? 'Daily project summary'
        : `Task ${String(index).padStart(2, '0')}`,
    enabled: index !== 1,
    lastRunStatus: index === 2 ? 'Failed' : 'Succeeded',
  }));
  const history = new Map();
  const creates = new Map();
  globalThis.__scheduleFixture = {
    tasks,
    failure: '',
    createFailure: false,
    runCount: 0,
  };
  const state = globalThis.__scheduleFixture;
  const ok = (value) => ({ ok: true, value });
  const failure = (code, status = 400) => ({
    ok: false,
    error: {
      code,
      httpStatus: status,
      message: code,
      retryDisposition: 'never',
    },
  });
  Object.assign(globalThis.__designOverrides, {
    'Schedule.List': ({ offset = 0, limit = 200 }) =>
      state.failure
        ? failure(state.failure)
        : ok({
            items: tasks.slice(offset, offset + limit),
            total: tasks.length,
            hasMore: offset + limit < tasks.length,
            nextOffset: offset + limit < tasks.length ? offset + limit : null,
            countAvailable: true,
            overview: { executionCount: 45, successRate: 0.8 },
          }),
    'Schedule.Read': ({ taskId }) =>
      ok({ task: tasks.find((task) => task.id === taskId) }),
    'Schedule.Runs': ({ taskId }) =>
      ok({
        items: history.get(taskId) || [],
        hasMore: false,
        countAvailable: true,
        total: (history.get(taskId) || []).length,
      }),
    'Schedule.Create': (input) => {
      if (state.createFailure) return failure('InvalidSchedule');
      if (creates.has(input.clientToken))
        return ok({ task: creates.get(input.clientToken) });
      const task = {
        ...structuredClone(base),
        ...input,
        id: `created-${tasks.length}`,
        version: 1,
      };
      delete task.connectionEpoch;
      delete task.clientToken;
      tasks.unshift(task);
      creates.set(input.clientToken, task);
      return ok({ task });
    },
    'Schedule.Update': (input) => {
      const task = tasks.find((task) => task.id === input.taskId);
      if (!task || task.version !== input.expectedVersion)
        return failure('ScheduledTaskVersionConflict', 409);
      for (const key of [
        'name',
        'prompt',
        'agentId',
        'enabled',
        'schedule',
        'delivery',
        'jitterSeconds',
        'timeoutSeconds',
      ])
        if (input[key] !== undefined) task[key] = structuredClone(input[key]);
      task.version++;
      return ok({ task });
    },
    'Schedule.Run': ({ taskId }) => {
      const run = {
        id: `run-${++state.runCount}`,
        status: 'Queued',
        sessionId: `session-run-${state.runCount}`,
        scheduledAt: new Date().toISOString(),
      };
      history.set(taskId, [run, ...(history.get(taskId) || [])]);
      return ok({ run });
    },
    'Schedule.Delete': ({ taskId }) => {
      tasks.splice(
        tasks.findIndex((task) => task.id === taskId),
        1,
      );
      history.delete(taskId);
      return ok({ removed: true, taskId });
    },
  });
}
