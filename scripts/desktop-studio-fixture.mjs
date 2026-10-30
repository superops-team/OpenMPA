// Extends the shared desktop fixture without calling a Runtime or cloud API.
export function installStudioFixture() {
  const runtime = globalThis.mygo;
  const baseCall = runtime.call.bind(runtime);
  const baseOn = runtime.on.bind(runtime);
  const listeners = new Map();
  let language = 'en';
  let conflict = false;
  let document = { content: '# Shared fixture instructions', revision: 3 };
  let profile = {
    status: 'configured',
    connectionEpoch: 1,
    snapshotId: 'fixture-snapshot-1',
    record: {
      mpaInstanceId: 'mpa-fixture',
      revision: 5,
      digest: 'fixture-digest',
      etag: '"5"',
      profile: {
        agentsMd: '# Main fixture instructions',
        model: { id: 'design-model' },
        skills: [],
      },
      sourceProfileId: 'profile-fixture',
      sourceProfileDigest: 'source-digest',
      updatedBy: 'fixture',
      createdAt: '2026-10-28T00:00:00Z',
    },
  };
  let binding = {
    space_id: 'as-fixture',
    space_name: 'Fixture team',
    region: 'cn-beijing',
    revision: 2,
  };
  const member = {
    Id: 'a-fixture',
    Name: 'Fixture collaborator',
    Description: 'Reviews fixture requests',
    Status: 'running',
    Alias: 'reviewer',
    AliasRevision: 4,
    AliasAvailable: true,
    runtimeId: 'runtime-peer',
    card: { name: 'Peer', description: 'A local test member' },
  };
  const methods = {
    'Localization.Status': () => ({
      mode: language,
      locale: language,
      dir: 'ltr',
    }),
    'Localization.Set': (request) => {
      language = request.mode;
      runtime.locale = language;
      for (const listener of listeners.get('mygo:locale') ?? [])
        listener({ locale: language, dir: 'ltr' });
      return { mode: language, locale: language, dir: 'ltr' };
    },
    'Usage.BoundSkillSpaces': () => ({
      spaces: [{ id: 'space-fixture', name: 'Mounted fixture skills' }],
    }),
    'Usage.ListWorkerSkills': (request) => ({
      spaceId: request.spaceId,
      region: request.region,
      items: [{ name: 'release-review', description: 'Review a release' }],
      totalCount: 1,
      pageNumber: request.pageNumber,
      pageSize: request.pageSize,
      hasMore: false,
      serverPaginated: false,
      connectionEpoch: 1,
      platformEpoch: 1,
    }),
    'Usage.WorkerMCPServers': () => ({
      servers: [
        {
          id: 'fixture-mcp',
          name: 'Fixture MCP',
          alias: 'fixture',
          description: 'Read-only fixture tools',
          tools: [{ name: 'read_contract', description: 'Read a contract' }],
          toolsKnown: true,
        },
        {
          id: 'unknown-tools',
          name: 'Unknown tool list',
          tools: null,
          toolsKnown: false,
        },
        {
          id: 'empty-tools',
          name: 'Empty tool list',
          tools: [],
          toolsKnown: true,
        },
      ],
    }),
    'Usage.ReadAgentsMD': () => document,
    'Usage.SaveAgentsMD': (request) => {
      if (conflict) throw { code: 'conflict', httpStatus: 409 };
      document = {
        content: request.document.content,
        revision: document.revision + 1,
      };
      return document;
    },
    'Usage.ResourceInstanceID': () => ({ mpaInstanceId: 'mpa-fixture' }),
    'Usage.ReadMainProfile': () => profile,
    'Usage.SaveMainProfileInstructions': (request) => {
      if (conflict) throw { code: 'conflict', httpStatus: 412 };
      const revision = profile.record.revision + 1;
      profile = {
        ...profile,
        snapshotId: `fixture-snapshot-${revision}`,
        record: {
          ...profile.record,
          revision,
          etag: `"${revision}"`,
          profile: {
            ...profile.record.profile,
            agentsMd: request.instructions,
          },
        },
      };
      return profile;
    },
    'Usage.CollaborationSpaces': (request) => ({
      items: [{ Id: 'as-fixture', Name: 'Fixture team' }],
      total: 1,
      page: request.page,
      page_size: 20,
    }),
    'Usage.CollaborationBinding': () => binding,
    'Usage.CollaborationAgents': (request) => ({
      items: [member],
      total: 1,
      page: request.page,
      page_size: 20,
    }),
    'Usage.SetCollaborationBinding': (request) => {
      binding = {
        ...binding,
        space_id: request.spaceId,
        revision: binding.revision + 1,
      };
      return binding;
    },
    'Usage.SetCollaborationAlias': (request) => {
      member.Alias = request.alias;
      member.AliasRevision += 1;
      return { alias: member.Alias, revision: member.AliasRevision };
    },
    'Usage.CollaborationMention': (request) => ({
      text: `@${member.Alias} ${request.task.trim()}`,
    }),
  };
  globalThis.__studioRequests = [];
  runtime.on = (name, listener) => {
    if (!listeners.has(name)) listeners.set(name, new Set());
    listeners.get(name).add(listener);
    const remove = baseOn(name, listener);
    return () => {
      listeners.get(name).delete(listener);
      remove();
    };
  };
  runtime.call = async (method, request) => {
    if (!methods[method]) return baseCall(method, request);
    globalThis.__studioRequests.push({ method, request });
    try {
      const value = methods[method](request);
      return { ok: true, value, ...(value.status && { status: value.status }) };
    } catch (error) {
      return {
        ok: false,
        status: 'unavailable',
        error: {
          ...error,
          plane: 'runtimeIngress',
          retry: 'reload',
          message: 'Fixture resource changed',
        },
      };
    }
  };
  // Navigation is exercised by actual clicks, as the native rail is outside
  // this browser fixture. No evaluate() call invokes a production action.
  addEventListener('DOMContentLoaded', () => {
    const toolbar = globalThis.document.createElement('nav');
    toolbar.setAttribute('aria-label', 'Fixture navigation');
    toolbar.style.cssText =
      'position:fixed;right:8px;bottom:8px;z-index:99999;display:flex;gap:4px;background:white;padding:8px;border:1px solid #aaa';
    for (const [label, action, id] of [
      ['Fixture resources', 'resources', ''],
      ['Fixture collaboration', 'collaboration', ''],
      ['Fixture chat', 'selectSession', 'session-design'],
      ['Fixture language', 'selectSettingsSection', 'Language'],
    ]) {
      const button = globalThis.document.createElement('button');
      button.textContent = label;
      button.onclick = () => globalThis.__designNavigate(action, id);
      toolbar.append(button);
    }
    const toggle = globalThis.document.createElement('button');
    toggle.textContent = 'Fixture conflict off';
    toggle.onclick = () => {
      conflict = !conflict;
      toggle.textContent = `Fixture conflict ${conflict ? 'on' : 'off'}`;
    };
    toolbar.append(toggle);
    globalThis.document.body.append(toolbar);
  });
}
