import { StrictMode } from 'react';
import { act, create, type ReactTestRenderer } from 'react-test-renderer';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { DesktopResources } from '../src/client/desktop/Resources';
import { DesktopCollaboration } from '../src/client/desktop/Collaboration';
import { ComposerDraftStore } from '../src/client/desktop/composer-drafts';
import { studioFailure } from '../src/client/desktop/studio-services';
import { LocaleProvider, renderMessage } from '../src/client/desktop/locale';
import { messagesIn } from '../src/client/desktop/generated/locales';
import type {
  ResourcesServices,
  CollaborationServices,
  MainProfileOutcome,
} from '../src/client/desktop/studio-models';

const connection = {
  epoch: 2,
  scope: 'scope',
  runtimeId: 'r-current',
  status: 'connected',
};
const target = { connectionEpoch: 2, scope: 'scope', sessionId: 'first' };
const renderers: ReactTestRenderer[] = [];
beforeEach(() => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  vi.stubGlobal('window', { confirm: vi.fn(() => true) });
  vi.stubGlobal('mygo', { locale: 'en', on: () => () => {} });
});
afterEach(async () => {
  await act(async () => {
    for (const renderer of renderers.splice(0)) renderer.unmount();
  });
  vi.unstubAllGlobals();
});
function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<T>((done, fail) => {
    resolve = done;
    reject = fail;
  });
  return { promise, resolve, reject };
}
async function mount(node: React.ReactNode) {
  let renderer!: ReactTestRenderer;
  await act(async () => {
    renderer = create(
      <StrictMode>
        <LocaleProvider>{node}</LocaleProvider>
      </StrictMode>,
    );
  });
  renderers.push(renderer);
  return renderer;
}
const button = (renderer: ReactTestRenderer, text: string) =>
  renderer.root
    .findAllByType('button')
    .find(
      (item) =>
        item.children.filter((child) => typeof child === 'string').join('') ===
          text || item.props['aria-label'] === text,
    )!;
const input = (renderer: ReactTestRenderer, id: string) =>
  renderer.root.findByProps({ 'data-testid': id });
const text = (renderer: ReactTestRenderer) => JSON.stringify(renderer.toJSON());
const en = messagesIn('en');
function resourceServices(
  overrides: Partial<ResourcesServices> = {},
): ResourcesServices {
  return {
    discoverSkillSpaces: vi.fn(async () => [{ id: 'skills', name: 'Skills' }]),
    listSkills: vi.fn(async () => ({
      spaceId: 'skills',
      region: 'cn-beijing',
      items: [
        { name: 'safe-skill', description: '<img src=x onerror=alert(1)>' },
      ],
      totalCount: 1,
      pageNumber: 1,
      pageSize: 20,
      hasMore: false,
      connectionEpoch: 2,
      platformEpoch: 1,
    })),
    listWorkerMCPServers: vi.fn(async () => ({
      servers: [],
    })),
    readAgentsMD: vi.fn(async () => ({ content: 'original', revision: 1 })),
    saveAgentsMD: vi.fn(async (_epoch, document) => ({
      ...document,
      revision: 2,
    })),
    readMainProfile: vi.fn(async (): Promise<MainProfileOutcome> => ({
      status: 'configured',
      value: {
        status: 'configured',
        connectionEpoch: 2,
        snapshotId: 'token',
        record: {
          mpaInstanceId: 'mpa-current',
          revision: 1,
          digest: 'digest',
          etag: '"1"',
          profile: {
            agentsMd: 'original',
            model: { id: 'pinned-model' },
            skills: [],
          },
          sourceProfileId: null,
          sourceProfileDigest: null,
          updatedBy: '',
          createdAt: '',
        },
      },
    })),
    saveMainProfileInstructions: vi.fn(
      async (): Promise<MainProfileOutcome> => ({
        status: 'unavailable',
        error: { code: 'conflict', message: 'opaque' },
      }),
    ),
    ...overrides,
  };
}
function collaborationServices(
  overrides: Partial<CollaborationServices> = {},
): CollaborationServices {
  return {
    binding: vi.fn(async () => ({
      spaceId: 'space',
      spaceName: 'Space',
      region: 'cn-beijing',
      revision: 1,
    })),
    spaces: vi.fn(async () => ({
      items: [
        {
          id: 'space',
          name: 'Space',
          isDefault: false,
          intentEnabled: true,
          projectName: 'default',
        },
      ],
      page: 1,
      pageSize: 20,
      total: 1,
    })),
    agents: vi.fn(async () => ({
      items: [
        {
          id: 'a',
          name: 'A',
          description: '<script>not HTML</script>',
          status: 'running',
          alias: 'alice',
          aliasRevision: 2,
          aliasAvailable: true,
          runtimeId: 'r-peer',
        },
        {
          id: 'b',
          name: 'B',
          description: '',
          status: 'running',
          alias: 'bob',
          aliasRevision: 3,
          aliasAvailable: true,
          runtimeId: 'r-peer-2',
        },
      ],
      page: 1,
      pageSize: 20,
      total: 2,
    })),
    setBinding: vi.fn(async (_epoch, spaceId) => ({
      spaceId,
      spaceName: null,
      region: 'cn-beijing',
      revision: 2,
    })),
    setAlias: vi.fn(async (_epoch, _space, _agent, alias) => ({
      alias,
      revision: 3,
    })),
    mention: vi.fn(async () => '@alice review this'),
    ...overrides,
  };
}

describe('Studio resources', () => {
  it('hides retained pages explicitly while keeping instruction drafts mounted', async () => {
    const services = resourceServices();
    const props = {
      services,
      connection,
      onUse: () => true,
      onConfigureMCP: () => {},
    };
    const renderer = await mount(
      <DesktopResources {...props} hidden={false} />,
    );
    await act(async () =>
      input(renderer, 'resources-agentsMd-draft').props.onChange({
        target: { value: 'retained' },
      }),
    );
    await act(async () =>
      renderer.update(
        <StrictMode>
          <LocaleProvider>
            <DesktopResources {...props} hidden />
          </LocaleProvider>
        </StrictMode>,
      ),
    );
    expect(input(renderer, 'studio-resources').props.style).toEqual({
      display: 'none',
    });
    expect(input(renderer, 'resources-agentsMd-draft').props.value).toBe(
      'retained',
    );
  });
  it('loads fast Skill responses in StrictMode and inserts text only', async () => {
    const services = resourceServices(),
      onUse = vi.fn(() => true);
    const renderer = await mount(
      <DesktopResources
        services={services}
        connection={connection}
        target={target}
        onUse={onUse}
        onConfigureMCP={() => {}}
        hidden={false}
      />,
    );
    await act(async () => button(renderer, en.refreshSkills()).props.onClick());
    expect(services.listSkills).toHaveBeenCalledWith(
      2,
      'cn-beijing',
      'skills',
      1,
      20,
    );
    expect(button(renderer, en.refreshSkills()).props.disabled).toBe(false);
    await act(async () =>
      button(renderer, en.useInConversation()).props.onClick(),
    );
    expect(onUse).toHaveBeenCalledWith(target, '$safe-skill');
    expect(renderer.root.findAllByType('img')).toHaveLength(0);
  });
  it('retains instruction edits across conflict and requires a fresh read', async () => {
    const services = resourceServices();
    const renderer = await mount(
      <DesktopResources
        services={services}
        connection={connection}
        onUse={() => true}
        onConfigureMCP={() => {}}
        hidden={false}
      />,
    );
    await act(async () =>
      button(renderer, en.reloadMainProfile()).props.onClick(),
    );
    await act(async () =>
      input(renderer, 'resources-mainProfile-draft').props.onChange({
        target: { value: 'my edit' },
      }),
    );
    await act(async () =>
      button(renderer, en.saveMainProfile()).props.onClick(),
    );
    expect(services.saveMainProfileInstructions).toHaveBeenCalledWith(
      2,
      'token',
      'my edit',
    );
    expect(input(renderer, 'resources-mainProfile-draft').props.value).toBe(
      'my edit',
    );
    expect(button(renderer, en.saveMainProfile()).props.disabled).toBe(true);
    expect(text(renderer)).toContain(en.conflict());
    await act(async () =>
      button(renderer, en.reloadMainProfile()).props.onClick(),
    );
    expect(input(renderer, 'resources-mainProfile-draft').props.value).toBe(
      'my edit',
    );
    expect(button(renderer, en.saveMainProfile()).props.disabled).toBe(false);
  });
  it('preserves edits made while a save is pending', async () => {
    const saved = deferred<{ content: string; revision: number }>();
    const services = resourceServices({ saveAgentsMD: () => saved.promise });
    const renderer = await mount(
      <DesktopResources
        services={services}
        connection={connection}
        onUse={() => true}
        onConfigureMCP={() => {}}
        hidden={false}
      />,
    );
    await act(async () =>
      button(renderer, en.reloadAgentsMd()).props.onClick(),
    );
    await act(async () =>
      input(renderer, 'resources-agentsMd-draft').props.onChange({
        target: { value: 'submitted' },
      }),
    );
    await act(async () => button(renderer, en.saveAgentsMd()).props.onClick());
    await act(async () =>
      input(renderer, 'resources-agentsMd-draft').props.onChange({
        target: { value: 'newer edit' },
      }),
    );
    await act(async () => saved.resolve({ content: 'submitted', revision: 2 }));
    expect(input(renderer, 'resources-agentsMd-draft').props.value).toBe(
      'newer edit',
    );
  });
  it('keeps status outside failed profile value and permits no save', async () => {
    const outcome: MainProfileOutcome = {
      status: 'unsupportedOrUnconfigured',
      error: { code: 'capabilityUnavailable', message: 'opaque' },
    };
    const services = resourceServices({ readMainProfile: async () => outcome });
    const renderer = await mount(
      <DesktopResources
        services={services}
        connection={connection}
        onUse={() => true}
        onConfigureMCP={() => {}}
        hidden={false}
      />,
    );
    await act(async () =>
      button(renderer, en.reloadMainProfile()).props.onClick(),
    );
    expect(text(renderer)).toContain(en.mainProfileUnsupportedOrUnconfigured());
    expect(button(renderer, en.saveMainProfile()).props.disabled).toBe(true);
  });
  it('rejects a late Worker MCP response for an old Runtime', async () => {
    const late =
      deferred<
        Awaited<ReturnType<ResourcesServices['listWorkerMCPServers']>>
      >();
    const services = resourceServices({
      listWorkerMCPServers: () => late.promise,
    });
    const props = {
      services,
      connection,
      onUse: () => true,
      onConfigureMCP: () => {},
      hidden: false,
    };
    const renderer = await mount(
      <DesktopResources {...props} session={{ id: 'first', title: 'First' }} />,
    );
    await act(async () =>
      button(renderer, en.refreshMcpServers()).props.onClick(),
    );
    await act(async () =>
      renderer.update(
        <StrictMode>
          <LocaleProvider>
            <DesktopResources
              {...props}
              connection={{ ...connection, epoch: 3, scope: 'new-runtime' }}
              session={{ id: 'second', title: 'Second' }}
            />
          </LocaleProvider>
        </StrictMode>,
      ),
    );
    await act(async () =>
      late.resolve({
        sessionId: 'first',
        servers: [
          {
            id: 'private',
            name: 'old private server',
            alias: '',
            description: '',
            toolsKnown: false,
            tools: [],
          },
        ],
      }),
    );
    expect(text(renderer)).not.toContain('old private server');
  });
});

describe('Studio collaboration', () => {
  it.each([
    { status: 'stopped', aliasAvailable: true },
    { status: 'running', aliasAvailable: false },
  ])(
    'shows registry management without a task shortcut: %j',
    async (eligibility) => {
      const services = collaborationServices();
      const page = await services.agents(2, 'space', 1);
      services.agents = vi.fn(async () => ({
        ...page,
        items: [{ ...page.items[0], ...eligibility }],
      }));
      const renderer = await mount(
        <DesktopCollaboration
          services={services}
          connection={connection}
          hidden={false}
        />,
      );
      await act(async () =>
        button(renderer, en.refreshCollaboration()).props.onClick(),
      );
      expect(
        renderer.root.findAllByProps({ 'data-testid': 'collaboration-task' }),
      ).toHaveLength(0);
      expect(button(renderer, en.useInConversation())).toBeUndefined();
      expect(services.mention).not.toHaveBeenCalled();
    },
  );
  it('loads a coherent registry in StrictMode and retains per-member alias drafts', async () => {
    const services = collaborationServices();
    const renderer = await mount(
      <DesktopCollaboration
        services={services}
        connection={connection}
        hidden={false}
      />,
    );
    await act(async () =>
      button(renderer, en.refreshCollaboration()).props.onClick(),
    );
    expect(services.agents).toHaveBeenCalledWith(connection.epoch, 'space', 1);
    await act(async () =>
      input(renderer, 'collaboration-alias').props.onChange({
        target: { value: 'new alias draft' },
      }),
    );
    await act(async () =>
      input(renderer, 'collaboration-member-b').props.onClick(),
    );
    await act(async () =>
      input(renderer, 'collaboration-member-a').props.onClick(),
    );
    expect(input(renderer, 'collaboration-alias').props.value).toBe(
      'new alias draft',
    );
    expect(renderer.root.findAllByType('script')).toHaveLength(0);
  });
  it('can unbind a freshly read binding when the directory is unavailable', async () => {
    const services = collaborationServices({
      spaces: async () => {
        throw { code: 'denied', message: 'opaque' };
      },
    });
    const renderer = await mount(
      <DesktopCollaboration
        services={services}
        connection={connection}
        hidden={false}
      />,
    );
    await act(async () =>
      button(renderer, en.refreshCollaboration()).props.onClick(),
    );
    expect(button(renderer, en.unbindSpace()).props.disabled).toBe(false);
    expect(button(renderer, en.bindSelectedSpace())).toBeUndefined();
    await act(async () => button(renderer, en.unbindSpace()).props.onClick());
    expect(services.setBinding).toHaveBeenCalledWith(2, null, 1);
  });
  it('rejects binding changes during a refresh', async () => {
    let reads = 0;
    const services = collaborationServices({
      binding: async () => ({
        spaceId: ++reads === 1 ? 'space' : 'changed',
        spaceName: null,
        region: 'cn-beijing',
        revision: reads,
      }),
    });
    const renderer = await mount(
      <DesktopCollaboration
        services={services}
        connection={connection}
        hidden={false}
      />,
    );
    await act(async () =>
      button(renderer, en.refreshCollaboration()).props.onClick(),
    );
    expect(
      renderer.root.findAllByProps({
        'data-testid': 'collaboration-member-details',
      }),
    ).toHaveLength(0);
    expect(text(renderer)).toContain(en.collaborationRefreshRequired());
  });
  it('saves a member alias without a conversation or task shortcut', async () => {
    const services = collaborationServices();
    const renderer = await mount(
      <DesktopCollaboration
        services={services}
        connection={connection}
        hidden={false}
      />,
    );
    await act(async () =>
      input(renderer, 'collaboration-alias').props.onChange({
        target: { value: 'reviewer' },
      }),
    );
    await act(async () =>
      button(renderer, en.saveMemberAlias()).props.onClick(),
    );
    expect(services.setAlias).toHaveBeenCalledWith(
      2,
      'space',
      'a',
      'reviewer',
      2,
    );
    expect(input(renderer, 'collaboration-alias').props.value).toBe('reviewer');
    expect(button(renderer, en.useInConversation())).toBeUndefined();
    expect(services.mention).not.toHaveBeenCalled();
  });
});

describe('Composer drafts', () => {
  it('restores a rejected submission alongside later choices without overwriting edits', () => {
    const store = new ComposerDraftStore();
    store.reset(2, 'scope');
    store.set(target, '  original prompt  ');
    const original = store.snapshot(target);
    store.accept(original);
    const cleared = store.snapshot(target);
    store.insert(target, '$later');
    store.restore(original, cleared);
    expect(store.value(target)).toBe('  original prompt  $later');
    store.set(target, 'send again');
    const again = store.snapshot(target);
    store.accept(again);
    const empty = store.snapshot(target);
    store.set(target, '');
    store.restore(again, empty);
    expect(store.value(target)).toBe('');
    store.remove(target.sessionId);
    store.restore(again, empty);
    expect(store.value(target)).toBeUndefined();
  });
  it('does not let a no-op recovery seed consume later choice history', () => {
    const store = new ComposerDraftStore();
    store.reset(2, 'scope');
    store.set(target, 'submitted');
    const submitted = store.snapshot(target);
    store.insert(target, '$later');
    store.set(target, (previous) => previous ?? 'pending text');
    store.accept(submitted);
    expect(store.value(target)).toBe('$later');
  });
  it('preserves choices after the submitted snapshot and rejects old scope receipts', () => {
    const store = new ComposerDraftStore();
    store.reset(2, 'scope');
    store.set(target, 'body');
    store.insert(target, '$first');
    const submitted = store.snapshot(target);
    store.insert(target, '$later');
    store.insert(target, '@alice task');
    store.accept(submitted);
    expect(store.value(target)).toBe('@alice task\n$later');
    const stale = store.snapshot(target);
    store.reset(2, 'new-scope');
    const next = { ...target, scope: 'new-scope' };
    store.set(next, 'private draft');
    store.accept(stale);
    expect(store.value(next)).toBe('private draft');
    expect(store.insert(target, '$stale')).toBe(false);
  });
  it('keeps deliberate empty and manual edits, and isolates sessions', () => {
    const store = new ComposerDraftStore();
    store.reset(2, 'scope');
    expect(store.value(target)).toBeUndefined();
    store.set(target, '');
    expect(store.value(target)).toBe('');
    store.set(target, 'submitted');
    const submitted = store.snapshot(target);
    store.set(target, 'new manual edit');
    store.accept(submitted);
    expect(store.value(target)).toBe('new manual edit');
    expect(store.value({ ...target, sessionId: 'second' })).toBeUndefined();
    store.remove('first');
    expect(store.value(target)).toBeUndefined();
    expect(store.insert(target, '\0')).toBe(false);
  });
});

describe('Studio failure messages', () => {
  it.each([
    'authRequired',
    'businessIdentityRequired',
    'businessJWTRequired',
  ] as const)('localizes %s dynamically without provider text', (code) => {
    const message = studioFailure({ code, message: 'secret provider text' });
    expect(renderMessage(message, en)).toBe(en[code]());
    const zh = messagesIn('zh-Hans');
    expect(renderMessage(message, zh)).toBe(zh[code]());
  });
});
