import { runtimeModels } from './fixtures/runtime-models';
import { act, create, type ReactTestRenderer } from 'react-test-renderer';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { DesktopChat } from '../src/client/desktop/Chat';
import { ComposerModelPicker } from '../src/client/desktop/ComposerModelPicker';
import { LocaleProvider } from '../src/client/desktop/locale';
import { messagesIn } from '../src/client/desktop/generated/locales';
import { TracePanel } from '../src/client/desktop/TracePanel';
import { TraceStepDetail } from '../src/client/desktop/TraceStepDetail';
import { TurnProcess } from '../src/client/desktop/TurnProcess';
import { useComposerModel } from '../src/client/desktop/use-composer-model';
import type { ConversationTurn } from '../src/client/desktop/turn-model';
import type {
  DesktopServices,
  ExecutionConfigVM,
  TraceVM,
} from '../src/client/desktop/models';

const connection = {
  epoch: 2,
  scope: 'scope',
  runtimeId: 'r-1',
  status: 'connected',
};
const session = { id: 's', title: 'Remote title: Working' };
const config: ExecutionConfigVM = {
  executionConfigVersion: '7',
  etag: '"rev-7"',
  model: 'deepseek-v4-pro',
  modelMode: 'inherit',
  mcpMode: 'inherit',
  mcpServers: [],
  profileBound: true,
  invalidRefsCount: 0,
};
const zh = messagesIn('zh-Hans');
const en = messagesIn('en');
let language = 'zh-Hans';
let changed: ((payload: { locale: string; dir: string }) => void) | undefined;
let renderer: ReactTestRenderer | undefined;

beforeEach(() => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  language = 'zh-Hans';
  changed = undefined;
  vi.stubGlobal('mygo', {
    get locale() {
      return language;
    },
    on: (_name: string, listener: typeof changed) => {
      changed = listener;
      return () => {};
    },
  });
});

afterEach(async () => {
  if (renderer) await act(async () => renderer?.unmount());
  renderer = undefined;
  vi.unstubAllGlobals();
});

async function mount(children: React.ReactNode) {
  await act(async () => {
    renderer = create(<LocaleProvider>{children}</LocaleProvider>);
  });
}

async function switchLanguage(next = 'en') {
  await act(async () => {
    language = next;
    changed?.({ locale: language, dir: 'ltr' });
  });
}

function output() {
  return JSON.stringify(renderer?.toJSON());
}

function chatServices() {
  return {
    sessions: {
      models: async () => runtimeModels,
      read: vi.fn(async () => ({ session, messages: [] })),
      pending: vi.fn(async () => []),
      executionConfig: vi.fn(async () => config),
      updateExecutionConfig: vi.fn(async () => ({
        ...config,
        executionConfigVersion: '8',
        etag: '"rev-8"',
      })),
      submit: vi.fn(async () => {
        throw null;
      }),
    },
  } as unknown as DesktopServices;
}

it.each(['running', 'completed', 'failed', 'stopped'] as const)(
  'redraws the %s process title and tool states without translating remote content',
  async (status) => {
    const turn: ConversationTurn = {
      invocationId: 'inv-1',
      status,
      segments: [
        {
          type: 'reasoning',
          id: 'reasoning',
          content: 'Remote reasoning: Completed',
          visibility: 'detail',
          partial: false,
        },
        ...(
          ['preparing', 'running', 'success', 'failed', 'stopped'] as const
        ).map((toolStatus, index) => ({
          type: 'tool' as const,
          id: `tool-${index}`,
          toolId: `tool-${index}`,
          name: index === 0 ? 'read_file' : 'Remote tool: Read',
          subject: 'remote/Working.ts',
          status: toolStatus,
        })),
        {
          type: 'text',
          id: 'progress',
          role: 'assistant',
          content: 'Remote progress: Running',
          partial: false,
        },
        {
          type: 'text',
          id: 'answer',
          role: 'assistant',
          content: 'Remote final answer',
          partial: false,
        },
      ],
    };
    const titles = {
      running: zh.chatProcessRunning(7),
      completed: zh.chatProcessCompleted(7),
      failed: zh.chatProcessFailed(7),
      stopped: zh.chatProcessStopped(7),
    };
    await mount(<TurnProcess turn={turn} onInspectTool={() => {}} />);
    if (status !== 'running')
      await act(async () => {
        renderer!.root.findByType('button').props.onClick();
      });
    expect(output()).toContain(titles[status]);
    expect(
      renderer!.root.findByProps({
        'aria-label': zh.collapseSection(zh.desktopExecutionProcess()),
      }).props['aria-expanded'],
    ).toBe(true);
    for (const label of [
      zh.desktopRead(),
      zh.desktopPreparing(),
      zh.desktopRunning(),
      zh.desktopCompleted(),
      zh.desktopFailed(),
      zh.desktopStopped(),
      zh.desktopProgress(),
    ])
      expect(output()).toContain(label);
    expect(output()).toContain(zh.chatInspectTool(zh.desktopRead()));
    await switchLanguage();
    expect(output()).toContain(en.desktopPreparing());
    expect(
      renderer!.root.findByProps({
        'aria-label': en.collapseSection(en.desktopExecutionProcess()),
      }).props['aria-expanded'],
    ).toBe(true);
    expect(output()).toContain(en.chatInspectTool(en.desktopRead()));
    expect(output()).toContain('Remote tool: Read');
    expect(output()).toContain('Remote reasoning: Completed');
    expect(output()).toContain('Remote progress: Running');
    expect(output()).toContain('remote/Working.ts');
    const englishTitles = {
      running: en.chatProcessRunning(7),
      completed: en.chatProcessCompleted(7),
      failed: en.chatProcessFailed(7),
      stopped: en.chatProcessStopped(7),
    };
    expect(output()).toContain(englishTitles[status]);
    await switchLanguage('zh-Hans');
    expect(output()).toContain(titles[status]);
    expect(output()).toContain('Remote tool: Read');
    expect(
      renderer!.root.findByProps({
        'aria-label': zh.collapseSection(zh.desktopExecutionProcess()),
      }).props['aria-expanded'],
    ).toBe(true);
  },
);

it('keeps the model filter and IDs while redrawing static and parameterized labels', async () => {
  const apply = vi.fn(async () => true);
  await mount(
    <ComposerModelPicker
      catalog={runtimeModels}
      config={{ ...config, model: 'remote-invalid-model' }}
      busy={false}
      onApply={apply}
    />,
  );
  await act(async () =>
    renderer!.root
      .findByProps({ 'aria-label': zh.chatModelLabel('remote-invalid-model') })
      .props.onClick(),
  );
  expect(output()).toContain(
    zh.chatCurrentModelInvalid('remote-invalid-model'),
  );
  await act(async () =>
    renderer!.root
      .findByProps({ 'aria-label': zh.desktopFilterModels() })
      .props.onChange({ target: { value: 'deepseek-v4' } }),
  );
  await switchLanguage();
  expect(
    renderer!.root.findByProps({ 'aria-label': en.desktopFilterModels() }).props
      .value,
  ).toBe('deepseek-v4');
  expect(output()).toContain(
    en.chatCurrentModelInvalid('remote-invalid-model'),
  );
  expect(output()).toContain('deepseek-v4-pro');
  expect(output()).not.toContain('M context window');
  await switchLanguage('zh-Hans');
  expect(output()).toContain(
    zh.chatCurrentModelInvalid('remote-invalid-model'),
  );
  expect(
    renderer!.root.findByProps({ 'aria-label': zh.desktopFilterModels() }).props
      .value,
  ).toBe('deepseek-v4');
  expect(apply).not.toHaveBeenCalled();
});

function ModelHarness({ services }: { services: DesktopServices }) {
  const state = useComposerModel({ services, connection, session });
  return (
    <div>
      <p role="alert">{state.error}</p>
      <p role="status">{state.notice}</p>
      <button onClick={() => void state.apply('inherit')}>apply</button>
      <button onClick={() => void state.apply('replace', 'remote-model')}>
        invalid
      </button>
    </div>
  );
}

it('redraws a saved model notice and local validation error without another request', async () => {
  const services = chatServices();
  await mount(<ModelHarness services={services} />);
  await act(async () =>
    renderer!.root.findAllByType('button')[0].props.onClick(),
  );
  expect(output()).toContain(
    zh.desktopModelAppliesToFutureTurnsExistingTurnsKeepTheir(),
  );
  await act(async () =>
    renderer!.root.findAllByType('button')[1].props.onClick(),
  );
  expect(output()).toContain(zh.chatModelInvalid('remote-model'));
  await switchLanguage();
  expect(output()).toContain(
    en.desktopModelAppliesToFutureTurnsExistingTurnsKeepTheir(),
  );
  expect(output()).toContain(en.chatModelInvalid('remote-model'));
  await switchLanguage('zh-Hans');
  expect(output()).toContain(
    zh.desktopModelAppliesToFutureTurnsExistingTurnsKeepTheir(),
  );
  expect(output()).toContain(zh.chatModelInvalid('remote-model'));
  expect(services.sessions.executionConfig).toHaveBeenCalledTimes(1);
  expect(services.sessions.updateExecutionConfig).toHaveBeenCalledTimes(1);
});

it.each([
  [null, 'fallback'],
  [new Error('Remote model failure: Failed'), 'remote'],
] as const)(
  'redraws model load errors while preserving remote Error messages (%s)',
  async (failure, kind) => {
    const services = chatServices();
    services.sessions.executionConfig = vi.fn(async () => {
      throw failure;
    });
    await mount(<ModelHarness services={services} />);
    const expected =
      kind === 'remote'
        ? 'Remote model failure: Failed'
        : zh.desktopCouldNotUpdateTheConversationModel();
    expect(output()).toContain(expected);
    await switchLanguage();
    expect(output()).toContain(
      kind === 'remote'
        ? expected
        : en.desktopCouldNotUpdateTheConversationModel(),
    );
    await switchLanguage('zh-Hans');
    expect(output()).toContain(expected);
    expect(services.sessions.executionConfig).toHaveBeenCalledTimes(1);
  },
);

it('retains the chat draft and redraws a local submit error without reloading history', async () => {
  const services = chatServices();
  await mount(
    <DesktopChat
      services={services}
      connection={connection}
      session={session}
      onSchedule={() => {}}
    />,
  );
  await act(async () =>
    renderer!.root.findByType('textarea').props.onChange({
      target: { value: 'Remote draft: Working' },
    }),
  );
  await act(async () =>
    renderer!.root.findByType('form').props.onSubmit({ preventDefault() {} }),
  );
  expect(output()).toContain(zh.desktopCouldNotSubmitMessage());
  expect(renderer!.root.findByType('textarea').props.value).toBe(
    'Remote draft: Working',
  );
  await switchLanguage();
  expect(output()).toContain(en.desktopCouldNotSubmitMessage());
  expect(renderer!.root.findByType('textarea').props.value).toBe(
    'Remote draft: Working',
  );
  expect(output()).toContain(session.title);
  await switchLanguage('zh-Hans');
  expect(output()).toContain(zh.desktopCouldNotSubmitMessage());
  expect(renderer!.root.findByType('textarea').props.value).toBe(
    'Remote draft: Working',
  );
  expect(services.sessions.read).toHaveBeenCalledTimes(1);
  expect(services.sessions.executionConfig).toHaveBeenCalledTimes(1);
  expect(services.sessions.submit).toHaveBeenCalledTimes(1);
});

it('redraws an initial history failure without retrying bootstrap', async () => {
  const services = chatServices();
  vi.mocked(services.sessions.read).mockRejectedValue(null);
  await mount(
    <DesktopChat
      services={services}
      connection={connection}
      session={session}
      onSchedule={() => {}}
    />,
  );
  expect(output()).toContain(zh.desktopCouldNotLoadConversation());
  await switchLanguage();
  expect(output()).toContain(en.desktopCouldNotLoadConversation());
  await switchLanguage('zh-Hans');
  expect(output()).toContain(zh.desktopCouldNotLoadConversation());
  expect(services.sessions.read).toHaveBeenCalledOnce();
});

const trace: TraceVM = {
  traceVersion: 'trace-api-v1',
  sessionId: 's',
  invocationId: 'inv-1',
  run: { status: 'success', wallDurationMs: 100 },
  coverage: { complete: false, recordCount: 2, returnedCount: 2 },
  steps: [
    {
      stepId: 'root',
      sequence: 1,
      kind: 'run.total',
      title: 'Remote title: Turn',
      status: 'success',
      startedAt: '',
    },
    {
      stepId: 'tool-id',
      parentStepId: 'root',
      sequence: 2,
      kind: 'tool.call',
      category: 'tool',
      title: 'Remote title: Read file',
      status: 'not_observed',
      startedAt: '',
      details: { message: 'Remote message: Unavailable', apiType: 'tool.call' },
    },
  ],
};

it.each([
  ['skipped', zh.statusSkipped(), en.statusSkipped()],
  ['denied', zh.statusDenied(), en.statusDenied()],
  ['future_remote_status', 'future_remote_status', 'future_remote_status'],
  ['toString', 'toString', 'toString'],
])(
  'preserves unknown trace states and localizes known states (%s)',
  async (status, chinese, english) => {
    await mount(
      <TraceStepDetail
        step={{
          ...trace.steps[1],
          status,
          category: 'remote_category',
          children: [],
          depth: 0,
          duration: 65000,
        }}
        onClose={() => {}}
      />,
    );
    expect(output()).toContain(chinese);
    expect(output()).toContain(zh.durationMinutesSeconds('1', '5'));
    expect(output()).toContain('remote_category');
    expect(output()).toContain('Remote title: Read file');
    await switchLanguage();
    expect(output()).toContain(english);
    expect(output()).toContain(en.durationMinutesSeconds('1', '5'));
    await switchLanguage('zh-Hans');
    expect(output()).toContain(chinese);
    expect(output()).toContain(zh.durationMinutesSeconds('1', '5'));
  },
);

it('redraws trace categories, statuses and unavailable metrics while preserving filters and remote titles', async () => {
  const read = vi.fn(async () => trace);
  await mount(
    <TracePanel
      services={{ trace: { read } } as unknown as DesktopServices}
      connection={connection}
      session={session}
      invocationIds={['inv-1']}
      onClose={() => {}}
    />,
  );
  expect(output()).toContain(zh.tracePartial(2));
  expect(output()).toContain(zh.valueUnavailable());
  expect(output()).toContain(zh.traceCategoryTerminal());
  expect(output()).toContain(zh.desktopNotObserved());
  await act(async () =>
    renderer!.root
      .findByProps({ 'aria-label': zh.desktopFilterTraceCategory() })
      .props.onChange({ target: { value: 'tool' } }),
  );
  await act(async () =>
    renderer!.root
      .findByProps({ 'aria-label': zh.desktopSearchTrace() })
      .props.onChange({ target: { value: 'Read file' } }),
  );
  const row = renderer!.root
    .findAll(
      (node) =>
        node.type === 'button' &&
        String(node.props.className).split(/\s+/).includes('trace-row'),
    )
    .find((node) =>
      JSON.stringify(node.findAllByType('strong')[0]?.children).includes(
        'Read file',
      ),
    )!;
  await act(async () => row.props.onClick());
  expect(output()).toContain('Remote message: Unavailable');
  await switchLanguage();
  expect(output()).toContain(en.tracePartial(2));
  expect(output()).toContain(en.valueUnavailable());
  expect(output()).toContain(en.desktopNotObserved());
  expect(output()).toContain('Remote title: Read file');
  expect(output()).toContain('Remote message: Unavailable');
  expect(output()).toContain('tool.call');
  expect(output()).toContain('tool-id');
  expect(
    renderer!.root.findByProps({
      'aria-label': en.desktopFilterTraceCategory(),
    }).props.value,
  ).toBe('tool');
  expect(
    renderer!.root.findByProps({
      'aria-label': en.desktopSearchTrace(),
    }).props.value,
  ).toBe('Read file');
  await switchLanguage('zh-Hans');
  expect(output()).toContain(zh.valueUnavailable());
  expect(output()).toContain(zh.desktopNotObserved());
  expect(
    renderer!.root.findByProps({ 'aria-label': zh.desktopSearchTrace() }).props
      .value,
  ).toBe('Read file');
  expect(read).toHaveBeenCalledTimes(1);
});

it.each([
  [
    null,
    zh.desktopCouldNotLoadThisTraceRetryAfterTheTurn(),
    en.desktopCouldNotLoadThisTraceRetryAfterTheTurn(),
  ],
  [
    { httpStatus: 403 },
    zh.desktopTheCurrentRuntimeIdentityIsNotAllowedToRead(),
    en.desktopTheCurrentRuntimeIdentityIsNotAllowedToRead(),
  ],
  [
    { httpStatus: 404 },
    zh.desktopTraceIsUnavailableForThisTurnItMayNot(),
    en.desktopTraceIsUnavailableForThisTurnItMayNot(),
  ],
  [
    new Error('Remote trace error: Unavailable'),
    'Remote trace error: Unavailable',
    'Remote trace error: Unavailable',
  ],
])(
  'redraws trace load errors without refetching (%s)',
  async (failure, chinese, english) => {
    const read = vi.fn(async () => {
      throw failure;
    });
    await mount(
      <TracePanel
        services={{ trace: { read } } as unknown as DesktopServices}
        connection={connection}
        session={session}
        invocationIds={['inv-1']}
        onClose={() => {}}
      />,
    );
    expect(output()).toContain(chinese);
    await switchLanguage();
    expect(output()).toContain(english);
    await switchLanguage('zh-Hans');
    expect(output()).toContain(chinese);
    expect(read).toHaveBeenCalledTimes(1);
  },
);
