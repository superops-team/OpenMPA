import { act, create, type ReactTestRenderer } from 'react-test-renderer';
import { expect, it, vi } from 'vitest';
import { DesktopChat } from '../src/client/desktop/Chat';
import type { DesktopServices } from '../src/client/desktop/models';

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
  return renderer.root.findByProps({ 'aria-label': 'Send message' });
}

function stopButton(renderer: ReactTestRenderer) {
  return renderer.root.findByProps({ 'aria-label': 'Stop current session' });
}

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
it('releases the next turn immediately after a terminal event even when the host call hangs', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  const hostCall = deferred<void>();
  const terminalSent = deferred<void>();
  const close = vi.fn();
  const read = vi
    .fn()
    .mockResolvedValueOnce({
      session: { id: 's', title: 'Chat' },
      messages: [],
    })
    .mockResolvedValue({
      session: { id: 's', title: 'Chat' },
      messages: [],
      completedInvocations: ['i'],
    });
  const service = {
    sessions: {
      pending: async () => [],
      read,
      submit: vi.fn(async () => ({ invocationId: 'i', status: 'accepted' })),
      subscribe: vi.fn((_epoch, _id, _invocation, subscriptionEpoch) => ({
        finished: hostCall.promise,
        close,
        events: (async function* () {
          terminalSent.resolve();
          yield {
            connectionEpoch: 2,
            subscriptionEpoch,
            sessionId: 's',
            invocationId: 'i',
            eventId: 'done-i',
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
  await act(async () =>
    renderer.root
      .findByProps({ 'aria-label': 'Message' })
      .props.onChange({ target: { value: 'First' } }),
  );
  await act(async () => {
    renderer.root.findByType('form').props.onSubmit({ preventDefault() {} });
    await Promise.resolve();
  });
  await act(async () =>
    renderer.root
      .findByProps({ 'aria-label': 'Message' })
      .props.onChange({ target: { value: 'Next' } }),
  );
  expect(close).toHaveBeenCalled();
  expect(sendButton(renderer).props.disabled).toBe(false);
  expect(
    renderer.root.findAllByProps({ 'aria-label': 'Stop current session' }),
  ).toHaveLength(0);
  await act(async () => renderer.unmount());
  hostCall.resolve();
});
it('releases the next turn after a terminal error while preserving the failure message', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  const hostCall = deferred<void>();
  const read = vi.fn(async () => ({
    session: { id: 's', title: 'Chat' },
    messages: [],
  }));
  const service = {
    sessions: {
      pending: async () => [],
      read,
      submit: vi.fn(async () => ({ invocationId: 'i', status: 'accepted' })),
      subscribe: vi.fn((_epoch, _id, _invocation, subscriptionEpoch) => ({
        finished: hostCall.promise,
        close() {},
        events: (async function* () {
          yield {
            connectionEpoch: 2,
            subscriptionEpoch,
            sessionId: 's',
            invocationId: 'i',
            eventId: 'failed-i',
            kind: 'error',
            text: 'The model request failed.',
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
  await act(async () =>
    renderer.root
      .findByProps({ 'aria-label': 'Message' })
      .props.onChange({ target: { value: 'First' } }),
  );
  await act(async () => {
    renderer.root.findByType('form').props.onSubmit({ preventDefault() {} });
    await Promise.resolve();
  });
  await act(async () =>
    renderer.root
      .findByProps({ 'aria-label': 'Message' })
      .props.onChange({ target: { value: 'Next' } }),
  );
  expect(sendButton(renderer).props.disabled).toBe(false);
  expect(JSON.stringify(renderer.toJSON())).toContain(
    'The model request failed.',
  );
  await act(async () => renderer.unmount());
  hostCall.resolve();
});
it('clears the composer and renders the user message before slow submit returns', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  const submit = deferred<{ invocationId: string; status: string }>();
  const service = {
    sessions: {
      pending: async () => [],
      read: vi.fn(async () => ({
        session: { id: 's', title: 'Chat' },
        messages: [],
      })),
      submit: vi.fn(() => submit.promise),
      subscribe: vi.fn((_epoch, _id, _invocation, subscriptionEpoch) => ({
        finished: Promise.resolve(),
        close() {},
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
    renderer.root
      .findByProps({ 'aria-label': 'Message' })
      .props.onChange({ target: { value: 'Hello while slow' } });
  });
  await act(async () => {
    renderer.root.findByType('form').props.onSubmit({ preventDefault() {} });
    await Promise.resolve();
  });
  expect(
    renderer.root.findByProps({ 'aria-label': 'Message' }).props.value,
  ).toBe('');
  expect(JSON.stringify(renderer.toJSON())).toContain('Hello while slow');
  expect(sendButton(renderer).props.disabled).toBe(true);
  await act(async () =>
    submit.resolve({ invocationId: 'i', status: 'accepted' }),
  );
  await act(async () => renderer.unmount());
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
    activeRun: {
      operationId: 'op',
      invocationId: 'i',
      state: 'accepted',
      authority: 'localReceipt',
      recoveryDisposition: 'subscribeAccepted',
    },
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
it('unlocks the composer after abort when authoritative history has already completed', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  const read = vi.fn(async () => ({
    session: { id: 's', title: 'Chat', status: 'running' },
    messages: [],
    activeRun: {
      operationId: 'op',
      invocationId: 'i',
      state: 'accepted',
      authority: 'localReceipt',
      recoveryDisposition: 'subscribeAccepted',
    },
    completedInvocations: ['i'],
  }));
  const service = {
    sessions: {
      read,
      pending: async () => [],
      abort: vi.fn(async () => true),
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
  await act(async () => stopButton(renderer).props.onClick());
  await act(async () =>
    renderer.root
      .findByProps({ 'aria-label': 'Message' })
      .props.onChange({ target: { value: 'Next' } }),
  );
  expect(sendButton(renderer).props.disabled).toBe(false);
  expect(JSON.stringify(renderer.toJSON())).not.toContain('verifying outcome');
  await act(async () => renderer.unmount());
});
it('treats an abort race as completed when the turn ended before abort acknowledgement', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  const read = vi
    .fn()
    .mockResolvedValueOnce({
      session: { id: 's', title: 'Chat', status: 'running' },
      messages: [],
      activeRun: {
        operationId: 'op',
        invocationId: 'i',
        state: 'accepted',
        authority: 'runtime',
        recoveryDisposition: 'subscribeAccepted',
      },
    })
    .mockResolvedValue({
      session: { id: 's', title: 'Chat', status: 'completed' },
      messages: [],
      completedInvocations: ['i'],
    });
  const service = {
    sessions: {
      read,
      pending: async () => [],
      abort: vi.fn(async () => false),
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
  await act(async () => stopButton(renderer).props.onClick());
  await act(async () =>
    renderer.root
      .findByProps({ 'aria-label': 'Message' })
      .props.onChange({ target: { value: 'Next' } }),
  );
  expect(sendButton(renderer).props.disabled).toBe(false);
  expect(JSON.stringify(renderer.toJSON())).not.toContain(
    'did not acknowledge an abort',
  );
  await act(async () => renderer.unmount());
});
it('waits for the terminal history after abort acknowledgement before unlocking', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  const firstStopRead = deferred<void>();
  let stopReads = 0;
  const active = {
    session: { id: 's', title: 'Chat', status: 'running' },
    messages: [],
    activeRun: {
      operationId: 'op',
      invocationId: 'i',
      state: 'accepted',
      authority: 'runtime',
      recoveryDisposition: 'subscribeAccepted',
    },
  };
  const read = vi.fn(async () => {
    if (read.mock.calls.length === 1) return active;
    stopReads++;
    if (stopReads === 1) {
      firstStopRead.resolve();
      return active;
    }
    return {
      session: { id: 's', title: 'Chat', status: 'completed' },
      messages: [],
      completedInvocations: ['i'],
    };
  });
  const service = {
    sessions: {
      read,
      pending: async () => [],
      abort: vi.fn(async () => true),
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
    stopButton(renderer).props.onClick();
    await firstStopRead.promise;
  });
  expect(JSON.stringify(renderer.toJSON())).toContain('verifying outcome');
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 180));
  });
  await act(async () =>
    renderer.root
      .findByProps({ 'aria-label': 'Message' })
      .props.onChange({ target: { value: 'Next' } }),
  );
  expect(sendButton(renderer).props.disabled).toBe(false);
  await act(async () => renderer.unmount());
});
