import { expect, it, vi } from 'vitest';

const connection = vi.hoisted(() => ({
  status: vi.fn(),
}));

vi.mock('../src/client/desktop/generated/mygo', () => ({
  Artifact: {},
  Host: {},
  Platform: {},
  Connection: connection,
  Deployment: {},
  Schedule: {},
  Channels: {},
  Session: {},
  SessionBot: {},
  Terminal: {},
  Trace: {},
  Localization: {},
  Workspace: {},
  events: {},
}));

import { createDesktopServices } from '../src/client/desktop/services';

it('projects missing, explicit, and local test business identity states', async () => {
  const services = createDesktopServices();
  connection.status
    .mockResolvedValueOnce({
      ok: true,
      value: {
        connected: true,
        connectionEpoch: 1,
        scope: 'account/User/principal/cn-beijing/default/runtime',
        runtimeId: 'runtime',
        hasCredential: true,
        hasBusinessIdentity: false,
      },
    })
    .mockResolvedValueOnce({
      ok: true,
      value: {
        connected: true,
        connectionEpoch: 2,
        scope:
          'account/User/principal/cn-beijing/default/runtime/business/profile-one',
        runtimeId: 'runtime',
        hasCredential: true,
        hasBusinessIdentity: true,
      },
    })
    .mockResolvedValueOnce({
      ok: true,
      value: {
        connected: true,
        connectionEpoch: 3,
        scope:
          'account/User/principal/cn-beijing/default/runtime/business/test/local-user',
        runtimeId: 'runtime',
        hasCredential: true,
        hasBusinessIdentity: true,
      },
    });

  await expect(services.connection.status()).resolves.toMatchObject({
    businessIdentity: 'missing',
  });
  await expect(services.connection.status()).resolves.toMatchObject({
    businessIdentity: 'explicit',
  });
  await expect(services.connection.status()).resolves.toMatchObject({
    businessIdentity: 'localTest',
  });
});
