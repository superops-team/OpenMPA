import { Channels } from './generated/mygo';
import { unwrap } from './result';
import { useMessages } from './locale';
import type { useChannelManagementState } from './ChannelManagementState';
export function ChannelBindingRecovery({
  state,
}: {
  state: ReturnType<typeof useChannelManagementState>;
}) {
  const m = useMessages();
  const {
    run,
    connection,
    channel,
    recovery,
    setRecovery,
    setBinding,
    busy,
    setSelected,
    onChanged,
  } = state;
  return (
    <details className="studio-channel-recovery">
      <summary>{m.channelResumeBinding()}</summary>
      <form
        className="desktop-form"
        onSubmit={(event) => {
          event.preventDefault();
          if (!window.confirm(m.channelResumeConfirm())) return;
          void run(async (current) => {
            const value = unwrap(
              await Channels.readBinding({
                connectionEpoch: connection.epoch,
                channel,
                operationId: recovery.operationId,
                bindingId: recovery.bindingId,
                resumeConfirmed: true,
              }),
            );
            if (current()) {
              setBinding(value);
              if (value.state === 'registered') {
                if (value.appId) setSelected(`${value.channel}:${value.appId}`);
                await onChanged();
              }
            }
          });
        }}
      >
        <label>
          {m.desktopOperationID()}
          <input
            required
            value={recovery.operationId}
            onChange={(event) =>
              setRecovery({ ...recovery, operationId: event.target.value })
            }
          />
        </label>
        <label>
          {m.desktopBindingID()}
          <input
            required
            value={recovery.bindingId}
            onChange={(event) =>
              setRecovery({ ...recovery, bindingId: event.target.value })
            }
          />
        </label>
        <button className="text-button" disabled={busy}>
          {m.desktopReadAndResume()}
        </button>
      </form>
    </details>
  );
}
