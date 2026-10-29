import { expect, it, vi } from 'vitest';

const connection = vi.hoisted(() => ({
  prewarmIdentity: vi.fn(),
  status: vi.fn(),
}));
const platform = vi.hoisted(() => ({
  forgetUserTokenVault: vi.fn(),
  saveUserTokenVault: vi.fn(),
  unlockUserTokenVault: vi.fn(),
  userTokenVaultStatus: vi.fn(),
}));

vi.mock('../src/client/desktop/generated/mygo', () => ({
  Artifact: {},
  Host: {},
  Platform: platform,
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

it('exposes Runtime user token vault APIs without projecting token material', async () => {
  const services = createDesktopServices();
  platform.saveUserTokenVault.mockResolvedValueOnce({
    ok: true,
    value: {
      exists: true,
      runtimeId: 'runtime-1',
      issuer: 'https://issuer.example',
      clientId: 'client-1',
      subject: 'user-1',
      mpaId: 'mi-1',
      spaceId: 'space-1',
      expiresAt: '2026-10-29T12:00:00Z',
    },
  });
  platform.userTokenVaultStatus.mockResolvedValueOnce({
    ok: true,
    value: {
      exists: true,
      runtimeId: 'runtime-1',
      subject: 'user-1',
    },
  });
  platform.unlockUserTokenVault
    .mockResolvedValueOnce({
      ok: true,
      value: {
        exists: true,
        runtimeId: 'runtime-1',
        subject: 'user-1',
      },
    })
    .mockResolvedValueOnce({
      ok: false,
      error: {
        code: 'vaultWrongPassphrase',
        message: 'The passphrase did not unlock the saved credentials.',
        httpStatus: 0,
      },
    });
  platform.forgetUserTokenVault.mockResolvedValueOnce({
    ok: true,
    value: {},
  });

  const input = {
    passphrase: 'correct horse battery',
    runtimeId: 'runtime-1',
    issuer: 'https://issuer.example',
    clientId: 'client-1',
    subject: 'user-1',
    mpaId: 'mi-1',
    spaceId: 'space-1',
    idToken: 'id-token-secret',
    refreshToken: 'refresh-token-secret',
    expiresAt: '2026-10-29T12:00:00Z',
  };
  await expect(services.platform.saveUserTokenVault!(input)).resolves.toEqual(
    expect.objectContaining({
      exists: true,
      runtimeId: 'runtime-1',
      subject: 'user-1',
    }),
  );
  expect(platform.saveUserTokenVault).toHaveBeenCalledWith(input);
  await expect(
    services.platform.userTokenVaultStatus!('runtime-1'),
  ).resolves.toEqual(
    expect.not.objectContaining({
      idToken: expect.anything(),
      refreshToken: expect.anything(),
    }),
  );
  await expect(
    services.platform.unlockUserTokenVault!(
      'runtime-1',
      'correct horse battery',
      7,
    ),
  ).resolves.toEqual(
    expect.not.objectContaining({
      idToken: expect.anything(),
      refreshToken: expect.anything(),
    }),
  );
  await expect(
    services.platform.unlockUserTokenVault!(
      'runtime-1',
      'id-token-secret-refresh-token-secret',
      7,
    ),
  ).rejects.not.toThrow(/id-token-secret|refresh-token-secret/);
  expect(platform.unlockUserTokenVault).toHaveBeenCalledWith({
    runtimeId: 'runtime-1',
    passphrase: 'correct horse battery',
    connectionEpoch: 7,
  });
  await expect(
    services.platform.forgetUserTokenVault!('runtime-1'),
  ).resolves.toBeUndefined();
});
