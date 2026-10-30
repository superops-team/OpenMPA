import type { MpaCreationInput, MpaCreationTask } from './lifecycle-models';

export const creationActive = (state: string) =>
  state === 'running' || state === 'cancelling';
export const creationTerminal = (state: string) =>
  ['succeeded', 'failed', 'cancelled'].includes(state);
export function creationInput(task: MpaCreationTask): MpaCreationInput {
  return {
    requestId: task.requestId,
    name: task.name,
    description: task.description,
    region: task.region,
    maxInstance: task.maxInstance,
    pgHost: task.pgHost,
    pgPort: task.pgPort,
    openvikingMode: task.openvikingMode || 'manual',
    openvikingUrl: task.openvikingUrl,
    openvikingResourceId: task.openvikingResourceId,
    businessWorkspaceMode: task.businessWorkspaceMode || 'shared',
    businessDatabaseName: task.businessDatabaseName,
    tosBucket: task.tosBucket,
  };
}
export function safeCreationDraft(input: MpaCreationInput): MpaCreationInput {
  // Explicit allowlist: secrets and unrecognized properties never enter storage.
  return creationInput({ ...input, taskId: '', state: '', stage: '' });
}
export function restoreCreationDraft(
  key: string,
  region: string,
): { input: MpaCreationInput; submitted: boolean } | undefined {
  try {
    const value = JSON.parse(sessionStorage.getItem(key) || 'null');
    if (
      !value ||
      typeof value !== 'object' ||
      !value.input ||
      value.input.region !== region ||
      !/^[0-9a-f-]{36}$/i.test(value.input.requestId) ||
      typeof value.input.name !== 'string' ||
      typeof value.input.description !== 'string' ||
      !Number.isSafeInteger(value.input.maxInstance)
    )
      return;
    return {
      input: safeCreationDraft(value.input),
      submitted: value.submitted === true,
    };
  } catch {
    return;
  }
}
export function persistCreationDraft(
  key: string,
  input: MpaCreationInput,
  submitted: boolean,
) {
  try {
    sessionStorage.setItem(
      key,
      JSON.stringify({ input: safeCreationDraft(input), submitted }),
    );
  } catch {
    /* The native creation journal remains authoritative after submission. */
  }
}
