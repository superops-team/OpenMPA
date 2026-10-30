export function installLifecycleFixture() {
  const runtime = globalThis.mygo;
  const base = runtime.call.bind(runtime);
  const baseOn = runtime.on.bind(runtime);
  const localeListeners = new Set();
  let language = 'en';
  runtime.on = (name, listener) => {
    if (name === 'mygo:locale') localeListeners.add(listener);
    const remove = baseOn(name, listener);
    return () => {
      localeListeners.delete(listener);
      remove();
    };
  };
  const tasks = [];
  globalThis.__lifecycleTasks = tasks;
  globalThis.__holdCreation = false;
  const deletions = [];
  let present = true;
  let activeSessions = false;
  globalThis.__lifecycleRequests = [];
  const methods = {
    'Localization.Status': () => ({
      mode: language,
      locale: language,
      dir: 'ltr',
    }),
    'Localization.Set': (request) => {
      language = request.mode;
      runtime.locale = language;
      for (const listener of localeListeners)
        listener({ locale: language, dir: 'ltr' });
      return { mode: language, locale: language, dir: 'ltr' };
    },
    'Platform.Status': () => ({
      signedIn: true,
      epoch: 7,
      principal: { accountId: 'fixture-account', scope: 'fixture-owner' },
      region: 'cn-beijing',
    }),
    'Platform.ListRuntimes': () => ({
      complete: true,
      runtimes: present
        ? [
            {
              runtimeId: 'runtime-design',
              name: 'Design review assistant',
              status: 'Ready',
              currentVersionNumber: 3,
            },
          ]
        : [],
    }),
    'Platform.ResetRuntime': () => ({}),
    'Connection.Status': () =>
      present
        ? {
            connectionEpoch: 1,
            connected: true,
            scope: 'fixture-owner/cn-beijing/default/runtime-design',
            runtimeId: 'runtime-design',
            runtimeName: 'Design review assistant',
            hasCredential: true,
          }
        : { connectionEpoch: 2, connected: false },
    'MPALifecycle.ListTOSBuckets': ({ platformEpoch, region }) => ({
      platformEpoch,
      region,
      buckets: [
        { name: 'agentkit-platform-fixture-outputs', region },
        { name: 'agentkit-platform-fixture-media', region },
      ],
    }),
    'MPALifecycle.Config': ({ region }) => ({
      configured: true,
      region,
      postgresMode: 'auto',
      openvikingMode: 'auto',
    }),
    'MPALifecycle.ListCreations': () => ({ tasks }),
    'MPALifecycle.ListDeletions': () => ({ tasks: deletions }),
    'MPALifecycle.CheckName': () => ({ available: true }),
    'MPALifecycle.Create': ({ input }) => {
      const value = {
        ...input,
        openvikingApiKey: undefined,
        taskId: crypto.randomUUID(),
        state: 'running',
        stage: 'worker',
        visitedStages: [
          'queued',
          'checking',
          'iam_role',
          'admin_workspace',
          'admin_database',
          'business_workspace',
          'network',
          'gateway',
          'worker',
        ],
        latestDiagnostic: {
          stage: 'worker',
          operation: 'worker_agent_key',
          category: 'metadata_pending',
          attempt: 1,
          outcome: 'retrying',
        },
      };
      tasks.push(value);
      return value;
    },
    'MPALifecycle.ReadCreation': ({ taskId }) => {
      const task = tasks.find((task) => task.taskId === taskId);
      if (task.state === 'running' && !globalThis.__holdCreation)
        Object.assign(task, {
          state: 'succeeded',
          stage: 'verifying',
          visitedStages: [
            ...task.visitedStages,
            'openviking',
            'deploying',
            'verifying',
          ],
          result: {
            runtime_id: 'runtime-created',
            skill_space_id: 'space-created',
            gateway_id: 'gateway',
          },
        });
      return task;
    },
    'MPALifecycle.CancelCreation': ({ taskId }) => {
      const task = tasks.find((task) => task.taskId === taskId);
      task.state = 'cancelled';
      return task;
    },
    'MPALifecycle.PreviewDelete': ({ runtimeId, region }) => ({
      runtimeId,
      region,
      agentId: 'mpa-one',
      name: 'Design review assistant',
      fingerprint: 'a'.repeat(64),
      canDelete: !activeSessions,
      blockers: activeSessions ? ['active_sessions'] : [],
      sessionCounts: {
        total: 2,
        active: activeSessions ? 1 : 0,
        idle: activeSessions ? 1 : 2,
      },
      cleanupPlan: [
        { stage: 'runtime_sessions', count: 2 },
        { stage: 'worker_template', count: 1, resourceId: 't-worker-owned' },
        { stage: 'skill_space', count: 1, resourceId: 'ss-owned' },
        { stage: 'agentkit_runtime', count: 1 },
      ],
      retainedResources: [
        {
          stage: 'skill_space',
          count: 1,
          resourceId: 'ss-shared',
          reason: 'shared',
        },
      ],
    }),
    'MPALifecycle.Delete': (request) => {
      if (
        activeSessions ||
        !request.confirm ||
        request.fingerprint !== 'a'.repeat(64)
      )
        throw Error('Invalid fixture deletion');
      const task = {
        ...request,
        name: 'Design review assistant',
        state: 'running',
        stage: 'managed_resources',
        updatedAt: Date.now() / 1000,
      };
      deletions.push(task);
      return task;
    },
    'MPALifecycle.ReadDeletion': ({ operationId }) => {
      const task = deletions.find((task) => task.operationId === operationId);
      Object.assign(task, { state: 'succeeded', stage: 'complete' });
      present = false;
      return task;
    },
  };
  runtime.call = async (method, request) => {
    if (!methods[method]) return base(method, request);
    globalThis.__lifecycleRequests.push({ method, request });
    if (
      method === 'MPALifecycle.PreviewDelete' &&
      globalThis.__deletePreviewFailure
    )
      return {
        ok: false,
        error: {
          code: 'deletePreviewFailed',
          message: 'Safe deletion check failure',
          httpStatus: 503,
          diagnostic: {
            stage: 'references',
            operation: 'list_runtimes',
            category: 'timeout',
            attempt: 1,
            outcome: 'failed',
            exceptionType: 'TimeoutError',
            httpStatus: 504,
            source: 'threads.py:25:to_thread',
            resourceHost: 'registry.example.invalid',
            requestId: '0123456789abcdef0123456789abcdef',
          },
        },
      };
    return { ok: true, value: methods[method](request) };
  };
  addEventListener('DOMContentLoaded', () => {
    const toolbar = document.createElement('nav');
    toolbar.style.cssText =
      'position:fixed;bottom:0;left:0;z-index:99999;background:white;display:flex;gap:4px';
    for (const [label, action, id] of [
      ['Manage fixture', 'runtimes', ''],
      ['Chat fixture', 'selectSession', 'session-design'],
      ['Language fixture', 'selectSettingsSection', 'Language'],
    ]) {
      const button = document.createElement('button');
      button.textContent = label;
      button.onclick = () => globalThis.__designNavigate(action, id);
      toolbar.append(button);
    }
    const toggle = document.createElement('button');
    toggle.textContent = 'Toggle active sessions';
    toggle.onclick = () => {
      activeSessions = !activeSessions;
    };
    toolbar.append(toggle);
    document.body.append(toolbar);
  });
}
