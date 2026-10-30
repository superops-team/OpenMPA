import { expect, it } from 'vitest';
import { checkTokenUsage, declaredDesignTokens } from '../src/design/usage';

const declared = declaredDesignTokens(
  ':root { --color-panel: #fff; --radius-lg: 8px; }',
);

it('checks declarations separately even on one line or across lines', () => {
  expect(
    checkTokenUsage(
      '.card { background: var(--color-panel); border-radius: 7px; }',
      declared,
    ),
  ).toEqual(['line 1: border-radius does not use a semantic token']);
  expect(
    checkTokenUsage(
      '.card { box-shadow:\n 0 2px 10px rgba(0,0,0,.2); }',
      declared,
    ),
  ).toHaveLength(2);
});

it('rejects misspelled references while allowing declared semantics and forced colors', () => {
  expect(
    checkTokenUsage('.card { color: var(--color-nonexistent); }', declared)[0],
  ).toContain('unknown token');
  expect(
    checkTokenUsage(
      '.card { background: var(--color-panel); border-radius: var(--radius-lg); outline-color: CanvasText; }',
      declared,
    ),
  ).toEqual([]);
});
