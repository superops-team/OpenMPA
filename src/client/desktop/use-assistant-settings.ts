import { useEffect, useRef, useState } from 'react';
import type {
  AssistantConfigVM,
  AssistantSetupInput,
  AssistantSkillVM,
  ConnectionVM,
  DesktopServices,
  ExecutionConfigVM,
  SessionVM,
} from './models';
import { uniqueSkills } from './AssistantSkills';

const emptyConfig = (id: string): AssistantConfigVM => ({
  assistantId: id,
  agentsMd: '',
  skills: [],
});
const defaultModel: ExecutionConfigVM = {
  modelMode: 'inherit',
  mcpMode: 'inherit',
  mcpServers: [],
  executionConfigVersion: '',
  profileBound: false,
  invalidRefsCount: 0,
};

export function useAssistantSettings({
  services,
  connection,
  session,
  onSaved,
  onCreate,
  onClose,
}: {
  services: DesktopServices;
  connection: ConnectionVM;
  session?: SessionVM;
  onSaved?: (session: SessionVM) => void;
  onCreate?: (input: AssistantSetupInput) => Promise<void>;
  onClose: () => void;
}) {
  const id = session?.id || '';
  const [name, setName] = useState(session?.title || '');
  const [config, setConfig] = useState(() => emptyConfig(id));
  const [catalog, setCatalog] = useState<AssistantSkillVM[]>([]);
  const [model, setModel] = useState<ExecutionConfigVM>(defaultModel);
  const savedModel = useRef<ExecutionConfigVM | undefined>(undefined);
  const [loading, setLoading] = useState(!!session);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>();
  const [skillsFailed, setSkillsFailed] = useState(false);
  const [modelUnavailable, setModelUnavailable] = useState(false);
  const generation = useRef(0);
  const saving = useRef(false);
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    const current = ++generation.current;
    const active = () => generation.current === current;
    setError(undefined);
    setSkillsFailed(false);
    setModelUnavailable(false);
    void Promise.resolve()
      .then(() => services.sessions.skillCatalog(connection.epoch, ''))
      .then(
        (value) => {
          if (active()) setCatalog(value);
        },
        () => {
          if (active()) setSkillsFailed(true);
        },
      );
    // Creation starts with the final form immediately. Metadata never replaces
    // its draft, even if it arrives after the user has begun typing.
    if (id) {
      setLoading(true);
      void Promise.all([
        services.sessions.assistantConfig(connection.epoch, id, id),
        services.sessions
          .executionConfig(connection.epoch, id)
          .catch((failure: unknown) => {
            if (
              failure &&
              typeof failure === 'object' &&
              'code' in failure &&
              failure.code === 'diagnosticModelUnsupported'
            ) {
              if (active()) setModelUnavailable(true);
              return defaultModel;
            }
            throw failure;
          }),
      ]).then(
        ([value, execution]) => {
          if (!active()) return;
          setConfig(value);
          setModel(execution);
          savedModel.current = execution;
          setLoading(false);
        },
        (failure: unknown) => {
          if (active()) setError(failure);
        },
      );
    }
    return () => {
      generation.current++;
    };
  }, [services, connection.epoch, id, attempt]);

  const save = async () => {
    if (loading || saving.current || !name.trim()) return;
    saving.current = true;
    setBusy(true);
    setError(undefined);
    const current = generation.current;
    const active = () => current === generation.current;
    const input: AssistantSetupInput = {
      name: name.trim(),
      agentsMd: config.agentsMd,
      skills: uniqueSkills(config.skills),
      modelMode: model.modelMode,
      model: model.modelMode === 'replace' ? model.model || '' : '',
    };
    try {
      if (!session) {
        if (!onCreate) throw new Error('assistantCreationUnavailable');
        await onCreate(input);
        if (active()) onClose();
        return;
      }
      let execution = savedModel.current;
      const modelChanged =
        execution?.modelMode !== input.modelMode ||
        (input.modelMode === 'replace' && execution?.model !== input.model);
      if (modelChanged && !modelUnavailable) {
        if (!execution?.etag) throw new Error('executionConfigUnavailable');
        execution = await services.sessions.updateExecutionConfig(
          connection.epoch,
          id,
          {
            etag: execution.etag,
            modelMode: input.modelMode,
            model: input.model,
            mcpMode: execution.mcpMode,
            mcpServers: execution.mcpServers,
          },
        );
        if (!active()) return;
        // Preserve a successful CAS revision if a later local write fails.
        savedModel.current = execution;
        setModel(execution);
      }
      await services.sessions.saveAssistantConfig(connection.epoch, id, id, {
        agentsMd: input.agentsMd,
        skills: input.skills,
      });
      if (!active()) return;
      if (input.name !== session.title)
        await services.sessions.rename(connection.epoch, id, input.name);
      if (!active()) return;
      onSaved?.({
        ...session,
        title: input.name,
        configVersion:
          execution?.executionConfigVersion ?? session.configVersion,
      });
    } catch (failure) {
      if (active()) setError(failure);
    } finally {
      saving.current = false;
      if (active()) setBusy(false);
    }
  };
  return {
    name,
    setName,
    config,
    setConfig,
    catalog,
    model,
    setModel,
    loading,
    busy,
    error,
    skillsFailed,
    modelUnavailable,
    save,
    retry: () => setAttempt((value) => value + 1),
  };
}
