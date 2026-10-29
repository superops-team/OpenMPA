import { useMessages } from './locale';
import type { Messages } from './generated/locales';
import { useEffect, useMemo, useRef, useState } from 'react';
import { Disclosure, StatusRow } from '../ui';
import { formatTraceDuration } from './trace-model';
import { processSegments } from './turn-model';
import type {
  ConversationSegment,
  ConversationTurn,
  ToolStatus,
} from './turn-model';

function processTitle(turn: ConversationTurn, stepCount: number, m: Messages) {
  if (turn.status === 'running') return m.chatProcessRunning(stepCount);
  if (turn.status === 'failed') return m.chatProcessFailed(stepCount);
  if (turn.status === 'stopped') return m.chatProcessStopped(stepCount);
  return m.chatProcessCompleted(stepCount);
}

function toolAction(name: string, m: Messages) {
  const normalized = name.toLowerCase();
  if (!name || normalized === 'tool') return m.desktopTool();
  if (['read', 'read_file'].includes(normalized)) return m.desktopRead();
  if (['write', 'write_file'].includes(normalized)) return m.desktopWrite();
  if (['edit', 'edit_file'].includes(normalized)) return m.desktopEdit();
  if (['search', 'grep', 'find'].includes(normalized)) return m.desktopSearch();
  if (['model_proxy', 'modelproxy', 'model_call'].includes(normalized))
    return m.desktopModelCall();
  if (
    [
      'bash',
      'shell',
      'run',
      'run_command',
      'exec_command',
      'commandexecution',
    ].includes(normalized)
  )
    return m.desktopRunCommand();
  return name;
}

function toolStatusLabel(status: ToolStatus, m: Messages) {
  return {
    preparing: m.desktopPreparing(),
    running: m.desktopRunning(),
    success: m.desktopCompleted(),
    failed: m.desktopFailed(),
    stopped: m.desktopStopped(),
  }[status];
}

function statusWithDuration(
  status: string,
  durationMs: number | undefined,
  m: Messages,
) {
  const duration =
    durationMs !== undefined && Number.isFinite(durationMs)
      ? formatTraceDuration(Math.max(0, durationMs), m)
      : '';
  return duration ? `${status} · ${duration}` : status;
}

function usageDescription(content: string, m: Messages) {
  return (
    content.replace(/^\[usage\]\s*/iu, '').trim() ||
    m.desktopTokenUsageUpdated()
  );
}

function ReasoningRow({
  segment,
}: {
  segment: Extract<ConversationSegment, { type: 'reasoning' }>;
}) {
  const m = useMessages();
  const status = segment.status ?? (segment.partial ? 'running' : 'completed');
  return (
    <StatusRow
      title={m.desktopReasoning()}
      description={segment.content.trim() || m.desktopNoReasoningDetails()}
      descriptionMode="wrap"
      status={statusWithDuration(
        status === 'running'
          ? m.desktopThinking()
          : status === 'failed'
            ? m.desktopFailed()
            : status === 'stopped'
              ? m.desktopStopped()
              : m.desktopCompleted(),
        segment.durationMs,
        m,
      )}
      tone={
        status === 'running'
          ? 'progress'
          : status === 'failed'
            ? 'destructive'
            : status === 'stopped'
              ? 'warning'
              : 'neutral'
      }
    />
  );
}

function ToolRow({
  segment,
}: {
  segment: Extract<ConversationSegment, { type: 'tool' }>;
}) {
  const m = useMessages();
  return (
    <StatusRow
      title={toolAction(segment.name, m)}
      description={
        segment.subject ||
        segment.safeCommandSummary?.commandPreview ||
        segment.outputPreview
      }
      descriptionMode="wrap"
      status={statusWithDuration(
        toolStatusLabel(segment.status, m),
        segment.durationMs,
        m,
      )}
      tone={
        segment.status === 'success'
          ? 'success'
          : segment.status === 'failed'
            ? 'destructive'
            : segment.status === 'stopped'
              ? 'warning'
              : 'progress'
      }
    />
  );
}

function NarrationRow({
  segment,
}: {
  segment: Extract<ConversationSegment, { type: 'text' }>;
}) {
  const m = useMessages();
  const usage = /^\[usage\]/iu.test(segment.content);
  const status = segment.status ?? (segment.partial ? 'running' : 'completed');
  return (
    <StatusRow
      title={usage ? m.desktopUsage() : m.desktopProgress()}
      description={
        usage ? usageDescription(segment.content, m) : segment.content
      }
      status={statusWithDuration(
        status === 'running'
          ? m.desktopRunning()
          : status === 'failed'
            ? m.desktopFailed()
            : status === 'stopped'
              ? m.desktopStopped()
              : m.desktopCompleted(),
        segment.durationMs,
        m,
      )}
    />
  );
}

export function TurnProcess({ turn }: { turn: ConversationTurn }) {
  const m = useMessages();
  const segments = useMemo(() => processSegments(turn), [turn]);
  const userChanged = useRef(false);
  const [open, setOpen] = useState(turn.status === 'running');
  useEffect(() => {
    if (!userChanged.current && turn.status === 'running') setOpen(true);
  }, [turn.status]);
  if (segments.length === 0) return null;
  const title = processTitle(turn, segments.length, m);
  return (
    <div className="turn-process" data-state={turn.status}>
      <span className="sr-only" role="status" aria-live="polite">
        {title}
      </span>
      <Disclosure
        open={open}
        label={title}
        ariaLabel={m.desktopExecutionProcess()}
        toggleLabel={
          open
            ? m.collapseSection(m.desktopExecutionProcess())
            : m.expandSection(m.desktopExecutionProcess())
        }
        className="turn-process-disclosure"
        onToggle={() => {
          userChanged.current = true;
          setOpen((current) => !current);
        }}
      >
        {segments.map((segment) =>
          segment.type === 'reasoning' ? (
            <ReasoningRow key={segment.id} segment={segment} />
          ) : segment.type === 'tool' ? (
            <ToolRow key={segment.id} segment={segment} />
          ) : segment.type === 'text' ? (
            <NarrationRow key={segment.id} segment={segment} />
          ) : null,
        )}
      </Disclosure>
    </div>
  );
}
