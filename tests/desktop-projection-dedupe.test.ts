import { describe, expect, it } from 'vitest';

import {
  emptyProjection,
  projectEvent,
  type DisplayEvent,
  type Projection,
} from '../src/client/desktop/projection';

const scope = {
  connectionEpoch: 3,
  subscriptionEpoch: 7,
  sessionId: 'session-1',
};

function event(overrides: Partial<DisplayEvent>): DisplayEvent {
  return {
    ...scope,
    invocationId: 'inv-1',
    kind: 'text',
    role: 'assistant',
    ...overrides,
  };
}

function withOptimisticUser(text: string): Projection {
  return {
    ...emptyProjection(scope),
    invocationId: 'inv-1',
    messages: [
      {
        id: 'local-optimistic',
        role: 'user',
        content: text,
        invocationId: 'inv-1',
        partial: false,
        kind: 'message',
      },
    ],
  };
}

describe('conversation projection dedupe', () => {
  it('preserves failure when a completed terminal is replayed late', () => {
    let projection = projectEvent(
      emptyProjection(scope),
      event({
        kind: 'error',
        terminal: true,
        text: 'Run failed',
        eventId: 'failure',
      }),
    );
    projection = projectEvent(
      projection,
      event({ kind: 'done', terminal: true, eventId: 'done-late' }),
    );
    expect(projection.messages.at(-1)).toMatchObject({
      terminalStatus: 'failed',
      content: 'Run failed',
    });
  });

  it('closes reasoning at a tool boundary without completing the turn', () => {
    let projection = projectEvent(
      emptyProjection(scope),
      event({
        kind: 'reasoning',
        text: 'Checking',
        partial: true,
        durationMs: 1200,
      }),
    );
    projection = projectEvent(
      projection,
      event({ kind: 'toolCall', toolId: 'tool' }),
    );
    expect(projection.messages[0]).toMatchObject({
      partial: false,
      stepStatus: 'completed',
      content: 'Checking',
      durationMs: 1200,
    });
    expect(projection.running).toBe(true);
    expect(
      projection.messages.some((message) => message.kind === 'status'),
    ).toBe(false);
  });

  it('closes reasoning when an answer starts and keeps final text below turn authority', () => {
    let projection = projectEvent(
      emptyProjection(scope),
      event({
        kind: 'reasoning',
        text: 'Checking',
        partial: true,
      }),
    );
    projection = projectEvent(
      projection,
      event({ text: 'Answer', partial: false }),
    );
    expect(projection.messages[0].partial).toBe(false);
    expect(projection.running).toBe(true);
    expect(
      projection.messages.some((message) => message.kind === 'status'),
    ).toBe(false);
  });

  it.each(['completed', 'failed', 'stopped'] as const)(
    'settles partial steps on a %s terminal and retains successful tool evidence',
    (terminalStatus) => {
      let projection = projectEvent(
        emptyProjection(scope),
        event({
          kind: 'toolResult',
          toolId: 'finished',
          durationMs: 423,
        }),
      );
      projection = projectEvent(
        projection,
        event({ kind: 'toolCall', toolId: 'unfinished' }),
      );
      projection = projectEvent(
        projection,
        event({
          kind: 'reasoning',
          partial: true,
          text: 'Checking',
          durationMs: 1200,
        }),
      );
      projection = projectEvent(
        projection,
        event({
          kind: terminalStatus === 'failed' ? 'error' : 'done',
          terminal: true,
          terminalStatus,
        }),
      );
      expect(
        projection.messages.find((message) => message.kind === 'reasoning'),
      ).toMatchObject({
        partial: false,
        stepStatus: terminalStatus,
        durationMs: 1200,
      });
      expect(
        projection.messages.find((message) => message.toolId === 'finished'),
      ).toMatchObject({ toolStatus: 'success', durationMs: 423 });
      expect(
        projection.messages.find((message) => message.toolId === 'unfinished')
          ?.toolStatus,
      ).toBe(terminalStatus === 'failed' ? 'failed' : 'stopped');
      expect(projection.messages.at(-1)?.terminalStatus).toBe(terminalStatus);
      expect(projection.running).toBe(false);
    },
  );

  it('preserves terminal answer text and projects a separate turn completion', () => {
    let projection = projectEvent(
      emptyProjection(scope),
      event({ text: 'Answer', partial: true }),
    );
    projection = projectEvent(
      projection,
      event({ text: 'Full answer', partial: false, terminal: true }),
    );
    expect(projection.messages).toHaveLength(2);
    expect(projection.messages[0]).toMatchObject({
      kind: 'text',
      content: 'Full answer',
      partial: false,
    });
    expect(projection.messages[1]).toMatchObject({
      kind: 'status',
      terminalStatus: 'completed',
    });
    expect(projection.running).toBe(false);
  });

  it('keeps repeated tool ids in different historical invocations separate', () => {
    const historyScope = { ...scope, subscriptionEpoch: 0 };
    let projection = projectEvent(
      emptyProjection(historyScope),
      event({
        ...historyScope,
        kind: 'toolResult',
        invocationId: 'old',
        toolId: 'tool',
        durationMs: 423,
      }),
    );
    projection = projectEvent(
      projection,
      event({
        ...historyScope,
        kind: 'toolCall',
        invocationId: 'new',
        toolId: 'tool',
      }),
    );
    expect(projection.messages).toHaveLength(2);
    expect(projection.messages[0]).toMatchObject({
      invocationId: 'old',
      toolStatus: 'success',
      durationMs: 423,
    });
    expect(projection.messages[1]).toMatchObject({
      invocationId: 'new',
      toolStatus: 'running',
      durationMs: undefined,
    });
  });

  it('replaces the optimistic user message when the server echoes the same invocation', () => {
    let projection = withOptimisticUser('把本周发布计划整理成一页');
    projection = projectEvent(
      projection,
      event({
        kind: 'text',
        role: 'user',
        text: '把本周发布计划整理成一页',
        partial: false,
        eventId: 'user-echo',
      }),
    );

    const users = projection.messages.filter(
      (message) => message.role === 'user',
    );
    expect(users).toHaveLength(1);
    expect(users[0].content).toBe('把本周发布计划整理成一页');
  });

  it('merges partial assistant chunks even when every chunk has a different block id', () => {
    let projection = withOptimisticUser('你好');
    for (let index = 0; index < 20; index += 1) {
      projection = projectEvent(
        projection,
        event({
          text: `chunk-${index} `,
          partial: true,
          blockId: `block-${index}`,
          eventId: `event-${index}`,
        }),
      );
    }

    const assistants = projection.messages.filter(
      (message) => message.role === 'assistant',
    );
    expect(assistants).toHaveLength(1);
    expect(assistants[0].content).toBe(
      Array.from({ length: 20 }, (_, index) => `chunk-${index} `).join(''),
    );
    expect(assistants[0].partial).toBe(true);
  });

  it('replaces the streamed partial with the authoritative full text', () => {
    let projection = withOptimisticUser('你好');
    projection = projectEvent(
      projection,
      event({ text: '部分', partial: true, blockId: 'chunk-a', eventId: 'e1' }),
    );
    projection = projectEvent(
      projection,
      event({
        text: '部分继续',
        partial: true,
        blockId: 'chunk-b',
        eventId: 'e2',
      }),
    );
    projection = projectEvent(
      projection,
      event({
        text: '完整回复',
        partial: false,
        blockId: 'final-block',
        eventId: 'e3',
      }),
    );

    const assistants = projection.messages.filter(
      (message) => message.role === 'assistant',
    );
    expect(assistants).toHaveLength(1);
    expect(assistants[0].content).toBe('完整回复');
    expect(assistants[0].partial).toBe(false);
  });

  it('does not merge text across a tool event into one bubble', () => {
    let projection = withOptimisticUser('查一下');
    projection = projectEvent(
      projection,
      event({
        text: '先读取记录。',
        partial: true,
        blockId: 'a',
        eventId: 'e1',
      }),
    );
    projection = projectEvent(
      projection,
      event({
        kind: 'toolCall',
        text: 'toolCall: search (tool-1)',
        partial: false,
        blockId: 'tool-a',
        eventId: 'e2',
      }),
    );
    projection = projectEvent(
      projection,
      event({
        text: '记录已读取。',
        partial: true,
        blockId: 'b',
        eventId: 'e3',
      }),
    );

    const text = projection.messages.filter(
      (message) => message.kind === 'text',
    );
    const tools = projection.messages.filter(
      (message) => message.kind === 'toolCall',
    );
    expect(text).toHaveLength(2);
    expect(text.map((message) => message.content)).toEqual([
      '先读取记录。',
      '记录已读取。',
    ]);
    expect(tools).toHaveLength(1);
  });

  it('ignores a repeated event id', () => {
    let projection = emptyProjection(scope);
    const first = event({ text: '一次', partial: false, eventId: 'same' });
    projection = projectEvent(projection, first);
    projection = projectEvent(projection, first);
    expect(
      projection.messages.filter((message) => message.role === 'assistant'),
    ).toHaveLength(1);
  });

  it('updates one tool row by tool id and keeps its safe subject', () => {
    let projection = emptyProjection(scope);
    projection = projectEvent(
      projection,
      event({
        kind: 'toolCall',
        toolId: 'tool-1',
        toolName: 'read',
        toolStatus: 'running',
        toolSubject: 'src/client/desktop/Chat.tsx',
        blockId: 'call',
        eventId: 'e1',
      }),
    );
    projection = projectEvent(
      projection,
      event({
        kind: 'toolResult',
        toolId: 'tool-1',
        toolName: 'read',
        toolStatus: 'success',
        blockId: 'result',
        eventId: 'e2',
      }),
    );

    const tools = projection.messages.filter(
      (message) => message.toolId === 'tool-1',
    );
    expect(tools).toHaveLength(1);
    expect(tools[0]).toMatchObject({
      kind: 'toolResult',
      toolName: 'read',
      toolStatus: 'success',
      toolSubject: 'src/client/desktop/Chat.tsx',
    });
  });

  it('does not regress a completed tool when its call arrives late', () => {
    let projection = emptyProjection(scope);
    projection = projectEvent(
      projection,
      event({
        kind: 'toolResult',
        toolId: 'tool-1',
        toolName: 'read',
        toolStatus: 'success',
        eventId: 'result-first',
      }),
    );
    projection = projectEvent(
      projection,
      event({
        kind: 'toolCall',
        toolId: 'tool-1',
        toolName: 'read',
        toolStatus: 'running',
        toolSubject: 'src/client/desktop/Chat.tsx',
        eventId: 'call-late',
      }),
    );
    expect(
      projection.messages.find((message) => message.toolId === 'tool-1'),
    ).toMatchObject({
      kind: 'toolResult',
      toolStatus: 'success',
      toolSubject: 'src/client/desktop/Chat.tsx',
    });
  });

  it('retains reasoning text when the Runtime sends it', () => {
    const projection = projectEvent(
      emptyProjection(scope),
      event({
        kind: 'reasoning',
        text: 'visible reasoning summary',
        reasoningVisibility: 'hidden',
        eventId: 'reasoning-1',
      }),
    );
    expect(projection.messages[0]).toMatchObject({
      kind: 'reasoning',
      content: 'visible reasoning summary',
      reasoningVisibility: 'hidden',
    });
  });

  it('stops an unfinished tool when a terminal event arrives', () => {
    let projection = projectEvent(
      emptyProjection(scope),
      event({
        kind: 'toolCall',
        toolId: 'tool-1',
        toolName: 'shell',
        toolStatus: 'running',
        eventId: 'tool-1',
      }),
    );
    projection = projectEvent(
      projection,
      event({ kind: 'done', terminal: true, eventId: undefined }),
    );
    expect(
      projection.messages.find((message) => message.toolId === 'tool-1'),
    ).toMatchObject({ toolStatus: 'stopped' });
    expect(projection.messages.at(-1)).toMatchObject({
      kind: 'status',
      terminalStatus: 'completed',
    });
  });
});
