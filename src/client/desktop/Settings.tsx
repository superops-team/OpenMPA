import { useEffect, useState } from 'react';
import { setThemePreference, type ThemePreference } from '../theme';
import { Button } from '../ui';
import { DesktopBotSettings } from './BotSettings';
import type {
  ConnectionVM,
  DesktopServices,
  DesktopSettingsSection,
  SessionVM,
} from './models';
import { reviewImages } from './image-review';
import { RuntimeConfiguration } from './RuntimeConfiguration';
import { useDesktopSettingsState } from './SettingsState';
import { SettingsAccount } from './SettingsAccount';
import { SettingsRuntime } from './SettingsRuntime';
import { useMessages } from './locale';
export function DesktopSettings({
  services,
  connection,
  onConnection,
  section = 'Account',
  session,
  theme,
  onThemeChange,
}: {
  services: DesktopServices;
  connection?: ConnectionVM;
  onConnection: (connection: ConnectionVM | undefined) => void;
  section?: DesktopSettingsSection;
  onSectionChange?: (section: DesktopSettingsSection) => void;
  session?: SessionVM;
  theme?: ThemePreference;
  onThemeChange?: (theme: ThemePreference) => void;
}) {
  const m = useMessages();
  const state = useDesktopSettingsState({ services, connection, onConnection });
  const [languageMode, setLanguageMode] = useState('system');
  const [assistantSettingsOpen, setAssistantSettingsOpen] = useState(false);
  const [selectingRuntimeId, setSelectingRuntimeId] = useState('');
  const {
    error,
    notice,
    busy,
    act,
    generation,
    setNotice,
    identity,
    setIdentity,
    channelAuth,
    setChannelAuth,
    setUpdates,
    setImages,
    images,
    setReview,
    setError,
    review,
    setUpdate,
    updates,
    update,
  } = state;
  const sectionLabel =
    {
      Account: m.account(),
      Runtime: m.runtime(),
      Assistant: m.assistant(),
      Authentication: m.authentication(),
      Deployment: m.deployment(),
      Theme: m.theme(),
      Language: m.language(),
    }[section] || section;
  useEffect(() => {
    let active = true;
    void services.localization
      ?.status()
      .then((value) => {
        if (active) setLanguageMode(value.mode);
      })
      .catch(() => {});
    return () => {
      active = false;
    };
  }, [services]);
  const currentTheme = theme || 'system';
  useEffect(() => {
    if (section !== 'Assistant') setAssistantSettingsOpen(false);
  }, [section]);
  return (
    <main className="main-content desktop-settings">
      <div className="page-heading">
        <div>
          <span className="eyebrow">{m.settings()}</span>
          <h1>{sectionLabel}</h1>
        </div>
      </div>
      {error && (
        <div className="error-banner" role="alert">
          {error}
        </div>
      )}
      {notice && (
        <div className="notice" role="status">
          {notice}
        </div>
      )}
      {section === 'Account' && (
        <SettingsAccount
          state={state}
          services={services}
          onConnection={onConnection}
        />
      )}
      {section === 'Runtime' && (
        <SettingsRuntime
          state={state}
          services={services}
          connection={connection}
          onConnection={onConnection}
          selectingRuntimeId={selectingRuntimeId}
          setSelectingRuntimeId={setSelectingRuntimeId}
        />
      )}
      {section === 'Authentication' && connection && (
        <section className="task-detail-card">
          <h2>Business connection</h2>
          <p>
            {connection.runtimeId} · {connection.status} · scope{' '}
            {connection.scope}
          </p>
          <button
            className="text-button"
            disabled={busy}
            onClick={() =>
              void act(async () =>
                setNotice(await services.connection.test(connection.epoch)),
              )
            }
          >
            Test connection
          </button>
          <form
            className="desktop-form"
            onSubmit={(event) => {
              event.preventDefault();
              void act(async () => {
                const currentEpoch = connection.epoch;
                const current = generation.current;
                const next = await services.connection.identity(
                  currentEpoch,
                  identity.userId,
                  '',
                  identity.applicationToken,
                  '',
                  '',
                  identity.testDeployment,
                );
                if (generation.current !== current) return;
                onConnection(next);
                setIdentity({ ...identity, applicationToken: '' });
                setNotice(
                  'Business identity saved. Credential inputs cleared.',
                );
              });
            }}
          >
            <label>
              Business user ID
              <input
                required
                value={identity.userId}
                onChange={(event) =>
                  setIdentity({ ...identity, userId: event.target.value })
                }
              />
            </label>
            <label className="desktop-check">
              <input
                type="checkbox"
                checked={identity.testDeployment}
                onChange={(event) =>
                  setIdentity({
                    ...identity,
                    testDeployment: event.target.checked,
                  })
                }
              />{' '}
              This is a verified test deployment with JWT authentication
              disabled
            </label>
            {(['applicationToken'] as const).map((slot) => (
              <label key={slot}>
                Legacy JWT (for authenticated deployments)
                <input
                  type="password"
                  autoComplete="off"
                  value={identity[slot]}
                  onChange={(event) =>
                    setIdentity({ ...identity, [slot]: event.target.value })
                  }
                />
              </label>
            ))}
            <button className="primary" disabled={busy}>
              Save business identity
            </button>
          </form>
        </section>
      )}
      {section === 'Authentication' && connection && (
        <section className="task-detail-card">
          <h2>Channel administrator credentials</h2>
          <p>
            Gateway credentials and channel management credentials occupy
            separate host slots.
          </p>
          <form
            className="desktop-form"
            onSubmit={(event) => {
              event.preventDefault();
              void act(async () => {
                const current = generation.current;
                const next = await services.connection.channelAuth(
                  connection.epoch,
                  channelAuth.mode,
                  channelAuth.secret,
                );
                if (generation.current !== current) return;
                onConnection(next);
                setChannelAuth({ ...channelAuth, secret: '' });
                setNotice(
                  'Channel authentication saved. Credential input cleared.',
                );
              });
            }}
          >
            <label>
              Authentication mode
              <select
                value={channelAuth.mode}
                onChange={(event) =>
                  setChannelAuth({ mode: event.target.value, secret: '' })
                }
              >
                <option value="runtimeKey">Runtime channel key</option>
                <option value="legacyJwt">Legacy JWT</option>
                <option value="none">No administrator credential</option>
              </select>
            </label>
            {channelAuth.mode !== 'none' && (
              <label>
                Channel credential
                <input
                  required
                  type="password"
                  autoComplete="off"
                  value={channelAuth.secret}
                  onChange={(event) =>
                    setChannelAuth({
                      ...channelAuth,
                      secret: event.target.value,
                    })
                  }
                />
              </label>
            )}
            <button className="primary" disabled={busy}>
              Save channel authentication
            </button>
          </form>
        </section>
      )}
      {section === 'Assistant' &&
        (connection && session ? (
          <>
            <section className="task-detail-card desktop-form assistant-settings-launcher">
              <h2>Assistant settings</h2>
              <p>
                Persistent instructions and selected skills apply to new turns.
                They do not rewrite conversation history.
              </p>
              <Button
                variant="primary"
                onClick={() => setAssistantSettingsOpen(true)}
              >
                Open assistant settings
              </Button>
            </section>
            {assistantSettingsOpen && (
              <DesktopBotSettings
                services={services}
                connection={connection}
                session={session}
                onSaved={() =>
                  setNotice('Assistant conversation configuration saved.')
                }
                onClose={() => setAssistantSettingsOpen(false)}
              />
            )}
          </>
        ) : (
          <section className="task-detail-card">
            <div className="setup-card">
              Select a Runtime conversation before editing assistant settings.
            </div>
          </section>
        ))}
      {section === 'Theme' && (
        <section className="task-detail-card desktop-form">
          <h2>Display theme</h2>
          <p>
            The theme applies to the desktop shell and conversation content.
          </p>
          <fieldset className="desktop-segmented">
            <legend>Theme</legend>
            {(['system', 'light', 'dark'] as const).map((option) => (
              <label key={option}>
                <input
                  type="radio"
                  name="desktop-theme"
                  checked={currentTheme === option}
                  onChange={() => {
                    setThemePreference(option);
                    onThemeChange?.(option);
                    setNotice('Display theme updated.');
                  }}
                />
                <span>
                  {option === 'system'
                    ? 'Follow system'
                    : option === 'light'
                      ? 'Light'
                      : 'Dark'}
                </span>
              </label>
            ))}
          </fieldset>
        </section>
      )}
      {section === 'Language' && (
        <section className="task-detail-card desktop-form">
          <h2>Display language</h2>
          <p>
            OpenMPA uses one language across the desktop UI, system menus and
            content pages.
          </p>
          <fieldset className="desktop-segmented">
            <legend>Language</legend>
            {[
              ['system', 'Follow system'],
              ['zh-Hans', '中文'],
              ['en', 'English'],
            ].map(([mode, label]) => (
              <label key={mode}>
                <input
                  type="radio"
                  name="desktop-language"
                  checked={languageMode === mode}
                  disabled={!services.localization}
                  onChange={() =>
                    void act(async () => {
                      const next = await services.localization!.set(mode);
                      setLanguageMode(next.mode);
                      setNotice('Display language updated.');
                    })
                  }
                />
                <span>{label}</span>
              </label>
            ))}
          </fieldset>
        </section>
      )}
      {section === 'Deployment' && connection && (
        <RuntimeConfiguration
          key={connection.epoch}
          connection={connection}
          busy={busy}
          run={act}
        />
      )}
      {section === 'Deployment' && connection && (
        <section className="task-detail-card desktop-form">
          <h2>Runtime and Worker Tool images</h2>
          <button
            className="text-button"
            disabled={busy || !services.deployment}
            onClick={() =>
              void act(async () => {
                const current = generation.current;
                const saved = await services.deployment!.updates(
                  connection.runtimeId,
                );
                if (generation.current === current) setUpdates(saved);
              })
            }
          >
            Refresh saved receipts
          </button>
          <p>
            Read the server-verified Tool binding, review exact resource IDs and
            versions, then apply each update. Submission and deployment status
            appear separately.
          </p>
          <button
            className="text-button"
            disabled={busy || !services.deployment}
            onClick={() =>
              void act(async () => {
                const current = generation.current;
                const value = await services.deployment!.images(
                  connection.runtimeId,
                );
                if (generation.current === current) setImages(value);
              })
            }
          >
            Read current images
          </button>
          {!services.deployment && (
            <p>Image update capability is unavailable in this build.</p>
          )}
          {images && (
            <>
              <p>
                Runtime {images.runtimeId} · version {images.runtimeVersion}
                <br />
                Tool {images.toolId} · updated at {images.toolVersion}
              </p>
              <label>
                Runtime image
                <input
                  value={images.runtimeImage}
                  onChange={(event) => {
                    setImages({ ...images, runtimeImage: event.target.value });
                    setReview(undefined);
                  }}
                />
              </label>
              <label>
                Worker Tool image
                <input
                  value={images.toolImage}
                  onChange={(event) => {
                    setImages({ ...images, toolImage: event.target.value });
                    setReview(undefined);
                  }}
                />
              </label>
              <button
                className="primary"
                disabled={busy}
                onClick={() => {
                  try {
                    setReview(reviewImages(images));
                  } catch (error) {
                    setError(
                      error instanceof Error ? error.message : 'Review failed.',
                    );
                  }
                }}
              >
                Review exact targets
              </button>
            </>
          )}
          {review && services.deployment && (
            <div className="document-save-notice">
              <h3>Review updates</h3>
              <p>
                Runtime ID: {review.runtimeId}
                <br />
                Expected version: {review.expectedRuntimeVersion}
                <br />
                Image: {review.runtimeImage}
              </p>
              <button
                disabled={busy}
                onClick={() =>
                  void act(async () => {
                    const operationId =
                      await services.deployment!.runtime(review);
                    setUpdate({
                      operationId,
                      status: 'Submitted; effectiveness not yet verified',
                    });
                  })
                }
              >
                Apply Runtime image
              </button>
              <p>
                Tool ID: {review.toolId}
                <br />
                Worker configuration updated at: {review.expectedToolVersion}
                <br />
                Image: {review.toolImage}
              </p>
              <button
                disabled={busy}
                onClick={() =>
                  void act(async () => {
                    const operationId =
                      await services.deployment!.worker(review);
                    setUpdate({
                      operationId,
                      status: 'Submitted; effectiveness not yet verified',
                    });
                  })
                }
              >
                Apply Worker image
              </button>
            </div>
          )}
          {updates.length > 0 && services.deployment && (
            <div>
              <h3>Saved image update receipts</h3>
              {updates.map((operation) => (
                <div key={operation.operationId}>
                  <p>
                    {operation.operationId} · {operation.status}
                  </p>
                  <button
                    className="text-button"
                    disabled={busy}
                    onClick={() =>
                      void act(async () => {
                        const status = await services.deployment!.read(
                          connection!.runtimeId,
                          operation.operationId,
                        );
                        setUpdate({
                          operationId: operation.operationId,
                          status,
                        });
                      })
                    }
                  >
                    Read current status of this operation
                  </button>
                </div>
              ))}
            </div>
          )}
          {update && services.deployment && (
            <div role="status">
              <p>
                Operation {update.operationId} · {update.status}
              </p>
              <button
                disabled={busy}
                onClick={() =>
                  void act(async () =>
                    setUpdate({
                      ...update,
                      status: await services.deployment!.read(
                        connection.runtimeId,
                        update.operationId,
                      ),
                    }),
                  )
                }
              >
                Read update status
              </button>
            </div>
          )}
        </section>
      )}
    </main>
  );
}
