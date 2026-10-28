import type { ConnectionVM, DesktopServices, SessionVM } from './models';
import type { useDesktopChatState } from './ChatState';

type DesktopChatState = ReturnType<typeof useDesktopChatState>;

export function ChatStatus({
  visible,
  state,
  services,
  connection,
  session,
}: {
  visible: boolean;
  state: DesktopChatState;
  services: DesktopServices;
  connection: ConnectionVM;
  session: SessionVM;
}) {
  if (!visible) return null;
  const {
    error,
    setError,
    pending,
    generation,
    setProjection,
    setPending,
    submitting,
    resume,
    setExecution,
    resumeInvocation,
    execution,
  } = state;
  return (
    <section className="chat-status" aria-label="Conversation status">
      {error && (
        <div className="chat-status-error" role="alert">
          {error}
        </div>
      )}
      {pending.length > 0 && (
        <details className="chat-status-details" open={pending.length === 1}>
          <summary>Original operation needs review · {pending.length}</summary>
          {pending.map((operation) => (
            <div className="chat-status-operation" key={operation.operationId}>
              <p>
                {operation.operationId} · {operation.state} · configuration{' '}
                {operation.executionConfigVersion || 'not pinned'}
              </p>
              <p>
                The host retains the original message and idempotency key. A new
                request remains blocked while its outcome is unresolved.
              </p>
              <div className="chat-status-actions">
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
                      .catch((failure: unknown) => {
                        if (generation.current === current)
                          setError(
                            failure instanceof Error
                              ? failure.message
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
                    })().catch((failure: unknown) => {
                      if (generation.current === current)
                        setError(
                          failure instanceof Error
                            ? failure.message
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
            The submission outcome needs review. Your message draft is retained.
            Read authoritative history before starting another message.
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
  );
}
