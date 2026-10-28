import type { DisplayMessage } from './projection';

export type TurnStatus = 'running' | 'completed' | 'failed' | 'stopped';
export type ToolStatus =
  'preparing' | 'running' | 'success' | 'failed' | 'stopped';

export type ConversationSegment =
  | {
      type: 'text';
      id: string;
      role: string;
      content: string;
      partial: boolean;
    }
  | {
      type: 'reasoning';
      id: string;
      visibility: 'hidden' | 'summary' | 'detail';
      content: string;
      partial: boolean;
    }
  | {
      type: 'tool';
      id: string;
      toolId: string;
      name: string;
      status: ToolStatus;
      subject?: string;
    }
  | {
      type: 'status';
      id: string;
      status: TurnStatus;
      content: string;
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
    };
  }
  if (message.kind === 'toolCall' || message.kind === 'toolResult') {
    return {
      type: 'tool',
      id: message.id,
      toolId: message.toolId ?? message.id,
      name: message.toolName ?? 'tool',
      status:
        message.toolStatus ??
        (message.kind === 'toolResult' ? 'success' : 'running'),
      ...(message.toolSubject ? { subject: message.toolSubject } : {}),
    };
  }
  const status = terminalStatus(message);
  if (status) {
    return { type: 'status', id: message.id, status, content: message.content };
  }
  return {
    type: 'text',
    id: message.id,
    role: message.role,
    content: message.content,
    partial: message.partial,
  };
}

export function buildConversationTurns(
  messages: DisplayMessage[],
): ConversationTurn[] {
  const turns: ConversationTurn[] = [];
  const byInvocation = new Map<string, ConversationTurn>();
  for (const message of messages) {
    let turn = byInvocation.get(message.invocationId);
    if (!turn) {
      turn = {
        invocationId: message.invocationId,
        status: 'running',
        segments: [],
      };
      byInvocation.set(message.invocationId, turn);
      turns.push(turn);
    }
    const next = segment(message);
    turn.segments.push(next);
    if (next.type === 'status') turn.status = next.status;
  }
  return turns;
}

export function finalAssistantText(
  turn: ConversationTurn,
): Extract<ConversationSegment, { type: 'text' }> | undefined {
  for (let index = turn.segments.length - 1; index >= 0; index -= 1) {
    const candidate = turn.segments[index];
    if (candidate.type === 'text' && candidate.role !== 'user')
      return candidate;
  }
  return undefined;
}

export function processSegments(turn: ConversationTurn): ConversationSegment[] {
  const finalTextId = finalAssistantText(turn)?.id;
  return turn.segments.filter(
    (segment) =>
      segment.type !== 'status' &&
      !(segment.type === 'text' && segment.role === 'user') &&
      segment.id !== finalTextId,
  );
}
