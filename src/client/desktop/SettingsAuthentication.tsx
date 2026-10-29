import { Button } from '../ui';
import type { ConnectionVM, DesktopServices } from './models';
import { localized, useMessages } from './locale';
import type { useDesktopSettingsState } from './SettingsState';

type AuthenticationSettingsState = ReturnType<typeof useDesktopSettingsState>;

export function SettingsAuthentication({
  connection,
  onConnection,
  services,
  state,
}: {
  connection: ConnectionVM;
  onConnection: (connection: ConnectionVM | undefined) => void;
  services: DesktopServices;
  state: AuthenticationSettingsState;
}) {
  const m = useMessages();
  const {
    act,
    agentIdentityUserPoolId,
    busy,
    channelAuth,
    generation,
    identity,
    loadUserTokenVaultStatus,
    setChannelAuth,
    setAgentIdentityUserPoolId,
    setIdentity,
    setNotice,
    setUserTokenVaultPassphrase,
    setUserTokenVaultStatus,
    userTokenVaultPassphrase,
    userTokenVaultStatus,
  } = state;
  const canLoginWithAgentIdentity = Boolean(
    services.platform?.loginAgentIdentity,
  );
  return (
    <>
      <section className="task-detail-card">
        <h2>{m.settingsBusinessConnection()}</h2>
        <p>
          {m.settingsConnectionSummary(
            connection.runtimeId,
            connection.status,
            connection.scope,
          )}
        </p>
        <button
          className="text-button"
          disabled={busy}
          onClick={() =>
            void act(async () =>
              setNotice(await services.connection.test(connection.epoch)),
            )
          }
        >
          {m.testConnection()}
        </button>
        {services.platform?.userTokenVaultStatus && (
          <div className="desktop-form">
            <h3>{m.businessIdentity()}</h3>
            {services.platform.saveAgentIdentityConfig && (
              <label>
                {m.agentIdentityUserPoolId()}
                <input
                  value={agentIdentityUserPoolId}
                  onChange={(event) =>
                    setAgentIdentityUserPoolId(event.target.value)
                  }
                />
              </label>
            )}
            {services.platform.saveAgentIdentityConfig && (
              <Button
                variant="outline"
                disabled={busy || !agentIdentityUserPoolId.trim()}
                onClick={() =>
                  void act(async () => {
                    const saved = await services.platform
                      .saveAgentIdentityConfig!(agentIdentityUserPoolId);
                    setAgentIdentityUserPoolId(saved.userPoolId || '');
                    await loadUserTokenVaultStatus(connection.runtimeId);
                    setNotice(
                      localized((m) => m.settingsBusinessIdentitySaved()),
                    );
                  })
                }
              >
                {m.saveAgentIdentityUserPool()}
              </Button>
            )}
            <p>
              {userTokenVaultStatus.exists
                ? m.settingsConnectionSummary(
                    `${userTokenVaultStatus.subject || connection.runtimeId} · ${userTokenVaultStatus.userPoolId || m.settingsUnknown()}`,
                    m.settingsAgentIdentityAuthenticated(),
                    userTokenVaultStatus.expiresAt || m.settingsUnknown(),
                  )
                : m.businessJWTRequired()}
            </p>
            {(canLoginWithAgentIdentity || userTokenVaultStatus.exists) && (
              <label>
                {m.passphrase()}
                <input
                  aria-label={m.agentIdentityPassphrase()}
                  type="password"
                  autoComplete="current-password"
                  value={userTokenVaultPassphrase}
                  onChange={(event) =>
                    setUserTokenVaultPassphrase(event.target.value)
                  }
                />
              </label>
            )}
            {(canLoginWithAgentIdentity || userTokenVaultStatus.exists) && (
              <div className="desktop-inline">
                {canLoginWithAgentIdentity && (
                  <Button
                    variant="primary"
                    disabled={
                      busy ||
                      !agentIdentityUserPoolId.trim() ||
                      !userTokenVaultPassphrase.trim()
                    }
                    onClick={() =>
                      void act(async () => {
                        const current = generation.current;
                        await services.platform.loginAgentIdentity!(
                          connection.runtimeId,
                          agentIdentityUserPoolId,
                          userTokenVaultPassphrase,
                          connection.epoch,
                        );
                        setUserTokenVaultPassphrase('');
                        const next = await services.connection.status();
                        if (generation.current !== current) return;
                        if (next) onConnection(next);
                        await loadUserTokenVaultStatus(connection.runtimeId);
                        setNotice(
                          localized((m) => m.settingsBusinessIdentitySaved()),
                        );
                      })
                    }
                  >
                    {m.loginWithAgentIdentity()}
                  </Button>
                )}
                {services.platform?.unlockUserTokenVault && (
                  <Button
                    variant={canLoginWithAgentIdentity ? 'outline' : 'primary'}
                    disabled={
                      busy ||
                      !userTokenVaultStatus.exists ||
                      !userTokenVaultPassphrase.trim()
                    }
                    onClick={() =>
                      void act(async () => {
                        const current = generation.current;
                        await services.platform.unlockUserTokenVault!(
                          connection.runtimeId,
                          userTokenVaultPassphrase,
                          connection.epoch,
                        );
                        setUserTokenVaultPassphrase('');
                        const next = await services.connection.status();
                        if (generation.current !== current) return;
                        if (next) onConnection(next);
                        await loadUserTokenVaultStatus(connection.runtimeId);
                        setNotice(
                          localized((m) => m.settingsBusinessIdentitySaved()),
                        );
                      })
                    }
                  >
                    {m.unlockSavedAgentIdentity()}
                  </Button>
                )}
                {services.platform?.forgetUserTokenVault && (
                  <button
                    className="text-button"
                    disabled={busy}
                    onClick={() =>
                      void act(async () => {
                        await services.platform.forgetUserTokenVault!(
                          connection.runtimeId,
                        );
                        setUserTokenVaultStatus({ exists: false });
                        setUserTokenVaultPassphrase('');
                        setNotice(localized((m) => m.forgetSavedCredentials()));
                      })
                    }
                  >
                    {m.forgetSavedCredentials()}
                  </button>
                )}
              </div>
            )}
          </div>
        )}
        <details className="desktop-form">
          <summary>{m.verifiedTest()}</summary>
          <form
            className="desktop-form"
            onSubmit={(event) => {
              event.preventDefault();
              void act(async () => {
                const currentEpoch = connection.epoch;
                const current = generation.current;
                const next = await services.connection.identity(
                  currentEpoch,
                  identity.userId,
                  '',
                  '',
                  '',
                  '',
                  true,
                );
                if (generation.current !== current) return;
                onConnection(next);
                setIdentity({ ...identity, applicationToken: '' });
                setNotice(localized((m) => m.settingsBusinessIdentitySaved()));
              });
            }}
          >
            <label>
              {m.businessUserId()}
              <input
                required
                value={identity.userId}
                onChange={(event) =>
                  setIdentity({ ...identity, userId: event.target.value })
                }
              />
            </label>
            <button className="primary" disabled={busy}>
              {m.saveBusinessIdentity()}
            </button>
          </form>
        </details>
      </section>
      <section className="task-detail-card">
        <h2>{m.settingsChannelAdministratorCredentials()}</h2>
        <p>{m.settingsChannelCredentialsHint()}</p>
        <form
          className="desktop-form"
          onSubmit={(event) => {
            event.preventDefault();
            void act(async () => {
              const current = generation.current;
              const next = await services.connection.channelAuth(
                connection.epoch,
                channelAuth.mode,
                channelAuth.secret,
              );
              if (generation.current !== current) return;
              onConnection(next);
              setChannelAuth({ ...channelAuth, secret: '' });
              setNotice(
                localized((m) => m.settingsChannelAuthenticationSaved()),
              );
            });
          }}
        >
          <label>
            {m.channelAuthenticationMode()}
            <select
              value={channelAuth.mode}
              onChange={(event) =>
                setChannelAuth({ mode: event.target.value, secret: '' })
              }
            >
              <option value="runtimeKey">
                {m.settingsRuntimeChannelKey()}
              </option>
              <option value="legacyJwt">{m.legacyJwt()}</option>
              <option value="none">{m.channelUseRuntimeIdentity()}</option>
            </select>
          </label>
          {channelAuth.mode !== 'none' && (
            <label>
              {m.channelCredential()}
              <input
                required
                type="password"
                autoComplete="off"
                value={channelAuth.secret}
                onChange={(event) =>
                  setChannelAuth({
                    ...channelAuth,
                    secret: event.target.value,
                  })
                }
              />
            </label>
          )}
          <button className="primary" disabled={busy}>
            {m.saveChannelAuthentication()}
          </button>
        </form>
      </section>
    </>
  );
}
