import { messages } from './generated/locales';
import { localized, LocalizedError } from './locale';
import { Channel } from 'mygo-runtime';
import {
  Artifact,
  Host,
  Platform,
  Connection,
  Session,
  Terminal,
  Trace,
  Localization,
  Workspace,
  events,
} from './generated/mygo';
import { desktopFeatures } from './features';
import type {
  ClientEvent,
  ConnectionState,
  ExecutionConfig,
  SessionSummary,
} from './generated/mygo';
type NativePage = NonNullable<
  Awaited<ReturnType<typeof Workspace.getPage>>['value']
>;
import { unwrap } from './result';
import type { ConnectionVM, DesktopServices, Page, SessionVM } from './models';
import { displayEvent, historyMessages } from './service-events';
import { createStudioServices } from './studio-services';
import { createLifecycleServices } from './lifecycle-services';
export { displayEvent } from './service-events';

const platformVM = (state: {
  provider?: string;
  signedIn: boolean;
  principal?: { accountId?: string; scope?: string } | null;
  region?: string;
  epoch?: number;
}) => ({
  provider: state.provider || 'volcengine',
  signedIn: state.signedIn,
  accountId: state.principal?.accountId || '',
  region: state.region || 'cn-beijing',
  epoch: state.epoch,
  scope: state.principal?.scope,
});
const connectionVM = (state: ConnectionState): ConnectionVM => ({
  epoch: state.connectionEpoch,
  scope: state.scope,
  runtimeId: state.runtimeId,
  runtimeName: state.runtimeName,
  workerToolId: state.workerToolId,
  runtimeImage: state.runtimeImage,
  status: state.connected
    ? state.hasBusinessIdentity
      ? 'Explicit business identity'
      : state.hasCredential
        ? messages.runtimeKeyStatus()
        : messages.authenticationRequired()
    : 'Disconnected',
});
const pageVM = (page: NativePage): Page => ({ ...page, sourceThreadId: null });
const sessionVM = (session: SessionSummary) => ({
  id: session.sessionId || session.id,
  title: session.title || session.sessionId || session.id,
  mpaInstanceId: session.mpaInstanceId,
  traceEnabled: session.traceEnabled,
  updatedAt: session.lastUpdateTime,
  status: session.status,
});
async function digest(title: string, content: string) {
  const bytes = await crypto.subtle.digest(
    'SHA-256',
    new TextEncoder().encode(JSON.stringify([title, content])),
  );
  return Array.from(new Uint8Array(bytes), (byte) =>
    byte.toString(16).padStart(2, '0'),
  ).join('');
}
export function createDesktopServices(): DesktopServices {
  let draftWrites = Promise.resolve();
  let draftFailure: unknown;
  let lastCapture = 0;
  const drafts = new Map<string, number>();
  const workspaceReady = async () => {
    return unwrap(await Workspace.init({}));
  };
  return {
    ...desktopFeatures(),
    ...createStudioServices(),
    lifecycle: createLifecycleServices(),
    host: {
      theme: async () => {
        const value = unwrap(await Host.theme());
        return { theme: value.theme };
      },
      onNavigation: (listener) =>
        events.navigationRequested.on((request) =>
          listener(request.action, request.id),
        ),
      setShellState: async (state) => {
        unwrap(await Host.setShellState(state));
      },
      onClose: (listener) =>
        events.closeRequested.on((request) => listener(request.requestId)),
      closeState: async () => unwrap(await Host.closeState()),
      resolveClose: async (requestId, allow) => {
        unwrap(await Host.resolveClose({ requestId, allow }));
      },
    },
    platform: {
      status: async () => {
        return platformVM(unwrap(await Platform.status()));
      },
      regions: async () =>
        unwrap(await Platform.regions()).regions.map((region) => ({
          provider: region.provider || 'volcengine',
          id: region.region,
          name: region.displayName,
          endpointHost: region.endpointHost,
        })),
      resetRuntime: async () => {
        unwrap(await Platform.resetRuntime());
      },
      vaultStatus: async () => unwrap(await Platform.vaultStatus()),
      saveVault: async (passphrase) =>
        unwrap(await Platform.saveVault({ passphrase })),
      unlockVault: async (passphrase) =>
        platformVM(unwrap(await Platform.unlockVault({ passphrase }))),
      forgetVault: async () => {
        unwrap(await Platform.forgetVault());
      },
      signIn: async (input) => {
        const state = unwrap(await Platform.signIn(input));
        return platformVM(state);
      },
      signOut: async () => {
        unwrap(await Platform.signOut());
      },
      list: async (region, projectName, forceRefresh = false) => {
        const result = unwrap(
          await Platform.listRuntimes({ region, projectName, forceRefresh }),
        );
        if (!result.complete)
          throw new LocalizedError(
            localized((m) => m.runtimeDirectoryIncomplete()),
          );
        return result.runtimes.map((runtime) => ({
          id: runtime.runtimeId,
          name: runtime.name,
          status: runtime.status,
          version: String(runtime.currentVersionNumber),
        }));
      },
      bind: async (region, projectName, runtimeId) =>
        connectionVM(
          unwrap(
            await Platform.bindRuntime({ region, projectName, runtimeId }),
          ),
        ),
    },
    connection: {
      status: async () => {
        const state = unwrap(await Connection.status());
        return state.connected ? connectionVM(state) : undefined;
      },
      test: async (connectionEpoch) => {
        const result = unwrap(await Connection.test({ connectionEpoch }));
        return result.capabilities
          .map(
            (item) =>
              `${item.name}: ${item.state}${item.error ? ` (${item.error.message})` : ''}`,
          )
          .join(' · ');
      },
      channelAuth: async (connectionEpoch, authMode, secret) =>
        connectionVM(
          unwrap(
            await Connection.configureChannelAuth({
              connectionEpoch,
              authMode,
              channelKey: authMode === 'runtimeKey' ? secret : undefined,
              legacyJwt: authMode === 'legacyJwt' ? secret : undefined,
            }),
          ),
        ),
      identity: async (
        connectionEpoch,
        userId,
        gatewayToken,
        applicationToken,
        taskToken,
        channelToken,
        testDeployment,
      ) => {
        if (gatewayToken || taskToken || channelToken)
          throw new LocalizedError(
            localized((m) => m.connectionSlotsUnsupported()),
          );
        return connectionVM(
          unwrap(
            await Connection.setBusinessIdentity({
              connectionEpoch,
              userId,
              authMode: testDeployment ? 'explicitTest' : 'legacyJwt',
              legacyJwt: testDeployment ? undefined : applicationToken,
              testSubject: testDeployment ? userId : undefined,
              verifiedTestDeployment: testDeployment,
            }),
          ),
        );
      },
    },
    localization: {
      status: async () => unwrap(await Localization.status()),
      set: async (mode) => unwrap(await Localization.set({ mode })),
    },
    trace: {
      read: async (connectionEpoch, sessionId, invocationId, forceRefresh) =>
        unwrap(
          await Trace.read({
            connectionEpoch,
            sessionId,
            invocationId,
            forceRefresh,
          }),
        ),
      prefetch: async (connectionEpoch, sessionId, invocationId) => {
        unwrap(
          await Trace.prefetch({ connectionEpoch, sessionId, invocationId }),
        );
      },
    },
    terminal: {
      open: async (connectionEpoch, sessionId, title) => {
        unwrap(await Terminal.open({ connectionEpoch, sessionId, title }));
      },
    },
    sessions: {
      models: async (connectionEpoch) => {
        const catalog = unwrap(await Session.models({ connectionEpoch }));
        return {
          models: catalog.models.map((id) => ({ id })),
          defaultModel: catalog.defaultModel,
          supported: catalog.supported,
        };
      },
      recover: async (connectionEpoch, operationId, mode) => {
        const result = unwrap(
          await Session.recover({ connectionEpoch, operationId, mode }),
        );
        return { invocationId: result.invocationId, status: 'accepted' };
      },
      resolvePending: async (connectionEpoch, operationId, mode) => {
        unwrap(
          await Session.resolvePending({
            connectionEpoch,
            operationId,
            mode,
            confirm: true,
          }),
        );
      },
      pending: async (connectionEpoch, sessionId) =>
        unwrap(await Session.pending({ connectionEpoch })).operations.filter(
          (operation) => operation.sessionId === sessionId,
        ),
      list: async (connectionEpoch, forceRefresh = false) => {
        const list: SessionVM[] = unwrap(
          await Session.list({ connectionEpoch, forceRefresh }),
        ).sessions.map(sessionVM);
        return list;
      },
      create: async (connectionEpoch, title, mpaInstanceId, sessionId) => {
        const session = sessionVM(
          unwrap(
            await Session.create({ connectionEpoch, mpaInstanceId, sessionId }),
          ),
        );
        if (title)
          unwrap(
            await Session.rename({
              connectionEpoch,
              sessionId: session.id,
              title,
            }),
          );
        return { ...session, title };
      },
      read: async (connectionEpoch, sessionId) => {
        const result = unwrap(
          await Session.read({ connectionEpoch, sessionId }),
        );
        let configVersion: string | undefined;
        try {
          configVersion = unwrap(
            await Session.executionConfig({ connectionEpoch, sessionId }),
          ).executionConfigVersion;
        } catch {
          /* History remains readable when execution config is not supported. */
        }
        return {
          session: { ...sessionVM(result.summary), configVersion },
          messages: historyMessages(result.events),
          completedInvocations: result.events
            .filter((event) => event.terminal && !event.error)
            .map((event) => event.invocationId),
          activeRun: result.activeRun,
        };
      },
      rename: async (connectionEpoch, sessionId, title) => {
        unwrap(await Session.rename({ connectionEpoch, sessionId, title }));
      },
      remove: async (connectionEpoch, sessionId) => {
        unwrap(await Session.delete({ connectionEpoch, sessionId }));
      },
      submit: async (
        connectionEpoch,
        sessionId,
        content,
        executionConfigVersion,
        assistantId,
      ) => {
        const pending = unwrap(
          await Session.pending({ connectionEpoch }),
        ).operations.find((operation) => operation.sessionId === sessionId);
        if (pending)
          throw new LocalizedError(
            localized((m) =>
              m.originalSubmissionNeedsReview(pending.operationId),
            ),
          );
        const operationId = crypto.randomUUID();
        const result = unwrap(
          await Session.submit({
            connectionEpoch,
            sessionId,
            operationId,
            idempotencyKey: operationId,
            content,
            executionConfigVersion,
            assistantId,
          }),
        );
        return { invocationId: result.invocationId, status: 'accepted' };
      },
      subscribe: (
        connectionEpoch,
        sessionId,
        invocationId,
        subscriptionEpoch,
        lastEventId,
      ) => {
        const channel = new Channel<ClientEvent>();
        const finished = Session.subscribe(
          {
            connectionEpoch,
            sessionId,
            invocationId,
            subscriptionEpoch,
            lastEventId,
          },
          channel,
        ).then((result) => {
          unwrap(result);
        });
        return {
          events: (async function* () {
            for await (const event of channel) yield displayEvent(event);
          })(),
          finished,
          close: () => channel.close(),
        };
      },
      abort: async (connectionEpoch, sessionId) => {
        return unwrap(await Session.abort({ connectionEpoch, sessionId }))
          .aborted;
      },
      executionConfig: async (connectionEpoch, sessionId) =>
        unwrap(await Session.executionConfig({ connectionEpoch, sessionId })),
      ensureExecutionConfig: async (connectionEpoch, sessionId) =>
        unwrap(
          await Session.ensureExecutionConfig({ connectionEpoch, sessionId }),
        ),
      modelCatalog: async (connectionEpoch) =>
        unwrap(await Session.models({ connectionEpoch })).models.map((id) => ({
          id,
        })),
      updateExecutionConfig: async (
        connectionEpoch,
        sessionId,
        input,
      ): Promise<ExecutionConfig> =>
        unwrap(
          await Session.updateExecutionConfig({
            connectionEpoch,
            sessionId,
            etag: input.etag,
            modelMode: input.modelMode,
            model: input.model,
            mcpMode: input.mcpMode,
            mcpServers: input.mcpServers,
          }),
        ),
      assistantConfig: async (connectionEpoch, sessionId, assistantId) =>
        unwrap(
          await Session.assistantConfig({
            connectionEpoch,
            sessionId,
            assistantId,
          }),
        ),
      saveAssistantConfig: async (
        connectionEpoch,
        sessionId,
        assistantId,
        input,
      ) =>
        unwrap(
          await Session.saveAssistantConfig({
            connectionEpoch,
            sessionId,
            assistantId,
            agentsMd: input.agentsMd,
            skills: input.skills,
          }),
        ),
      skillCatalog: async (connectionEpoch, query = '') =>
        unwrap(await Session.skillCatalog({ connectionEpoch, query })).skills,
    },
    workspace: {
      exportDraft: async (draft) => {
        unwrap(
          await Artifact.exportMarkdown({
            title: draft.title,
            content: draft.content,
          }),
        );
      },
      abandonDraft: async (draft) => {
        unwrap(
          await Workspace.abandonUnsavedDraft({
            pageId: draft.pageId,
            capturedAt: draft.capturedAt,
            draftDigest: await digest(draft.title, draft.content),
            confirmAbandon: true,
          }),
        );
        draftFailure = undefined;
      },
      state: async () => {
        const state = await workspaceReady();
        return { readOnly: state.readOnly, recoveryCode: state.recoveryCode };
      },
      spaces: async () => {
        await workspaceReady();
        return unwrap(await Workspace.listSpaces({})).spaces.map((space) => ({
          ...space,
          description: '',
        }));
      },
      createSpace: async (name) => ({
        ...unwrap(await Workspace.createSpace({ name })),
        description: '',
      }),
      pages: async (spaceId) =>
        unwrap(await Workspace.listPages({ spaceId })).pages.map(pageVM),
      read: async (id) => pageVM(unwrap(await Workspace.getPage({ id }))),
      create: async (spaceId, parentId) =>
        pageVM(
          unwrap(
            await Workspace.createPage({
              spaceId,
              parentId,
              title: messages.desktopUntitledPage(),
              content: '',
            }),
          ),
        ),
      save: async (id, patch, signal) => {
        if (signal.aborted)
          throw new LocalizedError(localized((m) => m.saveCancelled()));
        const capturedAt = drafts.get(id);
        await draftWrites;
        const result = pageVM(
          unwrap(await Workspace.patchPage({ id, ...patch, setParent: true })),
        );
        if (capturedAt !== undefined && drafts.get(id) === capturedAt) {
          unwrap(await Workspace.deleteDraft({ pageId: id, capturedAt }));
          drafts.delete(id);
        }
        return result;
      },
      preserveDraft: async (page, draft) => {
        const capturedAt = Math.max(Date.now(), lastCapture + 1);
        lastCapture = capturedAt;
        drafts.set(page.id, capturedAt);
        const write = draftWrites.then(async () => {
          unwrap(
            await Workspace.saveDraft({
              pageId: page.id,
              baseRevision: page.revision,
              ...draft,
              capturedAt,
            }),
          );
        });
        draftWrites = write.then(
          () => {
            draftFailure = undefined;
          },
          (error: unknown) => {
            draftFailure = error;
          },
        );
        await write;
      },
      recover: async (pageId) => {
        const list = unwrap(
          await Workspace.listRecoverableDrafts({ pageId }),
        ).drafts;
        const draft = list.sort((a, b) => b.capturedAt - a.capturedAt)[0];
        return draft
          ? {
              baseRevision: draft.baseRevision,
              draft: {
                title: draft.title,
                content: draft.content,
                parentId: draft.parentId,
              },
            }
          : undefined;
      },
      remove: async (id) => {
        const page = unwrap(await Workspace.getPage({ id }));
        unwrap(
          await Workspace.deletePage({ id, expectedRevision: page.revision }),
        );
      },
      linked: async (pageId, connectionScope) =>
        unwrap(await Workspace.listConversationLinks({ pageId })).links.find(
          (link) =>
            link.deletedAt == null &&
            link.botRef.connectionScope === connectionScope,
        )?.botRef.sessionId,
      link: async (pageId, connectionScope, sessionId) => {
        unwrap(
          await Workspace.linkConversation({
            pageId,
            botRef: { connectionScope, sessionId },
          }),
        );
      },
      export: async (id, content) => {
        const page = unwrap(await Workspace.getPage({ id }));
        unwrap(await Artifact.exportMarkdown({ title: page.title, content }));
      },
      saveReply: async (
        page,
        connectionScope,
        sessionId,
        message,
        receiptId,
      ) => {
        const content = `${page.content}${page.content ? '\n\n' : ''}${message.content}`;
        const result = unwrap(
          await Workspace.saveReviewedResult({
            receiptId,
            sourceRef: {
              connectionScope,
              sessionId,
              invocationId: message.invocationId,
              eventRef: message.id,
            },
            draftDigest: await digest(page.title, content),
            spaceId: page.spaceId,
            pageId: page.id,
            parentId: page.parentId,
            expectedRevision: page.revision,
            title: page.title,
            content,
          }),
        );
        if (!result.page)
          throw new LocalizedError(localized((m) => m.savedPageDeleted()));
        return pageVM(result.page);
      },
      flush: async () => {
        await draftWrites;
        const result = unwrap(await Workspace.flush({}));
        if (result.closeState?.safeClose === false)
          throw Object.assign(
            new LocalizedError(localized((m) => m.unsavedDraftsPreventClose())),
            { closeState: result.closeState },
          );
        if (draftFailure && result.closeState?.safeClose !== true)
          throw draftFailure;
      },
    },
  };
}
