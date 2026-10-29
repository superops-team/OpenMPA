import { Platform } from './generated/mygo';
import type { DesktopServices } from './models';
import { unwrap } from './result';

type PlatformServices = DesktopServices['platform'];

export function createPlatformIdentityServices(): Pick<
  PlatformServices,
  | 'agentIdentityConfig'
  | 'saveAgentIdentityConfig'
  | 'loginAgentIdentity'
  | 'userTokenVaultStatus'
  | 'saveUserTokenVault'
  | 'unlockUserTokenVault'
  | 'forgetUserTokenVault'
> {
  return {
    agentIdentityConfig: async () => unwrap(await Platform.agentIdentityConfig()),
    saveAgentIdentityConfig: async (userPoolId) =>
      unwrap(await Platform.saveAgentIdentityConfig({ userPoolId })),
    loginAgentIdentity: async (
      runtimeId,
      userPoolId,
      passphrase,
      connectionEpoch,
    ) =>
      unwrap(
        await Platform.loginAgentIdentity({
          runtimeId,
          userPoolId,
          passphrase,
          connectionEpoch,
        }),
      ),
    userTokenVaultStatus: async (runtimeId) =>
      unwrap(await Platform.userTokenVaultStatus({ runtimeId })),
    saveUserTokenVault: async (input) =>
      unwrap(await Platform.saveUserTokenVault(input)),
    unlockUserTokenVault: async (runtimeId, passphrase, connectionEpoch) =>
      unwrap(
        await Platform.unlockUserTokenVault({
          runtimeId,
          passphrase,
          connectionEpoch,
        }),
      ),
    forgetUserTokenVault: async (runtimeId) => {
      unwrap(await Platform.forgetUserTokenVault({ runtimeId }));
    },
  };
}
