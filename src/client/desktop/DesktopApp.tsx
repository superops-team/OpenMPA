import { useEffect, useState } from 'react';
import {
  ActivitySquare,
  Pencil,
  Settings2,
  SquareTerminal,
  Trash2,
  X,
} from 'lucide-react';
import { Mascot } from '../Mascot';
import { DesktopBotSettings } from './BotSettings';
import { DesktopChannels } from './Channels';
import { DesktopChat } from './Chat';
import { DesktopKnowledge } from './Knowledge';
import { DesktopResources } from './Resources';
import { DesktopCollaboration } from './Collaboration';
import { DesktopSchedules } from './Schedules';
import { DesktopSettings } from './Settings';
import { RuntimeManager } from './RuntimeManager';
import type { DesktopServices } from './models';
import { useDesktopAppState } from './DesktopAppState';
import { LocaleProvider, localized, useMessages } from './locale';

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
  const canUseConversationTools = Boolean(connection && session);
  useEffect(() => {
    if (!canUseConversationTools && traceOpen) setTraceOpen(false);
  }, [canUseConversationTools, traceOpen, setTraceOpen]);
  const openRuntimeTerminal = () => {
    if (!connection || !session || !services.terminal) return;
    void services.terminal
      .open(connection.epoch, session.id, session.title)
      .catch((error: unknown) =>
        setError(
          error instanceof Error
            ? error.message
            : 'Could not open the Runtime terminal.',
        ),
      );
  };
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
            <h2>{m.desktopPreserveYourUnsavedDraftsBeforeClosing()}</h2>
            <p>
              {m.desktopContinueEditingExportAnExactCopyOrExplicitlyAbandon()}
            </p>
            {closeDrafts.map((draft) => (
              <article key={`${draft.pageId}:${draft.capturedAt}`}>
                <strong>{draft.title}</strong>
                <p>
                  {m.desktopPage()} {draft.pageId} {m.desktopBaseRevision()}{' '}
                  {draft.baseRevision} {m.desktopCaptured()} {draft.capturedAt}
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
                            : localized((m) =>
                                m.desktopCouldNotExportUnsavedDraft(),
                              ),
                        ),
                      )
                  }
                >
                  {m.desktopExportThisDraft()}
                </button>
                <button
                  className="text-button"
                  onClick={() => {
                    if (
                      !window.confirm(
                        m.abandonDraftConfirm(
                          draft.title,
                          String(draft.capturedAt),
                        ),
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
                          localized((m) =>
                            m.desktopTheIndicatedInMemoryDraftWasAbandonedCloseThe(),
                          ),
                        );
                      })
                      .catch((error: unknown) =>
                        setError(
                          error instanceof Error
                            ? error.message
                            : localized((m) =>
                                m.desktopCouldNotAbandonThisExactDraft(),
                              ),
                        ),
                      );
                  }}
                >
                  {m.desktopAbandonThisExactDraft()}
                </button>
              </article>
            ))}
            <button className="text-button" onClick={() => setCloseDrafts([])}>
              {m.desktopContinueEditing()}
            </button>
          </section>
        )}
        <DesktopResources
          key={`resources:${JSON.stringify([connection?.epoch, connection?.scope, state.platform?.epoch, state.platform?.region])}`}
          services={services.resources}
          connection={connection}
          platform={state.platform}
          session={session}
          target={state.conversationTarget}
          onUse={state.useInConversation}
          onConfigureMCP={() => setConfiguringSession(true)}
          hidden={view !== 'resources'}
        />
        <DesktopCollaboration
          key={`collaboration:${JSON.stringify([connection?.epoch, connection?.scope])}`}
          services={services.collaboration}
          connection={connection}
          hidden={view !== 'collaboration'}
        />
        {configuringSession && connection && session && (
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
        {view === 'resources' || view === 'collaboration' ? null : view ===
          'runtimes' ? (
          <RuntimeManager
            key={`runtimes:${state.platform?.scope}:${state.platform?.epoch}`}
            services={services}
            platform={state.platform}
            connection={connection}
            onConnection={connect}
            onDirectoryChanged={state.refreshRuntimes}
          />
        ) : view === 'settings' ? (
          <DesktopSettings
            services={services}
            connection={connection}
            onConnection={connect}
            onManageRuntimes={() => navigate('runtimes')}
            section={settingsSection}
            onSectionChange={setSettingsSection}
            session={session}
            theme={theme}
            onThemeChange={setTheme}
          />
        ) : view === 'channels' ? (
          <DesktopChannels
            key={`channels:${connection?.epoch}:${connection?.scope}`}
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
              <div className="desktop-chat-actions">
                <button
                  className="text-button"
                  aria-label="Open Runtime terminal"
                  title={
                    canUseConversationTools
                      ? 'Open Runtime terminal'
                      : 'Connect a Runtime and choose a conversation first'
                  }
                  disabled={!canUseConversationTools || !services.terminal}
                  onClick={openRuntimeTerminal}
                >
                  <SquareTerminal size={13} />
                  Terminal
                </button>
                <button
                  className="text-button"
                  aria-label="Open turn trace"
                  title={
                    canUseConversationTools
                      ? 'Open turn trace'
                      : 'Connect a Runtime and choose a conversation first'
                  }
                  disabled={!canUseConversationTools}
                  aria-pressed={traceOpen}
                  onClick={() => setTraceOpen((previous) => !previous)}
                >
                  <ActivitySquare size={13} />
                  Trace
                </button>
                {connection && session && (
                  <>
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
                          m.conversationTitle(),
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
                                  : localized((m) => m.desktopRenameFailed()),
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
                            m.deleteConversationConfirm(session.id),
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
                                  : localized((m) => m.desktopDeleteFailed()),
                              ),
                            );
                        }
                      }}
                    >
                      <Trash2 size={13} />
                      {m.delete()}
                    </button>
                  </>
                )}
              </div>
              {connection && session ? (
                <>
                  <DesktopChat
                    key={`${connection.epoch}:${connection.scope}:${session.id}:${session.configVersion || ''}`}
                    services={services}
                    connection={connection}
                    session={session}
                    composerDraft={state.composerDraft}
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
                          ? m.startConversation()
                          : m.connectAccount()}
                      </button>
                    </div>
                  </div>
                </>
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
