import { expect, it } from 'vitest';
import {
  filterModels,
  isKnownModel,
  runtimeModelCatalog,
} from '../src/client/desktop/model-catalog';

it('does not ship a static model catalog fallback', () => {
  expect(runtimeModelCatalog).toEqual([]);
  expect(isKnownModel('doubao-seed-2-1-pro')).toBe(false);
  expect(isKnownModel('deepseek-v4-pro')).toBe(false);
  expect(isKnownModel('gpt-5.6-luna')).toBe(false);
  expect(isKnownModel('')).toBe(false);
});

it('recognizes dynamic Ark Responses catalog IDs from the backend', () => {
  const catalog = [
    { id: 'deepseek-v4-pro-260101' },
    { id: 'kimi-k2-6-modelhub' },
    { id: 'glm-5-2-20261020' },
  ];
  expect(isKnownModel('deepseek-v4-pro-260101', catalog)).toBe(true);
  expect(filterModels('k2', catalog).map((model) => model.id)).toEqual([
    'kimi-k2-6-modelhub',
  ]);
});

it('filters the supplied dynamic catalog by input', () => {
  const catalog = [
    { id: 'doubao-seed-2-1-pro-260628' },
    { id: 'deepseek-v4-pro-260101' },
  ];
  expect(filterModels('doubao', catalog).map((model) => model.id)).toEqual([
    'doubao-seed-2-1-pro-260628',
  ]);
  expect(filterModels('deepseek', catalog).map((model) => model.id)).toEqual([
    'deepseek-v4-pro-260101',
  ]);
  expect(filterModels('no-such-model')).toEqual([]);
});

it('keeps the catalog free of duplicate IDs', () => {
  const ids = runtimeModelCatalog.map((model) => model.id);
  expect(new Set(ids).size).toBe(ids.length);
});
