import { act, create, type ReactTestRenderer } from 'react-test-renderer';
import { afterEach, expect, it, vi } from 'vitest';
import { messagesIn, languages } from '../src/client/desktop/generated/locales';
import { LocaleProvider, useMessages } from '../src/client/desktop/locale';

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
