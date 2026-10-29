import { useEffect, useRef, useState } from 'react';
import { ModalDialog } from '../ui/ModalDialog';
import { Button } from '../ui';
import { useMessages, useMessageState, localized } from './locale';
import { studioFailure } from './studio-services';
import { LifecycleStage, LifecycleStatus } from './LifecycleStatus';
import {
  MpaDeletionDiagnostic,
  deletionErrorDiagnostic,
} from './MpaDeletionDiagnostic';
import type {
  MpaDeletePreview,
  MpaDeletionTask,
  MpaLifecycleServices,
} from './lifecycle-models';

export function MpaDeleteDialog({
  services,
  epoch,
  region,
  runtimeId,
  name,
  task: initialTask,
  onClose,
  onChanged,
  onDeleted,
}: {
  services: MpaLifecycleServices;
  epoch: number;
  region: string;
  runtimeId: string;
  name: string;
  task?: MpaDeletionTask;
  onClose(): void;
  onChanged(): void;
  onDeleted(runtimeId: string): Promise<void>;
}) {
  const m = useMessages();
  const [preview, setPreview] = useState<MpaDeletePreview>();
  const [task, setTask] = useState(initialTask);
  const [busy, setBusy] = useState(false);
  const [uncertain, setUncertain] = useState(false);
  const [notice, setNotice] = useMessageState();
  const [failure, setFailure] = useState<unknown>();
  const report = (error: unknown) => {
    setFailure(error);
    setNotice(deletionErrorDiagnostic(error) ? '' : studioFailure(error));
  };
  const operationId = useRef(crypto.randomUUID());
  const alive = useRef(true);
  const writing = useRef(false);
  const inspecting = useRef(false);
  const notified = useRef(false);
  const callbacks = useRef({ onChanged, onDeleted });
  callbacks.current = { onChanged, onDeleted };
  useEffect(() => {
    alive.current = true;
    return () => {
      alive.current = false;
    };
  }, []);
  const accept = async (value: MpaDeletionTask) => {
    if (!alive.current) return;
    setTask(value);
    setUncertain(false);
    if (value.state !== 'running') callbacks.current.onChanged();
    if (value.state === 'succeeded' && !notified.current) {
      notified.current = true;
      try {
        await callbacks.current.onDeleted(runtimeId);
      } catch (error) {
        if (alive.current) {
          notified.current = false;
          setNotice(studioFailure(error));
        }
      }
    }
  };
  const read = async () => {
    setBusy(true);
    setNotice('');
    setFailure(undefined);
    try {
      await accept(
        await services.readDeletion(
          epoch,
          region,
          task?.operationId || operationId.current,
        ),
      );
    } catch (error) {
      if (alive.current) report(error);
    } finally {
      if (alive.current) setBusy(false);
    }
  };
  const inspect = async () => {
    if (writing.current || inspecting.current) return;
    inspecting.current = true;
    setBusy(true);
    setNotice('');
    setFailure(undefined);
    setPreview(undefined);
    try {
      // Resolve an ambiguous receipt read-only before preparing a new intent.
      if (task || uncertain) {
        try {
          const current = await services.readDeletion(
            epoch,
            region,
            task?.operationId || operationId.current,
          );
          if (current.state === 'running' || current.state === 'succeeded') {
            await accept(current);
            return;
          }
        } catch (error) {
          // The engine persists acceptance before any cloud writes. A missing
          // original receipt permits a new preview; other failures do not.
          if (!(
            error &&
            typeof error === 'object' &&
            'code' in error &&
            error.code === 'resourceNotVisible' &&
            'httpStatus' in error &&
            error.httpStatus === 404
          ))
            throw error;
        }
      }
      const value = await services.previewDelete(epoch, region, runtimeId);
      if (alive.current) {
        setPreview(value);
        setFailure(undefined);
        setNotice('');
        setTask(undefined);
        setUncertain(false);
        operationId.current = crypto.randomUUID();
      }
    } catch (error) {
      if (alive.current) {
        setPreview(undefined);
        report(error);
      }
    } finally {
      inspecting.current = false;
      if (alive.current) setBusy(false);
    }
  };
  useEffect(() => {
    if (!initialTask) void inspect();
    else if (initialTask.state === 'succeeded') void accept(initialTask);
  }, []);
  useEffect(() => {
    if (!task || task.state !== 'running') return;
    let active = true;
    let timer: ReturnType<typeof setTimeout>;
    const poll = async () => {
      try {
        const value = await services.readDeletion(
          epoch,
          region,
          task.operationId,
        );
        if (active) await accept(value);
      } catch (error) {
        if (active) setNotice(studioFailure(error));
      }
      if (active) timer = setTimeout(poll, 2000);
    };
    timer = setTimeout(poll, 1000);
    return () => {
      active = false;
      clearTimeout(timer);
    };
  }, [services, epoch, region, task?.operationId, task?.state]);
  const submit = async () => {
    if (busy || writing.current || !preview?.canDelete || task || uncertain)
      return;
    writing.current = true;
    setBusy(true);
    setNotice('');
    setFailure(undefined);
    try {
      await accept(await services.delete(epoch, preview, operationId.current));
      callbacks.current.onChanged();
    } catch (error) {
      if (alive.current) {
        const status =
          error && typeof error === 'object' && 'httpStatus' in error
            ? Number(error.httpStatus)
            : 0;
        const preflightRejected =
          error &&
          typeof error === 'object' &&
          'code' in error &&
          error.code === 'deletePreviewFailed';
        const rejected = preflightRejected || (status >= 400 && status < 500);
        setUncertain(!rejected);
        setPreview(undefined);
        setFailure(error);
        setNotice(
          rejected
            ? localized((m) => m.mpaDeleteChanged())
            : localized((m) => m.mpaDeleteUnknown()),
        );
      }
    } finally {
      writing.current = false;
      if (alive.current) setBusy(false);
    }
  };
  return (
    <ModalDialog
      title={`${m.mpaDelete()} · ${name}`}
      description={m.mpaDeleteHint()}
      closeLabel={m.mpaClose()}
      onClose={onClose}
      footer={
        <>
          <Button variant="outline" onClick={onClose}>
            {m.mpaClose()}
          </Button>
          {preview && (
            <Button
              variant="destructive"
              disabled={
                busy || !preview.canDelete || Boolean(task) || uncertain
              }
              onClick={() => void submit()}
            >
              {m.mpaDeleteConfirm()}
            </Button>
          )}
        </>
      }
    >
      <div className="mpa-dialog-content">
        <p className="mpa-identifier">
          {runtimeId} · {region}
        </p>
        {notice && (
          <p role="status" className="mpa-notice">
            {notice}
          </p>
        )}
        {busy && <p role="status">{m.mpaDeleteChecking()}</p>}
        <MpaDeletionDiagnostic
          key={JSON.stringify(
            deletionErrorDiagnostic(failure) || task?.latestDiagnostic,
          )}
          diagnostic={
            deletionErrorDiagnostic(failure) || task?.latestDiagnostic
          }
        />
        {preview && (
          <>
            <p>
              {m.mpaDeleteSessions(
                String(preview.sessionCounts.total),
                String(preview.sessionCounts.active),
                String(preview.sessionCounts.idle),
              )}
            </p>
            {preview.blockers.map((code) => (
              <p role="alert" key={code}>
                {code === 'active_sessions'
                  ? m.mpaDeleteActive()
                  : m.mpaDeleteBusy()}
              </p>
            ))}
            <h3>{m.mpaDeletePlan()}</h3>
            <ol className="mpa-cleanup-plan">
              {preview.cleanupPlan.map((item, index) => (
                <li key={`${item.stage}:${index}`}>
                  <LifecycleStage deleting stage={item.stage} />
                  {item.resourceId && <code>{item.resourceId}</code>}
                  {item.count > 0 && <span> × {item.count}</span>}
                </li>
              ))}
            </ol>
            <h3>{m.mpaDeleteRetained()}</h3>
            <p>{m.mpaDeleteSharedHint()}</p>
            {preview.retainedResources.length > 0 && (
              <ul>
                {preview.retainedResources.map((item, index) => (
                  <li className="mpa-identifier" key={index}>
                    {item.resourceId}
                  </li>
                ))}
              </ul>
            )}
          </>
        )}
        {task && (
          <div role="status" className="mpa-task-progress">
            <LifecycleStatus state={task.state} />
            <LifecycleStage deleting stage={task.stage} />
            {task.state === 'running' && (
              <progress aria-label={m.mpaOpenTask()} />
            )}
            {task.error && <p className="mpa-identifier">{task.error}</p>}
          </div>
        )}
        {(uncertain || task?.state === 'unknown') && (
          <p>{m.mpaDeleteUnknown()}</p>
        )}
        {task?.state === 'running' && <p>{m.mpaBackgroundHint()}</p>}
        <div className="studio-actions">
          {(task || uncertain) && (
            <Button
              variant="outline"
              disabled={busy}
              onClick={() => void read()}
            >
              {m.mpaReadStatus()}
            </Button>
          )}
          {(!task || ['failed', 'unknown'].includes(task.state)) && (
            <Button
              variant="outline"
              disabled={busy}
              onClick={() => void inspect()}
            >
              {m.mpaDeleteRecheck()}
            </Button>
          )}
        </div>
      </div>
    </ModalDialog>
  );
}
