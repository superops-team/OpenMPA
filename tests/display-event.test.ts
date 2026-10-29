import { expect, it } from 'vitest';
import { displayEvent } from '../src/client/desktop/services';
import { historyMessages } from '../src/client/desktop/service-events';

it('retains a terminal text answer and projects its completion during history replay', () => {
  const event = {
    schemaVersion: 2,
    connectionEpoch: 1,
    subscriptionEpoch: 0,
    sessionId: 'session',
    invocationId: 'invocation',
    kind: 'text',
    completeness: 'complete',
    partial: false,
    terminal: true,
    text: 'The final answer',
  };
  expect(displayEvent(event)).toMatchObject({
    kind: 'text',
    text: 'The final answer',
    terminal: true,
  });
  expect(historyMessages([event])).toEqual([
    expect.objectContaining({ kind: 'text', content: 'The final answer' }),
    expect.objectContaining({ kind: 'status', terminalStatus: 'completed' }),
  ]);
});

it('keeps explicit stopped authority when a terminal event has no text', () => {
  const event = {
    schemaVersion: 2,
    connectionEpoch: 1,
    subscriptionEpoch: 0,
    sessionId: 'session',
    invocationId: 'invocation',
    kind: 'stopped',
    completeness: 'complete',
    partial: false,
    terminal: true,
  };
  expect(historyMessages([event]).at(-1)).toMatchObject({
    kind: 'status',
    terminalStatus: 'stopped',
  });
});

it('maps v2 reasoning and safe tool fields into the display event', () => {
  expect(
    displayEvent({
      schemaVersion: 2,
      connectionEpoch: 1,
      subscriptionEpoch: 2,
      sessionId: 'session',
      invocationId: 'invocation',
      kind: 'toolCall',
      completeness: 'partial',
      partial: true,
      terminal: false,
      partIndex: 3,
      toolId: 'tool-1',
      toolName: 'read',
      toolStatus: 'running',
      toolSubject: 'src/client/desktop/Chat.tsx',
      durationMs: 423,
      outputPreview: 'ok',
      safeCommandSummary: {
        commandPreview: 'npm test',
      },
    }),
  ).toMatchObject({
    kind: 'toolCall',
    toolId: 'tool-1',
    toolName: 'read',
    toolStatus: 'running',
    toolSubject: 'src/client/desktop/Chat.tsx',
    durationMs: 423,
    outputPreview: 'ok',
    safeCommandSummary: {
      commandPreview: 'npm test',
    },
  });

  expect(
    displayEvent({
      schemaVersion: 2,
      connectionEpoch: 1,
      subscriptionEpoch: 2,
      sessionId: 'session',
      invocationId: 'invocation',
      kind: 'reasoning',
      completeness: 'complete',
      partial: false,
      terminal: false,
      reasoningVisibility: 'summary',
      text: 'checking runtime events',
    }),
  ).toMatchObject({
    kind: 'reasoning',
    reasoningVisibility: 'summary',
    text: 'checking runtime events',
  });
});
