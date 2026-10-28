import { ArrowDown, ArrowUp, FileText, Square } from 'lucide-react';
import { useEffect, useMemo, useRef, useState } from 'react';
import { submitComposerOnEnter } from '../chat-composer';
import { Mascot } from '../Mascot';
import { type DisplayMessage } from './projection';
import type { ConnectionVM, DesktopServices, SessionVM } from './models';
import { useDesktopChatState } from './ChatState';
import { localized, useMessages } from './locale';
import { ComposerModelPicker } from './ComposerModelPicker';
import {
  architectureDisplayLabel,
  remoteArchitecture,
} from './remote-architecture';
import { useComposerModel } from './use-composer-model';
import { TracePanel } from './TracePanel';
import { TurnProcess } from './TurnProcess';
import { buildConversationTurns, finalAssistantText } from './turn-model';
import type { ConversationSegment } from './turn-model';
import { AssistantMarkdown } from './AssistantMarkdown';
import { Button, ComposerFooter, ComposerShell } from '../ui';
import { ChatStatus } from './ChatStatus';
import type { ComposerDraft } from './studio-models';

export function DesktopChat({
  services,
  connection,
  session,
  material,
  onSchedule,
  onSaveReply,
  traceOpen,
  onCloseTrace,
  onOpenTrace,
  composerDraft,
}: {
  services: DesktopServices;
  connection: ConnectionVM;
  session: SessionVM;
  material?: () => Promise<string>;
  onSchedule: () => void;
  onSaveReply?: (message: DisplayMessage) => Promise<void>;
  traceOpen?: boolean;
  onCloseTrace?: () => void;
  onOpenTrace?: () => void;
  composerDraft?: ComposerDraft;
}) {
  const m = useMessages();
  const state = useDesktopChatState({
    services,
    connection,
    session,
    material,
    onSchedule,
    onSaveReply,
    composerDraft,
  });
  const {
    execution,
    projection,
    error,
    setError,
    transcript,
    showJump,
    onTranscriptScroll,
    jumpToBottom,
    pending,
    generation,
    setProjection,
    setPending,
    submitting,
    setExecution,
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
  } = state;
  const [stopping, setStopping] = useState(false);
  const [traceFocus, setTraceFocus] = useState<{
    invocationId: string;
    toolId?: string;
  }>();
  const model = useComposerModel({
    services,
    connection,
    session,
    onConfigVersion: setConfigVersion,
    locked:
      submitting ||
      stopping ||
      execution === 'running' ||
      execution === 'submitting' ||
      execution === 'unknown' ||
      execution === 'stop acknowledged; verifying outcome' ||
      pending.length > 0,
  });
  const architecture = remoteArchitecture({
    runtimeName: connection.runtimeName,
    workerToolId: connection.workerToolId,
    runtimeImage: connection.runtimeImage,
  });
  const architectureLabel = architectureDisplayLabel(
    architecture,
    connection.runtimeName,
    connection.runtimeId,
  );
  const invocationIds = useMemo(() => {
    const ids: string[] = [];
    for (const id of completedInvocationIds) {
      if (id && !ids.includes(id)) ids.push(id);
    }
    for (const message of projection.messages) {
      if (message.invocationId && !ids.includes(message.invocationId))
        ids.push(message.invocationId);
    }
    if (activeInvocation.current && !ids.includes(activeInvocation.current))
      ids.push(activeInvocation.current);
    return ids.slice(-5);
  }, [projection.messages, activeInvocation, completedInvocationIds]);
  const currentInvocationId = activeInvocation.current || invocationIds.at(-1);
  const turns = useMemo(
    () => buildConversationTurns(projection.messages),
    [projection.messages],
  );
  const textarea = useRef<HTMLTextAreaElement>(null);
  useEffect(() => {
    const element = textarea.current;
    if (!element) return;
    element.style.height = 'auto';
    const next = Math.min(element.scrollHeight, 168);
    element.style.height = `${next}px`;
    element.style.overflowY = element.scrollHeight > 168 ? 'auto' : 'hidden';
  }, [draft]);
  const canSend =
    !!draft.trim() &&
    !submitting &&
    historyLoaded &&
    execution !== 'running' &&
    execution !== 'unknown' &&
    execution !== 'stop acknowledged; verifying outcome' &&
    pending.length === 0 &&
    session.profileVerified !== false;
  const stopExecution = async () => {
    if (stopping) return;
    const current = generation.current;
    const invocation = activeInvocation.current;
    setStopping(true);
    setExecution('stop acknowledged; verifying outcome');
    cancelSubscription();
    try {
      const aborted = await services.sessions.abort(
        connection.epoch,
        session.id,
      );
      for (let attempt = 0; attempt < 4; attempt++) {
        if (generation.current !== current) return;
        const [history, savedBeforeResolve] = await Promise.all([
          services.sessions.read(connection.epoch, session.id),
          services.sessions.pending(connection.epoch, session.id),
        ]);
        if (generation.current !== current) return;
        for (const operation of savedBeforeResolve) {
          if (
            operation.terminalVerified &&
            (!invocation || operation.invocationId === invocation)
          )
            await services.sessions.resolvePending(
              connection.epoch,
              operation.operationId,
              'verifiedTerminal',
            );
        }
        const saved = await services.sessions.pending(
          connection.epoch,
          session.id,
        );
        if (generation.current !== current) return;
        const completed =
          !!invocation && !!history.completedInvocations?.includes(invocation);
        const active =
          !!history.activeRun &&
          (!invocation || history.activeRun.invocationId === invocation);
        const unresolved = saved.some(
          (operation) =>
            !invocation ||
            !operation.invocationId ||
            operation.invocationId === invocation,
        );
        setProjection((previous) => ({
          ...previous,
          messages: history.messages,
        }));
        setCompletedInvocationIds(history.completedInvocations || []);
        setPending(saved);
        if (completed || (!active && !unresolved)) {
          activeInvocation.current = undefined;
          setResumeInvocation(undefined);
          setError('');
          setExecution(completed ? 'completed' : 'idle');
          return;
        }
        if (!aborted) {
          setError(
            localized((m) =>
              m.desktopTheRuntimeDidNotAcknowledgeAnAbortAndStill(),
            ),
          );
          setExecution('running');
          return;
        }
        if (attempt < 3)
          await new Promise((resolve) => setTimeout(resolve, 150));
      }
      setExecution('unknown');
      setError(
        localized((m) =>
          m.desktopStopWasAcknowledgedButTheAuthoritativeTerminalResultIs(),
        ),
      );
    } catch (error) {
      if (generation.current !== current) return;
      setExecution('unknown');
      setError(
        error instanceof Error
          ? error.message
          : localized((m) => m.desktopCouldNotStopExecution()),
      );
    } finally {
      if (generation.current === current) setStopping(false);
    }
  };
  const statusVisible =
    !!error ||
    pending.length > 0 ||
    execution === 'unknown' ||
    session.profileVerified === false;
  return (
    <section
      className="persistent-chat live-chat"
      aria-label={m.conversation()}
    >
      <div className="chat-transcript-wrap">
        <div
          className="chat-transcript"
          ref={transcript}
          onScroll={onTranscriptScroll}
          role="log"
          aria-live="polite"
        >
          {!projection.messages.length && (
            <div className="chat-welcome">
              <h1>{m.continuingConversation()}</h1>
              <p>{m.messagesFromRuntime()}</p>
            </div>
          )}
          {turns.map((turn) => {
            const userSegments = turn.segments.filter(
              (
                segment,
              ): segment is Extract<ConversationSegment, { type: 'text' }> =>
                segment.type === 'text' && segment.role === 'user',
            );
            const answer = finalAssistantText(turn);
            return (
              <section
                key={turn.invocationId}
                className="conversation-turn"
                data-turn-status={turn.status}
              >
                {userSegments.map((message) => (
                  <div
                    key={message.id}
                    className={`chat-bubble ${message.role}`}
                  >
                    <AssistantMarkdown text={message.content} />
                  </div>
                ))}
                <TurnProcess
                  turn={turn}
                  onInspectTool={
                    onOpenTrace
                      ? (toolId) => {
                          setTraceFocus({
                            invocationId: turn.invocationId,
                            toolId,
                          });
                          onOpenTrace();
                        }
                      : undefined
                  }
                />
                {answer &&
                  [answer].map((message) => {
                    const source = projection.messages.find(
                      (candidate) => candidate.id === message.id,
                    );
                    return (
                      <div
                        key={message.id}
                        className={`chat-bubble ${message.role}`}
                      >
                        <AssistantMarkdown text={message.content} />
                        {source &&
                          onSaveReply &&
                          message.role === 'assistant' &&
                          !message.partial && (
                            <button
                              className="text-button"
                              onClick={() =>
                                void onSaveReply(source).catch(
                                  (error: unknown) =>
                                    setError(
                                      error instanceof Error
                                        ? error.message
                                        : localized((m) =>
                                            m.desktopCouldNotSaveReply(),
                                          ),
                                    ),
                                )
                              }
                            >
                              <FileText size={13} />{' '}
                              {m.desktopReviewSaveToPage()}
                            </button>
                          )}
                      </div>
                    );
                  })}
              </section>
            );
          })}
        </div>
        {showJump && (
          <button
            type="button"
            className="chat-jump"
            aria-label={m.jumpToLatest()}
            onClick={jumpToBottom}
          >
            <ArrowDown size={15} />
          </button>
        )}
      </div>
      <ChatStatus
        visible={statusVisible}
        state={state}
        services={services}
        connection={connection}
        session={session}
      />
      <form
        className="chat-composer"
        onSubmit={(event) => {
          event.preventDefault();
          void send();
        }}
      >
        <div
          className="composer-context"
          aria-label={m.desktopConversationContext()}
        >
          <div className="composer-context-identity">
            <Mascot
              identity={session.id}
              name={session.title}
              small
              decorative
            />
            <strong title={session.title}>{session.title}</strong>
            <span
              className={`composer-architecture ${
                architecture.label === 'Codex Worker'
                  ? 'codex-worker'
                  : architecture.label === 'MPA'
                    ? 'mpa'
                    : 'runtime'
              }`}
              title={architecture.evidence || connection.runtimeId}
            >
              {architectureLabel}
            </span>
          </div>
        </div>
        <ComposerShell className="composer-field">
          <textarea
            ref={textarea}
            rows={1}
            aria-label={m.message()}
            placeholder={m.messageAssistant()}
            value={draft}
            maxLength={100000}
            onChange={(event) => setDraft(event.target.value)}
            onKeyDown={(event) => submitComposerOnEnter(event, canSend)}
          />
          <ComposerFooter className="composer-bar">
            <ComposerModelPicker
              catalog={model.catalog}
              catalogLoading={model.catalogLoading}
              catalogError={model.catalogError}
              onRefreshCatalog={model.refreshCatalog}
              config={model.config}
              busy={model.busy}
              needsInitialization={model.needsInitialization}
              locked={model.locked}
              onApply={model.apply}
            />
            <span className="composer-hint">
              {execution === 'stop acknowledged; verifying outcome'
                ? m.verifyingOutcome()
                : submitting
                  ? m.submittingReceiving()
                  : m.enterToSend()}
            </span>
            {execution === 'running' || stopping ? (
              <Button
                type="button"
                variant="secondary"
                size="icon-lg"
                className={`composer-action stop ${
                  stopping ? 'is-submitting' : 'is-running'
                }`}
                aria-label={m.stopCurrentSession()}
                aria-busy={stopping}
                disabled={stopping}
                onClick={stopExecution}
              >
                <Square size={14} />
              </Button>
            ) : (
              <Button
                type="submit"
                variant="primary"
                size="icon-lg"
                className={`composer-action send ${
                  submitting || execution === 'submitting'
                    ? 'is-submitting'
                    : ''
                }`}
                aria-label={m.sendMessage()}
                aria-busy={submitting}
                disabled={
                  submitting ||
                  execution === 'unknown' ||
                  execution === 'stop acknowledged; verifying outcome' ||
                  pending.length > 0 ||
                  !historyLoaded ||
                  session.profileVerified === false ||
                  !draft.trim()
                }
              >
                <ArrowUp size={16} />
              </Button>
            )}
          </ComposerFooter>
        </ComposerShell>
        {model.error && (
          <p className="composer-model-error" role="alert">
            {model.error}
          </p>
        )}
        {!model.error && model.notice && (
          <p className="composer-model-notice" role="status">
            {model.notice}
          </p>
        )}
      </form>
      {traceOpen && (
        <TracePanel
          services={services}
          connection={connection}
          session={session}
          invocationIds={invocationIds}
          currentInvocationId={currentInvocationId}
          focus={traceFocus}
          onClose={() => {
            setTraceFocus(undefined);
            onCloseTrace?.();
          }}
        />
      )}
    </section>
  );
}
