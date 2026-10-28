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

function processTitle(turn: ConversationTurn, stepCount: number) {
  const steps = `${stepCount} ${stepCount === 1 ? 'step' : 'steps'}`;
  if (turn.status === 'running') return `Working · ${steps}`;
  if (turn.status === 'failed') return `Execution failed · ${steps}`;
  if (turn.status === 'stopped') return `Stopped · ${steps}`;
  return `Worked · ${steps}`;
}

function toolAction(name: string) {
  const normalized = name.toLowerCase();
  if (['read', 'read_file'].includes(normalized)) return 'Read';
  if (['write', 'write_file'].includes(normalized)) return 'Write';
  if (['edit', 'edit_file'].includes(normalized)) return 'Edit';
  if (['search', 'grep', 'find'].includes(normalized)) return 'Search';
  if (['bash', 'shell', 'run', 'run_command'].includes(normalized))
    return 'Run command';
  return name || 'Tool';
}

function toolStatusLabel(status: ToolStatus) {
  return {
    preparing: 'Preparing',
    running: 'Running',
    success: 'Completed',
    failed: 'Failed',
    stopped: 'Stopped',
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
  const summary =
    segment.visibility === 'hidden'
      ? 'Details hidden'
      : segment.content
          .split(/\r?\n/u)
          .find((line) => line.trim())
          ?.trim() || 'No summary';
  return (
    <StatusRow
      icon={<Brain aria-hidden="true" />}
      title="Reasoning"
      description={summary}
      status={segment.partial ? 'Thinking' : 'Completed'}
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
  const action = toolAction(segment.name);
  return (
    <StatusRow
      icon={<ToolStatusIcon status={segment.status} />}
      title={action}
      description={segment.subject}
      status={toolStatusLabel(segment.status)}
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
            label={`Inspect ${action} in full trace`}
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
  return (
    <StatusRow
      icon={<MessageCircle aria-hidden="true" />}
      title="Progress"
      description={segment.content}
      status="Completed"
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
  const segments = useMemo(() => processSegments(turn), [turn]);
  const userChanged = useRef(false);
  const [open, setOpen] = useState(turn.status === 'running');
  useEffect(() => {
    if (!userChanged.current) setOpen(turn.status === 'running');
  }, [turn.status]);
  if (segments.length === 0) return null;
  const title = processTitle(turn, segments.length);
  return (
    <div className="turn-process" data-state={turn.status}>
      <span className="sr-only" role="status" aria-live="polite">
        {title}
      </span>
      <Disclosure
        open={open}
        label={title}
        ariaLabel="execution process"
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
