import { useEffect, useState, type ReactNode } from 'react';
import { Plus, CheckCircle2, Bot } from 'lucide-react';
import { useMessages } from './locale';
import type { ChannelVM, ConnectionVM, SessionVM } from './models';
import { useChannelManagementState } from './ChannelManagementState';
import {
  ChannelManagementAccountForm,
  ChannelProviderTabs,
} from './ChannelManagementAccountForm';
import { ChannelAccountOverview } from './ChannelAccountOverview';
import { ChannelAccountAccess } from './ChannelAccountAccess';
import { ChannelAccountPrivate } from './ChannelAccountPrivate';
import { ChannelBindingPanel } from './ChannelBindingPanel';
import { ChannelBindingRecovery } from './ChannelBindingRecovery';

export function ChannelManagement({
  connection,
  accounts,
  sessions,
  onChanged,
  loading = false,
  loaded = true,
  externalBusy = false,
  revision = 0,
  onToggleAccount,
  onRemoveAccount,
  renderAccountTools,
  advancedContent,
}: {
  connection: ConnectionVM;
  accounts: ChannelVM[];
  sessions: SessionVM[];
  onChanged: () => Promise<void>;
  loading?: boolean;
  loaded?: boolean;
  externalBusy?: boolean;
  revision?: number;
  onToggleAccount?(account: ChannelVM): void;
  onRemoveAccount?(account: ChannelVM): void;
  renderAccountTools?(account: ChannelVM): ReactNode;
  advancedContent?: ReactNode;
}) {
  const m = useMessages();
  const state = useChannelManagementState({
    connection,
    accounts,
    sessions,
    onChanged,
  });
  const [setupKey, setSetupKey] = useState<string>();
  const adding = setupKey === state.providerKey;
  const setAdding = (value: boolean) =>
    setSetupKey(value ? state.providerKey : undefined);
  const {
    capabilities,
    account,
    selected,
    setSelected,
    generation,
    setBusy,
    setPrivateUser,
    binding,
    pendingBinding,
    uncertainBinding,
  } = state;
  const busy = state.busy || externalBusy || loading;
  const unresolved = pendingBinding || uncertainBinding;
  const providerName =
    state.channel === 'feishu'
      ? state.domain === 'lark'
        ? 'Lark'
        : m.channelProviderFeishu()
      : state.channel === 'dingtalk'
        ? m.channelProviderDingtalk()
        : m.channelProviderWecom();
  const showSetup = adding;
  useEffect(() => {
    setSetupKey(undefined);
  }, [state.providerKey]);
  useEffect(() => {
    if (binding?.state === 'registered' && account?.id === binding.appId)
      setAdding(false);
  }, [binding?.state, binding?.appId, account?.id]);
  const choose = (value: string) => {
    generation.current++;
    state.followBinding.current = false;
    setBusy(false);
    setSelected(value);
    setPrivateUser(undefined);
    setAdding(false);
    state.clearFeedback();
  };
  return (
    <section className="channel-workspace">
      <ChannelProviderTabs state={state} disabled={externalBusy || loading} />
      {capabilities && !capabilities.bindingReady && (
        <p role="status" className="channel-muted">
          {m.channelBindingNotReady()}
        </p>
      )}
      {state.error && (
        <p className="channel-inline-error" role="alert">
          {state.error}
        </p>
      )}
      {state.notice && (
        <p className="channel-notice" role="status">
          {state.notice}
        </p>
      )}
      <section className="channel-provider-panel" aria-label={providerName}>
        <header className="channel-section-heading">
          <h2>{providerName}</h2>
        </header>
        <div className="channel-account-picker">
          <label className="channel-field">
            <span>{m.desktopChannelAccount()}</span>
            <select
              value={selected}
              disabled={busy || !loaded}
              data-testid="channel-account-picker"
              onChange={(event) => choose(event.target.value)}
            >
              <option value="">{m.desktopChooseAnAccount()}</option>
              {state.accounts.map((item) => (
                <option
                  key={`${item.type}:${item.id}`}
                  value={`${item.type}:${item.id}`}
                >
                  {item.name || item.id} ·{' '}
                  {item.enabled ? m.pageEnabled() : m.pageDisabled()}
                </option>
              ))}
            </select>
          </label>
          <button
            type="button"
            className="channel-button is-primary"
            disabled={
              busy ||
              adding ||
              !loaded ||
              !capabilities?.serverSideBinding ||
              !capabilities.bindingReady ||
              !capabilities.ownerIdentityAvailable
            }
            onClick={() => {
              state.followBinding.current = true;
              if (!unresolved) {
                state.setBinding(undefined);
                state.setBindingOperation(undefined);
              }
              state.setCredentials({ appId: '', secret: '' });
              state.clearFeedback();
              setAdding(true);
            }}
          >
            <Plus size={16} aria-hidden="true" />
            {unresolved ? m.channelResumeBinding() : m.channelAddAccountTitle()}
          </button>
          {account &&
            !showSetup &&
            onToggleAccount &&
            account.type === 'feishu' && (
              <button
                type="button"
                className="channel-button"
                disabled={busy || !loaded}
                onClick={() => onToggleAccount(account)}
              >
                {account.enabled
                  ? m.desktopDisableAccount()
                  : m.desktopEnableAccount()}
              </button>
            )}
        </div>
        {accounts
          .filter(
            (item) =>
              item.addressable === false &&
              item.type === state.channel &&
              (item.type !== 'feishu' ||
                (item.domain || 'feishu') === state.domain),
          )
          .map((item) => (
            <p className="channel-muted" key={`${item.type}:${item.id}`}>
              {item.name || item.id} · {m.channelAccountIdUnavailable()}
            </p>
          ))}
        {loading && (
          <p role="status" className="channel-muted">
            {m.channelAccountsLoading()}
          </p>
        )}
        {!loading && !loaded && (
          <p className="channel-muted">{m.channelAccountsNotLoaded()}</p>
        )}
        {!loading && loaded && !state.accounts.length && !showSetup && (
          <div className="channel-empty">
            <Bot size={28} aria-hidden="true" />
            <p>{m.channelNoAccounts()}</p>
            <p>{m.channelAddHint()}</p>
          </div>
        )}
        {!loading &&
          loaded &&
          state.accounts.length > 0 &&
          !account &&
          !showSetup && (
            <p className="channel-muted">{m.desktopChooseAChannelAccount()}</p>
          )}
        {showSetup && (
          <section
            className="channel-setup"
            aria-label={m.channelAddAccountTitle()}
          >
            <header className="channel-section-heading">
              <h3>{m.channelAddAccountTitle()}</h3>
              <button
                type="button"
                className="channel-button is-quiet"
                disabled={state.busy}
                onClick={() => {
                  state.followBinding.current = false;
                  setAdding(false);
                  state.clearFeedback();
                  state.setCredentials({ appId: '', secret: '' });
                }}
              >
                {m.cancel()}
              </button>
            </header>
            {unresolved && (
              <p className="channel-muted">{m.channelBindingRetained()}</p>
            )}
            <ChannelManagementAccountForm
              {...state}
              disabled={externalBusy || loading || !loaded}
            />
            <ChannelBindingPanel state={state} />
          </section>
        )}
        {account && !showSetup && loaded && (
          <section
            className="channel-selected-account"
            data-testid={`channel-account-${account.type}-${account.id}`}
          >
            <div className="channel-bound-summary">
              <span className="channel-bound-badge">
                <CheckCircle2 size={16} aria-hidden="true" />
                {m.desktopBound()}
              </span>
              <div>
                <strong>{account.name || account.id}</strong>
                <code>{account.id}</code>
              </div>
              {onRemoveAccount && (
                <button
                  type="button"
                  className="channel-button is-quiet"
                  disabled={busy}
                  onClick={() => onRemoveAccount(account)}
                >
                  {m.channelUnbindAccount()}
                </button>
              )}
            </div>
            <ChannelAccountOverview
              key={`${connection.epoch}:${selected}`}
              account={account}
              epoch={connection.epoch}
              revision={revision}
            />
            <ChannelAccountAccess
              key={`access:${connection.epoch}:${selected}`}
              state={state}
              disabled={busy || !loaded}
              revision={revision}
            />
          </section>
        )}
        <details className="channel-advanced">
          <summary>{m.channelAdvancedSettings()}</summary>
          <div className="channel-advanced-body">
            {account && loaded && (
              <>
                {renderAccountTools?.(account)}
                {state.selectedFeishu && (
                  <ChannelAccountPrivate
                    state={state}
                    disabled={externalBusy || loading}
                  />
                )}
              </>
            )}
            <ChannelBindingRecovery state={state} />
            {advancedContent}
          </div>
        </details>
      </section>
    </section>
  );
}
