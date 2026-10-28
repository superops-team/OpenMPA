/**
 * Snapshot of the Runtime-owned Codex model catalog.
 *
 * Source: agentkit-mpa-agent/app/runtime/main/codex-model-catalog.json on the
 * deployed Runtime. The Runtime does not expose a catalog endpoint yet, so the
 * client must not accept arbitrary model IDs. Replace this snapshot with a
 * Runtime catalog binding when that contract exists.
 */
export interface ModelOption {
  id: string;
  family: string;
  contextWindow: number;
}

export const runtimeModelCatalog: ModelOption[] = [
  { id: 'deepseek-v4-flash', family: 'DeepSeek', contextWindow: 1_000_000 },
  {
    id: 'deepseek-v4-flash-260425',
    family: 'DeepSeek',
    contextWindow: 1_000_000,
  },
  { id: 'deepseek-v4-flash-ga', family: 'DeepSeek', contextWindow: 1_000_000 },
  {
    id: 'deepseek-v4-flash-ga-260731',
    family: 'DeepSeek',
    contextWindow: 1_000_000,
  },
  { id: 'deepseek-v4-pro', family: 'DeepSeek', contextWindow: 1_000_000 },
  {
    id: 'deepseek-v4-pro-260425',
    family: 'DeepSeek',
    contextWindow: 1_000_000,
  },
  { id: 'deepseek-v4-pro-ga', family: 'DeepSeek', contextWindow: 1_000_000 },
  {
    id: 'deepseek-v4-pro-ga-260813',
    family: 'DeepSeek',
    contextWindow: 1_000_000,
  },
  { id: 'doubao-seed-2-0-code', family: 'Doubao', contextWindow: 256_000 },
  {
    id: 'doubao-seed-2-0-code-preview-260215',
    family: 'Doubao',
    contextWindow: 256_000,
  },
  { id: 'doubao-seed-2-0-lite', family: 'Doubao', contextWindow: 256_000 },
  {
    id: 'doubao-seed-2-0-lite-260428',
    family: 'Doubao',
    contextWindow: 256_000,
  },
  { id: 'doubao-seed-2-0-pro', family: 'Doubao', contextWindow: 256_000 },
  {
    id: 'doubao-seed-2-0-pro-260215',
    family: 'Doubao',
    contextWindow: 256_000,
  },
  { id: 'doubao-seed-2-1-pro', family: 'Doubao', contextWindow: 256_000 },
  {
    id: 'doubao-seed-2-1-pro-260628',
    family: 'Doubao',
    contextWindow: 256_000,
  },
  { id: 'glm-4-7-251222', family: 'GLM', contextWindow: 200_000 },
  { id: 'glm-5-2', family: 'GLM', contextWindow: 1_000_000 },
  { id: 'glm-5-2-260617', family: 'GLM', contextWindow: 1_000_000 },
  { id: 'kimi-k2', family: 'Kimi', contextWindow: 128_000 },
  { id: 'kimi-k2-thinking-251104', family: 'Kimi', contextWindow: 128_000 },
  { id: 'dola-seed-2-1-turbo-260628', family: 'Dola', contextWindow: 256_000 },
];

export function isKnownModel(id?: string) {
  return !!id && runtimeModelCatalog.some((model) => model.id === id);
}

export function filterModels(query: string) {
  const value = query.trim().toLowerCase();
  if (!value) return runtimeModelCatalog;
  return runtimeModelCatalog.filter((model) =>
    `${model.id} ${model.family}`.toLowerCase().includes(value),
  );
}
