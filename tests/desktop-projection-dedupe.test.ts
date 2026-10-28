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

  it('retains a hidden reasoning marker without its text', () => {
    const projection = projectEvent(
      emptyProjection(scope),
      event({
        kind: 'reasoning',
        text: 'must not render',
        reasoningVisibility: 'hidden',
        eventId: 'reasoning-1',
      }),
    );
    expect(projection.messages[0]).toMatchObject({
      kind: 'reasoning',
      content: '',
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
