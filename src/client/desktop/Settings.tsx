import { RefreshCw, Check, LogOut } from 'lucide-react';
import type { ConnectionVM, DesktopServices } from './models';
import { reviewImages } from './image-review';
import { RuntimeConfiguration } from './RuntimeConfiguration';
import { useDesktopSettingsState } from './SettingsState';
import { SettingsAccountForm } from './SettingsAccountForm';
export function DesktopSettings({
  services,
  connection,
  onConnection,
}: {
  services: DesktopServices;
  connection?: ConnectionVM;
  onConnection: (connection: ConnectionVM | undefined) => void;
}) {
  const state = useDesktopSettingsState({ services, connection, onConnection });
  const {
    error,
    notice,
    platform,
    busy,
    act,
    setPlatform,
    setRuntimes,
    profiles,
    generation,
    region,
    project,
    setRegion,
    setProject,
    directoryLoading,
    load,
    query,
    setQuery,
    visibleRuntimes,
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
  return (
    <main className="main-content desktop-settings">
      <div className="page-heading">
        <div>
          <span className="eyebrow">SETTINGS & SETUP</span>
          <h1>Your account and Runtime.</h1>
          <p>
            Choose an MPA Runtime from the current cloud account. Business
            identity is configured separately.
          </p>
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
      <section className="task-detail-card">
        <h2>Cloud account</h2>
        {platform?.signedIn ? (
          <>
            <p>
              <Check size={14} /> Account {platform.accountId}
            </p>
            <button
              className="text-button"
              disabled={busy}
              onClick={() =>
                void act(async () => {
                  await services.platform.signOut();
                  setPlatform(undefined);
                  setRuntimes([]);
                  onConnection(undefined);
                })
              }
            >
              <LogOut size={14} /> Sign out
            </button>
          </>
        ) : (
          <SettingsAccountForm {...state} />
        )}
        {profiles.length > 0 && (
          <div>
            <h3>Saved accounts</h3>
            {profiles.map((profile) => (
              <div className="desktop-runtime" key={profile.profileId}>
                <div>
                  <strong>{profile.label || profile.accountId}</strong>
                  <p>
                    Account {profile.accountId}
                    {profile.runtimeId ? ` · Runtime ${profile.runtimeId}` : ''}
                  </p>
                </div>
                <button
                  className="text-button"
                  disabled={busy}
                  onClick={() =>
                    void act(async () => {
                      const current = generation.current;
                      try {
                        const value = await services.platform.restore(
                          profile.profileId,
                        );
                        if (generation.current === current) setPlatform(value);
                      } finally {
                        const value = await services.platform.status();
                        const next = await services.connection.status();
                        if (generation.current === current) {
                          setPlatform(value);
                          onConnection(next);
                        }
                      }
                    })
                  }
                >
                  Restore account
                </button>
              </div>
            ))}
          </div>
        )}
      </section>
      <section className="task-detail-card desktop-form">
        <h2>AgentKit MPA Runtimes</h2>
        <p>
          {platform?.signedIn
            ? `Account ${platform.accountId} · ${region} · ${project}`
            : 'Sign in to choose an MPA Runtime.'}
        </p>
        <div className="desktop-inline">
          <label>
            Region
            <input
              disabled={busy}
              value={region}
              onChange={(event) => setRegion(event.target.value)}
            />
          </label>
          <label>
            Project
            <input
              disabled={busy}
              value={project}
              onChange={(event) => setProject(event.target.value)}
            />
          </label>
          <button
            className="primary"
            disabled={busy || directoryLoading || !platform?.signedIn}
            onClick={() => void act(load)}
          >
            <RefreshCw size={14} /> List current account
          </button>
        </div>
        <label>
          Filter Runtimes
          <input
            placeholder="Name or Runtime ID"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
          />
        </label>
        {directoryLoading && <p role="status">Loading MPA Runtimes…</p>}
        {platform?.signedIn && !directoryLoading && !visibleRuntimes.length && (
          <p>
            No matching MPA Runtimes. Check the region, project or filter, then
            refresh.
          </p>
        )}
        <div className="task-list">
          {visibleRuntimes.map((runtime) => (
            <div className="desktop-runtime" key={runtime.id}>
              <div>
                <strong>{runtime.name || runtime.id}</strong>
                <p>
                  {runtime.id} · {runtime.status} · version {runtime.version}
                </p>
              </div>
              <button
                className="primary"
                disabled={
                  busy ||
                  directoryLoading ||
                  connection?.runtimeId === runtime.id
                }
                onClick={() =>
                  void act(async () => {
                    const current = generation.current;
                    const next = await services.platform.bind(
                      region,
                      project,
                      runtime.id,
                    );
                    if (generation.current === current) {
                      onConnection(next);
                      setNotice(
                        'Runtime selected. Verify business identity and connection.',
                      );
                    }
                  })
                }
              >
                {connection?.runtimeId === runtime.id
                  ? 'Selected'
                  : 'Use Runtime'}
              </button>
            </div>
          ))}
        </div>
      </section>
      {connection && (
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
      {connection && (
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
      {connection && (
        <RuntimeConfiguration
          key={connection.epoch}
          connection={connection}
          busy={busy}
          run={act}
        />
      )}
      {connection && (
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
