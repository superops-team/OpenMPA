import { ArrowUp, Clock3, Square, FileText } from 'lucide-react';
import ReactMarkdown from 'react-markdown';
import { submitComposerOnEnter } from '../chat-composer';
import { Mascot } from '../Mascot';
import { type DisplayMessage } from './projection';
import type { ConnectionVM, DesktopServices, SessionVM } from './models';
import { useDesktopChatState } from './ChatState';

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
    scroll,
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
  return (
    <section className="persistent-chat live-chat" aria-label="Conversation">
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
            aria-label="Schedule this conversation"
          >
            <Clock3 size={17} />
          </button>
        </div>
      </header>
      <div className="chat-transcript" role="log" aria-live="polite">
        {!projection.messages.length && (
          <div className="chat-welcome">
            <h1>A continuing conversation.</h1>
            <p>
              Messages and execution history come from your selected Runtime.
            </p>
          </div>
        )}
        {projection.messages.map((message) => (
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
            {!['message', 'text', 'text_delta', 'content'].includes(
              message.kind,
            ) && (
              <small>
                {message.kind === 'tool_call'
                  ? 'Tool call'
                  : message.kind === 'tool_result'
                    ? 'Tool result'
                    : 'Runtime output'}
              </small>
            )}
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
        ))}
        <div ref={scroll} />
      </div>
      {error && (
        <div className="chat-error" role="alert">
          {error}
        </div>
      )}
      {session.profileVerified === false && (
        <div className="document-save-notice">
          This Bot profile snapshot is unverified. Open account settings to
          establish its authorized configuration before sending.
        </div>
      )}
      {execution === 'unknown' && (
        <div className="document-save-notice">
          <p>
            The submission outcome needs review. Your message draft is retained.
            Reopen this conversation to read authoritative history before
            starting another message.
          </p>
        </div>
      )}
      {pending.map((operation) => (
        <div className="document-save-notice" key={operation.operationId}>
          <p>
            Original operation {operation.operationId} · {operation.state} ·
            configuration {operation.executionConfigVersion || 'not pinned'}
          </p>
          <p>
            The host retains the original message and idempotency key. A new
            request remains blocked while its outcome is unresolved.
          </p>
          <button
            type="button"
            className="text-button"
            onClick={() => {
              const current = generation.current;
              void Promise.all([
                services.sessions.read(connection.epoch, session.id),
                services.sessions.pending(connection.epoch, session.id),
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
            Read authoritative history and pending operation
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
                await services.sessions.read(connection.epoch, session.id);
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
              : 'Acknowledge outcome after reviewing history'}
          </button>
        </div>
      ))}
      {resumeInvocation && execution === 'unknown' && (
        <button
          type="button"
          className="text-button"
          disabled={submitting}
          onClick={() => void resume()}
        >
          Read history and resume the same invocation
        </button>
      )}
      <form
        className="chat-composer"
        onSubmit={(event) => {
          event.preventDefault();
          void send();
        }}
      >
        <div className="chat-compose-row">
          <textarea
            aria-label="Message"
            placeholder="Message your assistant…"
            value={draft}
            maxLength={100000}
            onChange={(event) => setDraft(event.target.value)}
            onKeyDown={(event) =>
              submitComposerOnEnter(
                event,
                !!draft.trim() &&
                  !submitting &&
                  historyLoaded &&
                  execution !== 'running' &&
                  execution !== 'unknown' &&
                  execution !== 'stop acknowledged; verifying outcome' &&
                  pending.length === 0 &&
                  session.profileVerified !== false,
              )
            }
          />
          <button
            className="send-button"
            aria-label="Send message"
            disabled={
              submitting ||
              execution === 'running' ||
              execution === 'unknown' ||
              execution === 'stop acknowledged; verifying outcome' ||
              pending.length > 0 ||
              !historyLoaded ||
              session.profileVerified === false ||
              !draft.trim()
            }
          >
            <ArrowUp size={18} />
          </button>
        </div>
        <div className="composer-bottom">
          <span>
            {submitting
              ? 'Submitting / receiving'
              : 'Enter to send · Shift Enter for a new line'}
          </span>
          {execution === 'running' && (
            <button
              type="button"
              className="text-button"
              onClick={() => {
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
                      history.completedInvocations?.includes(
                        activeInvocation.current,
                      );
                    setExecution(
                      completed
                        ? 'completed'
                        : 'stop acknowledged; verifying outcome',
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
              }}
            >
              <Square size={13} /> Stop current session
            </button>
          )}
        </div>
      </form>
    </section>
  );
}
