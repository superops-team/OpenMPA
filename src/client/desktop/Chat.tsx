import { ArrowDown, ArrowUp, Clock3, FileText, Square } from 'lucide-react';
import { useEffect, useRef } from 'react';
import ReactMarkdown from 'react-markdown';
import { submitComposerOnEnter } from '../chat-composer';
import { Mascot } from '../Mascot';
import { type DisplayMessage } from './projection';
import type { ConnectionVM, DesktopServices, SessionVM } from './models';
import { useDesktopChatState } from './ChatState';
import { useMessages } from './locale';

const toolKinds = new Set([
  'toolCall',
  'toolResult',
  'tool_call',
  'tool_result',
]);

function toolLabel(kind: string) {
  return kind === 'toolResult' || kind === 'tool_result'
    ? 'Tool result'
    : 'Tool call';
}

export function DesktopChat({
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
  const m = useMessages();
  const state = useDesktopChatState({
    services,
    connection,
    session,
    material,
    onSchedule,
    onSaveReply,
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
    resume,
    setExecution,
    resumeInvocation,
    send,
    draft,
    setDraft,
    historyLoaded,
    activeInvocation,
  } = state;
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
  const stopExecution = () => {
    const current = generation.current;
    void services.sessions
      .abort(connection.epoch, session.id)
      .then(async (aborted) => {
        if (generation.current !== current) return;
        const history = await services.sessions.read(
          connection.epoch,
          session.id,
        );
        if (generation.current !== current) return;
        setProjection((previous) => ({
          ...previous,
          messages: history.messages,
        }));
        if (!aborted) {
          setError(
            'The Runtime did not acknowledge an abort. Current execution remains unverified; history has been refreshed.',
          );
          return;
        }
        const completed =
          activeInvocation.current &&
          history.completedInvocations?.includes(activeInvocation.current);
        setExecution(
          completed ? 'completed' : 'stop acknowledged; verifying outcome',
        );
      })
      .catch((error: unknown) => {
        if (generation.current === current)
          setError(
            error instanceof Error
              ? error.message
              : 'Could not stop execution.',
          );
      });
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
      <header className="chat-persona">
        <Mascot
          identity={session.id}
          name={session.title}
          state={execution === 'running' ? 'working' : 'idle'}
        />
        <div>
          <strong>{session.title}</strong>
          <span>
            Runtime {connection.runtimeId} · {execution}
          </span>
        </div>
        <div className="chat-persona-actions">
          <button
            className="icon-button"
            onClick={onSchedule}
            aria-label={m.scheduleConversation()}
          >
            <Clock3 size={17} />
          </button>
        </div>
      </header>
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
          {projection.messages.map((message) =>
            toolKinds.has(message.kind) ? (
              <div
                key={message.id}
                className="chat-tool"
                aria-label={toolLabel(message.kind)}
              >
                <span className="chat-tool-dot" aria-hidden="true" />
                <span className="chat-tool-kind">
                  {toolLabel(message.kind)}
                </span>
                <code>{message.content}</code>
              </div>
            ) : (
              <div key={message.id} className={`chat-bubble ${message.role}`}>
                <ReactMarkdown
                  components={{
                    img: ({ alt }) => <span>{alt}</span>,
                    a: ({ href, children }) => (
                      <a href={href} target="_blank" rel="noreferrer">
                        {children}
                      </a>
                    ),
                  }}
                >
                  {message.content}
                </ReactMarkdown>
                {onSaveReply &&
                  message.role === 'assistant' &&
                  !message.partial && (
                    <button
                      className="text-button"
                      onClick={() =>
                        void onSaveReply(message).catch((error: unknown) =>
                          setError(
                            error instanceof Error
                              ? error.message
                              : 'Could not save reply.',
                          ),
                        )
                      }
                    >
                      <FileText size={13} /> Review & save to page
                    </button>
                  )}
              </div>
            ),
          )}
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
      {statusVisible && (
        <section className="chat-status" aria-label="Conversation status">
          {error && (
            <div className="chat-status-error" role="alert">
              {error}
            </div>
          )}
          {pending.length > 0 && (
            <details
              className="chat-status-details"
              open={pending.length === 1}
            >
              <summary>
                Original operation needs review · {pending.length}
              </summary>
              {pending.map((operation) => (
                <div
                  className="chat-status-operation"
                  key={operation.operationId}
                >
                  <p>
                    {operation.operationId} · {operation.state} · configuration{' '}
                    {operation.executionConfigVersion || 'not pinned'}
                  </p>
                  <p>
                    The host retains the original message and idempotency key. A
                    new request remains blocked while its outcome is unresolved.
                  </p>
                  <div className="chat-status-actions">
                    <button
                      type="button"
                      className="text-button"
                      onClick={() => {
                        const current = generation.current;
                        void Promise.all([
                          services.sessions.read(connection.epoch, session.id),
                          services.sessions.pending(
                            connection.epoch,
                            session.id,
                          ),
                        ])
                          .then(([history, saved]) => {
                            if (generation.current === current) {
                              setProjection((previous) => ({
                                ...previous,
                                messages: history.messages,
                              }));
                              setPending(saved);
                            }
                          })
                          .catch((error: unknown) => {
                            if (generation.current === current)
                              setError(
                                error instanceof Error
                                  ? error.message
                                  : 'Could not verify the original operation.',
                              );
                          });
                      }}
                    >
                      Read authoritative history
                    </button>
                    {['subscribeAccepted', 'retryDurable'].includes(
                      operation.recoveryDisposition || '',
                    ) && (
                      <button
                        type="button"
                        className="text-button"
                        disabled={submitting}
                        onClick={() => {
                          if (
                            operation.recoveryDisposition === 'retryDurable' &&
                            !window.confirm(
                              'Recover this original request using its original content, key and execution version? The host verifies durable idempotency and permissions.',
                            )
                          )
                            return;
                          void resume(operation);
                        }}
                      >
                        {operation.recoveryDisposition === 'retryDurable'
                          ? 'Recover original durable submission'
                          : 'Resume accepted invocation'}
                      </button>
                    )}
                    <button
                      type="button"
                      className="text-button"
                      disabled={submitting}
                      onClick={() => {
                        const mode = operation.terminalVerified
                          ? 'verifiedTerminal'
                          : 'userReviewed';
                        if (
                          !operation.terminalVerified &&
                          !window.confirm(
                            'After reviewing authoritative history, acknowledge this unknown operation? It may already have executed. This removes the recovery block; submitting the original prompt again can produce duplicate work.',
                          )
                        )
                          return;
                        const current = generation.current;
                        void (async () => {
                          await services.sessions.read(
                            connection.epoch,
                            session.id,
                          );
                          await services.sessions.resolvePending(
                            connection.epoch,
                            operation.operationId,
                            mode,
                          );
                          const saved = await services.sessions.pending(
                            connection.epoch,
                            session.id,
                          );
                          if (generation.current === current) {
                            setPending(saved);
                            if (!saved.length) setExecution('idle');
                          }
                        })().catch((error: unknown) => {
                          if (generation.current === current)
                            setError(
                              error instanceof Error
                                ? error.message
                                : 'Could not acknowledge this original operation.',
                            );
                        });
                      }}
                    >
                      {operation.terminalVerified
                        ? 'Resolve verified terminal operation'
                        : 'Acknowledge outcome'}
                    </button>
                  </div>
                </div>
              ))}
            </details>
          )}
          {execution === 'unknown' && pending.length === 0 && (
            <div className="chat-status-note" role="status">
              <p>
                The submission outcome needs review. Your message draft is
                retained. Read authoritative history before starting another
                message.
              </p>
              {resumeInvocation && (
                <button
                  type="button"
                  className="text-button"
                  disabled={submitting}
                  onClick={() => void resume()}
                >
                  Read history and resume the same invocation
                </button>
              )}
            </div>
          )}
          {session.profileVerified === false && (
            <div className="chat-status-note" role="status">
              This Bot profile snapshot is unverified. Open account settings to
              establish its authorized configuration before sending.
            </div>
          )}
        </section>
      )}
      <form
        className="chat-composer"
        onSubmit={(event) => {
          event.preventDefault();
          void send();
        }}
      >
        <div className="composer-field">
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
          <div className="composer-bar">
            <span className="composer-hint">
              {submitting ? m.submittingReceiving() : m.enterToSend()}
            </span>
            {execution === 'running' ? (
              <button
                type="button"
                className="composer-action stop"
                aria-label={m.stopCurrentSession()}
                onClick={stopExecution}
              >
                <Square size={14} />
              </button>
            ) : (
              <button
                type="submit"
                className="composer-action send"
                aria-label={m.sendMessage()}
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
              </button>
            )}
          </div>
        </div>
      </form>
    </section>
  );
}
