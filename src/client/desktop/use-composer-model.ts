import { useEffect, useRef, useState } from 'react';
import type {
  ConnectionVM,
  DesktopServices,
  ExecutionConfigVM,
  SessionVM,
} from './models';
import { isKnownModel } from './model-catalog';
import { useModelCatalog } from './use-model-catalog';
import { useMessages, localized, useMessageState } from './locale';
import type { Messages } from './generated/locales';

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
    m?.desktopRuntimeDefault() ||
    'Runtime default'
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
  const m = useMessages();
  const [config, setConfig] = useState<ExecutionConfigVM>();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useMessageState();
  const [notice, setNotice] = useMessageState();
  const [needsInitialization, setNeedsInitialization] = useState(false);
  const [unavailable, setUnavailable] = useState(false);
  const catalog = useModelCatalog(services, connection, session.id);
  const modelCatalog = catalog.models;
  const generation = useRef(0);
  useEffect(() => {
    const current = ++generation.current;
    setConfig(undefined);
    setBusy(false);
    setError('');
    setNotice('');
    setNeedsInitialization(false);
    setUnavailable(false);
    if (!services.sessions.executionConfig) return;
    void services.sessions
      .executionConfig(connection.epoch, session.id)
      .then((value) => {
        if (generation.current === current) setConfig(value);
      })
      .catch((failure: unknown) => {
        if (generation.current !== current) return;
        if (
          failure &&
          typeof failure === 'object' &&
          'code' in failure &&
          failure.code === 'diagnosticModelUnsupported'
        ) {
          setUnavailable(true);
          setError(failureMessage(failure));
          return;
        }
        if (isMissingConfiguration(failure)) {
          setNeedsInitialization(true);
          return;
        }
        setError(failureMessage(failure));
      });
    return () => {
      generation.current++;
    };
  }, [
    services,
    connection.epoch,
    connection.runtimeId,
    connection.scope,
    session.id,
    session.configVersion,
  ]);
  const apply = async (mode: ModelSelectionMode, model = '') => {
    if (
      busy ||
      locked ||
      unavailable ||
      catalog.loading ||
      catalog.failed ||
      !catalog.supported
    )
      return false;
    if (
      mode === 'inherit' &&
      config?.modelMode === 'inherit' &&
      config.model &&
      !isKnownModel(config.model, modelCatalog)
    ) {
      setError(localized((m) => m.chatModelInvalid(config.model || '')));
      return false;
    }
    if (mode === 'replace' && !isKnownModel(model.trim(), modelCatalog)) {
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
        if (!services.sessions.ensureExecutionConfig)
          throw new Error(
            'This Runtime cannot initialize a next-turn model configuration.',
          );
        currentConfig = await services.sessions.ensureExecutionConfig(
          connection.epoch,
          session.id,
        );
        if (generation.current !== requestGeneration) return false;
        setConfig(currentConfig);
        setNeedsInitialization(false);
      }
      if (!currentConfig.etag)
        throw new Error(
          'The Runtime returned a next-turn configuration without a revision token.',
        );
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
        setError(failureMessage(failure));
      return false;
    } finally {
      if (generation.current === requestGeneration) setBusy(false);
    }
  };
  return {
    config,
    busy,
    error: unavailable ? m.diagnosticModelUnavailable() : error,
    notice,
    needsInitialization,
    unavailable,
    locked,
    editable: !!config?.etag,
    modelCatalog,
    catalog: {
      models: catalog.models,
      defaultModel: catalog.defaultModel,
      supported: catalog.supported,
    },
    catalogError: catalog.failed ? m.modelCatalogReadFailed() : '',
    refreshCatalog: catalog.retry,
    catalogLoading: catalog.loading,
    catalogFailed: catalog.failed,
    retryCatalog: catalog.retry,
    apply,
  };
}
