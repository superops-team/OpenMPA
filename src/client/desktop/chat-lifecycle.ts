import {
  useEffect,
  useRef,
  useState,
  type Dispatch,
  type MutableRefObject,
  type SetStateAction,
} from 'react';
import type {
  ConnectionVM,
  DesktopServices,
  PendingRunVM,
  SessionVM,
  Stream,
} from './models';
import {
  emptyProjection,
  type DisplayMessage,
  type Projection,
} from './projection';
import { closeStream } from './chat-subscription';

export function useChatBootstrap({
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
}: {
  services: DesktopServices;
  connection: ConnectionVM;
  session: SessionVM;
  generation: MutableRefObject<number>;
  activeInvocation: MutableRefObject<string | undefined>;
  stream: MutableRefObject<Stream | undefined>;
  setProjection: Dispatch<SetStateAction<Projection>>;
  setHistoryLoaded: Dispatch<SetStateAction<boolean>>;
  setCompletedInvocationIds: Dispatch<SetStateAction<string[]>>;
  setConfigVersion: Dispatch<SetStateAction<string | undefined>>;
  setPending: Dispatch<SetStateAction<PendingRunVM[]>>;
  setDraft: Dispatch<SetStateAction<string>>;
  setExecution: Dispatch<SetStateAction<string>>;
  setHistoryScope: Dispatch<SetStateAction<string>>;
  setError: Dispatch<SetStateAction<string>>;
  setResumeInvocation: Dispatch<SetStateAction<string | undefined>>;
}) {
  useEffect(() => {
    setHistoryLoaded(false);
    setCompletedInvocationIds([]);
    const current = ++generation.current;
    void (async () => {
      const history = await services.sessions.read(
        connection.epoch,
        session.id,
      );
      let saved = await services.sessions.pending(connection.epoch, session.id);
      for (const operation of saved) {
        if (operation.terminalVerified)
          await services.sessions.resolvePending(
            connection.epoch,
            operation.operationId,
            'verifiedTerminal',
          );
      }
      if (saved.some((operation) => operation.terminalVerified))
        saved = await services.sessions.pending(connection.epoch, session.id);
      return { history, saved };
    })()
      .then(({ history, saved }) => {
        if (generation.current !== current) return;
        setProjection({
          ...emptyProjection({
            connectionEpoch: connection.epoch,
            subscriptionEpoch: current,
            sessionId: session.id,
          }),
          messages: history.messages,
        });
        setCompletedInvocationIds(history.completedInvocations || []);
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
        } else if (invocation) setExecution('running');
        else if (history.completedInvocations?.length)
          setExecution('completed');
        else setExecution('idle');
        setHistoryScope(`${connection.epoch}:${session.id}`);
        setHistoryLoaded(true);
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
      generation.current++;
      closeStream(stream.current);
    };
  }, [
    services,
    connection.epoch,
    session.id,
    activeInvocation,
    generation,
    setCompletedInvocationIds,
    setConfigVersion,
    setDraft,
    setError,
    setExecution,
    setHistoryLoaded,
    setHistoryScope,
    setPending,
    setProjection,
    setResumeInvocation,
    stream,
  ]);
}

export function useChatTranscript(messages: DisplayMessage[]) {
  const transcript = useRef<HTMLDivElement>(null);
  const stickToBottom = useRef(true);
  const [showJump, setShowJump] = useState(false);
  useEffect(() => {
    const element = transcript.current;
    if (!element) return;
    if (stickToBottom.current) {
      element.scrollTop = element.scrollHeight;
      setShowJump(false);
    } else {
      setShowJump(true);
    }
  }, [messages]);
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
  return { transcript, showJump, onTranscriptScroll, jumpToBottom };
}
