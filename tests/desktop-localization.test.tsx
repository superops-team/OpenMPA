import { act, create, type ReactTestRenderer } from 'react-test-renderer';
import { afterEach, expect, it, vi } from 'vitest';
import { messagesIn, languages } from '../src/client/desktop/generated/locales';
import {
  LocaleProvider,
  localized,
  useMessages,
  useMessageState,
} from '../src/client/desktop/locale';
import { DesktopSettings } from '../src/client/desktop/Settings';
import type {
  DesktopServices,
  DesktopSettingsSection,
} from '../src/client/desktop/models';

const originalMyGo = (globalThis as { mygo?: unknown }).mygo;
afterEach(() => {
  (globalThis as { mygo?: unknown }).mygo = originalMyGo;
});

it('ships complete Chinese and English catalogs with Chinese source fallback', () => {
  expect(languages).toEqual(['zh-Hans', 'en']);
  expect(messagesIn('zh-Hans').settings()).toBe('设置');
  expect(messagesIn('en').settings()).toBe('Settings');
  expect(messagesIn('fr').settings()).toBe('设置');
});

it('redraws WebView content from the MyGo locale event', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  let locale = 'zh-Hans';
  let changed: ((payload: { locale: string; dir: string }) => void) | undefined;
  (globalThis as { mygo?: unknown }).mygo = {
    get locale() {
      return locale;
    },
    on: (_name: string, listener: typeof changed) => {
      changed = listener;
      return () => {};
    },
  };
  const Label = () => <span>{useMessages().settings()}</span>;
  let renderer!: ReactTestRenderer;
  await act(async () => {
    renderer = create(
      <LocaleProvider>
        <Label />
      </LocaleProvider>,
    );
  });
  expect(renderer.root.findByType('span').children).toEqual(['设置']);
  await act(async () => {
    locale = 'en';
    changed?.({ locale, dir: 'ltr' });
  });
  expect(renderer.root.findByType('span').children).toEqual(['Settings']);
  await act(async () => renderer.unmount());
});

it.each([
  ['Account', '登录', 'Sign in'],
  ['Runtime', '地域', 'Region'],
  ['Authentication', '业务用户 ID', 'Business user ID'],
  ['Assistant', '助手设置', 'Assistant settings'],
  ['Theme', '显示主题', 'Display theme'],
  ['Language', '显示语言', 'Display language'],
  ['Deployment', '读取当前镜像', 'Read current images'],
] as const)(
  'translates the React %s settings page when the app language changes',
  async (section, chinese, english) => {
    vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
    let locale = 'zh-Hans';
    let changed:
      ((payload: { locale: string; dir: string }) => void) | undefined;
    (globalThis as { mygo?: unknown }).mygo = {
      get locale() {
        return locale;
      },
      on: (_name: string, listener: typeof changed) => {
        changed = listener;
        return () => {};
      },
    };
    const status = vi.fn(async () => ({ signedIn: false }));
    const service = {
      platform: { status },
      localization: { status: vi.fn(async () => ({ mode: 'system' })) },
    } as unknown as DesktopServices;
    let renderer!: ReactTestRenderer;
    await act(async () => {
      renderer = create(
        <LocaleProvider>
          <DesktopSettings
            services={service}
            connection={{
              epoch: 2,
              scope: 'scope-原值',
              runtimeId: 'runtime-原值',
              status: 'connected',
            }}
            session={{ id: 'session-原值', title: 'My assistant 原值' }}
            section={section as DesktopSettingsSection}
            onConnection={() => {}}
          />
        </LocaleProvider>,
      );
    });
    try {
      expect(JSON.stringify(renderer.toJSON())).toContain(chinese);
      if (section === 'Account') {
        await act(async () =>
          renderer.root.findAllByType('input')[0].props.onChange({
            target: { value: 'credential-draft' },
          }),
        );
      }
      const accountReads = status.mock.calls.length;
      await act(async () => {
        locale = 'en';
        changed?.({ locale, dir: 'ltr' });
      });
      expect(JSON.stringify(renderer.toJSON())).toContain(english);
      expect(status).toHaveBeenCalledTimes(accountReads);
      if (section === 'Account')
        expect(renderer.root.findAllByType('input')[0].props.value).toBe(
          'credential-draft',
        );
      await act(async () => {
        locale = 'zh-Hans';
        changed?.({ locale, dir: 'ltr' });
      });
      expect(JSON.stringify(renderer.toJSON())).toContain(chinese);
      expect(status).toHaveBeenCalledTimes(accountReads);
    } finally {
      await act(async () => renderer.unmount());
    }
  },
);

it('redraws saved client notices but preserves raw service text', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  let locale = 'zh-Hans';
  let changed: ((payload: { locale: string; dir: string }) => void) | undefined;
  (globalThis as { mygo?: unknown }).mygo = {
    get locale() {
      return locale;
    },
    on: (_name: string, listener: typeof changed) => {
      changed = listener;
      return () => {};
    },
  };
  function Notice() {
    const [notice, setNotice] = useMessageState();
    return (
      <>
        <button onClick={() => setNotice(localized((m) => m.themeChanged()))} />
        <button onClick={() => setNotice('Service text 原值')} />
        <p>{notice}</p>
      </>
    );
  }
  let renderer!: ReactTestRenderer;
  await act(async () => {
    renderer = create(
      <LocaleProvider>
        <Notice />
      </LocaleProvider>,
    );
  });
  try {
    await act(async () =>
      renderer.root.findAllByType('button')[0].props.onClick(),
    );
    expect(renderer.root.findByType('p').children).toEqual([
      '显示主题已更新。',
    ]);
    await act(async () => {
      locale = 'en';
      changed?.({ locale, dir: 'ltr' });
    });
    expect(renderer.root.findByType('p').children).toEqual([
      'Display theme updated.',
    ]);
    await act(async () =>
      renderer.root.findAllByType('button')[1].props.onClick(),
    );
    await act(async () => {
      locale = 'zh-Hans';
      changed?.({ locale, dir: 'ltr' });
    });
    expect(renderer.root.findByType('p').children).toEqual([
      'Service text 原值',
    ]);
  } finally {
    await act(async () => renderer.unmount());
  }
});
