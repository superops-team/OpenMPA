import { MPALifecycle } from './generated/mygo';
import { unwrap } from './result';
import type { MpaLifecycleServices } from './lifecycle-models';

export function createLifecycleServices(
  bridge?: typeof MPALifecycle,
): MpaLifecycleServices {
  const native = () => bridge ?? MPALifecycle;
  return {
    tosBuckets: async (platformEpoch, region) => {
      const value = unwrap(
        await native().listTOSBuckets({ platformEpoch, region }),
      );
      if (value.platformEpoch !== platformEpoch || value.region !== region) {
        throw Object.assign(new Error('Stale bucket directory'), {
          code: 'staleConnection',
        });
      }
      return value.buckets;
    },
    config: async (platformEpoch, region) =>
      unwrap(await native().config({ platformEpoch, region })),
    checkName: async (platformEpoch, region, requestId, name) => {
      unwrap(
        await native().checkName({ platformEpoch, region, requestId, name }),
      );
    },
    create: async (platformEpoch, input) =>
      unwrap(await native().create({ platformEpoch, input })),
    creations: async (platformEpoch, region) =>
      unwrap(await native().listCreations({ platformEpoch, region })).tasks,
    readCreation: async (platformEpoch, region, taskId) =>
      unwrap(await native().readCreation({ platformEpoch, region, taskId })),
    cancelCreation: async (platformEpoch, region, taskId) =>
      unwrap(await native().cancelCreation({ platformEpoch, region, taskId })),
    previewDelete: async (platformEpoch, region, runtimeId) =>
      unwrap(
        await native().previewDelete({ platformEpoch, region, runtimeId }),
      ),
    delete: async (platformEpoch, preview, operationId) =>
      unwrap(
        await native().delete({
          platformEpoch,
          region: preview.region,
          runtimeId: preview.runtimeId,
          fingerprint: preview.fingerprint,
          operationId,
          confirm: true,
        }),
      ),
    deletions: async (platformEpoch, region) =>
      unwrap(await native().listDeletions({ platformEpoch, region })).tasks,
    readDeletion: async (platformEpoch, region, operationId) =>
      unwrap(
        await native().readDeletion({ platformEpoch, region, operationId }),
      ),
  };
}
