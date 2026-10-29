import { messages } from '../src/client/desktop/generated/locales';
import { act, create, type ReactTestRenderer } from 'react-test-renderer';
import { expect, it, vi } from 'vitest';
import { DesktopSchedules } from '../src/client/desktop/Schedules';
import type { DesktopServices } from '../src/client/desktop/models';

it('prompts for Runtime login before loading schedules when business identity is missing', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  const list = vi.fn(async () => []);
  const services = { schedules: { list } } as unknown as DesktopServices;
  let renderer!: ReactTestRenderer;
  await act(async () => {
    renderer = create(
      <DesktopSchedules
        services={services}
        connection={{
          epoch: 4,
          runtimeId: 'r',
          scope: 'scope',
          status: 'connected',
          businessIdentity: 'missing',
        }}
        sessions={[]}
      />,
    );
  });
  expect(list).not.toHaveBeenCalled();
  expect(JSON.stringify(renderer.toJSON())).toContain(
    messages.businessIdentityRequired(),
  );
  await act(async () => renderer.unmount());
});

it('shows the accepted server run immediately while historical indexing is still empty', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  const task = {
    id: 't',
    name: 'Task',
    prompt: 'Prompt',
    agentId: 'agent',
    enabled: true,
    version: '3',
    schedule: { type: 'Once', runAt: '2026-10-26T00:00:00Z' },
    timezone: 'UTC',
  };
  const trigger = vi.fn(async () => ({ id: 'server-run', status: 'Queued' }));
  const services = {
    schedules: {
      list: async () => [task],
      read: async () => ({ task, runs: [], hasMore: false }),
      trigger,
    },
  } as unknown as DesktopServices;
  let renderer!: ReactTestRenderer;
  await act(async () => {
    renderer = create(
      <DesktopSchedules
        services={services}
        connection={{
          epoch: 4,
          runtimeId: 'r',
          scope: 'scope',
          status: 'connected',
        }}
        sessions={[]}
      />,
    );
  });
  await act(async () =>
    renderer.root.findByProps({ className: 'task-row' }).props.onClick(),
  );
  const button = renderer.root
    .findAllByType('button')
    .find((button) =>
      button.children.some(
        (child) =>
          typeof child === 'string' &&
          child.trim() === messages.desktopRunNow(),
      ),
    )!;
  await act(async () => button.props.onClick());
  expect(
    renderer.root.findByProps({ className: 'task-event' }).findByType('strong')
      .children,
  ).toEqual([messages.scheduleQueued()]);
  expect(JSON.stringify(renderer.toJSON())).toContain('server-run');
  expect(trigger).toHaveBeenCalledWith(4, task);
  await act(async () => renderer.unmount());
});
