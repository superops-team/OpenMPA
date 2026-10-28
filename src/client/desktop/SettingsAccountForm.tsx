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
    loadVaultStatus,
    busy,
    regions,
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
          });
          setRegion(credentials.region);
          await loadVaultStatus();
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
        <select
          required
          value={credentials.region}
          onChange={(event) =>
            setCredentials({ ...credentials, region: event.target.value })
          }
        >
          {regions.map((item) => (
            <option key={item.id} value={item.id}>
              {item.name} ({item.id})
            </option>
          ))}
        </select>
      </label>
      <p>
        Credentials stay in memory unless you save them to the encrypted local
        vault after sign-in.
      </p>
      <button className="primary" disabled={busy}>
        Sign in
      </button>
    </form>
  );
}
