import { act, create, type ReactTestRenderer } from 'react-test-renderer';
import { expect, it, vi } from 'vitest';
import { DesktopApp } from '../src/client/desktop/DesktopApp';
import type { DesktopServices } from '../src/client/desktop/models';
it('clears an old conversation-load error after the same connection refresh succeeds', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  const list = vi
    .fn()
    .mockRejectedValueOnce(
      Object.assign(new Error('Not supported'), { httpStatus: 404 }),
    )
    .mockResolvedValue([{ id: 's', title: 'A conversation' }]);
  let navigation!: (action: string) => void;
  const setShellState = vi.fn(async () => {});
  const services = {
    host: {
      onNavigation: (listener: typeof navigation) => {
        navigation = listener;
        return () => {};
      },
      setShellState,
      onClose: () => () => {},
      closeState: async () => ({ pending: false, requestId: 0 }),
    },
    workspace: { spaces: async () => [] },
    connection: {
      status: async () => ({
        epoch: 2,
        scope: 'scope',
        runtimeId: 'r',
        status: 'connected',
      }),
    },
    sessions: {
      list,
      read: async () => ({
        session: { id: 's', title: 'A conversation' },
        messages: [],
      }),
      pending: async () => [],
    },
  } as unknown as DesktopServices;
  let renderer!: ReactTestRenderer;
  await act(async () => {
    renderer = create(<DesktopApp services={services} />);
  });
  expect(
    renderer.root.findByProps({ role: 'alert' }).props['data-error-source'],
  ).toBe('sessions');
  await act(async () => {
    navigation('refreshSessions');
  });
  await act(async () => {});
  expect(renderer.root.findAllByProps({ role: 'alert' })).toHaveLength(0);
  expect(setShellState).toHaveBeenLastCalledWith(
    expect.objectContaining({
      sessions: [{ id: 's', title: 'A conversation', agentId: '' }],
    }),
  );
  await act(async () => renderer.unmount());
});
