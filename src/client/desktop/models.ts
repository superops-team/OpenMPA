import type { RuntimeModelCatalog } from './model-catalog';
import type { Page } from '../../shared/page-types';
import type { Space } from '../../shared/types';
import type { SavePage } from '../editor/autosave';
import type { PageDraft } from '../editor/autosave';
import type { DisplayEvent, DisplayMessage } from './projection';
import type { MessageText } from './locale';
import type { ResourcesServices, CollaborationServices } from './studio-models';
export type { Page, Space };
export interface RuntimeVM {
  id: string;
  name: string;
  status: string;
  version: string;
  image?: string;
}
export interface ConnectionVM {
  epoch: number;
  scope: string;
  runtimeId: string;
  runtimeName?: string;
  workerToolId?: string;
  runtimeImage?: string;
  status: string;
  userId?: string;
}
export interface SessionVM {
  id: string;
  title: string;
  mpaInstanceId?: string;
  traceEnabled?: boolean;
  updatedAt?: number;
  configVersion?: string;
  source?: string;
  canConfigure?: boolean;
  profileVerified?: boolean;
  status?: string;
}
export interface TraceStepVM {
  stepId: string;
  parentStepId?: string;
  sequence: number;
  kind: string;
  title: string;
  status: string;
  startedAt: string;
  endedAt?: string;
  durationMs?: number;
  sourceEventId?: string;
  category?: string;
  timing?: Record<string, unknown>;
  correlation?: Record<string, unknown>;
  details?: Record<string, unknown>;
}
export interface TraceVM {
  traceVersion: string;
  sessionId: string;
  invocationId: string;
  correlation?: Record<string, unknown>;
  run?: Record<string, unknown>;
  coverage?: Record<string, unknown>;
  steps: TraceStepVM[];
}
export interface ExecutionConfigVM {
  executionConfigVersion: string;
  model?: string;
  profileBound: boolean;
  etag?: string;
  modelMode: string;
  mcpMode: string;
  mcpServers: string[];
  invalidRefsCount: number;
}
export interface PendingRunVM {
  sessionId: string;
  operationId: string;
  idempotencyKey: string;
  content: string;
  executionConfigVersion?: string;
  attachmentRefs?: string[];
  state: string;
  invocationId?: string;
  recoveryDisposition?: string;
  terminalVerified?: boolean;
}
export interface AssistantSkillVM {
  id: string;
  name: string;
  description?: string;
  source: string;
  spaceId?: string;
  content?: string;
}
export interface AssistantConfigVM {
  assistantId: string;
  agentsMd: string;
  skills: AssistantSkillVM[];
  updatedAt?: string;
}
export interface PlatformVM {
  signedIn: boolean;
  accountId: string;
  region: string;
  epoch?: number;
}
export interface VaultStatusVM {
  exists: boolean;
  accountId?: string;
  region?: string;
  updatedAt?: string;
}
export interface RegionVM {
  id: string;
  name: string;
  endpointHost: string;
}
export interface AttachmentVM {
  sessionId: string;
  operationRef: string;
  displayName: string;
  digest: string;
  state: string;
}
export interface Stream {
  events: AsyncIterable<DisplayEvent>;
  finished: Promise<void>;
  close(): void;
}
export interface RecoveryDraftVM {
  pageId: string;
  baseRevision: number;
  title: string;
  content: string;
  parentId: string | null;
  capturedAt: number;
}
export interface ScheduleConfigVM {
  type: string;
  timezone?: string;
  runAt?: string;
  intervalSeconds?: number;
  anchorAt?: string;
  time?: string;
  weekdays?: number[];
  monthDays?: number[];
  cronExpression?: string;
}
export interface ScheduleVM {
  id: string;
  name: string;
  prompt: string;
  enabled: boolean;
  version: string;
  agentId: string;
  schedule: ScheduleConfigVM;
  timezone: string;
  nextRun?: string;
  status?: string;
  delivery?: {
    channel: string;
    appId?: string;
    targetId?: string;
    receiveIdType: string;
    bestEffort: boolean;
  };
}
export interface RunVM {
  id: string;
  status: string;
  startedAt?: string;
  error?: string;
  output?: string;
  sessionId?: string;
  invocationId?: string;
}
export interface ChannelVM {
  id: string;
  domain?: string;
  bindingType?: string;
  addressable?: boolean;
  type: string;
  name: string;
  enabled: boolean;
  status: string;
  ownerId?: string;
  profile?: string;
  version?: string;
}
export interface ImagesVM {
  runtimeId: string;
  runtimeVersion: string;
  runtimeImage: string;
  toolId: string;
  toolVersion: string;
  toolImage: string;
}
export interface NativeShellState {
  connectionEpoch: number;
  view: string;
  collapsed: boolean;
  traceOpen?: boolean;
  location: string;
  pageTitle: string;
  selectedSession: string;
  selectedSpace: string;
  selectedAssistant?: string;
  settingsSection?: string;
  theme?: string;
  sessions: Array<{ id: string; title: string; agentId?: string }>;
  spaces: Array<{ id: string; title: string }>;
  assistants?: Array<{ id: string; title: string }>;
  revision: number;
}
export type DesktopSettingsSection =
  | 'Account'
  | 'Runtime'
  | 'Assistant'
  | 'Authentication'
  | 'Deployment'
  | 'Theme'
  | 'Language';
export interface DesktopServices {
  resources?: ResourcesServices;
  collaboration?: CollaborationServices;
  host?: {
    theme?(): Promise<{ theme: string }>;
    onNavigation?(listener: (action: string, id?: string) => void): () => void;
    setShellState?(state: NativeShellState): Promise<void>;
    onClose(listener: (requestId: number) => void): () => void;
    closeState(): Promise<{ pending: boolean; requestId: number }>;
    resolveClose(requestId: number, allow: boolean): Promise<void>;
  };
  platform: {
    status(): Promise<PlatformVM>;
    regions?(): Promise<RegionVM[]>;
    resetRuntime?(): Promise<void>;
    vaultStatus?(): Promise<VaultStatusVM>;
    saveVault?(passphrase: string): Promise<VaultStatusVM>;
    unlockVault?(passphrase: string): Promise<PlatformVM>;
    forgetVault?(): Promise<void>;
    signIn(input: {
      accessKey: string;
      secretKey: string;
      securityToken: string;
      region: string;
    }): Promise<PlatformVM>;
    signOut(): Promise<void>;
    list(
      region: string,
      projectName: string,
      forceRefresh?: boolean,
    ): Promise<RuntimeVM[]>;
    bind(
      region: string,
      projectName: string,
      runtimeId: string,
    ): Promise<ConnectionVM>;
  };
  localization?: {
    status(): Promise<{ mode: string; locale: string }>;
    set(mode: string): Promise<{ mode: string; locale: string }>;
  };
  trace: {
    read(
      epoch: number,
      sessionId: string,
      invocationId: string,
    ): Promise<TraceVM>;
  };
  terminal?: {
    open(epoch: number, sessionId: string, title: string): Promise<void>;
  };
  connection: {
    status(): Promise<ConnectionVM | undefined>;
    test(epoch: number): Promise<string>;
    identity(
      epoch: number,
      userId: string,
      gatewayToken: string,
      applicationToken: string,
      taskToken: string,
      channelToken: string,
      testDeployment: boolean,
    ): Promise<ConnectionVM>;
    channelAuth(
      epoch: number,
      mode: string,
      secret: string,
    ): Promise<ConnectionVM>;
  };
  sessions: {
    pending(epoch: number, sessionId: string): Promise<PendingRunVM[]>;
    recover(
      epoch: number,
      operationId: string,
      mode: string,
    ): Promise<{ invocationId: string; status: string }>;
    resolvePending(
      epoch: number,
      operationId: string,
      mode: string,
    ): Promise<void>;
    list(epoch: number, forceRefresh?: boolean): Promise<SessionVM[]>;
    create(
      epoch: number,
      title: string,
      mpaInstanceId?: string,
    ): Promise<SessionVM>;
    read(
      epoch: number,
      id: string,
    ): Promise<{
      session: SessionVM;
      messages: DisplayMessage[];
      completedInvocations?: string[];
      activeRun?: {
        operationId: string;
        invocationId?: string;
        state: string;
        authority: string;
        recoveryDisposition: string;
      };
    }>;
    rename(epoch: number, id: string, title: string): Promise<void>;
    remove(epoch: number, id: string): Promise<void>;
    submit(
      epoch: number,
      id: string,
      content: string,
      configVersion?: string,
      assistantId?: string,
    ): Promise<{ invocationId: string; status: string }>;
    subscribe(
      epoch: number,
      id: string,
      invocationId: string,
      subscriptionEpoch: number,
      lastEventId?: string,
    ): Stream;
    abort(epoch: number, id: string): Promise<boolean>;
    models?(epoch: number): Promise<RuntimeModelCatalog>;
    executionConfig(epoch: number, id: string): Promise<ExecutionConfigVM>;
    ensureExecutionConfig?(
      epoch: number,
      id: string,
    ): Promise<ExecutionConfigVM>;
    updateExecutionConfig(
      epoch: number,
      id: string,
      input: {
        etag: string;
        modelMode: string;
        model: string;
        mcpMode: string;
        mcpServers: string[];
      },
    ): Promise<ExecutionConfigVM>;
    assistantConfig(
      epoch: number,
      sessionId: string,
      assistantId?: string,
    ): Promise<AssistantConfigVM>;
    saveAssistantConfig(
      epoch: number,
      sessionId: string,
      assistantId: string | undefined,
      input: { agentsMd: string; skills: AssistantSkillVM[] },
    ): Promise<AssistantConfigVM>;
    skillCatalog(epoch: number, query?: string): Promise<AssistantSkillVM[]>;
  };
  workspace: {
    exportDraft(draft: RecoveryDraftVM): Promise<void>;
    abandonDraft(draft: RecoveryDraftVM): Promise<void>;
    state(): Promise<{ readOnly: boolean; recoveryCode: string }>;
    spaces(): Promise<Space[]>;
    createSpace(name: string): Promise<Space>;
    pages(spaceId: string): Promise<Page[]>;
    read(id: string): Promise<Page>;
    create(spaceId: string, parentId: string | null): Promise<Page>;
    save: SavePage;
    preserveDraft(page: Page, draft: PageDraft): Promise<void>;
    recover(
      id: string,
    ): Promise<{ draft: PageDraft; baseRevision: number } | undefined>;
    remove(id: string): Promise<void>;
    link(
      pageId: string,
      connectionScope: string,
      sessionId: string,
    ): Promise<void>;
    linked(
      pageId: string,
      connectionScope: string,
    ): Promise<string | undefined>;
    export(pageId: string, content: string): Promise<void>;
    saveReply(
      page: Page,
      connectionScope: string,
      sessionId: string,
      message: DisplayMessage,
      receiptId: string,
    ): Promise<Page>;
    flush(): Promise<void>;
  };
  schedules?: {
    deliveryTargets(
      epoch: number,
      channel: string,
      appId: string,
    ): Promise<
      Array<{ appId?: string; targetId: string; name: string; channel: string }>
    >;
    list(epoch: number): Promise<ScheduleVM[]>;
    read(
      epoch: number,
      id: string,
    ): Promise<{
      task: ScheduleVM;
      runs: RunVM[];
      count?: number;
      hasMore?: boolean;
      nextOffset?: number;
    }>;
    runs(
      epoch: number,
      id: string,
      offset: number,
    ): Promise<{
      runs: RunVM[];
      count?: number;
      hasMore?: boolean;
      nextOffset?: number;
    }>;
    create(
      epoch: number,
      task: Omit<ScheduleVM, 'id' | 'version'>,
    ): Promise<ScheduleVM>;
    update(epoch: number, task: ScheduleVM): Promise<ScheduleVM>;
    enable(
      epoch: number,
      task: ScheduleVM,
      enabled: boolean,
    ): Promise<ScheduleVM>;
    trigger(epoch: number, task: ScheduleVM): Promise<RunVM>;
    remove(epoch: number, task: ScheduleVM): Promise<void>;
  };
  channels?: {
    list(epoch: number): Promise<ChannelVM[]>;
    enable(epoch: number, channel: ChannelVM, enabled: boolean): Promise<void>;
    templates(epoch: number): Promise<string[]>;
    readProfile(epoch: number, channel: ChannelVM): Promise<ChannelVM>;
    saveProfile(
      epoch: number,
      channel: ChannelVM,
      profile: string,
    ): Promise<void>;
    saveConfig(
      epoch: number,
      channel: ChannelVM,
      secret: string,
    ): Promise<void>;
    remove(epoch: number, channel: ChannelVM): Promise<void>;
    diagnostics(epoch: number, channel: ChannelVM): Promise<string>;
  };
  deployment?: {
    updates(
      runtimeId: string,
    ): Promise<Array<{ operationId: string; status: MessageText }>>;
    images(runtimeId: string): Promise<ImagesVM>;
    runtime(input: {
      runtimeId: string;
      expectedRuntimeVersion: string;
      runtimeImage: string;
    }): Promise<string>;
    worker(input: {
      runtimeId: string;
      toolId: string;
      expectedToolVersion: string;
      toolImage: string;
    }): Promise<string>;
    read(runtimeId: string, operationId: string): Promise<MessageText>;
  };
}
