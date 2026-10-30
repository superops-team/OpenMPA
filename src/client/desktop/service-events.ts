import type { ClientEvent } from './generated/mygo';
import {
  emptyProjection,
  projectEvent,
  type DisplayEvent,
  type DisplayMessage,
} from './projection';

function finiteDuration(value?: number | null) {
  return typeof value === 'number' && Number.isFinite(value)
    ? value
    : undefined;
}

export function displayEvent(event: ClientEvent): DisplayEvent {
  const reasoningVisibility = ['hidden', 'summary', 'detail'].includes(
    event.reasoningVisibility || '',
  )
    ? (event.reasoningVisibility as 'hidden' | 'summary' | 'detail')
    : undefined;
  const toolStatus = [
    'preparing',
    'running',
    'success',
    'failed',
    'stopped',
  ].includes(event.toolStatus || '')
    ? (event.toolStatus as
        'preparing' | 'running' | 'success' | 'failed' | 'stopped')
    : undefined;
  return {
    schemaVersion: event.schemaVersion,
    connectionEpoch: event.connectionEpoch,
    subscriptionEpoch: event.subscriptionEpoch,
    sessionId: event.sessionId,
    invocationId: event.invocationId,
    eventId: event.eventId,
    blockId: event.blockId,
    partIndex: event.partIndex,
    kind: event.error
      ? 'error'
      : event.terminal && !event.text
        ? 'done'
        : event.kind,
    text:
      event.error?.message ||
      event.text ||
      (event.toolName
        ? `${event.kind}: ${event.toolName}${event.toolId ? ` (${event.toolId})` : ''}`
        : ''),
    role: event.author === 'user' ? 'user' : 'assistant',
    reasoningVisibility,
    toolId: event.toolId,
    toolName: event.toolName,
    toolStatus,
    toolSubject: event.toolSubject || event.safeCommandSummary?.commandPreview,
    startedAt: event.startedAt,
    endedAt: event.endedAt,
    durationMs: finiteDuration(event.durationMs),
    safeCommandSummary: event.safeCommandSummary,
    outputPreview: event.outputPreview,
    stderrPreview: event.stderrPreview,
    partial: event.partial,
    terminal: event.terminal,
    terminalStatus:
      event.terminal && event.kind === 'stopped' ? 'stopped' : undefined,
  };
}

export function historyMessages(events: ClientEvent[]): DisplayMessage[] {
  if (!events.length) return [];
  const first = events[0];
  let projection = emptyProjection({
    connectionEpoch: first.connectionEpoch,
    subscriptionEpoch: first.subscriptionEpoch,
    sessionId: first.sessionId,
  });
  for (const event of events)
    projection = projectEvent(projection, displayEvent(event));
  return projection.messages;
}
