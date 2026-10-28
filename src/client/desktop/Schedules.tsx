import {
  errorMessage,
  localized,
  useMessages,
  useMessageState,
} from './locale';
import { useEffect, useRef, useState } from 'react';
import { Clock3, Plus, Play, Pause, Trash2, RefreshCw } from 'lucide-react';
import type {
  ConnectionVM,
  DesktopServices,
  RunVM,
  ScheduleVM,
  SessionVM,
} from './models';
export function DesktopSchedules({
  services,
  connection,
  sessions: _sessions,
}: {
  services: DesktopServices;
  connection?: ConnectionVM;
  sessions: SessionVM[];
}) {
  const m = useMessages();
  const [tasks, setTasks] = useState<ScheduleVM[]>([]);
  const [detail, setDetail] = useState<{
    task: ScheduleVM;
    runs: RunVM[];
    count?: number;
    hasMore?: boolean;
    nextOffset?: number;
  }>();
  const [draft, setDraft] = useState<ScheduleVM>();
  const [targets, setTargets] = useState<
    Array<{ appId?: string; targetId: string; name: string; channel: string }>
  >([]);
  const [query, setQuery] = useState('');
  const [error, setError] = useMessageState();
  const [notice, setNotice] = useMessageState();
  const typeLabels: Record<string, string> = {
    Once: m.scheduleOnce(),
    Interval: m.scheduleInterval(),
    Daily: m.scheduleDaily(),
    Weekly: m.scheduleWeekly(),
    Monthly: m.scheduleMonthly(),
    Cron: m.scheduleCron(),
  };
  const typeLabel = (type: string) =>
    Object.hasOwn(typeLabels, type) ? typeLabels[type] : type;
  const statusLabel = (status: string) => {
    const labels: Record<string, string> = {
      queued: m.scheduleQueued(),
      pending: m.pagePending(),
      running: m.desktopRunning(),
      succeeded: m.desktopSuccess(),
      success: m.desktopSuccess(),
      completed: m.pageCompleted(),
      failed: m.desktopFailed(),
      cancelled: m.pageCancelled(),
      canceled: m.pageCancelled(),
    };
    const key = status.toLowerCase();
    return Object.hasOwn(labels, key) ? labels[key] : status;
  };
  const [busy, setBusy] = useState(false);
  const epoch = useRef(connection?.epoch);
  epoch.current = connection?.epoch;
  const refresh = async () => {
    if (!connection || !services.schedules) return;
    const next = await services.schedules.list(connection.epoch);
    if (epoch.current === connection.epoch) setTasks(next);
  };
  useEffect(() => {
    void refresh().catch((error: unknown) =>
      setError(
        error instanceof Error
          ? error.message
          : localized((m) => m.desktopCouldNotLoadSchedules()),
      ),
    );
  }, [services, connection?.epoch]);
  const action = async (run: () => Promise<void>) => {
    setBusy(true);
    setError('');
    try {
      await run();
      await refresh();
    } catch (error) {
      setError(
        errorMessage(
          error,
          localized((m) => m.desktopScheduleOperationFailed()),
        ),
      );
    } finally {
      setBusy(false);
    }
  };
  return (
    <main className="main-content">
      <div className="page-heading">
        <div>
          <span className="eyebrow">{m.desktopYOURWORKSPACE()}</span>
          <h1>{m.desktopALittleFollowThrough()}</h1>
          <p>{m.desktopScheduledTasksExecuteOnTheRuntimeAndContinueWhen()}</p>
        </div>
        <button
          className="primary"
          disabled={!connection || !services.schedules}
          onClick={() =>
            setDraft({
              id: '',
              name: '',
              prompt: '',
              agentId: '',
              enabled: true,
              version: '',
              schedule: { type: 'Cron', cronExpression: '' },
              timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
            })
          }
        >
          <Plus size={15} /> {m.desktopNewScheduledTask()}
        </button>
      </div>
      {error && (
        <div role="alert" className="error-banner">
          {error}
        </div>
      )}
      {notice && (
        <div className="notice" role="status">
          {notice}
        </div>
      )}
      {!services.schedules || !connection ? (
        <div className="setup-card">
          <Clock3 size={24} />
          <p>{m.desktopConnectARuntimeWithAVerifiedScheduledTaskIdentity()}</p>
        </div>
      ) : (
        <>
          <label className="search-box">
            <input
              aria-label={m.desktopSearchTasks()}
              placeholder={m.scheduleFindTask()}
              value={query}
              onChange={(event) => setQuery(event.target.value)}
            />
          </label>
          <div className="task-list">
            {tasks
              .filter((task) =>
                `${task.name} ${task.prompt}`
                  .toLowerCase()
                  .includes(query.toLowerCase()),
              )
              .map((task) => (
                <button
                  className="task-row"
                  key={task.id}
                  onClick={() =>
                    void action(async () => {
                      const next = await services.schedules!.read(
                        connection.epoch,
                        task.id,
                      );
                      if (epoch.current === connection.epoch) setDetail(next);
                    })
                  }
                >
                  <Clock3 size={18} />
                  <div>
                    <strong>{task.name || task.prompt}</strong>
                    <span>
                      {task.enabled ? m.pageEnabled() : m.pageDisabled()} ·{' '}
                      {typeLabel(task.schedule.type)}{' '}
                      {task.schedule.cronExpression ||
                        task.schedule.time ||
                        task.schedule.runAt ||
                        task.schedule.intervalSeconds}{' '}
                      · {task.timezone}
                    </span>
                  </div>
                </button>
              ))}
          </div>
          {!tasks.length && (
            <div className="large-empty">
              <Clock3 size={32} />
              <h2>{m.desktopLetAThoughtComeBackAround()}</h2>
              <p>{m.desktopCreateAServerScheduleForThisRuntime()}</p>
            </div>
          )}
          {!!tasks.length &&
            !tasks.some((task) =>
              `${task.name} ${task.prompt}`
                .toLowerCase()
                .includes(query.toLowerCase()),
            ) && (
              <div className="large-empty">
                <h2>{m.scheduleNoMatches()}</h2>
                <p>{m.tryDifferentSearch()}</p>
              </div>
            )}
        </>
      )}
      {detail && connection && services.schedules && (
        <section className="task-detail-card">
          <h2>{detail.task.name}</h2>
          <p>{detail.task.prompt}</p>
          <p>
            {typeLabel(detail.task.schedule.type)}{' '}
            {detail.task.schedule.cronExpression ||
              detail.task.schedule.time ||
              detail.task.schedule.runAt ||
              detail.task.schedule.intervalSeconds}{' '}
            · {detail.task.timezone} {m.desktopNext()}{' '}
            {detail.task.nextRun ?? m.valueUnavailable()}
          </p>
          <div className="task-controls">
            <button disabled={busy} onClick={() => setDraft(detail.task)}>
              {m.desktopEdit()}
            </button>
            <button
              disabled={busy}
              onClick={() =>
                void action(async () => {
                  const next = await services.schedules!.enable(
                    connection.epoch,
                    detail.task,
                    !detail.task.enabled,
                  );
                  setDetail({ ...detail, task: next });
                })
              }
            >
              {detail.task.enabled ? <Pause size={14} /> : <Play size={14} />}
              {detail.task.enabled ? m.scheduleDisable() : m.scheduleEnable()}
            </button>
            <button
              disabled={busy}
              onClick={() =>
                void action(async () => {
                  const run = await services.schedules!.trigger(
                    connection.epoch,
                    detail.task,
                  );
                  if (epoch.current === connection.epoch) {
                    setDetail((previous) =>
                      previous && previous.task.id === detail.task.id
                        ? {
                            ...previous,
                            runs: [
                              run,
                              ...previous.runs.filter(
                                (item) => item.id !== run.id,
                              ),
                            ],
                          }
                        : previous,
                    );
                    setNotice(
                      localized((m) => m.runAcceptedNotice(run.id, run.status)),
                    );
                  }
                })
              }
            >
              <Play size={14} /> {m.desktopRunNow()}
            </button>
            <button
              disabled={busy}
              onClick={() =>
                void action(async () => {
                  const value = await services.schedules!.read(
                    connection.epoch,
                    detail.task.id,
                  );
                  if (epoch.current === connection.epoch) setDetail(value);
                })
              }
            >
              <RefreshCw size={14} /> {m.desktopRefreshHistory()}
            </button>
            <button
              disabled={busy}
              onClick={() => {
                if (window.confirm(m.scheduleDeleteConfirm(detail.task.id)))
                  void action(async () => {
                    await services.schedules!.remove(
                      connection.epoch,
                      detail.task,
                    );
                    setDetail(undefined);
                  });
              }}
            >
              <Trash2 size={14} /> {m.delete()}
            </button>
          </div>
          <p>
            {detail.count === undefined
              ? m.scheduleCountUnavailable()
              : `${detail.count} runs`}
          </p>
          <p>
            {detail.runs.length} {m.desktopRunsShown()}
            {detail.hasMore === undefined
              ? ' · History pagination metadata unavailable'
              : detail.hasMore
                ? ' · More available'
                : ' · History page complete'}
          </p>
          {detail.hasMore === true && typeof detail.nextOffset === 'number' && (
            <button
              className="text-button"
              disabled={busy}
              onClick={() =>
                void action(async () => {
                  const page = await services.schedules!.runs(
                    connection.epoch,
                    detail.task.id,
                    detail.nextOffset!,
                  );
                  if (epoch.current === connection.epoch)
                    setDetail({
                      ...detail,
                      ...page,
                      runs: [...detail.runs, ...page.runs],
                    });
                })
              }
            >
              {m.desktopLoadMoreHistory()}
            </button>
          )}
          {detail.runs.map((run) => (
            <article className="task-event" key={run.id}>
              <strong>{statusLabel(run.status)}</strong> · {run.startedAt}
              <p>
                {[
                  run.sessionId ? m.runSessionLabel(run.sessionId) : '',
                  run.invocationId
                    ? m.runInvocationLabel(run.invocationId)
                    : '',
                  run.output || '',
                ]
                  .filter(Boolean)
                  .join(' · ')}
              </p>
              {run.error && <p role="alert">{run.error}</p>}
            </article>
          ))}
        </section>
      )}
      {draft && connection && services.schedules && (
        <form
          className="task-detail-card desktop-form"
          onSubmit={(event) => {
            event.preventDefault();
            void action(async () => {
              const next = draft.id
                ? await services.schedules!.update(connection.epoch, draft)
                : await services.schedules!.create(connection.epoch, draft);
              setDraft(undefined);
              setDetail({ task: next, runs: [] });
            });
          }}
        >
          <h2>{draft.id ? m.scheduleEdit() : m.desktopNewScheduledTask()}</h2>
          <label>
            {m.desktopName()}
            <input
              required
              value={draft.name}
              onChange={(event) =>
                setDraft({ ...draft, name: event.target.value })
              }
            />
          </label>
          <label>
            {m.desktopAgentIDOptional()}
            <input
              value={draft.agentId}
              onChange={(event) =>
                setDraft({ ...draft, agentId: event.target.value })
              }
            />
          </label>
          <label>
            {m.desktopPrompt()}
            <textarea
              required
              value={draft.prompt}
              onChange={(event) =>
                setDraft({ ...draft, prompt: event.target.value })
              }
            />
          </label>
          <label>
            {m.desktopScheduleType()}
            <select
              value={draft.schedule.type}
              onChange={(event) =>
                setDraft({ ...draft, schedule: { type: event.target.value } })
              }
            >
              {['Once', 'Interval', 'Daily', 'Weekly', 'Monthly', 'Cron'].map(
                (type) => (
                  <option key={type} value={type}>
                    {typeLabels[type]}
                  </option>
                ),
              )}
            </select>
          </label>
          {draft.schedule.type === 'Cron' && (
            <label>
              {m.desktopCronExpression()}
              <input
                required
                placeholder="0 9 * * *"
                value={draft.schedule.cronExpression || ''}
                onChange={(event) =>
                  setDraft({
                    ...draft,
                    schedule: {
                      ...draft.schedule,
                      cronExpression: event.target.value,
                    },
                  })
                }
              />
            </label>
          )}
          {draft.schedule.type === 'Once' && (
            <label>
              {m.desktopRunAtRFC3339()}
              <input
                required
                placeholder="2026-10-26T09:00:00+08:00"
                value={draft.schedule.runAt || ''}
                onChange={(event) =>
                  setDraft({
                    ...draft,
                    schedule: { ...draft.schedule, runAt: event.target.value },
                  })
                }
              />
            </label>
          )}
          {draft.schedule.type === 'Interval' && (
            <label>
              {m.desktopIntervalSeconds()}
              <input
                required
                type="number"
                min={30}
                max={31536000}
                value={draft.schedule.intervalSeconds || ''}
                onChange={(event) =>
                  setDraft({
                    ...draft,
                    schedule: {
                      ...draft.schedule,
                      intervalSeconds: Number(event.target.value),
                    },
                  })
                }
              />
            </label>
          )}
          {['Daily', 'Weekly', 'Monthly'].includes(draft.schedule.type) && (
            <label>
              {m.scheduleTime()}
              <input
                required
                type="time"
                value={draft.schedule.time || ''}
                onChange={(event) =>
                  setDraft({
                    ...draft,
                    schedule: { ...draft.schedule, time: event.target.value },
                  })
                }
              />
            </label>
          )}
          {draft.schedule.type === 'Weekly' && (
            <label>
              {m.desktopWeekdays1MondayThrough7Sunday()}
              <input
                required
                value={draft.schedule.weekdays?.join(',') || ''}
                onChange={(event) =>
                  setDraft({
                    ...draft,
                    schedule: {
                      ...draft.schedule,
                      weekdays: event.target.value.split(',').map(Number),
                    },
                  })
                }
              />
            </label>
          )}
          {draft.schedule.type === 'Monthly' && (
            <label>
              {m.desktopMonthDays1Through31()}
              <input
                required
                value={draft.schedule.monthDays?.join(',') || ''}
                onChange={(event) =>
                  setDraft({
                    ...draft,
                    schedule: {
                      ...draft.schedule,
                      monthDays: event.target.value.split(',').map(Number),
                    },
                  })
                }
              />
            </label>
          )}
          <label>
            {m.desktopTimeZone()}
            <input
              required
              value={draft.timezone}
              onChange={(event) =>
                setDraft({ ...draft, timezone: event.target.value })
              }
            />
          </label>
          <label>
            {m.desktopDeliveryChannel()}
            <select
              value={draft.delivery?.channel || 'Web'}
              onChange={(event) =>
                setDraft({
                  ...draft,
                  delivery: {
                    channel: event.target.value,
                    receiveIdType: 'chat_id',
                    bestEffort: true,
                  },
                })
              }
            >
              <option value="Web">{m.desktopWeb()}</option>
              <option value="Feishu">{m.desktopFeishu()}</option>
            </select>
          </label>
          {draft.delivery?.channel === 'Feishu' && (
            <>
              <label>
                {m.desktopFeishuAppID()}
                <input
                  value={draft.delivery.appId || ''}
                  onChange={(event) =>
                    setDraft({
                      ...draft,
                      delivery: {
                        ...draft.delivery!,
                        appId: event.target.value,
                      },
                    })
                  }
                />
              </label>
              <button
                type="button"
                className="text-button"
                disabled={busy}
                onClick={() =>
                  void action(async () => {
                    const value = await services.schedules!.deliveryTargets(
                      connection.epoch,
                      'Feishu',
                      draft.delivery?.appId || '',
                    );
                    if (epoch.current === connection.epoch) setTargets(value);
                  })
                }
              >
                {m.desktopReadPermittedDeliveryTargets()}
              </button>
              <label>
                {m.desktopDeliveryTarget()}
                <select
                  required
                  value={draft.delivery.targetId || ''}
                  onChange={(event) =>
                    setDraft({
                      ...draft,
                      delivery: {
                        ...draft.delivery!,
                        targetId: event.target.value,
                      },
                    })
                  }
                >
                  <option value="">{m.desktopChooseAPermittedGroup()}</option>
                  {targets.map((target) => (
                    <option
                      key={`${target.appId}:${target.targetId}`}
                      value={target.targetId}
                    >
                      {target.name || target.targetId}
                    </option>
                  ))}
                </select>
              </label>
            </>
          )}
          <button className="primary" disabled={busy}>
            {m.desktopSaveSchedule()}
          </button>
          <button
            type="button"
            className="text-button"
            onClick={() => setDraft(undefined)}
          >
            {m.cancel()}
          </button>
        </form>
      )}
    </main>
  );
}
