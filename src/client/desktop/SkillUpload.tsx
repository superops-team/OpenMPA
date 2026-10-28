import { useEffect, useRef, useState } from 'react';
import {
  Upload,
  FileArchive,
  CheckCircle2,
  LoaderCircle,
  AlertCircle,
} from 'lucide-react';
import { ModalDialog } from '../ui/ModalDialog';
import { useMessages, useMessageState, localized } from './locale';
import { studioFailure } from './studio-services';
import type {
  ResourcesServices,
  SkillSpaceVM,
  SkillUploadVM,
} from './studio-models';

export function SkillUploadButton({
  services,
  epoch,
  space,
  onUploaded,
  disabled,
}: {
  services: ResourcesServices;
  epoch: number;
  space?: SkillSpaceVM;
  onUploaded(spaceId: string): Promise<void>;
  disabled?: boolean;
}) {
  const [uploadSpace, setUploadSpace] = useState<SkillSpaceVM>();
  const destination = uploadSpace || space;
  const m = useMessages();
  const [open, setOpen] = useState(false);
  const [file, setFile] = useState<File>();
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<SkillUploadVM>();
  const [error, setError] = useMessageState();
  const [operation, setOperation] = useState('');
  const alive = useRef(true);
  const latest = useRef(onUploaded);
  latest.current = onUploaded;
  const notified = useRef('');
  const polling = useRef(false);
  useEffect(() => {
    alive.current = true;
    return () => {
      alive.current = false;
    };
  }, []);
  const active =
    !!result && ['uploading', 'processing'].includes(result.status);
  const accept = (value: SkillUploadVM) => {
    if (!alive.current) return;
    setResult(value);
    if (
      value.status === 'published' &&
      notified.current !== value.operationId
    ) {
      notified.current = value.operationId;
      void latest.current(value.spaceId).catch(() => {});
    }
  };
  const read = async () => {
    if (!services.readSkillUpload || !operation || polling.current) return;
    polling.current = true;
    try {
      accept(await services.readSkillUpload(epoch, operation));
      if (alive.current) setError('');
    } catch (reason) {
      if (alive.current) setError(studioFailure(reason));
    } finally {
      polling.current = false;
    }
  };
  useEffect(() => {
    if (!active || error) return;
    const timer = setTimeout(() => void read(), 2500);
    return () => clearTimeout(timer);
  }, [result, error]);
  const submit = async () => {
    if (
      busy ||
      !file ||
      !destination ||
      !services.uploadSkill ||
      result?.status === 'unknown'
    )
      return;
    const id = crypto.randomUUID();
    setOperation(id);
    setBusy(true);
    setError('');
    try {
      const encoded = await new Promise<string>((resolve, reject) => {
        const reader = new FileReader();
        reader.onerror = () => reject(new Error());
        reader.onload = () =>
          resolve(String(reader.result).split(',')[1] || '');
        reader.readAsDataURL(file);
      });
      if (!alive.current) return;
      accept(
        await services.uploadSkill(
          epoch,
          destination.id,
          id,
          file.name,
          encoded,
        ),
      );
    } catch (reason) {
      if (alive.current) {
        setError(studioFailure(reason));
        setResult({
          operationId: id,
          spaceId: destination.id,
          name: file.name,
          skillId: '',
          version: '',
          status:
            reason &&
            typeof reason === 'object' &&
            'code' in reason &&
            ['outcomeUnknown', 'staleConnection'].includes(String(reason.code))
              ? 'unknown'
              : 'failed',
        });
      }
    } finally {
      if (alive.current) setBusy(false);
    }
  };
  return (
    <>
      <button
        type="button"
        className="resource-upload-button"
        disabled={disabled || !destination || !services.uploadSkill}
        onClick={() => {
          if (!active && result?.status !== 'unknown') {
            setFile(undefined);
            setResult(undefined);
            setError('');
            setUploadSpace(space);
          }
          setOpen(true);
        }}
      >
        <Upload size={16} aria-hidden="true" />
        {active ? m.skillUploadProgress() : m.uploadBoundSkill()}
      </button>
      {open && (
        <ModalDialog
          title={m.uploadBoundSkill()}
          description={m.skillUploadTarget(
            destination?.name || destination?.id || '',
          )}
          closeLabel={m.skillUploadClose()}
          onClose={() => {
            if (!busy) setOpen(false);
          }}
          footer={
            <>
              <button
                type="button"
                className="resource-page-button"
                disabled={busy}
                onClick={() => setOpen(false)}
              >
                {m.skillUploadClose()}
              </button>
              {result?.status === 'published' ? (
                <span className="skill-upload-success">
                  <CheckCircle2 size={16} aria-hidden="true" />
                  {m.skillPublished()}
                </span>
              ) : result?.status === 'unknown' || (error && active) ? (
                <button
                  type="button"
                  className="resource-upload-button"
                  onClick={() => void read()}
                >
                  {m.skillReadUploadStatus()}
                </button>
              ) : (
                <button
                  type="button"
                  className="resource-upload-button"
                  disabled={busy || active || !file}
                  onClick={() => void submit()}
                >
                  {busy || active ? (
                    <LoaderCircle
                      size={16}
                      className="resource-spinning"
                      aria-hidden="true"
                    />
                  ) : (
                    <Upload size={16} aria-hidden="true" />
                  )}
                  {active ? m.skillPublishing() : m.skillUploadAndPublish()}
                </button>
              )}
            </>
          }
        >
          <div className="skill-upload-form">
            <div className="skill-upload-target">
              <strong>{destination?.name}</strong>
              <code>{destination?.id}</code>
            </div>
            <label className="skill-upload-dropzone">
              <FileArchive size={30} aria-hidden="true" />
              <strong>{file?.name || m.chooseSkillPackage()}</strong>
              <span>
                {file
                  ? `${(file.size / 1024).toFixed(1)} KB`
                  : m.skillPackageFormats()}
              </span>
              <input
                type="file"
                accept=".zip,.md"
                aria-label={m.chooseSkillPackage()}
                disabled={busy || active || result?.status === 'unknown'}
                onChange={(event) => {
                  const next = event.target.files?.[0];
                  setError('');
                  setResult(undefined);
                  if (next && next.size > 20 * 1024 * 1024) {
                    setFile(undefined);
                    setError(localized((m) => m.skillPackageTooLarge()));
                    return;
                  }
                  setFile(next);
                }}
              />
            </label>
            <p className="skill-upload-hint">{m.skillPackageRequirements()}</p>
            {result && (
              <div
                className={`skill-upload-status is-${result.status}`}
                role="status"
              >
                {result.status === 'published' ? (
                  <CheckCircle2 size={18} aria-hidden="true" />
                ) : result.status === 'unknown' ||
                  result.status === 'failed' ? (
                  <AlertCircle size={18} aria-hidden="true" />
                ) : (
                  <LoaderCircle
                    size={18}
                    className="resource-spinning"
                    aria-hidden="true"
                  />
                )}
                <div>
                  <strong>
                    {result.status === 'published'
                      ? m.skillPublished()
                      : result.status === 'unknown'
                        ? m.skillUploadUnknown()
                        : result.status === 'failed'
                          ? m.skillUploadFailed()
                          : m.skillPublishing()}
                  </strong>
                  {result.skillId && (
                    <code>
                      {result.skillId}
                      {result.version ? ` / ${result.version}` : ''}
                    </code>
                  )}
                </div>
              </div>
            )}
            {error && (
              <p className="skill-upload-error" role="alert">
                {error}
              </p>
            )}
          </div>
        </ModalDialog>
      )}
    </>
  );
}
