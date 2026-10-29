import {
  errorMessage,
  localized,
  LocalizedError,
  renderMessage,
  useMessages,
  useMessageState,
} from './locale';
import {
  useCallback,
  useEffect,
  useEffectEvent,
  useMemo,
  useRef,
  useState,
} from 'react';

import type {
  ConnectionVM,
  DesktopServices,
  DesktopSettingsSection,
  RecoveryDraftVM,
  RuntimeVM,
  SessionVM,
  PlatformVM,
} from './models';
import { ComposerDraftStore } from './composer-drafts';
import type { ComposerDraft, ConversationTarget } from './studio-models';
import {
  setThemePreference,
  themePreference,
  type ThemePreference,
} from '../theme';
type View =
  | 'chat'
  | 'tasks'
  | 'channels'
  | 'settings'
  | 'knowledge'
  | 'resources'
  | 'collaboration'
  | 'runtimes';
export function useDesktopAppState({
  services,
}: {
  services: DesktopServices;
}) {
  const m = useMessages();
  const [view, setView] = useState<View>('chat');
  const [connection, setConnection] = useState<ConnectionVM>();
  const [runtimeOptions, setRuntimeOptions] = useState<RuntimeVM[]>([]);
  const [sessions, setSessions] = useState<SessionVM[]>([]);
  const [sessionEpoch, setSessionEpoch] = useState(0);
  const [selected, selectSession] = useState<string>();
  const selectedRef = useRef<string | undefined>(undefined);
  const setSelected = useCallback((value: string | undefined) => {
    selectedRef.current = value;
    selectSession(value);
  }, []);
  const [collapsed, setCollapsed] = useState(false);
  const [traceOpen, setTraceOpen] = useState(false);
  const [settingsSection, setSettingsSection] =
    useState<DesktopSettingsSection>('Account');
  const [theme, setTheme] = useState<ThemePreference>(themePreference());
  const platformRegion = useRef('cn-beijing');
  const platformSignedIn = useRef(false);
  const [platform, setPlatform] = useState<PlatformVM>();
  const [error, setError] = useMessageState();
  const [closeDrafts, setCloseDrafts] = useState<RecoveryDraftVM[]>([]);
  const errorSource = useRef('');
  const reportFailure = useCallback((failure: unknown, source: string) => {
    errorSource.current = source;
    const status =
      failure && typeof failure === 'object' && 'httpStatus' in failure
        ? failure.httpStatus
        : undefined;
    const remoteMessage =
      failure instanceof LocalizedError
        ? failure.text
        : failure instanceof Error
          ? failure.message
          : failure &&
              typeof failure === 'object' &&
              'message' in failure &&
              typeof failure.message === 'string'
            ? failure.message
            : undefined;
    setError(
      localized((m) => {
        const message =
          (remoteMessage === undefined
            ? undefined
            : renderMessage(remoteMessage, m)) ??
          (status === 404 && source === 'bots'
            ? m.desktopThisRuntimeDoesNotYetSupportDesktopBotCreation()
            : m.desktopTheOperationFailed());
        const labels: Record<string, string> = {
          sessions: m.desktopLoadConversations(),
          bots: m.desktopCreateDesktopBot(),
          workspace: m.desktopOpenLocalWorkspace(),
          connection: m.desktopReadConnection(),
        };
        return `${labels[source] || source}${status ? ` (HTTP ${status})` : ''}: ${message}`;
      }),
    );
  }, []);
  const dirty = useRef(false);
  const epoch = useRef(connection?.epoch);
  epoch.current = connection?.epoch;
  const connectionScope = useRef(connection?.scope);
  connectionScope.current = connection?.scope;
  const draftStore = useRef(new ComposerDraftStore());
  draftStore.current.reset(connection?.epoch, connection?.scope);
  const [, draftChanged] = useState(0);
  const notifyDraft = useCallback(
    () => draftChanged((revision) => revision + 1),
    [],
  );
  const sessionsRef = useRef(sessions);
  sessionsRef.current = sessions;
  const lifecycle = useRef(0);
  useEffect(() => {
    if (!services.host) return;
    let active = true;
    const handled = new Set<number>();
    const close = async (requestId: number) => {
      if (!active || handled.has(requestId)) return;
      handled.add(requestId);
      try {
        await services.workspace.flush();
        if (active) await services.host!.resolveClose(requestId, true);
      } catch (error) {
        if (active) {
          if (error && typeof error === 'object' && 'closeState' in error) {
            const state = error.closeState as {
              unsavedDrafts?: RecoveryDraftVM[];
            };
            if (Array.isArray(state.unsavedDrafts))
              setCloseDrafts(state.unsavedDrafts);
          }
          setError(
            errorMessage(
              error,
              localized((m) =>
                m.desktopDraftRecoveryCouldNotBeSavedContinueEditingOr(),
              ),
            ),
          );
          await services.host!.resolveClose(requestId, false);
        }
      }
    };
    const remove = services.host.onClose((id) => void close(id));
    void services.host
      .closeState()
      .then((state) => {
        if (active && state.pending) void close(state.requestId);
      })
      .catch((error: unknown) => {
        if (active)
          setError(
            errorMessage(
              error,
              localized((m) => m.desktopCouldNotRegisterTheCloseProtocol()),
            ),
          );
      });
    return () => {
      active = false;
      remove();
    };
  }, [services]);
  useEffect(() => {
    void services.host
      ?.theme?.()
      .then(({ theme }) => {
        const next = theme as ThemePreference;
        setThemePreference(next);
        setTheme(next);
      })
      .catch(() => {});
  }, [services]);
  useEffect(() => {
    const current = ++lifecycle.current;
    void services.connection
      .status()
      .then((value) => {
        if (lifecycle.current === current) setConnection(value);
      })
      .catch((error: unknown) => {
        if (lifecycle.current === current) reportFailure(error, 'connection');
      });
    return () => {
      lifecycle.current++;
    };
  }, [services]);
  useEffect(() => {
    let active = true;
    void services.platform
      ?.status()
      .then((platform) => {
        if (!active) return;
        platformSignedIn.current = platform.signedIn;
        platformRegion.current = platform.region || 'cn-beijing';
        setPlatform(platform);
        if (!platform.signedIn) {
          setRuntimeOptions([]);
          return;
        }
        return services.platform
          .list(platform.region || 'cn-beijing', 'default', false)
          .then((list) => {
            if (active) setRuntimeOptions(list);
          });
      })
      .catch(() => {
        if (active) setRuntimeOptions([]);
      });
    return () => {
      active = false;
    };
  }, [services, connection?.epoch]);
  const refreshRuntimes = useCallback(async () => {
    const current = await services.platform.status();
    setPlatform(current);
    if (!current.signedIn) {
      setRuntimeOptions([]);
      return;
    }
    const list = await services.platform.list(
      current.region || 'cn-beijing',
      'default',
      true,
    );
    const latest = await services.platform.status();
    if (latest.epoch === current.epoch && latest.signedIn)
      setRuntimeOptions(list);
  }, [services]);
  const refresh = useCallback(
    async (forceRefresh = false) => {
      if (!connection) return;
      const current = connection.epoch;
      const scope = connection.scope;
      const list = await services.sessions.list(current, forceRefresh);
      if (epoch.current === current && connectionScope.current === scope) {
        setSessions(list);
        setSessionEpoch(current);
        if (errorSource.current === 'sessions') setError('');
      }
    },
    [services, connection],
  );
  useEffect(() => {
    setSessions([]);
    setSessionEpoch(0);
    setSelected(undefined);
    void refresh().catch((error: unknown) => {
      if (epoch.current === connection?.epoch) reportFailure(error, 'sessions');
    });
  }, [refresh, connection?.epoch]);
  const navigate = (next: View) => {
    if (
      dirty.current &&
      next !== view &&
      !window.confirm(m.unsavedPageConfirm())
    )
      return false;
    setView(next);
    return true;
  };
  const setDirty = useCallback((value: boolean) => {
    dirty.current = value;
  }, []);
  const connect = useCallback((next: ConnectionVM | undefined) => {
    lifecycle.current++;
    epoch.current = next?.epoch;
    connectionScope.current = next?.scope;
    draftStore.current.reset(next?.epoch, next?.scope);
    sessionsRef.current = [];
    setConnection(next);
    setSessions([]);
    setSessionEpoch(0);
    setSelected(undefined);
    if (['sessions', 'bots', 'connection'].includes(errorSource.current))
      setError('');
  }, []);
  const createSession = async () => {
    if (!connection) {
      navigate('settings');
      return;
    }
    const current = connection.epoch;
    const scope = connection.scope;
    try {
      const name = window.prompt(m.conversationTitle(), m.newConversation());
      if (!name?.trim()) return;
      const next = await services.sessions.create(
        current,
        name.trim(),
        session?.mpaInstanceId ||
          sessions.find((item) => item.mpaInstanceId)?.mpaInstanceId,
      );
      if (epoch.current === current && connectionScope.current === scope) {
        setSessions((previous) => [next, ...previous]);
        setSessionEpoch(current);
        setSelected(next.id);
        navigate('chat');
      }
    } catch (error) {
      if (epoch.current === current) reportFailure(error, 'bots');
    }
  };
  const session = sessions.find((session) => session.id === selected);
  const conversationTarget: ConversationTarget | undefined =
    connection && session && sessionEpoch === connection.epoch
      ? {
          connectionEpoch: connection.epoch,
          scope: connection.scope,
          sessionId: session.id,
        }
      : undefined;
  const composerController = useMemo(() => {
    if (!connection || !session || sessionEpoch !== connection.epoch)
      return undefined;
    const target = {
      connectionEpoch: connection.epoch,
      scope: connection.scope,
      sessionId: session.id,
    };
    return {
      setValue: (update: Parameters<ComposerDraft['setValue']>[0]) => {
        draftStore.current.set(target, update);
        notifyDraft();
      },
      snapshot: () => draftStore.current.snapshot(target),
      accept: (snapshot: ReturnType<ComposerDraft['snapshot']>) => {
        draftStore.current.accept(snapshot);
        notifyDraft();
      },
    };
  }, [
    connection?.epoch,
    connection?.scope,
    session?.id,
    sessionEpoch,
    notifyDraft,
  ]);
  const composerDraft: ComposerDraft | undefined =
    composerController && conversationTarget
      ? {
          ...composerController,
          value: draftStore.current.value(conversationTarget),
        }
      : undefined;
  const useInConversation = (target: ConversationTarget, text: string) => {
    if (
      target.connectionEpoch !== epoch.current ||
      target.scope !== connectionScope.current ||
      target.sessionId !== selectedRef.current ||
      !sessionsRef.current.some((item) => item.id === target.sessionId)
    )
      return false;
    if (!navigate('chat')) return false;
    const inserted = draftStore.current.insert(target, text);
    if (inserted) notifyDraft();
    return inserted;
  };
  const shellAssistants = useMemo(() => {
    const items = runtimeOptions.map((runtime) => ({
      id: runtime.id,
      title: runtime.name || runtime.id,
    }));
    if (
      connection?.runtimeId &&
      !items.some((item) => item.id === connection.runtimeId)
    )
      items.unshift({
        id: connection.runtimeId,
        title: connection.runtimeName || connection.runtimeId,
      });
    return items;
  }, [runtimeOptions, connection?.runtimeId, connection?.runtimeName]);
  const shellRevision = useRef(Date.now() * 1000);
  const onNativeNavigation = useEffectEvent((action: string, id?: string) => {
    if (action === 'connectionChanged') {
      const request = ++lifecycle.current;
      void services.connection
        .status()
        .then((value) => {
          if (lifecycle.current === request) connect(value);
        })
        .catch((error: unknown) => {
          if (lifecycle.current === request) reportFailure(error, 'connection');
        });
    } else if (
      (action === 'sessionCreated' ||
        action === 'sessionChanged' ||
        action === 'sessionDeleted') &&
      id &&
      connection
    ) {
      const current = connection.epoch;
      const scope = connection.scope;
      void services.sessions
        .list(current, true)
        .then((list) => {
          if (epoch.current !== current || connectionScope.current !== scope)
            return;
          setSessions(list);
          setSessionEpoch(current);
          if (action === 'sessionDeleted') {
            draftStore.current.remove(id);
            if (selected === id) setSelected(undefined);
          } else if (list.some((item) => item.id === id)) {
            setSelected(id);
            navigate('chat');
          }
        })
        .catch((error: unknown) => reportFailure(error, 'sessions'));
    } else if (action === 'themeChanged' && id) {
      const next = id as ThemePreference;
      setThemePreference(next);
      setTheme(next);
    } else if (action === 'toggleSidebar')
      setCollapsed((previous) => !previous);
    else if (action === 'trace') {
      if (connection && selected) setTraceOpen((previous) => !previous);
    } else if (
      action === 'selectSettingsSection' &&
      [
        'Account',
        'Runtime',
        'Assistant',
        'Authentication',
        'Deployment',
        'Theme',
        'Language',
      ].includes(id || '')
    ) {
      setSettingsSection(id as DesktopSettingsSection);
      navigate('settings');
    } else if (action === 'newConversation') void createSession();
    else if (action === 'selectAssistant' && id) {
      if (connection?.runtimeId === id) return;
      const runtime = runtimeOptions.find((item) => item.id === id);
      if (!runtime || !platformSignedIn.current) {
        setSettingsSection('Runtime');
        navigate('settings');
        return;
      }
      void services.platform
        .bind(platformRegion.current, 'default', runtime.id)
        .then((next) => {
          connect(next);
          navigate('chat');
        })
        .catch((error: unknown) => reportFailure(error, 'connection'));
    } else if (action === 'home') {
      setCollapsed(false);
      navigate('chat');
    } else if (action === 'refreshSessions')
      void refresh(true).catch((error: unknown) =>
        reportFailure(error, 'sessions'),
      );
    else if (
      action === 'selectSession' &&
      sessions.some((item) => item.id === id)
    ) {
      if (navigate('chat')) setSelected(id);
    } else if (
      action === 'tasks' ||
      action === 'settings' ||
      action === 'channels' ||
      action === 'knowledge' ||
      action === 'resources' ||
      action === 'collaboration' ||
      action === 'runtimes'
    )
      navigate(action);
  });
  useEffect(
    () => services.host?.onNavigation?.(onNativeNavigation),
    [services],
  );
  useEffect(() => {
    void services.host
      ?.setShellState?.({
        connectionEpoch: sessionEpoch,
        view,
        collapsed,
        traceOpen,
        location: connection?.runtimeId ?? m.yourWorkspace(),
        pageTitle:
          view === 'runtimes'
            ? m.manageRuntimes()
            : view === 'resources'
              ? m.resources()
              : view === 'collaboration'
                ? m.collaboration()
                : view === 'settings'
                  ? m.settings()
                  : view === 'channels'
                    ? m.channels()
                    : view === 'knowledge'
                      ? m.knowledge()
                      : view === 'tasks'
                        ? m.scheduledTasks()
                        : m.conversations(),
        selectedSession: selected ?? '',
        selectedSpace: '',
        selectedAssistant: connection?.runtimeId ?? '',
        settingsSection,
        theme,
        sessions: sessions.map((item) => ({
          id: item.id,
          title: item.title || item.id,
          agentId: item.mpaInstanceId ?? '',
        })),
        spaces: [],
        assistants: shellAssistants,
        revision: ++shellRevision.current,
      })
      .catch(() => {});
  }, [
    services,
    m,
    view,
    collapsed,
    traceOpen,
    connection?.runtimeId,
    shellAssistants,
    selected,
    sessions,
    sessionEpoch,
    settingsSection,
    theme,
  ]);
  return {
    error,
    errorSource,
    setError,
    closeDrafts,
    services,
    setCloseDrafts,
    view,
    traceOpen,
    setTraceOpen,
    settingsSection,
    setSettingsSection,
    theme,
    setTheme,
    connection,
    platform,
    conversationTarget,
    composerDraft,
    useInConversation,
    sessions,
    setDirty,
    navigate,
    setSelected,
    session,
    reportFailure,
    epoch,
    refresh,
    createSession,
    connect,
    refreshRuntimes,
  };
}
