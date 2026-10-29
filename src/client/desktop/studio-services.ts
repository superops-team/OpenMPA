import { Usage } from './generated/mygo';
import { decodeResult, unwrap, type SafeFailure } from './result';
import { localized, LocalizedError, type MessageText } from './locale';
import type {
  CollaborationServices,
  ResourcesServices,
  MainProfileOutcome,
} from './studio-models';
import type { MainProfileResult } from './generated/mygo';

export function studioFailure(error: unknown): MessageText {
  const code =
    error && typeof error === 'object' && 'code' in error ? error.code : '';
  return localized((m) => {
    switch (code) {
      case 'runtimeNameTaken':
        return m.mpaNameTaken();
      case 'lifecycleUnavailable':
        return m.mpaEngineUnavailable();
      case 'creationRequestFailed':
        return m.mpaRequestRejected();
      case 'operationBusy':
        return m.mpaDeleteBusy();
      case 'staleDeletePreview':
        return m.mpaDeleteChanged();
      case 'authRequired':
        return m.authRequired();
      case 'businessIdentityRequired':
        return m.businessIdentityRequired();
      case 'businessJWTRequired':
        return m.businessJWTRequired();
      case 'conflict':
        return m.conflict();
      case 'outcomeUnknown':
        return m.outcomeUnknown();
      case 'staleConnection':
        return m.staleConnection();
      case 'capabilityUnavailable':
        return m.capabilityUnavailable();
      case 'resourceNotVisible':
        return m.resourceNotVisible();
      case 'invalidResponse':
        return m.invalidResponse();
      case 'skillArchiveInvalid':
        return m.skillArchiveInvalid();
      case 'skillNameConflict':
        return m.skillNameConflict();
      case 'skillStorageFailed':
        return m.skillStorageFailed();
      case 'skillProcessingFailed':
        return m.skillProcessingFailed();
      case 'invalidInput':
        return m.invalidInput();
      case 'denied':
        return m.denied();
      case 'connectionFailure':
        return m.connectionFailure();
      default:
        return m.unavailable();
    }
  });
}

function read<T>(result: {
  ok: boolean;
  value?: T | null;
  error?: SafeFailure | null;
}): T {
  try {
    return unwrap(result);
  } catch (error) {
    throw Object.assign(new LocalizedError(studioFailure(error)), error, {
      text: studioFailure(error),
    });
  }
}
function profile(result: MainProfileResult): MainProfileOutcome {
  const decoded = decodeResult(result);
  return decoded.ok
    ? { status: result.status, value: decoded.value }
    : { status: result.status, error: decoded.error };
}
const bindingVM = (binding: {
  space_id: string | null;
  space_name: string | null;
  region: string;
  revision: number;
}) => ({
  spaceId: binding.space_id,
  spaceName: binding.space_name,
  region: binding.region,
  revision: binding.revision,
});

export function createStudioServices(bridge?: typeof Usage): {
  resources: ResourcesServices;
  collaboration: CollaborationServices;
} {
  const native = () => bridge ?? Usage;
  return {
    resources: {
      deleteSkill: async (connectionEpoch, spaceId, skillId) =>
        read(await native().deleteSkill({ connectionEpoch, spaceId, skillId })),
      uploadSkill: async (
        connectionEpoch,
        spaceId,
        operationId,
        filename,
        archiveBase64,
      ) =>
        read(
          await native().uploadSkill({
            connectionEpoch,
            spaceId,
            operationId,
            filename,
            archiveBase64,
          }),
        ),
      readSkillUpload: async (connectionEpoch, operationId) =>
        read(await native().readSkillUpload({ connectionEpoch, operationId })),

      discoverSkillSpaces: async (connectionEpoch) =>
        read(await native().boundSkillSpaces({ connectionEpoch })).spaces,
      listSkills: async (
        connectionEpoch,
        region,
        spaceId,
        pageNumber,
        pageSize,
      ) =>
        read(
          await native().listWorkerSkills({
            connectionEpoch,
            region,
            spaceId,
            pageNumber,
            pageSize,
          }),
        ),
      listWorkerMCPServers: async (connectionEpoch) =>
        read(await native().workerMCPServers({ connectionEpoch })),
      readAgentsMD: async (connectionEpoch) =>
        read(await native().readAgentsMD({ connectionEpoch })),
      saveAgentsMD: async (connectionEpoch, document) =>
        read(await native().saveAgentsMD({ connectionEpoch, document })),
      readMainProfile: async (connectionEpoch, mpaInstanceId = '') =>
        profile(
          await native().readMainProfile({ connectionEpoch, mpaInstanceId }),
        ),
      createMainProfile: async (connectionEpoch, value) =>
        profile(
          await native().createMainProfile({ connectionEpoch, profile: value }),
        ),
      saveMainProfile: async (connectionEpoch, snapshotId, value) =>
        profile(
          await native().saveMainProfile({
            connectionEpoch,
            snapshotId,
            profile: value,
          }),
        ),
      saveMainProfileInstructions: async (
        connectionEpoch,
        snapshotId,
        instructions,
      ) =>
        profile(
          await native().saveMainProfileInstructions({
            connectionEpoch,
            snapshotId,
            instructions,
          }),
        ),
    },
    collaboration: {
      createSpace: async (connectionEpoch, name, projectName, intentEnabled) =>
        read(
          await native().createCollaborationSpace({
            connectionEpoch,
            name,
            projectName,
            intentEnabled,
          }),
        ),
      registerAgent: async (
        connectionEpoch,
        spaceId,
        projectName,
        registration,
      ) =>
        read(
          await native().registerCollaborationAgent({
            connectionEpoch,
            spaceId,
            projectName,
            runtimeId: '',
            networkType: '',
            importType: '',
            cardUrl: '',
            agentCard: '',
            ...registration,
          }),
        ),
      agentDetail: async (connectionEpoch, spaceId, agentId) =>
        read(
          await native().collaborationAgentDetail({
            connectionEpoch,
            spaceId,
            agentId,
          }),
        ),
      deleteAgent: async (connectionEpoch, spaceId, agentId) =>
        read(
          await native().deleteCollaborationAgent({
            connectionEpoch,
            spaceId,
            agentId,
          }),
        ),
      deleteSpace: async (connectionEpoch, spaceId) =>
        read(
          await native().deleteCollaborationSpace({ connectionEpoch, spaceId }),
        ),
      runtimes: async (connectionEpoch, nextToken) =>
        read(
          await native().collaborationRuntimes({ connectionEpoch, nextToken }),
        ),
      runtimeEndpoints: async (connectionEpoch, runtimeId) =>
        read(
          await native().collaborationRuntimeEndpoints({
            connectionEpoch,
            runtimeId,
          }),
        ),

      binding: async (connectionEpoch) =>
        bindingVM(
          read(await native().collaborationBinding({ connectionEpoch })),
        ),
      spaces: async (connectionEpoch, page) => {
        const result = read(
          await native().collaborationSpaces({ connectionEpoch, page }),
        );
        return {
          ...result,
          pageSize: result.page_size,
          items: result.items.map((item) => ({
            id: item.Id,
            name: item.Name,
            isDefault: item.IsDefault,
            intentEnabled: item.IntentEnabled,
            projectName: item.ProjectName,
          })),
        };
      },
      agents: async (connectionEpoch, spaceId, page) => {
        const result = read(
          await native().collaborationAgents({
            connectionEpoch,
            spaceId,
            page,
          }),
        );
        return {
          ...result,
          pageSize: result.page_size,
          items: result.items.map((item) => ({
            id: item.Id,
            name: item.Name,
            description: item.Description,
            status: item.Status,
            alias: item.Alias,
            aliasRevision: item.AliasRevision,
            aliasAvailable: item.AliasAvailable,
            runtimeId: item.runtimeId,
            card: item.card,
          })),
        };
      },
      setBinding: async (connectionEpoch, spaceId, revision) =>
        bindingVM(
          read(
            await native().setCollaborationBinding({
              connectionEpoch,
              spaceId,
              revision,
            }),
          ),
        ),
      setAlias: async (connectionEpoch, spaceId, agentId, alias, revision) =>
        read(
          await native().setCollaborationAlias({
            connectionEpoch,
            spaceId,
            agentId,
            alias,
            revision,
          }),
        ),
      mention: async (
        connectionEpoch,
        spaceId,
        bindingRevision,
        agentId,
        aliasRevision,
        page,
        task,
      ) =>
        read(
          await native().collaborationMention({
            connectionEpoch,
            spaceId,
            bindingRevision,
            agentId,
            aliasRevision,
            page,
            task,
          }),
        ).text,
    },
  };
}
