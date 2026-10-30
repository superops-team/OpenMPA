import { useEffect, useRef, useState } from 'react';
import {
  CalendarDays,
  Clock3,
  List,
  Play,
  Plus,
  Power,
  PowerOff,
  RefreshCw,
} from 'lucide-react';
import { Button, IconButton } from '../ui';
import { ModalDialog } from '../ui/ModalDialog';
import { localized, useMessages, useMessageState } from './locale';
import { ScheduleCalendar } from './ScheduleCalendar';
import { ScheduleDetail } from './ScheduleDetail';
import { ScheduleEditor } from './ScheduleEditor';
import { ScheduleTaskMenu } from './ScheduleTaskMenu';
import { scheduleFailure } from './schedule-failure';
import {
  formatTime,
  runStatuses,
  scheduleStatusLabel,
  scheduleText,
  taskStatus,
} from './schedule-view';
import type {
  ConnectionVM,
  DesktopServices,
  ScheduleDirectoryVM,
  ScheduleVM,
  SessionVM,
} from './models';
import './schedule-page.css';

export function DesktopSchedules({
  services,
  connection,
  onConfigure,
}: {
  services: DesktopServices;
  connection?: ConnectionVM;
  sessions: SessionVM[];
  onConfigure?(): void;
}) {
  const m = useMessages();
  if (!connection || !services.schedules)
    return (
      <main className="main-content schedule-page">
        <div className="page-heading">
          <h1>{m.scheduledTasks()}</h1>
        </div>
        <div className="setup-card">
          <Clock3 size={24} />
          <p>{m.desktopConnectARuntimeWithAVerifiedScheduledTaskIdentity()}</p>
        </div>
      </main>
    );
  return (
    <RuntimeSchedules
      key={`${connection.epoch}:${connection.scope}`}
      services={services}
      connection={connection}
      onConfigure={onConfigure}
    />
  );
}

function RuntimeSchedules({
  services,
  connection,
  onConfigure,
}: {
  services: DesktopServices;
  connection: ConnectionVM;
  onConfigure?(): void;
}) {
  const m = useMessages();
  const api = services.schedules!;
  const [directory, setDirectory] = useState<ScheduleDirectoryVM>();
  const [selected, setSelected] = useState<ScheduleVM>();
  const [draft, setDraft] = useState<ScheduleVM>();
  const [deleting, setDeleting] = useState<ScheduleVM>();
  const [view, setView] = useState('list');
  const [query, setQuery] = useState('');
  const [status, setStatus] = useState('all');
  const [index, setIndex] = useState(0);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [loadError, setLoadError] = useMessageState();
  const [needsIdentity, setNeedsIdentity] = useState(false);
  const [error, setError] = useMessageState();
  const [notice, setNotice] = useMessageState();
  const alive = useRef(true);
  const writing = useRef(false);
  const sequence = useRef(0);
  useEffect(() => {
    alive.current = true;
    return () => {
      alive.current = false;
      sequence.current++;
    };
  }, []);
  async function refresh() {
    const request = ++sequence.current;
    setLoading(true);
    setLoadError('');
    setNeedsIdentity(false);
    try {
      const value = await api.list(connection.epoch);
      if (alive.current && request === sequence.current) {
        setDirectory(value);
        setSelected((current) =>
          current
            ? value.items.find((task) => task.id === current.id) || current
            : undefined,
        );
        setIndex((current) =>
          Math.min(
            current,
            Math.max(0, Math.floor((value.items.length - 1) / 10) * 10),
          ),
        );
      }
    } catch (reason) {
      if (alive.current && request === sequence.current) {
        setLoadError(scheduleFailure(reason));
        const code =
          reason && typeof reason === 'object' && 'code' in reason
            ? reason.code
            : '';
        setNeedsIdentity(
          code === 'businessIdentityRequired' || code === 'businessJWTRequired',
        );
      }
    } finally {
      if (alive.current && request === sequence.current) setLoading(false);
    }
  }
  useEffect(() => {
    void refresh();
  }, [services, connection.epoch]);
  async function mutate<T>(
    operation: () => Promise<T>,
    accepted: (value: T) => void,
  ): Promise<T | undefined> {
    if (writing.current) return;
    writing.current = true;
    setBusy(true);
    setError('');
    setNotice('');
    try {
      const value = await operation();
      if (alive.current) {
        accepted(value);
        await refresh();
      }
      return alive.current ? value : undefined;
    } catch (reason) {
      if (alive.current) setError(scheduleFailure(reason));
    } finally {
      writing.current = false;
      if (alive.current) setBusy(false);
    }
  }
  function editor(task?: ScheduleVM, copy = false) {
    setError('');
    setSelected(undefined);
    setDraft(
      task
        ? {
            ...structuredClone(task),
            ...(copy
              ? {
                  id: '',
                  version: '',
                  name: m.scheduleCopyName(task.name),
                  nextRun: undefined,
                  lastRun: undefined,
                  runningAt: undefined,
                  createdAt: undefined,
                  status: undefined,
                  schedule: { ...task.schedule, anchorAt: undefined },
                }
              : {}),
          }
        : {
            id: '',
            version: '',
            name: '',
            prompt: '',
            agentId: '',
            enabled: true,
            schedule: { type: 'Daily', time: '09:00' },
            timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
            delivery: {
              channel: 'Web',
              receiveIdType: 'chat_id',
              bestEffort: true,
            },
          },
    );
  }
  const toggle = (task: ScheduleVM) => {
    void mutate(
      () => api.enable(connection.epoch, task, !task.enabled),
      (next) => {
        setSelected((current) => (current?.id === next.id ? next : current));
        setNotice(
          localized((m) =>
            next.enabled
              ? m.scheduleEnabledNotice()
              : m.scheduleDisabledNotice(),
          ),
        );
      },
    );
  };
  const run = (task: ScheduleVM) =>
    mutate(
      () => api.trigger(connection.epoch, task),
      (run) => {
        setNotice(
          localized((m) =>
            m.runAcceptedNotice(run.id, scheduleStatusLabel(run.status, m)),
          ),
        );
      },
    );
  const filtered = (directory?.items || []).filter(
    (task) =>
      `${task.name} ${task.prompt} ${task.agentId}`
        .toLowerCase()
        .includes(query.toLowerCase()) &&
      (status === 'all' ||
        (status === 'Enabled' && task.enabled) ||
        (status === 'Disabled' && !task.enabled) ||
        taskStatus(task) === status),
  );
  const visibleIndex = Math.min(
    index,
    Math.max(0, Math.floor((filtered.length - 1) / 10) * 10),
  );
  const visible = filtered.slice(visibleIndex, visibleIndex + 10);
  const open = (task: ScheduleVM) => {
    setError('');
    setSelected(task);
  };
  const remove = (task: ScheduleVM) => {
    setSelected(undefined);
    setError('');
    setDeleting(task);
  };
  return (
    <main className="main-content schedule-page">
      <div className="page-heading">
        <div>
          <h1>{m.scheduledTasks()}</h1>
          <p>{m.desktopScheduledTasksExecuteOnTheRuntimeAndContinueWhen()}</p>
        </div>
        <Button
          variant="primary"
          disabled={busy}
          leading={<Plus size={15} />}
          onClick={() => editor()}
        >
          {m.desktopNewScheduledTask()}
        </Button>
      </div>
      <div className="schedule-runtime">
        <strong>{connection.runtimeName || connection.runtimeId}</strong>
        <span>{m.scheduleScopeHint()}</span>
      </div>
      {notice && (
        <p className="notice" role="status">
          {notice}
        </p>
      )}
      {error && !draft && !deleting && !selected && (
        <p className="error-banner" role="alert">
          {error}
        </p>
      )}
      <div className="schedule-toolbar">
        <div
          className="schedule-view-switch"
          role="group"
          aria-label={m.scheduleView()}
        >
          <Button
            variant={view === 'list' ? 'secondary' : 'ghost'}
            aria-pressed={view === 'list'}
            leading={<List size={15} />}
            onClick={() => setView('list')}
          >
            {m.scheduleListView()}
          </Button>
          <Button
            variant={view === 'calendar' ? 'secondary' : 'ghost'}
            aria-pressed={view === 'calendar'}
            leading={<CalendarDays size={15} />}
            onClick={() => setView('calendar')}
          >
            {m.scheduleCalendarView()}
          </Button>
        </div>
        <label className="schedule-search">
          <span className="sr-only">{m.desktopSearchTasks()}</span>
          <input
            type="search"
            aria-label={m.desktopSearchTasks()}
            placeholder={m.scheduleFindTask()}
            value={query}
            onChange={(event) => {
              setQuery(event.target.value);
              setIndex(0);
            }}
          />
        </label>
        <select
          aria-label={m.scheduleLastStatus()}
          value={status}
          onChange={(event) => {
            setStatus(event.target.value);
            setIndex(0);
          }}
        >
          <option value="all">{m.scheduleAllStatuses()}</option>
          <option value="Enabled">{m.pageEnabled()}</option>
          <option value="Disabled">{m.pageDisabled()}</option>
          {['Pending', ...runStatuses, 'Stuck'].map((value) => (
            <option key={value} value={value}>
              {scheduleStatusLabel(value, m)}
            </option>
          ))}
        </select>
        <IconButton
          label={m.desktopRefresh()}
          disabled={loading || busy}
          onClick={() => {
            void refresh();
          }}
        >
          <RefreshCw size={16} />
        </IconButton>
      </div>
      {directory?.overview && (
        <div className="schedule-overview">
          {directory.overview.executionCount != null && (
            <span>
              {m.scheduleExecutionCount(
                String(directory.overview.executionCount),
              )}
            </span>
          )}
          {directory.overview.successRate != null && (
            <span>
              {m.scheduleSuccessRate(
                String(Math.round(directory.overview.successRate * 100)),
              )}
            </span>
          )}
        </div>
      )}
      {loadError && (
        <div className="error-banner">
          <p role="alert">{loadError}</p>
          {onConfigure && needsIdentity && (
            <Button onClick={onConfigure}>
              {m.scheduleConfigureIdentity()}
            </Button>
          )}
        </div>
      )}
      {loading && (
        <div className="schedule-state" role="status">
          {m.scheduleLoading()}
        </div>
      )}
      {!loading &&
        !loadError &&
        directory &&
        (view === 'calendar' ? (
          <ScheduleCalendar tasks={filtered} onOpen={open} />
        ) : (
          <>
            <div
              className="schedule-table-scroll"
              role="region"
              aria-label={m.scheduledTasks()}
              tabIndex={0}
            >
              <table className="schedule-table">
                <thead>
                  <tr>
                    <th>{m.desktopName()}</th>
                    <th>{m.scheduleExecutionPlan()}</th>
                    <th>{m.scheduleLastRun()}</th>
                    <th>{m.scheduleNextRun()}</th>
                    <th>{m.scheduleLastStatus()}</th>
                    <th>{m.scheduleAgent()}</th>
                    <th>{m.scheduleActions()}</th>
                  </tr>
                </thead>
                <tbody>
                  {visible.map((task) => (
                    <tr key={task.id}>
                      <td>
                        <button
                          className="task-row"
                          disabled={busy}
                          onClick={() => open(task)}
                        >
                          {task.name || task.prompt}
                        </button>
                        <span
                          className="schedule-badge"
                          data-status={task.enabled ? 'enabled' : 'disabled'}
                        >
                          {task.enabled ? m.pageEnabled() : m.pageDisabled()}
                        </span>
                      </td>
                      <td>{scheduleText(task, m)}</td>
                      <td>
                        {formatTime(task.lastRun, undefined, task.timezone)}
                      </td>
                      <td>
                        {formatTime(task.nextRun, undefined, task.timezone)}
                      </td>
                      <td>
                        <span
                          className="schedule-badge"
                          data-status={taskStatus(task).toLowerCase()}
                        >
                          {scheduleStatusLabel(taskStatus(task), m)}
                        </span>
                      </td>
                      <td>{task.agentId || 'Main'}</td>
                      <td>
                        <div className="schedule-actions">
                          <IconButton
                            label={`${m.desktopRunNow()} · ${task.name}`}
                            disabled={busy}
                            onClick={() => {
                              void run(task);
                            }}
                          >
                            <Play size={14} />
                          </IconButton>
                          <IconButton
                            label={`${task.enabled ? m.scheduleDisable() : m.scheduleEnable()} · ${task.name}`}
                            disabled={busy}
                            onClick={() => toggle(task)}
                          >
                            {task.enabled ? (
                              <PowerOff size={14} aria-hidden="true" />
                            ) : (
                              <Power size={14} aria-hidden="true" />
                            )}
                          </IconButton>
                          <ScheduleTaskMenu
                            task={task}
                            busy={busy}
                            onOpen={() => open(task)}
                            onEdit={() => editor(task)}
                            onCopy={() => editor(task, true)}
                            onDelete={() => remove(task)}
                          />
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            {!filtered.length && (
              <div className="schedule-state">
                <Clock3 size={28} />
                <h2>
                  {directory.items.length
                    ? m.scheduleNoMatches()
                    : m.scheduleEmptyTitle()}
                </h2>
                <p>
                  {directory.items.length
                    ? m.tryDifferentSearch()
                    : m.desktopCreateAServerScheduleForThisRuntime()}
                </p>
                {!directory.items.length && (
                  <Button disabled={busy} onClick={() => editor()}>
                    {m.desktopNewScheduledTask()}
                  </Button>
                )}
              </div>
            )}
            <nav
              className="schedule-pagination"
              aria-label={m.scheduledTasks()}
            >
              <span>{m.scheduleTaskCount(String(filtered.length))}</span>
              <Button
                disabled={visibleIndex === 0}
                onClick={() => setIndex(visibleIndex - 10)}
              >
                {m.schedulePreviousPage()}
              </Button>
              <span>
                {m.schedulePage(String(Math.floor(visibleIndex / 10) + 1))}
              </span>
              <Button
                disabled={visibleIndex + 10 >= filtered.length}
                onClick={() => setIndex(visibleIndex + 10)}
              >
                {m.scheduleNextPage()}
              </Button>
            </nav>
          </>
        ))}
      {draft && (
        <ScheduleEditor
          task={draft}
          services={services}
          epoch={connection.epoch}
          busy={busy}
          error={error}
          onClose={() => {
            setDraft(undefined);
            setError('');
          }}
          onSave={(task) => {
            void mutate(
              () =>
                task.id
                  ? api.update(connection.epoch, task)
                  : api.create(connection.epoch, task),
              () => {
                setDraft(undefined);
                setNotice(localized((m) => m.scheduleSaved()));
              },
            );
          }}
        />
      )}
      {selected && (
        <ScheduleDetail
          key={selected.id}
          task={selected}
          services={services}
          epoch={connection.epoch}
          busy={busy}
          error={error}
          onRun={run}
          onToggle={toggle}
          onEdit={editor}
          onDelete={remove}
          onClose={() => setSelected(undefined)}
        />
      )}
      {deleting && (
        <ModalDialog
          title={m.delete()}
          closeLabel={m.closeDialog(m.scheduledTasks())}
          description={m.scheduleDeleteDescription(deleting.name)}
          onClose={() => {
            if (!busy) setDeleting(undefined);
          }}
          footer={
            <>
              <Button disabled={busy} onClick={() => setDeleting(undefined)}>
                {m.cancel()}
              </Button>
              <Button
                variant="destructive"
                disabled={busy}
                onClick={() => {
                  void mutate(
                    () => api.remove(connection.epoch, deleting),
                    () => {
                      setDeleting(undefined);
                      setNotice(localized((m) => m.scheduleDeleted()));
                    },
                  );
                }}
              >
                {busy ? m.working() : m.delete()}
              </Button>
            </>
          }
        >
          {error && (
            <p className="error-banner" role="alert">
              {error}
            </p>
          )}
        </ModalDialog>
      )}
    </main>
  );
}
