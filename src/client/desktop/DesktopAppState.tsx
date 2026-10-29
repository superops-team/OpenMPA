import { localized, useMessages, useMessageState } from './locale';
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
  AssistantSetupInput,
  RuntimeVM,
  SessionVM,
  PlatformVM,
} from './models';
import { ComposerDraftStore } from './composer-drafts';
import type { ComposerDraft, ConversationTarget } from './studio-models';
import { desktopFailureMessage } from './desktop-app-errors';
import {
  desktopPageTitle,
  type DesktopView as View,
} from './desktop-shell-state';
import { useDesktopCloseProtocol } from './use-desktop-close-protocol';
import {
  setThemePreference,
  themePreference,
  type ThemePreference,
} from '../theme';
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
  const setSelected = useCallback(
    (
      update:
        | string
        | undefined
        | ((previous: string | undefined) => string | undefined),
    ) => {
      const value =
        typeof update === 'function' ? update(selectedRef.current) : update;
      selectedRef.current = value;
      selectSession(value);
    },
    [],
  );
  const [collapsed, setCollapsed] = useState(false);
  const [traceOpen, setTraceOpen] = useState(false);
  const [settingsSection, setSettingsSection] =
    useState<DesktopSettingsSection>('Account');
  const [creatingAssistant, setCreatingAssistant] = useState(false);
  const creation = useRef<
    | { epoch: number; scope: string; id: string; mpaInstanceId?: string }
    | undefined
  >(undefined);
  const deleting = useRef(new Set<string>());
  const [assistantSettingsSessionId, setAssistantSettingsSessionId] =
    useState<string>();
  const [theme, setTheme] = useState<ThemePreference>(themePreference());
  const platformRegion = useRef('cn-beijing');
  const platformSignedIn = useRef(false);
  const [platform, setPlatform] = useState<PlatformVM>();
  const [error, setError] = useMessageState();
  const errorSource = useRef('');
  const reportFailure = useCallback((failure: unknown, source: string) => {
    errorSource.current = source;
    setError(localized((m) => desktopFailureMessage(m, failure, source)));
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
  const { closeDrafts, setCloseDrafts } = useDesktopCloseProtocol({
    services,
    setError,
  });
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
      if (connection.businessIdentity === 'missing') {
        blockMissingBusinessJWT();
        return;
      }
      const list = await services.sessions.list(current, forceRefresh);
      if (epoch.current === current && connectionScope.current === scope) {
        setSessions(list);
        setSessionEpoch(current);
        setSelected((previous) =>
          previous && list.some((item) => item.id === previous)
            ? previous
            : list[0]?.id,
        );
        if (errorSource.current === 'sessions') setError('');
      }
    },
    [services, connection],
  );
  useEffect(() => {
    setSessions([]);
    setSessionEpoch(0);
    setSelected(undefined);
    setCreatingAssistant(false);
    setAssistantSettingsSessionId(undefined);
    creation.current = undefined;
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
  const updateSession = useCallback((updated: SessionVM) => {
    setSessions((previous) =>
      previous.map((item) =>
        item.id === updated.id ? { ...item, ...updated } : item,
      ),
    );
  }, []);
  const blockMissingBusinessJWT = useCallback(() => {
    errorSource.current = 'sessions';
    setSessions([]);
    setSessionEpoch(0);
    setSelected(undefined);
    setError(localized((m) => m.businessJWTRequired()));
  }, [setSelected]);
  const createSession = async () => {
    if (!connection) {
      navigate('settings');
      return;
    }
    if (connection.businessIdentity === 'missing') {
      setSettingsSection('Authentication');
      navigate('settings');
      blockMissingBusinessJWT();
      return;
    }
    if (creatingAssistant) return;
    creation.current = {
      epoch: connection.epoch,
      scope: connection.scope,
      id: crypto.randomUUID(),
      mpaInstanceId:
        session?.mpaInstanceId ||
        sessions.find((item) => item.mpaInstanceId)?.mpaInstanceId,
    };
    setAssistantSettingsSessionId(undefined);
    setCreatingAssistant(true);
  };
  const saveNewAssistant = async (input: AssistantSetupInput) => {
    if (!connection || !creation.current)
      throw new Error('assistantCreationUnavailable');
    const current = connection.epoch;
    const request = creation.current;
    const assertCurrent = () => {
      if (
        epoch.current !== current ||
        connectionScope.current !== request.scope ||
        request.epoch !== current ||
        creation.current !== request
      )
        throw new Error('connectionChanged');
    };
    // Keep the UUID across all retries, including an unknown Create/Rename outcome.
    assertCurrent();
    const next = await services.sessions.create(
      current,
      input.name.trim(),
      request.mpaInstanceId,
      request.id,
    );
    assertCurrent();
    await services.sessions.saveAssistantConfig(current, next.id, next.id, {
      agentsMd: input.agentsMd,
      skills: input.skills,
    });
    assertCurrent();
    let configured = next;
    if (input.modelMode === 'replace' || input.modelMode === 'clear') {
      let currentConfig =
        services.sessions.ensureExecutionConfig &&
        (await services.sessions.ensureExecutionConfig(current, next.id));
      assertCurrent();
      if (!currentConfig?.etag) {
        currentConfig = await services.sessions.executionConfig(
          current,
          next.id,
        );
        assertCurrent();
      }
      if (!currentConfig?.etag) throw new Error('executionConfigUnavailable');
      if (currentConfig.etag) {
        const savedConfig = await services.sessions.updateExecutionConfig(
          current,
          next.id,
          {
            etag: currentConfig.etag,
            modelMode: input.modelMode,
            model: input.modelMode === 'replace' ? input.model : '',
            mcpMode: currentConfig.mcpMode,
            mcpServers: currentConfig.mcpServers,
          },
        );
        assertCurrent();
        configured = {
          ...next,
          configVersion: savedConfig.executionConfigVersion,
        };
      }
    }
    if (epoch.current === current) {
      setSessions((previous) => [
        configured,
        ...previous.filter((item) => item.id !== configured.id),
      ]);
      setSessionEpoch(current);
      setSelected(configured.id);
      setCreatingAssistant(false);
      creation.current = undefined;
      navigate('chat');
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
      restore: (
        snapshot: ReturnType<ComposerDraft['snapshot']>,
        cleared: ReturnType<ComposerDraft['snapshot']>,
      ) => {
        draftStore.current.restore(snapshot, cleared);
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
      if (connection.businessIdentity === 'missing') {
        blockMissingBusinessJWT();
        return;
      }
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
    else if (
      action === 'editSession' &&
      id &&
      sessions.some((item) => item.id === id)
    ) {
      if (navigate('chat')) {
        creation.current = undefined;
        setCreatingAssistant(false);
        setSelected(id);
        setAssistantSettingsSessionId(id);
      }
    } else if (
      action === 'deleteSession' &&
      id &&
      connection &&
      sessions.some((item) => item.id === id)
    ) {
      const target = sessions.find((item) => item.id === id);
      const current = connection.epoch;
      const deletionKey = `${current}:${id}`;
      if (deleting.current.has(deletionKey)) return;
      if (!window.confirm(m.deleteAssistantConfirm(target?.title || id)))
        return;
      deleting.current.add(deletionKey);
      void services.sessions
        .remove(current, id)
        .then(() => {
          if (
            epoch.current === current &&
            connectionScope.current === connection.scope
          ) {
            draftStore.current.remove(id);
            setSelected((previous) => (previous === id ? undefined : previous));
            setAssistantSettingsSessionId((previous) =>
              previous === id ? undefined : previous,
            );
            return refresh(true);
          }
        })
        .catch((error: unknown) => {
          if (epoch.current === current) reportFailure(error, 'sessions');
        })
        .finally(() => deleting.current.delete(deletionKey));
    } else if (action === 'selectAssistant' && id) {
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
        pageTitle: desktopPageTitle(m, view),
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
    creatingAssistant,
    setCreatingAssistant,
    saveNewAssistant,
    updateSession,
    assistantSettingsSessionId,
    setAssistantSettingsSessionId,
    connect,
    refreshRuntimes,
  };
}
