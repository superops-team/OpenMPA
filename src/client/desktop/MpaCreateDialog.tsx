import { useEffect, useRef, useState } from 'react';
import { ModalDialog } from '../ui/ModalDialog';
import { Button } from '../ui';
import { useMessages, useMessageState, localized } from './locale';
import { studioFailure } from './studio-services';
import { LifecycleStage, LifecycleStatus } from './LifecycleStatus';
import { MpaTosBucketPicker } from './MpaTosBucketPicker';
import { MpaCreationProgress } from './MpaCreationProgress';
import { MpaCreationDiagnostic } from './MpaCreationDiagnostic';
import {
  creationActive,
  creationTerminal,
  creationInput,
  persistCreationDraft,
  restoreCreationDraft,
} from './lifecycle-state';
import type {
  MpaCreationConfig,
  MpaCreationInput,
  MpaCreationTask,
  MpaLifecycleServices,
} from './lifecycle-models';

export function MpaCreateDialog({
  services,
  epoch,
  scope,
  region,
  task: initialTask,
  onClose,
  onChanged,
  onConnect,
}: {
  services: MpaLifecycleServices;
  epoch: number;
  scope: string;
  region: string;
  task?: MpaCreationTask;
  onClose(): void;
  onChanged(): void;
  onConnect(runtimeId: string): Promise<void>;
}) {
  const m = useMessages();
  const key = `openmpa:mpa-create:${scope}:${region}`;
  const [restored] = useState(() =>
    initialTask ? undefined : restoreCreationDraft(key, region),
  );
  const [input, setInput] = useState<MpaCreationInput>(() =>
    initialTask
      ? creationInput(initialTask)
      : restored?.input || {
          requestId: crypto.randomUUID(),
          region,
          name: '',
          description: '',
          maxInstance: 20,
          businessWorkspaceMode: 'per-agent',
          businessDatabaseName: 'aidb',
          openvikingMode: 'auto',
        },
  );
  const [step, setStep] = useState(0);
  const [config, setConfig] = useState<MpaCreationConfig>();
  const [task, setTask] = useState(initialTask);
  const [submitted, setSubmitted] = useState(
    Boolean(initialTask || restored?.submitted),
  );
  const [uncertain, setUncertain] = useState(Boolean(restored?.submitted));
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useMessageState();
  const alive = useRef(true);
  const writing = useRef(false);
  const changed = useRef(onChanged);
  changed.current = onChanged;
  useEffect(() => {
    alive.current = true;
    return () => {
      alive.current = false;
    };
  }, []);
  useEffect(() => {
    persistCreationDraft(key, input, submitted);
  }, [key, input, submitted]);
  const loadConfig = async () => {
    setBusy(true);
    setNotice('');
    try {
      const value = await services.config(epoch, region);
      if (alive.current) {
        setConfig(value);
        if (!value.configured)
          setNotice(localized((m) => m.mpaCreateUnconfigured()));
      }
    } catch (error) {
      if (alive.current) setNotice(studioFailure(error));
    } finally {
      if (alive.current) setBusy(false);
    }
  };
  useEffect(() => {
    void loadConfig();
  }, []);
  const applyTask = (value: MpaCreationTask) => {
    if (!alive.current) return;
    setTask(value);
    setUncertain(false);
    setSubmitted(true);
    if (creationTerminal(value.state)) changed.current();
  };
  const check = async () => {
    if (writing.current) return;
    setBusy(true);
    setNotice('');
    try {
      if (task)
        applyTask(await services.readCreation(epoch, region, task.taskId));
      else {
        const values = await services.creations(epoch, region);
        const found = values.find(
          (value) => value.requestId === input.requestId,
        );
        if (found) applyTask(found);
        else if (alive.current) {
          setUncertain(true);
          setNotice(localized((m) => m.mpaCreateUnknown()));
        }
      }
    } catch (error) {
      if (alive.current) setNotice(studioFailure(error));
    } finally {
      if (alive.current) setBusy(false);
    }
  };
  useEffect(() => {
    if (!task || !creationActive(task.state)) return;
    let active = true;
    let timer: ReturnType<typeof setTimeout>;
    const poll = async () => {
      try {
        const value = await services.readCreation(epoch, region, task.taskId);
        if (active) applyTask(value);
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
  }, [services, epoch, region, task?.taskId, task?.state]);
  const valid =
    /^[A-Za-z0-9_-]{4,64}$/.test(input.name) &&
    input.description.trim().length > 0 &&
    input.description.length <= 512 &&
    Number.isSafeInteger(input.maxInstance) &&
    input.maxInstance >= 1;
  const next = async () => {
    if (busy || writing.current || !valid) {
      setNotice(localized((m) => m.mpaInvalidForm()));
      return;
    }
    setBusy(true);
    setNotice('');
    try {
      if (step === 0)
        await services.checkName(epoch, region, input.requestId, input.name);
      if (alive.current) setStep((value) => Math.min(2, value + 1));
    } catch (error) {
      if (alive.current) setNotice(studioFailure(error));
    } finally {
      if (alive.current) setBusy(false);
    }
  };
  const manual = input.openvikingMode !== 'auto';
  const needsKey =
    manual && Boolean(input.openvikingUrl || input.openvikingResourceId);
  const resourcesValid =
    !manual ||
    (!input.openvikingUrl &&
      !input.openvikingResourceId &&
      !input.openvikingApiKey) ||
    (Boolean(
      input.openvikingUrl &&
      input.openvikingResourceId &&
      input.openvikingApiKey,
    ) &&
      /^https?:\/\//.test(input.openvikingUrl || ''));
  const tosValid =
    !input.tosBucket ||
    (input.tosBucket.startsWith('agentkit-platform-') &&
      /^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$/.test(input.tosBucket));
  const submit = async () => {
    if (
      busy ||
      writing.current ||
      !valid ||
      !resourcesValid ||
      !tosValid ||
      !config?.configured ||
      (task && creationActive(task.state))
    )
      return;
    writing.current = true;
    setBusy(true);
    setNotice('');
    setSubmitted(true);
    persistCreationDraft(key, input, true);
    try {
      const value = await services.create(epoch, input);
      applyTask(value);
      changed.current();
    } catch (error) {
      if (alive.current) {
        const status =
          error && typeof error === 'object' && 'httpStatus' in error
            ? Number(error.httpStatus)
            : 0;
        const rejected = status >= 400 && status < 500;
        setUncertain(!rejected);
        if (rejected && !task) setSubmitted(false);
        setNotice(
          rejected
            ? studioFailure(error)
            : localized((m) => m.mpaCreateUnknown()),
        );
      }
    } finally {
      writing.current = false;
      if (alive.current) setBusy(false);
    }
  };
  const editable = !submitted;
  const active = task && creationActive(task.state);
  const succeeded = task?.state === 'succeeded';
  const update = (value: Partial<MpaCreationInput>) =>
    setInput((previous) => ({ ...previous, ...value }));
  return (
    <ModalDialog
      title={m.mpaCreate()}
      description={m.mpaCreateHint()}
      closeLabel={m.mpaClose()}
      onClose={onClose}
      footer={
        <>
          <Button variant="outline" onClick={onClose}>
            {m.mpaClose()}
          </Button>
          {editable && step > 0 && (
            <Button
              variant="outline"
              disabled={busy}
              onClick={() => setStep(step - 1)}
            >
              {m.mpaBack()}
            </Button>
          )}
          {editable && step < 2 && (
            <Button
              variant="primary"
              disabled={busy || !config?.configured || (step === 0 && !valid)}
              onClick={() => void next()}
            >
              {m.mpaNext()}
            </Button>
          )}
          {editable && step === 2 && (
            <Button
              variant="primary"
              disabled={
                busy ||
                !config?.configured ||
                !valid ||
                !resourcesValid ||
                !tosValid
              }
              onClick={() => void submit()}
            >
              {m.mpaSubmit()}
            </Button>
          )}
        </>
      }
    >
      <div className="mpa-dialog-content">
        {notice && (
          <p role="status" className="mpa-notice">
            {notice}
          </p>
        )}
        {busy && !config && <p role="status">{m.mpaLoadingManagement()}</p>}
        {!config?.configured && (
          <Button
            variant="outline"
            disabled={busy}
            onClick={() => void loadConfig()}
          >
            {m.mpaRetryLoad()}
          </Button>
        )}
        {editable ? (
          <>
            <ol className="mpa-steps" aria-label={m.mpaCreate()}>
              {[m.mpaBasicStep(), m.mpaPgStep(), m.mpaOvStep()].map(
                (label, index) => (
                  <li
                    key={label}
                    aria-current={step === index ? 'step' : undefined}
                  >
                    <span>{index + 1}</span>
                    {label}
                  </li>
                ),
              )}
            </ol>
            <div className="mpa-fields">
              {step === 0 && (
                <>
                  <label>
                    {m.desktopName()}
                    <input
                      type="text"
                      autoCapitalize="none"
                      autoCorrect="off"
                      autoComplete="off"
                      spellCheck={false}
                      ref={(node) => {
                        node?.setAttribute('writingsuggestions', 'false');
                      }}
                      value={input.name}
                      required
                      maxLength={64}
                      pattern="[A-Za-z0-9_-]{4,64}"
                      onChange={(event) => update({ name: event.target.value })}
                    />
                  </label>
                  <p>{m.mpaNameHint()}</p>
                  <label>
                    {m.description()}
                    <textarea
                      required
                      maxLength={512}
                      value={input.description}
                      onChange={(event) =>
                        update({ description: event.target.value })
                      }
                    />
                  </label>
                  <label>
                    {m.maximumInstances()}
                    <input
                      required
                      type="number"
                      min={1}
                      step={1}
                      value={input.maxInstance}
                      onChange={(event) =>
                        update({ maxInstance: Number(event.target.value) })
                      }
                    />
                  </label>
                  <p>
                    {region} · {m.mpaDefaultImages()}
                  </p>
                </>
              )}
              {step === 1 && (
                <>
                  <h3>{m.mpaPgStep()}</h3>
                  <p>
                    {config?.postgresMode === 'auto'
                      ? m.mpaAutoPgHint()
                      : m.mpaManualPgHint()}
                  </p>
                  {config?.postgresMode !== 'auto' && (
                    <>
                      <label>
                        {m.mpaPgHost()}
                        <input
                          maxLength={255}
                          value={input.pgHost || ''}
                          placeholder={config?.pgHost}
                          onChange={(event) =>
                            update({ pgHost: event.target.value })
                          }
                        />
                      </label>
                      <label>
                        {m.mpaPgPort()}
                        <input
                          inputMode="numeric"
                          maxLength={5}
                          value={input.pgPort || ''}
                          placeholder={config?.pgPort}
                          onChange={(event) =>
                            update({ pgPort: event.target.value })
                          }
                        />
                      </label>
                    </>
                  )}
                </>
              )}
              {step === 2 && (
                <>
                  <label>
                    {m.mpaOvStep()}
                    <select
                      value={input.openvikingMode || 'auto'}
                      onChange={(event) =>
                        update({
                          openvikingMode: event.target.value,
                          openvikingUrl: '',
                          openvikingResourceId: '',
                          openvikingApiKey: '',
                        })
                      }
                    >
                      <option value="auto">{m.mpaAutoOv()}</option>
                      <option value="manual">{m.mpaManualOv()}</option>
                    </select>
                  </label>
                  <p>{manual ? m.mpaManualOvHint() : m.mpaAutoOvHint()}</p>
                  {manual && (
                    <>
                      <label>
                        {m.mpaOvUrl()}
                        <input
                          type="url"
                          maxLength={1024}
                          value={input.openvikingUrl || ''}
                          onChange={(event) =>
                            update({ openvikingUrl: event.target.value })
                          }
                        />
                      </label>
                      <label>
                        {m.mpaOvResource()}
                        <input
                          maxLength={128}
                          value={input.openvikingResourceId || ''}
                          onChange={(event) =>
                            update({ openvikingResourceId: event.target.value })
                          }
                        />
                      </label>
                      <label>
                        {m.mpaOvKey()}
                        <input
                          type="password"
                          autoComplete="off"
                          maxLength={512}
                          value={input.openvikingApiKey || ''}
                          onChange={(event) =>
                            update({ openvikingApiKey: event.target.value })
                          }
                        />
                      </label>
                    </>
                  )}
                  <MpaTosBucketPicker
                    services={services}
                    epoch={epoch}
                    region={region}
                    value={input.tosBucket || ''}
                    disabled={busy}
                    onChange={(tosBucket) => update({ tosBucket })}
                  />
                  {!tosValid && <p role="alert">{m.mpaTosInvalid()}</p>}
                </>
              )}
            </div>
          </>
        ) : (
          <>
            <h3>{input.name}</h3>
            <p className="mpa-identifier">
              {m.mpaRequestId()}: {input.requestId}
            </p>
            {task && (
              <div role="status" className="mpa-task-progress">
                <LifecycleStatus state={task.state} />
                {!succeeded && <LifecycleStage stage={task.stage} />}
                {active && <progress aria-label={m.mpaOpenTask()} />}
              </div>
            )}
            {task && <MpaCreationProgress task={task} />}
            {task && (
              <MpaCreationDiagnostic
                key={`${task.taskId}:${task.state}:${JSON.stringify(task.latestDiagnostic)}`}
                task={task}
              />
            )}
            {uncertain && <p>{m.mpaCreateUnknown()}</p>}
            {!succeeded && (
              <p>
                {active ? m.mpaBackgroundHint() : m.mpaResourcesMayRemain()}
              </p>
            )}
            <div className="studio-actions">
              <Button
                variant="outline"
                disabled={busy}
                onClick={() => void check()}
              >
                {m.mpaReadStatus()}
              </Button>
              {active && (
                <Button
                  variant="outline"
                  disabled={busy || task.state === 'cancelling'}
                  onClick={() => {
                    setBusy(true);
                    void services
                      .cancelCreation(epoch, region, task.taskId)
                      .then(applyTask)
                      .catch((error) => {
                        if (alive.current) setNotice(studioFailure(error));
                      })
                      .finally(() => {
                        if (alive.current) setBusy(false);
                      });
                  }}
                >
                  {m.mpaCancelCreation()}
                </Button>
              )}
              {task?.state === 'succeeded' && task.result?.runtime_id && (
                <Button
                  variant="primary"
                  disabled={busy}
                  onClick={() => {
                    setBusy(true);
                    void onConnect(task.result!.runtime_id)
                      .then(onClose)
                      .catch((error) => {
                        if (alive.current) setNotice(studioFailure(error));
                      })
                      .finally(() => {
                        if (alive.current) setBusy(false);
                      });
                  }}
                >
                  {m.mpaConnectCreated()}
                </Button>
              )}
            </div>
            {active && <p>{m.mpaCancelHint()}</p>}
            {(!task || ['failed', 'cancelled'].includes(task.state)) && (
              <>
                {needsKey && (
                  <label className="mpa-fields">
                    {m.mpaPendingSecret()}
                    <input
                      type="password"
                      autoComplete="off"
                      value={input.openvikingApiKey || ''}
                      onChange={(event) =>
                        update({ openvikingApiKey: event.target.value })
                      }
                    />
                  </label>
                )}
                <Button
                  variant="outline"
                  disabled={
                    busy || !config?.configured || !resourcesValid || !tosValid
                  }
                  onClick={() => void submit()}
                >
                  {m.mpaRetryCreation()}
                </Button>
              </>
            )}
            {task && creationTerminal(task.state) && (
              <Button
                variant="outline"
                disabled={busy}
                onClick={() => {
                  setInput({
                    requestId: crypto.randomUUID(),
                    name: '',
                    description: '',
                    maxInstance: 20,
                    region,
                    businessWorkspaceMode: 'per-agent',
                    businessDatabaseName: 'aidb',
                    openvikingMode: 'auto',
                  });
                  setSubmitted(false);
                  setTask(undefined);
                  setUncertain(false);
                  setNotice('');
                  setStep(0);
                }}
              >
                {m.mpaNewCreation()}
              </Button>
            )}
          </>
        )}
      </div>
    </ModalDialog>
  );
}
