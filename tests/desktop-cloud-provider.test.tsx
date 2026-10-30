import { act, create, type ReactTestRenderer } from 'react-test-renderer';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { SettingsAccountForm } from '../src/client/desktop/SettingsAccountForm';
import { useDesktopSettingsState } from '../src/client/desktop/SettingsState';
import { RuntimeManager } from '../src/client/desktop/RuntimeManager';
import type { DesktopServices } from '../src/client/desktop/models';

let renderer: ReactTestRenderer;
beforeEach(() => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  vi.stubGlobal('mygo', { locale: 'en' });
});
afterEach(async () => {
  if (renderer) await act(async () => renderer.unmount());
  vi.unstubAllGlobals();
});

it('changes login platform and clears credentials before using its region', async () => {
  const platform = {
    status: vi.fn(async () => ({
      signedIn: false,
      accountId: '',
      region: 'cn-beijing',
    })),
    signIn: vi.fn(async () => ({
      signedIn: true,
      provider: 'byteplus',
      accountId: '123',
      region: 'ap-southeast-1',
    })),
    list: vi.fn(async () => []),
  };
  const services = { platform } as unknown as DesktopServices;
  function Form() {
    const state = useDesktopSettingsState({
      services,
      onConnection: vi.fn(),
    });
    return <SettingsAccountForm {...state} />;
  }
  await act(async () => {
    renderer = create(<Form />);
  });
  expect(
    renderer.root
      .findAllByType('select')[1]
      .findAllByType('option')
      .map((n) => n.props.value),
  ).toEqual(['cn-beijing', 'cn-shanghai']);
  await act(async () => {
    renderer.root
      .findAllByType('input')[0]
      .props.onChange({ target: { value: 'domestic-ak' } });
  });
  await act(async () => {
    renderer.root
      .findAllByType('select')[0]
      .props.onChange({ target: { value: 'byteplus' } });
  });
  expect(renderer.root.findAllByType('input')[0].props.value).toBe('');
  expect(renderer.root.findAllByType('select')[1].props.value).toBe(
    'ap-southeast-1',
  );
  expect(
    renderer.root
      .findAllByType('select')[1]
      .findAllByType('option')
      .map((n) => n.props.value),
  ).toEqual(['ap-southeast-1']);
  await act(async () => {
    renderer.root
      .findAllByType('input')[0]
      .props.onChange({ target: { value: 'bp-ak' } });
  });
  await act(async () => {
    renderer.root
      .findAllByType('input')[1]
      .props.onChange({ target: { value: 'bp-sk' } });
  });
  await act(async () => {
    renderer.root
      .findByType('form')
      .props.onSubmit({ preventDefault: vi.fn() });
  });
  expect(platform.signIn).toHaveBeenCalledWith({
    provider: 'byteplus',
    region: 'ap-southeast-1',
    accessKey: 'bp-ak',
    secretKey: 'bp-sk',
    securityToken: '',
  });
});

it('loads BP runtimes in Singapore even when the initial platform snapshot is absent', async () => {
  const state = {
    signedIn: true,
    provider: 'byteplus',
    region: 'ap-southeast-1',
    accountId: '123',
    scope: 'byteplus:123/User/456',
    epoch: 2,
  };
  const platform = {
    status: vi.fn(async () => state),
    list: vi.fn(async () => []),
  };
  await act(async () => {
    renderer = create(
      <RuntimeManager
        services={{ platform } as unknown as DesktopServices}
        onConnection={vi.fn()}
        onDirectoryChanged={async () => {}}
      />,
    );
  });
  expect(platform.list).toHaveBeenCalledWith(
    'ap-southeast-1',
    'default',
    false,
  );
  expect(platform.list).toHaveBeenCalledTimes(1);
  expect(
    renderer.root
      .findByType('select')
      .findAllByType('option')
      .map((n) => n.props.value),
  ).toEqual(['ap-southeast-1']);
});
