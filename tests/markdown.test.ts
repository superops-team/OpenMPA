import { expect, it } from 'vitest';
import {
  inspectMarkdown,
  markdownManager,
} from '../src/client/editor/markdown';
it('round-trips supported writing structures without losing semantic content', () => {
  const source =
    '# Project\n\nA **bold**, *italic*, ~~done~~ and `code` sentence.\n\n- First\n  - Nested\n\n1. Ordered\n2. Second\n\n- [ ] Open\n- [x] Complete\n\n> A quoted thought\n\n```ts\nconst value = "<safe>";\n```\n\n---\n\n| Name | Status |\n| --- | --- |\n| A | Ready |\n\n[Page](/#/spaces/space/pages/page)';
  expect(inspectMarkdown(source).supported).toBe(true);
  const first = markdownManager.parse(source);
  const roundtrip = markdownManager.serialize(first);
  expect(markdownManager.parse(roundtrip)).toEqual(first);
  expect(roundtrip).toContain('Complete');
  expect(roundtrip).toContain('/#/spaces/space/pages/page');
});
it.each([
  '![remote](https://example.com/photo.png)',
  '<script>alert(1)</script>',
  '<details><summary>Note</summary>Hidden</details>',
  '[^1]: A footnote',
  '---\ntitle: Front matter\n---\nBody',
])('requires exact source editing for unsupported Markdown: %s', (source) => {
  const result = inspectMarkdown(source);
  expect(result.supported).toBe(false);
  expect(result.reason).toBeTruthy();
});
it('does not mistake source code containing HTML for active HTML', () => {
  expect(
    inspectMarkdown('```html\n<img src="https://example.com">\n```').supported,
  ).toBe(true);
});
it('detects unsupported content nested inside task items', () => {
  expect(
    inspectMarkdown('- [ ] ![remote](https://example.com/a.png)').supported,
  ).toBe(false);
  expect(inspectMarkdown('- [x] <iframe>embedded</iframe>').supported).toBe(
    false,
  );
});
it('preserves reference definitions through explicit source fallback, including unused definitions', () => {
  expect(
    inspectMarkdown('A\n\n[unused]: https://example.com "keep"').supported,
  ).toBe(false);
  expect(
    inspectMarkdown('[Linked][ref]\n\n[ref]: https://example.com').supported,
  ).toBe(false);
});
