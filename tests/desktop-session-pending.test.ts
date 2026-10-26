import { expect, it, vi } from 'vitest';
const api = vi.hoisted(() => ({
  pending: vi.fn(),
  submit: vi.fn(),
  abort: vi.fn(),
  read: vi.fn(),
  executionConfig: vi.fn(),
}));
vi.mock('../src/client/desktop/generated/mygo', () => ({
  Artifact: {},
  Host: {},
  Platform: {},
  Connection: {},
  Workspace: {},
  Deployment: {},
  Schedule: {},
  Channels: {},
  SessionBot: {},
  events: {},
  Session: api,
}));
import { createDesktopServices } from '../src/client/desktop/services';
it('blocks a new run after reopening while the Go host retains an unknown original submission', async () => {
  api.pending.mockResolvedValue({
    ok: true,
    value: {
      operations: [
        {
          sessionId: 's',
          operationId: 'original-op',
          idempotencyKey: 'original-key',
          content: 'Same prompt',
          executionConfigVersion: '4',
          state: 'unknown',
        },
      ],
    },
  });
  const first = createDesktopServices();
  await expect(
    first.sessions.submit(2, 's', 'Same prompt', '4'),
  ).rejects.toThrow(/original-op/);
  const reopened = createDesktopServices();
  await expect(
    reopened.sessions.submit(2, 's', 'Same prompt', '4'),
  ).rejects.toThrow(/original-op/);
  expect(api.submit).not.toHaveBeenCalled();
  expect(api.pending).toHaveBeenCalledTimes(2);
});
it('preserves a false abort receipt instead of claiming the session was stopped', async () => {
  api.abort.mockResolvedValue({
    ok: true,
    value: { sessionId: 's', aborted: false },
  });
  expect(await createDesktopServices().sessions.abort(2, 's')).toBe(false);
});
it('sends only the existing text run contract without a synthetic attachment field', async () => {
  api.pending.mockResolvedValue({ ok: true, value: { operations: [] } });
  api.submit.mockResolvedValue({
    ok: true,
    value: { invocationId: 'i', sessionId: 's' },
  });
  await createDesktopServices().sessions.submit(
    2,
    's',
    'Existing text contract',
    '4',
  );
  const input = api.submit.mock.calls.at(-1)?.[0];
  expect(input).toMatchObject({
    sessionId: 's',
    executionConfigVersion: '4',
    content: 'Existing text contract',
  });
  expect(input).not.toHaveProperty('attachmentRefs');
});
