import { RefreshCw } from 'lucide-react';
import { Button } from '../ui';
import type { ConnectionVM, DesktopServices } from './models';
import type { useDesktopSettingsState } from './SettingsState';

type RuntimeSettingsState = ReturnType<typeof useDesktopSettingsState>;

export function SettingsRuntime({
  state,
  services,
  connection,
  onConnection,
  selectingRuntimeId,
  setSelectingRuntimeId,
}: {
  state: RuntimeSettingsState;
  services: DesktopServices;
  connection?: ConnectionVM;
  onConnection: (connection: ConnectionVM | undefined) => void;
  selectingRuntimeId: string;
  setSelectingRuntimeId: React.Dispatch<React.SetStateAction<string>>;
}) {
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
  } = state;
  return (
    <section className="task-detail-card desktop-form">
      <h2>AgentKit MPA Runtimes</h2>
      <p>
        {platform?.signedIn
          ? `Account ${platform.accountId} · ${region} · ${project}`
          : 'Sign in to choose an MPA Runtime.'}
      </p>
      <div className="desktop-inline">
        <label>
          Region
          <select
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
                {item.name} ({item.id})
              </option>
            ))}
          </select>
        </label>
        <label>
          Project
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
          List current account
        </Button>
      </div>
      <label>
        Filter Runtimes
        <input
          placeholder="Name or Runtime ID"
          value={query}
          onChange={(event) => setQuery(event.target.value)}
        />
      </label>
      {directoryLoading && <p role="status">Loading MPA Runtimes…</p>}
      {platform?.signedIn && !directoryLoading && !visibleRuntimes.length && (
        <p>
          No matching MPA Runtimes. Check the region, project or filter, then
          refresh.
        </p>
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
                  {runtime.id} · {runtime.status} · version {runtime.version}
                </p>
              </div>
              <Button
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
                      setNotice(
                        'Runtime selected. Verify business identity and connection.',
                      );
                    }
                  }).finally(() =>
                    setSelectingRuntimeId((current) =>
                      current === runtime.id ? '' : current,
                    ),
                  );
                }}
              >
                {selecting
                  ? 'Selecting…'
                  : selected
                    ? 'Selected'
                    : 'Use Runtime'}
              </Button>
            </div>
          );
        })}
      </div>
    </section>
  );
}
