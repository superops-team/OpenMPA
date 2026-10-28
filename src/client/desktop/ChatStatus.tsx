import { localized, useMessages } from './locale';
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
  const m = useMessages();
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
    <section className="chat-status" aria-label={m.desktopConversationStatus()}>
      {error && (
        <div className="chat-status-error" role="alert">
          {error}
        </div>
      )}
      {pending.length > 0 && (
        <details className="chat-status-details" open={pending.length === 1}>
          <summary>{m.chatOriginalNeedsReview(pending.length)}</summary>
          {pending.map((operation) => (
            <div className="chat-status-operation" key={operation.operationId}>
              <p>
                {m.chatOperationConfiguration(
                  operation.operationId,
                  operation.state,
                  operation.executionConfigVersion || m.desktopNotPinned(),
                )}
              </p>
              <p>
                {m.desktopTheHostRetainsTheOriginalMessageAndIdempotencyKey()}
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
                              : localized((m) =>
                                  m.desktopCouldNotVerifyTheOriginalOperation(),
                                ),
                          );
                      });
                  }}
                >
                  {m.desktopReadAuthoritativeHistory()}
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
                        !window.confirm(m.chatRecoverConfirm())
                      )
                        return;
                      void resume(operation);
                    }}
                  >
                    {operation.recoveryDisposition === 'retryDurable'
                      ? m.desktopRecoverOriginalDurableSubmission()
                      : m.desktopResumeAcceptedInvocation()}
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
                      !window.confirm(m.chatAcknowledgeConfirm())
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
                            : localized((m) =>
                                m.desktopCouldNotAcknowledgeThisOriginalOperation(),
                              ),
                        );
                    });
                  }}
                >
                  {operation.terminalVerified
                    ? m.chatResolveTerminal()
                    : m.chatAcknowledgeOutcome()}
                </button>
              </div>
            </div>
          ))}
        </details>
      )}
      {execution === 'unknown' && pending.length === 0 && (
        <div className="chat-status-note" role="status">
          <p>{m.desktopTheSubmissionOutcomeNeedsReviewYourMessageDraftIs()}</p>
          {resumeInvocation && (
            <button
              type="button"
              className="text-button"
              disabled={submitting}
              onClick={() => void resume()}
            >
              {m.desktopReadHistoryAndResumeTheSameInvocation()}
            </button>
          )}
        </div>
      )}
      {session.profileVerified === false && (
        <div className="chat-status-note" role="status">
          {m.desktopThisBotProfileSnapshotIsUnverifiedOpenAccountSettings()}
        </div>
      )}
    </section>
  );
}
