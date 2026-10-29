import { act, create, type ReactTestRenderer } from 'react-test-renderer';
import { afterEach, expect, it, vi } from 'vitest';
import { DesktopApp } from '../src/client/desktop/DesktopApp';
import { DesktopBotSettings } from '../src/client/desktop/BotSettings';
import type {
  DesktopServices,
  ExecutionConfigVM,
  SessionVM,
} from '../src/client/desktop/models';

afterEach(() => vi.unstubAllGlobals());

async function setup() {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  const sessions = new Map<string, SessionVM>([
    ['first', { id: 'first', title: 'First' }],
    ['second', { id: 'second', title: 'Second' }],
  ]);
  let navigation!: (action: string, id?: string) => void;
  let config: ExecutionConfigVM = {
    modelMode: 'replace',
    model: 'model-one',
    executionConfigVersion: '1',
    etag: '1',
    mcpMode: 'inherit',
    mcpServers: [],
    profileBound: true,
    invalidRefsCount: 0,
  };
  const service = {
    host: {
      onNavigation: (handler: typeof navigation) => {
        navigation = handler;
        return () => {};
      },
      onClose: () => () => {},
      closeState: async () => ({ pending: false }),
    },
    connection: {
      status: async () => ({ epoch: 1, scope: 'test', status: 'connected' }),
    },
    sessions: {
      list: async () => [...sessions.values()],
      read: vi.fn(async (_epoch, id) => ({
        session: sessions.get(id),
        messages: [],
      })),
      pending: async () => [],
      executionConfig: vi.fn(async () => config),
      assistantConfig: async () => ({ agentsMd: '', skills: [] }),
      skillCatalog: async () => [],
      modelCatalog: vi.fn(async () => [
        { id: 'model-one' },
        { id: 'model-two' },
      ]),
      create: vi.fn(async (_epoch, title, _instance, id) => {
        const item = { id, title };
        sessions.set(id, item);
        return item;
      }),
      saveAssistantConfig: vi.fn(async () => ({})),
      updateExecutionConfig: vi.fn(
        async (_epoch, _id, patch) =>
          (config = {
            ...config,
            ...patch,
            etag: '2',
            executionConfigVersion: '2',
          }),
      ),
      rename: vi.fn(async () => {}),
      remove: vi.fn(async (_epoch, id) => {
        sessions.delete(id);
      }),
    },
  } as unknown as DesktopServices;
  let renderer!: ReactTestRenderer;
  await act(async () => {
    renderer = create(<DesktopApp services={service} />);
  });
  return {
    renderer,
    service,
    sessions,
    navigate: (action: string, id?: string) =>
      act(async () => navigation(action, id)),
  };
}

it('keeps a failed creation draft open and retries the same remote assistant', async () => {
  const { renderer, service, sessions, navigate } = await setup();
  vi.mocked(service.sessions.saveAssistantConfig).mockRejectedValueOnce(
    new Error('Local save failed.'),
  );
  await navigate('newConversation');
  await act(async () =>
    renderer.root
      .findByProps({ 'aria-label': 'Assistant name' })
      .props.onChange({ target: { value: 'New helper' } }),
  );
  const submit = () =>
    act(async () =>
      renderer.root
        .findByProps({ className: 'desktop-form assistant-settings' })
        .props.onSubmit({ preventDefault() {} }),
    );
  await submit();
  expect(renderer.root.findAllByType(DesktopBotSettings)).toHaveLength(1);
  expect(
    renderer.root.findByProps({ 'aria-label': 'Assistant name' }).props.value,
  ).toBe('New helper');
  expect(JSON.stringify(renderer.toJSON())).toContain('Local save failed.');
  await submit();
  const calls = vi.mocked(service.sessions.create).mock.calls;
  expect(calls).toHaveLength(2);
  expect(calls[0][3]).toMatch(/^[0-9a-f-]{36}$/);
  expect(calls[1][3]).toBe(calls[0][3]);
  expect(sessions.size).toBe(3);
  expect(renderer.root.findAllByType(DesktopBotSettings)).toHaveLength(0);
  await act(async () => renderer.unmount());
});

it('creates an assistant with the explicitly selected model and shows it in the composer', async () => {
  const { renderer, service, navigate } = await setup();
  await navigate('newConversation');
  const editor = renderer.root.findByType(DesktopBotSettings);
  await act(async () =>
    editor
      .findByProps({ 'aria-label': 'Assistant name' })
      .props.onChange({ target: { value: 'Selected model' } }),
  );
  await act(async () =>
    editor.findByProps({ className: 'composer-model-trigger' }).props.onClick(),
  );
  const option = editor
    .findAllByProps({ role: 'radio' })
    .find(
      (node) => node.findByType('strong').children.join('') === 'model-two',
    )!;
  await act(async () => option.props.onClick());
  expect(editor.findByProps({ 'aria-label': 'Model: model-two' })).toBeTruthy();
  await act(async () =>
    editor.findByType('form').props.onSubmit({ preventDefault() {} }),
  );
  expect(service.sessions.updateExecutionConfig).toHaveBeenCalledWith(
    1,
    expect.any(String),
    expect.objectContaining({ modelMode: 'replace', model: 'model-two' }),
  );
  expect(
    renderer.root.findByProps({ 'aria-label': 'Model: model-two' }),
  ).toBeTruthy();
  await act(async () => renderer.unmount());
});

it('keeps unrelated model-update 404 failures distinct from diagnostic history', async () => {
  const { renderer, service } = await setup();
  vi.mocked(service.sessions.updateExecutionConfig).mockRejectedValueOnce(
    Object.assign(new Error('Model configuration was not found.'), {
      status: 404,
    }),
  );
  await act(async () =>
    renderer.root
      .findByProps({ className: 'composer-model-trigger' })
      .props.onClick(),
  );
  const option = renderer.root
    .findAllByProps({ role: 'radio' })
    .find(
      (node) => node.findByType('strong').children.join('') === 'model-two',
    )!;
  await act(async () => option.props.onClick());
  expect(JSON.stringify(renderer.toJSON())).toContain(
    'Model configuration was not found.',
  );
  expect(JSON.stringify(renderer.toJSON())).not.toContain(
    'legacy diagnostic conversation',
  );
  await act(async () => renderer.unmount());
});

it('keeps instructions editable when Runtime confirms diagnostic history cannot change models', async () => {
  const { renderer, service, navigate } = await setup();
  vi.mocked(service.sessions.executionConfig).mockRejectedValue(
    Object.assign(new Error('Diagnostic model unsupported'), {
      code: 'diagnosticModelUnsupported',
      status: 404,
    }),
  );
  await navigate('editSession', 'first');
  const editor = renderer.root.findByType(DesktopBotSettings);
  expect(
    editor.findByProps({ className: 'composer-model-trigger' }).props.disabled,
  ).toBe(true);
  expect(editor.findByType('fieldset').props.disabled).toBe(false);
  await act(async () =>
    editor
      .findByType('textarea')
      .props.onChange({ target: { value: 'Saved instructions' } }),
  );
  await act(async () =>
    editor.findByType('form').props.onSubmit({ preventDefault() {} }),
  );
  expect(service.sessions.updateExecutionConfig).not.toHaveBeenCalled();
  expect(service.sessions.saveAssistantConfig).toHaveBeenCalledWith(
    1,
    'first',
    'first',
    expect.objectContaining({ agentsMd: 'Saved instructions' }),
  );
  await act(async () => renderer.unmount());
});

it('keeps the chat draft and history mounted when assistant settings change the model', async () => {
  const { renderer, service, navigate } = await setup();
  await act(async () =>
    renderer.root
      .findByProps({ 'aria-label': 'Message' })
      .props.onChange({ target: { value: 'Unsent draft' } }),
  );
  await navigate('editSession', 'first');
  // Reopening the same assistant from the native menu still yields one editor.
  await navigate('editSession', 'first');
  expect(renderer.root.findAllByType(DesktopBotSettings)).toHaveLength(1);
  const editor = renderer.root.findByType(DesktopBotSettings);
  await act(async () =>
    editor.findByProps({ className: 'composer-model-trigger' }).props.onClick(),
  );
  const option = editor
    .findAllByProps({ role: 'radio' })
    .find(
      (node) => node.findByType('strong').children.join('') === 'model-two',
    )!;
  await act(async () => option.props.onClick());
  await act(async () =>
    editor.findByType('form').props.onSubmit({ preventDefault() {} }),
  );
  expect(
    renderer.root.findByProps({ 'aria-label': 'Message' }).props.value,
  ).toBe('Unsent draft');
  expect(
    renderer.root.findByProps({ 'aria-label': 'Model: model-two' }),
  ).toBeTruthy();
  expect(service.sessions.read).toHaveBeenCalledTimes(1);
  expect(service.sessions.modelCatalog).toHaveBeenCalledTimes(1);
  await act(async () => renderer.unmount());
});

it('deletes the clicked assistant once and preserves a selection made while deletion is pending', async () => {
  const { renderer, service, sessions, navigate } = await setup();
  vi.stubGlobal('window', { confirm: vi.fn(() => true) });
  let finish!: () => void;
  vi.mocked(service.sessions.remove).mockImplementationOnce(
    async (_epoch, id) => {
      await new Promise<void>((resolve) => {
        finish = resolve;
      });
      sessions.delete(id);
    },
  );
  await navigate('deleteSession', 'second');
  await navigate('deleteSession', 'second');
  expect(service.sessions.remove).toHaveBeenCalledTimes(1);
  expect(service.sessions.remove).toHaveBeenCalledWith(1, 'second');
  await navigate('selectSession', 'first');
  await act(async () => finish());
  expect(renderer.root.findByProps({ title: 'First' })).toBeTruthy();
  expect(sessions.has('first')).toBe(true);
  await act(async () => renderer.unmount());
});
