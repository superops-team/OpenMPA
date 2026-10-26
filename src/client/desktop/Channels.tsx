import { useEffect, useRef, useState } from 'react';
import { MessageCircle, RefreshCw } from 'lucide-react';
import { ChannelManagement } from './ChannelManagement';
import type { ChannelVM, ConnectionVM, DesktopServices } from './models';
export function DesktopChannels({
  services,
  connection,
  sessions,
}: {
  services: DesktopServices;
  connection?: ConnectionVM;
  sessions: import('./models').SessionVM[];
}) {
  const [channels, setChannels] = useState<ChannelVM[]>([]);
  const [templates, setTemplates] = useState<string[]>([]);
  const [edit, setEdit] = useState<{ channel: ChannelVM; profile: string }>();
  const [configDraft, setConfigDraft] = useState<{
    channel: ChannelVM;
    secret: string;
  }>();
  const [error, setError] = useState('');
  const epoch = useRef(connection?.epoch);
  epoch.current = connection?.epoch;
  const load = async () => {
    if (!services.channels || !connection) return;
    const current = connection.epoch;
    const value = await services.channels.list(current);
    if (epoch.current === current) setChannels(value);
  };
  useEffect(() => {
    void load().catch((error: unknown) =>
      setError(
        error instanceof Error ? error.message : 'Could not read channels.',
      ),
    );
  }, [services, connection?.epoch]);
  const run = async (action: () => Promise<void>) => {
    try {
      await action();
      await load();
    } catch (error) {
      setError(
        error instanceof Error ? error.message : 'Channel action failed.',
      );
    }
  };
  return (
    <main className="main-content">
      <div className="page-heading">
        <div>
          <span className="eyebrow">CONNECTIONS</span>
          <h1>Channels and templates.</h1>
          <p>
            Channel management uses the Runtime administrator identity;
            conversation access remains independently authorized.
          </p>
        </div>
        <button className="text-button" onClick={() => void run(load)}>
          <RefreshCw size={15} /> Refresh
        </button>
      </div>
      {error && (
        <div role="alert" className="error-banner">
          {error}
        </div>
      )}
      {!services.channels || !connection ? (
        <div className="setup-card">
          <MessageCircle size={24} />
          <p>
            Connect a Runtime with channel-management credentials to view
            accounts and templates.
          </p>
        </div>
      ) : (
        <>
          <div className="memory-grid">
            {channels.map((channel) => (
              <article className="memory-card" key={channel.id}>
                <h2>{channel.name || channel.id}</h2>
                <p>
                  {channel.type} · {channel.status} ·{' '}
                  {channel.enabled ? 'Enabled' : 'Disabled'}
                </p>
                <small>
                  Account {channel.id} · Owner{' '}
                  {channel.ownerId ?? 'Unavailable'}
                </small>
                <div>
                  <button
                    className="text-button"
                    disabled={channel.type.toLowerCase() !== 'feishu'}
                    onClick={() =>
                      void run(() =>
                        services.channels!.enable(
                          connection.epoch,
                          channel,
                          !channel.enabled,
                        ),
                      )
                    }
                  >
                    {channel.enabled ? 'Disable account' : 'Enable account'}
                  </button>
                  <button
                    className="text-button"
                    onClick={() =>
                      void run(async () => {
                        const value = await services.channels!.readProfile(
                          connection.epoch,
                          channel,
                        );
                        if (epoch.current === connection.epoch)
                          setEdit({
                            channel: value,
                            profile: value.profile ?? '',
                          });
                      })
                    }
                  >
                    Bot profile
                  </button>
                  <button
                    className="text-button"
                    onClick={() => setConfigDraft({ channel, secret: '' })}
                  >
                    Update configuration
                  </button>
                  <button
                    className="text-button"
                    onClick={() =>
                      void run(async () =>
                        setError(
                          await services.channels!.diagnostics(
                            connection.epoch,
                            channel,
                          ),
                        ),
                      )
                    }
                  >
                    Diagnostics
                  </button>
                  <button
                    className="text-button"
                    onClick={() => {
                      if (
                        window.confirm(
                          `Remove channel account ${channel.id}? Conversation history is retained.`,
                        )
                      )
                        void run(() =>
                          services.channels!.remove(connection.epoch, channel),
                        );
                    }}
                  >
                    Remove
                  </button>
                </div>
              </article>
            ))}
          </div>
          <section className="task-detail-card">
            <h2>Runtime templates</h2>
            <button
              className="text-button"
              onClick={() =>
                void run(async () => {
                  const current = connection.epoch;
                  const list = await services.channels!.templates(current);
                  if (epoch.current === current) setTemplates(list);
                })
              }
            >
              Read templates
            </button>
            {templates.map((template, index) => (
              <pre key={index}>{template}</pre>
            ))}
          </section>
        </>
      )}
      {connection && services.channels && (
        <ChannelManagement
          key={connection.epoch}
          connection={connection}
          accounts={channels}
          sessions={sessions}
          onChanged={load}
        />
      )}
      {edit && connection && services.channels && (
        <form
          className="task-detail-card desktop-form"
          onSubmit={(event) => {
            event.preventDefault();
            void run(async () => {
              await services.channels!.saveProfile(
                connection.epoch,
                edit.channel,
                edit.profile,
              );
              setEdit(undefined);
            });
          }}
        >
          <h2>Profile · {edit.channel.id}</h2>
          <p>
            Review the current profile version{' '}
            {edit.channel.version ?? 'unavailable'} before saving.
          </p>
          <textarea
            aria-label="Bot profile JSON"
            value={edit.profile}
            onChange={(event) =>
              setEdit({ ...edit, profile: event.target.value })
            }
          />
          <button className="primary">Save profile</button>
          <button
            type="button"
            className="text-button"
            onClick={() => setEdit(undefined)}
          >
            Cancel
          </button>
        </form>
      )}
      {configDraft && connection && services.channels && (
        <form
          className="task-detail-card desktop-form"
          onSubmit={(event) => {
            event.preventDefault();
            void run(async () => {
              await services.channels!.saveConfig(
                connection.epoch,
                configDraft.channel,
                configDraft.secret,
              );
              setConfigDraft(undefined);
            });
          }}
        >
          <h2>Configuration · {configDraft.channel.id}</h2>
          <label>
            New channel credential
            <input
              type="password"
              required
              autoComplete="off"
              value={configDraft.secret}
              onChange={(event) =>
                setConfigDraft({ ...configDraft, secret: event.target.value })
              }
            />
          </label>
          <p>
            This update has no server CAS. Read configuration again after
            saving.
          </p>
          <button className="primary">Save credential</button>
          <button
            type="button"
            className="text-button"
            onClick={() => setConfigDraft(undefined)}
          >
            Cancel
          </button>
        </form>
      )}
    </main>
  );
}
