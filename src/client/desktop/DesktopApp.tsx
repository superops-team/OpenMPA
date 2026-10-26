import { Settings2, X, Pencil, Trash2 } from 'lucide-react';
import { Mascot } from '../Mascot';
import { DesktopChat } from './Chat';
import { DesktopPages } from './Pages';
import type { DesktopServices } from './models';
import { useDesktopAppState } from './DesktopAppState';
import { LocaleProvider, useMessages } from './locale';

export function DesktopApp({ services }: { services: DesktopServices }) {
  return (
    <LocaleProvider>
      <DesktopAppContent services={services} />
    </LocaleProvider>
  );
}

function DesktopAppContent({ services }: { services: DesktopServices }) {
  const m = useMessages();
  const state = useDesktopAppState({ services });
  const {
    error,
    errorSource,
    setError,
    closeDrafts,
    setCloseDrafts,
    view,
    space,
    connection,
    sessions,
    setDirty,
    navigate,
    setSelected,
    session,
    reportFailure,
    epoch,
    refresh,
    createSession,
  } = state;
  return (
    <div className="app template-app desktop-app">
      <div className="workspace">
        {error && (
          <div
            className="error-banner"
            role="alert"
            data-error-source={errorSource.current}
          >
            <span>{error}</span>
            <button
              className="icon-button"
              aria-label={m.dismissError()}
              onClick={() => setError('')}
            >
              <X size={16} />
            </button>
          </div>
        )}
        {closeDrafts.length > 0 && (
          <section className="document-save-notice desktop-form">
            <h2>Preserve your unsaved drafts before closing</h2>
            <p>
              Continue editing, export an exact copy, or explicitly abandon the
              indicated in-memory draft. Saved recovery files remain protected.
            </p>
            {closeDrafts.map((draft) => (
              <article key={`${draft.pageId}:${draft.capturedAt}`}>
                <strong>{draft.title}</strong>
                <p>
                  Page {draft.pageId} · Base revision {draft.baseRevision} ·
                  Captured {draft.capturedAt}
                </p>
                <button
                  className="text-button"
                  onClick={() =>
                    void services.workspace
                      .exportDraft(draft)
                      .catch((error: unknown) =>
                        setError(
                          error instanceof Error
                            ? error.message
                            : 'Could not export unsaved draft.',
                        ),
                      )
                  }
                >
                  Export this draft
                </button>
                <button
                  className="text-button"
                  onClick={() => {
                    if (
                      !window.confirm(
                        `Abandon exactly the in-memory draft for ${draft.title}, captured ${draft.capturedAt}? This does not delete the saved recovery file.`,
                      )
                    )
                      return;
                    void services.workspace
                      .abandonDraft(draft)
                      .then(() => {
                        setCloseDrafts((previous) =>
                          previous.filter(
                            (item) =>
                              item.pageId !== draft.pageId ||
                              item.capturedAt !== draft.capturedAt,
                          ),
                        );
                        setError(
                          'The indicated in-memory draft was abandoned. Close the window again to recheck safe closing.',
                        );
                      })
                      .catch((error: unknown) =>
                        setError(
                          error instanceof Error
                            ? error.message
                            : 'Could not abandon this exact draft.',
                        ),
                      );
                  }}
                >
                  Abandon this exact draft
                </button>
              </article>
            ))}
            <button className="text-button" onClick={() => setCloseDrafts([])}>
              Continue editing
            </button>
          </section>
        )}
        {view === 'settings' ? (
          <section aria-label="Native settings content" />
        ) : view === 'channels' ? (
          <section aria-label="Native channels content" />
        ) : view === 'knowledge' ? (
          <section aria-label="Native knowledge content" />
        ) : view === 'tasks' ? (
          <section aria-label="Native schedules content" />
        ) : view === 'pages' && space ? (
          <DesktopPages
            key={space.id}
            services={services}
            space={space}
            connection={connection}
            sessions={sessions}
            onDirty={setDirty}
            onSchedule={() => navigate('tasks')}
            onOpenSession={(id) => {
              setSelected(id);
              navigate('chat');
            }}
          />
        ) : (
          <div className="chat-workspace">
            <div className="chat-column">
              {connection && session ? (
                <>
                  <div className="desktop-chat-actions">
                    <button
                      className="text-button"
                      onClick={() => {
                        void services.host
                          ?.requestSessionAction?.('configureSession')
                          .catch((error: unknown) =>
                            reportFailure(error, 'sessions'),
                          );
                      }}
                    >
                      <Settings2 size={13} />
                      {m.openBotSettings()}
                    </button>
                    <button
                      className="text-button"
                      onClick={() => {
                        if (services.host?.requestSessionAction) {
                          void services.host
                            .requestSessionAction('renameSession')
                            .catch((error: unknown) =>
                              reportFailure(error, 'sessions'),
                            );
                          return;
                        }
                        const title = window.prompt(
                          'Conversation title',
                          session.title,
                        );
                        if (title?.trim()) {
                          const current = connection.epoch;
                          void services.sessions
                            .rename(current, session.id, title.trim())
                            .then(() => {
                              if (epoch.current === current) return refresh();
                            })
                            .catch((error: unknown) =>
                              setError(
                                error instanceof Error
                                  ? error.message
                                  : 'Rename failed.',
                              ),
                            );
                        }
                      }}
                    >
                      <Pencil size={13} />
                      {m.rename()}
                    </button>
                    <button
                      className="text-button"
                      onClick={() => {
                        if (services.host?.requestSessionAction) {
                          void services.host
                            .requestSessionAction('deleteSession')
                            .catch((error: unknown) =>
                              reportFailure(error, 'sessions'),
                            );
                          return;
                        }
                        if (
                          window.confirm(
                            `Delete conversation ${session.id}? Server retention and memory policy apply.`,
                          )
                        ) {
                          const current = connection.epoch;
                          void services.sessions
                            .remove(current, session.id)
                            .then(() => {
                              if (epoch.current === current) {
                                setSelected(undefined);
                                return refresh();
                              }
                            })
                            .catch((error: unknown) =>
                              setError(
                                error instanceof Error
                                  ? error.message
                                  : 'Delete failed.',
                              ),
                            );
                        }
                      }}
                    >
                      <Trash2 size={13} />
                      {m.delete()}
                    </button>
                  </div>

                  <DesktopChat
                    key={`${connection.epoch}:${session.id}:${session.configVersion || ''}`}
                    services={services}
                    connection={connection}
                    session={session}
                    onSchedule={() => navigate('tasks')}
                  />
                </>
              ) : (
                <div className="new-conversation">
                  <div className="empty-chat-persona">
                    <Mascot name="OpenMPA" />
                    <h2>{m.workspaceFollowThrough()}</h2>
                    <p>{m.connectRuntimeOpenConversation()}</p>
                    <button
                      className="primary"
                      onClick={() =>
                        connection ? void createSession() : navigate('settings')
                      }
                    >
                      {connection
                        ? 'Start a conversation'
                        : 'Connect your account'}
                    </button>
                  </div>
                </div>
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
