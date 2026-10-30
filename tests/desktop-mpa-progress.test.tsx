import { act, create, type ReactTestRenderer } from 'react-test-renderer';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import {
  creationStepStates,
  creationStages,
} from '../src/client/desktop/MpaCreationProgress';
import { MpaCreationDiagnostic } from '../src/client/desktop/MpaCreationDiagnostic';
import {
  mpaFailureReason,
  safeMpaDiagnostic,
} from '../src/client/desktop/mpa-diagnostic';
import { messagesIn } from '../src/client/desktop/generated/locales';
import type { MpaCreationTask } from '../src/client/desktop/lifecycle-models';

const m = messagesIn('en');
const task: MpaCreationTask = {
  taskId: 'task',
  requestId: 'request',
  region: 'cn-beijing',
  name: 'my-mpa',
  description: 'test',
  maxInstance: 20,
  state: 'running',
  stage: 'worker',
  openvikingMode: 'auto',
  visitedStages: [
    'queued',
    'checking',
    'iam_role',
    'network',
    'gateway',
    'worker',
  ],
};
const diagnostic = {
  stage: 'worker',
  operation: 'get_worker',
  category: 'permission',
  attempt: 2,
  outcome: 'failed',
  errorCode: 'AccessDenied.Tool',
  requestId: '0123456789abcdef0123456789abcdef',
  exceptionType: 'HTTPError -> ApiError',
  httpStatus: 403,
  responseType: 'application/json',
};
let renderer: ReactTestRenderer | undefined;
beforeEach(() => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  vi.stubGlobal('mygo', { locale: 'en' });
});
afterEach(async () => {
  if (renderer) await act(async () => renderer!.unmount());
  renderer = undefined;
  vi.unstubAllGlobals();
});
const states = (value: MpaCreationTask) =>
  Object.fromEntries(
    creationStepStates(value).map(({ stage, state }) => [stage, state]),
  );
it('shows observed completion, current execution, future steps and resources skipped between stages', () => {
  expect(creationStepStates(task)).toHaveLength(11);
  expect(states(task)).toMatchObject({
    checking: 'completed',
    iam_role: 'completed',
    admin_workspace: 'skipped',
    gateway: 'completed',
    worker: 'running',
    openviking: 'pending',
    deploying: 'pending',
    verifying: 'pending',
  });
});
it('does not invent history for older tasks and preserves failed and cancelled steps', () => {
  expect(states({ ...task, visitedStages: undefined })).toMatchObject({
    checking: 'passed',
    worker: 'running',
    verifying: 'pending',
  });
  expect(states({ ...task, visitedStages: ['worker'] }).checking).toBe(
    'passed',
  );
  expect(
    states({
      ...task,
      state: 'succeeded',
      stage: 'verifying',
      visitedStages: undefined,
    }).verifying,
  ).toBe('completed');
  expect(states({ ...task, state: 'failed' })).toMatchObject({
    checking: 'completed',
    worker: 'failed',
    openviking: 'pending',
  });
  expect(states({ ...task, state: 'cancelled' })).toMatchObject({
    worker: 'cancelled',
    deploying: 'pending',
  });
  expect(
    Object.values(
      states({ ...task, stage: 'queued', visitedStages: ['queued'] }),
    ),
  ).toEqual(creationStages.map(() => 'pending'));
});
it('supports repeated stages, configured skips and terminal success without a running step', () => {
  expect(
    states({
      ...task,
      stage: 'admin_database',
      visitedStages: [
        'checking',
        'admin_workspace',
        'admin_database',
        'business_workspace',
      ],
    }),
  ).toMatchObject({
    admin_database: 'running',
    business_workspace: 'completed',
    network: 'pending',
  });
  expect(states({ ...task, openvikingMode: 'manual' }).openviking).toBe(
    'skipped',
  );
  expect(
    states({
      ...task,
      stage: 'openviking_key',
      visitedStages: [...task.visitedStages!, 'openviking'],
    }),
  ).toMatchObject({ openviking: 'running', deploying: 'pending' });
  const completed = states({
    ...task,
    stage: 'verifying',
    state: 'succeeded',
    visitedStages: [...creationStages],
  });
  expect(Object.values(completed).every((value) => value === 'completed')).toBe(
    true,
  );
});
it.each([
  ['ExceededQuota.Tool', 'quota'],
  ['QuotaExceed', 'quota'],
  ['QuotaExceed.Resource', 'quota'],
  ['QuotaExceeded.Tool', 'quota'],
  ['QuotaExceed.EndpointEachVpc', 'vpcEndpointQuota'],
  ['AccessDenied.Tool', 'permission'],
  ['DuplicateName.Tool', 'nameConflict'],
  ['PostgreSQL.28P01', 'databaseAuthentication'],
  ['PostgreSQL.28000', 'databaseAuthentication'],
  ['PostgreSQL.42501', 'databasePermission'],
])('matches Studio reason for %s', (errorCode, reason) => {
  expect(
    mpaFailureReason(
      { ...task, state: 'failed' },
      { ...diagnostic, errorCode },
    ),
  ).toBe(reason);
});
it('preserves safe diagnostic fields but rejects free text, secrets, body content and invalid status', () => {
  expect(safeMpaDiagnostic(diagnostic)).toMatchObject({
    errorCode: diagnostic.errorCode,
    exceptionType: diagnostic.exceptionType,
    httpStatus: 403,
    responseType: 'application/json',
  });
  expect(
    safeMpaDiagnostic({
      ...diagnostic,
      errorCode: 'SECRET_TOKEN',
      requestId: 'password=private',
      exceptionType: 'private secret',
      httpStatus: 999,
      responseType: '<html>secret</html>',
      operation: 'https://secret.invalid',
    }),
  ).toEqual({
    errorCode: '',
    requestId: '',
    exceptionType: '',
    httpStatus: undefined,
    responseType: '',
    operation: '',
  });
});
it('shows and copies the localized failure cause with structured cloud details', async () => {
  const writeText = vi.fn(async () => {});
  vi.stubGlobal('navigator', { clipboard: { writeText } });
  await act(async () => {
    renderer = create(
      <MpaCreationDiagnostic
        task={{ ...task, state: 'failed', latestDiagnostic: diagnostic }}
      />,
    );
  });
  const text = JSON.stringify(renderer!.toJSON());
  expect(text).toContain(m.mpaFailureReasonPermission());
  expect(text).toContain('HTTPError -> ApiError');
  expect(text).toContain('403');
  expect(text).toContain('application/json');
  await act(async () => {
    renderer!.root.findByType('button').props.onClick();
  });
  expect(writeText).toHaveBeenCalledWith(
    expect.stringContaining(`Cloud Request ID: ${diagnostic.requestId}`),
  );
  expect(JSON.stringify(renderer!.toJSON())).toContain(m.mpaFailureCopied());
});
it.each(['en', 'zh-Hans'] as const)(
  'shows and copies the recovered VPC quota error in %s',
  async (language) => {
    vi.stubGlobal('mygo', { locale: language });
    const localized = messagesIn(language);
    const writeText = vi.fn(async () => {});
    vi.stubGlobal('navigator', { clipboard: { writeText } });
    const recovered = {
      ...diagnostic,
      stage: 'verifying',
      operation: 'wait_runtime',
      category: 'resource_failed',
      errorCode: 'QuotaExceed.EndpointEachVpc',
      exceptionType: 'RuntimeError',
      httpStatus: undefined,
    };
    await act(async () => {
      renderer = create(
        <MpaCreationDiagnostic
          task={{
            ...task,
            state: 'failed',
            stage: 'verifying',
            latestDiagnostic: recovered,
          }}
        />,
      );
    });
    const text = JSON.stringify(renderer!.toJSON());
    expect(text).toContain(localized.mpaFailureReasonVpcEndpointQuota());
    expect(text).toContain(recovered.errorCode);
    expect(text).toContain(recovered.requestId);
    expect(text).not.toContain(localized.mpaFailureReasonResourceFailed());
    await act(async () => renderer!.root.findByType('button').props.onClick());
    expect(writeText).toHaveBeenCalledWith(
      expect.stringContaining(recovered.errorCode),
    );
    expect(writeText).toHaveBeenCalledWith(
      expect.stringContaining(recovered.requestId),
    );
  },
);
it.each(['en', 'zh-Hans'] as const)(
  'shows recovered errors for Worker, network, gateway and database in %s',
  async (language) => {
    vi.stubGlobal('mygo', { locale: language });
    const localized = messagesIn(language);
    for (const [stage, operation, errorCode] of [
      ['worker', 'worker_state', 'ImagePullFailed'],
      ['network', 'describe_subnet_attributes', 'InsufficientSubnetCapacity'],
      ['gateway', 'GetGateway', 'GatewayProvisioningFailed'],
      [
        'admin_workspace',
        'aidap_describe_workspace_detail',
        'WorkspaceCreateFailed',
      ],
      ['business_workspace', 'aidap_describe_computes', 'ComputeCreateFailed'],
    ]) {
      const recovered = {
        ...diagnostic,
        stage,
        operation,
        category: 'resource_failed',
        errorCode,
      };
      await act(async () => {
        renderer?.unmount();
        renderer = create(
          <MpaCreationDiagnostic
            task={{
              ...task,
              state: 'failed',
              stage,
              latestDiagnostic: recovered,
            }}
          />,
        );
      });
      const text = JSON.stringify(renderer!.toJSON());
      expect(text).toContain(
        localized.mpaFailureReasonResourceFailedWithDetails(),
      );
      expect(text).toContain(errorCode);
      expect(text).toContain(recovered.requestId);
      expect(text).toContain(operation);
      expect(text).not.toContain(localized.mpaFailureReasonResourceFailed());
    }
  },
);
it('does not misattribute an earlier retry diagnostic to a later failure', async () => {
  await act(async () => {
    renderer = create(
      <MpaCreationDiagnostic
        task={{
          ...task,
          state: 'failed',
          stage: 'deploying',
          error: 'timeout',
          latestDiagnostic: { ...diagnostic, outcome: 'retrying' },
        }}
      />,
    );
  });
  const text = JSON.stringify(renderer!.toJSON());
  expect(text).toContain(m.mpaFailureReasonTimeout());
  expect(text).not.toContain(diagnostic.errorCode);
  expect(text).not.toContain(diagnostic.requestId);
  expect(text).not.toContain(m.mpaFailureReasonPermission());
});
it('provides manual-copy feedback if the clipboard is unavailable', async () => {
  vi.stubGlobal('navigator', {});
  await act(async () => {
    renderer = create(
      <MpaCreationDiagnostic
        task={{ ...task, state: 'failed', latestDiagnostic: diagnostic }}
      />,
    );
  });
  await act(async () => {
    renderer!.root.findByType('button').props.onClick();
  });
  expect(JSON.stringify(renderer!.toJSON())).toContain(
    m.mpaFailureCopyFailed(),
  );
});

it('moves an earlier step retry into collapsed history without showing it as still retrying', async () => {
  const latestDiagnostic = {
    ...diagnostic,
    category: 'metadata_pending',
    outcome: 'retrying',
  };
  await act(async () => {
    renderer = create(
      <MpaCreationDiagnostic task={{ ...task, latestDiagnostic }} />,
    );
  });
  expect(renderer!.root.findByType('summary').children).toContain(
    m.mpaSafeDiagnostic(),
  );
  expect(JSON.stringify(renderer!.toJSON())).toContain(
    m.mpaDiagnosticRetrying(),
  );
  await act(async () => {
    renderer!.update(
      <MpaCreationDiagnostic
        task={{ ...task, stage: 'verifying', latestDiagnostic }}
      />,
    );
  });
  const history = renderer!.root.findByType('details');
  expect(history.props.open).not.toBe(true);
  expect(history.findByType('summary').children).toContain(
    m.mpaHistoricalDiagnostic(),
  );
  const text = JSON.stringify(renderer!.toJSON());
  expect(text).toContain(m.mpaHistoricalDiagnosticHint());
  expect(text).not.toContain(m.mpaDiagnosticRetrying());
});
it('keeps diagnostics current within the same grouped creation step', async () => {
  await act(async () => {
    renderer = create(
      <MpaCreationDiagnostic
        task={{
          ...task,
          stage: 'openviking_key',
          latestDiagnostic: {
            ...diagnostic,
            stage: 'openviking',
            outcome: 'retrying',
          },
        }}
      />,
    );
  });
  const text = JSON.stringify(renderer!.toJSON());
  expect(text).toContain(m.mpaSafeDiagnostic());
  expect(text).toContain(m.mpaDiagnosticRetrying());
  expect(text).not.toContain(m.mpaHistoricalDiagnostic());
});
it('removes all intermediate diagnosis when creation succeeds', async () => {
  await act(async () => {
    renderer = create(
      <MpaCreationDiagnostic
        task={{ ...task, state: 'succeeded', latestDiagnostic: diagnostic }}
      />,
    );
  });
  expect(renderer!.toJSON()).toBeNull();
});
