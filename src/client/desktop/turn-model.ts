import type { DisplayMessage, SafeCommandSummary } from './projection';

export type TurnStatus = 'running' | 'completed' | 'failed' | 'stopped';
export type ToolStatus =
  'preparing' | 'running' | 'success' | 'failed' | 'stopped';

export type ConversationSegment =
  | {
      type: 'text';
      id: string;
      role: string;
      content: string;
      kind: string;
      partial: boolean;
      status?: TurnStatus;
      durationMs?: number;
    }
  | {
      type: 'reasoning';
      id: string;
      visibility: 'hidden' | 'summary' | 'detail';
      content: string;
      partial: boolean;
      status?: TurnStatus;
      durationMs?: number;
    }
  | {
      type: 'tool';
      id: string;
      toolId: string;
      name: string;
      status: ToolStatus;
      subject?: string;
      durationMs?: number;
      safeCommandSummary?: SafeCommandSummary;
      outputPreview?: string;
      stderrPreview?: string;
    }
  | {
      type: 'status';
      id: string;
      status: TurnStatus;
      content: string;
      durationMs?: number;
    };

export interface ConversationTurn {
  invocationId: string;
  status: TurnStatus;
  segments: ConversationSegment[];
}

function terminalStatus(message: DisplayMessage): TurnStatus | undefined {
  if (message.kind !== 'status') return undefined;
  return message.terminalStatus ?? 'completed';
}

function segment(message: DisplayMessage): ConversationSegment {
  if (message.kind === 'reasoning') {
    return {
      type: 'reasoning',
      id: message.id,
      visibility: message.reasoningVisibility ?? 'hidden',
      content: message.content,
      partial: message.partial,
      status: message.stepStatus ?? (message.partial ? 'running' : 'completed'),
      durationMs: message.durationMs,
    };
  }
  if (
    message.kind === 'toolCall' ||
    message.kind === 'toolResult' ||
    message.kind === 'tool_call' ||
    message.kind === 'tool_result'
  ) {
    return {
      type: 'tool',
      id: message.id,
      toolId: message.toolId ?? message.id,
      name: message.toolName ?? 'tool',
      status:
        message.toolStatus ??
        (message.kind.toLowerCase().includes('result') ? 'success' : 'running'),
      ...(message.toolSubject || message.safeCommandSummary?.commandPreview
        ? {
            subject:
              message.toolSubject ?? message.safeCommandSummary?.commandPreview,
          }
        : {}),
      durationMs: message.durationMs,
      safeCommandSummary: message.safeCommandSummary,
      outputPreview: message.outputPreview,
      stderrPreview: message.stderrPreview,
    };
  }
  const status = terminalStatus(message);
  if (status) {
    return {
      type: 'status',
      id: message.id,
      status,
      content: message.content,
      durationMs: message.durationMs,
    };
  }
  return {
    type: 'text',
    id: message.id,
    role: message.role,
    content: message.content,
    kind: message.kind,
    partial: message.partial,
    status: message.stepStatus ?? (message.partial ? 'running' : 'completed'),
    durationMs: message.durationMs,
  };
}

export function buildConversationTurns(
  messages: DisplayMessage[],
  completedInvocationIds: readonly string[] = [],
): ConversationTurn[] {
  const turns: ConversationTurn[] = [];
  const byInvocation = new Map<string, ConversationTurn>();
  const completed = new Set(completedInvocationIds);
  for (const message of messages) {
    let turn = byInvocation.get(message.invocationId);
    if (!turn) {
      turn = {
        invocationId: message.invocationId,
        status: completed.has(message.invocationId) ? 'completed' : 'running',
        segments: [],
      };
      byInvocation.set(message.invocationId, turn);
      turns.push(turn);
    }
    const next = segment(message);
    turn.segments.push(next);
    if (
      next.type === 'status' &&
      (next.status !== 'completed' ||
        turn.status === 'running' ||
        turn.status === 'completed')
    )
      turn.status = next.status;
  }
  for (const turn of turns)
    turn.segments = turn.segments.map((segment) =>
      settleSegment(segment, turn.status),
    );
  return turns;
}

function settleSegment(
  segment: ConversationSegment,
  status: TurnStatus,
): ConversationSegment {
  if (status === 'running') return segment;
  if (
    (segment.type === 'text' || segment.type === 'reasoning') &&
    (segment.partial || segment.status === 'running')
  )
    return { ...segment, partial: false, status };
  if (
    segment.type === 'tool' &&
    ['preparing', 'running'].includes(segment.status)
  )
    return { ...segment, status: status === 'failed' ? 'failed' : 'stopped' };
  return segment;
}

export function finalAssistantText(
  turn: ConversationTurn,
): Extract<ConversationSegment, { type: 'text' }> | undefined {
  for (let index = turn.segments.length - 1; index >= 0; index -= 1) {
    const candidate = turn.segments[index];
    if (
      candidate.type === 'text' &&
      candidate.role !== 'user' &&
      !/^\[usage\]/iu.test(candidate.content)
    )
      return candidate;
  }
  return undefined;
}

export function processSegments(turn: ConversationTurn): ConversationSegment[] {
  const finalText = finalAssistantText(turn);
  const finalTextId = finalText?.id;
  const finalContent = finalText?.content.trim() || '';
  return turn.segments
    .filter((segment) => {
      if (segment.type === 'status') return false;
      if (segment.type !== 'text') return true;
      if (segment.role === 'user') return false;
      if (segment.id === finalTextId) return false;
      if (/^\[usage\]/iu.test(segment.content)) return true;
      const content = segment.content.trim();
      if (
        finalContent &&
        content.length >= 16 &&
        (finalContent.includes(content) || content.includes(finalContent))
      )
        return false;
      return true;
    })
    .map((segment) => settleSegment(segment, turn.status));
}
