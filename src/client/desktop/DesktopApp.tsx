import { useState } from 'react';
import { Settings2, X, Pencil, Trash2 } from 'lucide-react';
import { Mascot } from '../Mascot';
import { DesktopBotSettings } from './BotSettings';
import { DesktopChannels } from './Channels';
import { DesktopChat } from './Chat';
import { DesktopKnowledge } from './Knowledge';
import { DesktopSchedules } from './Schedules';
import { DesktopSettings } from './Settings';
import { TracePanel } from './TracePanel';
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
  const [configuringSession, setConfiguringSession] = useState(false);
  const {
    error,
    errorSource,
    setError,
    closeDrafts,
    setCloseDrafts,
    view,
    traceOpen,
    setTraceOpen,
    settingsSection,
    setSettingsSection,
    theme,
    setTheme,
    connection,
    navigate,
    setSelected,
    session,
    epoch,
    refresh,
    createSession,
    connect,
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
          <DesktopSettings
            services={services}
            connection={connection}
            onConnection={connect}
            section={settingsSection}
            onSectionChange={setSettingsSection}
            session={session}
            theme={theme}
            onThemeChange={setTheme}
          />
        ) : view === 'channels' ? (
          <DesktopChannels
            services={services}
            connection={connection}
            sessions={state.sessions}
          />
        ) : view === 'knowledge' ? (
          <DesktopKnowledge connection={connection} sessions={state.sessions} />
        ) : view === 'tasks' ? (
          <DesktopSchedules
            services={services}
            connection={connection}
            sessions={state.sessions}
          />
        ) : (
          <div className="chat-workspace">
            <div className="chat-column">
              {connection && session ? (
                <>
                  <div className="desktop-chat-actions">
                    <button
                      className="text-button"
                      onClick={() => setConfiguringSession(true)}
                    >
                      <Settings2 size={13} />
                      {m.openBotSettings()}
                    </button>
                    <button
                      className="text-button"
                      onClick={() => {
                        const title = window.prompt(
                          'Conversation title',
                          session.title,
                        );
                        if (title?.trim()) {
                          const current = connection.epoch;
                          void services.sessions
                            .rename(current, session.id, title.trim())
                            .then(() => {
                              if (epoch.current === current)
                                return refresh(true);
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
                                return refresh(true);
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
                  {configuringSession && (
                    <DesktopBotSettings
                      services={services}
                      connection={connection}
                      session={session}
                      onSaved={() => {
                        setConfiguringSession(false);
                        void refresh(true);
                      }}
                      onClose={() => setConfiguringSession(false)}
                    />
                  )}

                  <DesktopChat
                    key={`${connection.epoch}:${session.id}:${session.configVersion || ''}`}
                    services={services}
                    connection={connection}
                    session={session}
                    traceOpen={traceOpen}
                    onCloseTrace={() => setTraceOpen(false)}
                    onOpenTrace={() => setTraceOpen(true)}
                    onSchedule={() => navigate('tasks')}
                  />
                </>
              ) : (
                <>
                  <div className="new-conversation">
                    <div className="empty-chat-persona">
                      <Mascot name="OpenMPA" />
                      <h2>{m.workspaceFollowThrough()}</h2>
                      <p>{m.connectRuntimeOpenConversation()}</p>
                      <button
                        className="primary"
                        onClick={() =>
                          connection
                            ? void createSession()
                            : navigate('settings')
                        }
                      >
                        {connection
                          ? 'Start a conversation'
                          : 'Connect your account'}
                      </button>
                    </div>
                  </div>
                  {traceOpen && (
                    <TracePanel
                      services={services}
                      connection={connection}
                      invocationIds={[]}
                      onClose={() => setTraceOpen(false)}
                    />
                  )}
                </>
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
