/** Model choices are read from the selected Runtime's Agent Card, like Studio. */
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
