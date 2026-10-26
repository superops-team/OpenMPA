import { useEffect, useRef, useState } from 'react';
import { X, Download } from 'lucide-react';
import type { ConnectionVM, DesktopServices, SessionVM } from './models';
export function DesktopArtifacts({
  services,
  connection,
  session,
  onClose,
}: {
  services: DesktopServices;
  connection?: ConnectionVM;
  session?: SessionVM;
  onClose: () => void;
}) {
  const [path, setPath] = useState('');
  const [bytes, setBytes] = useState<number>();
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState('');
  const [details, setDetails] = useState<
    Array<{ name: string; value: string }>
  >([]);
  const generation = useRef(0);
  const download = useRef<AbortController | undefined>(undefined);
  useEffect(() => {
    const current = ++generation.current;
    if (connection && services.capabilities)
      void services.capabilities
        .read(connection.epoch, session?.id)
        .then((value) => {
          if (generation.current === current) setDetails(value);
        })
        .catch((error: unknown) => {
          if (generation.current === current)
            setNotice(
              error instanceof Error
                ? error.message
                : 'Could not read capability details.',
            );
        });
    return () => {
      generation.current++;
      download.current?.abort();
    };
  }, [services, connection?.epoch, session?.id]);
  return (
    <aside className="result-pane">
      <div className="pane-heading">
        <h2>Capabilities & artifacts</h2>
        <button
          className="icon-button"
          aria-label="Close results"
          onClick={onClose}
        >
          <X size={16} />
        </button>
      </div>
      <p>Runtime {connection?.runtimeId ?? 'not selected'}</p>
      <dl>
        {details.map((detail) => (
          <div key={detail.name}>
            <dt>{detail.name}</dt>
            <dd>{detail.value}</dd>
          </div>
        ))}
      </dl>
      <p>
        Skills and MCP dependencies require a verified Bot profile. Availability
        does not grant access to every tool.
      </p>
      <form
        className="desktop-form"
        onSubmit={(event) => {
          event.preventDefault();
          if (!services.artifacts || !connection || !session) return;
          const current = generation.current;
          download.current?.abort();
          const controller = new AbortController();
          download.current = controller;
          setBusy(true);
          setNotice('');
          void services.artifacts
            .download(
              connection.epoch,
              session.id,
              path,
              (value) => {
                if (generation.current === current) setBytes(value);
              },
              controller.signal,
            )
            .then((receipt) => {
              if (generation.current === current) setNotice(receipt);
            })
            .catch((error: unknown) => {
              if (generation.current === current)
                setNotice(
                  error instanceof Error
                    ? error.message
                    : 'Could not download artifact.',
                );
            })
            .finally(() => {
              if (generation.current === current) setBusy(false);
            });
        }}
      >
        <h3>Download a sandbox artifact</h3>
        <label>
          Artifact path
          <input
            required
            placeholder="Path supplied by the Runtime"
            value={path}
            onChange={(event) => setPath(event.target.value)}
          />
        </label>
        <p>
          Choose a native destination after the host validates this path and
          session access.
        </p>
        <button
          className="primary"
          disabled={!services.artifacts || !connection || !session || busy}
        >
          <Download size={14} />
          Download
        </button>
        {bytes !== undefined && <p role="status">{bytes} bytes received</p>}
      </form>
      {notice && <p role="status">{notice}</p>}
    </aside>
  );
}
