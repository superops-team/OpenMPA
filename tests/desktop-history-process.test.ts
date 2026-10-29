import { expect, it } from 'vitest';
import { mergeHistoryMessages } from '../src/client/desktop/ChatState';
import type { DisplayMessage } from '../src/client/desktop/projection';

it('retains SSE process evidence while terminal history persistence is temporarily empty', () => {
  const previous: DisplayMessage[] = [
    {
      id: 'reasoning',
      invocationId: 'turn',
      role: 'assistant',
      kind: 'reasoning',
      content: 'Recorded reasoning',
      partial: false,
    },
    {
      id: 'command',
      invocationId: 'turn',
      role: 'assistant',
      kind: 'toolResult',
      content: '',
      toolName: 'exec_command',
      toolStatus: 'success',
      durationMs: 40,
      partial: false,
    },
  ];
  const duringPersistence = mergeHistoryMessages(previous, []);
  expect(duringPersistence).toEqual(previous);
  const final: DisplayMessage = {
    id: 'answer',
    invocationId: 'turn',
    role: 'assistant',
    kind: 'text',
    content: 'Finished',
    partial: false,
  };
  const settled = mergeHistoryMessages(duringPersistence, [final]);
  expect(settled.map((message) => message.id)).toEqual([
    'reasoning',
    'command',
    'answer',
  ]);
  expect(mergeHistoryMessages(settled, [final])).toEqual(settled);
});
