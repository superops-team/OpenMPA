import { act, create, type ReactTestRenderer } from 'react-test-renderer';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { useComposerModel } from '../src/client/desktop/use-composer-model';
import { ComposerModelPicker } from '../src/client/desktop/ComposerModelPicker';
import type {
  DesktopServices,
  ExecutionConfigVM,
} from '../src/client/desktop/models';
import type { RuntimeModelCatalog } from '../src/client/desktop/model-catalog';

let renderer: ReactTestRenderer | undefined;
beforeEach(() => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  vi.stubGlobal('mygo', { locale: 'en' });
});
afterEach(async () => {
  if (renderer) await act(async () => renderer!.unmount());
  renderer = undefined;
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

it('refreshes a continuously mounted catalog when its memory cache expires', async () => {
  vi.useFakeTimers();
  vi.setSystemTime(0);
  let calls = 0;
  const models = vi.fn(async () => ({
    supported: true,
    defaultModel: 'new-runtime-model',
    models: [
      {
        id: ++calls === 1 ? 'old-model' : 'new-runtime-model',
      },
    ],
  }));
  const api = services(models);
  await act(async () => {
    renderer = create(<Harness services={api} />);
  });
  expect(models).toHaveBeenCalledTimes(1);
  expect(
    renderer!.root.findByProps({ 'data-testid': 'models' }).children.join(''),
  ).toBe('old-model');
  await act(async () => {
    await vi.advanceTimersByTimeAsync(12 * 60 * 60 * 1000);
  });
  expect(models).toHaveBeenCalledTimes(2);
  expect(
    renderer!.root.findByProps({ 'data-testid': 'models' }).children.join(''),
  ).toBe('new-runtime-model');
  expect(api.sessions.updateExecutionConfig).not.toHaveBeenCalled();
});

it('does not bypass an excluded known default by selecting inherit', async () => {
  const api = services(async () => ({
    supported: true,
    defaultModel: 'excluded-default',
    models: [{ id: 'new-runtime-model' }],
  }));
  vi.mocked(api.sessions.executionConfig).mockResolvedValue({
    ...config,
    modelMode: 'inherit',
    model: 'excluded-default',
  });
  function DefaultHarness() {
    const state = useComposerModel({
      services: api,
      connection: {
        epoch: 1,
        scope: 'default-test',
        runtimeId: 'r',
        status: 'connected',
      },
      session: { id: 's', title: 's' },
    });
    return (
      <>
        <button onClick={() => void state.apply('inherit')}>inherit</button>
        <ComposerModelPicker
          config={state.config}
          busy={state.busy}
          catalog={state.catalog}
          onApply={state.apply}
        />
      </>
    );
  }
  await act(async () => {
    renderer = create(<DefaultHarness />);
  });
  await act(async () => {
    renderer!.root
      .findAllByType('button')
      .find((button) => button.children.includes('inherit'))!
      .props.onClick();
  });
  expect(api.sessions.updateExecutionConfig).not.toHaveBeenCalled();
  await act(async () =>
    renderer!.root
      .findByProps({ className: 'composer-model-trigger' })
      .props.onClick(),
  );
  const option = renderer!.root
    .findAllByProps({ role: 'radio' })
    .find((button) =>
      button.findByType('strong').children.includes('Runtime default'),
    )!;
  expect(option.props.disabled).toBe(true);
});
const config: ExecutionConfigVM = {
  executionConfigVersion: '7',
  etag: '"7"',
  model: 'new-runtime-model',
  modelMode: 'replace',
  mcpMode: 'replace',
  mcpServers: ['existing-mcp'],
  profileBound: true,
  invalidRefsCount: 0,
};
function Harness({
  services,
  epoch = 1,
}: {
  services: DesktopServices;
  epoch?: number;
}) {
  const state = useComposerModel({
    services,
    connection: {
      epoch,
      scope: `scope-${epoch}`,
      runtimeId: `r-${epoch}`,
      status: 'connected',
    },
    session: { id: 's', title: 's' },
  });
  return (
    <>
      <p data-testid="models">
        {state.catalog.models.map((m) => m.id).join(',')}
      </p>
      <button onClick={() => void state.apply('replace', 'new-runtime-model')}>
        apply
      </button>
      <button onClick={() => void state.apply('inherit')}>inherit</button>
      <ComposerModelPicker
        config={state.config}
        busy={state.busy}
        catalog={state.catalog}
        catalogLoading={state.catalogLoading}
        catalogError={state.catalogError}
        onRefreshCatalog={state.refreshCatalog}
        onApply={state.apply}
      />
    </>
  );
}
function services(models: (epoch: number) => Promise<RuntimeModelCatalog>) {
  return {
    sessions: {
      models,
      executionConfig: vi.fn(async () => config),
      updateExecutionConfig: vi.fn(async () => ({
        ...config,
        executionConfigVersion: '8',
        etag: '"8"',
      })),
    },
  } as unknown as DesktopServices;
}
it('accepts newly advertised model IDs without a client release and keeps MCP unchanged', async () => {
  const api = services(async () => ({
    supported: true,
    defaultModel: 'new-runtime-model',
    models: [{ id: 'new-runtime-model' }],
  }));
  await act(async () => {
    renderer = create(<Harness services={api} />);
  });
  expect(api.sessions.updateExecutionConfig).not.toHaveBeenCalled();
  await act(async () =>
    renderer!.root
      .findAllByType('button')
      .find((b) => b.children.includes('apply'))!
      .props.onClick(),
  );
  expect(api.sessions.updateExecutionConfig).toHaveBeenCalledExactlyOnceWith(
    1,
    's',
    {
      etag: '"7"',
      modelMode: 'replace',
      model: 'new-runtime-model',
      mcpMode: 'replace',
      mcpServers: ['existing-mcp'],
    },
  );
});
it('does not restore a stale model list after switching Runtime', async () => {
  let first!: (value: RuntimeModelCatalog) => void;
  const api = services(async (epoch) =>
    epoch === 1
      ? new Promise((resolve) => {
          first = resolve;
        })
      : {
          supported: true,
          defaultModel: 'second-model',
          models: [{ id: 'second-model' }],
        },
  );
  await act(async () => {
    renderer = create(<Harness services={api} />);
  });
  await act(async () => renderer!.update(<Harness services={api} epoch={2} />));
  await act(async () =>
    first({
      supported: true,
      defaultModel: 'old-model',
      models: [{ id: 'old-model' }],
    }),
  );
  expect(
    renderer!.root.findByProps({ 'data-testid': 'models' }).children.join(''),
  ).toBe('second-model');
});
it('shows explicit failure with retry and no static fallback or configuration write', async () => {
  const load = vi
    .fn()
    .mockRejectedValueOnce(new Error('offline'))
    .mockResolvedValueOnce({
      supported: true,
      defaultModel: 'new-runtime-model',
      models: [{ id: 'new-runtime-model' }],
    });
  const api = services(load);
  await act(async () => {
    renderer = create(<Harness services={api} />);
  });
  await act(async () =>
    renderer!.root
      .findByProps({ 'aria-label': 'Model: new-runtime-model' })
      .props.onClick(),
  );
  expect(JSON.stringify(renderer!.toJSON())).toContain(
    'Could not read this Runtime',
  );
  expect(JSON.stringify(renderer!.toJSON())).not.toContain('deepseek-v4-flash');
  await act(async () =>
    renderer!.root
      .findAllByType('button')
      .find((b) => b.children.includes('Refresh models'))!
      .props.onClick(),
  );
  expect(
    renderer!.root.findByProps({ 'data-testid': 'models' }).children.join(''),
  ).toBe('new-runtime-model');
  expect(api.sessions.updateExecutionConfig).not.toHaveBeenCalled();
});
it('does not submit the enclosing chat form when Enter is used in model search', async () => {
  const apply = vi.fn(async () => true);
  await act(async () => {
    renderer = create(
      <ComposerModelPicker
        config={config}
        busy={false}
        catalog={{
          supported: true,
          defaultModel: 'new-runtime-model',
          models: [{ id: 'new-runtime-model' }],
        }}
        onApply={apply}
      />,
    );
  });
  await act(async () =>
    renderer!.root
      .findByProps({ 'aria-label': 'Model: new-runtime-model' })
      .props.onClick(),
  );
  const event = {
    key: 'Enter',
    preventDefault: vi.fn(),
    stopPropagation: vi.fn(),
  };
  renderer!.root
    .findByProps({ 'aria-label': 'Filter models' })
    .props.onKeyDown(event);
  expect(event.preventDefault).toHaveBeenCalledOnce();
  expect(event.stopPropagation).toHaveBeenCalledOnce();
  expect(apply).not.toHaveBeenCalled();
});

it('uses the known inherited profile model rather than treating the advertised default as its target', async () => {
  const api = services(async () => ({
    supported: true,
    defaultModel: 'excluded-advertised-default',
    models: [{ id: 'new-runtime-model' }],
  }));
  vi.mocked(api.sessions.executionConfig).mockResolvedValue({
    ...config,
    modelMode: 'inherit',
  });
  await act(async () => {
    renderer = create(<Harness services={api} />);
  });
  await act(async () =>
    renderer!.root
      .findAllByType('button')
      .find((button) => button.children.includes('inherit'))!
      .props.onClick(),
  );
  expect(api.sessions.updateExecutionConfig).toHaveBeenCalledWith(
    1,
    's',
    expect.objectContaining({ modelMode: 'inherit', model: '' }),
  );
});
