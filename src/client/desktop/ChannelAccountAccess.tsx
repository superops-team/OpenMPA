import { useEffect, useRef, useState } from 'react';
import { RefreshCw, Plus, Trash2 } from 'lucide-react';
import { Channels, type Permission } from './generated/mygo';
import { unwrap } from './result';
import { useMessages, useMessageState } from './locale';
import type { useChannelManagementState } from './ChannelManagementState';
import { studioFailure } from './studio-services';

export function ChannelAccountAccess({
  state,
  disabled,
  revision,
}: {
  state: ReturnType<typeof useChannelManagementState>;
  disabled: boolean;
  revision?: number;
}) {
  const m = useMessages();
  const {
    account,
    selectedFeishu,
    accountRequest,
    chatId,
    setChatId,
    groupScope,
    setGroupScope,
    run,
  } = state;
  const [permissions, setPermissions] = useState<Permission[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useMessageState();
  const sequence = useRef(0);
  const refresh = async () => {
    const id = ++sequence.current;
    setLoading(true);
    setError('');
    try {
      const result = unwrap(await Channels.permissions(accountRequest()));
      if (id === sequence.current) setPermissions(result.permissions);
    } catch (reason) {
      if (id === sequence.current) setError(studioFailure(reason));
    } finally {
      if (id === sequence.current) setLoading(false);
    }
  };
  useEffect(() => {
    if (selectedFeishu) void refresh();
    return () => {
      sequence.current++;
    };
  }, [
    account?.id,
    account?.type,
    selectedFeishu,
    state.connection.epoch,
    revision,
  ]);
  const scopeLabels: Record<string, string> = {
    group: m.channelScopeGroup(),
    group_sender: m.channelScopeSender(),
    group_topic: m.channelScopeTopic(),
    group_topic_sender: m.channelScopeTopicSender(),
  };
  if (!selectedFeishu) return null;
  return (
    <section
      className="channel-groups"
      aria-label={m.channelGroupPermissions()}
      aria-busy={loading}
    >
      <header className="channel-section-heading">
        <h3>
          {m.channelGroupPermissions()}
          <span className="channel-count">
            {loading ? '…' : permissions.length}
          </span>
        </h3>
        <button
          type="button"
          className="channel-icon-button"
          aria-label={m.desktopReadFeishuGroupPermissions()}
          title={m.desktopReadFeishuGroupPermissions()}
          disabled={disabled || loading}
          onClick={() => void refresh()}
        >
          <RefreshCw size={15} aria-hidden="true" />
        </button>
      </header>
      <p className="channel-muted">{m.channelGroupHint()}</p>
      {error && (
        <p className="channel-inline-error" role="alert">
          {error}
        </p>
      )}
      {loading ? (
        <p role="status" className="channel-muted">
          {m.desktopLoading()}
        </p>
      ) : (
        !error &&
        !permissions.length && (
          <p className="channel-muted">{m.channelNoGroups()}</p>
        )
      )}
      {permissions.map((permission) => (
        <div className="channel-group-row" key={permission.chatId}>
          <div>
            <strong>{permission.chatName || permission.chatId}</strong>
            {permission.chatName && <code>{permission.chatId}</code>}
            <span>
              {scopeLabels[permission.groupSessionScope || 'group'] ||
                permission.groupSessionScope}
            </span>
          </div>
          <button
            type="button"
            className="channel-icon-button"
            aria-label={m.channelRemovePermissionConfirm(permission.chatId)}
            title={m.desktopRemoveGroupPermission()}
            disabled={disabled || loading}
            onClick={() => {
              if (
                !window.confirm(
                  m.channelRemovePermissionConfirm(permission.chatId),
                )
              )
                return;
              void run(async (current) => {
                unwrap(
                  await Channels.deletePermission({
                    ...accountRequest(),
                    chatId: permission.chatId,
                  }),
                );
                if (current()) await refresh();
              });
            }}
          >
            <Trash2 size={15} aria-hidden="true" />
          </button>
        </div>
      ))}
      <form
        className="channel-group-form"
        onSubmit={(event) => {
          event.preventDefault();
          if (disabled || !chatId.trim()) return;
          void run(async (current) => {
            unwrap(
              await Channels.savePermission({
                ...accountRequest(),
                chatId: chatId.trim(),
                groupSessionScope: groupScope,
              }),
            );
            if (current()) {
              setChatId('');
              await refresh();
            }
          });
        }}
      >
        <label className="channel-field">
          <span>{m.desktopGroupChatID()}</span>
          <input
            required
            value={chatId}
            placeholder="oc_…"
            disabled={disabled || loading}
            onChange={(event) => setChatId(event.target.value)}
          />
        </label>
        <button
          type="submit"
          className="channel-button"
          disabled={disabled || loading || !chatId.trim()}
        >
          <Plus size={15} aria-hidden="true" />
          {m.desktopSaveGroupPermission()}
        </button>
        <details className="channel-group-options">
          <summary>{m.desktopGroupSessionScope()}</summary>
          <label className="channel-field">
            <span>{m.desktopGroupSessionScope()}</span>
            <select
              value={groupScope}
              disabled={disabled}
              onChange={(event) => setGroupScope(event.target.value)}
            >
              {Object.entries(scopeLabels).map(([scope, label]) => (
                <option key={scope} value={scope}>
                  {label}
                </option>
              ))}
            </select>
          </label>
        </details>
      </form>
    </section>
  );
}
