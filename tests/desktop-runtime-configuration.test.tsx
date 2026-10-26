import { act, create, type ReactTestRenderer } from 'react-test-renderer';
import { expect, it, vi } from 'vitest';
const api = vi.hoisted(() => ({
  read: vi.fn(),
  update: vi.fn(),
  readUpdate: vi.fn(),
}));
vi.mock('../src/client/desktop/generated/mygo', () => ({ RuntimeConfig: api }));
import { RuntimeConfiguration } from '../src/client/desktop/RuntimeConfiguration';

it('reads the selected Runtime and submits reviewed resource settings with one original operation', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  vi.stubGlobal('window', { confirm: vi.fn(() => true) });
  const persisted = new Map<string, string>();
  vi.stubGlobal('localStorage', {
    getItem: (key: string) => persisted.get(key),
    setItem: (key: string, value: string) => persisted.set(key, value),
  });
  const values = {
    description: 'Test',
    cpuMilli: 2000,
    memoryMb: 4096,
    minInstance: 1,
    maxInstance: 20,
    maxConcurrency: 100,
  };
  api.read.mockResolvedValue({
    ok: true,
    value: {
      runtimeId: 'r',
      version: 29,
      status: 'Ready',
      values,
      fingerprint: 'reviewed',
    },
  });
  api.update.mockImplementation(async (input) => ({
    ok: true,
    value: { operationId: input.operationId, state: 'accepted' },
  }));
  api.readUpdate.mockImplementation(async (input) => ({
    ok: true,
    value: { operationId: input.operationId, state: 'ready' },
  }));
  let renderer!: ReactTestRenderer;
  await act(async () => {
    renderer = create(
      <RuntimeConfiguration
        connection={{ epoch: 3, scope: 'a', runtimeId: 'r', status: 'Ready' }}
        busy={false}
        run={async (fn) => fn()}
      />,
    );
  });
  expect(api.read).not.toHaveBeenCalled();
  await act(async () => {
    renderer.root
      .findAllByType('button')
      .find((button) => button.children.includes('Read Runtime configuration'))!
      .props.onClick();
  });
  expect(api.read).toHaveBeenCalledWith({ connectionEpoch: 3 });
  expect(
    renderer.root
      .findAllByType('input')
      .some((input) => input.props.value === 2000),
  ).toBe(true);
  await act(async () => {
    renderer.root.findByType('form').props.onSubmit({ preventDefault() {} });
  });
  const request = api.update.mock.calls[0][0];
  expect(request).toEqual({
    connectionEpoch: 3,
    operationId: expect.any(String),
    fingerprint: 'reviewed',
    values,
    confirm: true,
  });
  expect(api.update).toHaveBeenCalledTimes(1);
  await act(async () => {
    renderer.root
      .findAllByType('button')
      .find((button) =>
        button.children.includes('Verify original configuration update'),
      )!
      .props.onClick();
  });
  expect(api.readUpdate).toHaveBeenCalledWith({
    connectionEpoch: 3,
    operationId: request.operationId,
  });
  expect(api.update).toHaveBeenCalledTimes(1);
  expect(JSON.stringify(renderer.toJSON())).toContain('new Ready version');
  await act(async () => renderer.unmount());
});

it('cancelling review does not submit a Runtime update', async () => {
  api.update.mockClear();
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  vi.stubGlobal('window', { confirm: () => false });
  let renderer!: ReactTestRenderer;
  await act(async () => {
    renderer = create(
      <RuntimeConfiguration
        connection={{ epoch: 3, scope: 'a', runtimeId: 'r', status: 'Ready' }}
        busy={false}
        run={async (fn) => fn()}
      />,
    );
  });
  await act(async () => {
    renderer.root
      .findAllByType('button')
      .find((button) => button.children.includes('Read Runtime configuration'))!
      .props.onClick();
  });
  await act(async () => {
    renderer.root.findByType('form').props.onSubmit({ preventDefault() {} });
  });
  expect(api.update).not.toHaveBeenCalled();
  await act(async () => renderer.unmount());
});
