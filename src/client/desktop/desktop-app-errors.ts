import type { Messages } from './generated/locales';
import { LocalizedError, renderMessage } from './locale';

export function desktopFailureMessage(
  m: Messages,
  failure: unknown,
  source: string,
) {
  const status =
    failure && typeof failure === 'object' && 'httpStatus' in failure
      ? failure.httpStatus
      : undefined;
  const remoteMessage =
    failure instanceof LocalizedError
      ? failure.text
      : failure instanceof Error
        ? failure.message
        : failure &&
            typeof failure === 'object' &&
            'message' in failure &&
            typeof failure.message === 'string'
          ? failure.message
          : undefined;
  const message =
    (remoteMessage === undefined
      ? undefined
      : renderMessage(remoteMessage, m)) ??
    (status === 404 && source === 'bots'
      ? m.desktopThisRuntimeDoesNotYetSupportDesktopBotCreation()
      : m.desktopTheOperationFailed());
  const labels: Record<string, string> = {
    sessions: m.desktopLoadConversations(),
    bots: m.desktopCreateDesktopBot(),
    workspace: m.desktopOpenLocalWorkspace(),
    connection: m.desktopReadConnection(),
  };
  return `${labels[source] || source}${status ? ` (HTTP ${status})` : ''}: ${message}`;
}
