import { Channels } from './generated/mygo';
import { unwrap } from './result';

import type { useChannelManagementState } from './ChannelManagementState';
export function ChannelManagementAccountForm(
  props: ReturnType<typeof useChannelManagementState>,
) {
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
    setCredentials,
    setNotice,
    setChannel,
    setMode,
    capabilities,
    setDomain,
    setBindingType,
    busy,
    bindingOperation,
    binding,
  } = props;
  return (
    <form
      className="desktop-form"
      onSubmit={(event) => {
        event.preventDefault();
        void run(async (current) => {
          const operationId = crypto.randomUUID();
          setBindingOperation(operationId);
          const value = unwrap(
            await Channels.createBinding({
              connectionEpoch: connection.epoch,
              operationId,
              channel,
              mode,
              ...(channel.toLowerCase() === 'feishu'
                ? { domain, bindingType }
                : {}),
              ...(mode === 'manual'
                ? channel.toLowerCase() === 'feishu'
                  ? {
                      appId: credentials.appId,
                      appSecret: credentials.secret,
                    }
                  : channel.toLowerCase() === 'dingtalk'
                    ? {
                        clientId: credentials.appId,
                        clientSecret: credentials.secret,
                      }
                    : {
                        botId: credentials.appId,
                        secret: credentials.secret,
                      }
                : {}),
            }),
          );
          if (current()) {
            setBinding(value);
            setRecovery({
              operationId: value.operationId,
              bindingId: value.id || '',
            });
            setCredentials({ appId: '', secret: '' });
            setNotice('Binding submitted. Credential inputs cleared.');
          }
        });
      }}
    >
      <label>
        Channel
        <select
          value={channel}
          onChange={(event) => {
            setChannel(event.target.value);
            setMode(
              event.target.value.toLowerCase() === 'wecom' ? 'manual' : 'qr',
            );
          }}
        >
          {(capabilities?.channels || ['feishu', 'dingtalk', 'wecom']).map(
            (name) => (
              <option key={name} value={name}>
                {name}
              </option>
            ),
          )}
        </select>
      </label>
      <label>
        Binding method
        <select value={mode} onChange={(event) => setMode(event.target.value)}>
          {channel.toLowerCase() !== 'wecom' && (
            <option value="qr">Scan QR code</option>
          )}
          <option value="manual">Manual credentials</option>
        </select>
      </label>
      {channel.toLowerCase() === 'feishu' && (
        <>
          <label>
            Domain
            <select
              value={domain}
              onChange={(event) => setDomain(event.target.value)}
            >
              {(capabilities?.feishuDomains.length
                ? capabilities.feishuDomains
                : ['feishu', 'lark']
              ).map((name) => (
                <option key={name}>{name}</option>
              ))}
            </select>
          </label>
          <label>
            Bot binding type
            <select
              value={bindingType}
              onChange={(event) => setBindingType(event.target.value)}
            >
              {(capabilities?.botBindingTypes.length
                ? capabilities.botBindingTypes
                : ['shared', 'private']
              ).map((name) => (
                <option key={name}>{name}</option>
              ))}
            </select>
          </label>
        </>
      )}
      {mode === 'manual' && (
        <>
          <label>
            {channel.toLowerCase() === 'dingtalk'
              ? 'Client ID'
              : channel.toLowerCase() === 'wecom'
                ? 'Bot ID'
                : 'App ID'}
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
            Provider secret
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
      <button
        className="primary"
        disabled={
          busy ||
          !capabilities?.ownerIdentityAvailable ||
          (!!bindingOperation && !binding?.id)
        }
      >
        Start binding
      </button>
    </form>
  );
}
