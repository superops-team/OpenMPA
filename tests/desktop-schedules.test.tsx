import { act, create, type ReactTestRenderer } from 'react-test-renderer';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { DesktopSchedules } from '../src/client/desktop/Schedules';
import { messages } from '../src/client/desktop/generated/locales';
import type { DesktopServices, ScheduleVM } from '../src/client/desktop/models';
const connection = {
  epoch: 4,
  runtimeId: 'r1',
  runtimeName: 'My MPA',
  scope: 'user/r1',
  status: 'connected',
};
const task: ScheduleVM = {
  id: 't1',
  name: 'Existing chat task',
  prompt: 'Original prompt',
  agentId: 'main',
  enabled: true,
  version: '2',
  schedule: { type: 'Daily', time: '09:00' },
  timezone: 'Asia/Shanghai',
  nextRun: '2026-10-30T01:00:00Z',
  status: 'Succeeded',
  lastRun: '2026-10-29T01:00:00Z',
};
let renderer: ReactTestRenderer;
beforeEach(() => vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true));
afterEach(async () => {
  if (renderer) await act(async () => renderer.unmount());
  vi.unstubAllGlobals();
});
const text = () => JSON.stringify(renderer.toJSON());
const button = (label: string) =>
  renderer.root
    .findAllByType('button')
    .find((button) =>
      button.children.some(
        (child) => typeof child === 'string' && child.trim() === label,
      ),
    )!;
const deferred = <T,>() => {
  let resolve!: (value: T) => void;
  let reject!: (value: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
};
async function render(schedules: Record<string, unknown>, conn = connection) {
  const services = { schedules } as unknown as DesktopServices;
  await act(async () => {
    renderer = create(
      <DesktopSchedules services={services} connection={conn} sessions={[]} />,
    );
  });
  return services;
}
it('shows existing Runtime tasks, latest result, next run and authoritative statistics', async () => {
  const list = vi.fn(async () => ({
    items: [task],
    overview: { executionCount: 12, successRate: 0.75 },
  }));
  await render({ list });
  expect(text()).toContain('Existing chat task');
  expect(text()).toContain(messages.desktopSuccess());
  expect(text()).toContain('75%');
  expect(renderer.root.findAllByProps({ className: 'task-row' })).toHaveLength(
    1,
  );
  await act(async () =>
    renderer.root
      .findByProps({ 'aria-label': messages.desktopSearchTasks() })
      .props.onChange({ target: { value: 'not present' } }),
  );
  expect(text()).toContain(messages.scheduleNoMatches());
  expect(list).toHaveBeenCalledWith(4);
});
it('does not confuse a failed list request with an empty task list, and allows refresh', async () => {
  const list = vi
    .fn()
    .mockRejectedValueOnce(
      Object.assign(new Error('Identity missing'), {
        code: 'businessIdentityRequired',
      }),
    )
    .mockResolvedValueOnce({ items: [task] });
  await render({ list });
  expect(text()).toContain(messages.scheduleIdentityHint());
  expect(text()).not.toContain(messages.scheduleEmptyTitle());
  await act(async () =>
    renderer.root
      .findByProps({ 'aria-label': messages.desktopRefresh() })
      .props.onClick(),
  );
  expect(text()).toContain(task.name);
  expect(renderer.root.findAllByProps({ role: 'alert' })).toHaveLength(0);
});
it('creates a server task once and disables inputs until the receipt arrives', async () => {
  const pending = deferred<ScheduleVM>();
  const createTask = vi.fn(() => pending.promise);
  const list = vi
    .fn()
    .mockResolvedValueOnce({ items: [] })
    .mockResolvedValue({ items: [task] });
  await render({ list, create: createTask });
  await act(async () =>
    button(messages.desktopNewScheduledTask()).props.onClick(),
  );
  const inputFor = (label: string, type: 'input' | 'textarea') =>
    renderer.root
      .findAllByType('label')
      .find((node) => node.children.some((child) => child === label))!
      .findByType(type);
  await act(async () =>
    inputFor(messages.desktopName(), 'input').props.onChange({
      target: { value: 'New task' },
    }),
  );
  await act(async () =>
    inputFor(messages.desktopPrompt(), 'textarea').props.onChange({
      target: { value: 'Generate a report' },
    }),
  );
  const submit = () =>
    renderer.root.findByType('form').props.onSubmit({ preventDefault() {} });
  await act(async () => {
    submit();
    submit();
  });
  expect(createTask).toHaveBeenCalledOnce();
  expect(createTask).toHaveBeenCalledWith(
    4,
    expect.objectContaining({
      name: 'New task',
      prompt: 'Generate a report',
      schedule: { type: 'Daily', time: '09:00' },
      enabled: true,
      delivery: { channel: 'Web', receiveIdType: 'chat_id', bestEffort: true },
    }),
  );
  expect(renderer.root.findByType('fieldset').props.disabled).toBe(true);
  await act(async () => pending.resolve({ ...task, name: 'New task' }));
  expect(renderer.root.findAllByProps({ role: 'dialog' })).toHaveLength(0);
  expect(text()).toContain(messages.scheduleSaved());
  expect(list).toHaveBeenCalledTimes(2);
});
it('keeps the editor and original draft when creation fails', async () => {
  await render({
    list: async () => ({ items: [task] }),
    create: async () => {
      throw new Error('Rejected');
    },
  });
  await act(async () =>
    button(messages.desktopNewScheduledTask()).props.onClick(),
  );
  const labels = () => renderer.root.findAllByType('label');
  await act(async () =>
    labels()
      .find((node) => node.children.includes(messages.desktopName()))!
      .findByType('input')
      .props.onChange({ target: { value: 'Keep name' } }),
  );
  await act(async () =>
    renderer.root
      .findByType('textarea')
      .props.onChange({ target: { value: 'Keep prompt' } }),
  );
  await act(async () =>
    renderer.root.findByType('form').props.onSubmit({ preventDefault() {} }),
  );
  expect(text()).toContain('Rejected');
  expect(renderer.root.findByType('textarea').props.value).toBe('Keep prompt');
});
it('clears tasks and editing state on a Runtime switch and ignores late reads', async () => {
  const first = deferred<{ items: ScheduleVM[] }>();
  const list = vi
    .fn()
    .mockReturnValueOnce(first.promise)
    .mockResolvedValue({
      items: [{ ...task, id: 'other', name: 'Other Runtime task' }],
    });
  const services = await render({ list });
  expect(text()).toContain(messages.scheduleLoading());
  await act(async () =>
    renderer.update(
      <DesktopSchedules
        services={services}
        connection={{
          ...connection,
          epoch: 5,
          runtimeId: 'r2',
          scope: 'user/r2',
        }}
        sessions={[]}
      />,
    ),
  );
  await act(async () => first.resolve({ items: [task] }));
  expect(text()).toContain('Other Runtime task');
  expect(text()).not.toContain('Existing chat task');
});
it('copies a task without its old ID or interval anchor', async () => {
  const copied = vi.fn(async (_epoch: number, value: ScheduleVM) => ({
    ...value,
    id: 'copy',
    version: '1',
  }));
  await render({
    create: copied,
    list: async () => ({
      items: [
        {
          ...task,
          schedule: {
            type: 'Interval',
            intervalSeconds: 3600,
            anchorAt: '2026-10-29T00:00:00Z',
          },
        },
      ],
    }),
  });
  await act(async () => button(messages.scheduleCopy()).props.onClick());
  expect(
    renderer.root
      .findAllByType('input')
      .some(
        (node) => node.props.value === messages.scheduleCopyName(task.name),
      ),
  ).toBe(true);
  await act(async () =>
    renderer.root.findByType('form').props.onSubmit({ preventDefault() {} }),
  );
  expect(copied).toHaveBeenCalledWith(
    4,
    expect.objectContaining({
      id: '',
      version: '',
      schedule: {
        type: 'Interval',
        intervalSeconds: 3600,
        anchorAt: undefined,
      },
    }),
  );
});
