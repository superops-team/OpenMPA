import { expect, it, vi } from 'vitest';
const api = vi.hoisted(() => ({ sessionList: vi.fn(), botList: vi.fn() }));
vi.mock('../src/client/desktop/generated/mygo', () => ({
  Artifact: {},
  Host: {},
  Platform: {},
  Connection: {},
  Workspace: {},
  Deployment: {},
  Schedule: {},
  Channels: {},
  events: {},
  Session: { list: api.sessionList },
  SessionBot: { list: api.botList },
}));
import { createDesktopServices } from '../src/client/desktop/services';
it('lists the Runtime Sessions without probing an undeployed SessionBot API', async () => {
  api.sessionList.mockResolvedValue({
    ok: true,
    value: { sessions: [{ id: 's', title: 'Existing conversation' }] },
  });
  const services = createDesktopServices();
  expect(await services.sessions.list(4)).toEqual([
    expect.objectContaining({ id: 's', title: 'Existing conversation' }),
  ]);
  expect(api.sessionList).toHaveBeenCalledWith({ connectionEpoch: 4 });
  expect(api.botList).not.toHaveBeenCalled();
});
