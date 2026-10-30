import { act, create, type ReactTestRenderer } from 'react-test-renderer';
import { beforeEach, afterEach, it, expect, vi } from 'vitest';
const api = vi.hoisted(() => ({
  capabilities: vi.fn(),
  createBinding: vi.fn(),
  readBinding: vi.fn(),
  diagnostics: vi.fn(),
  permissions: vi.fn(),
}));
vi.mock('../src/client/desktop/generated/mygo', () => ({ Channels: api }));
vi.mock('../src/client/desktop/wecom-authorization', () => ({
  prepareWecomAuthorization: async () => {},
  authorizeWecom: async () => ({ botId: 'wecom-bot', secret: 'test-secret' }),
  WecomAuthorizationError: class extends Error {},
}));
import { ChannelManagement } from '../src/client/desktop/ChannelManagement';
import type { ChannelVM } from '../src/client/desktop/models';
import { messagesIn } from '../src/client/desktop/generated/locales';
let renderer: ReactTestRenderer | undefined;
const m = messagesIn('en');
const caps = {
  channels: ['feishu', 'dingtalk', 'wecom'],
  feishuDomains: ['feishu', 'lark'],
  botBindingTypes: ['shared', 'private'],
  credentialBindingChannels: ['feishu', 'dingtalk', 'wecom'],
  serverSideBinding: true,
  bindingReady: true,
  ownerIdentityAvailable: true,
};
beforeEach(() => {
  vi.resetAllMocks();
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  vi.stubGlobal('mygo', { locale: 'en' });
  vi.stubGlobal('window', { confirm: () => true });
  api.capabilities.mockResolvedValue({ ok: true, value: caps });
  api.diagnostics.mockResolvedValue({
    ok: true,
    value: { missingConfiguration: [] },
  });
  api.permissions.mockResolvedValue({ ok: true, value: { permissions: [] } });
});
afterEach(async () => {
  if (renderer) await act(async () => renderer!.unmount());
  renderer = undefined;
  vi.useRealTimers();
  vi.unstubAllGlobals();
});
async function mount(
  onChanged = vi.fn(async () => {}),
  accounts: ChannelVM[] = [],
) {
  await act(async () => {
    renderer = create(
      <ChannelManagement
        connection={{
          epoch: 8,
          runtimeId: 'r',
          scope: 'scope',
          status: 'ready',
        }}
        accounts={accounts}
        sessions={[]}
        onChanged={onChanged}
      />,
    );
  });
  return onChanged;
}
async function submit() {
  if (
    !renderer!.root.findAllByProps({ 'data-testid': 'channel-binding-form' })
      .length
  ) {
    await act(async () =>
      renderer!.root
        .findAllByType('button')
        .find((b) => b.children.includes(m.channelAddAccountTitle()))!
        .props.onClick(),
    );
  }
  await act(async () =>
    renderer!.root
      .findByProps({ 'data-testid': 'channel-binding-form' })
      .props.onSubmit({ preventDefault() {} }),
  );
}
it('does not submit bindings when the Runtime reports not ready', async () => {
  api.capabilities.mockResolvedValue({
    ok: true,
    value: { ...caps, bindingReady: false },
  });
  await mount();
  expect(
    renderer!.root
      .findAllByType('button')
      .find((b) => b.children.includes(m.channelAddAccountTitle()))!.props
      .disabled,
  ).toBe(true);
  expect(api.createBinding).not.toHaveBeenCalled();
  expect(JSON.stringify(renderer!.toJSON())).toContain(
    m.channelBindingNotReady(),
  );
});
it('polls the same QR operation to completion and refreshes accounts', async () => {
  vi.useFakeTimers();
  api.createBinding.mockResolvedValue({
    ok: true,
    value: {
      operationId: 'op',
      id: 'qr-1',
      channel: 'feishu',
      state: 'waiting',
      status: 'PENDING',
      pollAfterSeconds: 1,
    },
  });
  api.readBinding.mockResolvedValue({
    ok: true,
    value: {
      operationId: 'op',
      id: 'qr-1',
      channel: 'feishu',
      state: 'registered',
      status: 'BOUND',
    },
  });
  const changed = await mount();
  await submit();
  await act(async () => {
    await vi.advanceTimersByTimeAsync(1000);
  });
  expect(api.readBinding).toHaveBeenCalledExactlyOnceWith({
    connectionEpoch: 8,
    operationId: 'op',
    channel: 'feishu',
    bindingId: 'qr-1',
    resumeConfirmed: false,
  });
  expect(changed).toHaveBeenCalledOnce();
  await act(async () => {
    await vi.advanceTimersByTimeAsync(5000);
  });
  expect(api.readBinding).toHaveBeenCalledOnce();
});
it('can correct a rejected request but blocks a second binding while its outcome is unknown', async () => {
  api.createBinding
    .mockResolvedValueOnce({
      ok: false,
      error: { code: 'invalidInput', message: 'Invalid' },
    })
    .mockResolvedValueOnce({
      ok: false,
      error: { code: 'outcomeUnknown', message: 'Unknown' },
    });
  await mount();
  await submit();
  await submit();
  await submit();
  expect(api.createBinding).toHaveBeenCalledTimes(2);
  expect(JSON.stringify(renderer!.toJSON())).toContain(
    m.channelBindingUncertain(),
  );
});
it('binds the authorized WeCom credentials through the same server-side manual contract', async () => {
  api.createBinding.mockResolvedValue({
    ok: true,
    value: {
      operationId: 'op',
      channel: 'wecom',
      appId: 'wecom-bot',
      state: 'registered',
      status: 'BOUND',
    },
  });
  const changed = await mount();
  await act(async () =>
    renderer!.root
      .findAllByProps({ role: 'tab' })
      .find((tab) => tab.children.includes(m.channelProviderWecom()))!
      .props.onClick(),
  );
  await submit();
  expect(api.createBinding).toHaveBeenCalledWith(
    expect.objectContaining({
      connectionEpoch: 8,
      channel: 'wecom',
      mode: 'manual',
      botId: 'wecom-bot',
      secret: 'test-secret',
    }),
  );
  expect(changed).toHaveBeenCalledOnce();
});

const waiting = (operationId = 'op-feishu') => ({
  operationId,
  id: `qr-${operationId}`,
  channel: 'feishu',
  state: 'waiting',
  status: 'PENDING',
  pollAfterSeconds: 1,
  qrCodeImage: 'data:image/png;base64,test-qr',
});
function button(label: string) {
  return renderer!.root
    .findAllByType('button')
    .find((b) => b.children.includes(label))!;
}
async function clickTab(label: string) {
  const tab = renderer!.root
    .findAllByProps({ role: 'tab' })
    .find((t) => t.children.includes(label))!;
  expect(tab.props.disabled).toBe(false);
  await act(async () => tab.props.onClick());
}
it('keeps existing bots usable during QR waiting and resumes the same request after closing or changing provider', async () => {
  vi.useFakeTimers();
  api.createBinding.mockResolvedValue({ ok: true, value: waiting() });
  api.readBinding.mockResolvedValue({ ok: true, value: waiting() });
  const accounts = ['existing-a', 'existing-b'].map((id) => ({
    id,
    type: 'feishu',
    name: id,
    enabled: true,
    status: 'configured',
    domain: 'feishu',
  }));
  await mount(undefined, accounts);
  await submit();
  expect(button(m.cancel()).props.disabled).toBe(false);
  const picker = () =>
    renderer!.root.findByProps({ 'data-testid': 'channel-account-picker' });
  expect(picker().props.disabled).toBe(false);
  await act(async () => button(m.cancel()).props.onClick());
  expect(renderer!.root.findAllByType('img')).toHaveLength(0);
  await act(async () =>
    picker().props.onChange({ target: { value: 'feishu:existing-b' } }),
  );
  expect(
    renderer!.root.findByProps({
      'data-testid': 'channel-account-feishu-existing-b',
    }),
  ).toBeTruthy();
  await clickTab('Lark');
  expect(button(m.channelAddAccountTitle()).props.disabled).toBe(false);
  expect(renderer!.root.findAllByType('img')).toHaveLength(0);
  await clickTab(m.channelProviderFeishu());
  await act(async () => button(m.channelResumeBinding()).props.onClick());
  expect(renderer!.root.findByType('img').props.src).toBe(
    waiting().qrCodeImage,
  );
  await submit();
  expect(api.createBinding).toHaveBeenCalledOnce();
  await act(async () => {
    await vi.advanceTimersByTimeAsync(1000);
  });
  expect(api.readBinding).toHaveBeenLastCalledWith(
    expect.objectContaining({
      operationId: 'op-feishu',
      bindingId: 'qr-op-feishu',
      channel: 'feishu',
    }),
  );
});
it('retains unknown outcomes when navigating away and keeps Feishu and Lark requests separate', async () => {
  api.createBinding
    .mockResolvedValueOnce({
      ok: false,
      error: { code: 'outcomeUnknown', message: 'Unknown' },
    })
    .mockResolvedValueOnce({ ok: true, value: waiting('op-lark') });
  await mount();
  await submit();
  await clickTab('Lark');
  await submit();
  expect(api.createBinding).toHaveBeenLastCalledWith(
    expect.objectContaining({ domain: 'lark' }),
  );
  await clickTab(m.channelProviderFeishu());
  await act(async () => button(m.channelResumeBinding()).props.onClick());
  expect(JSON.stringify(renderer!.toJSON())).toContain(
    m.channelBindingUncertain(),
  );
  await submit();
  expect(api.createBinding).toHaveBeenCalledTimes(2);
  await clickTab('Lark');
  await act(async () => button(m.channelResumeBinding()).props.onClick());
  expect(JSON.stringify(renderer!.toJSON())).toContain('qr-op-lark');
  expect(JSON.stringify(renderer!.toJSON())).not.toContain(
    m.channelBindingUncertain(),
  );
});
it('ignores an old provider poll after navigation and resumes its original operation on return', async () => {
  vi.useFakeTimers();
  let resolve!: (value: unknown) => void;
  api.createBinding.mockResolvedValue({ ok: true, value: waiting() });
  api.readBinding.mockReturnValueOnce(
    new Promise((r) => {
      resolve = r;
    }),
  );
  const changed = await mount();
  await submit();
  await act(async () => {
    await vi.advanceTimersByTimeAsync(1000);
  });
  await clickTab('Lark');
  await act(async () =>
    resolve({
      ok: true,
      value: {
        ...waiting(),
        state: 'registered',
        status: 'BOUND',
        appId: 'new-bot',
      },
    }),
  );
  expect(changed).not.toHaveBeenCalled();
  expect(renderer!.root.findAllByType('img')).toHaveLength(0);
  api.readBinding.mockResolvedValue({
    ok: true,
    value: {
      ...waiting(),
      state: 'registered',
      status: 'BOUND',
      appId: 'new-bot',
    },
  });
  await clickTab(m.channelProviderFeishu());
  await act(async () => {
    await vi.advanceTimersByTimeAsync(1000);
  });
  expect(api.readBinding).toHaveBeenCalledTimes(2);
  expect(changed).toHaveBeenCalledOnce();
});
