import { expect, it } from 'vitest';
import {
  filterModels,
  isKnownModel,
  runtimeModelCatalog,
} from '../src/client/desktop/model-catalog';

it('recognizes Runtime catalog models and rejects foreign model IDs', () => {
  expect(isKnownModel('deepseek-v4-pro')).toBe(true);
  expect(isKnownModel('doubao-seed-2-1-pro')).toBe(true);
  expect(isKnownModel('gpt-5.6-luna')).toBe(false);
  expect(isKnownModel('')).toBe(false);
});

it('filters by model ID and family', () => {
  expect(
    filterModels('doubao').every((model) => model.family === 'Doubao'),
  ).toBe(true);
  expect(filterModels('kimi').map((model) => model.id)).toEqual([
    'kimi-k2',
    'kimi-k2-thinking-251104',
  ]);
  expect(filterModels('no-such-model')).toEqual([]);
});

it('keeps the catalog free of duplicate IDs', () => {
  const ids = runtimeModelCatalog.map((model) => model.id);
  expect(new Set(ids).size).toBe(ids.length);
});
