import { act, create, type ReactTestRenderer } from 'react-test-renderer';
import { expect, it, vi } from 'vitest';
import { DesktopSettings } from '../src/client/desktop/Settings';
import type { DesktopServices, RuntimeVM } from '../src/client/desktop/models';

it('loads the signed-in account directory and rejects old region results before binding', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  let resolveOld!: (value: RuntimeVM[]) => void;
  const list = vi
    .fn()
    .mockImplementationOnce(
      () =>
        new Promise<RuntimeVM[]>((resolve) => {
          resolveOld = resolve;
        }),
    )
    .mockResolvedValue([
      { id: 'r-new', name: 'New region MPA', status: 'Ready', version: '1' },
    ]);
  const bind = vi.fn().mockResolvedValue({
    epoch: 3,
    scope: 'new',
    runtimeId: 'r-new',
    status: 'Ready',
  });
  const services = {
    platform: {
      status: async () => ({
        signedIn: true,
        accountId: '1234567890',
        region: 'cn-beijing',
      }),
      profiles: async () => [],
      list,
      bind,
    },
  } as unknown as DesktopServices;
  const onConnection = vi.fn();
  let renderer!: ReactTestRenderer;
  await act(async () => {
    renderer = create(
      <DesktopSettings services={services} onConnection={onConnection} />,
    );
  });
  expect(list).toHaveBeenCalledWith('cn-beijing', 'default');
  await act(async () => {
    renderer.root
      .findAllByType('input')
      .find((input) => input.props.value === 'cn-beijing')!
      .props.onChange({ target: { value: 'cn-shanghai' } });
  });
  expect(list).toHaveBeenLastCalledWith('cn-shanghai', 'default');
  await act(async () => {
    resolveOld([
      { id: 'r-old', name: 'Old region MPA', status: 'Ready', version: '1' },
    ]);
  });
  expect(JSON.stringify(renderer.toJSON())).not.toContain('Old region MPA');
  await act(async () => {
    renderer.root
      .findAllByType('button')
      .find((button) => button.children.includes('Use Runtime'))!
      .props.onClick();
  });
  expect(bind).toHaveBeenCalledWith('cn-shanghai', 'default', 'r-new');
  expect(onConnection).toHaveBeenCalledWith(
    expect.objectContaining({ runtimeId: 'r-new', epoch: 3 }),
  );
  await act(async () => renderer.unmount());
});
