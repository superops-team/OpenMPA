import { Channels } from './generated/mygo';
import { unwrap } from './result';
import { useMessages } from './locale';
import {
  channelStatusLabel,
  type useChannelManagementState,
} from './ChannelManagementState';

export function ChannelBindingPanel({
  state,
}: {
  state: ReturnType<typeof useChannelManagementState>;
}) {
  const m = useMessages();
  const {
    bindingOperation,
    binding,
    busy,
    run,
    readBinding,
    setBinding,
    onChanged,
    connection,
  } = state;
  if (!binding)
    return bindingOperation ? (
      <p role="status" className="channel-muted">
        {m.channelBindingUncertain()}
      </p>
    ) : null;
  const active = ['waiting', 'pending', 'registering'].includes(
    binding.state.toLowerCase(),
  );
  return (
    <section className="channel-binding-result" aria-live="polite">
      <strong>{channelStatusLabel(binding.status, m)}</strong>
      {active && binding.qrCodeImage && (
        <img
          className="desktop-binding-qr"
          src={binding.qrCodeImage}
          alt={m.channelAuthorizeQr()}
        />
      )}
      {active && binding.expiresAt && (
        <p className="channel-muted">{m.channelExpires(binding.expiresAt)}</p>
      )}
      {binding.errorCode && (
        <p role="alert" className="channel-inline-error">
          {binding.errorCode === 'QR_MATERIAL_UNAVAILABLE'
            ? m.channelQrUnavailable()
            : binding.errorCode}
        </p>
      )}
      {binding.state === 'unknown' && (
        <p role="alert">{m.channelBindingUncertain()}</p>
      )}
      <div className="channel-actions">
        <button
          type="button"
          className="channel-button"
          disabled={busy || !binding.id}
          onClick={() =>
            void run(async (current) => {
              const value = await readBinding(binding);
              if (current()) setBinding(value);
              await onChanged();
            })
          }
        >
          {m.desktopReadBindingStatus()}
        </button>
        {!active && binding.state !== 'registered' && (
          <button
            type="button"
            className="channel-button"
            disabled={busy || !binding.id}
            onClick={() => {
              const uncertain = binding.state.toLowerCase() === 'unknown';
              if (uncertain && !window.confirm(m.channelRetryConfirm())) return;
              void run(async (current) => {
                const value = unwrap(
                  await Channels.retryBinding({
                    connectionEpoch: connection.epoch,
                    channel: binding.channel,
                    bindingId: binding.id || '',
                    operationId: binding.operationId,
                    gatewayVerified: uncertain,
                  }),
                );
                if (current()) setBinding(value);
              });
            }}
          >
            {m.desktopRetryThisBinding()}
          </button>
        )}
      </div>
      <details className="channel-operation-details">
        <summary>{m.channelOperationDetails()}</summary>
        <p>
          {m.desktopBindingID()}{' '}
          <code>{binding.id || m.channelAwaitingId()}</code>
        </p>
        <p>
          {m.desktopOperationID()} <code>{binding.operationId}</code>
        </p>
      </details>
    </section>
  );
}
