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
  it('uses completed invocation authority to settle historical steps without inventing tool success', () => {
    const messages = [
      message({
        id: 'reasoning',
        kind: 'reasoning',
        partial: true,
        durationMs: 1200,
      }),
      message({ id: 'tool', kind: 'toolCall', toolStatus: 'running' }),
      message({
        id: 'success',
        kind: 'toolResult',
        toolStatus: 'success',
        durationMs: 423,
      }),
      message({
        id: 'active',
        kind: 'reasoning',
        invocationId: 'inv-2',
        partial: true,
      }),
    ];
    const [completed, active] = buildConversationTurns(messages, ['inv-1']);
    expect(completed.status).toBe('completed');
    expect(completed.segments[0]).toMatchObject({
      partial: false,
      status: 'completed',
      durationMs: 1200,
    });
    expect(completed.segments[1]).toMatchObject({ status: 'stopped' });
    expect(completed.segments[2]).toMatchObject({
      status: 'success',
      durationMs: 423,
    });
    expect(active.status).toBe('running');
    expect(active.segments[0]).toMatchObject({ partial: true });
    expect(messages[0].partial).toBe(true);
  });

  it.each(['failed', 'stopped'] as const)(
    'preserves explicit %s authority over the completed list',
    (status) => {
      const [turn] = buildConversationTurns(
        [
          message({ id: 'terminal', kind: 'status', terminalStatus: status }),
          message({ id: 'reasoning', kind: 'reasoning', partial: true }),
          message({ id: 'tool', kind: 'toolCall' }),
        ],
        ['inv-1'],
      );
      expect(turn.status).toBe(status);
      expect(turn.segments[1]).toMatchObject({ partial: false, status });
      expect(turn.segments[2]).toMatchObject({ status });
    },
  );

  it('keeps completed steps complete when a later step fails', () => {
    const [turn] = buildConversationTurns([
      message({ id: 'reasoning', kind: 'reasoning', partial: false }),
      message({ id: 'terminal', kind: 'status', terminalStatus: 'failed' }),
    ]);
    expect(turn.segments[0]).toMatchObject({ partial: false });
    expect(turn.status).toBe('failed');
  });

  it('does not treat complete assistant text as whole turn completion', () => {
    const [turn] = buildConversationTurns([
      message({ id: 'final', kind: 'text', content: 'Answer', partial: false }),
    ]);
    expect(turn.status).toBe('running');
  });

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
        toolName: 'commandExecution',
        toolStatus: 'success',
        toolSubject: '/bin/bash -lc pwd',
        durationMs: 617,
        outputPreview: '/data/workspace',
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
      name: 'commandExecution',
      status: 'success',
      subject: '/bin/bash -lc pwd',
      durationMs: 617,
      outputPreview: '/data/workspace',
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

  it('keeps streamed answer deltas out of the process when the final answer arrives', () => {
    const [turn] = buildConversationTurns([
      message({ id: 'user', kind: 'text', role: 'user', content: '介绍项目' }),
      message({
        id: 'tool',
        kind: 'toolCall',
        toolId: 'tool-1',
        toolName: 'commandExecution',
        toolStatus: 'success',
      }),
      message({
        id: 'stream',
        kind: 'text',
        content: '### 当前工作环境硬件配置信息',
        partial: true,
      }),
      message({
        id: 'answer',
        kind: 'text',
        content: '### 当前工作环境硬件配置信息',
        partial: false,
      }),
    ]);
    expect(finalAssistantText(turn)?.id).toBe('answer');
    expect(processSegments(turn).map((segment) => segment.id)).toEqual([
      'tool',
    ]);
  });
});
