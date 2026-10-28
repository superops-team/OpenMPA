import type { ReactElement } from 'react';
import { act, create, type ReactTestRenderer } from 'react-test-renderer';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { LocaleProvider } from '../src/client/desktop/locale';
import { messagesIn } from '../src/client/desktop/generated/locales';
import type { DesktopServices } from '../src/client/desktop/models';

const bridge = vi.hoisted(() => ({
  channels: {
    capabilities: vi.fn(),
    diagnostics: vi.fn(),
    permissions: vi.fn(),
    createBinding: vi.fn(),
  },
  knowledge: {
    list: vi.fn(),
    importText: vi.fn(),
  },
  schedule: {
    list: vi.fn(),
    read: vi.fn(),
    runs: vi.fn(),
    run: vi.fn(),
  },
}));
vi.mock('../src/client/desktop/generated/mygo', () => ({
  Channels: bridge.channels,
  Knowledge: bridge.knowledge,
  Schedule: bridge.schedule,
}));

import { DesktopChannels } from '../src/client/desktop/Channels';
import { ChannelManagement } from '../src/client/desktop/ChannelManagement';
import { channelStatusLabel } from '../src/client/desktop/ChannelManagementState';
import { DesktopKnowledge } from '../src/client/desktop/Knowledge';
import { DesktopSchedules } from '../src/client/desktop/Schedules';
import { useDesktopAppState } from '../src/client/desktop/DesktopAppState';
import { desktopFeatures } from '../src/client/desktop/features';

const connection = {
  epoch: 7,
  runtimeId: 'runtime-id',
  scope: 'scope',
  status: 'connected',
};
const renderers: ReactTestRenderer[] = [];
let locale: string;
let localeChanged: (payload: { locale: string; dir: string }) => void;

beforeEach(() => {
  vi.resetAllMocks();
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  bridge.channels.diagnostics.mockResolvedValue({
    ok: true,
    value: {
      configured: true,
      gatewayConfigured: true,
      routeConfigured: true,
      deliveryHealth: 'not_observed',
      missingConfiguration: [],
    },
  });
  bridge.channels.permissions.mockResolvedValue({
    ok: true,
    value: { permissions: [] },
  });
  locale = 'zh-Hans';
  vi.stubGlobal('mygo', {
    get locale() {
      return locale;
    },
    on: (_event: string, listener: typeof localeChanged) => {
      localeChanged = listener;
      return () => {};
    },
  });
  bridge.channels.capabilities.mockResolvedValue({
    ok: true,
    value: {
      channels: ['feishu'],
      serverSideBinding: true,
      bindingReady: true,
      credentialBindingChannels: ['feishu'],
      feishuDomains: ['feishu', 'lark'],
      botBindingTypes: ['shared', 'private'],
      managementScope: 'runtimeAdmin',
      ownerIdentityAvailable: true,
      configHasCas: false,
    },
  });
  bridge.knowledge.list.mockResolvedValue({ ok: true, value: { items: [] } });
});
afterEach(async () => {
  await act(async () => {
    for (const renderer of renderers.splice(0)) renderer.unmount();
  });
  vi.unstubAllGlobals();
});

async function renderPage(page: ReactElement) {
  let renderer!: ReactTestRenderer;
  await act(async () => {
    renderer = create(<LocaleProvider>{page}</LocaleProvider>);
    renderers.push(renderer);
  });
  return renderer;
}
async function switchToEnglish() {
  await act(async () => {
    locale = 'en';
    localeChanged({ locale, dir: 'ltr' });
  });
}
function button(renderer: ReactTestRenderer, text: string) {
  return renderer.root
    .findAllByType('button')
    .find((item) =>
      item.children.some(
        (child) => typeof child === 'string' && child.trim() === text,
      ),
    )!;
}

it('redraws channel binding notices without resubmitting or reloading capabilities', async () => {
  bridge.channels.createBinding.mockResolvedValue({
    ok: true,
    value: {
      id: 'binding-id',
      operationId: 'operation-id',
      channel: 'feishu',
      status: 'REGISTERED',
      state: 'registered',
    },
  });
  const renderer = await renderPage(
    <ChannelManagement
      connection={connection}
      accounts={[]}
      sessions={[]}
      onChanged={async () => {}}
    />,
  );
  await act(async () =>
    button(
      renderer,
      messagesIn('zh-Hans').channelAddAccountTitle(),
    ).props.onClick(),
  );
  expect(
    renderer.root
      .findAllByType('option')
      .find((item) => item.props.value === 'shared')!.children,
  ).toEqual([messagesIn('zh-Hans').channelSharedBinding()]);
  await act(async () => {
    renderer.root
      .findByProps({ 'data-testid': 'channel-binding-form' })
      .props.onSubmit({ preventDefault() {} });
  });
  expect(renderer.root.findByProps({ role: 'status' }).children).toEqual([
    messagesIn('zh-Hans').desktopBound(),
  ]);
  await switchToEnglish();
  expect(renderer.root.findByProps({ role: 'status' }).children).toEqual([
    messagesIn('en').desktopBound(),
  ]);
  expect(bridge.channels.capabilities).toHaveBeenCalledOnce();
  expect(bridge.channels.createBinding).toHaveBeenCalledOnce();
  expect(bridge.channels.createBinding).toHaveBeenCalledWith(
    expect.objectContaining({
      channel: 'feishu',
      mode: 'qr',
      domain: 'feishu',
      bindingType: 'shared',
    }),
  );
});

it('redraws the channel read fallback without retrying the network request', async () => {
  const list = vi.fn().mockRejectedValue(undefined);
  const services = { channels: { list } } as unknown as DesktopServices;
  const renderer = await renderPage(
    <DesktopChannels
      services={services}
      connection={connection}
      sessions={[]}
    />,
  );
  expect(renderer.root.findByProps({ role: 'alert' }).children).toEqual([
    '无法读取渠道列表。',
  ]);
  await switchToEnglish();
  expect(renderer.root.findByProps({ role: 'alert' }).children).toEqual([
    'Could not read channels.',
  ]);
  expect(list).toHaveBeenCalledOnce();
});

it('retains unknown status values that match object prototype names', async () => {
  expect(channelStatusLabel('constructor', messagesIn('zh-Hans'))).toBe(
    'constructor',
  );
  expect(channelStatusLabel('constructor', messagesIn('en'))).toBe(
    'constructor',
  );
  const schedules = {
    schedules: {
      list: vi.fn(async () => [
        {
          id: 'task',
          name: 'Remote task',
          prompt: 'Prompt',
          enabled: true,
          version: '1',
          schedule: { type: 'constructor' },
          timezone: 'UTC',
          status: 'constructor',
        },
      ]),
    },
  } as unknown as DesktopServices;
  const schedulePage = await renderPage(
    <DesktopSchedules
      services={schedules}
      connection={connection}
      sessions={[]}
    />,
  );
  expect(JSON.stringify(schedulePage.toJSON())).toContain('constructor');
  await switchToEnglish();
  expect(JSON.stringify(schedulePage.toJSON())).toContain('constructor');
  expect(schedules.schedules!.list).toHaveBeenCalledOnce();
  await act(async () => schedulePage.unmount());
  bridge.knowledge.list.mockResolvedValue({
    ok: true,
    value: {
      items: [
        {
          knowledgeId: 'knowledge-id',
          displayName: 'Remote item',
          status: 'constructor',
          sourceType: 'text',
        },
      ],
    },
  });
  const knowledgePage = await renderPage(
    <DesktopKnowledge
      connection={connection}
      sessions={[{ id: 'session-id', title: 'Remote session' }]}
    />,
  );
  await act(async () =>
    knowledgePage.root
      .findByType('select')
      .props.onChange({ target: { value: 'session-id' } }),
  );
  expect(JSON.stringify(knowledgePage.toJSON())).toContain('constructor');
});

it('keeps remote knowledge errors and conversation names byte-for-byte in both locales', async () => {
  const remoteMessage = 'REMOTE validation:\n  keep <ID_42> exactly.';
  bridge.knowledge.list.mockResolvedValue({
    ok: false,
    error: { code: 'remoteFailure', message: remoteMessage },
  });
  const renderer = await renderPage(
    <DesktopKnowledge
      connection={connection}
      sessions={[{ id: 'session-id', title: 'Remote English title' }]}
    />,
  );
  await act(async () => {
    renderer.root
      .findByType('select')
      .props.onChange({ target: { value: 'session-id' } });
  });
  expect(renderer.root.findByProps({ role: 'alert' }).children).toEqual([
    remoteMessage,
  ]);
  expect(
    renderer.root
      .findAllByType('option')
      .find((item) => item.props.value === 'session-id')!.children,
  ).toEqual(['Remote English title']);
  await switchToEnglish();
  expect(renderer.root.findByProps({ role: 'alert' }).children).toEqual([
    remoteMessage,
  ]);
  expect(bridge.knowledge.list).toHaveBeenCalledOnce();
});

it('redraws a retained knowledge fallback without refetching the library', async () => {
  bridge.knowledge.list.mockRejectedValue(undefined);
  const renderer = await renderPage(
    <DesktopKnowledge
      connection={connection}
      sessions={[{ id: 'session-id', title: 'Remote title' }]}
    />,
  );
  await act(async () => {
    renderer.root
      .findByType('select')
      .props.onChange({ target: { value: 'session-id' } });
  });
  expect(renderer.root.findByProps({ role: 'alert' }).children).toEqual([
    '知识库操作失败。',
  ]);
  await switchToEnglish();
  expect(renderer.root.findByProps({ role: 'alert' }).children).toEqual([
    'Knowledge operation failed.',
  ]);
  expect(bridge.knowledge.list).toHaveBeenCalledOnce();
});

it('localizes schedule notices and labels while preserving delivery protocol values and remote text', async () => {
  const task = {
    id: 'task-id',
    name: 'Remote English task',
    prompt: 'REMOTE prompt:\n  exact content.',
    agentId: '',
    enabled: true,
    version: 'v1',
    schedule: { type: 'Once', runAt: '2026-10-28T00:00:00Z' },
    timezone: 'UTC',
  };
  const output = 'REMOTE output:\n  untouched.';
  const list = vi.fn(async () => [task]);
  const trigger = vi.fn(async () => ({
    id: 'run-id',
    status: 'Queued',
    output,
  }));
  const deliveryTargets = vi.fn(async () => []);
  const services = {
    schedules: {
      list,
      read: async () => ({ task, runs: [] }),
      trigger,
      deliveryTargets,
    },
  } as unknown as DesktopServices;
  const renderer = await renderPage(
    <DesktopSchedules
      services={services}
      connection={connection}
      sessions={[]}
    />,
  );
  await act(async () =>
    renderer.root.findByProps({ className: 'task-row' }).props.onClick(),
  );
  await act(async () =>
    button(renderer, messagesIn('zh-Hans').desktopRunNow()).props.onClick(),
  );
  expect(
    renderer.root.findByProps({ className: 'task-event' }).findByType('strong')
      .children,
  ).toEqual(['排队中']);
  expect(
    renderer.root.findByProps({ className: 'task-event' }).findByType('p')
      .children,
  ).toEqual([output]);
  const requests = list.mock.calls.length;
  await switchToEnglish();
  expect(list).toHaveBeenCalledTimes(requests);
  expect(trigger).toHaveBeenCalledOnce();
  expect(
    renderer.root.findByProps({ className: 'task-event' }).findByType('strong')
      .children,
  ).toEqual(['Queued']);
  expect(
    renderer.root.findByProps({ className: 'task-event' }).findByType('p')
      .children,
  ).toEqual([output]);
  expect(renderer.root.findByProps({ role: 'status' }).children).toEqual([
    messagesIn('en').runAcceptedNotice('run-id', 'Queued'),
  ]);
  await act(async () =>
    button(renderer, messagesIn('en').desktopEdit()).props.onClick(),
  );
  const delivery = renderer.root
    .findAllByType('select')
    .find((item) => item.props.value === 'Web')!;
  await act(async () =>
    delivery.props.onChange({ target: { value: 'Feishu' } }),
  );
  await act(async () =>
    button(
      renderer,
      messagesIn('en').desktopReadPermittedDeliveryTargets(),
    ).props.onClick(),
  );
  expect(deliveryTargets).toHaveBeenCalledWith(7, 'Feishu', '');
  expect(task.schedule.type).toBe('Once');
  expect(task.prompt).toBe('REMOTE prompt:\n  exact content.');
});

it('redraws app failure context without reconnecting or translating the remote message', async () => {
  const remoteMessage = 'REMOTE connection:\n  preserve byte-for-byte.';
  const status = vi.fn().mockRejectedValue(new Error(remoteMessage));
  const services = { connection: { status } } as unknown as DesktopServices;
  function Harness() {
    return <p role="alert">{useDesktopAppState({ services }).error}</p>;
  }
  const renderer = await renderPage(<Harness />);
  expect(renderer.root.findByType('p').children).toEqual([
    `读取连接状态: ${remoteMessage}`,
  ]);
  await switchToEnglish();
  expect(renderer.root.findByType('p').children).toEqual([
    `Read connection: ${remoteMessage}`,
  ]);
  expect(status).toHaveBeenCalledOnce();
});

it('keeps the unsupported template notice informational under Advanced', async () => {
  const templates = vi.fn(async () => []);
  const services = {
    channels: {
      ...desktopFeatures().channels!,
      list: vi.fn(async () => []),
      templates,
    },
  } as unknown as DesktopServices;
  const renderer = await renderPage(
    <DesktopChannels
      services={services}
      connection={connection}
      sessions={[]}
    />,
  );
  expect(renderer.root.findAllByProps({ role: 'alert' })).toHaveLength(0);
  expect(
    button(renderer, messagesIn('zh-Hans').desktopReadTemplates()),
  ).toBeUndefined();
  expect(JSON.stringify(renderer.toJSON())).toContain(
    messagesIn(
      'zh-Hans',
    ).desktopTheSelectedRuntimeDoesNotExposeATemplateCatalog(),
  );
  await switchToEnglish();
  expect(JSON.stringify(renderer.toJSON())).toContain(
    messagesIn('en').desktopTheSelectedRuntimeDoesNotExposeATemplateCatalog(),
  );
  expect(templates).not.toHaveBeenCalled();
  expect(services.channels!.list).toHaveBeenCalledOnce();
});

it('localizes the diagnostic failure code instead of retaining the Go English fallback', async () => {
  const fallback = 'The server returned an invalid response. Reload this view.';
  bridge.channels.diagnostics.mockResolvedValue({
    ok: false,
    error: { code: 'invalidResponse', message: fallback },
  });
  const services = {
    channels: {
      ...desktopFeatures().channels!,
      list: vi.fn(async () => [
        {
          id: 'a',
          name: 'Bot',
          type: 'feishu',
          enabled: true,
          status: 'configured',
          bindingType: 'shared',
        },
      ]),
    },
  } as unknown as DesktopServices;
  const renderer = await renderPage(
    <DesktopChannels
      services={services}
      connection={connection}
      sessions={[]}
    />,
  );
  expect(JSON.stringify(renderer.toJSON())).toContain(
    messagesIn('zh-Hans').invalidResponse(),
  );
  expect(JSON.stringify(renderer.toJSON())).not.toContain(fallback);
  await switchToEnglish();
  expect(JSON.stringify(renderer.toJSON())).toContain(
    messagesIn('en').invalidResponse(),
  );
  expect(bridge.channels.diagnostics).toHaveBeenCalledOnce();
});

it('localizes receipt prefixes from the actual schedule adapter and retains IDs', async () => {
  const task = {
    id: 'task-id',
    name: 'Remote task',
    prompt: 'Remote prompt',
    enabled: true,
    version: 1,
    schedule: { type: 'Once', runAt: '2026-10-28T00:00:00Z' },
    delivery: { channel: 'Web' },
  };
  const run = {
    id: 'run-id',
    status: 'Queued',
    sessionId: 'Remote Session ID',
    invocationId: 'Remote Invocation ID',
  };
  bridge.schedule.list.mockResolvedValue({
    ok: true,
    value: { items: [task], hasMore: false },
  });
  bridge.schedule.read.mockResolvedValue({ ok: true, value: { task } });
  bridge.schedule.runs.mockResolvedValue({
    ok: true,
    value: { items: [run], hasMore: false },
  });
  bridge.schedule.run.mockResolvedValue({ ok: true, value: { run } });
  vi.stubGlobal('crypto', { randomUUID: () => 'client-token' });
  const renderer = await renderPage(
    <DesktopSchedules
      services={desktopFeatures() as DesktopServices}
      connection={connection}
      sessions={[]}
    />,
  );
  await act(async () =>
    renderer.root.findByProps({ className: 'task-row' }).props.onClick(),
  );
  const receipt = () =>
    renderer.root.findByProps({ className: 'task-event' }).findByType('p')
      .children;
  expect(receipt()).toEqual([
    `${messagesIn('zh-Hans').runSessionLabel(run.sessionId)} · ${messagesIn('zh-Hans').runInvocationLabel(run.invocationId)}`,
  ]);
  await switchToEnglish();
  expect(receipt()).toEqual([
    `${messagesIn('en').runSessionLabel(run.sessionId)} · ${messagesIn('en').runInvocationLabel(run.invocationId)}`,
  ]);
  expect(bridge.schedule.read).toHaveBeenCalledOnce();
  expect(bridge.schedule.runs).toHaveBeenCalledOnce();
  await act(async () =>
    button(renderer, messagesIn('en').desktopRunNow()).props.onClick(),
  );
  expect(receipt()).toEqual([
    `${messagesIn('en').runSessionLabel(run.sessionId)} · ${messagesIn('en').runInvocationLabel(run.invocationId)}`,
  ]);
  expect(bridge.schedule.run).toHaveBeenCalledOnce();
});
