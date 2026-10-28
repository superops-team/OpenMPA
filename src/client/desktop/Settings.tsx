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
import { ImageReviewError, reviewImages } from './image-review';
import { RuntimeConfiguration } from './RuntimeConfiguration';
import { useDesktopSettingsState } from './SettingsState';
import { SettingsAccount } from './SettingsAccount';
import { SettingsRuntime } from './SettingsRuntime';
import { localized, renderMessage, useMessages } from './locale';
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
          <h2>{m.settingsBusinessConnection()}</h2>
          <p>
            {m.settingsConnectionSummary(
              connection.runtimeId,
              connection.status,
              connection.scope,
            )}
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
            {m.testConnection()}
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
                setNotice(localized((m) => m.settingsBusinessIdentitySaved()));
              });
            }}
          >
            <label>
              {m.businessUserId()}
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
              {m.verifiedTest()}
            </label>
            {(['applicationToken'] as const).map((slot) => (
              <label key={slot}>
                {m.settingsLegacyJwtAuthenticated()}
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
              {m.saveBusinessIdentity()}
            </button>
          </form>
        </section>
      )}
      {section === 'Authentication' && connection && (
        <section className="task-detail-card">
          <h2>{m.settingsChannelAdministratorCredentials()}</h2>
          <p>{m.settingsChannelCredentialsHint()}</p>
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
                  localized((m) => m.settingsChannelAuthenticationSaved()),
                );
              });
            }}
          >
            <label>
              {m.channelAuthenticationMode()}
              <select
                value={channelAuth.mode}
                onChange={(event) =>
                  setChannelAuth({ mode: event.target.value, secret: '' })
                }
              >
                <option value="runtimeKey">
                  {m.settingsRuntimeChannelKey()}
                </option>
                <option value="legacyJwt">{m.legacyJwt()}</option>
                <option value="none">{m.channelUseRuntimeIdentity()}</option>
              </select>
            </label>
            {channelAuth.mode !== 'none' && (
              <label>
                {m.channelCredential()}
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
              {m.saveChannelAuthentication()}
            </button>
          </form>
        </section>
      )}
      {section === 'Assistant' &&
        (connection && session ? (
          <>
            <section className="task-detail-card desktop-form assistant-settings-launcher">
              <h2>{m.settingsAssistantTitle()}</h2>
              <p>{m.settingsAssistantPersistenceHint()}</p>
              <Button
                variant="primary"
                onClick={() => setAssistantSettingsOpen(true)}
              >
                {m.settingsOpenAssistantSettings()}
              </Button>
            </section>
            {assistantSettingsOpen && (
              <DesktopBotSettings
                services={services}
                connection={connection}
                session={session}
                onSaved={() =>
                  setNotice(localized((m) => m.settingsAssistantSaved()))
                }
                onClose={() => setAssistantSettingsOpen(false)}
              />
            )}
          </>
        ) : (
          <section className="task-detail-card">
            <div className="setup-card">
              {m.settingsAssistantSelectConversation()}
            </div>
          </section>
        ))}
      {section === 'Theme' && (
        <section className="task-detail-card desktop-form">
          <h2>{m.displayTheme()}</h2>
          <p>{m.themeDescription()}</p>
          <fieldset className="desktop-segmented">
            <legend>{m.theme()}</legend>
            {(['system', 'light', 'dark'] as const).map((option) => (
              <label key={option}>
                <input
                  type="radio"
                  name="desktop-theme"
                  checked={currentTheme === option}
                  onChange={() => {
                    setThemePreference(option);
                    onThemeChange?.(option);
                    setNotice(localized((m) => m.themeChanged()));
                  }}
                />
                <span>
                  {option === 'system'
                    ? m.followSystem()
                    : option === 'light'
                      ? m.light()
                      : m.dark()}
                </span>
              </label>
            ))}
          </fieldset>
        </section>
      )}
      {section === 'Language' && (
        <section className="task-detail-card desktop-form">
          <h2>{m.displayLanguage()}</h2>
          <p>{m.languageDescription()}</p>
          <fieldset className="desktop-segmented">
            <legend>{m.language()}</legend>
            {[
              ['system', m.followSystem()],
              ['zh-Hans', m.chinese()],
              ['en', m.english()],
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
                      setNotice(localized((m) => m.languageChanged()));
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
          <h2>{m.runtimeWorkerImages()}</h2>
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
            {m.settingsRefreshSavedReceipts()}
          </button>
          <p>{m.settingsImageReviewHint()}</p>
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
            {m.readCurrentImages()}
          </button>
          {!services.deployment && <p>{m.settingsImageUpdateUnavailable()}</p>}
          {images && (
            <>
              <p>
                {m.settingsRuntimeImageVersion(
                  images.runtimeId,
                  images.runtimeVersion,
                )}
                <br />
                {m.settingsToolImageVersion(images.toolId, images.toolVersion)}
              </p>
              <label>
                {m.runtimeImage()}
                <input
                  value={images.runtimeImage}
                  onChange={(event) => {
                    setImages({ ...images, runtimeImage: event.target.value });
                    setReview(undefined);
                  }}
                />
              </label>
              <label>
                {m.settingsWorkerToolImage()}
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
                      error instanceof ImageReviewError
                        ? localized((m) =>
                            m.settingsReviewRequired(error.field),
                          )
                        : error instanceof Error
                          ? error.message
                          : localized((m) => m.settingsReviewFailed()),
                    );
                  }
                }}
              >
                {m.settingsReviewExactTargets()}
              </button>
            </>
          )}
          {review && services.deployment && (
            <div className="document-save-notice">
              <h3>{m.settingsReviewUpdates()}</h3>
              <p>
                {m.settingsRuntimeId(review.runtimeId)}
                <br />
                {m.settingsExpectedVersion(review.expectedRuntimeVersion)}
                <br />
                {m.settingsImageValue(review.runtimeImage)}
              </p>
              <button
                disabled={busy}
                onClick={() =>
                  void act(async () => {
                    const operationId =
                      await services.deployment!.runtime(review);
                    setUpdate({
                      operationId,
                    });
                  })
                }
              >
                {m.settingsApplyRuntimeImage()}
              </button>
              <p>
                {m.settingsToolId(review.toolId)}
                <br />
                {m.settingsWorkerConfigurationUpdated(
                  review.expectedToolVersion,
                )}
                <br />
                {m.settingsImageValue(review.toolImage)}
              </p>
              <button
                disabled={busy}
                onClick={() =>
                  void act(async () => {
                    const operationId =
                      await services.deployment!.worker(review);
                    setUpdate({
                      operationId,
                    });
                  })
                }
              >
                {m.settingsApplyWorkerImage()}
              </button>
            </div>
          )}
          {updates.length > 0 && services.deployment && (
            <div>
              <h3>{m.settingsSavedImageReceipts()}</h3>
              {updates.map((operation) => (
                <div key={operation.operationId}>
                  <p>
                    {operation.operationId} ·{' '}
                    {renderMessage(operation.status, m)}
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
                    {m.settingsReadOperationStatus()}
                  </button>
                </div>
              ))}
            </div>
          )}
          {update && services.deployment && (
            <div role="status">
              <p>
                {m.settingsOperationStatus(
                  update.operationId,
                  update.status === undefined
                    ? m.settingsImageSubmitted()
                    : renderMessage(update.status, m),
                )}
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
                {m.settingsReadUpdateStatus()}
              </button>
            </div>
          )}
        </section>
      )}
    </main>
  );
}
