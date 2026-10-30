import { beforeEach, expect, it, vi } from 'vitest';
const bridge = vi.hoisted(() => ({
  list: vi.fn(),
  create: vi.fn(),
  update: vi.fn(),
  read: vi.fn(),
  runs: vi.fn(),
}));
vi.mock('../src/client/desktop/generated/mygo', () => ({
  Schedule: bridge,
  Channels: {},
  Deployment: {},
  Connection: {},
}));
import { desktopFeatures } from '../src/client/desktop/features';
const nativeTask = {
  id: 'task',
  name: 'Task',
  prompt: 'Prompt',
  agentId: 'main',
  enabled: true,
  version: 3,
  schedule: {
    type: 'Interval',
    timezone: 'Asia/Shanghai',
    intervalSeconds: 3600,
    anchorAt: '2026-10-29T00:00:00Z',
  },
  delivery: {
    channel: 'Feishu',
    appId: 'bot',
    targetId: 'chat',
    receiveIdType: 'chat_id',
    bestEffort: false,
    threadId: 'omt_thread',
    replyMessageId: 'om_reply',
  },
  jitterSeconds: 90,
  timeoutSeconds: 7200,
  lastRunAt: '2026-10-28T00:00:00Z',
  runningAt: '2026-10-29T00:00:00Z',
  createdAt: '2026-10-01T00:00:00Z',
};
beforeEach(() => vi.resetAllMocks());
it('reads every page and retains server statistics and task settings for editing', async () => {
  bridge.list
    .mockResolvedValueOnce({
      ok: true,
      value: {
        items: [nativeTask],
        countAvailable: true,
        overview: { executionCount: 27, successRate: 0.9 },
        hasMore: true,
        nextOffset: 200,
      },
    })
    .mockResolvedValueOnce({
      ok: true,
      value: { items: [{ ...nativeTask, id: 'task2' }], hasMore: false },
    });
  const api = desktopFeatures().schedules!;
  const page = await api.list(4);
  expect(page.items.map((t) => t.id)).toEqual(['task', 'task2']);
  expect(bridge.list.mock.calls[1][0].offset).toBe(200);
  expect(page.overview).toEqual({ executionCount: 27, successRate: 0.9 });
  expect(page.items[0]).toMatchObject({
    lastRun: nativeTask.lastRunAt,
    runningAt: nativeTask.runningAt,
    jitterSeconds: 90,
    timeoutSeconds: 7200,
  });
  bridge.update.mockResolvedValue({
    ok: true,
    value: { task: { ...nativeTask, name: 'Renamed', version: 4 } },
  });
  await api.update(4, { ...page.items[0], name: 'Renamed' });
  expect(bridge.update).toHaveBeenCalledWith(
    expect.objectContaining({
      connectionEpoch: 4,
      expectedVersion: 3,
      name: 'Renamed',
      jitterSeconds: 90,
      timeoutSeconds: 7200,
      delivery: nativeTask.delivery,
      schedule: nativeTask.schedule,
    }),
  );
});
it('rejects a non-advancing page cursor and does not present a partial directory', async () => {
  bridge.list.mockResolvedValue({
    ok: true,
    value: { items: [nativeTask], hasMore: true, nextOffset: 0 },
  });
  await expect(desktopFeatures().schedules!.list(4)).rejects.toThrow();
  expect(bridge.list).toHaveBeenCalledOnce();
});
it('retains the original create token after an unknown write result', async () => {
  bridge.list.mockResolvedValue({
    ok: true,
    value: { items: [nativeTask], hasMore: false },
  });
  const api = desktopFeatures().schedules!;
  const task = (await api.list(4)).items[0];
  bridge.create
    .mockResolvedValueOnce({
      ok: false,
      error: {
        code: 'outcomeUnknown',
        message: 'Unknown',
        retryDisposition: 'verifyOnly',
      },
    })
    .mockResolvedValueOnce({ ok: true, value: { task: nativeTask } });
  await expect(api.create(4, task)).rejects.toThrow();
  await api.create(4, task);
  expect(bridge.create.mock.calls[0][0].clientToken).toBe(
    bridge.create.mock.calls[1][0].clientToken,
  );
});
