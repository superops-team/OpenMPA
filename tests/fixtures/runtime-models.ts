import type { RuntimeModelCatalog } from '../../src/client/desktop/model-catalog';
export const runtimeModels: RuntimeModelCatalog = {
  supported: true,
  defaultModel: 'deepseek-v4-pro',
  models: [
    { id: 'deepseek-v4-pro' },
    { id: 'deepseek-v4-flash' },
    { id: 'doubao-seed-2-1-pro' },
    { id: 'runtime-custom-model-v2' },
  ],
};
