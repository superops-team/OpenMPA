import { useEffect, useRef, useState } from 'react';
import type {
  ConnectionVM,
  DesktopServices,
  ExecutionConfigVM,
  SessionVM,
} from './models';
import { isKnownModel } from './model-catalog';

export type ModelSelectionMode = 'inherit' | 'clear' | 'replace';

export function modelSelectionLabel(config?: ExecutionConfigVM) {
  if (!config) return 'Runtime default';
  if (config.modelMode === 'clear') return 'No model override';
  return config.model || 'Runtime default';
}

function failureMessage(failure: unknown) {
  return failure instanceof Error
    ? failure.message
    : 'Could not update the conversation model.';
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
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [needsInitialization, setNeedsInitialization] = useState(false);
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
    if (busy || locked) return false;
    if (mode === 'replace' && !isKnownModel(model.trim())) {
      setError(
        `Model ${model.trim() || '(empty)'} is not in the Runtime catalog. Choose a listed model.`,
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
        'Model applies to future turns. Existing turns keep their configuration.',
      );
      return true;
    } catch (failure: unknown) {
      if (generation.current === requestGeneration)
        setError(
          isMissingConfiguration(failure)
            ? 'This legacy diagnostic conversation does not expose next-turn model configuration. Ordinary conversations remain model-switchable between completed turns.'
            : failureMessage(failure),
        );
      return false;
    } finally {
      if (generation.current === requestGeneration) setBusy(false);
    }
  };
  return {
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
