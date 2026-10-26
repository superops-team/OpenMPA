import { Channels } from './generated/mygo';
import { unwrap } from './result';
import type { ChannelVM, ConnectionVM, SessionVM } from './models';
import { useChannelManagementState } from './ChannelManagementState';
import { ChannelManagementAccountForm } from './ChannelManagementAccountForm';
export function ChannelManagement({
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
  const state = useChannelManagementState({
    connection,
    accounts,
    sessions,
    onChanged,
  });
  const {
    capabilities,
    error,
    notice,
    bindingOperation,
    binding,
    busy,
    run,
    readBinding,
    setBinding,
    channel,
    recovery,
    setRecovery,
    selected,
    generation,
    setBusy,
    setSelected,
    setPermissions,
    setPrivateUser,
    account,
    selectedFeishu,
    accountRequest,
    chatId,
    groupScope,
    setChatId,
    setGroupScope,
    permissions,
    privateUser,
    sessionId,
    setNotice,
    setSessionId,
  } = state;
  return (
    <>
      <section className="task-detail-card desktop-form">
        <h2>Connect a channel account</h2>
        {capabilities && (
          <p>
            Management scope: {capabilities.managementScope} · Owner actions:{' '}
            {capabilities.ownerIdentityAvailable
              ? 'verified identity available'
              : 'verified owner identity required'}{' '}
            · Configuration CAS:{' '}
            {capabilities.configHasCas ? 'available' : 'unavailable'}
          </p>
        )}
        {error && <p role="alert">{error}</p>}
        {notice && <p role="status">{notice}</p>}
        <ChannelManagementAccountForm {...state} />
        {bindingOperation && !binding?.id && (
          <p role="status">
            Binding operation {bindingOperation} needs verification. Preserve
            this ID and resume the server binding before starting another
            operation.
          </p>
        )}
        {binding && (
          <div className="document-save-notice">
            <h3>Binding {binding.id || 'awaiting server ID'}</h3>
            <p>
              Operation {binding.operationId} · {binding.status} ·{' '}
              {binding.state}
            </p>
            {binding.qrCodeImage &&
              ['waiting', 'pending', 'registering'].includes(
                binding.state.toLowerCase(),
              ) && (
                <img
                  className="desktop-binding-qr"
                  src={binding.qrCodeImage}
                  alt="Scan this code to authorize the selected channel account"
                />
              )}
            {binding.expiresAt && <p>Expires {binding.expiresAt}</p>}
            {binding.errorCode && <p role="alert">{binding.errorCode}</p>}
            <button
              className="text-button"
              disabled={busy || !binding.id}
              onClick={() =>
                void run(async (current) => {
                  const value = await readBinding(binding);
                  if (current()) setBinding(value);
                  await onChanged();
                })
              }
            >
              Read binding status
            </button>
            <button
              className="text-button"
              disabled={busy || !binding.id}
              onClick={() => {
                const uncertain = binding.state.toLowerCase() === 'unknown';
                if (
                  uncertain &&
                  !window.confirm(
                    'Verify the gateway connection before explicitly retrying this same binding. Retry with the same operation and binding IDs?',
                  )
                )
                  return;
                void run(async (current) => {
                  const value = unwrap(
                    await Channels.retryBinding({
                      connectionEpoch: connection.epoch,
                      channel: binding.channel,
                      bindingId: binding.id || '',
                      operationId: binding.operationId,
                      gatewayVerified: uncertain,
                    }),
                  );
                  if (current()) setBinding(value);
                });
              }}
            >
              Retry this binding
            </button>
          </div>
        )}
        <form
          className="desktop-form"
          onSubmit={(event) => {
            event.preventDefault();
            if (
              !window.confirm(
                'Resume reading this previously initiated binding operation?',
              )
            )
              return;
            void run(async (current) => {
              const value = unwrap(
                await Channels.readBinding({
                  connectionEpoch: connection.epoch,
                  channel,
                  operationId: recovery.operationId,
                  bindingId: recovery.bindingId,
                  resumeConfirmed: true,
                }),
              );
              if (current()) setBinding(value);
            });
          }}
        >
          <h3>Resume a binding</h3>
          <label>
            Operation ID
            <input
              required
              value={recovery.operationId}
              onChange={(event) =>
                setRecovery({ ...recovery, operationId: event.target.value })
              }
            />
          </label>
          <label>
            Binding ID
            <input
              required
              value={recovery.bindingId}
              onChange={(event) =>
                setRecovery({ ...recovery, bindingId: event.target.value })
              }
            />
          </label>
          <button className="text-button" disabled={busy}>
            Read and resume
          </button>
        </form>
      </section>
      <section className="task-detail-card desktop-form">
        <h2>Account permissions and private user</h2>
        <label>
          Channel account
          <select
            value={selected}
            onChange={(event) => {
              generation.current++;
              setBusy(false);
              setSelected(event.target.value);
              setPermissions([]);
              setPrivateUser(undefined);
            }}
          >
            <option value="">Choose an account</option>
            {accounts.map((account) => (
              <option
                key={`${account.type}:${account.id}`}
                value={`${account.type}:${account.id}`}
              >
                {account.name} · {account.type} · {account.id}
              </option>
            ))}
          </select>
        </label>
        {account && (
          <>
            <div className="desktop-inline">
              <button
                className="text-button"
                disabled={busy || !selectedFeishu}
                onClick={() =>
                  void run(async (current) => {
                    const value = unwrap(
                      await Channels.permissions(accountRequest()),
                    );
                    if (current()) setPermissions(value.permissions);
                  })
                }
              >
                Read Feishu group permissions
              </button>
              <button
                className="text-button"
                disabled={
                  busy ||
                  !selectedFeishu ||
                  !capabilities?.ownerIdentityAvailable
                }
                onClick={() =>
                  void run(async (current) => {
                    const value = unwrap(
                      await Channels.privateUser(accountRequest()),
                    );
                    if (current()) setPrivateUser(value);
                  })
                }
              >
                Read private user and slot
              </button>
            </div>
            {selectedFeishu && (
              <form
                className="desktop-form"
                onSubmit={(event) => {
                  event.preventDefault();
                  void run(async (current) => {
                    unwrap(
                      await Channels.savePermission({
                        ...accountRequest(),
                        chatId,
                        groupSessionScope: groupScope,
                      }),
                    );
                    const value = unwrap(
                      await Channels.permissions(accountRequest()),
                    );
                    if (current()) {
                      setPermissions(value.permissions);
                      setChatId('');
                    }
                  });
                }}
              >
                <label>
                  Group chat ID
                  <input
                    required
                    value={chatId}
                    onChange={(event) => setChatId(event.target.value)}
                  />
                </label>
                <label>
                  Group session scope
                  <select
                    value={groupScope}
                    onChange={(event) => setGroupScope(event.target.value)}
                  >
                    {[
                      'group',
                      'group_sender',
                      'group_topic',
                      'group_topic_sender',
                    ].map((scope) => (
                      <option key={scope}>{scope}</option>
                    ))}
                  </select>
                </label>
                <button className="primary" disabled={busy}>
                  Save group permission
                </button>
              </form>
            )}
            {permissions.map((permission) => (
              <article key={permission.chatId}>
                <strong>{permission.chatName || permission.chatId}</strong>
                <p>{permission.groupSessionScope}</p>
                <button
                  className="text-button"
                  disabled={busy}
                  onClick={() => {
                    if (
                      !window.confirm(
                        `Remove group permission for ${permission.chatId}?`,
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
                      const value = unwrap(
                        await Channels.permissions(accountRequest()),
                      );
                      if (current()) setPermissions(value.permissions);
                    });
                  }}
                >
                  Remove group permission
                </button>
              </article>
            ))}
            {privateUser && (
              <div className="document-save-notice">
                <h3>Private user authorization</h3>
                <p>
                  Authorization {privateUser.authorization.status} · Slot
                  version {privateUser.slot?.version || 'No slot'} · User{' '}
                  {privateUser.slot?.runtimeUserId || 'Unbound'}
                </p>
                <button
                  disabled={busy || !privateUser.slot}
                  onClick={() => {
                    const expectedVersion = privateUser.slot?.version;
                    if (
                      !expectedVersion ||
                      !window.confirm(
                        `Revoke private user authorization at slot version ${expectedVersion}?`,
                      )
                    )
                      return;
                    void run(async (current) => {
                      const value = unwrap(
                        await Channels.revokePrivateAuthorization({
                          ...accountRequest(),
                          expectedVersion,
                        }),
                      );
                      if (current()) setPrivateUser(value.current);
                    });
                  }}
                >
                  Revoke authorization
                </button>
                <button
                  disabled={busy || !privateUser.slot}
                  onClick={() => {
                    const expectedVersion = privateUser.slot?.version;
                    if (
                      !expectedVersion ||
                      !window.confirm(
                        `Reset the private user binding at slot version ${expectedVersion}?`,
                      )
                    )
                      return;
                    void run(async (current) => {
                      const value = unwrap(
                        await Channels.resetPrivateUser({
                          ...accountRequest(),
                          expectedVersion,
                        }),
                      );
                      if (current()) setPrivateUser(value.current);
                    });
                  }}
                >
                  Reset private user
                </button>
              </div>
            )}
            <form
              className="desktop-form"
              onSubmit={(event) => {
                event.preventDefault();
                if (
                  !window.confirm(
                    `Upgrade the existing conversation ${sessionId} to the account profile? This operation has no server CAS and does not advance its execution version.`,
                  )
                )
                  return;
                void run(async (current) => {
                  const value = unwrap(
                    await Channels.upgradeProfile({
                      ...accountRequest(),
                      sessionId,
                    }),
                  );
                  if (current())
                    setNotice(
                      `Profile upgraded for ${value.sessionId}. Execution-version advance: ${value.advancesExecutionVersion}. Read its current configuration before the next turn.`,
                    );
                });
              }}
            >
              <label>
                Existing conversation for profile upgrade
                <select
                  required
                  value={sessionId}
                  onChange={(event) => setSessionId(event.target.value)}
                >
                  <option value="">Choose a conversation</option>
                  {sessions.map((session) => (
                    <option key={session.id} value={session.id}>
                      {session.title}
                    </option>
                  ))}
                </select>
              </label>
              <button
                className="text-button"
                disabled={
                  busy ||
                  !selectedFeishu ||
                  !capabilities?.ownerIdentityAvailable
                }
              >
                Review and upgrade account profile
              </button>
            </form>
          </>
        )}
      </section>
    </>
  );
}
