export function installDesktopFixture() {
  const listeners = new Map();
  const summary = {
    id: 'session-design',
    sessionId: 'session-design',
    title: 'Review desktop consistency',
    status: 'idle',
    traceEnabled: true,
  };
  const connection = {
    connectionEpoch: 1,
    connected: true,
    scope: 'design-fixture',
    runtimeId: 'runtime-design',
    runtimeName: 'Design review assistant',
    hasCredential: true,
  };
  const event = (id, kind, text, author = 'assistant', extra = {}) => ({
    schemaVersion: 2,
    connectionEpoch: 1,
    subscriptionEpoch: 0,
    sessionId: summary.id,
    invocationId: 'turn-design',
    eventId: id,
    blockId: id,
    kind,
    text,
    author,
    ...extra,
  });
  const trace = {
    traceVersion: '1.2',
    sessionId: summary.id,
    invocationId: 'turn-design',
    run: {
      status: 'success',
      startedAt: '2026-10-27T10:00:00Z',
      wallDurationMs: 3200,
      modelRequestCount: 2,
      toolCount: 1,
      usage: { totalTokens: 1240 },
    },
    coverage: {
      complete: true,
      active: false,
      recordCount: 4,
      returnedCount: 4,
    },
    steps: [
      {
        stepId: 'turn',
        sequence: 1,
        kind: 'run.total',
        title: 'Review desktop consistency',
        status: 'success',
        startedAt: '2026-10-27T10:00:00Z',
        durationMs: 3200,
      },
      {
        stepId: 'model-1',
        parentStepId: 'turn',
        sequence: 2,
        kind: 'model.request',
        title: 'Read the request',
        status: 'success',
        startedAt: '2026-10-27T10:00:00Z',
        durationMs: 700,
      },
      {
        stepId: 'read-file',
        parentStepId: 'turn',
        sequence: 3,
        kind: 'tool.call',
        title: 'Read DESIGN.md',
        status: 'success',
        startedAt: '2026-10-27T10:00:00.700Z',
        durationMs: 500,
        correlation: { toolId: 'read-file' },
        details: {
          path: 'DESIGN.md',
          summary: 'Read the project design contract.',
        },
      },
      {
        stepId: 'model-2',
        parentStepId: 'turn',
        sequence: 4,
        kind: 'model.request',
        title: 'Write the review',
        status: 'success',
        startedAt: '2026-10-27T10:00:01.200Z',
        durationMs: 2000,
      },
    ],
  };
  const methods = {
    'Host.Theme': () => ({ theme: 'light' }),
    'Host.CloseState': () => ({ pending: false, requestId: 0 }),
    'Host.SetShellState': (state) => {
      globalThis.__designShell = state;
      return {};
    },
    'Platform.Status': () => ({
      signedIn: true,
      principal: { accountId: 'design-account' },
      region: 'cn-beijing',
    }),
    'Platform.Regions': () => ({
      regions: [
        {
          region: 'cn-beijing',
          displayName: 'Beijing',
          endpointHost: 'example.invalid',
        },
        {
          region: 'cn-shanghai',
          displayName: 'Shanghai',
          endpointHost: 'example.invalid',
        },
      ],
    }),
    'Platform.ListRuntimes': () => ({
      complete: true,
      runtimes: [
        {
          runtimeId: connection.runtimeId,
          name: connection.runtimeName,
          status: 'Ready',
          currentVersionNumber: 1,
        },
        {
          runtimeId: 'runtime-secondary',
          name: 'Secondary design assistant',
          status: 'Ready',
          currentVersionNumber: 3,
        },
      ],
    }),
    'Platform.BindRuntime': (request) => {
      connection.connectionEpoch += 1;
      connection.runtimeId = request.runtimeId;
      connection.runtimeName =
        request.runtimeId === 'runtime-secondary'
          ? 'Secondary design assistant'
          : 'Design review assistant';
      return connection;
    },
    'Platform.VaultStatus': () => ({
      exists: true,
      accountId: 'design-account',
      region: 'cn-beijing',
    }),
    'Connection.Status': () => connection,
    'Localization.Status': () => ({ mode: 'en', locale: 'en', dir: 'ltr' }),
    'Session.List': () => ({ sessions: [summary] }),
    'Session.Pending': () => ({ operations: [] }),
    'Session.Read': () => ({
      summary,
      events: [
        event(
          'user',
          'text',
          'Review the design system and its runtime behavior.',
          'user',
        ),
        event('call', 'toolCall', '', 'assistant', {
          toolId: 'read-file',
          toolName: 'read',
          toolStatus: 'success',
          toolSubject: 'DESIGN.md',
        }),
        event(
          'answer',
          'text',
          '## Review complete\n\nThe design contract is now shared across the workspace.\n\n| Surface | Result |\n| --- | --- |\n| Settings | Reviewed |\n| Trace | Reviewed |\n\n```ts\nconst theme = "light";\n```',
        ),
        event('done', 'done', '', 'assistant', { terminal: true }),
      ],
    }),
    'Session.ExecutionConfig': () => ({
      executionConfigVersion: '1',
      etag: '"1"',
      model: 'design-model',
      modelMode: 'inherit',
      mcpMode: 'inherit',
      mcpServers: [],
      profileBound: true,
      invalidRefsCount: 0,
      catalog: { models: [{ id: 'design-model', name: 'Design model' }] },
    }),
    'Session.AssistantConfig': () => ({
      assistantId: connection.runtimeId,
      agentsMd: '# Assistant\nFollow the project design contract.',
      skills: [],
    }),
    'Session.SkillCatalog': () => ({
      skills: [
        {
          id: 'meegle',
          name: 'meegle',
          description:
            '飞书项目操作：查询和管理工作项、节点流转、排期和个人待办。',
          source: 'skill-space',
          spaceId: 'agentkit',
        },
        {
          id: 'imagegen',
          name: 'imagegen',
          description:
            'AI 图像生成编辑，支持生成图片、编辑现有图像、生成视觉变体。',
          source: 'skill-space',
          spaceId: 'agentkit',
        },
        {
          id: 'openai-docs',
          name: 'openai-docs',
          description:
            'OpenAI Codex 官方文档查询，支持模型、API、技能、配置和常见问题。',
          source: 'skill-space',
          spaceId: 'agentkit',
        },
        {
          id: 'plugin-creator',
          name: 'plugin-creator',
          description:
            'Create and verify reusable plugins with manifests, resources and validation guidance.',
          source: 'skill-space',
          spaceId: 'agentkit',
        },
        {
          id: 'skill-creator',
          name: 'skill-creator',
          description:
            'Create Skill 开发指南，支持创建完整 Skill 指令及配套资源。',
          source: 'skill-space',
          spaceId: 'agentkit',
        },
        {
          id: 'skill-installer',
          name: 'skill-installer',
          description:
            '安装 Skill 到共享技能空间，并检查 Skill 内容与运行时兼容性。',
          source: 'skill-space',
          spaceId: 'agentkit',
        },
        {
          id: 'browser-automation',
          name: 'browser-automation',
          description:
            'Drive browser workflows and preserve screenshots, console errors and layout evidence.',
          source: 'skill-space',
          spaceId: 'agentkit',
        },
        {
          id: 'release-review',
          name: 'release-review',
          description:
            'Review a release against current requirements, tests and representative runtime evidence.',
          source: 'skill-space',
          spaceId: 'agentkit',
        },
      ],
    }),
    'Channels.List': () => ({ channels: [], managementScope: 'runtime' }),
    'Channels.Capabilities': () => ({
      version: '1',
      channels: ['feishu'],
      credentialBindingChannels: ['feishu'],
      multiBotChannels: ['feishu'],
      groupPermissionChannels: ['feishu'],
      feishuDomains: ['feishu', 'lark'],
      botBindingTypes: ['shared', 'private'],
      serverSideBinding: true,
      groupPermissions: true,
      diagnostics: true,
      botProfiles: true,
      privateBotUsers: true,
      accountScopedPermissions: true,
      authMode: 'runtimeKey',
      managementScope: 'runtime',
      ownerActionsRequireIdentity: false,
      ownerIdentityAvailable: true,
      upgradeHasCas: true,
      upgradeAdvancesExecutionVersion: true,
      configHasCas: true,
    }),
    'Schedule.List': () => ({
      items: [
        {
          id: 'schedule-design',
          name: 'Morning review',
          prompt: 'Summarize updates.',
          agentId: connection.runtimeId,
          enabled: true,
          version: 1,
          schedule: {
            type: 'Cron',
            cronExpression: '0 9 * * *',
            timezone: 'Asia/Shanghai',
          },
        },
      ],
      hasMore: false,
    }),
    'Schedule.DeliveryTargets': () => ({ items: [] }),
    'Knowledge.List': () => ({
      items: [
        {
          knowledgeId: 'knowledge-design',
          displayName: 'Design contract',
          version: 1,
          status: 'READY',
          sourceType: 'text',
          chunkCount: 4,
        },
      ],
    }),
    'Knowledge.Read': () => ({
      knowledgeId: 'knowledge-design',
      displayName: 'Design contract',
      version: 1,
      status: 'READY',
      sourceType: 'text',
      chunkCount: 4,
    }),
    'Deployment.ListUpdates': () => ({ operations: [] }),
    'RuntimeConfig.Read': () => ({
      runtimeId: connection.runtimeId,
      version: '1',
      values: {
        description: 'Design review runtime',
        cpu: 2,
        memoryMb: 4096,
        minInstances: 1,
        maxInstances: 2,
        maxConcurrency: 4,
      },
    }),
    'Trace.Read': () => trace,
  };
  globalThis.__designCalls = [];
  globalThis.__designMissing = [];
  globalThis.__designNavigate = (action, id = '') => {
    for (const listener of listeners.get('NavigationRequested') ?? [])
      listener({ action, id });
  };
  globalThis.mygo = {
    locale: 'en',
    dir: 'ltr',
    preferences: { reduceMotion: true, textScale: 1 },
    on(name, listener) {
      if (!listeners.has(name)) listeners.set(name, new Set());
      listeners.get(name).add(listener);
      return () => listeners.get(name).delete(listener);
    },
    async call(method, arg) {
      globalThis.__designCalls.push(method);
      if (!methods[method]) {
        globalThis.__designMissing.push(method);
        return {
          ok: false,
          error: {
            code: 'fixtureUnimplemented',
            message: `Fixture has no response for ${method}`,
          },
        };
      }
      return { ok: true, value: methods[method](arg) };
    },
  };
}
