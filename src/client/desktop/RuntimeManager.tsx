import { useEffect, useRef, useState } from 'react';
import { Plus, RefreshCw, Server, Trash2 } from 'lucide-react';
import { Button } from '../ui';
import { useMessages, useMessageState } from './locale';
import { studioFailure } from './studio-services';
import { LifecycleStatus } from './LifecycleStatus';
import { MpaCreateDialog } from './MpaCreateDialog';
import { MpaDeleteDialog } from './MpaDeleteDialog';
import type { MpaCreationTask, MpaDeletionTask } from './lifecycle-models';
import type {
  ConnectionVM,
  DesktopServices,
  PlatformVM,
  RuntimeVM,
} from './models';
import './runtime-manager.css';
import { creationActive } from './lifecycle-state';

export function RuntimeManager({
  services,
  platform: initialPlatform,
  connection,
  onConnection,
  onDirectoryChanged,
}: {
  services: DesktopServices;
  platform?: PlatformVM;
  connection?: ConnectionVM;
  onConnection(connection: ConnectionVM | undefined): void;
  onDirectoryChanged(): Promise<void>;
}) {
  const m = useMessages();
  const [platform, setPlatform] = useState(initialPlatform);
  const [region, setRegion] = useState(initialPlatform?.region || 'cn-beijing');
  const [project, setProject] = useState('default');
  const [query, setQuery] = useState('');
  const [runtimes, setRuntimes] = useState<RuntimeVM[]>([]);
  const [creations, setCreations] = useState<MpaCreationTask[]>([]);
  const [deletions, setDeletions] = useState<MpaDeletionTask[]>([]);
  const [loading, setLoading] = useState(false);
  const [connecting, setConnecting] = useState('');
  const [notice, setNotice] = useMessageState();
  const [create, setCreate] = useState<false | { task?: MpaCreationTask }>(
    false,
  );
  const [remove, setRemove] = useState<{
    runtimeId: string;
    name: string;
    task?: MpaDeletionTask;
  }>();
  const generation = useRef(0);
  const epoch = platform?.epoch;
  const scope = platform?.scope || `${platform?.accountId}:${epoch}`;
  const api = services.lifecycle;
  const load = async (force = true) => {
    const id = ++generation.current;
    setLoading(true);
    setNotice('');
    try {
      const current = await services.platform.status();
      if (generation.current !== id) return;
      setPlatform(current);
      if (!current.signedIn) {
        setRuntimes([]);
        setCreations([]);
        setDeletions([]);
        return;
      }
      const result = await services.platform.list(
        region,
        project.trim() || 'default',
        force,
      );
      if (generation.current === id) setRuntimes(result);
      if (api && current.epoch !== undefined) {
        const [created, deleted] = await Promise.all([
          api.creations(current.epoch, region),
          api.deletions(current.epoch, region),
        ]);
        if (generation.current === id) {
          setCreations(created);
          setDeletions(deleted);
        }
      }
    } catch (error) {
      if (generation.current === id) setNotice(studioFailure(error));
    } finally {
      if (generation.current === id) setLoading(false);
    }
  };
  useEffect(() => {
    setRuntimes([]);
    setCreations([]);
    setDeletions([]);
    setCreate(false);
    setRemove(undefined);
    void load(false);
    return () => {
      generation.current++;
    };
  }, [services, initialPlatform?.epoch, region]);
  const changed = () => {
    void load(true);
    void onDirectoryChanged().catch(() => {});
  };
  const connect = async (runtimeId: string) => {
    if (connecting) return;
    const id = generation.current;
    setConnecting(runtimeId);
    try {
      const next = await services.platform.bind(
        region,
        project.trim() || 'default',
        runtimeId,
      );
      if (id === generation.current) onConnection(next);
    } finally {
      if (id === generation.current) setConnecting('');
    }
  };
  const cleared = useRef(new Map<string, Promise<void>>());
  const deleted = (runtimeId: string): Promise<void> => {
    const existing = cleared.current.get(runtimeId);
    if (existing) return existing;
    const work = (async () => {
      if (connection?.runtimeId === runtimeId) {
        const current = await services.connection.status();
        if (current?.runtimeId !== runtimeId) onConnection(current);
      }
      changed();
    })();
    cleared.current.set(runtimeId, work);
    void work.catch(() => cleared.current.delete(runtimeId));
    return work;
  };
  const callbacks = useRef({ deleted, changed });
  callbacks.current = { deleted, changed };
  const activeCreations = creations
    .filter((task) => creationActive(task.state))
    .map((task) => task.taskId)
    .join(',');
  const activeDeletions = deletions
    .filter((task) => task.state === 'running')
    .map((task) => task.operationId)
    .join(',');
  useEffect(() => {
    if (!api || epoch === undefined || (!activeCreations && !activeDeletions))
      return;
    let active = true;
    let timer: ReturnType<typeof setTimeout>;
    const poll = async () => {
      try {
        const [created, deleted] = await Promise.all([
          Promise.all(
            activeCreations
              ? activeCreations
                  .split(',')
                  .map((id) => api.readCreation(epoch, region, id))
              : [],
          ),
          Promise.all(
            activeDeletions
              ? activeDeletions
                  .split(',')
                  .map((id) => api.readDeletion(epoch, region, id))
              : [],
          ),
        ]);
        if (!active) return;
        setCreations((previous) =>
          previous.map(
            (task) =>
              created.find((value) => value.taskId === task.taskId) || task,
          ),
        );
        setDeletions((previous) =>
          previous.map(
            (task) =>
              deleted.find((value) => value.operationId === task.operationId) ||
              task,
          ),
        );
        for (const task of deleted)
          if (task.state === 'succeeded')
            await callbacks.current.deleted(task.runtimeId);
        if (created.some((task) => task.state === 'succeeded'))
          callbacks.current.changed();
      } catch (error) {
        if (active) setNotice(studioFailure(error));
      }
      if (active) timer = setTimeout(poll, 2000);
    };
    timer = setTimeout(poll, 2000);
    return () => {
      active = false;
      clearTimeout(timer);
    };
  }, [api, epoch, region, activeCreations, activeDeletions]);
  const visible = runtimes.filter((value) =>
    `${value.name} ${value.id}`
      .toLowerCase()
      .includes(query.trim().toLowerCase()),
  );
  const enabled = Boolean(api && platform?.signedIn && epoch !== undefined);
  return (
    <main className="main-content mpa-manager">
      <header className="page-heading">
        <div>
          <h1>{m.manageRuntimes()}</h1>
          <p>{m.mpaManagerHint()}</p>
        </div>
        <Button
          variant="primary"
          leading={<Plus size={16} />}
          disabled={!enabled || loading || Boolean(connecting)}
          onClick={() => setCreate({})}
        >
          {m.mpaCreate()}
        </Button>
      </header>
      {notice && (
        <p role="status" className="mpa-notice">
          {notice}
        </p>
      )}
      {!platform?.signedIn && <p>{m.mpaReauthenticate()}</p>}
      <section
        className="mpa-runtime-panel"
        aria-label={m.settingsRuntimeDirectory()}
      >
        <form
          className="mpa-toolbar"
          onSubmit={(event) => {
            event.preventDefault();
            void load();
          }}
        >
          <label>
            {m.region()}
            <select
              disabled={loading || Boolean(connecting)}
              value={region}
              onChange={(event) => setRegion(event.target.value)}
            >
              <option value="cn-beijing">
                {m.settingsRegionBeijing()} (cn-beijing)
              </option>
              <option value="cn-shanghai">
                {m.settingsRegionShanghai()} (cn-shanghai)
              </option>
            </select>
          </label>
          <label>
            {m.project()}
            <input
              disabled={loading || Boolean(connecting)}
              value={project}
              onChange={(event) => setProject(event.target.value)}
            />
          </label>
          <label className="mpa-search">
            {m.settingsFilterRuntimes()}
            <input
              type="search"
              value={query}
              onChange={(event) => setQuery(event.target.value)}
            />
          </label>
          <Button
            variant="outline"
            type="submit"
            leading={<RefreshCw size={15} />}
            disabled={loading || !platform?.signedIn}
          >
            {m.settingsListCurrentAccount()}
          </Button>
        </form>
        {loading && (
          <p role="status" className="mpa-panel-message">
            {m.mpaLoadingManagement()}
          </p>
        )}
        {!loading && visible.length === 0 && (
          <p className="mpa-panel-message">{m.noVisibleRuntimes()}</p>
        )}
        <ul className="mpa-runtime-list">
          {visible.map((runtime) => (
            <li key={runtime.id}>
              <Server size={21} aria-hidden="true" />
              <div className="mpa-runtime-summary">
                <strong>{runtime.name || runtime.id}</strong>
                <p className="mpa-identifier">{runtime.id}</p>
                <p>
                  {runtime.status} · {runtime.version}
                </p>
              </div>
              <div className="mpa-row-actions">
                <Button
                  variant="outline"
                  disabled={
                    Boolean(connecting) ||
                    loading ||
                    connection?.runtimeId === runtime.id
                  }
                  onClick={() => {
                    void connect(runtime.id).catch((error) =>
                      setNotice(studioFailure(error)),
                    );
                  }}
                >
                  {connecting === runtime.id
                    ? m.settingsRuntimeSelecting()
                    : connection?.runtimeId === runtime.id
                      ? m.settingsRuntimeIsSelected()
                      : m.settingsUseRuntime()}
                </Button>
                <Button
                  variant="ghost"
                  leading={<Trash2 size={15} />}
                  disabled={!enabled || loading || Boolean(connecting)}
                  onClick={() =>
                    setRemove({
                      runtimeId: runtime.id,
                      name: runtime.name || runtime.id,
                    })
                  }
                >
                  {m.mpaDelete()}
                </Button>
              </div>
            </li>
          ))}
        </ul>
      </section>
      <section className="mpa-history" aria-label={m.mpaOperationHistory()}>
        <h2>{m.mpaOperationHistory()}</h2>
        {!creations.length && !deletions.length && <p>{m.mpaNoOperations()}</p>}
        <ul>
          {creations.map((task) => (
            <li key={task.taskId}>
              <div>
                <strong>
                  {m.mpaCreate()} · {task.name}
                </strong>
                <LifecycleStatus state={task.state} />
              </div>
              <Button variant="ghost" onClick={() => setCreate({ task })}>
                {m.mpaOpenTask()}
              </Button>
            </li>
          ))}
          {deletions.map((task) => (
            <li key={task.operationId}>
              <div>
                <strong>
                  {m.mpaDelete()} · {task.name}
                </strong>
                <LifecycleStatus state={task.state} />
              </div>
              <Button
                variant="ghost"
                onClick={() =>
                  setRemove({
                    runtimeId: task.runtimeId,
                    name: task.name,
                    task,
                  })
                }
              >
                {m.mpaOpenTask()}
              </Button>
            </li>
          ))}
        </ul>
      </section>
      {create && api && epoch !== undefined && (
        <MpaCreateDialog
          key={`${scope}:${region}:${create.task?.taskId || 'new'}`}
          services={api}
          epoch={epoch}
          scope={scope}
          region={region}
          task={create.task}
          onClose={() => setCreate(false)}
          onChanged={changed}
          onConnect={connect}
        />
      )}
      {remove && api && epoch !== undefined && (
        <MpaDeleteDialog
          key={`${scope}:${region}:${remove.runtimeId}:${remove.task?.operationId || 'new'}`}
          services={api}
          epoch={epoch}
          region={region}
          {...remove}
          onClose={() => setRemove(undefined)}
          onChanged={changed}
          onDeleted={deleted}
        />
      )}
    </main>
  );
}
