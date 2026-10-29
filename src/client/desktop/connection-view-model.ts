import { messages } from './generated/locales';
import type { ConnectionState } from './generated/mygo';
import type { ConnectionVM } from './models';

export const connectionVM = (state: ConnectionState): ConnectionVM => ({
  epoch: state.connectionEpoch,
  scope: state.scope,
  runtimeId: state.runtimeId,
  runtimeName: state.runtimeName,
  workerToolId: state.workerToolId,
  runtimeImage: state.runtimeImage,
  status: state.connected
    ? state.hasBusinessIdentity
      ? 'Explicit business identity'
      : state.hasCredential
        ? messages.runtimeKeyStatus()
        : messages.authenticationRequired()
    : 'Disconnected',
  businessIdentity: !state.connected
    ? 'unknown'
    : state.hasBusinessIdentity
      ? state.scope.includes('/business/test/')
        ? 'localTest'
        : 'explicit'
      : state.hasCredential
        ? 'missing'
        : 'unknown',
});
