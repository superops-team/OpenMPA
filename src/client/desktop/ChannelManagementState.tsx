import { localized, useMessageState } from './locale';
import {
  prepareWecomAuthorization,
  WecomAuthorizationError,
} from './wecom-authorization';
import { useEffect, useRef, useState } from 'react';
import {
  Channels,
  type Binding,
  type Capabilities,
  type PrivateUser,
} from './generated/mygo';
import { unwrap } from './result';
import type { ChannelVM, ConnectionVM, SessionVM } from './models';
import type { Messages } from './generated/locales';

type BindingRecord = {
  binding?: Binding;
  operationId?: string;
  recovery?: { operationId: string; bindingId: string };
};

const accountRequired = localized((m) => m.desktopChooseAChannelAccount());

export function channelStatusLabel(status: string, m: Messages): string {
  const labels: Record<string, string> = {
    configured: m.channelConfigured(),
    connected: m.runtimeAlreadyConnected(),
    disconnected: m.channelDisconnected(),
    waiting: m.channelWaiting(),
    pending: m.pagePending(),
    registering: m.channelRegistering(),
    registered: m.channelRegistered(),
    bound: m.channelRegistered(),
    scanned: m.channelScanned(),
    expired: m.pageExpired(),
    failed: m.desktopFailed(),
    unknown: m.desktopUnknown(),
    authorized: m.channelAuthorized(),
    revoked: m.channelRevoked(),
    unauthorized: m.channelUnauthorized(),
  };
  const key = status.toLowerCase();
  return Object.hasOwn(labels, key) ? labels[key] : status;
}
export function useChannelManagementState({
  connection,
  accounts: allAccounts,
  sessions,
  onChanged,
}: {
  connection: ConnectionVM;
  accounts: ChannelVM[];
  sessions: SessionVM[];
  onChanged: () => Promise<void>;
}) {
  const [capabilities, setCapabilities] = useState<Capabilities>();
  const [channel, setChannel] = useState('feishu');
  const [mode, setMode] = useState('qr');
  const [domain, setDomain] = useState('feishu');
  const [bindingType, setBindingType] = useState('shared');
  const [credentials, setCredentials] = useState({ appId: '', secret: '' });
  // Feishu and Lark use the same API channel but have separate binding flows.
  const providerKey = `${connection.epoch}:${channel}:${domain}`;
  const [records, setRecords] = useState<{
    epoch: number;
    values: Record<string, BindingRecord>;
  }>({ epoch: connection.epoch, values: {} });
  const record =
    records.epoch === connection.epoch
      ? records.values[providerKey]
      : undefined;
  const binding = record?.binding;
  const bindingOperation = record?.operationId;
  const recovery = record?.recovery || { operationId: '', bindingId: '' };
  const updateRecord = (patch: Partial<BindingRecord>) => {
    setRecords((previous) => {
      const values = previous.epoch === connection.epoch ? previous.values : {};
      return {
        epoch: connection.epoch,
        values: {
          ...values,
          [providerKey]: { ...values[providerKey], ...patch },
        },
      };
    });
  };
  const setBinding = (value: Binding | undefined) =>
    updateRecord({ binding: value });
  const setBindingOperation = (value: string | undefined) =>
    updateRecord({ operationId: value });
  const setRecovery = (value: { operationId: string; bindingId: string }) =>
    updateRecord({ recovery: value });
  const [selected, setSelected] = useState('');
  const [chatId, setChatId] = useState('');
  const [groupScope, setGroupScope] = useState('group');
  const [privateUser, setPrivateUser] = useState<PrivateUser>();
  const [sessionId, setSessionId] = useState('');
  const [notice, setNotice] = useMessageState();
  const [error, setError] = useMessageState();
  const [busy, setBusy] = useState(false);
  const generation = useRef(0);
  const followBinding = useRef(false);
  const runPending = useRef(false);
  const [wecomReady, setWecomReady] = useState(false);
  const authorization = useRef<AbortController | null>(null);
  useEffect(() => {
    if (channel !== 'wecom') return;
    let active = true;
    void prepareWecomAuthorization()
      .then(() => {
        if (active) setWecomReady(true);
      })
      .catch(() => {
        if (active) setError(localized((m) => m.channelWecomAuthUnavailable()));
      });
    return () => {
      active = false;
      authorization.current?.abort();
    };
  }, [channel]);
  useEffect(() => () => authorization.current?.abort(), []);

  const accounts = allAccounts.filter(
    (item) =>
      item.type === channel &&
      item.addressable !== false &&
      (channel !== 'feishu' || (item.domain || 'feishu') === domain),
  );
  useEffect(() => {
    setSelected((previous) =>
      accounts.some((item) => `${item.type}:${item.id}` === previous)
        ? previous
        : accounts.length === 1
          ? `${accounts[0].type}:${accounts[0].id}`
          : '',
    );
  }, [allAccounts, channel, domain]);
  useEffect(() => {
    setPrivateUser(undefined);
  }, [selected]);
  const account = accounts.find(
    (account) => `${account.type}:${account.id}` === selected,
  );
  useEffect(() => {
    let active = true;
    void Channels.capabilities({ connectionEpoch: connection.epoch })
      .then((result) => {
        if (active) {
          const value = unwrap(result);
          setCapabilities(value);
          setChannel((previous) =>
            value.channels.includes(previous)
              ? previous
              : value.channels[0] || '',
          );
          setDomain((previous) =>
            value.feishuDomains.includes(previous)
              ? previous
              : value.feishuDomains[0] || 'feishu',
          );
          setBindingType((previous) =>
            value.botBindingTypes.includes(previous)
              ? previous
              : value.botBindingTypes.includes('shared')
                ? 'shared'
                : value.botBindingTypes[0] || '',
          );
        }
      })
      .catch((error: unknown) => {
        if (active)
          setError(
            error instanceof Error
              ? error.message
              : localized((m) => m.desktopChannelCapabilitiesCouldNotBeRead()),
          );
      });
    return () => {
      active = false;
      generation.current++;
    };
  }, [connection.epoch]);
  const run = async (action: (current: () => boolean) => Promise<void>) => {
    if (runPending.current) return;
    runPending.current = true;
    const current = generation.current;
    const matches = () => generation.current === current;
    setBusy(true);
    setError('');
    try {
      await action(matches);
    } catch (error) {
      if (matches())
        setError(
          error instanceof WecomAuthorizationError
            ? localized((m) => m.channelWecomAuthUnavailable())
            : error instanceof Error
              ? error.message
              : error === accountRequired
                ? accountRequired
                : localized((m) => m.desktopChannelOperationFailed()),
        );
    } finally {
      runPending.current = false;
      if (matches()) setBusy(false);
    }
  };
  const readBinding = async (value: Binding, resumeConfirmed = false) => {
    const result = unwrap(
      await Channels.readBinding({
        connectionEpoch: connection.epoch,
        operationId: value.operationId,
        channel: value.channel,
        bindingId: value.id || '',
        resumeConfirmed,
      }),
    );
    return result;
  };
  useEffect(() => {
    if (
      busy ||
      !binding?.id ||
      !['waiting', 'pending', 'registering'].includes(
        binding.state.toLowerCase(),
      )
    )
      return;
    let active = true;
    const timer = setTimeout(
      () => {
        void readBinding(binding)
          .then((value) => {
            if (active) {
              setBinding(value);
              if (value.state.toLowerCase() === 'registered') {
                setBindingOperation(undefined);
                setNotice(localized((m) => m.desktopBound()));
                if (followBinding.current && value.appId)
                  setSelected(`${value.channel}:${value.appId}`);
                void onChanged().catch(() => {
                  if (active)
                    setError(localized((m) => m.desktopCouldNotReadChannels()));
                });
              }
            }
          })
          .catch((error: unknown) => {
            if (active)
              setError(
                error instanceof Error
                  ? error.message
                  : localized((m) => m.desktopBindingStatusCouldNotBeRead()),
              );
          });
      },
      Math.max(1, binding.pollAfterSeconds || 3) * 1000,
    );
    return () => {
      active = false;
      clearTimeout(timer);
    };
  }, [binding, connection.epoch, busy]);
  const accountRequest = () => {
    if (!account) throw accountRequired;
    return {
      connectionEpoch: connection.epoch,
      channel: account.type,
      appId: account.id,
    };
  };
  const selectedFeishu = account?.type.toLowerCase() === 'feishu';
  return {
    capabilities,
    providerKey,
    followBinding,
    clearFeedback: () => {
      setError('');
      setNotice('');
    },
    wecomReady,
    authorization,
    error,
    notice,
    run,
    setBindingOperation,
    connection,
    channel,
    mode,
    credentials,
    setBinding,
    setRecovery,
    setCredentials,
    setNotice,
    setChannel,
    setMode,
    domain,
    setDomain,
    bindingType,
    setBindingType,
    busy,
    bindingOperation,
    binding,
    pendingBinding:
      !!binding &&
      ['waiting', 'pending', 'registering', 'unknown'].includes(
        binding.state.toLowerCase(),
      ),
    uncertainBinding:
      !!bindingOperation && (!binding || binding.state === 'unknown'),
    readBinding,
    onChanged,
    recovery,
    selected,
    generation,
    setBusy,
    setSelected,
    setPrivateUser,
    accounts,
    account,
    selectedFeishu,
    accountRequest,
    groupScope,
    setChatId,
    chatId,
    setGroupScope,
    privateUser,
    sessionId,
    setSessionId,
    sessions,
  };
}
