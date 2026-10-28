import { useMessages } from './locale';
import type { Messages } from './generated/locales';
import {
  BarChart3,
  Brain,
  CheckCircle2,
  CircleAlert,
  LoaderCircle,
  MessageCircle,
  Square,
  Wrench,
} from 'lucide-react';
import { useEffect, useMemo, useRef, useState } from 'react';
import { Disclosure, IconButton, StatusRow } from '../ui';
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
  if (['bash', 'shell', 'run', 'run_command'].includes(normalized))
    return m.desktopRunCommand();
  return name || m.desktopTool();
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

function ToolStatusIcon({ status }: { status: ToolStatus }) {
  if (status === 'success')
    return <CheckCircle2 size={14} aria-hidden="true" />;
  if (status === 'failed') return <CircleAlert size={14} aria-hidden="true" />;
  if (status === 'stopped') return <Square size={12} aria-hidden="true" />;
  if (status === 'running')
    return (
      <LoaderCircle
        className="turn-process-spin"
        size={14}
        aria-hidden="true"
      />
    );
  return <Wrench size={14} aria-hidden="true" />;
}

function ReasoningRow({
  segment,
}: {
  segment: Extract<ConversationSegment, { type: 'reasoning' }>;
}) {
  const m = useMessages();
  const summary =
    segment.visibility === 'hidden'
      ? m.desktopDetailsHidden()
      : segment.content
          .split(/\r?\n/u)
          .find((line) => line.trim())
          ?.trim() || m.chatNoSummary();
  return (
    <StatusRow
      icon={<Brain aria-hidden="true" />}
      title={m.desktopReasoning()}
      description={summary}
      status={segment.partial ? m.desktopThinking() : m.desktopCompleted()}
      tone={segment.partial ? 'progress' : 'neutral'}
    />
  );
}

function ToolRow({
  segment,
  onInspect,
}: {
  segment: Extract<ConversationSegment, { type: 'tool' }>;
  onInspect?: (toolId: string) => void;
}) {
  const m = useMessages();
  const action = toolAction(segment.name, m);
  return (
    <StatusRow
      icon={<ToolStatusIcon status={segment.status} />}
      title={action}
      description={segment.subject}
      status={toolStatusLabel(segment.status, m)}
      tone={
        segment.status === 'success'
          ? 'success'
          : segment.status === 'failed'
            ? 'destructive'
            : segment.status === 'stopped'
              ? 'warning'
              : 'progress'
      }
      action={
        onInspect ? (
          <IconButton
            className="turn-process-inspect"
            label={m.chatInspectTool(action)}
            size="icon-sm"
            variant="ghost"
            onClick={() => onInspect(segment.toolId)}
          >
            <BarChart3 aria-hidden="true" />
          </IconButton>
        ) : undefined
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
  return (
    <StatusRow
      icon={<MessageCircle aria-hidden="true" />}
      title={m.desktopProgress()}
      description={segment.content}
      status={m.desktopCompleted()}
    />
  );
}

export function TurnProcess({
  turn,
  onInspectTool,
}: {
  turn: ConversationTurn;
  onInspectTool?: (toolId: string) => void;
}) {
  const m = useMessages();
  const segments = useMemo(() => processSegments(turn), [turn]);
  const userChanged = useRef(false);
  const [open, setOpen] = useState(turn.status === 'running');
  useEffect(() => {
    if (!userChanged.current) setOpen(turn.status === 'running');
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
            <ToolRow
              key={segment.id}
              segment={segment}
              onInspect={onInspectTool}
            />
          ) : segment.type === 'text' ? (
            <NarrationRow key={segment.id} segment={segment} />
          ) : null,
        )}
      </Disclosure>
    </div>
  );
}
