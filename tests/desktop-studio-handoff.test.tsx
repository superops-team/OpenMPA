import { StrictMode } from 'react';
import { act, create, type ReactTestRenderer } from 'react-test-renderer';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { useDesktopAppState } from '../src/client/desktop/DesktopAppState';
import { useDesktopChatState } from '../src/client/desktop/ChatState';
import type {
  DesktopServices,
  ConnectionVM,
  PendingRunVM,
} from '../src/client/desktop/models';
import type { ComposerDraft } from '../src/client/desktop/studio-models';

const connection = {
  epoch: 2,
  scope: 'scope',
  runtimeId: 'r-current',
  status: 'connected',
};
const sessions = [
  { id: 'first', title: 'First' },
  { id: 'second', title: 'Second' },
];
const renderers: ReactTestRenderer[] = [];
beforeEach(() => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  vi.stubGlobal('window', { confirm: vi.fn(() => true) });
});
afterEach(async () => {
  await act(async () => {
    for (const renderer of renderers.splice(0)) renderer.unmount();
  });
  vi.unstubAllGlobals();
});
function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<T>((done, fail) => {
    resolve = done;
    reject = fail;
  });
  return { promise, resolve, reject };
}
async function mount(overrides: Partial<DesktopServices['sessions']> = {}) {
  const receipt = deferred<{ invocationId: string; status: string }>();
  const submit = vi.fn(() => receipt.promise);
  let navigate!: (action: string, id?: string) => void;
  const subscribe = vi.fn();
  const services = {
    host: {
      onNavigation: (listener: typeof navigate) => {
        navigate = listener;
        return () => {};
      },
      onClose: () => () => {},
      closeState: async () => ({ pending: false, requestId: 0 }),
    },
    workspace: {},
    connection: { status: async () => connection },
    sessions: {
      list: async () => sessions,
      read: async (_epoch: number, id: string) => ({
        session: sessions.find((item) => item.id === id)!,
        messages: [],
      }),
      pending: async () => [],
      submit,
      subscribe,
      ...overrides,
    },
  } as unknown as DesktopServices;
  let state!: ReturnType<typeof useDesktopAppState>;
  let chat!: ReturnType<typeof useDesktopChatState>;
  function Chat({ composer }: { composer: ComposerDraft }) {
    chat = useDesktopChatState({
      services,
      connection: state.connection!,
      session: state.session!,
      composerDraft: composer,
      onSchedule: () => {},
    });
    return null;
  }
  function App() {
    state = useDesktopAppState({ services });
    return state.view === 'chat' && state.session && state.composerDraft ? (
      <Chat
        key={`${state.connection?.epoch}:${state.connection?.scope}:${state.session.id}`}
        composer={state.composerDraft}
      />
    ) : null;
  }
  let renderer!: ReactTestRenderer;
  await act(async () => {
    renderer = create(
      <StrictMode>
        <App />
      </StrictMode>,
    );
  });
  renderers.push(renderer);
  await act(async () => navigate('selectSession', 'first'));
  return {
    state: () => state,
    chat: () => chat,
    receipt,
    submit,
    subscribe,
    navigate,
  };
}

it('accepts the exact submitted draft after Chat unmount while retaining later choices', async () => {
  const app = await mount();
  await act(async () => app.chat().setDraft('submitted'));
  await act(async () => {
    void app.chat().send();
  });
  expect(app.submit).toHaveBeenCalledWith(
    2,
    'first',
    'submitted',
    undefined,
    'first',
  );
  expect(app.chat().draft).toBe('');
  await act(async () => {
    app.state().navigate('resources');
  });
  await act(async () =>
    expect(
      app.state().useInConversation(app.state().conversationTarget!, '$later'),
    ).toBe(true),
  );
  await act(async () =>
    expect(
      app
        .state()
        .useInConversation(app.state().conversationTarget!, '@alice task'),
    ).toBe(true),
  );
  await act(async () => {
    app.state().navigate('collaboration');
  });
  await act(async () =>
    app.receipt.resolve({ invocationId: 'accepted', status: 'accepted' }),
  );
  expect(app.state().composerDraft?.value).toBe('@alice task\n$later');
  expect(app.subscribe).not.toHaveBeenCalled();
});

it('retains newer manual edits on a late receipt and isolates other sessions', async () => {
  const app = await mount();
  await act(async () => app.chat().setDraft('submitted'));
  await act(async () => {
    void app.chat().send();
  });
  await act(async () => {
    app.state().navigate('resources');
  });
  await act(async () => app.state().composerDraft!.setValue('manual edit'));
  await act(async () => app.navigate('selectSession', 'second'));
  await act(async () => app.chat().setDraft('second draft'));
  await act(async () =>
    app.receipt.resolve({ invocationId: 'accepted', status: 'accepted' }),
  );
  expect(app.state().composerDraft?.value).toBe('second draft');
  await act(async () => app.navigate('selectSession', 'first'));
  expect(app.state().composerDraft?.value).toBe('manual edit');
});

it('restores a rejected shared draft after unmount when no submission is pending', async () => {
  const app = await mount();
  await act(async () => app.chat().setDraft('rejected prompt'));
  await act(async () => {
    void app.chat().send();
  });
  expect(app.chat().draft).toBe('');
  await act(async () => {
    app.state().navigate('resources');
  });
  await act(async () => app.receipt.reject(new Error('rejected')));
  expect(app.state().composerDraft?.value).toBe('rejected prompt');
});

it('clears before loading material and prevents repeated synchronous submissions', async () => {
  const app = await mount();
  await act(async () => app.chat().setDraft('one submission'));
  await act(async () => {
    const send = app.chat().send;
    void send();
    void send();
  });
  expect(app.submit).toHaveBeenCalledTimes(1);
  expect(app.chat().draft).toBe('');
});

it.each([
  { epoch: 3, scope: 'scope', label: 'epoch' },
  { epoch: 2, scope: 'new scope', label: 'scope' },
])(
  'rejects old $label receipts and resource choices',
  async ({ epoch, scope }) => {
    const app = await mount();
    await act(async () => app.chat().setDraft('submitted'));
    const oldTarget = app.state().conversationTarget!;
    await act(async () => {
      void app.chat().send();
    });
    await act(async () => app.state().connect({ ...connection, epoch, scope }));
    await act(async () => app.navigate('selectSession', 'first'));
    await act(async () => app.chat().setDraft('new private draft'));
    await act(async () =>
      app.receipt.resolve({ invocationId: 'accepted', status: 'accepted' }),
    );
    expect(app.state().composerDraft?.value).toBe('new private draft');
    await act(async () =>
      expect(app.state().useInConversation(oldTarget, '$stale')).toBe(false),
    );
    expect(app.state().composerDraft?.value).toBe('new private draft');
  },
);

it('does not insert or navigate when the current page refuses navigation', async () => {
  const app = await mount();
  await act(async () => app.chat().setDraft('body'));
  await act(async () => {
    app.state().navigate('resources');
  });
  app.state().setDirty(true);
  vi.mocked(window.confirm).mockReturnValue(false);
  await act(async () =>
    expect(
      app.state().useInConversation(app.state().conversationTarget!, '$skill'),
    ).toBe(false),
  );
  expect(app.state().view).toBe('resources');
  expect(app.state().composerDraft?.value).toBe('body');
  expect(app.submit).not.toHaveBeenCalled();
});

it('seeds recovery only when draft is undefined and keeps deliberate empty after remount', async () => {
  const operation = {
    content: 'pending text',
    operationId: 'op',
  } as PendingRunVM;
  const app = await mount({ pending: async () => [operation] });
  expect(app.chat().draft).toBe('pending text');
  await act(async () => app.chat().setDraft(''));
  await act(async () => {
    app.state().navigate('resources');
  });
  await act(async () => {
    app.state().navigate('chat');
  });
  expect(app.chat().draft).toBe('');
});

it('fences two out-of-order connection status notifications', async () => {
  let state!: ReturnType<typeof useDesktopAppState>;
  let navigation!: (action: string) => void;
  const first = deferred<ConnectionVM>(),
    second = deferred<ConnectionVM>();
  const status = vi
    .fn()
    .mockResolvedValueOnce(connection)
    .mockImplementationOnce(() => first.promise)
    .mockImplementationOnce(() => second.promise);
  const services = {
    connection: { status },
    sessions: { list: async () => [] },
    workspace: {},
    host: {
      onNavigation: (listener: typeof navigation) => {
        navigation = listener;
        return () => {};
      },
      onClose: () => () => {},
      closeState: async () => ({ pending: false, requestId: 0 }),
    },
  } as unknown as DesktopServices;
  function App() {
    state = useDesktopAppState({ services });
    return null;
  }
  let renderer!: ReactTestRenderer;
  await act(async () => {
    renderer = create(<App />);
  });
  renderers.push(renderer);
  await act(async () => {
    navigation('connectionChanged');
    navigation('connectionChanged');
  });
  await act(async () => second.resolve({ ...connection, epoch: 4 }));
  await act(async () => first.resolve({ ...connection, epoch: 3 }));
  expect(state.connection?.epoch).toBe(4);
});
