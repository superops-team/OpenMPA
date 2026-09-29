import { expect, it } from 'vitest';
import { currentTurnText } from '../src/server/headless.js';
it('rejects a swallowed SDK failure rather than reusing a previous answer', () => {
  expect(() => currentTurnText([], new Error('Provider failed'))).toThrow(
    'Provider failed',
  );
  expect(() => currentTurnText([])).toThrow('current compute turn');
  expect(
    currentTurnText([
      { id: 'new', role: 'assistant', content: 'Current answer' },
    ]),
  ).toBe('Current answer');
});
