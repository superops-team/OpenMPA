import { act, create, type ReactTestRenderer } from 'react-test-renderer';
import { beforeEach, expect, it, vi } from 'vitest';
const channels = vi.hoisted(() => ({
  capabilities: vi.fn(),
  privateUser: vi.fn(),
  resetPrivateUser: vi.fn(),
  createBinding: vi.fn(),
}));
vi.mock('../src/client/desktop/generated/mygo', () => ({ Channels: channels }));
import { ChannelManagement } from '../src/client/desktop/ChannelManagement';
beforeEach(() => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  vi.stubGlobal('window', { confirm: () => true });
  channels.capabilities.mockResolvedValue({
    ok: true,
    value: {
      channels: ['feishu', 'dingtalk', 'wecom'],
      feishuDomains: ['feishu', 'lark'],
      botBindingTypes: ['shared', 'private'],
      managementScope: 'runtimeAdmin',
      ownerIdentityAvailable: true,
      configHasCas: false,
    },
  });
});
it('displays the safe QR image while the canonical binding state is waiting', async () => {
  channels.createBinding.mockResolvedValue({
    ok: true,
    value: {
      operationId: 'op',
      id: 'binding',
      channel: 'feishu',
      state: 'waiting',
      status: 'PENDING',
      qrCodeImage: 'data:image/png;base64,c2FmZQ==',
      pollAfterSeconds: 60,
    },
  });
  let renderer!: ReactTestRenderer;
  await act(async () => {
    renderer = create(
      <ChannelManagement
        connection={{
          epoch: 7,
          runtimeId: 'r',
          scope: 'scope',
          status: 'ready',
        }}
        accounts={[]}
        sessions={[]}
        onChanged={async () => {}}
      />,
    );
  });
  await act(async () => {
    renderer.root
      .findAllByType('form')[0]
      .props.onSubmit({ preventDefault() {} });
  });
  expect(
    renderer.root.findByProps({ className: 'desktop-binding-qr' }).props.src,
  ).toBe('data:image/png;base64,c2FmZQ==');
  expect(channels.createBinding.mock.calls.at(-1)?.[0]).toMatchObject({
    connectionEpoch: 7,
    channel: 'feishu',
    mode: 'qr',
    domain: 'feishu',
    bindingType: 'shared',
  });
  await act(async () => renderer.unmount());
});
it('uses the reviewed private-user slot version and preserves it after a conflict', async () => {
  channels.privateUser.mockResolvedValue({
    ok: true,
    value: {
      appId: 'a',
      slot: { version: 'v1', runtimeUserId: 'u' },
      authorization: { status: 'authorized' },
    },
  });
  channels.resetPrivateUser.mockResolvedValue({
    ok: false,
    error: {
      code: 'conflict',
      message: 'Slot changed',
      httpStatus: 409,
      currentState: { status: 'changed' },
    },
  });
  let renderer!: ReactTestRenderer;
  await act(async () => {
    renderer = create(
      <ChannelManagement
        connection={{
          epoch: 7,
          runtimeId: 'r',
          scope: 'scope',
          status: 'ready',
        }}
        accounts={[
          {
            id: 'a',
            type: 'feishu',
            name: 'Bot',
            enabled: true,
            status: 'configured',
          },
        ]}
        sessions={[]}
        onChanged={async () => {}}
      />,
    );
  });
  const choose = renderer.root
    .findAllByType('select')
    .find((select) =>
      select.children.some(
        (child) =>
          typeof child === 'object' && child.props?.value === 'feishu:a',
      ),
    )!;
  await act(async () => {
    choose.props.onChange({ target: { value: 'feishu:a' } });
  });
  const button = (text: string) =>
    renderer.root
      .findAllByType('button')
      .find((button) => button.children.includes(text))!;
  await act(async () => {
    button('Read private user and slot').props.onClick();
  });
  await act(async () => {
    button('Reset private user').props.onClick();
  });
  expect(channels.resetPrivateUser).toHaveBeenLastCalledWith({
    connectionEpoch: 7,
    channel: 'feishu',
    appId: 'a',
    expectedVersion: 'v1',
  });
  expect(JSON.stringify(renderer.toJSON())).toContain('v1');
  expect(JSON.stringify(renderer.toJSON())).toContain('Slot changed');
  await act(async () => renderer.unmount());
});
