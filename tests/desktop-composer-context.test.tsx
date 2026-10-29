import { act, create, type ReactTestRenderer } from 'react-test-renderer';
import { readFileSync } from 'node:fs';
import { expect, it, vi } from 'vitest';
import { DesktopChat } from '../src/client/desktop/Chat';
import type { DesktopServices, TraceVM } from '../src/client/desktop/models';

const connection = {
  epoch: 2,
  scope: 'scope',
  runtimeId: 'r-yexsqsrpxcxfapor9fvl',
  runtimeName: 'mpa-agent',
  workerToolId: 't-example-codex-worker',
  runtimeImage: 'registry/mpa-agent:test',
  status: 'connected',
};

function byClass(renderer: ReactTestRenderer, name: string) {
  return renderer.root.findAll(
    (node) =>
      typeof node.props.className === 'string' &&
      node.props.className.split(/\s+/).includes(name),
  );
}

function chatService(
  updateExecutionConfig = vi.fn(),
  submit = vi.fn(async () => ({
    invocationId: 'inv-1',
    status: 'accepted',
  })),
) {
  return {
    sessions: {
      pending: async () => [],
      read: vi.fn(async () => ({
        session: { id: 's', title: 'Research assistant' },
        messages: [],
      })),
      abort: vi.fn(async () => true),
      executionConfig: vi.fn(async () => ({
        executionConfigVersion: '7',
        etag: '"rev-7"',
        model: 'deepseek-v4-pro',
        modelMode: 'inherit',
        mcpMode: 'inherit',
        mcpServers: [],
        profileBound: true,
        invalidRefsCount: 0,
      })),
      modelCatalog: vi.fn(async () => [
        { id: 'doubao-seed-2-1-pro' },
        { id: 'doubao-seed-2-1-pro-260628' },
        { id: 'doubao-seed-2-0-pro' },
        { id: 'deepseek-v4-pro-260101' },
      ]),
      updateExecutionConfig,
      submit,
      subscribe: () => ({
        events: (async function* () {})(),
        finished: Promise.resolve(),
        close: () => {},
      }),
    },
  } as unknown as DesktopServices;
}

const trace: TraceVM = {
  traceVersion: '1.2',
  sessionId: 's',
  invocationId: 'history-inv',
  coverage: { complete: true, recordCount: 1, returnedCount: 1 },
  run: { status: 'success', wallDurationMs: 10 },
  steps: [
    {
      stepId: 'root',
      sequence: 1,
      kind: 'run.total',
      title: 'Turn',
      status: 'success',
      startedAt: '2026-10-27T00:00:00Z',
      durationMs: 10,
    },
  ],
};

it('marks the Bot name and the remote architecture above the composer', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  let renderer!: ReactTestRenderer;
  await act(async () => {
    renderer = create(
      <DesktopChat
        services={chatService()}
        connection={connection}
        session={{ id: 's', title: 'Research assistant' }}
        onSchedule={() => {}}
      />,
    );
  });
  await act(async () => {});

  expect(
    renderer.root.findAllByProps({ title: 'Research assistant' }).length,
  ).toBeGreaterThan(0);
  const badge = byClass(renderer, 'composer-architecture')[0];
  expect(badge.children.join('')).toContain('MPA');

  await act(async () => renderer.unmount());
});

it('shows the current model and patches only the model while preserving MCP settings', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  const update = vi.fn(
    async (
      _epoch: number,
      _id: string,
      input: {
        etag: string;
        modelMode: string;
        model: string;
        mcpMode: string;
        mcpServers: string[];
      },
    ) => ({
      executionConfigVersion: '8',
      etag: '"rev-8"',
      model: input.model,
      modelMode: input.modelMode,
      mcpMode: input.mcpMode,
      mcpServers: input.mcpServers,
      profileBound: true,
      invalidRefsCount: 0,
    }),
  );
  const submit = vi.fn(async () => ({
    invocationId: 'inv-1',
    status: 'accepted',
  }));
  const service = chatService(update, submit);
  let renderer!: ReactTestRenderer;
  await act(async () => {
    renderer = create(
      <DesktopChat
        services={service}
        connection={connection}
        session={{ id: 's', title: 'Research assistant' }}
        onSchedule={() => {}}
      />,
    );
  });
  await act(async () => {});

  const trigger = renderer.root.findByProps({
    'aria-label': 'Model: deepseek-v4-pro',
  });
  expect(trigger.props.disabled).toBe(false);
  await act(async () => trigger.props.onClick());
  const menu = JSON.stringify(renderer.toJSON());
  expect(byClass(renderer, 'composer-model-menu').length).toBe(1);
  expect(menu).not.toContain('Model for upcoming messages');
  expect(
    renderer.root.findAllByProps({ 'aria-label': 'Close model selector' }),
  ).toHaveLength(0);

  const filter = renderer.root.findByProps({
    placeholder: 'Search by name or family',
  });
  await act(async () =>
    filter.props.onChange({ target: { value: 'doubao-seed-2-1' } }),
  );
  const optionLabels = byClass(renderer, 'composer-model-list')
    .flatMap((node) => node.findAllByType('strong'))
    .map((strong) => strong.children.join(''));
  expect(optionLabels).toContain('doubao-seed-2-1-pro');
  expect(optionLabels).not.toContain('doubao-seed-2-0-pro');
  expect(optionLabels).not.toContain('deepseek-v4-pro');
  const option = renderer.root
    .findAllByType('button')
    .find((button) =>
      button
        .findAllByType('strong')
        .some((strong) =>
          strong.children.join('').includes('doubao-seed-2-1-pro'),
        ),
    );
  expect(option).toBeTruthy();
  await act(async () => option!.props.onClick());

  expect(update).toHaveBeenCalledWith(2, 's', {
    etag: '"rev-7"',
    modelMode: 'replace',
    model: 'doubao-seed-2-1-pro',
    mcpMode: 'inherit',
    mcpServers: [],
  });
  expect(
    renderer.root.findByProps({
      'aria-label': 'Model: doubao-seed-2-1-pro',
    }),
  ).toBeTruthy();
  await act(async () =>
    renderer.root
      .findByProps({ 'aria-label': 'Model: doubao-seed-2-1-pro' })
      .props.onClick(),
  );
  const runtimeDefault = renderer.root
    .findAllByType('button')
    .find((button) =>
      button
        .findAllByType('strong')
        .some((strong) => strong.children.includes('Runtime default')),
    )!;
  await act(async () => runtimeDefault.props.onClick());
  expect(update).toHaveBeenLastCalledWith(2, 's', {
    etag: '"rev-8"',
    modelMode: 'inherit',
    model: '',
    mcpMode: 'inherit',
    mcpServers: [],
  });

  const textarea = renderer.root.findByProps({ 'aria-label': 'Message' });
  await act(async () =>
    textarea.props.onChange({ target: { value: 'next turn' } }),
  );
  await act(async () =>
    renderer.root.findByType('form').props.onSubmit({ preventDefault() {} }),
  );
  expect(submit).toHaveBeenCalledWith(2, 's', 'next turn', '8', 's');

  await act(async () => renderer.unmount());
});

it('initializes a legacy next-turn configuration on the first explicit model selection', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  const service = chatService();
  service.sessions.executionConfig = vi.fn(async () => {
    throw Object.assign(new Error('not found'), { status: 404 });
  });
  const ensure = vi.fn(async () => ({
    executionConfigVersion: '1',
    etag: '"rev-1"',
    model: 'deepseek-v4-pro',
    modelMode: 'inherit',
    mcpMode: 'inherit',
    mcpServers: [],
    profileBound: true,
    invalidRefsCount: 0,
  }));
  const update = vi.fn(async (_epoch, _session, input) => ({
    executionConfigVersion: '2',
    etag: '"rev-2"',
    model: input.model,
    modelMode: input.modelMode,
    mcpMode: input.mcpMode,
    mcpServers: input.mcpServers,
    profileBound: true,
    invalidRefsCount: 0,
  }));
  service.sessions.ensureExecutionConfig = ensure;
  service.sessions.updateExecutionConfig = update;
  let renderer!: ReactTestRenderer;
  await act(async () => {
    renderer = create(
      <DesktopChat
        services={service}
        connection={connection}
        session={{ id: 's', title: 'Research assistant' }}
        onSchedule={() => {}}
      />,
    );
  });
  await act(async () => {});

  const trigger = renderer.root.findByProps({
    'aria-label': 'Model: Runtime default',
  });
  expect(trigger.props.disabled).toBe(false);
  await act(async () => trigger.props.onClick());
  const menu = JSON.stringify(renderer.toJSON());
  expect(menu).not.toContain('first selection');
  expect(menu).not.toContain('context window');
  expect(menu).not.toContain('Use the assistant profile default');
  expect(
    renderer.root.findByProps({
      placeholder: 'Search by name or family',
    }).props.disabled,
  ).toBe(false);
  const option = renderer.root
    .findAllByType('button')
    .find((button) =>
      button
        .findAllByType('strong')
        .some((strong) => strong.children.includes('doubao-seed-2-1-pro')),
    )!;
  await act(async () => option.props.onClick());
  expect(ensure).toHaveBeenCalledWith(2, 's');
  expect(update).toHaveBeenCalledWith(
    2,
    's',
    expect.objectContaining({
      etag: '"rev-1"',
      modelMode: 'replace',
      model: 'doubao-seed-2-1-pro',
    }),
  );
  expect(byClass(renderer, 'composer-model-error').length).toBe(0);

  await act(async () => renderer.unmount());
});

it('locks next-turn model changes only while the current turn remains active', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  const service = chatService();
  service.sessions.read = vi.fn(async () => ({
    session: { id: 's', title: 'Research assistant' },
    messages: [],
    activeRun: {
      operationId: 'op',
      invocationId: 'inv-active',
      state: 'accepted',
      authority: 'runtime',
      recoveryDisposition: 'subscribeAccepted',
    },
  }));
  let renderer!: ReactTestRenderer;
  await act(async () => {
    renderer = create(
      <DesktopChat
        services={service}
        connection={connection}
        session={{ id: 's', title: 'Research assistant' }}
        onSchedule={() => {}}
      />,
    );
  });
  await act(async () => {});
  const trigger = renderer.root.findByProps({
    'aria-label': 'Model: deepseek-v4-pro',
  });
  await act(async () => trigger.props.onClick());
  expect(JSON.stringify(renderer.toJSON())).toContain(
    'current turn is still running',
  );
  expect(
    renderer.root.findByProps({
      placeholder: 'Search by name or family',
    }).props.disabled,
  ).toBe(true);
  await act(async () => renderer.unmount());
});

it('uses the shared composer radius and keeps the context row above the field', () => {
  const css = readFileSync(
    new URL('../src/client/desktop/style.css', import.meta.url),
    'utf8',
  );
  expect(css).toMatch(
    /\.desktop-app \.composer-field\s*\{[^}]*border-radius:\s*var\(--radius-2xl\);/s,
  );
  const tokens = JSON.parse(
    readFileSync(
      new URL('../design/react-desktop-v2.tokens.json', import.meta.url),
      'utf8',
    ),
  );
  expect(tokens.radius['2xl']).toBe('16px');
  expect(css).toMatch(
    /\.desktop-app \.composer-context\s*\{[^}]*border-bottom:\s*0;[^}]*border-radius:\s*var\(--radius-2xl\) var\(--radius-2xl\) 0 0;/s,
  );
  expect(css).toMatch(
    /\.desktop-app \.composer-field\s*\{[^}]*position:\s*relative;[^}]*z-index:\s*1;/s,
  );
});

it('warns when the current model is not in the Runtime catalog', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  const service = chatService();
  service.sessions.executionConfig = vi.fn(async () => ({
    executionConfigVersion: '7',
    etag: '"rev-7"',
    model: 'gpt-5.6-luna',
    modelMode: 'replace',
    mcpMode: 'inherit',
    mcpServers: [],
    profileBound: true,
    invalidRefsCount: 0,
  }));
  let renderer!: ReactTestRenderer;
  await act(async () => {
    renderer = create(
      <DesktopChat
        services={service}
        connection={connection}
        session={{ id: 's', title: 'Research assistant' }}
        onSchedule={() => {}}
      />,
    );
  });
  await act(async () => {});
  await act(async () =>
    renderer.root
      .findByProps({ 'aria-label': 'Model: gpt-5.6-luna' })
      .props.onClick(),
  );
  expect(JSON.stringify(renderer.toJSON())).toContain(
    'not in the Runtime catalog',
  );
  await act(async () => renderer.unmount());
});

it('passes completed history invocations to the trace panel', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  const service = chatService();
  service.sessions.read = vi.fn(async () => ({
    session: { id: 's', title: 'Research assistant' },
    messages: [],
    completedInvocations: ['history-inv'],
  }));
  const readTrace = vi.fn(async () => trace);
  const prefetchTrace = vi.fn(async () => {});
  service.trace = { read: readTrace, prefetch: prefetchTrace };
  let renderer!: ReactTestRenderer;
  await act(async () => {
    renderer = create(
      <DesktopChat
        services={service}
        connection={connection}
        session={{ id: 's', title: 'Research assistant' }}
        traceOpen
        onSchedule={() => {}}
      />,
    );
  });
  await act(async () => {});
  expect(prefetchTrace).toHaveBeenCalledWith(2, 's', 'history-inv');
  expect(readTrace).toHaveBeenCalledWith(2, 's', 'history-inv');
  await act(async () => renderer.unmount());
});

it('keeps trace-enabled session summaries from the desktop service', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  const service = chatService();
  service.sessions.read = vi.fn(async () => ({
    session: {
      id: 's',
      title: 'Research assistant',
      traceEnabled: true,
    },
    messages: [],
  }));
  let renderer!: ReactTestRenderer;
  await act(async () => {
    renderer = create(
      <DesktopChat
        services={service}
        connection={connection}
        session={{
          id: 's',
          title: 'Research assistant',
          traceEnabled: true,
        }}
        onSchedule={() => {}}
      />,
    );
  });
  await act(async () => {});
  expect(service.sessions.read).toHaveBeenCalledWith(2, 's');
  await act(async () => renderer.unmount());
});

it('keeps the model search focus ring aligned with the chat input', () => {
  const css = readFileSync(
    new URL('../src/client/desktop/style.css', import.meta.url),
    'utf8',
  );
  const desktopAlignment = readFileSync(
    new URL(
      '../src/client/styles/desktop-design-alignment.css',
      import.meta.url,
    ),
    'utf8',
  );
  expect(css).toMatch(
    /\.desktop-app \.composer-model-search:focus-within\s*\{[^}]*border-color:\s*var\(--color-focus\);[^}]*outline:\s*none;[^}]*box-shadow:\s*none;/s,
  );
  expect(desktopAlignment).toContain(
    '.template-app.desktop-app\n  .composer-model-search\n  input:not(.call-view *):focus-visible',
  );
  expect(desktopAlignment).toMatch(/appearance:\s*none;/);
  expect(desktopAlignment).toMatch(/outline-offset:\s*0;/);
});
