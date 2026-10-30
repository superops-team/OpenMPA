import type { WecomAIBotSDK } from '@wecom/wecom-aibot-sdk';
let SDK: typeof WecomAIBotSDK | undefined;
export async function prepareWecomAuthorization() {
  if (SDK) return;
  const module =
    await import('@wecom/wecom-aibot-sdk/dist/wecom-aibot-sdk.esm.js');
  module.default.destroy();
  SDK = module.WecomAIBotSDK;
}
export class WecomAuthorizationError extends Error {
  constructor(readonly code: string) {
    super('WeCom authorization failed');
  }
}
// Prepared before the click so opening the popup retains user activation.
export function authorizeWecom(
  signal: AbortSignal,
): Promise<{ botId: string; secret: string }> {
  if (!SDK || signal.aborted)
    return Promise.reject(new WecomAuthorizationError('CANCELLED'));
  const sdk = new SDK();
  return new Promise((resolve, reject) => {
    let settled = false;
    const cleanup = () => {
      settled = true;
      clearTimeout(timer);
      signal.removeEventListener('abort', abort);
      sdk.destroy();
    };
    const fail = (code: string) => {
      if (settled) return;
      cleanup();
      reject(new WecomAuthorizationError(code));
    };
    const abort = () => fail('CANCELLED');
    const timer = setTimeout(() => fail('AUTH_TIMEOUT'), 300000);
    signal.addEventListener('abort', abort, { once: true });
    try {
      sdk
        .openBotInfoAuthWindow({
          source: 'mpa-agent',
          state: crypto.randomUUID(),
          debug: false,
        })
        .then(
          (bot) => {
            if (settled) return;
            if (
              typeof bot?.botid !== 'string' ||
              !bot.botid.trim() ||
              typeof bot?.secret !== 'string' ||
              !bot.secret.trim()
            ) {
              fail('INVALID_RESULT');
              return;
            }
            cleanup();
            resolve({ botId: bot.botid.trim(), secret: bot.secret.trim() });
          },
          () => fail('FAILED'),
        );
    } catch {
      fail('FAILED');
    }
  });
}
