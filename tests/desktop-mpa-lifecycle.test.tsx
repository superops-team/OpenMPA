import { act, create, type ReactTestRenderer } from 'react-test-renderer';
import { beforeEach, afterEach, it, expect, vi } from 'vitest';
import { MpaCreateDialog } from '../src/client/desktop/MpaCreateDialog';
import { MpaDeleteDialog } from '../src/client/desktop/MpaDeleteDialog';
import { RuntimeManager } from '../src/client/desktop/RuntimeManager';
import {
  persistCreationDraft,
  restoreCreationDraft,
} from '../src/client/desktop/lifecycle-state';
import { createLifecycleServices } from '../src/client/desktop/lifecycle-services';
import { messagesIn } from '../src/client/desktop/generated/locales';
import type { MPALifecycle } from '../src/client/desktop/generated/mygo';
import type { DesktopServices } from '../src/client/desktop/models';
import type {
  MpaLifecycleServices,
  MpaCreationInput,
  MpaCreationTask,
  MpaDeletePreview,
} from '../src/client/desktop/lifecycle-models';

let renderer: ReactTestRenderer | undefined;
const m = messagesIn('en');
const requestId = '1ea8b724-893f-4731-a5fc-000000000001';
const input: MpaCreationInput = {
  requestId,
  region: 'cn-beijing',
  name: 'my-mpa',
  description: 'My MPA',
  maxInstance: 20,
  businessWorkspaceMode: 'per-agent',
  businessDatabaseName: 'aidb',
  openvikingMode: 'auto',
};
const task: MpaCreationTask = {
  ...input,
  taskId: '1ea8b724-893f-4731-a5fc-000000000002',
  state: 'running',
  stage: 'worker',
};
const preview: MpaDeletePreview = {
  runtimeId: 'r-one',
  region: 'cn-beijing',
  agentId: 'a-one',
  name: 'One',
  fingerprint: 'a'.repeat(64),
  canDelete: true,
  blockers: [],
  sessionCounts: { total: 1, active: 0, idle: 1 },
  cleanupPlan: [{ stage: 'runtime_sessions', count: 1 }],
  retainedResources: [],
};
const deletion = {
  operationId: requestId,
  runtimeId: 'r-one',
  region: 'cn-beijing',
  name: 'One',
  state: 'running',
  stage: 'checking',
  updatedAt: 0,
};
beforeEach(() => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  vi.stubGlobal('mygo', { locale: 'en' });
  const store = new Map<string, string>();
  vi.stubGlobal('sessionStorage', {
    getItem: (key: string) => store.get(key),
    setItem: (key: string, value: string) => store.set(key, value),
  });
});
afterEach(async () => {
  if (renderer) await act(async () => renderer!.unmount());
  renderer = undefined;
  vi.unstubAllGlobals();
  vi.useRealTimers();
});
const button = (text: string) =>
  renderer!.root
    .findAllByType('button')
    .find((node) => node.children.includes(text))!;
const click = (text: string) =>
  act(async () => {
    button(text).props.onClick();
  });
const api = (
  overrides: Partial<MpaLifecycleServices> = {},
): MpaLifecycleServices => ({
  tosBuckets: vi.fn(async () => []),
  config: async () => ({
    configured: true,
    region: 'cn-beijing',
    postgresMode: 'auto',
  }),
  checkName: vi.fn(async () => {}),
  create: vi.fn(async () => task),
  creations: vi.fn(async () => []),
  readCreation: vi.fn(async () => task),
  cancelCreation: vi.fn(async () => ({ ...task, state: 'cancelling' })),
  previewDelete: vi.fn(async () => preview),
  delete: vi.fn(async () => deletion),
  readDeletion: vi.fn(async () => deletion),
  deletions: vi.fn(async () => []),
  ...overrides,
});
async function mountCreate(
  services: MpaLifecycleServices,
  initial?: MpaCreationTask,
) {
  await act(async () => {
    renderer = create(
      <MpaCreateDialog
        services={services}
        epoch={7}
        scope="owner"
        region="cn-beijing"
        task={initial}
        onChanged={() => {}}
        onClose={() => {}}
        onConnect={async () => {}}
      />,
    );
  });
}
async function fillBasics() {
  const fields = renderer!.root.findAllByType('input');
  await act(async () => {
    fields[0].props.onChange({ target: { value: input.name } });
    renderer!.root
      .findByType('textarea')
      .props.onChange({ target: { value: input.description } });
  });
}
const workerDiagnostic = {
  stage: 'worker',
  operation: 'worker_agent_key',
  category: 'metadata_pending',
  attempt: 1,
  outcome: 'retrying',
};
it.each(['poll', 'reopen'])(
  'shows the completed status without intermediate diagnostics after success (%s)',
  async (source) => {
    vi.useFakeTimers();
    const completed: MpaCreationTask = {
      ...task,
      state: 'succeeded',
      stage: 'verifying',
      latestDiagnostic: workerDiagnostic,
      result: {
        runtime_id: 'r-created',
        skill_space_id: 'ss-created',
        gateway_id: 'g-created',
      },
    };
    await mountCreate(
      api({ readCreation: vi.fn(async () => completed) }),
      source === 'reopen'
        ? completed
        : { ...task, latestDiagnostic: workerDiagnostic },
    );
    if (source === 'poll') {
      expect(
        renderer!.root.findByProps({ role: 'status' }).findAllByType('details'),
      ).toHaveLength(0);
      expect(renderer!.root.findAllByType('progress')).toHaveLength(1);
      await act(async () => {
        await vi.advanceTimersByTimeAsync(1000);
      });
    }
    const status = renderer!.root.findByProps({ role: 'status' });
    expect(status.findAllByProps({ 'data-state': 'succeeded' })).toHaveLength(
      1,
    );
    expect(status.findAllByType('span')).toHaveLength(1);
    expect(status.findAllByType('progress')).toHaveLength(0);
    expect(status.findAllByType('details')).toHaveLength(0);
    expect(renderer!.root.findAllByType('details')).toHaveLength(0);
    const text = JSON.stringify(renderer!.toJSON());
    expect(text).not.toContain(m.mpaHistoricalDiagnostic());
    expect(text).not.toContain(m.mpaHistoricalDiagnosticHint());
    expect(text).not.toContain('worker_agent_key');
    expect(text).not.toContain(m.mpaResourcesMayRemain());
    expect(button(m.mpaConnectCreated())).toBeDefined();
  },
);
it('retains the current stage and actionable diagnosis for failed creation', async () => {
  await mountCreate(api(), {
    ...task,
    state: 'failed',
    error: 'worker_failed',
    latestDiagnostic: {
      ...workerDiagnostic,
      category: 'metadata_missing',
      outcome: 'failed',
    },
  });
  const text = JSON.stringify(renderer!.toJSON());
  expect(text).toContain(m.mpaFailed());
  expect(text).toContain(m.mpaStageCreateWorker());
  expect(text).toContain(m.mpaFailureReasonMetadataMissing());
  expect(text).toContain(m.mpaResourcesMayRemain());
  expect(text).not.toContain(m.mpaHistoricalDiagnosticHint());
  expect(renderer!.root.findByType('summary').children).toContain(
    m.mpaFailureDetails(),
  );
});
it('creates only at final confirmation, using the same name-check identity', async () => {
  const services = api();
  await mountCreate(services);
  await fillBasics();
  await click(m.mpaNext());
  expect(services.create).not.toHaveBeenCalled();
  const checked = vi.mocked(services.checkName).mock.calls[0];
  expect(checked).toEqual([7, 'cn-beijing', expect.any(String), input.name]);
  await click(m.mpaNext());
  await click(m.mpaSubmit());
  expect(services.create).toHaveBeenCalledExactlyOnceWith(
    7,
    expect.objectContaining({ ...input, requestId: checked[2] }),
  );
  expect(JSON.stringify(renderer!.toJSON())).toContain(m.mpaRunning());
});
it('disables automatic capitalization and correction while preserving the exact MPA name', async () => {
  const services = api();
  await mountCreate(services);
  const name = renderer!.root.findAllByType('input')[0];
  expect(name.props).toMatchObject({
    autoCapitalize: 'none',
    autoCorrect: 'off',
    autoComplete: 'off',
    spellCheck: false,
  });
  await act(async () => {
    name.props.onChange({ target: { value: 'myMpa-name' } });
    renderer!.root
      .findByType('textarea')
      .props.onChange({ target: { value: 'Test description' } });
  });
  await click(m.mpaNext());
  expect(services.checkName).toHaveBeenCalledWith(
    7,
    'cn-beijing',
    expect.any(String),
    'myMpa-name',
  );
});
it('submits the TOS bucket selected from the live directory', async () => {
  const selected = 'agentkit-platform-my-outputs';
  const services = api({
    tosBuckets: vi.fn(async () => [{ name: selected, region: 'cn-beijing' }]),
  });
  await mountCreate(services);
  await fillBasics();
  await click(m.mpaNext());
  await click(m.mpaNext());
  expect(services.tosBuckets).toHaveBeenCalledExactlyOnceWith(7, 'cn-beijing');
  await act(async () =>
    renderer!.root.findByProps({ role: 'combobox' }).props.onClick(),
  );
  await act(async () =>
    renderer!.root
      .findByProps({ role: 'option', value: selected })
      .props.onClick(),
  );
  await click(m.mpaSubmit());
  expect(services.create).toHaveBeenCalledWith(
    7,
    expect.objectContaining({ tosBucket: selected }),
  );
});
it('recovers an unknown creation by original request ID without submitting again', async () => {
  persistCreationDraft('openmpa:mpa-create:owner:cn-beijing', input, true);
  const services = api({ creations: vi.fn(async () => [task]) });
  await mountCreate(services);
  expect(button(m.mpaNewCreation())).toBeUndefined();
  await click(m.mpaReadStatus());
  expect(services.create).not.toHaveBeenCalled();
  expect(JSON.stringify(renderer!.toJSON())).toContain(m.mpaRunning());
  expect(button(m.mpaRetryCreation())).toBeUndefined();
});
it('retries terminal creation with the original resources and never persists an OpenViking key', async () => {
  const services = api();
  const manual = {
    ...task,
    state: 'failed',
    pgHost: 'private-pg',
    pgPort: '5432',
    tosBucket: 'agentkit-platform-outputs',
    openvikingMode: 'manual',
    openvikingUrl: 'https://ov.invalid',
    openvikingResourceId: 'ov-one',
  };
  await mountCreate(services, manual);
  expect(button(m.mpaRetryCreation()).props.disabled).toBe(true);
  await act(async () =>
    renderer!.root
      .findByType('input')
      .props.onChange({ target: { value: 'SECRET_KEY' } }),
  );
  const saved = restoreCreationDraft(
    'openmpa:mpa-create:owner:cn-beijing',
    'cn-beijing',
  );
  expect(saved?.input.openvikingApiKey).toBeUndefined();
  expect(JSON.stringify(saved)).not.toContain('SECRET_KEY');
  await click(m.mpaRetryCreation());
  expect(services.create).toHaveBeenCalledWith(
    7,
    expect.objectContaining({
      requestId,
      pgHost: 'private-pg',
      tosBucket: 'agentkit-platform-outputs',
      openvikingApiKey: 'SECRET_KEY',
    }),
  );
});
it('blocks deletion for active sessions before any write', async () => {
  const services = api({
    previewDelete: vi.fn(async () => ({
      ...preview,
      canDelete: false,
      blockers: ['active_sessions'],
      sessionCounts: { total: 1, active: 1, idle: 0 },
    })),
  });
  await act(async () => {
    renderer = create(
      <MpaDeleteDialog
        services={services}
        epoch={7}
        region="cn-beijing"
        runtimeId="r-one"
        name="One"
        onClose={() => {}}
        onChanged={() => {}}
        onDeleted={async () => {}}
      />,
    );
  });
  expect(button(m.mpaDeleteConfirm()).props.disabled).toBe(true);
  expect(services.delete).not.toHaveBeenCalled();
  expect(JSON.stringify(renderer!.toJSON())).toContain(m.mpaDeleteActive());
});
it('binds deletion to the preview and verifies an unknown receipt without another write', async () => {
  const services = api({
    delete: vi.fn(async () => {
      throw { code: 'outcomeUnknown' };
    }),
    readDeletion: vi.fn(async () => ({
      ...deletion,
      state: 'succeeded',
      stage: 'complete',
    })),
  });
  const onDeleted = vi.fn(async () => {});
  await act(async () => {
    renderer = create(
      <MpaDeleteDialog
        services={services}
        epoch={7}
        region="cn-beijing"
        runtimeId="r-one"
        name="One"
        onClose={() => {}}
        onChanged={() => {}}
        onDeleted={onDeleted}
      />,
    );
  });
  await click(m.mpaDeleteConfirm());
  expect(services.delete).toHaveBeenCalledExactlyOnceWith(
    7,
    preview,
    expect.any(String),
  );
  expect(button(m.mpaDeleteConfirm())).toBeUndefined();
  await click(m.mpaReadStatus());
  expect(services.delete).toHaveBeenCalledOnce();
  expect(onDeleted).toHaveBeenCalledExactlyOnceWith('r-one');
});
it('provides creation before any Runtime is connected and clears the deleted active connection', async () => {
  const services = api({
    readDeletion: vi.fn(async () => ({
      ...deletion,
      state: 'succeeded',
      stage: 'complete',
    })),
  });
  const resetRuntime = vi.fn(async () => {});
  const onConnection = vi.fn();
  const desktop = {
    lifecycle: services,
    connection: { status: async () => undefined },
    platform: {
      status: async () => ({
        signedIn: true,
        accountId: 'account',
        scope: 'owner',
        epoch: 7,
        region: 'cn-beijing',
      }),
      list: async () => [
        { id: 'r-one', name: 'One', status: 'Ready', version: '1' },
      ],
      resetRuntime,
    },
  } as unknown as DesktopServices;
  const mount = async (connected: boolean) =>
    act(async () => {
      renderer = create(
        <RuntimeManager
          services={desktop}
          connection={
            connected
              ? {
                  epoch: 2,
                  scope: 'connection',
                  runtimeId: 'r-one',
                  status: 'ready',
                }
              : undefined
          }
          onConnection={onConnection}
          onDirectoryChanged={async () => {}}
        />,
      );
    });
  await mount(false);
  expect(button(m.mpaCreate()).props.disabled).toBe(false);
  await act(async () => renderer!.unmount());
  await mount(true);
  await click(m.mpaDelete());
  await click(m.mpaDeleteConfirm());
  await click(m.mpaReadStatus());
  expect(resetRuntime).not.toHaveBeenCalled();
  expect(onConnection).toHaveBeenCalledWith(undefined);
});
it('maps only fixed typed IPC requests and preserves native outcome errors', async () => {
  const native = {
    delete: vi.fn(async () => ({ ok: true, value: deletion })),
    readDeletion: vi.fn(async () => ({
      ok: false,
      error: { code: 'outcomeUnknown', message: 'safe', httpStatus: 0 },
    })),
  } as unknown as typeof MPALifecycle;
  const services = createLifecycleServices(native);
  await services.delete(7, preview, requestId);
  expect(native.delete).toHaveBeenCalledExactlyOnceWith({
    platformEpoch: 7,
    region: preview.region,
    runtimeId: preview.runtimeId,
    fingerprint: preview.fingerprint,
    operationId: requestId,
    confirm: true,
  });
  await expect(
    services.readDeletion(7, preview.region, requestId),
  ).rejects.toMatchObject({ code: 'outcomeUnknown' });
});
