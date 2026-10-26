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
export interface Projection {
  scope: StreamScope;
  messages: DisplayMessage[];
  seen: Set<string>;
  invocationId?: string;
  running: boolean;
}
export function emptyProjection(scope: StreamScope): Projection {
  return { scope, messages: [], seen: new Set(), running: false };
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
      running: state.invocationId !== event.invocationId && state.running,
    };
  if (!event.text) return { ...state, seen };
  const partial = state.messages.find(
    (message) =>
      message.invocationId === event.invocationId &&
      message.role === (event.role ?? 'assistant') &&
      message.partial,
  );
  const id = `${event.invocationId}:${event.blockId ?? (partial ? partial.id.slice(event.invocationId.length + 1) : event.partial ? `${event.role ?? 'assistant'}:partial` : (event.eventId ?? event.kind))}`;
  const existing = state.messages.find((message) => message.id === id);
  const messages = event.partial
    ? state.messages
    : state.messages.filter(
        (message) =>
          message.invocationId !== event.invocationId ||
          !message.partial ||
          message.id === id,
      );
  const next: DisplayMessage = {
    id,
    invocationId: event.invocationId,
    role: event.role ?? 'assistant',
    kind: event.kind,
    partial: !!event.partial,
    content:
      event.partial && existing?.partial
        ? existing.content + event.text
        : event.text,
  };
  return {
    ...state,
    seen,
    invocationId: event.invocationId,
    running: !!event.partial && !event.terminal,
    messages: existing
      ? messages.map((message) => (message.id === id ? next : message))
      : [...messages, next],
  };
}
