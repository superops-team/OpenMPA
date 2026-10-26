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
      <h2>Runtime configuration</h2>
      <p>
        Review resources for {connection.runtimeId}. Applying configuration uses
        AgentKit and may restart Runtime instances.
      </p>
      <button disabled={busy} onClick={() => void run(read)}>
        Read Runtime configuration
      </button>
      {snapshot && draft && (
        <form
          className="desktop-form"
          onSubmit={(event) => {
            event.preventDefault();
            if (
              busy ||
              !window.confirm(
                `Apply configuration to ${connection.runtimeId}, based on version ${snapshot.version}?`,
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
            Version {snapshot.version} · {snapshot.status}
          </p>
          <label>
            Description
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
              ['cpuMilli', 'CPU (millicores)', 100, 64000],
              ['memoryMb', 'Memory (MiB)', 128, 262144],
              ['minInstance', 'Minimum instances', 0, 1000],
              ['maxInstance', 'Maximum instances', 1, 1000],
              ['maxConcurrency', 'Maximum concurrency', 1, 100000],
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
            Review and apply configuration
          </button>
          <p>
            Image, Tool binding and secret environment values are handled
            separately. Reload if another administrator changes the Runtime.
          </p>
        </form>
      )}
      <label>
        Configuration operation ID
        <input
          value={operationId}
          onChange={(event) => setOperationId(event.target.value)}
          placeholder="Original saved operation ID"
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
        Verify original configuration update
      </button>
      {operation && (
        <p role="status">
          {operation.operationId} · {operation.state}
          {operation.state === 'ready'
            ? ' · Target configuration and a new Ready version are visible.'
            : ' · Read back this operation to verify the outcome.'}
        </p>
      )}
    </section>
  );
}
