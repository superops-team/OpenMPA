import type { Stream } from './models';
import type { DisplayEvent } from './projection';

const closedStreams = new WeakSet<Stream>();

export function closeStream(stream?: Stream) {
  if (!stream || closedStreams.has(stream)) return;
  closedStreams.add(stream);
  stream.close();
}

export async function waitForSubscriptionBoundary(
  consume: Promise<void>,
  subscription: Stream,
) {
  const settled = (source: 'events' | 'host', error?: unknown) => ({
    source,
    error,
  });
  const boundary = await Promise.race([
    consume.then(
      () => settled('events'),
      (error) => settled('events', error),
    ),
    subscription.finished.then(
      () => settled('host'),
      (error) => settled('host', error),
    ),
  ]);
  closeStream(subscription);
  return boundary;
}

export interface TerminalObservation {
  verified: boolean;
  observed: boolean;
  failed: boolean;
}

export function terminalObservation(): TerminalObservation {
  return { verified: false, observed: false, failed: false };
}

export async function consumeSessionEvents({
  subscription,
  observation,
  active,
  matches,
  onEvent,
  onTerminal,
  onError,
}: {
  subscription: Stream;
  observation: TerminalObservation;
  active: () => boolean;
  matches: (event: DisplayEvent) => boolean;
  onEvent: (event: DisplayEvent) => void;
  onTerminal: (failed: boolean, text?: string) => void;
  onError?: (text?: string) => void;
}) {
  for await (const event of subscription.events) {
    if (!active()) break;
    if (!matches(event)) continue;
    onEvent(event);
    if (event.terminal) {
      observation.observed = true;
      observation.verified = event.kind !== 'error';
      observation.failed = event.kind === 'error';
      onTerminal(observation.failed, event.text);
      break;
    }
    if (event.kind === 'error') onError?.(event.text);
  }
}
