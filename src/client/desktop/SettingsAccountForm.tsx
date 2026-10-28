import type { useDesktopSettingsState } from './SettingsState';
import { localized, useMessages } from './locale';
export function SettingsAccountForm(
  props: ReturnType<typeof useDesktopSettingsState>,
) {
  const m = useMessages();
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
          setNotice(localized((m) => m.settingsSignedIn()));
        });
      }}
    >
      <label>
        {m.accessKey()}
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
        {m.secretKey()}
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
        {m.securityTokenOptional()}
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
        {m.region()}
        <select
          required
          value={credentials.region}
          onChange={(event) =>
            setCredentials({ ...credentials, region: event.target.value })
          }
        >
          {regions.map((item) => (
            <option key={item.id} value={item.id}>
              {item.id === 'cn-beijing'
                ? m.settingsRegionBeijing()
                : item.id === 'cn-shanghai'
                  ? m.settingsRegionShanghai()
                  : item.name}{' '}
              ({item.id})
            </option>
          ))}
        </select>
      </label>
      <p>{m.settingsCredentialsMemoryHint()}</p>
      <button className="primary" disabled={busy}>
        {m.signIn()}
      </button>
    </form>
  );
}
