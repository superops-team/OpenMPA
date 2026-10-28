import { useEffect, useRef, useState } from 'react';
import { CheckCircle2, Circle, AlertCircle } from 'lucide-react';
import { Channels, type Diagnostics } from './generated/mygo';
import { unwrap } from './result';
import { useMessages, useMessageState } from './locale';
import type { ChannelVM } from './models';
import { studioFailure } from './studio-services';

export function ChannelAccountOverview({
  account,
  epoch,
  revision,
}: {
  account: ChannelVM;
  epoch: number;
  revision?: number;
}) {
  const m = useMessages();
  const [status, setStatus] = useState<Diagnostics>();
  const [error, setError] = useMessageState();
  const sequence = useRef(0);
  useEffect(() => {
    const id = ++sequence.current;
    setStatus(undefined);
    setError('');
    void Channels.diagnostics({
      connectionEpoch: epoch,
      channel: account.type,
      appId: account.id,
    })
      .then((result) => {
        if (id === sequence.current) setStatus(unwrap(result));
      })
      .catch((reason) => {
        if (id === sequence.current) setError(studioFailure(reason));
      });
    return () => {
      sequence.current++;
    };
  }, [epoch, account.type, account.id, account.enabled, revision]);
  const rows = status && [
    {
      name: m.channelGateway(),
      value: status.gatewayConfigured
        ? m.channelConfigured()
        : m.channelMissingConfiguration(),
      ok: status.gatewayConfigured,
    },
    {
      name: m.channelRoute(),
      value: status.routeConfigured
        ? m.channelConfigured()
        : m.channelMissingConfiguration(),
      ok: status.routeConfigured,
    },
    {
      name: m.channelDelivery(),
      value:
        status.deliveryHealth === 'delivered'
          ? m.channelDelivered()
          : status.deliveryHealth === 'failed'
            ? m.desktopFailed()
            : m.channelDeliveryNotObserved(),
      ok: status.deliveryHealth === 'delivered',
    },
  ];
  return (
    <>
      {error ? (
        <p className="channel-inline-error" role="alert">
          <AlertCircle size={16} aria-hidden="true" />
          {error}
        </p>
      ) : !status ? (
        <p role="status" className="channel-muted">
          {m.desktopLoading()}
        </p>
      ) : (
        <dl className="channel-health">
          {rows &&
            rows.map((row) => (
              <div key={row.name}>
                <dt>{row.name}</dt>
                <dd data-ok={row.ok}>
                  {row.ok ? (
                    <CheckCircle2 size={16} aria-hidden="true" />
                  ) : (
                    <Circle size={16} aria-hidden="true" />
                  )}
                  {row.value}
                </dd>
              </div>
            ))}
        </dl>
      )}
    </>
  );
}
