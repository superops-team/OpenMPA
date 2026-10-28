import { act, create, type ReactTestRenderer } from 'react-test-renderer';
import { expect, it, vi } from 'vitest';
import { TracePanel } from '../src/client/desktop/TracePanel';
import type { DesktopServices, TraceVM } from '../src/client/desktop/models';

const connection = {
  epoch: 2,
  scope: 'scope',
  runtimeId: 'r-1',
  status: 'connected',
};
const session = { id: 'session-1', title: 'Trace conversation' };
const trace: TraceVM = {
  traceVersion: '1.2',
  sessionId: 'session-1',
  invocationId: 'inv-1',
  run: {
    status: 'success',
    wallDurationMs: 1200,
    modelRequestCount: 1,
    toolCount: 1,
  },
  coverage: { complete: true, recordCount: 2, returnedCount: 2 },
  steps: [
    {
      stepId: 'root',
      sequence: 1,
      kind: 'run.total',
      title: 'Turn',
      status: 'success',
      startedAt: '2026-10-27T00:00:00Z',
      durationMs: 1200,
    },
    {
      stepId: 'tool',
      parentStepId: 'root',
      sequence: 2,
      kind: 'tool.call',
      title: 'Read file',
      status: 'success',
      startedAt: '2026-10-27T00:00:00.200Z',
      durationMs: 300,
    },
  ],
};

function services(
  read: (
    epoch: number,
    sessionId: string,
    invocationId: string,
  ) => Promise<TraceVM> = vi.fn(async () => trace),
) {
  return { trace: { read } } as unknown as DesktopServices;
}

it('validates the current turn trace by default', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  const read = vi.fn(async () => trace);
  let renderer!: ReactTestRenderer;
  await act(async () => {
    renderer = create(
      <TracePanel
        services={services(read)}
        connection={connection}
        session={session}
        invocationIds={['inv-1']}
        currentInvocationId="inv-1"
        onClose={() => {}}
      />,
    );
  });
  await act(async () => {});
  expect(read).toHaveBeenCalledWith(2, 'session-1', 'inv-1');
  expect(JSON.stringify(renderer.toJSON())).toContain('Trace validated');
  expect(JSON.stringify(renderer.toJSON())).toContain('Read file');
  expect(JSON.stringify(renderer.toJSON())).toContain('Duration');
  await act(async () => renderer.unmount());
});

it('shows the admin_debug limitation for ordinary session trace 404', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  const read = vi.fn(async () => {
    throw Object.assign(new Error('not found'), { httpStatus: 404 });
  });
  let renderer!: ReactTestRenderer;
  await act(async () => {
    renderer = create(
      <TracePanel
        services={services(read)}
        connection={connection}
        session={session}
        invocationIds={['inv-1']}
        currentInvocationId="inv-1"
        onClose={() => {}}
      />,
    );
  });
  await act(async () => {});
  expect(JSON.stringify(renderer.toJSON())).toContain('admin_debug');
  await act(async () => renderer.unmount());
});

it('follows a newly accepted current turn while the panel stays open', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  const read = vi.fn(async (_epoch: number, _session: string, id: string) => ({
    ...trace,
    invocationId: id,
  }));
  let renderer!: ReactTestRenderer;
  await act(async () => {
    renderer = create(
      <TracePanel
        services={services(read)}
        connection={connection}
        session={session}
        invocationIds={['inv-old']}
        currentInvocationId="inv-old"
        onClose={() => {}}
      />,
    );
  });
  await act(async () => {});
  expect(read).toHaveBeenLastCalledWith(2, 'session-1', 'inv-old');

  await act(async () => {
    renderer.update(
      <TracePanel
        services={services(read)}
        connection={connection}
        session={session}
        invocationIds={['inv-old', 'inv-new']}
        currentInvocationId="inv-new"
        onClose={() => {}}
      />,
    );
  });
  await act(async () => {});
  expect(read).toHaveBeenLastCalledWith(2, 'session-1', 'inv-new');
  await act(async () => renderer.unmount());
});

it('waits for an accepted turn when no invocation exists', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  let renderer!: ReactTestRenderer;
  await act(async () => {
    renderer = create(
      <TracePanel
        services={services()}
        connection={connection}
        session={session}
        invocationIds={[]}
        onClose={() => {}}
      />,
    );
  });
  await act(async () => {});
  expect(JSON.stringify(renderer.toJSON())).toContain(
    'Waiting for an accepted turn',
  );
  await act(async () => renderer.unmount());
});

it('selects an exactly correlated tool step and falls back to the turn', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  const correlated: TraceVM = {
    ...trace,
    steps: trace.steps.map((step) =>
      step.stepId === 'tool'
        ? { ...step, correlation: { toolId: 'tool-1' } }
        : step,
    ),
  };
  let renderer!: ReactTestRenderer;
  await act(async () => {
    renderer = create(
      <TracePanel
        services={services(async () => correlated)}
        connection={connection}
        session={session}
        invocationIds={['inv-1']}
        currentInvocationId="inv-1"
        focus={{ invocationId: 'inv-1', toolId: 'tool-1' }}
        onClose={() => {}}
      />,
    );
  });
  await act(async () => {});
  let selected = renderer.root.findAll(
    (node) =>
      typeof node.props.className === 'string' &&
      node.props.className.includes('trace-row') &&
      node.props.className.includes('selected'),
  );
  expect(
    selected[0].findAll(
      (node) => node.type === 'strong' && node.children.includes('Read file'),
    ),
  ).toHaveLength(1);
  await act(async () => {
    renderer.update(
      <TracePanel
        services={services(async () => correlated)}
        connection={connection}
        session={session}
        invocationIds={['inv-1']}
        currentInvocationId="inv-1"
        focus={{ invocationId: 'inv-1', toolId: 'missing' }}
        onClose={() => {}}
      />,
    );
  });
  await act(async () => {});
  selected = renderer.root.findAll(
    (node) =>
      typeof node.props.className === 'string' &&
      node.props.className.includes('trace-row') &&
      node.props.className.includes('selected'),
  );
  expect(
    selected[0].findAll(
      (node) => node.type === 'strong' && node.children.includes('Turn'),
    ),
  ).toHaveLength(1);
  await act(async () => renderer.unmount());
});

it('follows an explicit tool focus into a different turn while already open', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  const read = vi.fn(async (_epoch: number, _session: string, id: string) => ({
    ...trace,
    invocationId: id,
  }));
  const service = services(read);
  const props = {
    services: service,
    connection,
    session,
    invocationIds: ['inv-1', 'inv-2'],
    currentInvocationId: 'inv-2',
    onClose() {},
  };
  let renderer!: ReactTestRenderer;
  await act(async () => {
    renderer = create(<TracePanel {...props} />);
  });
  await act(async () => {
    renderer.update(
      <TracePanel
        {...props}
        focus={{ invocationId: 'inv-1', toolId: 'tool-1' }}
      />,
    );
  });
  expect(read).toHaveBeenLastCalledWith(2, 'session-1', 'inv-1');
  await act(async () => renderer.unmount());
});

it('clears old trace details while another turn is loading and ignores late results', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  let resolveSecond!: (value: TraceVM) => void;
  const read = vi.fn(async (_epoch: number, _session: string, id: string) =>
    id === 'inv-2'
      ? new Promise<TraceVM>((resolve) => {
          resolveSecond = resolve;
        })
      : { ...trace, invocationId: id },
  );
  const props = {
    services: services(read),
    connection,
    session,
    invocationIds: ['inv-1', 'inv-2'],
    currentInvocationId: 'inv-1',
    onClose() {},
  };
  let renderer!: ReactTestRenderer;
  await act(async () => {
    renderer = create(<TracePanel {...props} />);
  });
  const turn = () => renderer.root.findAllByType('select')[0];
  await act(async () => turn().props.onChange({ target: { value: 'inv-2' } }));
  expect(JSON.stringify(renderer.toJSON())).not.toContain('Read file');
  await act(async () => turn().props.onChange({ target: { value: 'inv-1' } }));
  await act(async () =>
    resolveSecond({
      ...trace,
      invocationId: 'inv-2',
      steps: [{ ...trace.steps[0], title: 'Stale second turn' }],
    }),
  );
  expect(JSON.stringify(renderer.toJSON())).not.toContain('Stale second turn');
  expect(JSON.stringify(renderer.toJSON())).toContain('Read file');
  await act(async () => renderer.unmount());
});

it('refreshes active traces and stops polling after completion or close', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  vi.useFakeTimers();
  let renderer: ReactTestRenderer | undefined;
  try {
    const read = vi
      .fn()
      .mockResolvedValueOnce({
        ...trace,
        run: { status: 'running' },
        coverage: { active: true },
      })
      .mockResolvedValue(trace);
    await act(async () => {
      renderer = create(
        <TracePanel
          services={services(read)}
          connection={connection}
          session={session}
          invocationIds={['inv-1']}
          onClose={() => {}}
        />,
      );
    });
    expect(read).toHaveBeenCalledTimes(1);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(2000);
    });
    expect(read).toHaveBeenCalledTimes(2);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(6000);
    });
    expect(read).toHaveBeenCalledTimes(2);
    await act(async () => renderer?.unmount());
    renderer = undefined;
    await act(async () => {
      await vi.advanceTimersByTimeAsync(6000);
    });
    expect(read).toHaveBeenCalledTimes(2);
  } finally {
    if (renderer) await act(async () => renderer?.unmount());
    vi.useRealTimers();
  }
});
