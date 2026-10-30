export interface StreamScope {
  connectionEpoch: number;
  subscriptionEpoch: number;
  sessionId: string;
}
export interface DisplayMessage {
  id: string;
  role: string;
  content: string;
  invocationId: string;
  partial: boolean;
  kind: string;
  reasoningVisibility?: 'hidden' | 'summary' | 'detail';
  toolId?: string;
  toolName?: string;
  toolStatus?: 'preparing' | 'running' | 'success' | 'failed' | 'stopped';
  toolSubject?: string;
  startedAt?: string;
  endedAt?: string;
  durationMs?: number;
  safeCommandSummary?: SafeCommandSummary;
  outputPreview?: string;
  stderrPreview?: string;
  terminalStatus?: 'completed' | 'failed' | 'stopped';
  /** A closed text/reasoning step, independent of whole-turn completion. */
  stepStatus?: 'completed' | 'failed' | 'stopped';
}
export interface SafeCommandParameter {
  name: string;
  value: string;
}
export interface SafeCommandSummary {
  binary?: string;
  commandPreview?: string;
  argumentCount?: number;
  redacted?: boolean;
  parameters?: SafeCommandParameter[];
}
export interface DisplayEvent extends StreamScope {
  schemaVersion?: number;
  invocationId: string;
  eventId?: string;
  kind: string;
  blockId?: string;
  partIndex?: number;
  role?: string;
  text?: string;
  reasoningVisibility?: 'hidden' | 'summary' | 'detail';
  toolId?: string;
  toolName?: string;
  toolStatus?: 'preparing' | 'running' | 'success' | 'failed' | 'stopped';
  toolSubject?: string;
  startedAt?: string;
  endedAt?: string;
  durationMs?: number;
  safeCommandSummary?: SafeCommandSummary;
  outputPreview?: string;
  stderrPreview?: string;
  partial?: boolean;
  terminal?: boolean;
  terminalStatus?: 'completed' | 'failed' | 'stopped';
}
export interface StreamSlot {
  invocationId: string;
  author: string;
  kind: 'text' | 'reasoning';
  slot: number;
}
export interface Projection {
  scope: StreamScope;
  messages: DisplayMessage[];
  seen: Set<string>;
  invocationId?: string;
  running: boolean;
  /** The open partial-text slot; closed by final text, tools or terminal. */
  stream?: StreamSlot;
  /** Monotonic slot counter so a closed slot is never reused. */
  streamSlot: number;
}
export function emptyProjection(scope: StreamScope): Projection {
  return {
    scope,
    messages: [],
    seen: new Set(),
    running: false,
    streamSlot: 0,
  };
}
function streamID(invocationId: string, author: string, slot: number) {
  return `${invocationId}:stream:${author}:${slot}`;
}
function isToolKind(kind: string) {
  return ['toolCall', 'toolResult', 'tool_call', 'tool_result'].includes(kind);
}
function activeSlot(
  state: Projection,
  invocationId: string,
  author: string,
  kind: StreamSlot['kind'],
): StreamSlot | undefined {
  return state.stream &&
    state.stream.invocationId === invocationId &&
    state.stream.author === author &&
    state.stream.kind === kind
    ? state.stream
    : undefined;
}
function replaceMessage(
  messages: DisplayMessage[],
  id: string,
  next: DisplayMessage,
) {
  return messages.map((message) => (message.id === id ? next : message));
}
function metadataFromEvent(event: DisplayEvent) {
  return {
    startedAt: event.startedAt,
    endedAt: event.endedAt,
    durationMs: event.durationMs,
    safeCommandSummary: event.safeCommandSummary,
    outputPreview: event.outputPreview,
    stderrPreview: event.stderrPreview,
  };
}
function mergedMetadata(
  event: DisplayEvent,
  existing?: DisplayMessage,
): Pick<
  DisplayMessage,
  | 'startedAt'
  | 'endedAt'
  | 'durationMs'
  | 'safeCommandSummary'
  | 'outputPreview'
  | 'stderrPreview'
> {
  const incoming = metadataFromEvent(event);
  return {
    startedAt: incoming.startedAt ?? existing?.startedAt,
    endedAt: incoming.endedAt ?? existing?.endedAt,
    durationMs: incoming.durationMs ?? existing?.durationMs,
    safeCommandSummary:
      incoming.safeCommandSummary ?? existing?.safeCommandSummary,
    outputPreview: incoming.outputPreview ?? existing?.outputPreview,
    stderrPreview: incoming.stderrPreview ?? existing?.stderrPreview,
  };
}
export function projectEvent(
  state: Projection,
  event: DisplayEvent,
): Projection {
  if (
    event.connectionEpoch !== state.scope.connectionEpoch ||
    event.subscriptionEpoch !== state.scope.subscriptionEpoch ||
    event.sessionId !== state.scope.sessionId
  )
    return state;
  if (
    state.scope.subscriptionEpoch > 0 &&
    state.invocationId &&
    event.invocationId !== state.invocationId
  )
    return state;
  const identity = event.eventId
    ? `${event.invocationId}:${event.eventId}:${event.kind}:${event.blockId ?? ''}:${event.partIndex ?? ''}`
    : undefined;
  if (identity && state.seen.has(identity)) return state;
  const seen = new Set(state.seen);
  if (identity) seen.add(identity);
  if (event.kind === 'heartbeat') return state;
  if (event.kind === 'done' && !event.terminal) return state;
  if (event.kind === 'error' && !event.terminal) return { ...state, seen };
  if (event.terminal) {
    const requestedStatus =
      event.terminalStatus ??
      (event.kind === 'error'
        ? 'failed'
        : event.kind === 'stopped'
          ? 'stopped'
          : 'completed');
    const previousStatus = state.messages.find(
      (message) => message.id === `${event.invocationId}:terminal`,
    );
    const preserveStatus =
      requestedStatus === 'completed' &&
      (previousStatus?.terminalStatus === 'failed' ||
        previousStatus?.terminalStatus === 'stopped');
    const terminalStatus = preserveStatus
      ? previousStatus.terminalStatus!
      : requestedStatus;
    // Some Runtime events carry both the answer and terminal authority.
    // Project the content first so neither the final answer nor its open slot is lost.
    const contentState =
      event.kind !== 'done' && event.kind !== 'error'
        ? projectEvent(state, { ...event, terminal: false, partial: false })
        : state;
    const messages = contentState.messages.map((message) => {
      if (message.invocationId !== event.invocationId) return message;
      if (
        isToolKind(message.kind) &&
        ['preparing', 'running'].includes(
          message.toolStatus ??
            (message.kind.toLowerCase().includes('result')
              ? 'success'
              : 'running'),
        )
      )
        return {
          ...message,
          toolStatus:
            terminalStatus === 'failed'
              ? ('failed' as const)
              : ('stopped' as const),
        };
      if (message.partial)
        return { ...message, partial: false, stepStatus: terminalStatus };
      return message;
    });
    const status: DisplayMessage = preserveStatus
      ? {
          ...previousStatus,
          ...mergedMetadata(event, previousStatus),
        }
      : {
          id: `${event.invocationId}:terminal`,
          invocationId: event.invocationId,
          role: 'assistant',
          kind: 'status',
          partial: false,
          content:
            event.kind === 'done' || event.kind === 'error'
              ? (event.text ?? '')
              : '',
          terminalStatus,
          ...metadataFromEvent(event),
        };
    const existingStatus = messages.findIndex(
      (message) => message.id === status.id,
    );
    if (existingStatus >= 0) messages[existingStatus] = status;
    else messages.push(status);
    return {
      ...contentState,
      seen,
      stream: undefined,
      running: state.invocationId !== event.invocationId && state.running,
      messages,
    };
  }

  const role = event.role ?? 'assistant';
  const invocationId = event.invocationId;

  // Switching from reasoning to text/tools (or from text to reasoning/tools)
  // closes only the preceding step. It is not a turn terminal.
  const incomingKind = event.kind === 'reasoning' ? 'reasoning' : 'text';
  if (
    state.stream &&
    (isToolKind(event.kind) || event.kind === 'reasoning' || event.text) &&
    (isToolKind(event.kind) ||
      !activeSlot(state, invocationId, role, incomingKind))
  ) {
    const id = streamID(
      state.stream.invocationId,
      state.stream.author,
      state.stream.slot,
    );
    state = {
      ...state,
      stream: undefined,
      messages: state.messages.map((message) =>
        message.id === id && message.partial
          ? { ...message, partial: false, stepStatus: 'completed' }
          : message,
      ),
    };
  }

  // A tool call or result closes the current partial slot, so text before and
  // after a tool never collapse into one bubble.
  if (isToolKind(event.kind)) {
    const toolId = event.toolId ?? event.blockId ?? event.eventId ?? event.kind;
    const id = `${invocationId}:tool:${toolId}`;
    const existing = state.messages.find(
      (message) =>
        message.invocationId === invocationId &&
        (message.id === id ||
          (event.toolId && message.toolId === event.toolId)),
    );
    const incomingStatus =
      event.toolStatus ??
      (event.kind === 'toolResult' || event.kind === 'tool_result'
        ? 'success'
        : 'running');
    const existingSettled = ['success', 'failed', 'stopped'].includes(
      existing?.toolStatus ?? '',
    );
    const incomingSettled = ['success', 'failed', 'stopped'].includes(
      incomingStatus,
    );
    const next: DisplayMessage = {
      id,
      invocationId,
      role,
      kind:
        existingSettled && !incomingSettled
          ? (existing?.kind ?? event.kind)
          : event.kind,
      partial: false,
      content: event.text ?? existing?.content ?? '',
      toolId,
      toolName: event.toolName ?? existing?.toolName,
      toolStatus:
        existingSettled && !incomingSettled
          ? existing?.toolStatus
          : incomingStatus,
      toolSubject: event.toolSubject ?? existing?.toolSubject,
      ...mergedMetadata(event, existing),
    };
    return {
      ...state,
      seen,
      stream: undefined,
      invocationId,
      messages: existing
        ? replaceMessage(state.messages, existing.id, next)
        : [...state.messages, next],
    };
  }

  if (event.kind === 'reasoning') {
    const visibility = event.reasoningVisibility ?? 'hidden';
    const text = event.text ?? '';
    const active = activeSlot(state, invocationId, role, 'reasoning');
    const slot = active ? active.slot : state.streamSlot + 1;
    const id = streamID(invocationId, role, slot);
    const existing = state.messages.find((message) => message.id === id);
    const next: DisplayMessage = {
      id,
      invocationId,
      role,
      kind: 'reasoning',
      partial: event.partial ?? false,
      content: event.partial ? (existing?.content ?? '') + text : text,
      reasoningVisibility: visibility,
      ...mergedMetadata(event, existing),
    };
    return {
      ...state,
      seen,
      invocationId,
      running: event.partial || state.running,
      streamSlot: slot,
      stream: event.partial
        ? { invocationId, author: role, kind: 'reasoning', slot }
        : undefined,
      messages: existing
        ? replaceMessage(state.messages, id, next)
        : [...state.messages, next],
    };
  }

  if (!event.text) return { ...state, seen };

  if (event.partial) {
    const active = activeSlot(state, invocationId, role, 'text');
    const slot = active ? active.slot : state.streamSlot + 1;
    const id = streamID(invocationId, role, slot);
    const existing = state.messages.find((message) => message.id === id);
    const next: DisplayMessage = {
      id,
      invocationId,
      role,
      kind: event.kind,
      partial: true,
      content: (existing?.content ?? '') + event.text,
      ...mergedMetadata(event, existing),
    };
    return {
      ...state,
      seen,
      invocationId,
      running: true,
      streamSlot: slot,
      stream: { invocationId, author: role, kind: 'text', slot },
      messages: existing
        ? replaceMessage(state.messages, id, next)
        : [...state.messages, next],
    };
  }

  // Final text replaces the open partial slot when one exists.
  const active = activeSlot(state, invocationId, role, 'text');
  if (active) {
    const id = streamID(invocationId, role, active.slot);
    const existing = state.messages.find((message) => message.id === id);
    if (existing) {
      const next: DisplayMessage = {
        ...existing,
        kind: event.kind,
        partial: false,
        content: event.text,
        ...mergedMetadata(event, existing),
      };
      return {
        ...state,
        seen,
        invocationId,
        running: state.running,
        stream: undefined,
        messages: replaceMessage(state.messages, id, next),
      };
    }
  }

  // A server echo of the optimistic user message replaces it instead of
  // appending a second bubble for the same invocation.
  if (role === 'user') {
    const optimistic = state.messages.find(
      (message) =>
        message.invocationId === invocationId &&
        message.role === 'user' &&
        !message.partial,
    );
    if (optimistic) {
      const next: DisplayMessage = {
        ...optimistic,
        kind: event.kind,
        content: event.text,
        ...mergedMetadata(event, optimistic),
      };
      return {
        ...state,
        seen,
        invocationId,
        running: state.running,
        stream: undefined,
        messages: replaceMessage(state.messages, optimistic.id, next),
      };
    }
  }

  const id = `${invocationId}:${event.blockId ?? event.eventId ?? event.kind}`;
  const existing = state.messages.find((message) => message.id === id);
  const next: DisplayMessage = {
    id,
    invocationId,
    role,
    kind: event.kind,
    partial: false,
    content: event.text,
    ...mergedMetadata(event, existing),
  };
  return {
    ...state,
    seen,
    invocationId,
    running: state.running,
    stream: undefined,
    messages: existing
      ? replaceMessage(state.messages, id, next)
      : [...state.messages, next],
  };
}
