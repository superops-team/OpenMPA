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
    busy,
    channelAuth,
    generation,
    identity,
    loadUserTokenVaultStatus,
    setChannelAuth,
    setIdentity,
    setNotice,
    setUserTokenVaultPassphrase,
    setUserTokenVaultStatus,
    userTokenVaultPassphrase,
    userTokenVaultStatus,
  } = state;
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
        {services.platform.userTokenVaultStatus && (
          <div className="desktop-form">
            <h3>{m.businessIdentity()}</h3>
            <p>
              {userTokenVaultStatus.exists
                ? m.settingsConnectionSummary(
                    userTokenVaultStatus.subject || connection.runtimeId,
                    m.settingsLegacyJwtAuthenticated(),
                    userTokenVaultStatus.expiresAt || m.settingsUnknown(),
                  )
                : m.businessJWTRequired()}
            </p>
            {userTokenVaultStatus.exists &&
              services.platform.unlockUserTokenVault && (
                <label>
                  {m.vaultUnlockHint()}
                  <input
                    aria-label={m.vaultUnlockHint()}
                    type="password"
                    autoComplete="current-password"
                    value={userTokenVaultPassphrase}
                    onChange={(event) =>
                      setUserTokenVaultPassphrase(event.target.value)
                    }
                  />
                </label>
              )}
            {userTokenVaultStatus.exists && (
              <div className="desktop-inline">
                {services.platform.unlockUserTokenVault && (
                  <Button
                    variant="primary"
                    disabled={busy || !userTokenVaultPassphrase.trim()}
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
                    {m.unlockAndSignIn()}
                  </Button>
                )}
                {services.platform.forgetUserTokenVault && (
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
                        setNotice(
                          localized((m) => m.forgetSavedCredentials()),
                        );
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
                identity.applicationToken,
                '',
                '',
                identity.testDeployment,
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
          <label className="desktop-check">
            <input
              type="checkbox"
              checked={identity.testDeployment}
              onChange={(event) =>
                setIdentity({
                  ...identity,
                  testDeployment: event.target.checked,
                })
              }
            />{' '}
            {m.verifiedTest()}
          </label>
          {(['applicationToken'] as const).map((slot) => (
            <label key={slot}>
              {m.settingsLegacyJwtAuthenticated()}
              <input
                type="password"
                autoComplete="off"
                value={identity[slot]}
                onChange={(event) =>
                  setIdentity({ ...identity, [slot]: event.target.value })
                }
              />
            </label>
          ))}
          <button className="primary" disabled={busy}>
            {m.saveBusinessIdentity()}
          </button>
        </form>
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
              <option value="runtimeKey">{m.settingsRuntimeChannelKey()}</option>
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
