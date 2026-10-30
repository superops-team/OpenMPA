import {
  errorMessage,
  localized,
  useMessages,
  useMessageState,
} from './locale';
import { useEffect, useRef, useState } from 'react';
import { MessageCircle, RefreshCw } from 'lucide-react';
import { ModalDialog } from '../ui/ModalDialog';
import './channel-page.css';
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
  const m = useMessages();
  const [channels, setChannels] = useState<ChannelVM[]>([]);
  const [revision, setRevision] = useState(0);
  const [diagnostics, setDiagnostics] = useState<{
    accountId: string;
    value: string;
  }>();
  const [edit, setEdit] = useState<{ channel: ChannelVM; profile: string }>();
  const [configDraft, setConfigDraft] = useState<{
    channel: ChannelVM;
    secret: string;
  }>();
  const [error, setError] = useMessageState();
  const [loading, setLoading] = useState(false);
  const [loaded, setLoaded] = useState(false);
  const [busy, setBusy] = useState(false);
  const reading = useRef(0);
  const writing = useRef(false);
  const epoch = useRef(connection?.epoch);
  epoch.current = connection?.epoch;
  const load = async () => {
    if (!services.channels || !connection) return;
    const current = connection.epoch;
    const id = ++reading.current;
    setLoading(true);
    try {
      const value = await services.channels.list(current);
      if (epoch.current === current && id === reading.current) {
        setChannels(value);
        setLoaded(true);
        setRevision((value) => value + 1);
      }
    } catch (error) {
      if (epoch.current === current && id === reading.current) setLoaded(false);
      throw error;
    } finally {
      if (epoch.current === current && id === reading.current)
        setLoading(false);
    }
  };
  useEffect(() => {
    setChannels([]);
    setDiagnostics(undefined);
    setLoaded(false);
    setEdit(undefined);
    setConfigDraft(undefined);
    setError('');
    const current = connection?.epoch;
    void load().catch(
      (error: unknown) =>
        epoch.current === current &&
        setError(
          errorMessage(
            error,
            localized((m) => m.desktopCouldNotReadChannels()),
          ),
        ),
    );
    return () => {
      reading.current++;
    };
  }, [services, connection?.epoch]);
  const run = async (action: () => Promise<void>) => {
    if (writing.current) return;
    const current = epoch.current;
    writing.current = true;
    setBusy(true);
    setError('');
    try {
      await action();
      if (epoch.current === current) await load();
    } catch (error) {
      if (epoch.current === current)
        setError(
          errorMessage(
            error,
            localized((m) => m.desktopChannelActionFailed()),
          ),
        );
    } finally {
      writing.current = false;
      if (epoch.current === current) setBusy(false);
    }
  };
  return (
    <main className="main-content channel-page">
      <header className="page-heading">
        <div>
          <h1>{m.channelPageTitle()}</h1>
          <p>{m.channelPageDescription()}</p>
        </div>
        <button
          type="button"
          className="channel-button"
          disabled={busy || loading}
          onClick={() => void run(async () => {})}
        >
          <RefreshCw size={16} aria-hidden="true" />
          {m.desktopRefresh()}
        </button>
      </header>
      {error && (
        <p role="alert" className="channel-inline-error">
          {error}
        </p>
      )}
      {!services.channels || !connection ? (
        <div className="channel-empty">
          <MessageCircle size={28} aria-hidden="true" />
          <p>
            {m.desktopConnectARuntimeWithChannelManagementCredentialsToView()}
          </p>
        </div>
      ) : (
        <ChannelManagement
          key={connection.epoch}
          connection={connection}
          accounts={channels}
          sessions={sessions}
          onChanged={load}
          loading={loading}
          loaded={loaded}
          externalBusy={busy}
          revision={revision}
          onToggleAccount={(account) =>
            void run(() =>
              services.channels!.enable(
                connection.epoch,
                account,
                !account.enabled,
              ),
            )
          }
          onRemoveAccount={(account) => {
            if (window.confirm(m.channelRemoveConfirm(account.id)))
              void run(() =>
                services.channels!.remove(connection.epoch, account),
              );
          }}
          renderAccountTools={(account) => (
            <section className="channel-account-tools">
              <h3>{m.desktopBotProfile()}</h3>
              <code>{account.id}</code>
              {account.ownerId && (
                <p className="channel-muted">
                  {m.desktopOwner()} {account.ownerId}
                </p>
              )}
              <div className="channel-actions">
                {account.type === 'feishu' && (
                  <button
                    type="button"
                    className="channel-button"
                    disabled={busy || !loaded || account.addressable === false}
                    onClick={() =>
                      void run(async () => {
                        const value = await services.channels!.readProfile(
                          connection.epoch,
                          account,
                        );
                        if (epoch.current === connection.epoch)
                          setEdit({
                            channel: value,
                            profile: value.profile ?? '',
                          });
                      })
                    }
                  >
                    {m.desktopBotProfile()}
                  </button>
                )}
                <button
                  type="button"
                  className="channel-button"
                  disabled={busy || !loaded || account.addressable === false}
                  onClick={() =>
                    setConfigDraft({ channel: account, secret: '' })
                  }
                >
                  {m.desktopUpdateConfiguration()}
                </button>
                <button
                  type="button"
                  className="channel-button"
                  disabled={busy || !loaded || account.addressable === false}
                  onClick={() =>
                    void run(async () => {
                      const result = await services.channels!.diagnostics(
                        connection.epoch,
                        account,
                      );
                      if (epoch.current === connection.epoch)
                        setDiagnostics({
                          accountId: account.id,
                          value: result,
                        });
                    })
                  }
                >
                  {m.desktopDiagnostics()}
                </button>
              </div>
              {diagnostics?.accountId === account.id && (
                <pre className="channel-diagnostics-output">
                  {diagnostics.value}
                </pre>
              )}
            </section>
          )}
          advancedContent={
            <details className="channel-template-tools">
              <summary>{m.desktopRuntimeTemplates()}</summary>
              <p className="channel-muted">
                {m.desktopTheSelectedRuntimeDoesNotExposeATemplateCatalog()}
              </p>
            </details>
          }
        />
      )}
      {edit && connection && services.channels && (
        <ModalDialog
          title={m.desktopBotProfile()}
          description={edit.channel.name || edit.channel.id}
          closeLabel={m.closeDialog(m.desktopBotProfile())}
          onClose={() => {
            if (!busy) setEdit(undefined);
          }}
        >
          <form
            className="desktop-form channel-dialog-form"
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
            <p className="channel-muted">
              {m.desktopReviewTheCurrentProfileVersion()}{' '}
              {edit.channel.version ?? m.valueUnavailable()}{' '}
              {m.desktopBeforeSaving()}
            </p>
            <textarea
              aria-label={m.desktopBotProfileJSON()}
              rows={12}
              disabled={busy}
              value={edit.profile}
              onChange={(event) =>
                setEdit({ ...edit, profile: event.target.value })
              }
            />
            <div className="channel-actions">
              <button
                type="button"
                className="channel-button"
                disabled={busy}
                onClick={() => setEdit(undefined)}
              >
                {m.cancel()}
              </button>
              <button className="channel-button is-primary" disabled={busy}>
                {m.desktopSaveProfile()}
              </button>
            </div>
          </form>
        </ModalDialog>
      )}
      {configDraft && connection && services.channels && (
        <ModalDialog
          title={m.desktopUpdateConfiguration()}
          description={configDraft.channel.name || configDraft.channel.id}
          closeLabel={m.closeDialog(m.desktopUpdateConfiguration())}
          onClose={() => {
            if (!busy) setConfigDraft(undefined);
          }}
        >
          <form
            className="desktop-form channel-dialog-form"
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
            <label className="channel-field">
              <span>{m.desktopNewChannelCredential()}</span>
              <input
                type="password"
                required
                autoComplete="off"
                disabled={busy}
                value={configDraft.secret}
                onChange={(event) =>
                  setConfigDraft({ ...configDraft, secret: event.target.value })
                }
              />
            </label>
            <p className="channel-muted">
              {m.desktopThisUpdateHasNoServerCASReadConfigurationAgain()}
            </p>
            <div className="channel-actions">
              <button
                type="button"
                className="channel-button"
                disabled={busy}
                onClick={() => setConfigDraft(undefined)}
              >
                {m.cancel()}
              </button>
              <button className="channel-button is-primary" disabled={busy}>
                {m.desktopSaveCredential()}
              </button>
            </div>
          </form>
        </ModalDialog>
      )}
    </main>
  );
}
