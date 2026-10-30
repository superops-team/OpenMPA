import type { RegionVM } from './models';

// Mirrors the fixed native catalog. Used while the native bridge loads.
export const cloudRegions: RegionVM[] = [
  {
    provider: 'volcengine',
    id: 'cn-beijing',
    name: 'Beijing',
    endpointHost: 'agentkit.cn-beijing.volcengineapi.com',
  },
  {
    provider: 'volcengine',
    id: 'cn-shanghai',
    name: 'Shanghai',
    endpointHost: 'agentkit.cn-shanghai.volcengineapi.com',
  },
  {
    provider: 'byteplus',
    id: 'ap-southeast-1',
    name: 'Singapore',
    endpointHost: 'agentkit.ap-southeast-1.byteplusapi.com',
  },
];
export function regionsForProvider(provider = 'volcengine') {
  return cloudRegions.filter((region) => region.provider === provider);
}
