import { act, create, type ReactTestRenderer } from 'react-test-renderer';
import { beforeEach, afterEach, it, expect, vi } from 'vitest';
import type { DesktopServices } from '../src/client/desktop/models';
const api = vi.hoisted(() => ({
  capabilities: vi.fn(),
  diagnostics: vi.fn(),
  permissions: vi.fn(),
  list: vi.fn(),
}));
vi.mock('../src/client/desktop/generated/mygo', () => ({
  Channels: api,
  Schedule: {},
  Connection: {},
  Deployment: {},
}));
import { desktopFeatures } from '../src/client/desktop/features';
import { ChannelManagement } from '../src/client/desktop/ChannelManagement';
import { DesktopChannels } from '../src/client/desktop/Channels';
import { messagesIn } from '../src/client/desktop/generated/locales';
const m = messagesIn('en');
const connection = {
  epoch: 8,
  runtimeId: 'r',
  scope: 'scope',
  status: 'ready',
};
let renderer: ReactTestRenderer | undefined;
beforeEach(() => {
  vi.resetAllMocks();
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  api.diagnostics.mockResolvedValue({
    ok: true,
    value: {
      configured: true,
      gatewayConfigured: true,
      routeConfigured: true,
      deliveryHealth: 'not_observed',
      missingConfiguration: [],
    },
  });
  api.permissions.mockResolvedValue({ ok: true, value: { permissions: [] } });
  vi.stubGlobal('mygo', { locale: 'en' });
  api.capabilities.mockResolvedValue({
    ok: true,
    value: {
      channels: ['feishu', 'dingtalk'],
      feishuDomains: ['feishu', 'lark'],
      botBindingTypes: ['shared', 'private'],
      credentialBindingChannels: ['feishu'],
      serverSideBinding: true,
      bindingReady: true,
      ownerIdentityAvailable: true,
    },
  });
  api.permissions.mockResolvedValue({ ok: true, value: { permissions: [] } });
});
afterEach(async () => {
  if (renderer) await act(async () => renderer!.unmount());
  renderer = undefined;
  vi.unstubAllGlobals();
});
it('preserves every Bot and its Feishu/Lark domain in the production service', async () => {
  api.list.mockResolvedValue({
    ok: true,
    value: {
      channels: [
        {
          channel: 'feishu',
          appId: 'a',
          enabled: true,
          bindingType: 'shared',
          config: { domain: 'feishu' },
        },
        {
          channel: 'feishu',
          appId: 'b',
          enabled: false,
          bindingType: 'private',
          config: { domain: 'lark' },
        },
      ],
    },
  });
  const bots = await desktopFeatures().channels!.list(8);
  expect(bots.map((b) => [b.id, b.domain, b.bindingType])).toEqual([
    ['a', 'feishu', 'shared'],
    ['b', 'lark', 'private'],
  ]);
});
it('selects exact Bot IDs for permissions and separates Lark from Feishu accounts', async () => {
  const accounts = ['a', 'b', 'lark'].map((id) => ({
    id,
    type: 'feishu',
    domain: id === 'lark' ? 'lark' : 'feishu',
    enabled: true,
    name: id,
    status: 'configured',
  }));
  await act(async () => {
    renderer = create(
      <ChannelManagement
        connection={connection}
        accounts={accounts}
        sessions={[]}
        onChanged={async () => {}}
      />,
    );
  });
  const picker = () =>
    renderer!.root
      .findAllByType('select')
      .find((s) =>
        s.children.some(
          (c) => typeof c === 'object' && c.props?.value?.startsWith('feishu:'),
        ),
      )!;
  expect(
    picker()
      .findAllByType('option')
      .map((o) => o.props.value),
  ).toEqual(['', 'feishu:a', 'feishu:b']);
  await act(async () =>
    picker().props.onChange({ target: { value: 'feishu:b' } }),
  );
  const read = () =>
    renderer!.root
      .findAllByType('button')
      .find(
        (b) => b.props['aria-label'] === m.desktopReadFeishuGroupPermissions(),
      )!;
  await act(async () => read().props.onClick());
  expect(api.permissions).toHaveBeenLastCalledWith({
    connectionEpoch: 8,
    channel: 'feishu',
    appId: 'b',
  });
  await act(async () =>
    renderer!.root
      .findAllByType('button')
      .find((b) => b.props.role === 'tab' && b.children.includes('Lark'))!
      .props.onClick(),
  );
  expect(picker().props.value).toBe('feishu:lark');
  expect(
    picker()
      .findAllByType('option')
      .map((o) => o.props.value),
  ).toEqual(['', 'feishu:lark']);
  await act(async () => read().props.onClick());
  expect(api.permissions).toHaveBeenLastCalledWith({
    connectionEpoch: 8,
    channel: 'feishu',
    appId: 'lark',
  });
});
it('reports a failed directory read without claiming no accounts exist', async () => {
  const services = {
    channels: {
      list: vi.fn(async () => {
        throw new Error('Read failed');
      }),
    },
  } as unknown as DesktopServices;
  await act(async () => {
    renderer = create(
      <DesktopChannels
        services={services}
        connection={connection}
        sessions={[]}
      />,
    );
  });
  const text = JSON.stringify(renderer!.toJSON());
  expect(text).toContain(m.channelAccountsNotLoaded());
  expect(text).not.toContain(m.channelNoAccounts());
});
it('keeps setup fields out of the default view until Add a bot is chosen', async () => {
  const accounts = [
    {
      id: 'a',
      type: 'feishu',
      domain: 'feishu',
      bindingType: 'shared',
      enabled: true,
      name: 'Team bot',
      status: 'configured',
    },
  ];
  await act(async () => {
    renderer = create(
      <ChannelManagement
        connection={connection}
        accounts={accounts}
        sessions={[]}
        onChanged={async () => {}}
      />,
    );
  });
  expect(
    renderer!.root.findAllByProps({ 'data-testid': 'channel-binding-form' }),
  ).toHaveLength(0);
  expect(api.permissions).toHaveBeenLastCalledWith({
    connectionEpoch: 8,
    channel: 'feishu',
    appId: 'a',
  });
  expect(api.diagnostics).toHaveBeenLastCalledWith({
    connectionEpoch: 8,
    channel: 'feishu',
    appId: 'a',
  });
  expect(
    renderer!.root
      .findAllByType('summary')
      .some((s) => s.children.includes(m.channelAdvancedSettings())),
  ).toBe(true);
  await act(async () =>
    renderer!.root
      .findAllByType('button')
      .find((b) => b.children.includes(m.channelAddAccountTitle()))!
      .props.onClick(),
  );
  const form = renderer!.root.findByProps({
    'data-testid': 'channel-binding-form',
  });
  expect(form.findAllByProps({ role: 'tablist' })).toHaveLength(0);
  expect(form.findAllByType('select').map((s) => s.props.value)).toEqual([
    'qr',
    'shared',
  ]);
});
it('ignores a late health response after selecting another Bot', async () => {
  let resolveOld!: (value: unknown) => void;
  let resolveOldGroups!: (value: unknown) => void;
  api.permissions.mockImplementation((request: { appId: string }) =>
    request.appId === 'a'
      ? new Promise((resolve) => {
          resolveOldGroups = resolve;
        })
      : Promise.resolve({ ok: true, value: { permissions: [] } }),
  );
  api.diagnostics.mockImplementation((request: { appId: string }) =>
    request.appId === 'a'
      ? new Promise((resolve) => {
          resolveOld = resolve;
        })
      : Promise.resolve({
          ok: true,
          value: {
            configured: true,
            gatewayConfigured: true,
            routeConfigured: true,
            deliveryHealth: 'delivered',
          },
        }),
  );
  const accounts = ['a', 'b'].map((id) => ({
    id,
    type: 'feishu',
    enabled: true,
    name: id,
    status: 'configured',
  }));
  await act(async () => {
    renderer = create(
      <ChannelManagement
        connection={connection}
        accounts={accounts}
        sessions={[]}
        onChanged={async () => {}}
      />,
    );
  });
  const picker = () =>
    renderer!.root.findByProps({ 'data-testid': 'channel-account-picker' });
  await act(async () =>
    picker().props.onChange({ target: { value: 'feishu:a' } }),
  );
  await act(async () =>
    picker().props.onChange({ target: { value: 'feishu:b' } }),
  );
  await act(async () =>
    resolveOld({
      ok: true,
      value: {
        configured: false,
        gatewayConfigured: false,
        routeConfigured: false,
        deliveryHealth: 'failed',
      },
    }),
  );
  await act(async () =>
    resolveOldGroups({
      ok: true,
      value: {
        permissions: [{ chatId: 'oc_old_account', chatName: 'Old Bot group' }],
      },
    }),
  );
  expect(JSON.stringify(renderer!.toJSON())).not.toContain('oc_old_account');
  const health = renderer!.root
    .findByType('dl')
    .findAllByType('dd')
    .flatMap((item) =>
      item.children.filter((child) => typeof child === 'string'),
    )
    .join(' ');
  expect(health).toContain(m.channelDelivered());
  expect(picker().props.value).toBe('feishu:b');
});
