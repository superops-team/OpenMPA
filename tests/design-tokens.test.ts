import { readFileSync } from 'node:fs';
import { expect, it } from 'vitest';
import { contrastRatio, parseDesignTokens } from '../src/design/tokens';

const source = () =>
  JSON.parse(
    readFileSync(
      new URL('../design/react-desktop-v2.tokens.json', import.meta.url),
      'utf8',
    ),
  );

it('validates all semantic roles and readable color pairs in both themes', () => {
  expect(parseDesignTokens(source()).version).toBe('2.1.1');
  expect(contrastRatio('#000000', '#ffffff')).toBeCloseTo(21);
});

it('rejects missing dark roles instead of falling back to light', () => {
  const tokens = source();
  delete tokens.theme.dark['warning-foreground'];
  expect(() => parseDesignTokens(tokens)).toThrow();
});

it('rejects low contrast text even when the CSS would compile', () => {
  const tokens = source();
  tokens.theme.light['foreground-subtlest'] = '#aaaaaa';
  expect(() => parseDesignTokens(tokens)).toThrow(/contrast/);
});

it('rejects invalid dimensions, unexpected roles and unsafe color values', () => {
  for (const mutate of [
    (t: ReturnType<typeof source>) => {
      t.control.input = 'huge';
    },
    (t: ReturnType<typeof source>) => {
      t.theme.dark.randomAccent = '#ffffff';
    },
    (t: ReturnType<typeof source>) => {
      t.theme.light.primary = 'url(https://example.com)';
    },
  ]) {
    const tokens = source();
    mutate(tokens);
    expect(() => parseDesignTokens(tokens)).toThrow();
  }
});
