import { useEffect, useRef, useState } from 'react';
import { Play, Power, PowerOff, RefreshCw } from 'lucide-react';
import { ModalDialog } from '../ui/ModalDialog';
import { Button } from '../ui';
import { useMessages, useMessageState } from './locale';
import { scheduleFailure } from './schedule-failure';
import { formatTime, scheduleStatusLabel, scheduleText } from './schedule-view';
import type { DesktopServices, RunVM, ScheduleVM } from './models';

type Detail = Awaited<
  ReturnType<NonNullable<DesktopServices['schedules']>['read']>
>;
export function ScheduleDetail({
  task,
  services,
  epoch,
  busy,
  error: operationError,
  onRun,
  onToggle,
  onEdit,
  onDelete,
  onClose,
}: {
  task: ScheduleVM;
  services: DesktopServices;
  epoch: number;
  busy: boolean;
  error: string;
  onRun(task: ScheduleVM): Promise<RunVM | undefined>;
  onToggle(task: ScheduleVM): void;
  onEdit(task: ScheduleVM): void;
  onDelete(task: ScheduleVM): void;
  onClose(): void;
}) {
  const m = useMessages();
  const [detail, setDetail] = useState<Detail>({ task, runs: [] });
  const [loading, setLoading] = useState(true);
  const [error, setError] = useMessageState();
  const [reload, setReload] = useState(0);
  const alive = useRef(true);
  const receipts = useRef<RunVM[]>([]);
  useEffect(() => {
    alive.current = true;
    return () => {
      alive.current = false;
    };
  }, []);
  const combine = (runs: RunVM[]) => [
    ...new Map(
      [...receipts.current, ...runs].map((run) => [run.id, run]),
    ).values(),
  ];
  useEffect(() => {
    let current = true;
    setLoading(true);
    setError('');
    void services
      .schedules!.read(epoch, task.id)
      .then((value) => {
        if (current) setDetail({ ...value, runs: combine(value.runs) });
      })
      .catch((reason) => {
        if (current) setError(scheduleFailure(reason));
      })
      .finally(() => {
        if (current) setLoading(false);
      });
    return () => {
      current = false;
    };
  }, [services, epoch, task.id, reload]);
  const latest =
    Number(detail.task.version) > Number(task.version) ? detail.task : task;
  return (
    <ModalDialog
      title={task.name}
      className="schedule-dialog"
      closeLabel={m.closeDialog(m.scheduledTasks())}
      onClose={onClose}
    >
      <div className="schedule-detail">
        {operationError && (
          <p role="alert" className="error-banner">
            {operationError}
          </p>
        )}
        <dl>
          <dt>{m.desktopPrompt()}</dt>
          <dd className="schedule-prompt">{latest.prompt}</dd>
          <dt>{m.scheduleExecutionPlan()}</dt>
          <dd>{scheduleText(latest, m)}</dd>
          <dt>{m.scheduleNextRun()}</dt>
          <dd>{formatTime(task.nextRun, undefined, task.timezone)}</dd>
          <dt>{m.scheduleAgent()}</dt>
          <dd>{latest.agentId || 'Main'}</dd>
        </dl>
        <div className="schedule-actions">
          <Button
            disabled={busy || loading}
            onClick={async () => {
              const run = await onRun(latest);
              if (run && alive.current) {
                receipts.current.unshift(run);
                setDetail((current) => ({
                  ...current,
                  runs: combine(current.runs),
                }));
              }
            }}
            leading={<Play size={14} />}
          >
            {m.desktopRunNow()}
          </Button>
          <Button
            disabled={busy || loading}
            onClick={() => onToggle(latest)}
            leading={
              latest.enabled ? (
                <PowerOff size={14} aria-hidden="true" />
              ) : (
                <Power size={14} aria-hidden="true" />
              )
            }
          >
            {latest.enabled ? m.scheduleDisable() : m.scheduleEnable()}
          </Button>
          <Button disabled={busy || loading} onClick={() => onEdit(latest)}>
            {m.desktopEdit()}
          </Button>
          <Button
            variant="destructive"
            disabled={busy || loading}
            onClick={() => onDelete(latest)}
          >
            {m.delete()}
          </Button>
        </div>
        <div className="schedule-history-heading">
          <h3>{m.scheduleHistory()}</h3>
          <Button
            disabled={loading || busy}
            onClick={() => setReload((value) => value + 1)}
            leading={<RefreshCw size={14} />}
          >
            {m.desktopRefreshHistory()}
          </Button>
        </div>
        {error && (
          <p role="alert" className="error-banner">
            {error}
          </p>
        )}
        {loading && <p role="status">{m.working()}</p>}
        {!loading && !error && !detail.runs.length && (
          <p className="schedule-hint">{m.scheduleNoHistory()}</p>
        )}
        {detail.count !== undefined && (
          <p className="schedule-hint">
            {m.scheduleExecutionCount(String(detail.count))}
          </p>
        )}
        {detail.runs.map((run) => (
          <article className="task-event" key={run.id}>
            <strong>{scheduleStatusLabel(run.status, m)}</strong> ·{' '}
            {formatTime(
              run.startedAt || run.scheduledAt,
              undefined,
              task.timezone,
            )}
            {run.durationMs != null && (
              <span>
                {' '}
                · {m.scheduleDuration()} {run.durationMs} ms
              </span>
            )}
            <p>
              {[
                run.sessionId ? m.runSessionLabel(run.sessionId) : '',
                run.invocationId ? m.runInvocationLabel(run.invocationId) : '',
                run.output || '',
              ]
                .filter(Boolean)
                .join(' · ')}
            </p>
            <small>{run.id}</small>
            {run.error && <p className="error-banner">{run.error}</p>}
          </article>
        ))}
        {detail.hasMore && typeof detail.nextOffset === 'number' && (
          <Button
            disabled={loading || busy}
            onClick={async () => {
              setLoading(true);
              setError('');
              try {
                const page = await services.schedules!.runs(
                  epoch,
                  task.id,
                  detail.nextOffset!,
                );
                if (alive.current)
                  setDetail({
                    ...detail,
                    ...page,
                    runs: combine([...detail.runs, ...page.runs]),
                  });
              } catch (reason) {
                if (alive.current) setError(scheduleFailure(reason));
              } finally {
                if (alive.current) setLoading(false);
              }
            }}
          >
            {m.desktopLoadMoreHistory()}
          </Button>
        )}
      </div>
    </ModalDialog>
  );
}
