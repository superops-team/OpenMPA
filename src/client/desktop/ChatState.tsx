import { useRef, useState } from 'react';

import {
  emptyProjection,
  projectEvent,
  type DisplayMessage,
} from './projection';
import type {
  ConnectionVM,
  DesktopServices,
  PendingRunVM,
  SessionVM,
} from './models';
import {
  closeStream,
  consumeSessionEvents,
  terminalObservation,
  waitForSubscriptionBoundary,
} from './chat-subscription';
import { useChatBootstrap, useChatTranscript } from './chat-lifecycle';
export function useDesktopChatState({
  services,
  connection,
  session,
  material,
  onSchedule,
  onSaveReply,
}: {
  services: DesktopServices;
  connection: ConnectionVM;
  session: SessionVM;
  material?: () => Promise<string>;
  onSchedule: () => void;
  onSaveReply?: (message: DisplayMessage) => Promise<void>;
}) {
  const [projection, setProjection] = useState(() =>
    emptyProjection({
      connectionEpoch: connection.epoch,
      subscriptionEpoch: 0,
      sessionId: session.id,
    }),
  );
  const [draft, setDraft] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [historyLoaded, setHistoryLoaded] = useState(false);
  const [historyScope, setHistoryScope] = useState('');
  const [execution, setExecution] = useState('idle');
  const [configVersion, setConfigVersion] = useState(session.configVersion);
  const [error, setError] = useState('');
  const [pending, setPending] = useState<PendingRunVM[]>([]);
  const [completedInvocationIds, setCompletedInvocationIds] = useState<
    string[]
  >([]);
  const generation = useRef(0);
  const activeInvocation = useRef<string | undefined>(undefined);
  const cursor = useRef<string | undefined>(undefined);
  const subscriptionGeneration = useRef(0);
  const resumeGeneration = useRef(0);
  const [resumeInvocation, setResumeInvocation] = useState<string>();
  const stream = useRef<
    ReturnType<DesktopServices['sessions']['subscribe']> | undefined
  >(undefined);
  useChatBootstrap({
    services,
    connection,
    session,
    generation,
    activeInvocation,
    stream,
    setProjection,
    setHistoryLoaded,
    setCompletedInvocationIds,
    setConfigVersion,
    setPending,
    setDraft,
    setExecution,
    setHistoryScope,
    setError,
    setResumeInvocation,
  });
  const { transcript, showJump, onTranscriptScroll, jumpToBottom } =
    useChatTranscript(projection.messages);
  const send = async () => {
    if (
      submitting ||
      execution === 'running' ||
      execution === 'unknown' ||
      execution === 'stop acknowledged; verifying outcome' ||
      !historyLoaded ||
      historyScope !== `${connection.epoch}:${session.id}` ||
      pending.length > 0 ||
      session.profileVerified === false ||
      !draft.trim()
    )
      return;
    const current = generation.current;
    resumeGeneration.current++;
    const prompt = draft.trim();
    const optimisticId = crypto.randomUUID();
    const optimisticInvocation = `optimistic:${optimisticId}`;
    setSubmitting(true);
    setError('');
    setDraft('');
    setExecution('submitting');
    setProjection((previous) => ({
      ...previous,
      messages: [
        ...previous.messages,
        {
          id: optimisticId,
          role: 'user',
          content: prompt,
          invocationId: optimisticInvocation,
          partial: false,
          kind: 'message',
        },
      ],
    }));
    let receiptAccepted = false;
    try {
      const context = material ? await material() : '';
      if (generation.current !== current) return;
      const receipt = await services.sessions.submit(
        connection.epoch,
        session.id,
        context ? `${prompt}\n\n${context}` : prompt,
        configVersion,
        session.mpaInstanceId || connection.runtimeId,
      );
      if (generation.current !== current) return;
      const subscriptionEpoch = ++subscriptionGeneration.current;
      receiptAccepted = true;
      setExecution(receipt.status);
      activeInvocation.current = receipt.invocationId;
      setResumeInvocation(receipt.invocationId);
      cursor.current = undefined;
      setProjection((previous) => ({
        ...previous,
        invocationId: receipt.invocationId,
        scope: {
          connectionEpoch: connection.epoch,
          subscriptionEpoch,
          sessionId: session.id,
        },
        messages: previous.messages.map((message) =>
          message.id === optimisticId
            ? { ...message, invocationId: receipt.invocationId }
            : message,
        ),
      }));
      closeStream(stream.current);
      const subscription = services.sessions.subscribe(
        connection.epoch,
        session.id,
        receipt.invocationId,
        subscriptionEpoch,
      );
      stream.current = subscription;
      setExecution('running');
      const terminal = terminalObservation();
      // Subscribe is intentionally not awaited before consuming its Channel.
      const consume = consumeSessionEvents({
        subscription,
        observation: terminal,
        active: () =>
          generation.current === current &&
          subscriptionGeneration.current === subscriptionEpoch,
        matches: (event) =>
          !(
            event.connectionEpoch !== connection.epoch ||
            event.subscriptionEpoch !== subscriptionEpoch ||
            event.sessionId !== session.id ||
            event.invocationId !== activeInvocation.current
          ),
        onEvent: (event) => {
          if (event.eventId && event.kind !== 'heartbeat')
            cursor.current = event.eventId;
          setProjection((previous) => projectEvent(previous, event));
        },
        onTerminal: (failed, text) => {
          setExecution(failed ? 'unknown' : 'completed');
          if (failed)
            setError(
              text ||
                'The Runtime ended this turn with an error. You can start another turn.',
            );
        },
        onError: (text) =>
          setError(
            text || 'Stream ended with an error. Read history before retrying.',
          ),
      });
      const boundary = await waitForSubscriptionBoundary(consume, subscription);
      if (subscriptionGeneration.current === subscriptionEpoch)
        subscriptionGeneration.current++;
      if (generation.current === current) {
        const history = await services.sessions.read(
          connection.epoch,
          session.id,
        );
        let saved = await services.sessions.pending(
          connection.epoch,
          session.id,
        );
        const pendingVerified = saved.some(
          (operation) =>
            operation.terminalVerified &&
            operation.invocationId === receipt.invocationId,
        );
        for (const operation of saved) {
          if (generation.current !== current) return;
          if (
            operation.terminalVerified &&
            operation.invocationId === receipt.invocationId
          )
            await services.sessions.resolvePending(
              connection.epoch,
              operation.operationId,
              'verifiedTerminal',
            );
        }
        saved = await services.sessions.pending(connection.epoch, session.id);
        if (generation.current === current) {
          setPending(saved);
          setProjection((previous) => ({
            ...previous,
            messages: history.messages,
          }));
          setCompletedInvocationIds(history.completedInvocations || []);
          const completed =
            terminal.verified ||
            pendingVerified ||
            history.completedInvocations?.includes(receipt.invocationId);
          const settled = completed || terminal.failed;
          setExecution(
            settled ? (terminal.failed ? 'failed' : 'completed') : 'unknown',
          );
          if (settled) {
            activeInvocation.current = undefined;
            setResumeInvocation(undefined);
            cursor.current = undefined;
            if (!terminal.failed) setError('');
          }
          if (!settled && boundary.error) throw boundary.error;
          if (!settled)
            setError(
              terminal.observed
                ? 'The Runtime sent a terminal event, but authoritative history has not settled yet. Retry history verification before another submission.'
                : 'The subscription closed without a matching terminal event. Review authoritative history before another submission.',
            );
        }
      }
    } catch (error) {
      if (generation.current === current) {
        setError(
          error instanceof Error ? error.message : 'Could not submit message.',
        );
        setExecution('unknown');
        if (
          error &&
          typeof error === 'object' &&
          'httpStatus' in error &&
          error.httpStatus === 410
        ) {
          cursor.current = undefined;
          void services.sessions
            .read(connection.epoch, session.id)
            .then((history) => {
              if (generation.current === current) {
                setProjection((previous) => ({
                  ...emptyProjection(previous.scope),
                  messages: history.messages,
                }));
                setCompletedInvocationIds(history.completedInvocations || []);
                setError(
                  'The saved stream cursor expired. History has been rebuilt. Resume this same invocation without a cursor.',
                );
              }
            })
            .catch((readError: unknown) => {
              if (generation.current === current)
                setError(
                  readError instanceof Error
                    ? readError.message
                    : 'Cursor expired and history could not be read.',
                );
            });
        }
        void services.sessions
          .pending(connection.epoch, session.id)
          .then((saved) => {
            if (generation.current === current) {
              setPending(saved);
              if (!receiptAccepted && saved.length === 0) {
                setProjection((previous) => ({
                  ...previous,
                  messages: previous.messages.filter(
                    (message) => message.id !== optimisticId,
                  ),
                }));
                setDraft((currentDraft) => currentDraft || prompt);
                setExecution('idle');
              }
            }
          })
          .catch(() => {
            /* Unknown remains blocked if pending inspection fails. */
          });
      }
    } finally {
      if (generation.current === current) setSubmitting(false);
    }
  };
  const resume = async (
    operation?: PendingRunVM,
    acceptedInvocation?: string,
    propagateFailure = false,
  ) => {
    if (submitting && !acceptedInvocation) return;
    const current = generation.current;
    const attempt = ++resumeGeneration.current;
    setSubmitting(true);
    setError('');
    try {
      const history = await services.sessions.read(
        connection.epoch,
        session.id,
      );
      if (
        generation.current !== current ||
        resumeGeneration.current !== attempt
      )
        return;
      setProjection((previous) => ({
        ...emptyProjection(previous.scope),
        messages: history.messages,
      }));
      setCompletedInvocationIds(history.completedInvocations || []);
      let invocation =
        acceptedInvocation ||
        operation?.invocationId ||
        activeInvocation.current;
      if (operation) {
        const receipt = await services.sessions.recover(
          connection.epoch,
          operation.operationId,
          operation.recoveryDisposition === 'retryDurable'
            ? 'retryDurable'
            : 'subscribeAccepted',
        );
        invocation = receipt.invocationId;
      }
      if (!invocation)
        throw new Error(
          'No verified invocation is available for subscription recovery.',
        );
      if (history.completedInvocations?.includes(invocation)) {
        const saved = await services.sessions.pending(
          connection.epoch,
          session.id,
        );
        for (const operation of saved) {
          if (
            generation.current !== current ||
            resumeGeneration.current !== attempt
          )
            return;
          if (
            operation.terminalVerified &&
            operation.invocationId === invocation
          )
            await services.sessions.resolvePending(
              connection.epoch,
              operation.operationId,
              'verifiedTerminal',
            );
        }
        const remaining = await services.sessions.pending(
          connection.epoch,
          session.id,
        );
        if (
          generation.current !== current ||
          resumeGeneration.current !== attempt
        )
          return;
        setPending(remaining);
        setExecution('completed');
        setCompletedInvocationIds(history.completedInvocations || []);
        return;
      }
      if (
        generation.current !== current ||
        resumeGeneration.current !== attempt
      )
        return;
      activeInvocation.current = invocation;
      setResumeInvocation(invocation);
      closeStream(stream.current);
      const subscriptionEpoch = ++subscriptionGeneration.current;
      const subscription = services.sessions.subscribe(
        connection.epoch,
        session.id,
        invocation,
        subscriptionEpoch,
        cursor.current,
      );
      stream.current = subscription;
      setExecution('running');
      setProjection((previous) => ({
        ...previous,
        scope: { ...previous.scope, subscriptionEpoch },
        invocationId: invocation,
      }));
      const terminal = terminalObservation();
      const consume = consumeSessionEvents({
        subscription,
        observation: terminal,
        active: () =>
          generation.current === current &&
          resumeGeneration.current === attempt &&
          subscriptionGeneration.current === subscriptionEpoch,
        matches: (event) =>
          !(
            event.connectionEpoch !== connection.epoch ||
            event.subscriptionEpoch !== subscriptionEpoch ||
            event.sessionId !== session.id ||
            event.invocationId !== invocation
          ),
        onEvent: (event) => {
          if (event.eventId && event.kind !== 'heartbeat')
            cursor.current = event.eventId;
          setProjection((previous) => projectEvent(previous, event));
        },
        onTerminal: (failed, text) => {
          setExecution(failed ? 'unknown' : 'completed');
          if (failed)
            setError(
              text ||
                'The Runtime ended this turn with an error. You can start another turn.',
            );
        },
      });
      const boundary = await waitForSubscriptionBoundary(consume, subscription);
      if (subscriptionGeneration.current === subscriptionEpoch)
        subscriptionGeneration.current++;
      const latest = await services.sessions.read(connection.epoch, session.id);
      let saved = await services.sessions.pending(connection.epoch, session.id);
      const pendingVerified = saved.some(
        (operation) =>
          operation.terminalVerified && operation.invocationId === invocation,
      );
      for (const operation of saved) {
        if (
          generation.current !== current ||
          resumeGeneration.current !== attempt
        )
          return;
        if (operation.terminalVerified && operation.invocationId === invocation)
          await services.sessions.resolvePending(
            connection.epoch,
            operation.operationId,
            'verifiedTerminal',
          );
      }
      saved = await services.sessions.pending(connection.epoch, session.id);
      if (
        generation.current === current &&
        resumeGeneration.current === attempt
      ) {
        setProjection((previous) => ({
          ...previous,
          messages: latest.messages,
        }));
        setCompletedInvocationIds(latest.completedInvocations || []);
        setPending(saved);
        const completed =
          terminal.verified ||
          pendingVerified ||
          !!latest.completedInvocations?.includes(invocation);
        const settled = completed || terminal.failed;
        setExecution(
          settled ? (terminal.failed ? 'failed' : 'completed') : 'unknown',
        );
        if (settled) {
          activeInvocation.current = undefined;
          setResumeInvocation(undefined);
          cursor.current = undefined;
          if (!terminal.failed) setError('');
        } else if (boundary.error) {
          throw boundary.error;
        } else if (terminal.observed) {
          setError(
            'The Runtime sent a terminal event, but authoritative history has not settled yet.',
          );
        }
      }
    } catch (error) {
      if (
        generation.current === current &&
        resumeGeneration.current === attempt
      ) {
        setExecution('unknown');
        if (
          error &&
          typeof error === 'object' &&
          'httpStatus' in error &&
          error.httpStatus === 410
        ) {
          cursor.current = undefined;
          try {
            const latest = await services.sessions.read(
              connection.epoch,
              session.id,
            );
            if (
              generation.current === current &&
              resumeGeneration.current === attempt
            ) {
              setProjection((previous) => ({
                ...emptyProjection(previous.scope),
                messages: latest.messages,
              }));
              setCompletedInvocationIds(latest.completedInvocations || []);
              setError(
                'The saved stream cursor expired. History has been rebuilt. Resume the same invocation again without a cursor.',
              );
            }
          } catch (readError) {
            if (
              generation.current === current &&
              resumeGeneration.current === attempt
            )
              setError(
                readError instanceof Error
                  ? readError.message
                  : 'Cursor expired and history could not be read.',
              );
          }
        } else
          setError(
            error instanceof Error
              ? error.message
              : 'Subscription recovery failed.',
          );
      }
      if (propagateFailure) throw error;
    } finally {
      if (
        generation.current === current &&
        resumeGeneration.current === attempt
      )
        setSubmitting(false);
    }
  };
  const cancelSubscription = () => {
    resumeGeneration.current++;
    subscriptionGeneration.current++;
    cursor.current = undefined;
    closeStream(stream.current);
  };
  return {
    session,
    execution,
    connection,
    onSchedule,
    projection,
    onSaveReply,
    setError,
    transcript,
    showJump,
    onTranscriptScroll,
    jumpToBottom,
    error,
    pending,
    generation,
    services,
    setProjection,
    setPending,
    submitting,
    resume,
    setExecution,
    resumeInvocation,
    setResumeInvocation,
    send,
    draft,
    setDraft,
    historyLoaded,
    activeInvocation,
    completedInvocationIds,
    setCompletedInvocationIds,
    setConfigVersion,
    cancelSubscription,
  };
}
