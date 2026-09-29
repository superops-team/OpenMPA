import { expect, it } from 'vitest';
import { contextualMessage } from '../src/client/page-context';
it('cannot send a page message before the authoritative context request completes', () => {
  expect(() => contextualMessage('Review this', undefined)).toThrow(/context/);
  expect(
    contextualMessage('Review this', {
      id: 'page',
      spaceId: 'space',
      title: 'Notes',
    }),
  ).toBe('From [Notes](/#/spaces/space/pages/page):\n\nReview this');
});
it('allows ordinary chat only after the server explicitly returns no page context', () => {
  expect(contextualMessage('Hello', null)).toBe('Hello');
});
