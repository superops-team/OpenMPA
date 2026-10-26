import { useEffect, useRef, useState } from 'react';

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
  const generation = useRef(0);
  const mounted = useRef(false);
  const currentScope = useRef({
    epoch: connection.epoch,
    sessionId: session.id,
    scope: connection.scope,
  });
  currentScope.current = {
    epoch: connection.epoch,
    sessionId: session.id,
    scope: connection.scope,
  };
  const activeInvocation = useRef<string | undefined>(undefined);
  const cursor = useRef<string | undefined>(undefined);
  const subscriptionGeneration = useRef(0);
  const resumeGeneration = useRef(0);
  const [resumeInvocation, setResumeInvocation] = useState<string>();
  const stream = useRef<
    ReturnType<DesktopServices['sessions']['subscribe']> | undefined
  >(undefined);
  const transcript = useRef<HTMLDivElement>(null);
  const stickToBottom = useRef(true);
  const [showJump, setShowJump] = useState(false);
  useEffect(() => {
    mounted.current = true;
    stickToBottom.current = true;
    setShowJump(false);
    setHistoryLoaded(false);
    const current = ++generation.current;
    void Promise.all([
      services.sessions.read(connection.epoch, session.id),
      services.sessions.pending(connection.epoch, session.id),
    ])
      .then(([history, saved]) => {
        if (generation.current === current) {
          setProjection({
            ...emptyProjection({
              connectionEpoch: connection.epoch,
              subscriptionEpoch: current,
              sessionId: session.id,
            }),
            messages: history.messages,
          });
          setConfigVersion(history.session.configVersion);
          setPending(saved);
          const invocation =
            history.activeRun?.invocationId ||
            saved.find((operation) => operation.invocationId)?.invocationId;
          activeInvocation.current = invocation;
          setResumeInvocation(invocation);
          if (saved.length) {
            setDraft(saved[0].content);
            setExecution('unknown');
          } else if (
            ['running', 'queued', 'accepted'].includes(
              (history.session.status || '').toLowerCase(),
            )
          )
            setExecution('running');
          else setExecution('idle');
          setHistoryScope(`${connection.epoch}:${session.id}`);
          setHistoryLoaded(true);
        }
      })
      .catch((error: unknown) => {
        if (generation.current === current)
          setError(
            error instanceof Error
              ? error.message
              : 'Could not load conversation.',
          );
      });
    return () => {
      mounted.current = false;
      generation.current++;
      stream.current?.close();
    };
  }, [services, connection.epoch, session.id]);
  useEffect(() => {
    const element = transcript.current;
    if (!element) return;
    if (stickToBottom.current) {
      element.scrollTop = element.scrollHeight;
      setShowJump(false);
    } else {
      setShowJump(true);
    }
  }, [projection.messages]);
  const onTranscriptScroll = () => {
    const element = transcript.current;
    if (!element) return;
    const nearBottom =
      element.scrollHeight - element.scrollTop - element.clientHeight <= 80;
    stickToBottom.current = nearBottom;
    if (nearBottom) setShowJump(false);
  };
  const jumpToBottom = () => {
    const element = transcript.current;
    if (!element) return;
    stickToBottom.current = true;
    setShowJump(false);
    const reduceMotion =
      typeof window !== 'undefined' &&
      typeof window.matchMedia === 'function' &&
      window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    element.scrollTo({
      top: element.scrollHeight,
      behavior: reduceMotion ? 'auto' : 'smooth',
    });
  };
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
    setSubmitting(true);
    setError('');
    try {
      const context = material ? await material() : '';
      if (generation.current !== current) return;
      const receipt = await services.sessions.submit(
        connection.epoch,
        session.id,
        context ? `${prompt}\n\n${context}` : prompt,
        configVersion,
      );
      if (generation.current !== current) return;
      const subscriptionEpoch = ++subscriptionGeneration.current;
      setDraft('');
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
        messages: [
          ...previous.messages,
          {
            id: crypto.randomUUID(),
            role: 'user',
            content: prompt,
            invocationId: receipt.invocationId,
            partial: false,
            kind: 'message',
          },
        ],
      }));
      stream.current?.close();
      const subscription = services.sessions.subscribe(
        connection.epoch,
        session.id,
        receipt.invocationId,
        subscriptionEpoch,
      );
      stream.current = subscription;
      setExecution('running');
      let verifiedTerminal = false;
      // Subscribe is intentionally not awaited before consuming its Channel.
      const consume = (async () => {
        for await (const event of subscription.events) {
          if (
            generation.current !== current ||
            subscriptionGeneration.current !== subscriptionEpoch
          )
            break;
          if (
            event.connectionEpoch !== connection.epoch ||
            event.subscriptionEpoch !== subscriptionEpoch ||
            event.sessionId !== session.id ||
            event.invocationId !== activeInvocation.current
          )
            continue;
          if (event.eventId && event.kind !== 'heartbeat')
            cursor.current = event.eventId;
          setProjection((previous) => projectEvent(previous, event));
          if (
            event.terminal &&
            event.invocationId === activeInvocation.current
          ) {
            verifiedTerminal = event.kind !== 'error';
            setExecution(event.kind === 'error' ? 'unknown' : 'completed');
          }
          if (event.kind === 'error') {
            setError(
              event.text ??
                'Stream ended with an error. Read history before retrying.',
            );
          }
        }
      })();
      await Promise.all([consume, subscription.finished]);
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
          const completed =
            verifiedTerminal ||
            pendingVerified ||
            history.completedInvocations?.includes(receipt.invocationId);
          setExecution(completed ? 'completed' : 'unknown');
          if (!completed)
            setError(
              'The subscription closed without a matching terminal event. Review authoritative history before another submission.',
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
            if (generation.current === current) setPending(saved);
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
        return;
      }
      if (
        generation.current !== current ||
        resumeGeneration.current !== attempt
      )
        return;
      activeInvocation.current = invocation;
      setResumeInvocation(invocation);
      stream.current?.close();
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
      let verifiedTerminal = false;
      const consume = (async () => {
        for await (const event of subscription.events) {
          if (
            generation.current !== current ||
            resumeGeneration.current !== attempt ||
            subscriptionGeneration.current !== subscriptionEpoch
          )
            break;
          if (
            event.connectionEpoch !== connection.epoch ||
            event.subscriptionEpoch !== subscriptionEpoch ||
            event.sessionId !== session.id ||
            event.invocationId !== invocation
          )
            continue;
          if (event.eventId && event.kind !== 'heartbeat')
            cursor.current = event.eventId;
          setProjection((previous) => projectEvent(previous, event));
          if (event.terminal) {
            verifiedTerminal = event.kind !== 'error';
            setExecution(event.kind === 'error' ? 'unknown' : 'completed');
          }
        }
      })();
      await Promise.all([consume, subscription.finished]);
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
        setPending(saved);
        setExecution(
          verifiedTerminal ||
            pendingVerified ||
            latest.completedInvocations?.includes(invocation)
            ? 'completed'
            : 'unknown',
        );
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
    send,
    draft,
    setDraft,
    historyLoaded,
    activeInvocation,
  };
}
