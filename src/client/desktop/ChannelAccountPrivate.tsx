import { Channels } from './generated/mygo';
import { unwrap } from './result';
import { localized, useMessages } from './locale';
import {
  channelStatusLabel,
  type useChannelManagementState,
} from './ChannelManagementState';
export function ChannelAccountPrivate({
  state,
  disabled,
}: {
  state: ReturnType<typeof useChannelManagementState>;
  disabled: boolean;
}) {
  const m = useMessages();
  const {
    account,
    privateUser,
    setPrivateUser,
    run,
    accountRequest,
    sessionId,
    setSessionId,
    sessions,
    setNotice,
    selectedFeishu,
    capabilities,
  } = state;
  const busy = state.busy || disabled;
  return (
    <div className="channel-private-tools">
      {account?.bindingType !== 'shared' && (
        <button
          type="button"
          className="channel-button"
          disabled={
            busy || !selectedFeishu || !capabilities?.ownerIdentityAvailable
          }
          onClick={() =>
            void run(async (current) => {
              const value = unwrap(
                await Channels.privateUser(accountRequest()),
              );
              if (current()) setPrivateUser(value);
            })
          }
        >
          {m.desktopReadPrivateUserAndSlot()}
        </button>
      )}
      {privateUser && (
        <div className="document-save-notice">
          <h3>{m.desktopPrivateUserAuthorization()}</h3>
          <p>
            {m.desktopAuthorization()}{' '}
            {channelStatusLabel(privateUser.authorization.status, m)}{' '}
            {m.desktopSlotVersion()}{' '}
            {privateUser.slot?.version || m.desktopNoSlot()} {m.desktopUser()}{' '}
            {privateUser.slot?.runtimeUserId || m.channelUnbound()}
          </p>
          <button
            disabled={busy || !privateUser.slot}
            onClick={() => {
              const expectedVersion = privateUser.slot?.version;
              if (
                !expectedVersion ||
                !window.confirm(m.revokePrivateConfirm(expectedVersion))
              )
                return;
              void run(async (current) => {
                const value = unwrap(
                  await Channels.revokePrivateAuthorization({
                    ...accountRequest(),
                    expectedVersion,
                  }),
                );
                if (current()) setPrivateUser(value.current);
              });
            }}
          >
            {m.desktopRevokeAuthorization()}
          </button>
          <button
            disabled={busy || !privateUser.slot}
            onClick={() => {
              const expectedVersion = privateUser.slot?.version;
              if (
                !expectedVersion ||
                !window.confirm(m.resetPrivateConfirm(expectedVersion))
              )
                return;
              void run(async (current) => {
                const value = unwrap(
                  await Channels.resetPrivateUser({
                    ...accountRequest(),
                    expectedVersion,
                  }),
                );
                if (current()) setPrivateUser(value.current);
              });
            }}
          >
            {m.desktopResetPrivateUser()}
          </button>
        </div>
      )}
      <form
        className="desktop-form"
        onSubmit={(event) => {
          event.preventDefault();
          if (!window.confirm(m.channelUpgradeConfirm(sessionId))) return;
          void run(async (current) => {
            const value = unwrap(
              await Channels.upgradeProfile({
                ...accountRequest(),
                sessionId,
              }),
            );
            if (current())
              setNotice(
                localized((m) =>
                  m.profileUpgradedNotice(
                    value.sessionId,
                    value.advancesExecutionVersion ? m.pageYes() : m.pageNo(),
                  ),
                ),
              );
          });
        }}
      >
        <label>
          {m.desktopExistingConversationForProfileUpgrade()}
          <select
            required
            value={sessionId}
            onChange={(event) => setSessionId(event.target.value)}
          >
            <option value="">{m.chooseConversation()}</option>
            {sessions.map((session) => (
              <option key={session.id} value={session.id}>
                {session.title}
              </option>
            ))}
          </select>
        </label>
        <button
          className="text-button"
          disabled={
            busy || !selectedFeishu || !capabilities?.ownerIdentityAvailable
          }
        >
          {m.desktopReviewAndUpgradeAccountProfile()}
        </button>
      </form>
    </div>
  );
}
