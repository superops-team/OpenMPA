import { act, create, type ReactTestRenderer } from 'react-test-renderer';
import { expect, it, vi } from 'vitest';
import { DesktopSettings } from '../src/client/desktop/Settings';
import type { DesktopServices } from '../src/client/desktop/models';
it('discovers the original image operation on the same page after an unknown update and reads it without updating again', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  let submitted = false;
  const runtime = vi.fn(async () => {
    submitted = true;
    throw Object.assign(new Error('Outcome unknown'), {
      code: 'outcomeUnknown',
    });
  });
  const updates = vi.fn(async () =>
    submitted
      ? [{ operationId: 'original-image-operation', status: 'uncertain' }]
      : [],
  );
  const read = vi.fn(async () => 'Accepted; still verifying');
  const services = {
    platform: {
      status: async () => ({
        signedIn: true,
        accountId: 'account',
        region: 'cn-beijing',
      }),
      profiles: async () => [],
    },
    deployment: {
      images: async () => ({
        runtimeId: 'r',
        runtimeVersion: '3',
        runtimeImage: 'runtime:next',
        toolId: 't',
        toolVersion: 'timestamp',
        toolImage: 'worker:next',
      }),
      runtime,
      updates,
      read,
    },
  } as unknown as DesktopServices;
  let renderer!: ReactTestRenderer;
  await act(async () => {
    renderer = create(
      <DesktopSettings
        services={services}
        connection={{
          epoch: 4,
          runtimeId: 'r',
          scope: 'scope',
          status: 'connected',
        }}
        onConnection={() => {}}
      />,
    );
  });
  const button = (text: string) =>
    renderer.root
      .findAllByType('button')
      .find((button) => button.children.includes(text))!;
  await act(async () => button('Read current images').props.onClick());
  await act(async () => button('Review exact targets').props.onClick());
  await act(async () => button('Apply Runtime image').props.onClick());
  expect(JSON.stringify(renderer.toJSON())).toContain(
    'original-image-operation',
  );
  await act(async () =>
    button('Read current status of this operation').props.onClick(),
  );
  expect(read).toHaveBeenCalledWith('r', 'original-image-operation');
  expect(runtime).toHaveBeenCalledOnce();
  expect(button('Refresh saved receipts')).toBeDefined();
  await act(async () => renderer.unmount());
});
