import { act, create, type ReactTestRenderer } from 'react-test-renderer';
import { expect, it, vi } from 'vitest';
import { DesktopSettings } from '../src/client/desktop/Settings';
import { LocaleProvider } from '../src/client/desktop/locale';
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
      <DesktopSettings
        services={services}
        onConnection={onConnection}
        section="Runtime"
      />,
    );
  });
  expect(list).toHaveBeenCalledWith('cn-beijing', 'default', false);
  await act(async () => {
    renderer.root
      .findAllByType('select')
      .find((select) => select.props.value === 'cn-beijing')!
      .props.onChange({ target: { value: 'cn-shanghai' } });
  });
  expect(list).toHaveBeenLastCalledWith('cn-shanghai', 'default', false);
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

it('force refreshes the Runtime directory only from the explicit refresh button', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  const list = vi.fn(async () => [
    { id: 'r-cached', name: 'Cached MPA', status: 'Ready', version: '1' },
  ]);
  const services = {
    platform: {
      status: async () => ({
        signedIn: true,
        accountId: '1234567890',
        region: 'cn-beijing',
      }),
      list,
    },
  } as unknown as DesktopServices;
  let renderer!: ReactTestRenderer;
  await act(async () => {
    renderer = create(
      <DesktopSettings
        services={services}
        onConnection={() => {}}
        section="Runtime"
      />,
    );
  });
  await act(async () => {});
  expect(list).toHaveBeenCalledWith('cn-beijing', 'default', false);
  await act(async () =>
    renderer.root
      .findAllByType('button')
      .find((button) =>
        button.children.some(
          (child) =>
            typeof child === 'string' && child.includes('List current account'),
        ),
      )!
      .props.onClick(),
  );
  expect(list).toHaveBeenLastCalledWith('cn-beijing', 'default', true);
  await act(async () => renderer.unmount());
});

it('shows the Runtime being selected until the authoritative binding returns', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  let resolveBind!: (value: {
    epoch: number;
    scope: string;
    runtimeId: string;
    status: string;
  }) => void;
  const bind = vi.fn(
    () =>
      new Promise<{
        epoch: number;
        scope: string;
        runtimeId: string;
        status: string;
      }>((resolve) => {
        resolveBind = resolve;
      }),
  );
  const services = {
    platform: {
      status: async () => ({
        signedIn: true,
        accountId: '1234567890',
        region: 'cn-beijing',
      }),
      list: async () => [
        { id: 'r-new', name: 'New MPA', status: 'Ready', version: '1' },
      ],
      bind,
    },
  } as unknown as DesktopServices;
  const onConnection = vi.fn();
  let renderer!: ReactTestRenderer;
  await act(async () => {
    renderer = create(
      <DesktopSettings
        services={services}
        connection={{
          epoch: 2,
          scope: 'old',
          runtimeId: 'r-old',
          status: 'connected',
        }}
        onConnection={onConnection}
        section="Runtime"
      />,
    );
  });
  await act(async () => {});
  await act(async () => {
    renderer.root
      .findAllByType('button')
      .find((button) => button.children.includes('Use Runtime'))!
      .props.onClick();
  });
  expect(JSON.stringify(renderer.toJSON())).toContain('Selecting…');
  expect(renderer.root.findByProps({ 'aria-busy': true }).props.disabled).toBe(
    true,
  );
  await act(async () => {
    resolveBind({
      epoch: 3,
      scope: 'new',
      runtimeId: 'r-new',
      status: 'connected',
    });
    await Promise.resolve();
  });
  expect(onConnection).toHaveBeenCalledWith(
    expect.objectContaining({ runtimeId: 'r-new', epoch: 3 }),
  );
  await act(async () => renderer.unmount());
});

it('saves and unlocks encrypted cloud credentials from the React account settings', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  const saveVault = vi.fn(async () => ({
    exists: true,
    accountId: '1234567890',
    region: 'cn-beijing',
    updatedAt: '2026-10-27T00:00:00Z',
  }));
  const unlockVault = vi.fn(async () => ({
    signedIn: true,
    accountId: '1234567890',
    region: 'cn-beijing',
  }));
  const services = {
    platform: {
      status: async () => ({
        signedIn: true,
        accountId: '1234567890',
        region: 'cn-beijing',
      }),
      vaultStatus: vi.fn(async () => ({ exists: false })),
      saveVault,
      unlockVault,
      forgetVault: vi.fn(async () => {}),
      signOut: vi.fn(async () => {}),
    },
  } as unknown as DesktopServices;
  let renderer!: ReactTestRenderer;
  await act(async () => {
    renderer = create(
      <DesktopSettings
        services={services}
        onConnection={() => {}}
        section="Account"
      />,
    );
  });
  await act(async () => {});

  const passwordInputs = renderer.root
    .findAllByType('input')
    .filter((input) => input.props.type === 'password');
  await act(async () => {
    passwordInputs[0].props.onChange({
      target: { value: 'correct horse battery' },
    });
    passwordInputs[1].props.onChange({
      target: { value: 'correct horse battery' },
    });
  });
  await act(async () =>
    renderer.root
      .findAllByType('button')
      .find((button) => button.children.includes('Save credentials'))!
      .props.onClick(),
  );
  expect(saveVault).toHaveBeenCalledWith('correct horse battery');
  expect(JSON.stringify(renderer.toJSON())).toContain(
    'Account 1234567890 · cn-beijing',
  );

  await act(async () => {
    renderer.update(
      <DesktopSettings
        services={
          {
            platform: {
              status: async () => ({
                signedIn: false,
                accountId: '',
                region: 'cn-beijing',
              }),
              vaultStatus: async () => ({
                exists: true,
                accountId: '1234567890',
                region: 'cn-beijing',
              }),
              unlockVault,
            },
          } as unknown as DesktopServices
        }
        onConnection={() => {}}
        section="Account"
      />,
    );
  });
  await act(async () => {});
  expect(JSON.stringify(renderer.toJSON())).toContain(
    'Enter the passphrase to unlock the saved AK/SK.',
  );
  const unlockInput = renderer.root
    .findAllByType('input')
    .find((input) => input.props.autoComplete === 'current-password')!;
  const unlockButton = () =>
    renderer.root
      .findAllByType('button')
      .find((button) => button.children.includes('Unlock and sign in'))!;
  expect(unlockButton().props.disabled).toBe(true);
  await act(async () =>
    unlockInput.props.onChange({ target: { value: 'correct horse battery' } }),
  );
  expect(
    renderer.root
      .findAllByType('input')
      .find((input) => input.props.autoComplete === 'current-password')!.props
      .value,
  ).toBe('correct horse battery');
  expect(unlockButton().props.disabled).toBe(false);
  await act(async () => unlockButton().props.onClick());
  expect(unlockVault).toHaveBeenCalledWith('correct horse battery');
  expect(JSON.stringify(renderer.toJSON())).not.toContain(
    'correct horse battery',
  );

  await act(async () => renderer.unmount());
});

it('renders saved credential unlock copy in Chinese without crowding labels', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  let changed: ((payload: { locale: string; dir: string }) => void) | undefined;
  const previousMyGo = (globalThis as { mygo?: unknown }).mygo;
  (globalThis as { mygo?: unknown }).mygo = {
    get locale() {
      return 'zh-Hans';
    },
    on: (_name: string, listener: typeof changed) => {
      changed = listener;
      return () => {};
    },
  };
  const services = {
    platform: {
      status: async () => ({
        signedIn: false,
        accountId: '',
        region: 'cn-beijing',
      }),
      vaultStatus: async () => ({
        exists: true,
        accountId: '2112682748',
        region: 'cn-beijing',
      }),
      unlockVault: vi.fn(async () => ({
        signedIn: true,
        accountId: '2112682748',
        region: 'cn-beijing',
      })),
    },
  } as unknown as DesktopServices;
  let renderer!: ReactTestRenderer;
  await act(async () => {
    renderer = create(
      <LocaleProvider>
        <DesktopSettings
          services={services}
          onConnection={() => {}}
          section="Account"
        />
      </LocaleProvider>,
    );
  });
  await act(async () => {});
  const rendered = JSON.stringify(renderer.toJSON());
  expect(rendered).toContain('火山引擎账户');
  expect(rendered).toContain('输入口令解锁已保存的 AK/SK。');
  expect(rendered).toContain('口令');
  expect(rendered).toContain('解锁并登录');
  expect(rendered).toContain('账号 2112682748 · cn-beijing');
  await act(async () => renderer.unmount());
  (globalThis as { mygo?: unknown }).mygo = previousMyGo;
  void changed;
});
