import { useMessages } from './locale';
import type { RunVM } from './models';

function statusLabel(status: string, m: ReturnType<typeof useMessages>) {
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
}

export function ScheduleRunHistory({
  count,
  hasMore,
  nextOffset,
  runs,
  busy,
  onLoadMore,
}: {
  count?: number;
  hasMore?: boolean;
  nextOffset?: number;
  runs: RunVM[];
  busy: boolean;
  onLoadMore(offset: number): void;
}) {
  const m = useMessages();
  return (
    <>
      <p>
        {count === undefined ? m.scheduleCountUnavailable() : `${count} runs`}
      </p>
      <p>
        {runs.length} {m.desktopRunsShown()}
        {hasMore === undefined
          ? ' · History pagination metadata unavailable'
          : hasMore
            ? ' · More available'
            : ' · History page complete'}
      </p>
      {hasMore === true && typeof nextOffset === 'number' && (
        <button
          className="text-button"
          disabled={busy}
          onClick={() => onLoadMore(nextOffset)}
        >
          {m.desktopLoadMoreHistory()}
        </button>
      )}
      {runs.map((run) => (
        <article className="task-event" key={run.id}>
          <strong>{statusLabel(run.status, m)}</strong> · {run.startedAt}
          <p>
            {[
              run.sessionId ? m.runSessionLabel(run.sessionId) : '',
              run.invocationId ? m.runInvocationLabel(run.invocationId) : '',
              run.output || '',
            ]
              .filter(Boolean)
              .join(' · ')}
          </p>
          {run.error && <p role="alert">{run.error}</p>}
        </article>
      ))}
    </>
  );
}
