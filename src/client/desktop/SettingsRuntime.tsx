import { RefreshCw } from 'lucide-react';
import { Button } from '../ui';
import type { ConnectionVM, DesktopServices } from './models';
import type { useDesktopSettingsState } from './SettingsState';
import { localized, useMessages } from './locale';

type RuntimeSettingsState = ReturnType<typeof useDesktopSettingsState>;

export function SettingsRuntime({
  onManage,
  state,
  services,
  connection,
  onConnection,
  selectingRuntimeId,
  setSelectingRuntimeId,
}: {
  onManage?: () => void;
  state: RuntimeSettingsState;
  services: DesktopServices;
  connection?: ConnectionVM;
  onConnection: (connection: ConnectionVM | undefined) => void;
  selectingRuntimeId: string;
  setSelectingRuntimeId: React.Dispatch<React.SetStateAction<string>>;
}) {
  const m = useMessages();
  const {
    platform,
    busy,
    act,
    generation,
    region,
    regions,
    project,
    setRegion,
    setProject,
    directoryLoading,
    load,
    query,
    setQuery,
    visibleRuntimes,
    setNotice,
    error,
    notice,
  } = state;
  const statuses: Record<string, string> = {
    ready: m.knowledgeReady(),
    running: m.desktopRunning(),
    stopped: m.desktopStopped(),
    failed: m.desktopFailed(),
  };
  return (
    <section className="task-detail-card desktop-form">
      <h2>{m.settingsRuntimeDirectory()}</h2>
      {onManage && (
        <Button variant="outline" onClick={onManage}>
          {m.manageRuntimes()}
        </Button>
      )}
      <p>
        {platform?.signedIn
          ? m.runtimeAccountContext(platform.accountId, region, project)
          : m.settingsRuntimeSignInFirst()}
      </p>
      <div className="desktop-inline">
        <label>
          {m.region()}
          <select
            aria-label={m.region()}
            disabled={busy}
            value={region}
            onChange={(event) => {
              const selected = event.target.value;
              void act(async () => {
                await services.platform.resetRuntime?.();
                setRegion(selected);
                onConnection(undefined);
              });
            }}
          >
            {regions.map((item) => (
              <option key={item.id} value={item.id}>
                {item.id === 'cn-beijing'
                  ? m.settingsRegionBeijing()
                  : item.id === 'cn-shanghai'
                    ? m.settingsRegionShanghai()
                    : item.id === 'ap-southeast-1'
                      ? m.settingsRegionSingapore()
                      : item.name}{' '}
                ({item.id})
              </option>
            ))}
          </select>
        </label>
        <label>
          {m.project()}
          <input
            disabled={busy}
            value={project}
            onChange={(event) => setProject(event.target.value)}
          />
        </label>
        <Button
          variant="primary"
          size="input"
          leading={<RefreshCw size={14} />}
          disabled={busy || directoryLoading || !platform?.signedIn}
          onClick={() => void act(() => load(true))}
        >
          {m.settingsListCurrentAccount()}
        </Button>
      </div>
      <label>
        {m.settingsFilterRuntimes()}
        <input
          placeholder={m.settingsRuntimeFilterPlaceholder()}
          value={query}
          onChange={(event) => setQuery(event.target.value)}
        />
      </label>
      <div className="desktop-runtime-feedback">
        {error && (
          <div className="desktop-runtime-error" role="alert">
            {error}
          </div>
        )}
        <div role="status">{notice}</div>
      </div>
      {directoryLoading && <p role="status">{m.settingsRuntimesLoading()}</p>}
      {platform?.signedIn && !directoryLoading && !visibleRuntimes.length && (
        <p>{m.settingsRuntimesNoMatches()}</p>
      )}
      <div className="task-list">
        {visibleRuntimes.map((runtime) => {
          const selected = connection?.runtimeId === runtime.id;
          const selecting = selectingRuntimeId === runtime.id;
          return (
            <div
              className="desktop-runtime"
              data-selected={selected}
              data-selecting={selecting}
              key={runtime.id}
            >
              <div>
                <strong>{runtime.name || runtime.id}</strong>
                <p>
                  {m.settingsRuntimeVersion(
                    runtime.id,
                    Object.hasOwn(statuses, runtime.status.toLowerCase())
                      ? statuses[runtime.status.toLowerCase()]
                      : runtime.status,
                    runtime.version,
                  )}
                </p>
              </div>
              <Button
                className="desktop-runtime-action"
                variant={selecting ? 'primary' : 'outline'}
                disabled={busy || directoryLoading || selected}
                aria-busy={selecting}
                onClick={() => {
                  setSelectingRuntimeId(runtime.id);
                  void act(async () => {
                    const current = generation.current;
                    const next = await services.platform.bind(
                      region,
                      project,
                      runtime.id,
                    );
                    if (generation.current === current) {
                      onConnection(next);
                      setNotice(localized((m) => m.settingsRuntimeSelected()));
                    }
                  }).finally(() =>
                    setSelectingRuntimeId((current) =>
                      current === runtime.id ? '' : current,
                    ),
                  );
                }}
              >
                {[
                  {
                    label: m.settingsUseRuntime(),
                    visible: !selected && !selecting,
                  },
                  { label: m.settingsRuntimeSelecting(), visible: selecting },
                  {
                    label: m.settingsRuntimeIsSelected(),
                    visible: selected && !selecting,
                  },
                ].map(({ label, visible }) => (
                  <span
                    key={label}
                    aria-hidden={!visible}
                    data-visible={visible}
                  >
                    {label}
                  </span>
                ))}
              </Button>
            </div>
          );
        })}
      </div>
    </section>
  );
}
