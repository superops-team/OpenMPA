import { act, create, type ReactTestRenderer } from 'react-test-renderer';
import { expect, it, vi } from 'vitest';
const fixture = vi.hoisted(() => ({
  runs: 0,
  operations: [] as Array<{
    sessionId: string;
    operationId: string;
    idempotencyKey: string;
    content: string;
    state: string;
    invocationId: string;
    terminalVerified: boolean;
  }>,
  resolve: vi.fn(),
  submit: vi.fn(),
}));
vi.mock('mygo-runtime', () => ({
  Channel: class {
    values: unknown[] = [];
    async *[Symbol.asyncIterator]() {
      yield* this.values;
    }
    close() {}
  },
}));
vi.mock('../src/client/desktop/generated/mygo', () => ({
  Artifact: {},
  Host: {},
  Platform: {},
  Connection: {},
  Workspace: {},
  Deployment: {},
  Schedule: {},
  Channels: {},
  SessionBot: {},
  events: {},
  Session: {
    read: async () => ({
      ok: true,
      value: {
        summary: { id: 's', title: 'Chat', status: 'idle', lastUpdateTime: 0 },
        events: fixture.runs
          ? [
              {
                schemaVersion: 1,
                connectionEpoch: 2,
                subscriptionEpoch: 0,
                sessionId: 's',
                invocationId: `i${fixture.runs}`,
                eventId: `final${fixture.runs}`,
                blockId: 'part',
                kind: 'text',
                text: 'ADK final without terminal field',
                author: 'assistant',
                partial: false,
                terminal: false,
              },
            ]
          : [],
        complete: true,
      },
    }),
    executionConfig: async () => ({
      ok: true,
      value: { executionConfigVersion: '3', profileBound: true },
    }),
    pending: async () => ({
      ok: true,
      value: { operations: fixture.operations },
    }),
    submit: async (request: {
      operationId: string;
      idempotencyKey: string;
      content: string;
    }) => {
      fixture.submit(request);
      fixture.runs++;
      const invocationId = `i${fixture.runs}`;
      fixture.operations = [
        {
          sessionId: 's',
          ...request,
          invocationId,
          state: 'accepted',
          terminalVerified: false,
        },
      ];
      return {
        ok: true,
        value: {
          sessionId: 's',
          invocationId,
          operationId: request.operationId,
        },
      };
    },
    subscribe: async (
      request: { subscriptionEpoch: number; invocationId: string },
      channel: { values: unknown[] },
    ) => {
      fixture.operations[0] = {
        ...fixture.operations[0],
        state: 'terminalObserved',
        terminalVerified: true,
      };
      channel.values.push({
        schemaVersion: 1,
        connectionEpoch: 2,
        subscriptionEpoch: request.subscriptionEpoch,
        sessionId: 's',
        invocationId: request.invocationId,
        kind: 'done',
        completeness: 'complete',
        partial: false,
        terminal: true,
      });
      return { ok: true, value: {} };
    },
    resolvePending: async (request: { operationId: string; mode: string }) => {
      fixture.resolve(request);
      fixture.operations = [];
      return { ok: true, value: {} };
    },
  },
}));
import { createDesktopServices } from '../src/client/desktop/services';
import { DesktopChat } from '../src/client/desktop/Chat';
it('sends two consecutive turns through the actual adapter while resolving only host-verified terminal receipts', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  fixture.runs = 0;
  fixture.operations = [];
  let renderer!: ReactTestRenderer;
  await act(async () => {
    renderer = create(
      <DesktopChat
        services={createDesktopServices()}
        connection={{
          epoch: 2,
          runtimeId: 'r',
          scope: 'scope',
          status: 'ready',
        }}
        session={{ id: 's', title: 'Chat' }}
        onSchedule={() => {}}
      />,
    );
  });
  for (const content of ['First', 'Second']) {
    await act(async () =>
      renderer.root
        .findByProps({ 'aria-label': 'Message' })
        .props.onChange({ target: { value: content } }),
    );
    expect(
      renderer.root.findByProps({ 'aria-label': 'Send message' }).props
        .disabled,
    ).toBe(false);
    await act(async () =>
      renderer.root.findByType('form').props.onSubmit({ preventDefault() {} }),
    );
  }
  expect(fixture.submit).toHaveBeenCalledTimes(2);
  expect(fixture.resolve).toHaveBeenCalledTimes(2);
  expect(fixture.operations).toEqual([]);
  expect(
    fixture.resolve.mock.calls.every(
      ([request]) => request.mode === 'verifiedTerminal',
    ),
  ).toBe(true);
  await act(async () => renderer.unmount());
});
