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
}
export interface DisplayEvent extends StreamScope {
  invocationId: string;
  eventId?: string;
  kind: string;
  blockId?: string;
  role?: string;
  text?: string;
  partial?: boolean;
  terminal?: boolean;
}
export interface StreamSlot {
  invocationId: string;
  author: string;
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
  return kind === 'toolCall' || kind === 'toolResult';
}
function activeSlot(
  state: Projection,
  invocationId: string,
  author: string,
): StreamSlot | undefined {
  return state.stream &&
    state.stream.invocationId === invocationId &&
    state.stream.author === author
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
    ? `${event.invocationId}:${event.eventId}:${event.kind}:${event.blockId ?? ''}`
    : undefined;
  if (identity && state.seen.has(identity)) return state;
  const seen = new Set(state.seen);
  if (identity) seen.add(identity);
  if (event.kind === 'heartbeat') return state;
  if (event.kind === 'done' && !event.terminal) return state;
  if (event.kind === 'error' && !event.terminal) return { ...state, seen };
  if ((event.kind === 'done' || event.kind === 'error') && event.terminal)
    return {
      ...state,
      seen,
      stream: undefined,
      running: state.invocationId !== event.invocationId && state.running,
    };

  const role = event.role ?? 'assistant';
  const invocationId = event.invocationId;

  // A tool call or result closes the current partial slot, so text before and
  // after a tool never collapse into one bubble.
  if (isToolKind(event.kind)) {
    if (!event.text) return { ...state, seen, stream: undefined };
    const id = `${invocationId}:${event.blockId ?? event.eventId ?? event.kind}`;
    const existing = state.messages.find((message) => message.id === id);
    const next: DisplayMessage = {
      id,
      invocationId,
      role,
      kind: event.kind,
      partial: false,
      content: event.text,
    };
    return {
      ...state,
      seen,
      stream: undefined,
      invocationId,
      messages: existing
        ? replaceMessage(state.messages, id, next)
        : [...state.messages, next],
    };
  }

  if (!event.text) return { ...state, seen };

  if (event.partial) {
    const active = activeSlot(state, invocationId, role);
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
    };
    return {
      ...state,
      seen,
      invocationId,
      running: true,
      streamSlot: slot,
      stream: { invocationId, author: role, slot },
      messages: existing
        ? replaceMessage(state.messages, id, next)
        : [...state.messages, next],
    };
  }

  // Final text replaces the open partial slot when one exists.
  const active = activeSlot(state, invocationId, role);
  if (active) {
    const id = streamID(invocationId, role, active.slot);
    const existing = state.messages.find((message) => message.id === id);
    if (existing) {
      const next: DisplayMessage = {
        ...existing,
        kind: event.kind,
        partial: false,
        content: event.text,
      };
      return {
        ...state,
        seen,
        invocationId,
        running: false,
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
      };
      return {
        ...state,
        seen,
        invocationId,
        running: false,
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
  };
  return {
    ...state,
    seen,
    invocationId,
    running: false,
    stream: undefined,
    messages: existing
      ? replaceMessage(state.messages, id, next)
      : [...state.messages, next],
  };
}
