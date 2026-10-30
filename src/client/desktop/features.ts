import { Channels, Connection, Deployment, Schedule } from './generated/mygo';
import { unwrap } from './result';
import { parseProfile } from './profile';
import type { DesktopServices, ScheduleVM } from './models';
import { localized, LocalizedError } from './locale';
type NativeTask = NonNullable<
  Awaited<ReturnType<typeof Schedule.read>>['value']
>['task'];
type NativeImages = NonNullable<
  Awaited<ReturnType<typeof Deployment.readImages>>['value']
>;
const taskVM = (task: NativeTask): ScheduleVM => ({
  id: task.id,
  name: task.name,
  prompt: task.prompt,
  agentId: task.agentId,
  enabled: task.enabled,
  version: String(task.version),
  schedule: {
    ...task.schedule,
    intervalSeconds: task.schedule.intervalSeconds ?? undefined,
  },
  timezone: task.schedule.timezone || 'Asia/Shanghai',
  nextRun: task.nextRunAt ?? undefined,
  status: task.lastRunStatus ?? undefined,
  lastRun: task.lastRunAt ?? undefined,
  runningAt: task.runningAt ?? undefined,
  createdAt: task.createdAt,
  jitterSeconds: task.jitterSeconds,
  timeoutSeconds: task.timeoutSeconds,
  delivery: task.delivery,
});
const config = (task: Pick<ScheduleVM, 'schedule' | 'timezone'>) => ({
  ...task.schedule,
  timezone: task.timezone,
});
export function desktopFeatures(): Pick<
  DesktopServices,
  'schedules' | 'channels' | 'deployment'
> {
  const images = new Map<string, NativeImages>();
  const pendingCreates = new Map<
    number,
    Parameters<typeof Schedule.create>[0]
  >();
  const pendingRuns = new Map<string, Parameters<typeof Schedule.run>[0]>();
  const target = async (runtimeId: string) => {
    const state = unwrap(await Connection.status());
    if (!state.connected || state.runtimeId !== runtimeId)
      throw new LocalizedError(
        localized((m) => m.desktopRuntimeChangedReadAndReviewImagesAgain()),
      );
    return state.connectionEpoch;
  };
  const describeOperation = (
    operation: NonNullable<
      Awaited<ReturnType<typeof Deployment.readUpdate>>['value']
    >,
  ) =>
    localized((m) =>
      m.deploymentOperationSummary(
        operation.accepted ? m.deploymentAccepted() : m.deploymentNotAccepted(),
        operation.state,
        operation.runtimeReady ? m.pageYes() : m.pageNo(),
        operation.businessHealthVerified ? m.pageYes() : m.pageNo(),
        operation.workerConfigUpdated ? m.pageYes() : m.pageNo(),
        operation.existingSandboxesVerified ? m.pageYes() : m.pageNo(),
        operation.message,
      ),
    );
  return {
    schedules: {
      deliveryTargets: async (connectionEpoch, channel, appId) =>
        unwrap(
          await Schedule.deliveryTargets({
            connectionEpoch,
            channel,
            appId: appId || undefined,
          }),
        ).items,
      list: async (connectionEpoch) => {
        const all: NativeTask[] = [];
        let overview;
        let offset = 0;
        for (let page = 0; page < 100; page++) {
          const value = unwrap(
            await Schedule.list({
              connectionEpoch,
              includeDisabled: true,
              limit: 200,
              offset,
            }),
          );
          if (page === 0 && value.countAvailable) overview = value.overview;
          all.push(...value.items);
          if (value.hasMore === false)
            return {
              items: [
                ...new Map(all.map((task) => [task.id, taskVM(task)])).values(),
              ],
              overview: overview
                ? {
                    executionCount: overview.executionCount ?? undefined,
                    successRate: overview.successRate ?? undefined,
                  }
                : undefined,
            };
          if (value.hasMore === undefined)
            throw new LocalizedError(
              localized((m) =>
                m.desktopTheServerDidNotProvideAuthoritativeScheduledTaskPagination(),
              ),
            );
          const next = value.nextOffset;
          if (typeof next !== 'number' || next <= offset)
            throw new LocalizedError(
              localized((m) => m.desktopScheduledTaskPaginationDidNotAdvance()),
            );
          offset = next;
        }
        throw new LocalizedError(
          localized((m) => m.scheduleDirectoryTooLarge()),
        );
      },
      read: async (connectionEpoch, taskId) => {
        const task = unwrap(
          await Schedule.read({ connectionEpoch, taskId }),
        ).task;
        const value = unwrap(
          await Schedule.runs({
            connectionEpoch,
            taskId,
            limit: 200,
            offset: 0,
          }),
        );
        return {
          task: taskVM(task),
          count: value.countAvailable
            ? (value.total ?? value.overview?.executionCount)
            : undefined,
          hasMore: value.hasMore,
          nextOffset: value.nextOffset,
          runs: value.items.map((run) => ({
            id: run.id,
            status: run.status,
            startedAt: run.startedAt ?? undefined,
            error: run.errorCode ?? undefined,
            sessionId: run.sessionId ?? undefined,
            invocationId: run.invocationId ?? undefined,
            scheduledAt: run.scheduledAt,
            finishedAt: run.finishedAt ?? undefined,
            durationMs: run.durationMs ?? undefined,
          })),
        };
      },
      runs: async (connectionEpoch, taskId, offset) => {
        const value = unwrap(
          await Schedule.runs({ connectionEpoch, taskId, limit: 200, offset }),
        );
        return {
          count: value.countAvailable
            ? (value.total ?? value.overview?.executionCount)
            : undefined,
          hasMore: value.hasMore,
          nextOffset: value.nextOffset,
          runs: value.items.map((run) => ({
            id: run.id,
            status: run.status,
            startedAt: run.startedAt ?? undefined,
            error: run.errorCode ?? undefined,
            sessionId: run.sessionId ?? undefined,
            invocationId: run.invocationId ?? undefined,
            scheduledAt: run.scheduledAt,
            finishedAt: run.finishedAt ?? undefined,
            durationMs: run.durationMs ?? undefined,
          })),
        };
      },
      create: async (connectionEpoch, task) => {
        const pendingCreate = pendingCreates.get(connectionEpoch);
        const input = {
          connectionEpoch,
          clientToken: pendingCreate?.clientToken || crypto.randomUUID(),
          name: task.name,
          prompt: task.prompt,
          agentId: task.agentId || undefined,
          enabled: task.enabled,
          schedule: config(task),
          delivery: task.delivery,
          jitterSeconds: task.jitterSeconds,
          timeoutSeconds: task.timeoutSeconds,
        };
        if (
          pendingCreate &&
          JSON.stringify(input) !== JSON.stringify(pendingCreate)
        )
          throw new LocalizedError(
            localized((m) =>
              m.schedulePendingSubmission(pendingCreate.clientToken),
            ),
          );
        pendingCreates.set(connectionEpoch, input);
        try {
          const result = taskVM(unwrap(await Schedule.create(input)).task);
          pendingCreates.delete(connectionEpoch);
          return result;
        } catch (error) {
          if (
            !error ||
            typeof error !== 'object' ||
            !('retryDisposition' in error) ||
            error.retryDisposition === 'verifyOnly'
          )
            throw Object.assign(
              new LocalizedError(
                localized((m) =>
                  m.scheduleSubmissionUnknown(input.clientToken),
                ),
              ),
              error,
            );
          pendingCreates.delete(connectionEpoch);
          throw error;
        }
      },
      update: async (connectionEpoch, task) =>
        taskVM(
          unwrap(
            await Schedule.update({
              connectionEpoch,
              taskId: task.id,
              expectedVersion: Number(task.version),
              name: task.name,
              prompt: task.prompt,
              agentId: task.agentId || undefined,
              enabled: task.enabled,
              schedule: config(task),
              delivery: task.delivery,
              jitterSeconds: task.jitterSeconds,
              timeoutSeconds: task.timeoutSeconds,
            }),
          ).task,
        ),
      enable: async (connectionEpoch, task, enabled) =>
        taskVM(
          unwrap(
            await Schedule.update({
              connectionEpoch,
              taskId: task.id,
              expectedVersion: Number(task.version),
              enabled,
            }),
          ).task,
        ),
      trigger: async (connectionEpoch, task) => {
        const key = `${connectionEpoch}:${task.id}`;
        const input = pendingRuns.get(key) || {
          connectionEpoch,
          taskId: task.id,
          clientToken: crypto.randomUUID(),
          mode: 'Force',
        };
        pendingRuns.set(key, input);
        try {
          const run = unwrap(await Schedule.run(input)).run;
          pendingRuns.delete(key);
          return {
            id: run.id,
            status: run.status,
            startedAt: run.startedAt ?? undefined,
            error: run.errorCode ?? undefined,
            sessionId: run.sessionId ?? undefined,
            invocationId: run.invocationId ?? undefined,
          };
        } catch (error) {
          if (
            !error ||
            typeof error !== 'object' ||
            !('retryDisposition' in error) ||
            error.retryDisposition === 'verifyOnly'
          )
            throw Object.assign(
              new LocalizedError(
                localized((m) => m.scheduleTriggerUnknown(input.clientToken)),
              ),
              error,
            );
          pendingRuns.delete(key);
          throw error;
        }
      },
      remove: async (connectionEpoch, task) => {
        unwrap(await Schedule.delete({ connectionEpoch, taskId: task.id }));
      },
    },
    channels: {
      list: async (connectionEpoch) =>
        unwrap(await Channels.list({ connectionEpoch })).channels.map(
          (account) => ({
            id: account.appId || account.channel,
            domain:
              account.channel === 'feishu'
                ? account.config.domain || 'feishu'
                : undefined,
            bindingType: account.bindingType,
            addressable: !!account.appId,
            type: account.channel,
            name: account.appName || account.config.robotName || account.appId,
            enabled: account.enabled,
            status: 'configured',
            ownerId: account.ownerUserId,
          }),
        ),
      enable: async (connectionEpoch, channel, enabled) => {
        unwrap(
          await Channels.setEnabled({
            connectionEpoch,
            channel: channel.type,
            appId: channel.id,
            enabled,
          }),
        );
      },
      templates: async () => {
        throw new LocalizedError(
          localized((m) =>
            m.desktopTheSelectedRuntimeDoesNotExposeATemplateCatalog(),
          ),
        );
      },
      readProfile: async (connectionEpoch, channel) => {
        const profile = unwrap(
          await Channels.readProfile({
            connectionEpoch,
            channel: channel.type,
            appId: channel.id,
          }),
        );
        return {
          ...channel,
          version: String(profile.revision),
          profile: JSON.stringify(
            {
              agentsMd: profile.agentsMd,
              model: profile.model,
              skills: profile.skills,
              mcp: profile.mcp,
            },
            null,
            2,
          ),
        };
      },
      saveProfile: async (connectionEpoch, channel, text) => {
        if (!channel.version)
          throw new LocalizedError(
            localized((m) => m.desktopReadTheCurrentProfileBeforeSaving()),
          );
        unwrap(
          await Channels.saveProfile({
            connectionEpoch,
            channel: channel.type,
            appId: channel.id,
            expectedRevision: Number(channel.version),
            ...parseProfile(text),
          }),
        );
      },
      saveConfig: async (connectionEpoch, channel, secret) => {
        unwrap(
          await Channels.saveConfig({
            connectionEpoch,
            channel: channel.type,
            appId: channel.id,
            ...(channel.type.toLowerCase() === 'feishu'
              ? { appSecret: secret }
              : channel.type.toLowerCase() === 'dingtalk'
                ? { clientSecret: secret }
                : { secret }),
          }),
        );
      },
      remove: async (connectionEpoch, channel) => {
        unwrap(
          await Channels.remove({
            connectionEpoch,
            channel: channel.type,
            appId: channel.id,
          }),
        );
      },
      diagnostics: async (connectionEpoch, channel) =>
        JSON.stringify(
          unwrap(
            await Channels.diagnostics({
              connectionEpoch,
              channel: channel.type,
              appId: channel.id,
            }),
          ),
          null,
          2,
        ),
    },
    deployment: {
      updates: async (runtimeId) =>
        unwrap(
          await Deployment.listUpdates({
            connectionEpoch: await target(runtimeId),
          }),
        ).operations.map((operation) => ({
          operationId: operation.operationId,
          status: describeOperation(operation),
        })),
      images: async (runtimeId) => {
        const connectionEpoch = await target(runtimeId);
        const value = unwrap(await Deployment.readImages({ connectionEpoch }));
        images.set(runtimeId, value);
        return {
          runtimeId: value.runtimeId,
          runtimeVersion: value.runtimeVersion,
          runtimeImage: value.runtimeImage,
          toolId: value.toolId,
          toolVersion: value.workerUpdatedAt,
          toolImage: value.workerImage,
        };
      },
      runtime: async (input) => {
        const value = images.get(input.runtimeId);
        const connectionEpoch = await target(input.runtimeId);
        if (
          !value ||
          value.runtimeVersion !== input.expectedRuntimeVersion ||
          value.connectionEpoch !== connectionEpoch
        )
          throw new LocalizedError(
            localized((m) =>
              m.desktopReadAndReviewTheCurrentRuntimeImagesBeforeApplying(),
            ),
          );
        const operation = unwrap(
          await Deployment.updateRuntimeImage({
            connectionEpoch,
            operationId: crypto.randomUUID(),
            targetImage: input.runtimeImage,
            reviewFingerprint: value.reviewFingerprint,
            releaseEnable: true,
          }),
        );
        return operation.operationId;
      },
      worker: async (input) => {
        const value = images.get(input.runtimeId);
        const connectionEpoch = await target(input.runtimeId);
        if (
          !value ||
          value.toolId !== input.toolId ||
          value.workerUpdatedAt !== input.expectedToolVersion ||
          value.connectionEpoch !== connectionEpoch
        )
          throw new LocalizedError(
            localized((m) =>
              m.desktopTheToolBindingChangedReadAndReviewImagesAgain(),
            ),
          );
        const operation = unwrap(
          await Deployment.updateWorkerImage({
            connectionEpoch,
            operationId: crypto.randomUUID(),
            targetImage: input.toolImage,
            reviewFingerprint: value.reviewFingerprint,
          }),
        );
        return operation.operationId;
      },
      read: async (runtimeId, operationId) =>
        describeOperation(
          unwrap(
            await Deployment.readUpdate({
              connectionEpoch: await target(runtimeId),
              operationId,
            }),
          ),
        ),
    },
  };
}
