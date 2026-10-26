import { useEffect, useRef, useState } from 'react';
import {
  Channels,
  type Binding,
  type Capabilities,
  type Permission,
  type PrivateUser,
} from './generated/mygo';
import { unwrap } from './result';
import type { ChannelVM, ConnectionVM, SessionVM } from './models';
export function useChannelManagementState({
  connection,
  accounts,
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
  const [binding, setBinding] = useState<Binding>();
  const [recovery, setRecovery] = useState({ operationId: '', bindingId: '' });
  const [selected, setSelected] = useState('');
  const [permissions, setPermissions] = useState<Permission[]>([]);
  const [chatId, setChatId] = useState('');
  const [groupScope, setGroupScope] = useState('group');
  const [privateUser, setPrivateUser] = useState<PrivateUser>();
  const [sessionId, setSessionId] = useState('');
  const [notice, setNotice] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [bindingOperation, setBindingOperation] = useState<string>();
  const generation = useRef(0);
  const account = accounts.find(
    (account) => `${account.type}:${account.id}` === selected,
  );
  useEffect(() => {
    const current = ++generation.current;
    void Channels.capabilities({ connectionEpoch: connection.epoch })
      .then((result) => {
        if (generation.current === current) {
          const value = unwrap(result);
          setCapabilities(value);
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
        if (generation.current === current)
          setError(
            error instanceof Error
              ? error.message
              : 'Channel capabilities could not be read.',
          );
      });
    return () => {
      generation.current++;
    };
  }, [connection.epoch]);
  const run = async (action: (current: () => boolean) => Promise<void>) => {
    const current = generation.current;
    const matches = () => generation.current === current;
    setBusy(true);
    setError('');
    try {
      await action(matches);
    } catch (error) {
      if (matches())
        setError(
          error instanceof Error ? error.message : 'Channel operation failed.',
        );
    } finally {
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
      !binding?.id ||
      !['waiting', 'pending', 'registering', 'PENDING', 'REGISTERING'].includes(
        binding.state,
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
              if (value.state.toLowerCase() === 'registered') void onChanged();
            }
          })
          .catch((error: unknown) => {
            if (active)
              setError(
                error instanceof Error
                  ? error.message
                  : 'Binding status could not be read.',
              );
          });
      },
      Math.max(1, binding.pollAfterSeconds || 3) * 1000,
    );
    return () => {
      active = false;
      clearTimeout(timer);
    };
  }, [binding, connection.epoch]);
  const accountRequest = () => {
    if (!account) throw new Error('Choose a channel account.');
    return {
      connectionEpoch: connection.epoch,
      channel: account.type,
      appId: account.id,
    };
  };
  const selectedFeishu = account?.type.toLowerCase() === 'feishu';
  return {
    capabilities,
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
    readBinding,
    onChanged,
    recovery,
    selected,
    generation,
    setBusy,
    setSelected,
    setPermissions,
    setPrivateUser,
    accounts,
    account,
    selectedFeishu,
    accountRequest,
    groupScope,
    setChatId,
    chatId,
    setGroupScope,
    permissions,
    privateUser,
    sessionId,
    setSessionId,
    sessions,
  };
}
