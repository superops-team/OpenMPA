import { act, create, type ReactTestRenderer } from 'react-test-renderer';
import { beforeEach, afterEach, expect, it, vi } from 'vitest';
import { MpaTosBucketPicker } from '../src/client/desktop/MpaTosBucketPicker';
import { createLifecycleServices } from '../src/client/desktop/lifecycle-services';
import { messagesIn } from '../src/client/desktop/generated/locales';
import type { MPALifecycle } from '../src/client/desktop/generated/mygo';

let renderer: ReactTestRenderer | undefined;
const m = messagesIn('en');
const buckets = [
  { name: 'agentkit-platform-outputs', region: 'cn-beijing' },
  { name: 'agentkit-platform-images', region: 'cn-beijing' },
];
beforeEach(() => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  vi.stubGlobal('mygo', { locale: 'en' });
});
afterEach(async () => {
  if (renderer) await act(async () => renderer!.unmount());
  renderer = undefined;
  vi.unstubAllGlobals();
});
const options = () =>
  renderer!.root
    .findAllByProps({ role: 'option' })
    .map((node) => node.props.value);
const button = (name: string) =>
  renderer!.root
    .findAllByType('button')
    .find((node) => node.children.includes(name))!;
it('automatically loads, filters, selects, clears and refreshes without changing the selected value', async () => {
  const tosBuckets = vi.fn(async () => buckets);
  const onChange = vi.fn();
  await act(async () => {
    renderer = create(
      <MpaTosBucketPicker
        services={{ tosBuckets }}
        epoch={7}
        region="cn-beijing"
        value=""
        disabled={false}
        onChange={onChange}
      />,
    );
  });
  expect(tosBuckets).toHaveBeenCalledExactlyOnceWith(7, 'cn-beijing');
  expect(renderer!.root.findByType('legend').children).toContain(
    m.mpaTosTitle(),
  );
  expect(JSON.stringify(renderer!.toJSON())).toContain(m.mpaTosDescription());
  expect(JSON.stringify(renderer!.toJSON())).toContain(
    m.mpaTosLoaded('cn-beijing', '2', '2'),
  );
  await act(async () =>
    renderer!.root.findByProps({ role: 'combobox' }).props.onClick(),
  );
  expect(options()).toEqual([buckets[0].name, buckets[1].name]);
  await act(async () =>
    renderer!.root
      .findByType('input')
      .props.onChange({ target: { value: 'images' } }),
  );
  expect(options()).toEqual([buckets[1].name]);
  await act(async () =>
    renderer!.root
      .findByProps({ role: 'option', value: buckets[1].name })
      .props.onClick(),
  );
  expect(onChange).toHaveBeenCalledExactlyOnceWith(buckets[1].name);
  await act(async () =>
    renderer!.root.findByProps({ role: 'combobox' }).props.onClick(),
  );
  await act(async () => button(m.mpaTosRefresh()).props.onClick());
  expect(tosBuckets).toHaveBeenCalledTimes(2);
  expect(onChange).toHaveBeenCalledOnce();
  await act(async () =>
    renderer!.update(
      <MpaTosBucketPicker
        services={{ tosBuckets }}
        epoch={7}
        region="cn-beijing"
        value={buckets[1].name}
        disabled={false}
        onChange={onChange}
      />,
    ),
  );
  await act(async () => button(m.mpaTosClear()).props.onClick());
  expect(onChange).toHaveBeenLastCalledWith('');
});
it('distinguishes a permission error from empty results and allows retry', async () => {
  const tosBuckets = vi
    .fn()
    .mockRejectedValueOnce({ code: 'denied' })
    .mockResolvedValue([]);
  await act(async () => {
    renderer = create(
      <MpaTosBucketPicker
        services={{ tosBuckets }}
        epoch={7}
        region="cn-beijing"
        value="agentkit-platform-saved"
        disabled={false}
        onChange={() => {}}
      />,
    );
  });
  expect(
    renderer!.root.findByProps({ role: 'alert' }).findByType('span').children,
  ).toContain(m.denied());
  expect(renderer!.root.findByProps({ role: 'combobox' }).props.disabled).toBe(
    true,
  );
  expect(JSON.stringify(renderer!.toJSON())).toContain(
    'agentkit-platform-saved',
  );
  expect(JSON.stringify(renderer!.toJSON())).not.toContain(m.mpaTosEmpty());
  await act(async () => button(m.mpaTosRefresh()).props.onClick());
  expect(JSON.stringify(renderer!.toJSON())).toContain(m.mpaTosEmpty());
  expect(JSON.stringify(renderer!.toJSON())).toContain(
    'agentkit-platform-saved',
  );
});
it('discards replies from the previous account/region', async () => {
  let resolveOld!: (value: typeof buckets) => void;
  const old = new Promise<typeof buckets>((resolve) => {
    resolveOld = resolve;
  });
  const tosBuckets = vi
    .fn()
    .mockReturnValueOnce(old)
    .mockResolvedValue([
      { name: 'agentkit-platform-new', region: 'cn-shanghai' },
    ]);
  const props = {
    services: { tosBuckets },
    value: '',
    disabled: false,
    onChange: () => {},
  };
  await act(async () => {
    renderer = create(
      <MpaTosBucketPicker {...props} epoch={7} region="cn-beijing" />,
    );
  });
  expect(renderer!.root.findByProps({ role: 'combobox' }).props.disabled).toBe(
    true,
  );
  await act(async () =>
    renderer!.update(
      <MpaTosBucketPicker {...props} epoch={8} region="cn-shanghai" />,
    ),
  );
  await act(async () => resolveOld(buckets));
  await act(async () =>
    renderer!.root.findByProps({ role: 'combobox' }).props.onClick(),
  );
  expect(options()).toEqual(['agentkit-platform-new']);
});
it('maps the native bucket endpoint and rejects a mismatched scope', async () => {
  const listTOSBuckets = vi.fn(async () => ({
    ok: true,
    value: { buckets, platformEpoch: 7, region: 'cn-beijing' },
  }));
  const services = createLifecycleServices({
    listTOSBuckets,
  } as unknown as typeof MPALifecycle);
  expect(await services.tosBuckets(7, 'cn-beijing')).toEqual(buckets);
  expect(listTOSBuckets).toHaveBeenCalledExactlyOnceWith({
    platformEpoch: 7,
    region: 'cn-beijing',
  });
  await expect(services.tosBuckets(8, 'cn-beijing')).rejects.toMatchObject({
    code: 'staleConnection',
  });
});
