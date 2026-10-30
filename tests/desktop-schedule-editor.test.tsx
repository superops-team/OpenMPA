import { act, create, type ReactTestRenderer } from 'react-test-renderer';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { ScheduleEditor } from '../src/client/desktop/ScheduleEditor';
import { messages } from '../src/client/desktop/generated/locales';
import type { DesktopServices, ScheduleVM } from '../src/client/desktop/models';
let renderer: ReactTestRenderer;
const initial: ScheduleVM = {
  id: '',
  version: '',
  name: 'Report',
  prompt: 'Generate report',
  agentId: '',
  enabled: true,
  timezone: 'Asia/Shanghai',
  schedule: { type: 'Daily', time: '09:00' },
};
beforeEach(() => vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true));
afterEach(async () => {
  await act(async () => renderer?.unmount());
  vi.unstubAllGlobals();
});
const submit = async () =>
  act(async () =>
    renderer.root.findByType('form').props.onSubmit({ preventDefault() {} }),
  );
async function render(task = initial, services = {} as DesktopServices) {
  const save = vi.fn();
  await act(async () => {
    renderer = create(
      <ScheduleEditor
        task={task}
        services={services}
        epoch={4}
        busy={false}
        error=""
        onSave={save}
        onClose={() => {}}
      />,
    );
  });
  return save;
}
it('validates month dates before submitting a discriminated schedule', async () => {
  const save = await render();
  await act(async () =>
    renderer.root
      .findByProps({ 'aria-label': messages.desktopScheduleType() })
      .props.onChange({ target: { value: 'Monthly' } }),
  );
  const dates = () =>
    renderer.root
      .findAllByType('label')
      .find((node) =>
        node.children.includes(messages.desktopMonthDays1Through31()),
      )!
      .findByType('input');
  await act(async () => dates().props.onChange({ target: { value: '1,1' } }));
  await submit();
  expect(save).not.toHaveBeenCalled();
  expect(JSON.stringify(renderer.toJSON())).toContain(
    messages.scheduleDaysInvalid(),
  );
  await act(async () => dates().props.onChange({ target: { value: '1,15' } }));
  await submit();
  expect(save).toHaveBeenCalledWith(
    expect.objectContaining({
      schedule: { type: 'Monthly', time: '09:00', monthDays: [1, 15] },
    }),
  );
});
it('converts a local datetime to an absolute RFC3339 instant for Once', async () => {
  const save = await render({ ...initial, schedule: { type: 'Once' } });
  const date = new Date(Date.now() + 86400000);
  const input = new Date(date.getTime() - date.getTimezoneOffset() * 60000)
    .toISOString()
    .slice(0, 16);
  await act(async () =>
    renderer.root
      .findByProps({ type: 'datetime-local' })
      .props.onChange({ target: { value: input } }),
  );
  await submit();
  expect(save).toHaveBeenCalledWith(
    expect.objectContaining({
      schedule: { type: 'Once', runAt: new Date(input).toISOString() },
    }),
  );
});
it('retains interval anchors and rejects invalid IANA time zones', async () => {
  const save = await render({
    ...initial,
    timezone: 'Not/AZone',
    schedule: {
      type: 'Interval',
      intervalSeconds: 3600,
      anchorAt: '2026-10-29T00:00:00Z',
    },
  });
  await submit();
  expect(save).not.toHaveBeenCalled();
  expect(JSON.stringify(renderer.toJSON())).toContain(
    messages.scheduleTimezoneInvalid(),
  );
  await act(async () =>
    renderer.root
      .findAllByType('input')
      .find((node) => node.props.value === 'Not/AZone')!
      .props.onChange({ target: { value: 'Asia/Shanghai' } }),
  );
  await submit();
  expect(save).toHaveBeenCalledWith(
    expect.objectContaining({
      schedule: {
        type: 'Interval',
        intervalSeconds: 3600,
        anchorAt: '2026-10-29T00:00:00Z',
      },
    }),
  );
});
it('clears the previous target when switching Feishu bots and reads targets for the chosen bot', async () => {
  const deliveryTargets = vi.fn(
    async (_epoch: number, _channel: string, appId: string) => [
      {
        appId,
        targetId: `chat-${appId}`,
        name: `Group ${appId}`,
        channel: 'Feishu',
      },
    ],
  );
  const services = {
    channels: {
      list: async () => [
        { id: 'botA', name: 'A', type: 'feishu', enabled: true },
        { id: 'botB', name: 'B', type: 'feishu', enabled: true },
      ],
    },
    schedules: { deliveryTargets },
  } as unknown as DesktopServices;
  const save = await render(
    {
      ...initial,
      delivery: {
        channel: 'Feishu',
        appId: 'botA',
        targetId: 'chat-botA',
        receiveIdType: 'chat_id',
        bestEffort: true,
      },
    },
    services,
  );
  await act(async () =>
    renderer.root
      .findByProps({ 'aria-label': messages.scheduleBot() })
      .props.onChange({ target: { value: 'botB' } }),
  );
  expect(deliveryTargets).toHaveBeenLastCalledWith(4, 'Feishu', 'botB');
  expect(
    renderer.root.findByProps({
      'aria-label': messages.desktopDeliveryTarget(),
    }).props.value,
  ).toBe('');
  await submit();
  expect(save).not.toHaveBeenCalled();
  await act(async () =>
    renderer.root
      .findByProps({ 'aria-label': messages.desktopDeliveryTarget() })
      .props.onChange({ target: { value: 'chat-botB' } }),
  );
  await submit();
  expect(save).toHaveBeenCalledWith(
    expect.objectContaining({
      delivery: expect.objectContaining({
        appId: 'botB',
        targetId: 'chat-botB',
      }),
    }),
  );
});
