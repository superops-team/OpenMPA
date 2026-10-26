import type { useDesktopSettingsState } from './SettingsState';
export function SettingsAccountForm(
  props: ReturnType<typeof useDesktopSettingsState>,
) {
  const {
    act,
    generation,
    services,
    credentials,
    setPlatform,
    setCredentials,
    setRegion,
    setNotice,
    busy,
  } = props;
  return (
    <form
      className="desktop-form"
      onSubmit={(event) => {
        event.preventDefault();
        void act(async () => {
          const current = generation.current;
          const value = await services.platform.signIn(credentials);
          if (generation.current !== current) return;
          setPlatform(value);
          setCredentials({
            accessKey: '',
            secretKey: '',
            securityToken: '',
            region: credentials.region,
            remember: credentials.remember,
          });
          setRegion(credentials.region);
          setNotice('Signed in. Credential inputs cleared.');
        });
      }}
    >
      <label>
        Access Key
        <input
          autoComplete="off"
          type="password"
          required
          value={credentials.accessKey}
          onChange={(event) =>
            setCredentials({
              ...credentials,
              accessKey: event.target.value,
            })
          }
        />
      </label>
      <label>
        Secret Key
        <input
          autoComplete="off"
          type="password"
          required
          value={credentials.secretKey}
          onChange={(event) =>
            setCredentials({
              ...credentials,
              secretKey: event.target.value,
            })
          }
        />
      </label>
      <label>
        Security token (optional)
        <input
          autoComplete="off"
          type="password"
          value={credentials.securityToken}
          onChange={(event) =>
            setCredentials({
              ...credentials,
              securityToken: event.target.value,
            })
          }
        />
      </label>
      <label>
        Region
        <input
          required
          value={credentials.region}
          onChange={(event) =>
            setCredentials({ ...credentials, region: event.target.value })
          }
        />
      </label>
      <label className="desktop-check">
        <input
          type="checkbox"
          checked={credentials.remember}
          onChange={(event) =>
            setCredentials({
              ...credentials,
              remember: event.target.checked,
            })
          }
        />
        Remember this account in the system credential store
      </label>
      <p>
        {credentials.remember
          ? 'The host saves credentials in the system credential store.'
          : 'Credentials remain in this app process until sign out or exit.'}
      </p>
      <button className="primary" disabled={busy}>
        Sign in
      </button>
    </form>
  );
}
