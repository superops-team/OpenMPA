import { useEffect, useState } from 'react';
import type { DesktopServices, TraceVM } from './models';

function failureMessage(failure: unknown) {
  const metadata =
    failure && typeof failure === 'object'
      ? (failure as { status?: unknown; httpStatus?: unknown; code?: unknown })
      : {};
  const status = Number(metadata.status ?? metadata.httpStatus);
  if (status === 404 || /notfound|not_found/i.test(String(metadata.code ?? '')))
    return 'Trace is unavailable for this turn. It may not have been collected, may have expired, or may require a Runtime with ordinary-session tracing enabled. Some Runtime versions only collect admin_debug traces.';
  if (status === 401 || status === 403)
    return 'The current Runtime identity is not allowed to read this trace.';
  return failure instanceof Error
    ? failure.message
    : 'Could not load this trace. Retry after the turn is accepted.';
}

type Resource = {
  key: string;
  trace?: TraceVM;
  loading: boolean;
  error: string;
};

export function useTraceResource({
  read,
  epoch,
  sessionId,
  invocationId,
}: {
  read: DesktopServices['trace']['read'];
  epoch?: number;
  sessionId?: string;
  invocationId: string;
}) {
  const key = JSON.stringify([epoch, sessionId, invocationId]);
  const enabled = epoch !== undefined && !!sessionId && !!invocationId;
  const [resource, setResource] = useState<Resource>({
    key: '',
    loading: false,
    error: '',
  });
  const [refresh, setRefresh] = useState(0);

  useEffect(() => {
    if (epoch === undefined || !sessionId || !invocationId) return;
    let cancelled = false;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const load = async () => {
      setResource((previous) => ({
        key,
        trace: previous.key === key ? previous.trace : undefined,
        loading: true,
        error: '',
      }));
      try {
        const trace = await read(epoch, sessionId, invocationId);
        if (cancelled) return;
        setResource({ key, trace, loading: false, error: '' });
        if (trace.coverage?.active === true || trace.run?.status === 'running')
          timer = setTimeout(() => {
            void load();
          }, 2000);
      } catch (failure) {
        if (!cancelled)
          setResource({ key, loading: false, error: failureMessage(failure) });
      }
    };
    void load();
    return () => {
      cancelled = true;
      clearTimeout(timer);
    };
  }, [epoch, sessionId, invocationId, key, read, refresh]);

  const visible =
    enabled && resource.key === key
      ? resource
      : { trace: undefined, loading: enabled, error: '' };
  return { ...visible, reload: () => setRefresh((value) => value + 1) };
}
