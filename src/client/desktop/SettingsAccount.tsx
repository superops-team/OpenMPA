import { Check, LogOut } from 'lucide-react';
import type { ConnectionVM, DesktopServices } from './models';
import { SettingsAccountForm } from './SettingsAccountForm';
import type { useDesktopSettingsState } from './SettingsState';
import { localized, useMessages } from './locale';

type AccountSettingsState = ReturnType<typeof useDesktopSettingsState>;

export function SettingsAccount({
  state,
  services,
  onConnection,
}: {
  state: AccountSettingsState;
  services: DesktopServices;
  onConnection: (connection: ConnectionVM | undefined) => void;
}) {
  const m = useMessages();
  const {
    platform,
    busy,
    act,
    setPlatform,
    setRuntimes,
    vaultStatus,
    setVaultStatus,
    vaultPassphrase,
    setVaultPassphrase,
    vaultConfirm,
    setVaultConfirm,
    vaultUnlockPassphrase,
    setVaultUnlockPassphrase,
    showVaultPassphrase,
    setShowVaultPassphrase,
    loadVaultStatus,
    setRegion,
    setNotice,
    setError,
  } = state;
  const unlockSavedCredentials = (
    <div className="desktop-vault-unlock">
      <p>
        {m.vaultAccount(
          vaultStatus.accountId || m.settingsUnknown(),
          vaultStatus.region || m.settingsUnknown(),
        )}
      </p>
      <label className="desktop-field">
        <span>{m.passphrase()}</span>
        <input
          type={showVaultPassphrase ? 'text' : 'password'}
          autoComplete="current-password"
          autoFocus
          value={vaultUnlockPassphrase}
          onChange={(event) => setVaultUnlockPassphrase(event.target.value)}
        />
      </label>
      <label className="desktop-check">
        <input
          type="checkbox"
          checked={showVaultPassphrase}
          onChange={(event) => setShowVaultPassphrase(event.target.checked)}
        />{' '}
        {m.showPassphrase()}
      </label>
      <button
        className="primary"
        disabled={
          busy ||
          !services.platform.unlockVault ||
          !vaultUnlockPassphrase.trim()
        }
        onClick={() =>
          void act(async () => {
            const value = await services.platform.unlockVault!(
              vaultUnlockPassphrase,
            );
            setVaultUnlockPassphrase('');
            setPlatform(value);
            setRegion(value.region || 'cn-beijing');
            setNotice(localized((m) => m.settingsVaultSignedIn()));
            await loadVaultStatus();
          })
        }
      >
        {m.unlockAndSignIn()}
      </button>
    </div>
  );
  return (
    <>
      <section className="task-detail-card">
        <h2>
          {platform?.signedIn
            ? platform.provider === 'byteplus'
              ? m.byteplusAccount()
              : m.volcengineAccount()
            : m.cloudAccount()}
        </h2>
        {platform?.signedIn ? (
          <>
            <p>
              <Check size={14} /> {m.settingsAccountId(platform.accountId)}
            </p>
            <button
              className="text-button"
              disabled={busy}
              onClick={() =>
                void act(async () => {
                  await services.platform.signOut();
                  setPlatform(undefined);
                  setRuntimes([]);
                  onConnection(undefined);
                  await loadVaultStatus();
                })
              }
            >
              <LogOut size={14} /> {m.signOut()}
            </button>
          </>
        ) : vaultStatus.exists && services.platform.unlockVault ? (
          <>
            <p>{m.vaultUnlockHint()}</p>
            {unlockSavedCredentials}
            <details className="desktop-secondary-auth">
              <summary>{m.showCredentials()}</summary>
              <SettingsAccountForm {...state} />
            </details>
          </>
        ) : (
          <SettingsAccountForm {...state} />
        )}
      </section>
      <section className="task-detail-card desktop-form">
        <h2>{m.savedCredentials()}</h2>
        {!services.platform.vaultStatus ? (
          <p>{m.settingsVaultUnavailable()}</p>
        ) : vaultStatus.exists ? (
          <>
            <p>
              {m.vaultAccount(
                vaultStatus.accountId || m.settingsUnknown(),
                vaultStatus.region || m.settingsUnknown(),
              )}
              {vaultStatus.updatedAt ? ` · ${vaultStatus.updatedAt}` : ''}
            </p>
            {!platform?.signedIn && services.platform.unlockVault && (
              <p>{m.vaultUnlockHint()}</p>
            )}
            <button
              className="text-button"
              disabled={busy || !services.platform.forgetVault}
              onClick={() => {
                if (window.confirm(m.forgetVaultConfirm()))
                  void act(async () => {
                    await services.platform.forgetVault!();
                    setVaultPassphrase('');
                    setVaultConfirm('');
                    setVaultUnlockPassphrase('');
                    setVaultStatus({ exists: false });
                    setNotice(localized((m) => m.settingsVaultRemoved()));
                  });
              }}
            >
              {m.forgetSavedCredentials()}
            </button>
          </>
        ) : platform?.signedIn ? (
          <>
            <p>{m.vaultHint()}</p>
            <label>
              {m.passphrase()}
              <input
                type={showVaultPassphrase ? 'text' : 'password'}
                autoComplete="new-password"
                value={vaultPassphrase}
                onChange={(event) => setVaultPassphrase(event.target.value)}
              />
            </label>
            <label>
              {m.confirmPassphrase()}
              <input
                type={showVaultPassphrase ? 'text' : 'password'}
                autoComplete="new-password"
                value={vaultConfirm}
                onChange={(event) => setVaultConfirm(event.target.value)}
              />
            </label>
            <label className="desktop-check">
              <input
                type="checkbox"
                checked={showVaultPassphrase}
                onChange={(event) =>
                  setShowVaultPassphrase(event.target.checked)
                }
              />{' '}
              {m.showPassphrase()}
            </label>
            <button
              className="primary"
              disabled={
                busy ||
                !services.platform.saveVault ||
                !vaultPassphrase.trim() ||
                !vaultConfirm.trim()
              }
              onClick={() =>
                void act(async () => {
                  if (vaultPassphrase !== vaultConfirm) {
                    setError(localized((m) => m.settingsPassphrasesMismatch()));
                    return;
                  }
                  const next =
                    await services.platform.saveVault!(vaultPassphrase);
                  setVaultPassphrase('');
                  setVaultConfirm('');
                  setVaultStatus(next);
                  setNotice(localized((m) => m.settingsVaultSaved()));
                })
              }
            >
              {m.saveCredentials()}
            </button>
          </>
        ) : (
          <p>{m.vaultSignInFirst()}</p>
        )}
      </section>
    </>
  );
}
