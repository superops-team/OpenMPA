import { act, create, type ReactTestRenderer } from 'react-test-renderer';
import { readFileSync } from 'node:fs';
import { expect, it, vi } from 'vitest';
import { DesktopChat } from '../src/client/desktop/Chat';
import type { DesktopServices } from '../src/client/desktop/models';

const connection = {
  epoch: 2,
  scope: 'scope',
  runtimeId: 'r',
  status: 'connected',
};

function byClass(renderer: ReactTestRenderer, name: string) {
  return renderer.root.findAll(
    (node) =>
      typeof node.props.className === 'string' &&
      node.props.className.split(/\s+/).includes(name),
  );
}

it('keeps the scrollbar in the outer chat viewport and reserves a reading gutter', () => {
  const css = readFileSync(
    new URL('../src/client/desktop/style.css', import.meta.url),
    'utf8',
  );
  expect(css).toMatch(
    /\.desktop-app \.persistent-chat\.live-chat\s*\{[^}]*max-width:\s*none;/s,
  );
  expect(css).toMatch(
    /\.desktop-app \.chat-transcript\s*\{[^}]*scrollbar-gutter:\s*stable;[^}]*padding-inline:\s*max\(/s,
  );
  expect(css).toMatch(
    /\.desktop-app \.chat-(?:persona|composer)[^{]*\{[^}]*max-width:\s*760px;/s,
  );
});

it('keeps one composer field, one status block and the send action in the bottom bar', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  const service = {
    sessions: {
      pending: async () => [
        {
          sessionId: 's',
          operationId: 'op-1',
          idempotencyKey: 'key-1',
          content: 'hello',
          state: 'accepted',
          recoveryDisposition: 'subscribeAccepted',
        },
      ],
      read: vi.fn(async () => ({
        session: { id: 's', title: 'Chat' },
        messages: [
          {
            id: 'u1',
            role: 'user',
            content: 'hello',
            invocationId: 'i',
            partial: false,
            kind: 'message',
          },
        ],
      })),
    },
  } as unknown as DesktopServices;
  let renderer!: ReactTestRenderer;
  await act(async () => {
    renderer = create(
      <DesktopChat
        services={service}
        connection={connection}
        session={{ id: 's', title: 'Chat' }}
        onSchedule={() => {}}
      />,
    );
  });

  expect(byClass(renderer, 'composer-field')).toHaveLength(1);
  expect(byClass(renderer, 'composer-bar')).toHaveLength(1);
  expect(byClass(renderer, 'chat-status')).toHaveLength(1);
  expect(byClass(renderer, 'document-save-notice')).toHaveLength(0);
  const bar = byClass(renderer, 'composer-bar')[0];
  expect(
    bar.findAllByProps({ 'aria-label': 'Send message' }).length,
  ).toBeGreaterThan(0);
  expect(
    renderer.root.findByProps({ 'aria-label': 'Message' }).props.rows,
  ).toBe(1);

  await act(async () => renderer.unmount());
});

it('puts the stop action in the same composer slot while the run is active', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  const service = {
    sessions: {
      pending: async () => [],
      read: vi.fn(async () => ({
        session: { id: 's', title: 'Chat', status: 'running' },
        messages: [],
      })),
      abort: vi.fn(async () => true),
    },
  } as unknown as DesktopServices;
  let renderer!: ReactTestRenderer;
  await act(async () => {
    renderer = create(
      <DesktopChat
        services={service}
        connection={connection}
        session={{ id: 's', title: 'Chat' }}
        onSchedule={() => {}}
      />,
    );
  });

  const bar = byClass(renderer, 'composer-bar')[0];
  expect(
    bar.findAllByProps({ 'aria-label': 'Stop current session' }).length,
  ).toBe(1);
  expect(
    renderer.root.findAllByProps({ 'aria-label': 'Send message' }),
  ).toHaveLength(0);

  await act(async () => renderer.unmount());
});

it('renders tool events as compact rows instead of assistant bubbles', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  const service = {
    sessions: {
      pending: async () => [],
      read: vi.fn(async () => ({
        session: { id: 's', title: 'Chat' },
        messages: [
          {
            id: 'tool-1',
            role: 'assistant',
            content: 'toolCall: search (tool-1)',
            invocationId: 'i',
            partial: false,
            kind: 'toolCall',
          },
          {
            id: 'answer-1',
            role: 'assistant',
            content: '记录已读取。',
            invocationId: 'i',
            partial: false,
            kind: 'text',
          },
        ],
      })),
    },
  } as unknown as DesktopServices;
  let renderer!: ReactTestRenderer;
  await act(async () => {
    renderer = create(
      <DesktopChat
        services={service}
        connection={connection}
        session={{ id: 's', title: 'Chat' }}
        onSchedule={() => {}}
      />,
    );
  });

  expect(byClass(renderer, 'chat-tool')).toHaveLength(1);
  expect(byClass(renderer, 'assistant')).toHaveLength(1);
  expect(JSON.stringify(renderer.toJSON())).toContain('记录已读取。');

  await act(async () => renderer.unmount());
});
