import { act, create, type ReactTestRenderer } from 'react-test-renderer';
import { expect, it, vi } from 'vitest';
import { DesktopBotSettings } from '../src/client/desktop/BotSettings';
import type { DesktopServices } from '../src/client/desktop/models';
import { messages, messagesIn } from '../src/client/desktop/generated/locales';
import { LocaleProvider } from '../src/client/desktop/locale';

const m = messages;

it('edits local AGENTS.md and selected skills for the assistant', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  let locale = 'zh-Hans';
  let changed!: (payload: { locale: string; dir: string }) => void;
  const previousMyGo = (globalThis as { mygo?: unknown }).mygo;
  (globalThis as { mygo?: unknown }).mygo = {
    get locale() {
      return locale;
    },
    on: (_name: string, listener: typeof changed) => {
      changed = listener;
      return () => {};
    },
  };
  const saveAssistantConfig = vi.fn(
    async (_epoch, _session, assistantId, input) => ({
      assistantId,
      agentsMd: input.agentsMd,
      skills: input.skills,
      updatedAt: '2026-10-27T00:00:00Z',
    }),
  );
  const service = {
    sessions: {
      assistantConfig: vi.fn(async () => ({
        assistantId: 'runtime-a',
        agentsMd: 'Old instructions',
        skills: [],
      })),
      skillCatalog: vi.fn(async () => [
        {
          id: 'meegle',
          name: 'meegle',
          description: '飞书项目操作',
          source: 'skill-space',
          spaceId: 'agentkit',
        },
      ]),
      saveAssistantConfig,
    },
  } as unknown as DesktopServices;
  let renderer!: ReactTestRenderer;
  await act(async () => {
    renderer = create(
      <LocaleProvider>
        <DesktopBotSettings
          services={service}
          connection={{
            epoch: 2,
            scope: 'scope',
            runtimeId: 'runtime-a',
            status: 'connected',
          }}
          session={{ id: 's', title: 'Assistant' }}
          onSaved={() => {}}
          onClose={() => {}}
        />
      </LocaleProvider>,
    );
  });
  await act(async () => {});
  expect(JSON.stringify(renderer.toJSON())).toContain(
    m.settingsAssistantTitle(),
  );
  expect(renderer.root.findAllByProps({ role: 'dialog' })).toHaveLength(1);
  expect(
    renderer.root.findAll(
      (node) =>
        typeof node.props.className === 'string' &&
        node.props.className.includes('task-detail-card'),
    ),
  ).toHaveLength(0);
  expect(JSON.stringify(renderer.toJSON())).not.toContain(
    'Conversation configuration',
  );
  const agents = renderer.root.findByProps({
    placeholder: m.settingsAssistantInstructionsPlaceholder(),
  });
  await act(async () =>
    agents.props.onChange({ target: { value: 'Use concise answers.' } }),
  );
  expect(JSON.stringify(renderer.toJSON())).not.toContain(
    m.settingsSkillSpace(),
  );
  const skillsTab = renderer.root
    .findAllByProps({ role: 'tab' })
    .find((tab) =>
      tab
        .findAllByType('span')
        .some((span) => span.children.includes(m.settingsSkills())),
    )!;
  await act(async () => skillsTab.props.onClick());
  expect(JSON.stringify(renderer.toJSON())).toContain(m.settingsSkillSpace());
  const skill = renderer.root
    .findAllByType('button')
    .find((button) =>
      button
        .findAllByType('strong')
        .some((node) => node.children.includes('meegle')),
    )!;
  await act(async () => skill.props.onClick());
  await act(async () =>
    renderer.root
      .findByProps({ placeholder: m.settingsSkillImportPlaceholder() })
      .props.onChange({ target: { value: 'Unsaved skill instruction' } }),
  );
  const en = messagesIn('en');
  await act(async () => {
    locale = 'en';
    changed({ locale, dir: 'ltr' });
  });
  expect(JSON.stringify(renderer.toJSON())).toContain(en.settingsSkillSpace());
  expect(JSON.stringify(renderer.toJSON())).toContain('飞书项目操作');
  expect(
    renderer.root.findByProps({
      placeholder: en.settingsSkillImportPlaceholder(),
    }).props.value,
  ).toBe('Unsaved skill instruction');
  expect(service.sessions.assistantConfig).toHaveBeenCalledTimes(1);
  expect(service.sessions.skillCatalog).toHaveBeenCalledTimes(1);
  await act(async () =>
    renderer.root.findByType('form').props.onSubmit({ preventDefault() {} }),
  );
  expect(saveAssistantConfig).toHaveBeenCalledWith(2, 's', 'runtime-a', {
    agentsMd: 'Use concise answers.',
    skills: [
      {
        id: 'meegle',
        name: 'meegle',
        description: '飞书项目操作',
        source: 'skill-space',
        spaceId: 'agentkit',
      },
    ],
  });
  await act(async () => renderer.unmount());
  (globalThis as { mygo?: unknown }).mygo = previousMyGo;
});

it('closes the shared modal on cancel instead of changing page layout', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  const onClose = vi.fn();
  const service = {
    sessions: {
      assistantConfig: vi.fn(async () => ({
        assistantId: 'runtime-a',
        agentsMd: '',
        skills: [],
      })),
      skillCatalog: vi.fn(async () => []),
    },
  } as unknown as DesktopServices;
  let renderer!: ReactTestRenderer;
  await act(async () => {
    renderer = create(
      <DesktopBotSettings
        services={service}
        connection={{
          epoch: 2,
          scope: 'scope',
          runtimeId: 'runtime-a',
          status: 'connected',
        }}
        session={{ id: 's', title: 'Assistant' }}
        onSaved={() => {}}
        onClose={onClose}
      />,
    );
  });
  const layer = renderer.root.findByProps({ className: 'ui-modal-layer' });
  const preventDefault = vi.fn();
  await act(async () =>
    layer.props.onKeyDown({
      key: 'Escape',
      preventDefault,
    }),
  );
  expect(preventDefault).toHaveBeenCalled();
  expect(onClose).toHaveBeenCalledTimes(1);
  await act(async () => renderer.unmount());
});
