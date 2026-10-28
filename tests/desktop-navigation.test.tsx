import { readFileSync } from 'node:fs';
import { act, create, type ReactTestRenderer } from 'react-test-renderer';
import { expect, it, vi } from 'vitest';
import { DesktopApp } from '../src/client/desktop/DesktopApp';
import { messages } from '../src/client/desktop/generated/locales';
import { setThemePreference, themePreference } from '../src/client/theme';
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
    platform: {
      status: async () => ({
        signedIn: true,
        accountId: '123',
        region: 'cn-beijing',
      }),
      list: async () => [
        { id: 'r', name: 'Runtime A', status: 'Ready', version: '1' },
      ],
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
      sessions: [{ id: 's', title: 'My assistant', agentId: '' }],
      spaces: [],
      selectedAssistant: 'r',
      assistants: [{ id: 'r', title: 'Runtime A' }],
      theme: 'system',
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
  await act(async () => {
    navigation('selectSettingsSection', 'Assistant');
  });
  expect(JSON.stringify(renderer.toJSON())).toContain(
    messages.settingsOpenAssistantSettings(),
  );
  expect(renderer.root.findAllByProps({ role: 'dialog' })).toHaveLength(0);
  await act(async () => renderer.unmount());
});

it('switches Runtime from the native assistant selector', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  let navigation!: (action: string, id?: string) => void;
  const setShellState = vi.fn(async (state: NativeShellState) => {
    void state;
  });
  const bind = vi.fn(async () => ({
    epoch: 3,
    scope: 'next',
    runtimeId: 'r-two',
    status: 'connected',
  }));
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
    platform: {
      status: async () => ({
        signedIn: true,
        accountId: '123',
        region: 'cn-beijing',
      }),
      list: async () => [
        { id: 'r-one', name: 'Runtime One', status: 'Ready', version: '1' },
        { id: 'r-two', name: 'Runtime Two', status: 'Ready', version: '1' },
      ],
      bind,
    },
    connection: {
      status: async () => ({
        epoch: 2,
        scope: 'old',
        runtimeId: 'r-one',
        status: 'connected',
      }),
    },
    sessions: { list: async () => [] },
  } as unknown as DesktopServices;
  let renderer!: ReactTestRenderer;
  await act(async () => {
    renderer = create(<DesktopApp services={services} />);
  });
  await act(async () => {});
  await act(async () => navigation('selectAssistant', 'r-two'));
  expect(bind).toHaveBeenCalledWith('cn-beijing', 'default', 'r-two');
  expect(setShellState).toHaveBeenLastCalledWith(
    expect.objectContaining({ selectedAssistant: 'r-two' }),
  );
  await act(async () => renderer.unmount());
});

it('forces a remote session refresh from the shell refresh action', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  let navigation!: (action: string, id?: string) => void;
  const list = vi.fn(async () => []);
  const services = {
    host: {
      onNavigation: (listener: typeof navigation) => {
        navigation = listener;
        return () => {};
      },
      setShellState: async () => {},
      onClose: () => () => {},
      closeState: async () => ({ pending: false, requestId: 0 }),
    },
    connection: {
      status: async () => ({
        epoch: 2,
        scope: 'a',
        runtimeId: 'r',
        status: 'connected',
      }),
    },
    sessions: { list },
  } as unknown as DesktopServices;
  let renderer!: ReactTestRenderer;
  await act(async () => {
    renderer = create(<DesktopApp services={services} />);
  });
  await act(async () => navigation('refreshSessions'));
  expect(list).toHaveBeenLastCalledWith(2, true);
  await act(async () => renderer.unmount());
});

it('contains desktop scroll containment rules for trackpad scrolling', () => {
  const foundation = readFileSync(
    new URL('../src/client/styles/foundation.css', import.meta.url),
    'utf8',
  );
  const desktop = readFileSync(
    new URL('../src/client/desktop/style.css', import.meta.url),
    'utf8',
  );
  const trace = readFileSync(
    new URL('../src/client/desktop/trace.css', import.meta.url),
    'utf8',
  );
  expect(foundation).toMatch(/overscroll-behavior:\s*none;/);
  expect(desktop).toMatch(
    /\.desktop-app \.main-content\s*\{[^}]*overscroll-behavior:\s*contain;/s,
  );
  expect(desktop).toMatch(
    /\.desktop-app \.chat-transcript\s*\{[^}]*overscroll-behavior:\s*contain;/s,
  );
  expect(trace).toMatch(/\.trace-panel\s*\{[^}]*overscroll-behavior:\s*none;/s);
});

it('routes a disconnected shell new-conversation action to React settings', async () => {
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
    workspace: { spaces: async () => [] },
    platform: {
      status: async () => ({ signedIn: false, accountId: '', region: '' }),
    },
    connection: { status: async () => undefined },
  } as unknown as DesktopServices;
  let renderer!: ReactTestRenderer;
  await act(async () => {
    renderer = create(<DesktopApp services={services} />);
  });
  await act(async () => {
    navigation('newConversation');
  });
  expect(setShellState).toHaveBeenLastCalledWith(
    expect.objectContaining({ view: 'settings' }),
  );
  await act(async () => renderer.unmount());
});

it('home expands the sidebar and theme changes reach the React surface', async () => {
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
    connection: {
      status: async () => ({
        epoch: 2,
        scope: 'a',
        runtimeId: 'r',
        status: 'connected',
      }),
    },
    sessions: { list: async () => [] },
  } as unknown as DesktopServices;
  let renderer!: ReactTestRenderer;
  await act(async () => {
    renderer = create(<DesktopApp services={services} />);
  });
  await act(async () => navigation('toggleSidebar'));
  expect(setShellState).toHaveBeenLastCalledWith(
    expect.objectContaining({ collapsed: true }),
  );
  await act(async () => navigation('home'));
  expect(setShellState).toHaveBeenLastCalledWith(
    expect.objectContaining({ collapsed: false, view: 'chat' }),
  );
  await act(async () => navigation('themeChanged', 'dark'));
  expect(themePreference()).toBe('dark');
  setThemePreference('system');
  await act(async () => renderer.unmount());
});

it('reads the persisted native theme on mount', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  setThemePreference('system');
  const services = {
    host: {
      theme: async () => ({ theme: 'dark' }),
      onNavigation: () => () => {},
      setShellState: async () => {},
      onClose: () => () => {},
      closeState: async () => ({ pending: false, requestId: 0 }),
    },
    connection: {
      status: async () => ({
        epoch: 2,
        scope: 'a',
        runtimeId: 'r',
        status: 'connected',
      }),
    },
    sessions: { list: async () => [] },
  } as unknown as DesktopServices;
  let renderer!: ReactTestRenderer;
  await act(async () => {
    renderer = create(<DesktopApp services={services} />);
  });
  await act(async () => {});
  expect(themePreference()).toBe('dark');
  setThemePreference('system');
  await act(async () => renderer.unmount());
});

it('opens and closes the trace overlay from the native titlebar action', async () => {
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
    connection: {
      status: async () => ({
        epoch: 2,
        scope: 'a',
        runtimeId: 'r',
        status: 'connected',
      }),
    },
    sessions: { list: async () => [] },
    trace: { read: vi.fn() },
  } as unknown as DesktopServices;
  let renderer!: ReactTestRenderer;
  await act(async () => {
    renderer = create(<DesktopApp services={services} />);
  });
  await act(async () => navigation('trace'));
  expect(setShellState).toHaveBeenLastCalledWith(
    expect.objectContaining({ traceOpen: true }),
  );
  await act(async () => navigation('trace'));
  expect(setShellState).toHaveBeenLastCalledWith(
    expect.objectContaining({ traceOpen: false }),
  );
  await act(async () => renderer.unmount());
});

it('routes settings section clicks to React settings content', async () => {
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
    platform: {
      status: async () => ({ signedIn: false, accountId: '', region: '' }),
    },
    localization: {
      status: async () => ({ mode: 'system', locale: 'en' }),
      set: async (mode: string) => ({ mode, locale: mode }),
    },
    connection: {
      status: async () => ({
        epoch: 2,
        scope: 'a',
        runtimeId: 'r',
        status: 'connected',
      }),
    },
    sessions: { list: async () => [] },
  } as unknown as DesktopServices;
  let renderer!: ReactTestRenderer;
  await act(async () => {
    renderer = create(<DesktopApp services={services} />);
  });
  await act(async () => navigation('selectSettingsSection', 'Theme'));
  expect(setShellState).toHaveBeenLastCalledWith(
    expect.objectContaining({
      view: 'settings',
      settingsSection: 'Theme',
    }),
  );
  expect(JSON.stringify(renderer.toJSON())).toContain(messages.displayTheme());
  await act(async () => navigation('selectSettingsSection', 'Language'));
  expect(setShellState).toHaveBeenLastCalledWith(
    expect.objectContaining({
      view: 'settings',
      settingsSection: 'Language',
    }),
  );
  expect(JSON.stringify(renderer.toJSON())).toContain(
    messages.displayLanguage(),
  );
  await act(async () => renderer.unmount());
});
