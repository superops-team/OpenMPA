import { act, create, type ReactTestRenderer } from 'react-test-renderer';
import { expect, it, vi } from 'vitest';
import { DesktopChat } from '../src/client/desktop/Chat';
import type { DesktopServices } from '../src/client/desktop/models';
import { messages } from '../src/client/desktop/generated/locales';

const m = messages;

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

function sendButton(renderer: ReactTestRenderer) {
  return renderer.root.findByProps({ 'aria-label': m.sendMessage() });
}

it('treats stale running history without an active run as completed locally', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  const service = {
    sessions: {
      pending: async () => [],
      read: vi.fn(async () => ({
        session: { id: 's', title: 'Chat', status: 'running' },
        messages: [],
        completedInvocations: ['done-invocation'],
      })),
      submit: vi.fn(async () => ({ invocationId: 'next', status: 'accepted' })),
    },
  } as unknown as DesktopServices;
  let renderer!: ReactTestRenderer;
  await act(async () => {
    renderer = create(
      <DesktopChat
        services={service}
        connection={{
          epoch: 2,
          scope: 'scope',
          runtimeId: 'r',
          status: 'connected',
        }}
        session={{ id: 's', title: 'Chat' }}
        onSchedule={() => {}}
      />,
    );
  });
  expect(
    renderer.root.findAllByProps({ 'aria-label': m.stopCurrentSession() }),
  ).toHaveLength(0);
  await act(async () =>
    renderer.root
      .findByProps({ 'aria-label': m.message() })
      .props.onChange({ target: { value: 'Next' } }),
  );
  expect(sendButton(renderer).props.disabled).toBe(false);
  await act(async () => renderer.unmount());
});
it('resumes the accepted invocation with its cursor and clears an expired cursor after rebuilding history', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  const pending = {
    sessionId: 's',
    operationId: 'original',
    idempotencyKey: 'key',
    content: 'Prompt',
    state: 'accepted',
    invocationId: 'i',
    recoveryDisposition: 'subscribeAccepted',
  };
  const read = vi.fn(async () => ({
    session: { id: 's', title: 'Chat' },
    messages: [],
    activeRun: {
      operationId: 'original',
      invocationId: 'i',
      state: 'accepted',
      authority: 'localReceipt',
      recoveryDisposition: 'subscribeAccepted',
    },
  }));
  const recover = vi.fn(async () => ({
    invocationId: 'i',
    status: 'accepted',
  }));
  const submit = vi.fn();
  let calls = 0;
  const subscribe = vi.fn(
    (_epoch, _id, _invocation, subscriptionEpoch, _lastEventId) => {
      calls++;
      const call = calls;
      return {
        close() {},
        events: (async function* () {
          if (call === 1)
            yield {
              connectionEpoch: 2,
              subscriptionEpoch,
              sessionId: 's',
              invocationId: 'i',
              eventId: 'cursor-1',
              kind: 'text',
              text: 'Partial',
              partial: true,
            };
        })(),
        finished:
          call === 2
            ? Promise.reject(
                Object.assign(new Error('Cursor expired'), { httpStatus: 410 }),
              )
            : Promise.resolve(),
      };
    },
  );
  const service = {
    sessions: {
      read,
      pending: async () => [pending],
      recover,
      submit,
      subscribe,
    },
  } as unknown as DesktopServices;
  let renderer!: ReactTestRenderer;
  await act(async () => {
    renderer = create(
      <DesktopChat
        services={service}
        connection={{
          epoch: 2,
          scope: 'scope',
          runtimeId: 'r',
          status: 'connected',
        }}
        session={{ id: 's', title: 'Chat' }}
        onSchedule={() => {}}
      />,
    );
  });
  const button = () =>
    renderer.root
      .findAllByType('button')
      .find((button) =>
        button.children.includes(m.desktopResumeAcceptedInvocation()),
      )!;
  await act(async () => button().props.onClick());
  await act(async () => button().props.onClick());
  expect(JSON.stringify(renderer.toJSON())).toContain(
    m.desktopTheSavedStreamCursorExpiredHistoryHasBeenRebuiltInlineResumeTheSameInvocationAgainWithoutACursor(),
  );
  await act(async () => button().props.onClick());
  expect(
    subscribe.mock.calls.map((call) => ({
      invocation: call[2],
      cursor: call[4],
    })),
  ).toEqual([
    { invocation: 'i', cursor: undefined },
    { invocation: 'i', cursor: 'cursor-1' },
    { invocation: 'i', cursor: undefined },
  ]);
  expect(recover).toHaveBeenCalledWith(2, 'original', 'subscribeAccepted');
  expect(submit).not.toHaveBeenCalled();
  await act(async () => renderer.unmount());
});
it('reconciles a Go-verified terminal pending operation before enabling the next send', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  let submitted = false;
  let resolved = false;
  const pending = vi.fn(async () =>
    submitted && !resolved
      ? [
          {
            sessionId: 's',
            operationId: 'op',
            idempotencyKey: 'key',
            content: 'Hello',
            state: 'terminalObserved',
            invocationId: 'i',
            terminalVerified: true,
            recoveryDisposition: 'subscribeAccepted',
          },
        ]
      : [],
  );
  const resolvePending = vi.fn(async () => {
    resolved = true;
  });
  const service = {
    sessions: {
      read: async () => ({
        session: { id: 's', title: 'Chat' },
        messages: [],
        completedInvocations: submitted ? ['i'] : [],
      }),
      pending,
      resolvePending,
      submit: async () => {
        submitted = true;
        return { invocationId: 'i', status: 'accepted' };
      },
      subscribe: (
        _epoch: number,
        _id: string,
        _invocation: string,
        subscriptionEpoch: number,
      ) => ({
        events: (async function* () {
          yield {
            connectionEpoch: 2,
            subscriptionEpoch,
            sessionId: 's',
            invocationId: 'i',
            kind: 'done',
            terminal: true,
          };
        })(),
        finished: Promise.resolve(),
        close() {},
      }),
    },
  } as unknown as DesktopServices;
  let renderer!: ReactTestRenderer;
  await act(async () => {
    renderer = create(
      <DesktopChat
        services={service}
        connection={{
          epoch: 2,
          scope: 'scope',
          runtimeId: 'r',
          status: 'connected',
        }}
        session={{ id: 's', title: 'Chat' }}
        onSchedule={() => {}}
      />,
    );
  });
  await act(async () =>
    renderer.root
      .findByProps({ 'aria-label': m.message() })
      .props.onChange({ target: { value: 'Hello' } }),
  );
  await act(async () =>
    renderer.root.findByType('form').props.onSubmit({ preventDefault() {} }),
  );
  expect(resolvePending).toHaveBeenCalledWith(2, 'op', 'verifiedTerminal');
  await act(async () =>
    renderer.root
      .findByProps({ 'aria-label': m.message() })
      .props.onChange({ target: { value: 'Next' } }),
  );
  expect(
    renderer.root.findByProps({ 'aria-label': m.sendMessage() }).props.disabled,
  ).toBe(false);
  await act(async () => renderer.unmount());
});

it('releases a recovered turn on terminal without waiting for the recovered host call', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  const hostCall = deferred<void>();
  const recoveredTerminal = deferred<void>();
  const close = vi.fn();
  let historyReads = 0;
  const service = {
    sessions: {
      pending: vi
        .fn()
        .mockResolvedValueOnce([
          {
            sessionId: 's',
            operationId: 'op',
            idempotencyKey: 'key',
            content: 'Original',
            state: 'accepted',
            invocationId: 'i',
            recoveryDisposition: 'subscribeAccepted',
          },
        ])
        .mockResolvedValue([]),
      read: vi.fn(async () => {
        historyReads++;
        return historyReads <= 2
          ? {
              session: { id: 's', title: 'Chat' },
              messages: [],
              activeRun: {
                operationId: 'op',
                invocationId: 'i',
                state: 'accepted',
                authority: 'runtime',
                recoveryDisposition: 'subscribeAccepted',
              },
            }
          : {
              session: { id: 's', title: 'Chat' },
              messages: [],
              completedInvocations: ['i'],
            };
      }),
      recover: vi.fn(async () => ({ invocationId: 'i', status: 'accepted' })),
      resolvePending: vi.fn(async () => {}),
      subscribe: vi.fn((_epoch, _id, _invocation, subscriptionEpoch) => ({
        finished: hostCall.promise,
        close,
        events: (async function* () {
          recoveredTerminal.resolve();
          yield {
            connectionEpoch: 2,
            subscriptionEpoch,
            sessionId: 's',
            invocationId: 'i',
            eventId: 'done-recovered',
            kind: 'done',
            terminal: true,
          };
          await hostCall.promise;
        })(),
      })),
    },
  } as unknown as DesktopServices;
  let renderer!: ReactTestRenderer;
  await act(async () => {
    renderer = create(
      <DesktopChat
        services={service}
        connection={{
          epoch: 2,
          scope: 'scope',
          runtimeId: 'r',
          status: 'connected',
        }}
        session={{ id: 's', title: 'Chat' }}
        onSchedule={() => {}}
      />,
    );
  });
  await act(async () => {
    await Promise.resolve();
  });
  const resumeButton = renderer.root
    .findAllByType('button')
    .find((button) =>
      button.children.includes(m.desktopResumeAcceptedInvocation()),
    )!;
  await act(async () => {
    resumeButton.props.onClick();
    await recoveredTerminal.promise;
    await Promise.resolve();
  });
  await act(async () =>
    renderer.root
      .findByProps({ 'aria-label': m.message() })
      .props.onChange({ target: { value: 'Next' } }),
  );
  expect(close).toHaveBeenCalled();
  expect(sendButton(renderer).props.disabled).toBe(false);
  await act(async () => renderer.unmount());
  hostCall.resolve();
});
