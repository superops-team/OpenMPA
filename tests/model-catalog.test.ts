import { expect, it } from 'vitest';
import {
  filterModels,
  isKnownModel,
} from '../src/client/desktop/model-catalog';

const advertised = [
  { id: 'runtime-custom-model-v2' },
  { id: 'deepseek-v4-pro' },
];
it('accepts the current Runtime model even when absent from the old snapshot', () => {
  expect(isKnownModel('runtime-custom-model-v2', advertised)).toBe(true);
  expect(isKnownModel('doubao-seed-2-1-pro', advertised)).toBe(false);
  expect(isKnownModel('runtime-custom-model-v2')).toBe(false);
});
it('filters only the provided Runtime models and never invents fallback models', () => {
  expect(filterModels('CUSTOM', advertised)).toEqual([
    { id: 'runtime-custom-model-v2' },
  ]);
  expect(filterModels('', advertised)).toEqual(advertised);
  expect(filterModels('')).toEqual([]);
});
