import {
  act,
  create,
  type ReactTestRenderer,
  type ReactTestInstance,
} from 'react-test-renderer';
import { expect, it, vi } from 'vitest';
import { DesktopSettings } from '../src/client/desktop/Settings';
import { LocaleProvider } from '../src/client/desktop/locale';
import { messages, messagesIn } from '../src/client/desktop/generated/locales';
import type { DesktopServices, RuntimeVM } from '../src/client/desktop/models';

const m = messages;

function hasRuntimeAction(button: ReactTestInstance, label: string) {
  return button
    .findAllByType('span')
    .some(
      (span) =>
        span.props['aria-hidden'] === false && span.children.includes(label),
    );
}

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
      .find((button) => hasRuntimeAction(button, m.settingsUseRuntime()))!
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
            typeof child === 'string' &&
            child.includes(m.settingsListCurrentAccount()),
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
      .find((button) => hasRuntimeAction(button, m.settingsUseRuntime()))!
      .props.onClick();
  });
  expect(
    renderer.root
      .findAllByType('button')
      .some((button) => hasRuntimeAction(button, m.settingsRuntimeSelecting())),
  ).toBe(true);
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
    passwordInputs[0].props.onChange({ target: { value: 'first passphrase' } });
    passwordInputs[1].props.onChange({
      target: { value: 'different passphrase' },
    });
  });
  await act(async () =>
    renderer.root
      .findAllByType('button')
      .find((button) => button.children.includes(m.saveCredentials()))!
      .props.onClick(),
  );
  expect(saveVault).not.toHaveBeenCalled();
  expect(JSON.stringify(renderer.toJSON())).toContain(
    m.settingsPassphrasesMismatch(),
  );
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
      .find((button) => button.children.includes(m.saveCredentials()))!
      .props.onClick(),
  );
  expect(saveVault).toHaveBeenCalledWith('correct horse battery');
  expect(JSON.stringify(renderer.toJSON())).toContain(
    m.vaultAccount('1234567890', 'cn-beijing'),
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
  expect(JSON.stringify(renderer.toJSON())).toContain(m.vaultUnlockHint());
  const unlockInput = renderer.root
    .findAllByType('input')
    .find((input) => input.props.autoComplete === 'current-password')!;
  const unlockButton = () =>
    renderer.root
      .findAllByType('button')
      .find((button) => button.children.includes(m.unlockAndSignIn()))!;
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

it('unlocks saved Agent Identity for the current Runtime connection', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  const userTokenVaultStatus = vi.fn(async () => ({
    exists: true,
    runtimeId: 'runtime-1',
    subject: 'user-1',
    expiresAt: '2026-10-29T12:00:00Z',
  }));
  const unlockUserTokenVault = vi.fn(async () => ({
    exists: true,
    runtimeId: 'runtime-1',
    subject: 'user-1',
  }));
  const status = vi.fn(async () => ({
    epoch: 8,
    scope: 'scope/business/profile-user-1',
    runtimeId: 'runtime-1',
    status: m.explicitIdentityStatus(),
    businessIdentity: 'explicit' as const,
  }));
  const services = {
    platform: {
      status: async () => ({
        signedIn: true,
        accountId: '1234567890',
        region: 'cn-beijing',
      }),
      userTokenVaultStatus,
      unlockUserTokenVault,
      forgetUserTokenVault: vi.fn(async () => {}),
    },
    connection: {
      status,
      test: vi.fn(async () => 'ok'),
    },
  } as unknown as DesktopServices;
  const onConnection = vi.fn();
  let renderer!: ReactTestRenderer;
  await act(async () => {
    renderer = create(
      <DesktopSettings
        services={services}
        connection={{
          epoch: 7,
          scope: 'scope',
          runtimeId: 'runtime-1',
          status: m.runtimeKeyStatus(),
          businessIdentity: 'missing',
        }}
        onConnection={onConnection}
        section="Authentication"
      />,
    );
  });
  await act(async () => {});
  expect(userTokenVaultStatus).toHaveBeenCalledWith('runtime-1');
  expect(JSON.stringify(renderer.toJSON())).toContain(
    m.settingsConnectionSummary(
      'user-1',
      m.settingsLegacyJwtAuthenticated(),
      '2026-10-29T12:00:00Z',
    ),
  );
  const input = renderer.root.findByProps({
    'aria-label': m.vaultUnlockHint(),
  });
  const unlockButton = () =>
    renderer.root
      .findAllByType('button')
      .find((button) => button.children.includes(m.unlockAndSignIn()))!;
  expect(unlockButton().props.disabled).toBe(true);
  await act(async () =>
    input.props.onChange({ target: { value: 'correct horse battery' } }),
  );
  expect(unlockButton().props.disabled).toBe(false);
  await act(async () => unlockButton().props.onClick());
  expect(unlockUserTokenVault).toHaveBeenCalledWith(
    'runtime-1',
    'correct horse battery',
    7,
  );
  expect(status).toHaveBeenCalledTimes(1);
  expect(onConnection).toHaveBeenCalledWith(
    expect.objectContaining({
      runtimeId: 'runtime-1',
      epoch: 8,
      businessIdentity: 'explicit',
    }),
  );
  expect(JSON.stringify(renderer.toJSON())).not.toContain(
    'correct horse battery',
  );
  await act(async () => renderer.unmount());
});

it('renders saved credential unlock copy in Chinese without crowding labels', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  let changed: ((payload: { locale: string; dir: string }) => void) | undefined;
  let locale = 'zh-Hans';
  const previousMyGo = (globalThis as { mygo?: unknown }).mygo;
  (globalThis as { mygo?: unknown }).mygo = {
    get locale() {
      return locale;
    },
    on: (_name: string, listener: typeof changed) => {
      changed = listener;
      return () => {};
    },
  };
  const services = {
    platform: {
      status: vi.fn(async () => ({
        signedIn: false,
        accountId: '',
        region: 'cn-beijing',
      })),
      vaultStatus: vi.fn(async () => ({
        exists: true,
        accountId: '2112682748',
        region: 'cn-beijing',
      })),
      list: vi.fn(async () => []),
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
  await act(async () =>
    renderer.root
      .findByProps({ autoComplete: 'current-password' })
      .props.onChange({
        target: { value: 'unsaved passphrase' },
      }),
  );
  const en = messagesIn('en');
  await act(async () => {
    locale = 'en';
    changed!({ locale, dir: 'ltr' });
  });
  expect(JSON.stringify(renderer.toJSON())).toContain(
    en.settingsRegionBeijing(),
  );
  expect(
    renderer.root.findByProps({ autoComplete: 'current-password' }).props.value,
  ).toBe('unsaved passphrase');
  expect(services.platform.status).toHaveBeenCalledTimes(1);
  expect(services.platform.vaultStatus).toHaveBeenCalledTimes(1);
  expect(services.platform.unlockVault).not.toHaveBeenCalled();
  await act(async () =>
    renderer.root
      .findAllByType('button')
      .find((button) => button.children.includes(en.unlockAndSignIn()))!
      .props.onClick(),
  );
  expect(JSON.stringify(renderer.toJSON())).toContain(
    en.settingsVaultSignedIn(),
  );
  await act(async () => {
    locale = 'zh-Hans';
    changed!({ locale, dir: 'ltr' });
  });
  expect(JSON.stringify(renderer.toJSON())).toContain(
    m.settingsVaultSignedIn(),
  );
  expect(JSON.stringify(renderer.toJSON())).not.toContain(
    en.settingsVaultSignedIn(),
  );
  expect(services.platform.status).toHaveBeenCalledTimes(1);
  expect(services.platform.vaultStatus).toHaveBeenCalledTimes(2);
  expect(services.platform.unlockVault).toHaveBeenCalledTimes(1);
  expect(services.platform.list).toHaveBeenCalledTimes(1);
  await act(async () => renderer.unmount());
  (globalThis as { mygo?: unknown }).mygo = previousMyGo;
});
