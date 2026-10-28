import { act, create, type ReactTestRenderer } from 'react-test-renderer';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { DesktopResources } from '../src/client/desktop/Resources';
import { messagesIn } from '../src/client/desktop/generated/locales';
import { createStudioServices } from '../src/client/desktop/studio-services';
import type { Usage } from '../src/client/desktop/generated/mygo';
import type {
  ResourcesServices,
  SkillPageVM,
} from '../src/client/desktop/studio-models';

const m = messagesIn('en');
let renderer: ReactTestRenderer;
beforeEach(() => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  vi.stubGlobal('mygo', { locale: 'en' });
  vi.stubGlobal('window', { confirm: vi.fn(() => true) });
});
afterEach(async () => {
  if (renderer) await act(async () => renderer.unmount());
  vi.unstubAllGlobals();
});
async function mount(overrides: Partial<ResourcesServices> = {}) {
  const directory: SkillPageVM = {
    spaceId: 'bound',
    region: 'cn-beijing',
    items: [
      {
        id: 'skill-1',
        name: 'review',
        description: 'Review code',
        version: 'v1',
      },
    ],
    totalCount: 1,
    pageNumber: 1,
    pageSize: 20,
    hasMore: false,
    connectionEpoch: 5,
    platformEpoch: 1,
  };
  const services = {
    discoverSkillSpaces: vi.fn(async () => [
      { id: 'bound', name: 'Worker skills' },
    ]),
    listSkills: vi.fn(async () => directory),
    deleteSkill: vi.fn(async () => ({ deleted: true })),
    ...overrides,
  } as unknown as ResourcesServices;
  await act(async () => {
    renderer = create(
      <DesktopResources
        services={services}
        connection={{
          epoch: 5,
          scope: 'scope',
          runtimeId: 'r-test',
          status: 'ready',
        }}
        onUse={() => false}
        onConfigureMCP={() => {}}
        hidden={false}
      />,
    );
  });
  return services;
}
const deleteButton = () =>
  renderer.root.findByProps({ 'aria-label': m.deleteBoundSkill('review') });

it('confirms whole-skill deletion with its name and refreshes the bound space', async () => {
  const services = await mount();
  vi.mocked(services.listSkills).mockResolvedValueOnce({
    spaceId: 'bound',
    region: 'cn-beijing',
    items: [],
    totalCount: 0,
    pageNumber: 1,
    pageSize: 20,
    hasMore: false,
    connectionEpoch: 5,
    platformEpoch: 1,
  });
  await act(async () => deleteButton().props.onClick());
  expect(window.confirm).toHaveBeenCalledWith(
    m.deleteBoundSkillConfirm('review'),
  );
  expect(services.deleteSkill).toHaveBeenCalledExactlyOnceWith(
    5,
    'bound',
    'skill-1',
  );
  expect(services.listSkills).toHaveBeenCalledTimes(2);
  expect(renderer.root.findAllByType('article')).toHaveLength(0);
});

it('does not delete or refresh after cancellation', async () => {
  const services = await mount();
  vi.mocked(window.confirm).mockReturnValue(false);
  await act(async () => deleteButton().props.onClick());
  expect(services.deleteSkill).not.toHaveBeenCalled();
  expect(services.listSkills).toHaveBeenCalledOnce();
});

it('blocks duplicate deletion and space changes while the request is pending', async () => {
  let finish!: () => void;
  const deleteSkill = vi.fn(
    () =>
      new Promise<void>((resolve) => {
        finish = resolve;
      }),
  );
  await mount({ deleteSkill });
  const click = deleteButton().props.onClick;
  await act(async () => {
    click();
    click();
  });
  expect(deleteSkill).toHaveBeenCalledOnce();
  expect(deleteButton().props.disabled).toBe(true);
  expect(renderer.root.findByType('select').props.disabled).toBe(true);
  await act(async () => finish());
});

it.each(['denied', 'outcomeUnknown'] as const)(
  'preserves the row and shows a %s failure',
  async (code) => {
    const services = await mount({
      deleteSkill: vi.fn(async () => {
        throw { code };
      }),
    });
    await act(async () => deleteButton().props.onClick());
    expect(renderer.root.findByProps({ role: 'alert' }).children).toContain(
      m[code](),
    );
    expect(renderer.root.findAllByType('article')).toHaveLength(1);
    expect(services.listSkills).toHaveBeenCalledOnce();
  },
);

it('maps deletion through the native Usage bridge with the connection and exact skill identity', async () => {
  const deleteSkill = vi.fn(async () => ({
    ok: true,
    value: { deleted: true },
  }));
  const services = createStudioServices({
    deleteSkill,
  } as unknown as typeof Usage);
  await services.resources.deleteSkill!(5, 'bound', 'skill-1');
  expect(deleteSkill).toHaveBeenCalledExactlyOnceWith({
    connectionEpoch: 5,
    spaceId: 'bound',
    skillId: 'skill-1',
  });
});
