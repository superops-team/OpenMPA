import { act, create, type ReactTestRenderer } from 'react-test-renderer';
import { expect, it, vi } from 'vitest';
import { DesktopApp } from '../src/client/desktop/DesktopApp';
import type {
  DesktopServices,
  NativeShellState,
} from '../src/client/desktop/models';

it('projects navigation into the native shell and handles native selection and collapse', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  let navigation!: (action: string, id?: string) => void;
  const setShellState = vi.fn(async (state: NativeShellState) => {
    void state;
  });
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
    workspace: { spaces: async () => [{ id: 'space', name: 'Personal' }] },
    connection: {
      status: async () => ({
        epoch: 2,
        scope: 'a',
        runtimeId: 'r',
        status: 'connected',
      }),
    },
    sessions: {
      list: async () => [{ id: 's', title: 'My assistant' }],
      read: async () => ({
        session: { id: 's', title: 'My assistant' },
        messages: [],
      }),
      pending: async () => [],
    },
  } as unknown as DesktopServices;
  let renderer!: ReactTestRenderer;
  await act(async () => {
    renderer = create(<DesktopApp services={services} />);
  });
  expect(renderer.root.findAllByType('aside')).toHaveLength(0);
  expect(
    renderer.root.findAllByProps({ className: 'desktop-titlebar' }),
  ).toHaveLength(0);
  expect(setShellState).toHaveBeenLastCalledWith(
    expect.objectContaining({
      connectionEpoch: 2,
      view: 'chat',
      collapsed: false,
      sessions: [{ id: 's', title: 'My assistant' }],
      spaces: [{ id: 'space', title: 'Personal' }],
    }),
  );
  const revision = setShellState.mock.calls.at(-1)![0].revision;
  await act(async () => {
    navigation('toggleSidebar');
  });
  expect(setShellState).toHaveBeenLastCalledWith(
    expect.objectContaining({ collapsed: true, revision: expect.any(Number) }),
  );
  expect(setShellState.mock.calls.at(-1)![0].revision).toBeGreaterThan(
    revision,
  );
  await act(async () => {
    navigation('selectSession', 's');
  });
  expect(setShellState).toHaveBeenLastCalledWith(
    expect.objectContaining({
      selectedSession: 's',
      view: 'chat',
      collapsed: true,
    }),
  );
  await act(async () => {
    navigation('toggleSidebar');
  });
  expect(setShellState).toHaveBeenLastCalledWith(
    expect.objectContaining({ selectedSession: 's', collapsed: false }),
  );
  await act(async () => {
    navigation('selectSession', 'missing');
  });
  expect(setShellState).toHaveBeenLastCalledWith(
    expect.objectContaining({ selectedSession: 's' }),
  );
  await act(async () => renderer.unmount());
});
