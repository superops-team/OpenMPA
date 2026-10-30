import { act, create, type ReactTestRenderer } from 'react-test-renderer';
import { expect, it, vi } from 'vitest';
import { DesktopApp } from '../src/client/desktop/DesktopApp';
import type { DesktopServices } from '../src/client/desktop/models';
it('blocks native close if the recovery-draft durability barrier fails', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  let request!: (requestId: number) => void;
  const resolveClose = vi.fn();
  const services = {
    workspace: {
      spaces: async () => [],
      flush: async () => {
        throw new Error('Recovery store unavailable');
      },
    },
    connection: { status: async () => undefined },
    host: {
      onClose: (listener: typeof request) => {
        request = listener;
        return () => {};
      },
      closeState: async () => ({ pending: false, requestId: 0 }),
      resolveClose,
    },
  } as unknown as DesktopServices;
  let renderer!: ReactTestRenderer;
  await act(async () => {
    renderer = create(<DesktopApp services={services} />);
  });
  await act(async () => request(8));
  expect(resolveClose).toHaveBeenCalledWith(8, false);
  expect(JSON.stringify(renderer.toJSON())).toContain(
    'Recovery store unavailable',
  );
  await act(async () => renderer.unmount());
});
