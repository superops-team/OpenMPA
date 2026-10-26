import { useEffect, useRef, useState } from 'react';
import { Session } from './generated/mygo';
import { unwrap } from './result';
import type { ConnectionVM, DesktopServices, SessionVM } from './models';

type Configuration = NonNullable<
  Awaited<ReturnType<typeof Session.executionConfig>>['value']
>;
export function DesktopBotSettings({
  connection,
  session,
  onSaved,
  onClose,
}: {
  services: DesktopServices;
  connection: ConnectionVM;
  session: SessionVM;
  onSaved: (session: SessionVM) => void;
  onClose: () => void;
}) {
  const [config, setConfig] = useState<Configuration>();
  const [servers, setServers] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const generation = useRef(0);
  useEffect(() => {
    const current = ++generation.current;
    setConfig(undefined);
    setError('');
    void Session.executionConfig({
      connectionEpoch: connection.epoch,
      sessionId: session.id,
    })
      .then(unwrap)
      .then((value) => {
        if (generation.current === current) {
          setConfig(value);
          setServers(value.mcpServers.join('\n'));
        }
      })
      .catch((failure: unknown) => {
        if (generation.current === current)
          setError(
            failure &&
              typeof failure === 'object' &&
              'httpStatus' in failure &&
              failure.httpStatus === 404
              ? 'This conversation uses Runtime defaults and has no editable execution configuration.'
              : failure instanceof Error
                ? failure.message
                : 'Could not read conversation configuration.',
          );
      });
    return () => {
      generation.current++;
    };
  }, [connection.epoch, session.id]);
  return (
    <section className="task-detail-card desktop-form">
      <h2>Conversation configuration</h2>
      <p>
        Changes apply to future turns in this Session. Accepted turns keep their
        original configuration.
      </p>
      {error && <p role="alert">{error}</p>}
      {config && (
        <form
          className="desktop-form"
          onSubmit={(event) => {
            event.preventDefault();
            if (busy) return;
            const current = generation.current;
            setBusy(true);
            setError('');
            void Session.updateExecutionConfig({
              connectionEpoch: connection.epoch,
              sessionId: session.id,
              etag: config.etag || '',
              modelMode: config.modelMode,
              model: config.model || '',
              mcpMode: config.mcpMode,
              mcpServers: servers
                .split('\n')
                .map((id) => id.trim())
                .filter(Boolean),
            })
              .then(unwrap)
              .then((value) => {
                if (generation.current === current)
                  onSaved({
                    ...session,
                    configVersion: value.executionConfigVersion,
                  });
              })
              .catch((failure: unknown) => {
                if (generation.current === current)
                  setError(
                    failure instanceof Error
                      ? failure.message
                      : 'Configuration changed. Keep this draft and reload before applying.',
                  );
              })
              .finally(() => {
                if (generation.current === current) setBusy(false);
              });
          }}
        >
          <p>Execution version {config.executionConfigVersion}</p>
          <label>
            Model selection
            <select
              value={config.modelMode}
              onChange={(event) =>
                setConfig({ ...config, modelMode: event.target.value })
              }
            >
              <option value="inherit">Use Runtime default</option>
              <option value="replace">Choose model ID</option>
              <option value="clear">Clear override</option>
            </select>
          </label>
          {config.modelMode === 'replace' && (
            <label>
              Model ID
              <input
                required
                value={config.model || ''}
                onChange={(event) =>
                  setConfig({ ...config, model: event.target.value })
                }
              />
            </label>
          )}
          <label>
            MCP selection
            <select
              value={config.mcpMode}
              onChange={(event) =>
                setConfig({ ...config, mcpMode: event.target.value })
              }
            >
              <option value="inherit">Use Runtime default</option>
              <option value="replace">Choose MCP server IDs</option>
              <option value="clear">No MCP servers</option>
            </select>
          </label>
          {config.mcpMode === 'replace' && (
            <label>
              MCP server IDs, one per line
              <textarea
                value={servers}
                onChange={(event) => setServers(event.target.value)}
              />
            </label>
          )}
          {config.invalidRefsCount > 0 && (
            <p>
              Some configured resources are unavailable. Choose resources
              exposed by this Runtime.
            </p>
          )}
          <button className="primary" disabled={busy}>
            Apply configuration
          </button>
        </form>
      )}
      <button className="text-button" onClick={onClose}>
        Close configuration
      </button>
    </section>
  );
}
