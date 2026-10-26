import {
  useCallback,
  useEffect,
  useEffectEvent,
  useRef,
  useState,
} from 'react';

import type {
  ConnectionVM,
  DesktopServices,
  RecoveryDraftVM,
  SessionVM,
  Space,
} from './models';
type View = 'chat' | 'pages' | 'tasks' | 'channels' | 'settings' | 'knowledge';
export function useDesktopAppState({
  services,
}: {
  services: DesktopServices;
}) {
  const [view, setView] = useState<View>('chat');
  const [connection, setConnection] = useState<ConnectionVM>();
  const [sessions, setSessions] = useState<SessionVM[]>([]);
  const [sessionEpoch, setSessionEpoch] = useState(0);
  const [selected, setSelected] = useState<string>();
  const [spaces, setSpaces] = useState<Space[]>([]);
  const [spaceId, setSpaceId] = useState<string>();
  const [collapsed, setCollapsed] = useState(false);
  const [pane, setPane] = useState(false);
  const [error, setError] = useState('');
  const [closeDrafts, setCloseDrafts] = useState<RecoveryDraftVM[]>([]);
  const errorSource = useRef('');
  const reportFailure = useCallback((failure: unknown, source: string) => {
    errorSource.current = source;
    const status =
      failure && typeof failure === 'object' && 'httpStatus' in failure
        ? failure.httpStatus
        : undefined;
    const message =
      status === 404 && source === 'bots'
        ? 'This Runtime does not yet support desktop Bot creation and configuration.'
        : failure instanceof Error
          ? failure.message
          : 'The operation failed.';
    const labels: Record<string, string> = {
      sessions: 'Load conversations',
      bots: 'Create desktop Bot',
      workspace: 'Open local workspace',
      connection: 'Read connection',
    };
    setError(
      `${labels[source] || source}${status ? ` (HTTP ${status})` : ''}: ${message}`,
    );
  }, []);
  const dirty = useRef(false);
  const epoch = useRef(connection?.epoch);
  epoch.current = connection?.epoch;
  const connectionScope = useRef(connection?.scope);
  connectionScope.current = connection?.scope;
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
            error instanceof Error
              ? error.message
              : 'Draft recovery could not be saved. Continue editing or export a copy before closing.',
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
            error instanceof Error
              ? error.message
              : 'Could not register the close protocol.',
          );
      });
    return () => {
      active = false;
      remove();
    };
  }, [services]);
  useEffect(() => {
    const current = ++lifecycle.current;
    void services.workspace
      .spaces()
      .then((list) => {
        if (lifecycle.current === current) {
          setSpaces(list);
          setSpaceId(list[0]?.id);
        }
      })
      .catch((error: unknown) => {
        if (lifecycle.current === current) reportFailure(error, 'workspace');
      });
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
  const refresh = useCallback(async () => {
    if (!connection) return;
    const current = connection.epoch;
    const list = await services.sessions.list(current);
    if (epoch.current === current) {
      setSessions(list);
      setSessionEpoch(current);
      if (errorSource.current === 'sessions') setError('');
    }
  }, [services, connection]);
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
      !window.confirm(
        'This page has unsaved changes. Save or export before leaving. Discard this draft and continue?',
      )
    )
      return false;
    setView(next);
    return true;
  };
  const setDirty = useCallback((value: boolean) => {
    dirty.current = value;
  }, []);
  const connect = useCallback((next: ConnectionVM | undefined) => {
    setConnection(next);
    setSessions([]);
    setSessionEpoch(0);
    setSelected(undefined);
    if (['sessions', 'bots', 'connection'].includes(errorSource.current))
      setError('');
  }, []);
  const createSession = async () => {
    if (services.host?.requestConversation) {
      try {
        await services.host.requestConversation();
      } catch (error) {
        reportFailure(error, 'bots');
      }
      return;
    }
    if (!connection) {
      navigate('settings');
      return;
    }
    const current = connection.epoch;
    try {
      const name = window.prompt('Bot name', 'New assistant');
      if (!name?.trim()) return;
      const next = await services.sessions.create(current, name.trim());
      if (epoch.current === current) {
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
  const space = spaces.find((space) => space.id === spaceId);
  const shellRevision = useRef(Date.now() * 1000);
  const onNativeNavigation = useEffectEvent((action: string, id?: string) => {
    if (action === 'connectionChanged')
      void services.connection
        .status()
        .then(connect)
        .catch((error: unknown) => reportFailure(error, 'connection'));
    else if (
      (action === 'sessionCreated' ||
        action === 'sessionChanged' ||
        action === 'sessionDeleted') &&
      id &&
      connection
    ) {
      const current = connection.epoch;
      void services.sessions
        .list(current)
        .then((list) => {
          if (epoch.current !== current) return;
          setSessions(list);
          setSessionEpoch(current);
          if (action === 'sessionDeleted') {
            if (selected === id) setSelected(undefined);
          } else if (list.some((item) => item.id === id)) {
            setSelected(id);
            navigate('chat');
          }
        })
        .catch((error: unknown) => reportFailure(error, 'sessions'));
    } else if (action === 'spaceCreated' && id) {
      void services.workspace
        .spaces()
        .then((list) => {
          setSpaces(list);
          if (list.some((item) => item.id === id)) {
            setSpaceId(id);
            navigate('pages');
          }
        })
        .catch((error: unknown) => reportFailure(error, 'workspace'));
    } else if (action === 'toggleSidebar')
      setCollapsed((previous) => !previous);
    else if (action === 'artifacts') setPane((previous) => !previous);
    else if (action === 'newConversation') void createSession();
    else if (action === 'home') navigate('chat');
    else if (action === 'refreshSessions')
      void refresh().catch((error: unknown) =>
        reportFailure(error, 'sessions'),
      );
    else if (
      action === 'selectSession' &&
      sessions.some((item) => item.id === id)
    ) {
      if (navigate('chat')) setSelected(id);
    } else if (
      action === 'selectSpace' &&
      spaces.some((item) => item.id === id)
    ) {
      if (
        dirty.current &&
        view === 'pages' &&
        id !== spaceId &&
        !window.confirm('Discard unsaved page draft and switch Spaces?')
      )
        return;
      if (navigate('pages')) setSpaceId(id);
    } else if (action === 'createSpace') {
      const name = window.prompt('Space name');
      if (name?.trim())
        void services.workspace
          .createSpace(name.trim())
          .then((next) => {
            setSpaces((previous) => [...previous, next]);
            setSpaceId(next.id);
            navigate('pages');
          })
          .catch((error: unknown) => reportFailure(error, 'workspace'));
    } else if (
      action === 'pages' ||
      action === 'tasks' ||
      action === 'settings' ||
      action === 'channels' ||
      action === 'knowledge'
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
        pane,
        location: connection?.runtimeId ?? 'Your workspace',
        pageTitle: view === 'pages' ? (space?.name ?? 'Spaces') : view,
        selectedSession: selected ?? '',
        selectedSpace: spaceId ?? '',
        sessions: sessions.map((item) => ({
          id: item.id,
          title: item.title || item.id,
        })),
        spaces: spaces.map((item) => ({ id: item.id, title: item.name })),
        revision: ++shellRevision.current,
      })
      .catch(() => {});
  }, [
    services,
    view,
    collapsed,
    pane,
    connection?.runtimeId,
    space?.name,
    selected,
    spaceId,
    sessions,
    spaces,
    sessionEpoch,
  ]);
  return {
    error,
    errorSource,
    setError,
    closeDrafts,
    services,
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
  };
}
