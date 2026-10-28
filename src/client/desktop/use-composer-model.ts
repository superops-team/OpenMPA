import { localized, useMessageState } from './locale';
import type { Messages } from './generated/locales';
import { useEffect, useRef, useState } from 'react';
import type {
  ConnectionVM,
  DesktopServices,
  ExecutionConfigVM,
  SessionVM,
} from './models';
import { isKnownModel, type RuntimeModelCatalog } from './model-catalog';

export type ModelSelectionMode = 'inherit' | 'clear' | 'replace';

export function modelSelectionLabel(
  config?: ExecutionConfigVM,
  m?: Messages,
  defaultModel = '',
) {
  if (config?.modelMode === 'clear')
    return m?.desktopNoModelOverride() ?? 'No model override';
  return (
    config?.model ||
    defaultModel ||
    (m?.desktopRuntimeDefault() ?? 'Runtime default')
  );
}

function failureMessage(failure: unknown) {
  return failure instanceof Error
    ? failure.message
    : localized((m) => m.desktopCouldNotUpdateTheConversationModel());
}

function isMissingConfiguration(failure: unknown) {
  return (
    !!failure &&
    typeof failure === 'object' &&
    'status' in failure &&
    failure.status === 404
  );
}

/**
 * Session execution configuration is authoritative and versioned. The composer
 * reads the current revision, then sends a CAS patch that only changes the
 * next Turn's model while preserving the existing MCP choice. A legacy 404 is
 * recoverable on the first explicit selection; opening a conversation never
 * writes remote state.
 */
export function useComposerModel({
  services,
  connection,
  session,
  onConfigVersion,
  locked = false,
}: {
  services: DesktopServices;
  connection: ConnectionVM;
  session: SessionVM;
  onConfigVersion?: (version: string) => void;
  locked?: boolean;
}) {
  const [config, setConfig] = useState<ExecutionConfigVM>();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useMessageState();
  const [notice, setNotice] = useMessageState();
  const [needsInitialization, setNeedsInitialization] = useState(false);
  const [catalog, setCatalog] = useState<RuntimeModelCatalog>({
    models: [],
    defaultModel: '',
    supported: false,
  });
  const [catalogLoading, setCatalogLoading] = useState(true);
  const [catalogError, setCatalogError] = useMessageState();
  const [catalogRefresh, setCatalogRefresh] = useState(0);
  const catalogGeneration = useRef(0);
  useEffect(() => {
    const current = ++catalogGeneration.current;
    setCatalog({ models: [], defaultModel: '', supported: false });
    setCatalogError('');
    setCatalogLoading(true);
    const request = services.sessions.models;
    if (!request) {
      setCatalogLoading(false);
      return;
    }
    void request(connection.epoch)
      .then((value) => {
        if (catalogGeneration.current === current) setCatalog(value);
      })
      .catch(() => {
        if (catalogGeneration.current === current)
          setCatalogError(localized((m) => m.modelCatalogReadFailed()));
      })
      .finally(() => {
        if (catalogGeneration.current === current) setCatalogLoading(false);
      });
    return () => {
      catalogGeneration.current++;
    };
  }, [services, connection.epoch, catalogRefresh]);
  const generation = useRef(0);
  useEffect(() => {
    const current = ++generation.current;
    setConfig(undefined);
    setError('');
    setNotice('');
    setNeedsInitialization(false);
    if (!services.sessions.executionConfig) return;
    void services.sessions
      .executionConfig(connection.epoch, session.id)
      .then((value) => {
        if (generation.current === current) setConfig(value);
      })
      .catch((failure: unknown) => {
        if (generation.current !== current) return;
        if (isMissingConfiguration(failure)) {
          setNeedsInitialization(true);
          return;
        }
        setError(failureMessage(failure));
      });
    return () => {
      generation.current++;
    };
  }, [services, connection.epoch, session.id]);
  const apply = async (mode: ModelSelectionMode, model = '') => {
    if (busy || locked || catalogLoading || !catalog.supported || catalogError)
      return false;
    if (mode === 'replace' && !isKnownModel(model.trim(), catalog.models)) {
      setError(
        localized((m) =>
          m.chatModelInvalid(model.trim() || m.chatEmptyModel()),
        ),
      );
      return false;
    }
    const requestGeneration = generation.current;
    setBusy(true);
    setError('');
    setNotice('');
    try {
      let currentConfig = config;
      if (!currentConfig?.etag) {
        if (!services.sessions.ensureExecutionConfig) {
          setError(
            localized((m) =>
              m.desktopThisRuntimeCannotInitializeANextTurnModelConfiguration(),
            ),
          );
          return false;
        }
        currentConfig = await services.sessions.ensureExecutionConfig(
          connection.epoch,
          session.id,
        );
        if (generation.current !== requestGeneration) return false;
        setConfig(currentConfig);
        setNeedsInitialization(false);
      }
      if (!currentConfig.etag) {
        setError(
          localized((m) =>
            m.desktopTheRuntimeReturnedANextTurnConfigurationWithoutA(),
          ),
        );
        return false;
      }
      const next = await services.sessions.updateExecutionConfig(
        connection.epoch,
        session.id,
        {
          etag: currentConfig.etag,
          modelMode: mode,
          model: mode === 'replace' ? model.trim() : '',
          mcpMode: currentConfig.mcpMode,
          mcpServers: currentConfig.mcpServers,
        },
      );
      if (generation.current !== requestGeneration) return false;
      setConfig(next);
      onConfigVersion?.(next.executionConfigVersion);
      setNotice(
        localized((m) =>
          m.desktopModelAppliesToFutureTurnsExistingTurnsKeepTheir(),
        ),
      );
      return true;
    } catch (failure: unknown) {
      if (generation.current === requestGeneration)
        setError(
          isMissingConfiguration(failure)
            ? localized((m) =>
                m.desktopThisLegacyDiagnosticConversationDoesNotExposeNextTurn(),
              )
            : failureMessage(failure),
        );
      return false;
    } finally {
      if (generation.current === requestGeneration) setBusy(false);
    }
  };
  return {
    catalog,
    catalogLoading,
    catalogError,
    refreshCatalog: () => setCatalogRefresh((value) => value + 1),
    config,
    busy,
    error,
    notice,
    needsInitialization,
    locked,
    editable: !!config?.etag,
    apply,
  };
}
