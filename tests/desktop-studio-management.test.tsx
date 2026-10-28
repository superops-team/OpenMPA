import { act, create, type ReactTestRenderer } from 'react-test-renderer';
import { beforeEach, afterEach, it, expect, vi } from 'vitest';
import { CollaborationManagement } from '../src/client/desktop/CollaborationManagement';
import { DesktopResources } from '../src/client/desktop/Resources';
import type {
  ResourcesServices,
  CollaborationServices,
} from '../src/client/desktop/studio-models';
import { messagesIn } from '../src/client/desktop/generated/locales';
let renderer: ReactTestRenderer | undefined;
const m = messagesIn('en');
beforeEach(() => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  vi.stubGlobal('mygo', { locale: 'en' });
  vi.stubGlobal('window', { confirm: () => true });
});
afterEach(async () => {
  if (renderer) await act(async () => renderer!.unmount());
  renderer = undefined;
  vi.unstubAllGlobals();
});
const button = (text: string) =>
  renderer!.root
    .findAllByType('button')
    .find((b) => b.children.includes(text))!;
async function mount(
  services: CollaborationServices,
  onChanged = vi.fn(async () => {}),
) {
  await act(async () => {
    renderer = create(
      <CollaborationManagement
        services={services}
        epoch={3}
        spaceId="as-team"
        projectName="default"
        boundSpaceId={null}
        membersTotal={0}
        disabled={false}
        onBusy={() => {}}
        onUncertain={() => {}}
        onChanged={onChanged}
      />,
    );
  });
  return onChanged;
}
it('creates a space once and refreshes the directory with the created selection', async () => {
  const createSpace = vi.fn(async () => ({ id: 'as-new' }));
  const onChanged = await mount({
    createSpace,
  } as unknown as CollaborationServices);
  await act(async () => button(m.a2aCreateSpace()).props.onClick());
  const fields = renderer!.root.findAllByType('input');
  await act(async () =>
    fields[0].props.onChange({ target: { value: 'New Team' } }),
  );
  await act(async () =>
    renderer!.root.findByType('form').props.onSubmit({ preventDefault() {} }),
  );
  expect(createSpace).toHaveBeenCalledExactlyOnceWith(
    3,
    'New Team',
    'default',
    false,
  );
  expect(onChanged).toHaveBeenCalledWith('as-new');
  expect(renderer!.root.findAllByType('form')).toHaveLength(0);
});
it('keeps an uncertain registration visible without resubmitting', async () => {
  const registerAgent = vi.fn(async () => {
    throw { code: 'outcomeUnknown' };
  });
  await mount({ registerAgent } as unknown as CollaborationServices);
  await act(async () => button(m.a2aRegisterAgent()).props.onClick());
  const source = renderer!.root.findAllByType('select')[0];
  await act(async () => source.props.onChange({ target: { value: 'json' } }));
  await act(async () =>
    renderer!.root.findByType('textarea').props.onChange({
      target: {
        value: '{"name":"a","url":"https://example.invalid","version":"1"}',
      },
    }),
  );
  await act(async () =>
    renderer!.root.findByType('form').props.onSubmit({ preventDefault() {} }),
  );
  expect(registerAgent).toHaveBeenCalledOnce();
  expect(button(m.a2aSubmit()).props.disabled).toBe(true);
  expect(JSON.stringify(renderer!.toJSON())).toContain(
    m.collaborationRefreshRequired(),
  );
});
it('loads Worker MCP with no selected conversation and never reads Main configuration', async () => {
  const listWorkerMCPServers = vi.fn(async () => ({
    servers: [
      {
        id: 'agentkit:worker-search',
        name: 'Worker Search',
        alias: '',
        description: 'external',
        tools: null,
        toolsKnown: false,
      },
    ],
  }));
  const readMainProfile = vi.fn();
  const services = {
    discoverSkillSpaces: async () => [],
    listWorkerMCPServers,
    readMainProfile,
  } as unknown as ResourcesServices;
  await act(async () => {
    renderer = create(
      <DesktopResources
        services={services}
        connection={{
          epoch: 3,
          runtimeId: 'r-team',
          scope: 'scope',
          status: 'ready',
        }}
        onUse={() => false}
        onConfigureMCP={() => {}}
        hidden={false}
      />,
    );
  });
  await act(async () =>
    renderer!.root
      .findByProps({ 'data-testid': 'resources-tab-mcp' })
      .props.onClick(),
  );
  expect(listWorkerMCPServers).toHaveBeenCalledWith(3);
  expect(JSON.stringify(renderer!.toJSON())).toContain('Worker Search');
  expect(JSON.stringify(renderer!.toJSON())).not.toContain(
    m.selectConversationMcp(),
  );
  expect(readMainProfile).not.toHaveBeenCalled();
});
