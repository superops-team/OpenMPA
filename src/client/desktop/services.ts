import { Channel } from 'mygo-runtime';
import {
  Artifact,
  Host,
  Platform,
  Connection,
  Session,
  Workspace,
  events,
} from './generated/mygo';
import { desktopFeatures } from './features';
import type {
  ClientEvent,
  ConnectionState,
  SessionSummary,
} from './generated/mygo';
type NativePage = NonNullable<
  Awaited<ReturnType<typeof Workspace.getPage>>['value']
>;
import { unwrap } from './result';
import {
  emptyProjection,
  projectEvent,
  type DisplayEvent,
  type DisplayMessage,
} from './projection';
import type { ConnectionVM, DesktopServices, Page, SessionVM } from './models';

const connectionVM = (state: ConnectionState): ConnectionVM => ({
  epoch: state.connectionEpoch,
  scope: state.scope,
  runtimeId: state.runtimeId,
  status: state.connected
    ? state.hasBusinessIdentity
      ? 'Explicit business identity'
      : state.hasCredential
        ? 'Runtime key identity'
        : 'Authentication required'
    : 'Disconnected',
});
const pageVM = (page: NativePage): Page => ({ ...page, sourceThreadId: null });
const sessionVM = (session: SessionSummary) => ({
  id: session.sessionId || session.id,
  title: session.title || session.sessionId || session.id,
  updatedAt: session.lastUpdateTime,
  status: session.status,
});
export function displayEvent(event: ClientEvent): DisplayEvent {
  return {
    connectionEpoch: event.connectionEpoch,
    subscriptionEpoch: event.subscriptionEpoch,
    sessionId: event.sessionId,
    invocationId: event.invocationId,
    eventId: event.eventId,
    blockId: event.blockId,
    kind: event.error
      ? 'error'
      : event.terminal && !event.text
        ? 'done'
        : event.kind,
    text:
      event.error?.message ||
      event.text ||
      (event.toolName
        ? `${event.kind}: ${event.toolName}${event.toolId ? ` (${event.toolId})` : ''}`
        : ''),
    role: event.author === 'user' ? 'user' : 'assistant',
    partial: event.partial,
    terminal: event.terminal,
  };
}
function historyMessages(events: ClientEvent[]): DisplayMessage[] {
  if (!events.length) return [];
  const first = events[0];
  let projection = emptyProjection({
    connectionEpoch: first.connectionEpoch,
    subscriptionEpoch: first.subscriptionEpoch,
    sessionId: first.sessionId,
  });
  for (const event of events)
    projection = projectEvent(projection, displayEvent(event));
  return projection.messages;
}
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
    host: {
      requestSessionAction: async (action) => {
        unwrap(await Host.requestSessionAction(action));
      },
      requestConversation: async () => {
        unwrap(await Host.requestConversation());
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
    capabilities: {
      read: async (connectionEpoch, sessionId) => {
        const values = unwrap(
          await Connection.test({ connectionEpoch }),
        ).capabilities.map((capability) => ({
          name: capability.name,
          value: capability.state,
        }));
        if (sessionId) {
          const config = unwrap(
            await Session.executionConfig({ connectionEpoch, sessionId }),
          );
          values.push(
            {
              name: 'Execution configuration version',
              value: config.executionConfigVersion,
            },
            { name: 'Model', value: config.model || 'Not exposed' },
            {
              name: 'Bot profile',
              value: config.profileBound ? 'Bound' : 'Not bound',
            },
          );
        }
        return values;
      },
    },
    platform: {
      status: async () => {
        const state = unwrap(await Platform.status());
        return {
          signedIn: state.signedIn,
          accountId: state.principal?.accountId || '',
          region: state.region || 'cn-beijing',
        };
      },
      regions: async () =>
        unwrap(await Platform.regions()).regions.map((region) => ({
          id: region.region,
          name: region.displayName,
          endpointHost: region.endpointHost,
        })),
      resetRuntime: async () => {
        unwrap(await Platform.resetRuntime());
      },
      signIn: async (input) => {
        const state = unwrap(await Platform.signIn(input));
        return {
          signedIn: state.signedIn,
          accountId: state.principal?.accountId || '',
          region: input.region,
        };
      },
      signOut: async () => {
        unwrap(await Platform.signOut());
      },
      list: async (region, projectName) => {
        const result = unwrap(
          await Platform.listRuntimes({ region, projectName }),
        );
        if (!result.complete)
          throw new Error(
            'The Runtime directory is incomplete. Refresh before choosing a Runtime.',
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
          throw new Error(
            'Separate credential slots are not supported by this connection. Runtime gateway credentials are held by the host.',
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
    sessions: {
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
      list: async (connectionEpoch) => {
        const list: SessionVM[] = unwrap(
          await Session.list({ connectionEpoch }),
        ).sessions.map(sessionVM);
        return list;
      },
      create: async (connectionEpoch, title) => {
        const session = sessionVM(
          unwrap(await Session.create({ connectionEpoch })),
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
      ) => {
        const pending = unwrap(
          await Session.pending({ connectionEpoch }),
        ).operations.find((operation) => operation.sessionId === sessionId);
        if (pending)
          throw new Error(
            `Original submission ${pending.operationId} requires verification. Reopen and review its saved request before starting another run.`,
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
              title: 'Untitled page',
              content: '',
            }),
          ),
        ),
      save: async (id, patch, signal) => {
        if (signal.aborted)
          throw new Error('Save cancelled before submission.');
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
          throw new Error(
            'The saved page was deleted. The review receipt remains available.',
          );
        return pageVM(result.page);
      },
      flush: async () => {
        await draftWrites;
        const result = unwrap(await Workspace.flush({}));
        if (result.closeState?.safeClose === false)
          throw Object.assign(
            new Error('Unsaved recovery drafts prevent closing.'),
            { closeState: result.closeState },
          );
        if (draftFailure && result.closeState?.safeClose !== true)
          throw draftFailure;
      },
    },
  };
}
