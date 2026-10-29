import type {
  Config,
  CreationInput,
  CreationTask,
  DeletePreview,
  DeletionTask,
} from './generated/mygo';

export type MpaCreationConfig = Config;
export type MpaCreationInput = CreationInput;
export type MpaCreationTask = CreationTask;
export type MpaDeletePreview = DeletePreview;
export type MpaDeletionTask = DeletionTask;
export interface MpaLifecycleServices {
  tosBuckets(
    epoch: number,
    region: string,
  ): Promise<Array<{ name: string; region: string }>>;
  config(epoch: number, region: string): Promise<MpaCreationConfig>;
  checkName(
    epoch: number,
    region: string,
    requestId: string,
    name: string,
  ): Promise<void>;
  create(epoch: number, input: MpaCreationInput): Promise<MpaCreationTask>;
  creations(epoch: number, region: string): Promise<MpaCreationTask[]>;
  readCreation(
    epoch: number,
    region: string,
    taskId: string,
  ): Promise<MpaCreationTask>;
  cancelCreation(
    epoch: number,
    region: string,
    taskId: string,
  ): Promise<MpaCreationTask>;
  previewDelete(
    epoch: number,
    region: string,
    runtimeId: string,
  ): Promise<MpaDeletePreview>;
  delete(
    epoch: number,
    preview: MpaDeletePreview,
    operationId: string,
  ): Promise<MpaDeletionTask>;
  deletions(epoch: number, region: string): Promise<MpaDeletionTask[]>;
  readDeletion(
    epoch: number,
    region: string,
    operationId: string,
  ): Promise<MpaDeletionTask>;
}
