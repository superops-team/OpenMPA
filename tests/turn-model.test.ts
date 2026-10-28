import { describe, expect, it } from 'vitest';
import {
  buildConversationTurns,
  finalAssistantText,
  processSegments,
  type ConversationSegment,
} from '../src/client/desktop/turn-model';
import type { DisplayMessage } from '../src/client/desktop/projection';

function message(
  overrides: Partial<DisplayMessage> & Pick<DisplayMessage, 'id' | 'kind'>,
): DisplayMessage {
  return {
    role: 'assistant',
    content: '',
    invocationId: 'inv-1',
    partial: false,
    ...overrides,
  };
}

describe('conversation turn model', () => {
  it('keeps text, hidden reasoning and one correlated tool in event order', () => {
    const turns = buildConversationTurns([
      message({ id: 'user', kind: 'text', role: 'user', content: '检查项目' }),
      message({
        id: 'reasoning',
        kind: 'reasoning',
        reasoningVisibility: 'hidden',
      }),
      message({
        id: 'tool',
        kind: 'toolCall',
        toolId: 'tool-1',
        toolName: 'read',
        toolStatus: 'success',
        toolSubject: 'src/client/desktop/Chat.tsx',
      }),
      message({ id: 'answer', kind: 'text', content: '检查完成。' }),
      message({
        id: 'terminal',
        kind: 'status',
        terminalStatus: 'completed',
      }),
    ]);

    expect(turns).toHaveLength(1);
    expect(turns[0].status).toBe('completed');
    expect(turns[0].segments.map((segment) => segment.type)).toEqual([
      'text',
      'reasoning',
      'tool',
      'text',
      'status',
    ]);
    const tool = turns[0].segments.find(
      (segment): segment is Extract<ConversationSegment, { type: 'tool' }> =>
        segment.type === 'tool',
    );
    expect(tool).toMatchObject({
      toolId: 'tool-1',
      name: 'read',
      status: 'success',
      subject: 'src/client/desktop/Chat.tsx',
    });
  });

  it('keeps separate invocations as separate turns', () => {
    const turns = buildConversationTurns([
      message({ id: 'a', kind: 'text', invocationId: 'inv-1', content: 'A' }),
      message({ id: 'b', kind: 'text', invocationId: 'inv-2', content: 'B' }),
    ]);
    expect(turns.map((turn) => turn.invocationId)).toEqual(['inv-1', 'inv-2']);
  });

  it('keeps pre-tool narration inside the process and the final answer outside it', () => {
    const [turn] = buildConversationTurns([
      message({ id: 'user', kind: 'text', role: 'user', content: '检查项目' }),
      message({ id: 'before', kind: 'text', content: '先读取代码。' }),
      message({
        id: 'tool',
        kind: 'toolCall',
        toolId: 'tool-1',
        toolName: 'read',
        toolStatus: 'success',
      }),
      message({ id: 'answer', kind: 'text', content: '检查完成。' }),
    ]);
    expect(processSegments(turn).map((segment) => segment.id)).toEqual([
      'before',
      'tool',
    ]);
    expect(finalAssistantText(turn)?.id).toBe('answer');
  });
});
