/** Host filters Runtime model choices by the Studio account catalog when available. */
export interface ModelOption {
  id: string;
}

export interface RuntimeModelCatalog {
  models: ModelOption[];
  defaultModel: string;
  supported: boolean;
}

export function isKnownModel(
  id: string | undefined,
  models: readonly ModelOption[] = [],
) {
  return !!id && models.some((model) => model.id === id);
}

export function filterModels(
  query: string,
  models: readonly ModelOption[] = [],
) {
  const value = query.trim().toLowerCase();
  return value
    ? models.filter((model) => model.id.toLowerCase().includes(value))
    : models;
}
