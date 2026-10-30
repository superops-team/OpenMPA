import { useMessages } from './locale';
import { useEffect, useRef, useState } from 'react';
import { RuntimeConfig } from './generated/mygo';
import { unwrap } from './result';
import type { ConnectionVM } from './models';

type Snapshot = NonNullable<
  Awaited<ReturnType<typeof RuntimeConfig.read>>['value']
>;
type Operation = NonNullable<
  Awaited<ReturnType<typeof RuntimeConfig.update>>['value']
>;
export function RuntimeConfiguration({
  connection,
  busy,
  run,
}: {
  connection: ConnectionVM;
  busy: boolean;
  run: (action: () => Promise<void>) => Promise<void>;
}) {
  const m = useMessages();
  const [snapshot, setSnapshot] = useState<Snapshot>();
  const [draft, setDraft] = useState<Snapshot['values']>();
  const [operationId, setOperationId] = useState('');
  const [operation, setOperation] = useState<Operation>();
  const epoch = useRef(connection.epoch);
  epoch.current = connection.epoch;
  const key = `openmpa:runtime-config:${connection.scope}:${connection.runtimeId}`;
  useEffect(() => {
    setSnapshot(undefined);
    setDraft(undefined);
    setOperation(undefined);
    try {
      setOperationId(localStorage.getItem(key) || '');
    } catch {
      setOperationId('');
    }
  }, [connection.epoch, key]);
  const read = async () => {
    const current = connection.epoch;
    const value = unwrap(
      await RuntimeConfig.read({ connectionEpoch: current }),
    );
    if (epoch.current === current) {
      setSnapshot(value);
      setDraft(value.values);
    }
  };
  return (
    <section className="task-detail-card desktop-form">
      <h2>{m.settingsRuntimeConfiguration()}</h2>
      <p>{m.settingsRuntimeConfigurationHint(connection.runtimeId)}</p>
      <button disabled={busy} onClick={() => void run(read)}>
        {m.settingsReadRuntimeConfiguration()}
      </button>
      {snapshot && draft && (
        <form
          className="desktop-form"
          onSubmit={(event) => {
            event.preventDefault();
            if (
              busy ||
              !window.confirm(
                m.settingsApplyConfigurationConfirm(
                  String(snapshot.version),
                  connection.runtimeId,
                ),
              )
            )
              return;
            const current = connection.epoch;
            const original = crypto.randomUUID();
            try {
              localStorage.setItem(key, original);
            } catch {
              /* Native host owns the durable intent. */
            }
            setOperationId(original);
            void run(async () => {
              const value = unwrap(
                await RuntimeConfig.update({
                  connectionEpoch: current,
                  operationId: original,
                  fingerprint: snapshot.fingerprint,
                  values: draft,
                  confirm: true,
                }),
              );
              if (epoch.current === current) setOperation(value);
            });
          }}
        >
          <p>
            {m.settingsConfigurationVersion(
              String(snapshot.version),
              snapshot.status,
            )}
          </p>
          <label>
            {m.description()}
            <textarea
              value={draft.description}
              maxLength={4096}
              onChange={(event) =>
                setDraft({ ...draft, description: event.target.value })
              }
            />
          </label>
          {(
            [
              ['cpuMilli', m.cpuMilli(), 100, 64000],
              ['memoryMb', m.settingsMemoryMib(), 128, 262144],
              ['minInstance', m.minimumInstances(), 0, 1000],
              ['maxInstance', m.maximumInstances(), 1, 1000],
              ['maxConcurrency', m.maximumConcurrency(), 1, 100000],
            ] as const
          ).map(([name, label, min, max]) => (
            <label key={name}>
              {label}
              <input
                required
                type="number"
                min={min}
                max={max}
                step={1}
                value={draft[name]}
                onChange={(event) =>
                  setDraft({ ...draft, [name]: Number(event.target.value) })
                }
              />
            </label>
          ))}
          <button className="primary" disabled={busy}>
            {m.settingsReviewApplyConfiguration()}
          </button>
          <p>{m.settingsConfigurationSeparateHint()}</p>
        </form>
      )}
      <label>
        {m.settingsConfigurationOperationId()}
        <input
          value={operationId}
          onChange={(event) => setOperationId(event.target.value)}
          placeholder={m.settingsOriginalOperationIdPlaceholder()}
        />
      </label>
      <button
        disabled={busy || !operationId}
        onClick={() =>
          void run(async () => {
            const current = connection.epoch;
            const value = unwrap(
              await RuntimeConfig.readUpdate({
                connectionEpoch: current,
                operationId,
              }),
            );
            if (epoch.current === current) setOperation(value);
          })
        }
      >
        {m.settingsVerifyConfigurationUpdate()}
      </button>
      {operation && (
        <p role="status">
          {operation.operationId} · {operation.state}
          {operation.state === 'ready'
            ? ` · ${m.settingsConfigurationReady()}`
            : ` · ${m.settingsConfigurationVerifyHint()}`}
        </p>
      )}
    </section>
  );
}
