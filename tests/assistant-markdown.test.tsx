import { act, create, type ReactTestRenderer } from 'react-test-renderer';
import { expect, it, vi } from 'vitest';
import { AssistantMarkdown } from '../src/client/desktop/AssistantMarkdown';

it('renders safe GFM with code controls and a scrollable table', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  let renderer!: ReactTestRenderer;
  await act(async () => {
    renderer = create(
      <AssistantMarkdown
        text={[
          '[Safe](https://example.com) [Unsafe](javascript:alert(1))',
          '',
          '| 列一 | 列二 |',
          '| --- | --- |',
          '| 内容 | 值 |',
          '',
          '```ts',
          'const answer = 42',
          '```',
          '',
          '```',
          'plain fence',
          '```',
          '',
          '<script>never()</script>',
        ].join('\n')}
      />,
    );
  });

  expect(renderer.root.findAllByType('a')).toHaveLength(1);
  expect(renderer.root.findByType('a').props.href).toBe('https://example.com');
  expect(
    renderer.root.findByProps({ 'aria-label': 'Scrollable Markdown table' }),
  ).toBeTruthy();
  expect(
    renderer.root.findAll(
      (node) =>
        node.type === 'button' && node.props['aria-label'] === 'Copy code',
    ),
  ).toHaveLength(2);
  const wraps = renderer.root.findAll(
    (node) =>
      node.type === 'button' && node.props['aria-label'] === 'Wrap code lines',
  );
  expect(wraps).toHaveLength(2);
  const wrap = wraps[0];
  expect(JSON.stringify(renderer.toJSON())).toContain('TypeScript');
  expect(JSON.stringify(renderer.toJSON())).toContain('Plain text');
  expect(JSON.stringify(renderer.toJSON())).not.toContain('<script>');
  await act(async () => wrap.props.onClick());
  expect(
    renderer.root.find(
      (node) =>
        node.type === 'button' &&
        node.props['aria-label'] === 'Do not wrap code lines',
    ),
  ).toBeTruthy();
  await act(async () => renderer.unmount());
});
