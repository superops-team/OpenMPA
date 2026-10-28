import { expect, it } from 'vitest';
import { displayEvent } from '../src/client/desktop/services';

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
    }),
  ).toMatchObject({
    kind: 'toolCall',
    toolId: 'tool-1',
    toolName: 'read',
    toolStatus: 'running',
    toolSubject: 'src/client/desktop/Chat.tsx',
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
      reasoningVisibility: 'hidden',
      text: 'must not render',
    }),
  ).toMatchObject({
    kind: 'reasoning',
    reasoningVisibility: 'hidden',
  });
});
