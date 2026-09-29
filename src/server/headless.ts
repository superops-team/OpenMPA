import {
  CopilotKitCore,
  CopilotKitCoreRuntimeConnectionStatus,
} from '@copilotkit/core';
import type { Message } from '@ag-ui/core';
import { randomUUID } from 'node:crypto';
export function currentTurnText(messages: Message[], error?: Error): string {
  if (error) throw error;
  const content = messages
    .filter((message) => message.role === 'assistant')
    .at(-1)?.content;
  if (typeof content !== 'string' || !content.trim())
    throw new Error('The current compute turn returned no assistant response.');
  return content;
}
export async function runThreadTurn(
  runtimeUrl: string,
  headers: Record<string, string>,
  dotId: string,
  threadId: string,
  prompt: string,
  signal: AbortSignal,
): Promise<string> {
  signal.throwIfAborted();
  const core = new CopilotKitCore({
    runtimeUrl,
    headers,
    deferInitialConnection: true,
  });
  const { agent, unregister } = core.registerProxiedAgent({
    agentId: `worker-${randomUUID()}`,
    runtimeAgentId: dotId,
  });
  agent.threadId = threadId;
  let runError: Error | undefined;
  const errorSubscription = core.subscribe({
    onError: ({ error }) => {
      runError = error;
    },
  });
  const agentSubscription = agent.subscribe({
    onRunErrorEvent: ({ event }) => {
      runError = new Error(event.message);
    },
  });
  const stop = () => core.stopAgent({ agent });
  signal.addEventListener('abort', stop, { once: true });
  try {
    await new Promise<void>((resolve, reject) => {
      const cleanup = () => {
        clearTimeout(timer);
        subscription.unsubscribe();
        signal.removeEventListener('abort', aborted);
      };
      const aborted = () => {
        cleanup();
        reject(signal.reason ?? new Error('Run cancelled.'));
      };
      const timer = setTimeout(() => {
        cleanup();
        reject(new Error('Intelligence runtime connection timed out.'));
      }, 15_000);
      const subscription = core.subscribe({
        onRuntimeConnectionStatusChanged: ({ status }) => {
          if (status === CopilotKitCoreRuntimeConnectionStatus.Connected) {
            cleanup();
            resolve();
          } else if (status === CopilotKitCoreRuntimeConnectionStatus.Error) {
            cleanup();
            reject(new Error('Intelligence runtime connection failed.'));
          }
        },
      });
      signal.addEventListener('abort', aborted, { once: true });
      core.connect();
    });
    signal.throwIfAborted();
    await core.connectAgent({ agent });
    signal.throwIfAborted();
    if (runError) throw runError;
    agent.addMessage({ id: randomUUID(), role: 'user', content: prompt });
    const result = await core.runAgent({ agent });
    signal.throwIfAborted();
    return currentTurnText(result.newMessages, runError);
  } finally {
    signal.removeEventListener('abort', stop);
    errorSubscription.unsubscribe();
    agentSubscription.unsubscribe();
    unregister();
    core.setRuntimeUrl(undefined);
  }
}
