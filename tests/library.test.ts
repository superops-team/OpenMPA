import { expect, it } from 'vitest';
import { pageExcerpt } from '../src/client/SpaceLibrary';
it('shows readable library previews without changing source content', () => {
  const source =
    '---\ntitle: Metadata\n---\n\n# Checklist\n- [ ] First task\n- [x] **Done**\n\n<details><summary>Context</summary>Notes</details>';
  expect(pageExcerpt(source)).toBe('Checklist First task Done Context Notes');
  expect(source).toContain('<details>');
  expect(source).toContain('- [ ]');
});
