import { StrictMode } from 'react';
import { act, create, type ReactTestRenderer } from 'react-test-renderer';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import {
  MpaDeletionDiagnostic,
  deletionErrorDiagnostic,
} from '../src/client/desktop/MpaDeletionDiagnostic';
import { MpaDeleteDialog } from '../src/client/desktop/MpaDeleteDialog';
import { createLifecycleServices } from '../src/client/desktop/lifecycle-services';
import { messagesIn } from '../src/client/desktop/generated/locales';
import type { MPALifecycle } from '../src/client/desktop/generated/mygo';
import type { MpaLifecycleServices } from '../src/client/desktop/lifecycle-models';
const m = messagesIn('en');
const diagnostic = {
  stage: 'references',
  operation: 'list_runtimes',
  category: 'timeout',
  attempt: 1,
  outcome: 'failed',
  exceptionType: 'TimeoutError',
  httpStatus: 504,
  requestId: '0123456789abcdef0123456789abcdef',
  source: 'threads.py:25:to_thread',
  resourceHost: 'registry.example.invalid',
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
it('renders and copies the actual deletion stage and structured error details', async () => {
  const writeText = vi.fn(async () => {});
  vi.stubGlobal('navigator', { clipboard: { writeText } });
  await act(async () => {
    renderer = create(<MpaDeletionDiagnostic diagnostic={diagnostic} />);
  });
  const text = JSON.stringify(renderer!.toJSON());
  expect(text).toContain(m.mpaDeleteReferences());
  expect(text).toContain('list_runtimes');
  expect(text).toContain('TimeoutError');
  expect(text).toContain('registry.example.invalid');
  expect(text).not.toContain('Creation failed');
  await act(async () => {
    renderer!.root.findByType('button').props.onClick();
  });
  expect(writeText).toHaveBeenCalledWith(
    expect.stringContaining('HTTP status: 504'),
  );
  expect(JSON.stringify(renderer!.toJSON())).toContain(m.mpaFailureCopied());
});
it('drops unbounded provider text and unsafe diagnostic extras', async () => {
  await act(async () => {
    renderer = create(
      <MpaDeletionDiagnostic
        diagnostic={{
          ...diagnostic,
          exceptionType: 'SECRET password',
          source: 'postgres://secret',
          resourceHost: 'https://secret?password=SECRET',
          reason: 'SECRET',
          errorCode: 'SECRET_TOKEN',
          requestId: 'SECRET',
        }}
      />,
    );
  });
  const text = JSON.stringify(renderer!.toJSON());
  expect(text).not.toContain('SECRET');
  expect(text).not.toContain('postgres://');
  expect(
    deletionErrorDiagnostic({ diagnostic: { message: 'SECRET' } }),
  ).toBeUndefined();
});
it('preserves the diagnostic across the real lifecycle result adapter', async () => {
  const api = {
    previewDelete: vi.fn(async () => ({
      ok: false,
      error: {
        code: 'deletePreviewFailed',
        message: 'Safe summary',
        httpStatus: 503,
        diagnostic,
      },
    })),
  } as unknown as typeof MPALifecycle;
  const services = createLifecycleServices(api);
  await expect(
    services.previewDelete(7, 'cn-beijing', 'r-one'),
  ).rejects.toMatchObject({ code: 'deletePreviewFailed', diagnostic });
});
it('shows preflight error details without exposing a delete confirmation or submitting a deletion', async () => {
  const remove = vi.fn();
  const services = {
    previewDelete: vi.fn(async () => {
      throw Object.assign(new Error('private exception body'), {
        code: 'deletePreviewFailed',
        httpStatus: 503,
        diagnostic,
      });
    }),
    delete: remove,
  } as unknown as MpaLifecycleServices;
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
  const text = JSON.stringify(renderer!.toJSON());
  expect(text).toContain(m.mpaDeleteReferences());
  expect(text).not.toContain('private exception body');
  expect(
    renderer!.root
      .findAllByType('button')
      .some((node) => node.children.includes(m.mpaDeleteConfirm())),
  ).toBe(false);
  expect(remove).not.toHaveBeenCalled();
});

it('deduplicates the initial deletion check under React StrictMode', async () => {
  let resolve!: (
    value: Awaited<ReturnType<MpaLifecycleServices['previewDelete']>>,
  ) => void;
  const previewDelete = vi.fn(
    () =>
      new Promise<Awaited<ReturnType<MpaLifecycleServices['previewDelete']>>>(
        (done) => {
          resolve = done;
        },
      ),
  );
  const services = { previewDelete } as unknown as MpaLifecycleServices;
  await act(async () => {
    renderer = create(
      <StrictMode>
        <MpaDeleteDialog
          services={services}
          epoch={7}
          region="cn-beijing"
          runtimeId="r-one"
          name="One"
          onClose={() => {}}
          onChanged={() => {}}
          onDeleted={async () => {}}
        />
      </StrictMode>,
    );
  });
  expect(previewDelete).toHaveBeenCalledTimes(1);
  const recheck = renderer!.root
    .findAllByType('button')
    .find((node) => node.children.includes(m.mpaDeleteRecheck()))!;
  await act(async () => {
    recheck.props.onClick();
  });
  expect(previewDelete).toHaveBeenCalledTimes(1);
  await act(async () => {
    resolve({
      runtimeId: 'r-one',
      region: 'cn-beijing',
      agentId: 'a-one',
      name: 'One',
      fingerprint: 'a'.repeat(64),
      canDelete: true,
      blockers: [],
      sessionCounts: { total: 0, active: 0, idle: 0 },
      cleanupPlan: [],
      retainedResources: [],
    });
  });
  expect(
    renderer!.root
      .findAllByType('button')
      .some((node) => node.children.includes(m.mpaDeleteConfirm())),
  ).toBe(true);
  expect(renderer!.root.findAllByProps({ role: 'alert' })).toHaveLength(0);
});
it('classifies AccountFlowLimitExceeded as throttling instead of an unknown failure', async () => {
  await act(async () => {
    renderer = create(
      <MpaDeletionDiagnostic
        diagnostic={{
          ...diagnostic,
          category: 'provider_error',
          errorCode: 'AccountFlowLimitExceeded',
        }}
      />,
    );
  });
  const text = JSON.stringify(renderer!.toJSON());
  expect(text).toContain(m.mpaFailureReasonThrottled());
  expect(text).toContain('AccountFlowLimitExceeded');
  expect(text).not.toContain(m.mpaDeleteFailureUnknown());
});
