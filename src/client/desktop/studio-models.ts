import type { Dispatch, SetStateAction } from 'react';
import type { SafeFailure } from './result';

export interface ConversationTarget {
  connectionEpoch: number;
  scope: string;
  sessionId: string;
}
export interface DraftSnapshot extends ConversationTarget {
  value: string;
  editRevision: number;
  choiceCount: number;
}
export interface ComposerDraft {
  value: string | undefined;
  setValue: Dispatch<SetStateAction<string | undefined>>;
  snapshot(): DraftSnapshot;
  accept(snapshot: DraftSnapshot): void;
}
export interface AgentsMDDocumentVM {
  content: string;
  revision: number;
}
export interface SkillSpaceVM {
  description?: string;
  projectName?: string;
  region?: string;
  id: string;
  name: string;
}
export interface SkillPageVM {
  spaceId: string;
  region: string;
  items: Array<{
    id?: string;
    version?: string;
    status?: string;
    name: string;
    description: string;
  }>;
  totalCount: number;
  pageNumber: number;
  pageSize: number;
  hasMore: boolean;
  connectionEpoch: number;
  platformEpoch: number;
}
export interface MCPDirectoryVM {
  sessionId?: string;
  servers: Array<{
    id: string;
    name: string;
    alias: string;
    description: string;
    tools: Array<{ name: string; description: string }> | null;
    toolsKnown: boolean;
    source?: string;
    address?: string;
  }>;
}
export type MainProfileStatus =
  | 'configured'
  | 'unsupported'
  | 'unconfigured'
  | 'unsupportedOrUnconfigured'
  | 'unavailable';
export interface MainProfileReadVM {
  status: MainProfileStatus;
  connectionEpoch: number;
  snapshotId?: string;
  record?: {
    mpaInstanceId: string;
    revision: number;
    digest: string;
    etag: string;
    profile: {
      agentsMd: string;
      model: { id: string } | null;
      skills: Array<{ name: string; version: string; content: string }>;
    };
    sourceProfileId: string | null;
    sourceProfileDigest: string | null;
    updatedBy: string;
    createdAt: string;
  };
}
export interface MainProfileOutcome {
  status: MainProfileStatus;
  value?: MainProfileReadVM;
  error?: SafeFailure;
}
export interface SkillUploadVM {
  operationId: string;
  spaceId: string;
  name: string;
  skillId: string;
  version: string;
  status: string;
}
export interface ResourcesServices {
  deleteSkill?(
    epoch: number,
    spaceId: string,
    skillId: string,
  ): Promise<unknown>;
  uploadSkill?(
    epoch: number,
    spaceId: string,
    operationId: string,
    filename: string,
    archiveBase64: string,
  ): Promise<SkillUploadVM>;
  readSkillUpload?(epoch: number, operationId: string): Promise<SkillUploadVM>;

  discoverSkillSpaces(epoch: number): Promise<SkillSpaceVM[]>;
  listSkills(
    epoch: number,
    region: string,
    spaceId: string,
    page: number,
    pageSize: number,
  ): Promise<SkillPageVM>;
  listWorkerMCPServers(epoch: number): Promise<MCPDirectoryVM>;
  readAgentsMD(epoch: number): Promise<AgentsMDDocumentVM>;
  saveAgentsMD(
    epoch: number,
    document: AgentsMDDocumentVM,
  ): Promise<AgentsMDDocumentVM>;
  readMainProfile(
    epoch: number,
    mpaInstanceId?: string,
  ): Promise<MainProfileOutcome>;
  saveMainProfileInstructions(
    epoch: number,
    snapshotId: string,
    instructions: string,
  ): Promise<MainProfileOutcome>;
}
export interface CollaborationBindingVM {
  spaceId: string | null;
  spaceName: string | null;
  region: string;
  revision: number;
}
export interface CollaborationSpaceVM {
  id: string;
  name: string;
  isDefault: boolean;
  intentEnabled: boolean;
  projectName: string;
}
export interface CollaborationAgentVM {
  id: string;
  name: string;
  description: string;
  status: string;
  alias: string | null;
  aliasRevision: number;
  aliasAvailable: boolean;
  runtimeId: string;
  card?: {
    name: string;
    description: string;
    version: string;
    protocolVersion: string;
  } | null;
}
export interface RegistryPageVM<T> {
  items: T[];
  total: number;
  page: number;
  pageSize: number;
}
export interface CollaborationServices {
  createSpace?(
    epoch: number,
    name: string,
    projectName: string,
    intentEnabled: boolean,
  ): Promise<{ id: string }>;
  registerAgent?(
    epoch: number,
    spaceId: string,
    projectName: string,
    registration: CollaborationRegistration,
  ): Promise<{ id: string }>;
  agentDetail?(
    epoch: number,
    spaceId: string,
    agentId: string,
  ): Promise<CollaborationDetailVM>;
  deleteAgent?(
    epoch: number,
    spaceId: string,
    agentId: string,
  ): Promise<unknown>;
  deleteSpace?(epoch: number, spaceId: string): Promise<unknown>;
  runtimes?(
    epoch: number,
    nextToken: string,
  ): Promise<{ items: Array<{ id: string; name: string }>; nextToken: string }>;
  runtimeEndpoints?(
    epoch: number,
    runtimeId: string,
  ): Promise<{
    runtimeId: string;
    endpoints: Array<{ networkType: string; url: string }>;
  }>;

  binding(epoch: number): Promise<CollaborationBindingVM>;
  spaces(
    epoch: number,
    page: number,
  ): Promise<RegistryPageVM<CollaborationSpaceVM>>;
  agents(
    epoch: number,
    spaceId: string,
    page: number,
  ): Promise<RegistryPageVM<CollaborationAgentVM>>;
  setBinding(
    epoch: number,
    spaceId: string | null,
    revision: number,
  ): Promise<CollaborationBindingVM>;
  setAlias(
    epoch: number,
    spaceId: string,
    agentId: string,
    alias: string | null,
    revision: number,
  ): Promise<{ alias: string | null; revision: number }>;
  mention(
    epoch: number,
    spaceId: string,
    bindingRevision: number,
    agentId: string,
    aliasRevision: number,
    page: number,
    task: string,
  ): Promise<string>;
}

export interface CollaborationDetailVM {
  id: string;
  name: string;
  status: string;
  source: string;
  defaultVersion: string;
  projectName: string;
  runtimeId: string;
  networkType: string;
  importType: string;
  serviceAddress: string;
  skills: Array<{
    id: string;
    name: string;
    description: string;
    tags: string[];
    examples: string[];
  }> | null;
  skillsTruncated: boolean;
}
export type CollaborationRegistration =
  | { source: 'Runtime'; runtimeId: string; networkType: string }
  | { source: 'Standard'; importType: 'url'; cardUrl: string }
  | { source: 'Standard'; importType: 'json'; agentCard: string };
