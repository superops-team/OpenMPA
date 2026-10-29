import { expect, it, vi } from 'vitest';

const connection = vi.hoisted(() => ({
  prewarmIdentity: vi.fn(),
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

it('prewarms Runtime identity without projecting token material', async () => {
  const services = createDesktopServices();
  connection.prewarmIdentity
    .mockResolvedValueOnce({
      ok: true,
      value: { senderId: 'sender-1' },
    })
    .mockResolvedValueOnce({
      ok: false,
      error: {
        code: 'invalidInput',
        message: 'The request contains invalid input.',
        httpStatus: 400,
      },
    });

  await expect(
    services.connection.prewarmIdentity!(
      7,
      'id-token-secret',
      'refresh-token-secret',
    ),
  ).resolves.toEqual({ senderId: 'sender-1' });
  expect(connection.prewarmIdentity).toHaveBeenCalledWith({
    connectionEpoch: 7,
    idToken: 'id-token-secret',
    refreshToken: 'refresh-token-secret',
  });

  await expect(
    services.connection.prewarmIdentity!(
      7,
      'id-token-secret',
      'refresh-token-secret',
    ),
  ).rejects.not.toThrow(/id-token-secret|refresh-token-secret/);
});
