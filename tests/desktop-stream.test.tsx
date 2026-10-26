import { act, create, type ReactTestRenderer } from 'react-test-renderer';
import { expect, it, vi } from 'vitest';
import { DesktopChat } from '../src/client/desktop/Chat';
import type { DesktopServices } from '../src/client/desktop/models';
it('consumes a subscription while the Go call is still pending and closes without abort on unmount', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  let finish!: () => void;
  const finished = new Promise<void>((resolve) => {
    finish = resolve;
  });
  const close = vi.fn();
  const abort = vi.fn();
  const service = {
    sessions: {
      pending: async () => [],
      read: vi.fn(async () => ({
        session: { id: 's', title: 'Chat' },
        messages: [],
      })),
      submit: vi.fn(async () => ({ invocationId: 'i', status: 'accepted' })),
      subscribe: vi.fn((_epoch, _id, _invocation, subscriptionEpoch) => ({
        finished,
        close,
        events: (async function* () {
          yield {
            connectionEpoch: 2,
            subscriptionEpoch,
            sessionId: 's',
            invocationId: 'i',
            eventId: 'e',
            kind: 'text',
            text: 'Arrived before completion',
            partial: true,
          };
          await finished;
        })(),
      })),
      abort,
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
    renderer.root
      .findByProps({ 'aria-label': 'Message' })
      .props.onChange({ target: { value: 'Hello' } });
  });
  await act(async () => {
    renderer.root.findByType('form').props.onSubmit({ preventDefault() {} });
    await Promise.resolve();
  });
  expect(JSON.stringify(renderer.toJSON())).toContain(
    'Arrived before completion',
  );
  await act(async () => {
    renderer.unmount();
    finish();
  });
  expect(close).toHaveBeenCalledOnce();
  expect(abort).not.toHaveBeenCalled();
});
it('prevents a submission while authoritative initial history is still loading', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  let resolve!: (value: {
    session: { id: string; title: string };
    messages: [];
  }) => void;
  const read = new Promise<{
    session: { id: string; title: string };
    messages: [];
  }>((done) => {
    resolve = done;
  });
  const submit = vi.fn();
  const service = {
    sessions: { pending: async () => [], read: () => read, submit },
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
    renderer.root
      .findByProps({ 'aria-label': 'Message' })
      .props.onChange({ target: { value: 'Hello' } });
  });
  expect(
    renderer.root.findByProps({ 'aria-label': 'Send message' }).props.disabled,
  ).toBe(true);
  await act(async () => {
    renderer.root.findByType('form').props.onSubmit({ preventDefault() {} });
  });
  expect(submit).not.toHaveBeenCalled();
  await act(async () =>
    resolve({ session: { id: 's', title: 'Chat' }, messages: [] }),
  );
  expect(
    renderer.root.findByProps({ 'aria-label': 'Send message' }).props.disabled,
  ).toBe(false);
  await act(async () => renderer.unmount());
});
it('restores an unresolved original request after unmount and reopening and prevents a new submission', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  const pending = {
    sessionId: 's',
    operationId: 'original',
    idempotencyKey: 'key',
    content: 'Original saved prompt',
    executionConfigVersion: '3',
    state: 'unknown',
  };
  const submit = vi.fn();
  const service = {
    sessions: {
      read: async () => ({ session: { id: 's', title: 'Chat' }, messages: [] }),
      pending: async () => [pending],
      submit,
    },
  } as unknown as DesktopServices;
  const props = {
    services: service,
    connection: {
      epoch: 2,
      scope: 'scope',
      runtimeId: 'r',
      status: 'connected',
    },
    session: { id: 's', title: 'Chat' },
    onSchedule: () => {},
  };
  let renderer!: ReactTestRenderer;
  await act(async () => {
    renderer = create(<DesktopChat {...props} />);
  });
  await act(async () => renderer.unmount());
  await act(async () => {
    renderer = create(<DesktopChat {...props} />);
  });
  expect(
    renderer.root.findByProps({ 'aria-label': 'Message' }).props.value,
  ).toBe('Original saved prompt');
  expect(
    renderer.root.findByProps({ 'aria-label': 'Send message' }).props.disabled,
  ).toBe(true);
  await act(async () =>
    renderer.root.findByType('form').props.onSubmit({ preventDefault() {} }),
  );
  expect(submit).not.toHaveBeenCalled();
  await act(async () => renderer.unmount());
});
it('keeps running when the Runtime returns aborted false and reads authoritative state', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  const read = vi.fn(async () => ({
    session: { id: 's', title: 'Chat', status: 'running' },
    messages: [],
  }));
  const abort = vi.fn(async () => false);
  const service = {
    sessions: { read, pending: async () => [], abort },
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
      .findAllByType('button')
      .find((button) => button.props['aria-label'] === 'Stop current session')!
      .props.onClick(),
  );
  expect(read).toHaveBeenCalledTimes(2);
  expect(JSON.stringify(renderer.toJSON())).toContain(
    'did not acknowledge an abort',
  );
  expect(JSON.stringify(renderer.toJSON())).not.toContain('stop requested');
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
        button.children.includes('Resume accepted invocation'),
      )!;
  await act(async () => button().props.onClick());
  await act(async () => button().props.onClick());
  expect(JSON.stringify(renderer.toJSON())).toContain('cursor expired');
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
      .findByProps({ 'aria-label': 'Message' })
      .props.onChange({ target: { value: 'Hello' } }),
  );
  await act(async () =>
    renderer.root.findByType('form').props.onSubmit({ preventDefault() {} }),
  );
  expect(resolvePending).toHaveBeenCalledWith(2, 'op', 'verifiedTerminal');
  await act(async () =>
    renderer.root
      .findByProps({ 'aria-label': 'Message' })
      .props.onChange({ target: { value: 'Next' } }),
  );
  expect(
    renderer.root.findByProps({ 'aria-label': 'Send message' }).props.disabled,
  ).toBe(false);
  await act(async () => renderer.unmount());
});
