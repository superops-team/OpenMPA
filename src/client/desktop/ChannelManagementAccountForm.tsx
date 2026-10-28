import { localized, useMessages } from './locale';
import { Channels } from './generated/mygo';
import { authorizeWecom } from './wecom-authorization';
import { unwrap } from './result';

import type { useChannelManagementState } from './ChannelManagementState';
export function ChannelManagementAccountForm(
  props: ReturnType<typeof useChannelManagementState> & { disabled?: boolean },
) {
  const m = useMessages();
  const {
    run,
    setBindingOperation,
    connection,
    channel,
    mode,
    domain,
    bindingType,
    credentials,
    setBinding,
    setRecovery,
    setSelected,
    setCredentials,
    setNotice,
    setMode,
    capabilities,
    setBindingType,
    busy: pending,
    pendingBinding,
    uncertainBinding,
    wecomReady,
    authorization,
    onChanged,
  } = props;
  const busy = pending || props.disabled;
  return (
    <form
      data-testid="channel-binding-form"
      className="desktop-form"
      onSubmit={(event) => {
        event.preventDefault();
        if (
          busy ||
          !capabilities?.serverSideBinding ||
          !capabilities.bindingReady ||
          !capabilities.channels.includes(channel) ||
          !capabilities.ownerIdentityAvailable ||
          uncertainBinding ||
          pendingBinding
        )
          return;
        void run(async (current) => {
          let suppliedCredentials = credentials;
          if (channel === 'wecom' && mode === 'qr') {
            const controller = new AbortController();
            authorization.current = controller;
            try {
              const result = await authorizeWecom(controller.signal);
              suppliedCredentials = {
                appId: result.botId,
                secret: result.secret,
              };
            } finally {
              authorization.current = null;
            }
            if (!current()) return;
          }
          const operationId = crypto.randomUUID();
          setBindingOperation(operationId);
          setBinding(undefined);
          const result = await Channels.createBinding({
            connectionEpoch: connection.epoch,
            operationId,
            channel,
            mode: channel === 'wecom' ? 'manual' : mode,
            ...(channel.toLowerCase() === 'feishu'
              ? { domain, bindingType }
              : {}),
            ...(mode === 'manual' || channel === 'wecom'
              ? channel.toLowerCase() === 'feishu'
                ? {
                    appId: suppliedCredentials.appId,
                    appSecret: suppliedCredentials.secret,
                  }
                : channel.toLowerCase() === 'dingtalk'
                  ? {
                      clientId: suppliedCredentials.appId,
                      clientSecret: suppliedCredentials.secret,
                    }
                  : {
                      botId: suppliedCredentials.appId,
                      secret: suppliedCredentials.secret,
                    }
              : {}),
          });
          if (
            current() &&
            !result.ok &&
            result.error?.code !== 'outcomeUnknown' &&
            result.error?.code !== 'registrationUncertain'
          )
            setBindingOperation(undefined);
          const value = unwrap(result);
          if (current()) {
            setBinding(value);
            setRecovery({
              operationId: value.operationId,
              bindingId: value.id || '',
            });
            setCredentials({ appId: '', secret: '' });
            if (value.state === 'registered') {
              if (value.appId) setSelected(`${value.channel}:${value.appId}`);
              setBindingOperation(undefined);
              await onChanged();
            }
            setNotice(
              localized((m) =>
                value.state === 'registered'
                  ? m.desktopBound()
                  : m.channelWaiting(),
              ),
            );
          }
        });
      }}
    >
      <label>
        {m.desktopBindingMethod()}
        <select
          value={mode}
          disabled={busy || pendingBinding || uncertainBinding}
          onChange={(event) => setMode(event.target.value)}
        >
          <option value="qr">
            {channel === 'wecom'
              ? m.channelWecomAuthorize()
              : m.desktopScanQRCode()}
          </option>
          {(channel === 'wecom' ||
            capabilities?.credentialBindingChannels?.includes(channel)) && (
            <option value="manual">{m.desktopManualCredentials()}</option>
          )}
        </select>
      </label>
      {!capabilities?.bindingReady && (
        <p role="status">{m.channelBindingNotReady()}</p>
      )}
      {uncertainBinding && <p role="alert">{m.channelBindingUncertain()}</p>}
      {channel.toLowerCase() === 'feishu' && (
        <>
          <label>
            {m.desktopBotBindingType()}
            <select
              value={bindingType}
              disabled={busy || pendingBinding || uncertainBinding}
              onChange={(event) => setBindingType(event.target.value)}
            >
              {(capabilities?.botBindingTypes.length
                ? capabilities.botBindingTypes
                : ['shared', 'private']
              ).map((name) => (
                <option key={name} value={name}>
                  {name === 'shared'
                    ? m.channelSharedBinding()
                    : name === 'private'
                      ? m.channelPrivateBinding()
                      : name}
                </option>
              ))}
            </select>
          </label>
        </>
      )}
      {mode === 'manual' && (
        <>
          <label>
            {channel.toLowerCase() === 'dingtalk'
              ? m.desktopClientID()
              : channel.toLowerCase() === 'wecom'
                ? m.desktopBotID()
                : m.desktopAppID()}
            <input
              required
              autoComplete="off"
              value={credentials.appId}
              onChange={(event) =>
                setCredentials({
                  ...credentials,
                  appId: event.target.value,
                })
              }
            />
          </label>
          <label>
            {m.desktopProviderSecret()}
            <input
              required
              type="password"
              autoComplete="off"
              value={credentials.secret}
              onChange={(event) =>
                setCredentials({
                  ...credentials,
                  secret: event.target.value,
                })
              }
            />
          </label>
        </>
      )}
      {busy && authorization.current && (
        <button
          type="button"
          className="text-button"
          onClick={() => authorization.current?.abort()}
        >
          {m.cancel()}
        </button>
      )}
      <button
        className="primary"
        disabled={
          busy ||
          !capabilities?.ownerIdentityAvailable ||
          !capabilities?.bindingReady ||
          !capabilities?.serverSideBinding ||
          uncertainBinding ||
          pendingBinding ||
          (channel === 'wecom' && mode === 'qr' && !wecomReady)
        }
      >
        {m.desktopStartBinding()}
      </button>
    </form>
  );
}

export function ChannelProviderTabs({
  state,
  disabled = false,
}: {
  state: ReturnType<typeof useChannelManagementState>;
  disabled?: boolean;
}) {
  const m = useMessages();
  const {
    capabilities,
    channel,
    domain,
    busy,
    generation,
    setChannel,
    setDomain,
    setMode,
    setCredentials,
  } = state;
  return (
    <div
      className="studio-channel-tabs"
      role="tablist"
      aria-label={m.desktopChannel()}
      onKeyDown={(event) => {
        if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key))
          return;
        const items = Array.from(
          event.currentTarget.querySelectorAll<HTMLButtonElement>(
            'button[role="tab"]:not(:disabled)',
          ),
        );
        if (!items.length) return;
        const index = items.indexOf(event.target as HTMLButtonElement);
        const next =
          event.key === 'Home'
            ? 0
            : event.key === 'End'
              ? items.length - 1
              : (index + (event.key === 'ArrowRight' ? 1 : -1) + items.length) %
                items.length;
        event.preventDefault();
        items[next].focus();
        items[next].click();
      }}
    >
      {(capabilities?.channels || ['feishu', 'dingtalk', 'wecom'])
        .flatMap((name) =>
          name === 'feishu' && capabilities?.feishuDomains.includes('lark')
            ? ['feishu', 'lark']
            : [name],
        )
        .map((provider) => (
          <button
            key={provider}
            type="button"
            role="tab"
            aria-selected={
              provider === 'lark'
                ? channel === 'feishu' && domain === 'lark'
                : channel === provider &&
                  (provider !== 'feishu' || domain !== 'lark')
            }
            className="text-button"
            disabled={disabled || busy}
            onClick={() => {
              generation.current++;
              state.followBinding.current = false;
              state.clearFeedback();
              setChannel(provider === 'lark' ? 'feishu' : provider);
              setDomain(provider === 'lark' ? 'lark' : 'feishu');
              setMode('qr');
              setCredentials({ appId: '', secret: '' });
            }}
          >
            {provider === 'feishu'
              ? m.channelProviderFeishu()
              : provider === 'lark'
                ? 'Lark'
                : provider === 'dingtalk'
                  ? m.channelProviderDingtalk()
                  : m.channelProviderWecom()}
          </button>
        ))}
    </div>
  );
}
