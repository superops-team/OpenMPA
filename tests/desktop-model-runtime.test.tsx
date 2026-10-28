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
    .findByProps({ placeholder: 'Search Runtime models' })
    .props.onKeyDown(event);
  expect(event.preventDefault).toHaveBeenCalledOnce();
  expect(event.stopPropagation).toHaveBeenCalledOnce();
  expect(apply).not.toHaveBeenCalled();
});
