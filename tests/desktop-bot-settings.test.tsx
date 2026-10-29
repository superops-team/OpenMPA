import { act, create, type ReactTestRenderer } from 'react-test-renderer';
import { expect, it, vi } from 'vitest';
import { DesktopBotSettings } from '../src/client/desktop/BotSettings';
import type { DesktopServices } from '../src/client/desktop/models';

it('edits local AGENTS.md and selected skills for the assistant', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  const rename = vi.fn(async () => {});
  const updateExecutionConfig = vi.fn(async (_epoch, _session, input) => ({
    executionConfigVersion: '8',
    etag: '"rev-8"',
    model: input.model,
    modelMode: input.modelMode,
    mcpMode: input.mcpMode,
    mcpServers: input.mcpServers,
    profileBound: true,
    invalidRefsCount: 0,
  }));
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
        assistantId: 's',
        agentsMd: 'Old instructions',
        skills: [],
      })),
      modelCatalog: vi.fn(async () => [
        { id: 'deepseek-v4-pro-260101' },
        { id: 'doubao-seed-2-1-pro-260628' },
      ]),
      executionConfig: vi.fn(async () => ({
        executionConfigVersion: '7',
        etag: '"rev-7"',
        model: 'deepseek-v4-pro-260101',
        modelMode: 'replace',
        mcpMode: 'inherit',
        mcpServers: [],
        profileBound: true,
        invalidRefsCount: 0,
      })),
      rename,
      updateExecutionConfig,
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
  const onSaved = vi.fn();
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
        onSaved={onSaved}
        onClose={() => {}}
      />,
    );
  });
  await act(async () => {});
  expect(JSON.stringify(renderer.toJSON())).toContain('Assistant settings');
  await act(async () =>
    renderer.root
      .findByProps({ className: 'composer-model-trigger' })
      .props.onClick(),
  );
  expect(
    renderer.root
      .findAllByProps({ 'aria-checked': true })
      .some((node) =>
        node
          .findAllByType('strong')
          .some((span) => span.children.includes('deepseek-v4-pro-260101')),
      ),
  ).toBe(true);
  const modelSearch = renderer.root.findByProps({
    'aria-label': 'Filter models',
  });
  await act(async () =>
    modelSearch.props.onChange({ target: { value: 'doubao' } }),
  );
  expect(JSON.stringify(renderer.toJSON())).toContain(
    'doubao-seed-2-1-pro-260628',
  );
  expect(
    renderer.root
      .findAllByProps({ role: 'radio' })
      .map((node) => node.findByType('strong').children.join('')),
  ).not.toContain('deepseek-v4-pro-260101');
  await act(async () => modelSearch.props.onChange({ target: { value: '' } }));
  await act(async () =>
    renderer.root
      .findByProps({ className: 'composer-model-trigger' })
      .props.onClick(),
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
    placeholder: 'Write persistent instructions for this assistant.',
  });
  await act(async () =>
    agents.props.onChange({ target: { value: 'Use concise answers.' } }),
  );
  expect(JSON.stringify(renderer.toJSON())).not.toContain('Skill Space');
  const skillsTab = renderer.root
    .findAllByProps({ role: 'tab' })
    .find((tab) =>
      tab
        .findAllByType('span')
        .some((span) => span.children.includes('Skills')),
    )!;
  await act(async () => skillsTab.props.onClick());
  expect(JSON.stringify(renderer.toJSON())).toContain('Skill Space');
  const skill = renderer.root
    .findAllByType('button')
    .find((button) =>
      button
        .findAllByType('strong')
        .some((node) => node.children.includes('meegle')),
    )!;
  await act(async () => skill.props.onClick());
  await act(async () =>
    renderer.root.findByType('form').props.onSubmit({ preventDefault() {} }),
  );
  expect(saveAssistantConfig).toHaveBeenCalledWith(2, 's', 's', {
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
  expect(updateExecutionConfig).not.toHaveBeenCalled();
  expect(onSaved).toHaveBeenCalledWith(
    expect.objectContaining({
      id: 's',
      title: 'Assistant',
      configVersion: '7',
    }),
  );
  await act(async () => renderer.unmount());
});

it('closes the shared modal on cancel instead of changing page layout', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  const onClose = vi.fn();
  const service = {
    sessions: {
      assistantConfig: vi.fn(async () => ({
        assistantId: 's',
        agentsMd: '',
        skills: [],
      })),
      modelCatalog: vi.fn(async () => []),
      executionConfig: vi.fn(async () => ({
        executionConfigVersion: '7',
        etag: '"rev-7"',
        modelMode: 'inherit',
        mcpMode: 'inherit',
        mcpServers: [],
        profileBound: true,
        invalidRefsCount: 0,
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
