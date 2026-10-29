import { useEffect, useState } from 'react';
import type { DesktopServices, RecoveryDraftVM } from './models';
import { errorMessage, localized, type MessageText } from './locale';

export function useDesktopCloseProtocol({
  services,
  setError,
}: {
  services: DesktopServices;
  setError: (message: MessageText) => void;
}) {
  const [closeDrafts, setCloseDrafts] = useState<RecoveryDraftVM[]>([]);
  useEffect(() => {
    if (!services.host) return;
    let active = true;
    const handled = new Set<number>();
    const close = async (requestId: number) => {
      if (!active || handled.has(requestId)) return;
      handled.add(requestId);
      try {
        await services.workspace.flush();
        if (active) await services.host!.resolveClose(requestId, true);
      } catch (error) {
        if (active) {
          if (error && typeof error === 'object' && 'closeState' in error) {
            const state = error.closeState as {
              unsavedDrafts?: RecoveryDraftVM[];
            };
            if (Array.isArray(state.unsavedDrafts))
              setCloseDrafts(state.unsavedDrafts);
          }
          setError(
            errorMessage(
              error,
              localized((m) =>
                m.desktopDraftRecoveryCouldNotBeSavedContinueEditingOr(),
              ),
            ),
          );
          await services.host!.resolveClose(requestId, false);
        }
      }
    };
    const remove = services.host.onClose((id) => void close(id));
    void services.host
      .closeState()
      .then((state) => {
        if (active && state.pending) void close(state.requestId);
      })
      .catch((error: unknown) => {
        if (active)
          setError(
            errorMessage(
              error,
              localized((m) => m.desktopCouldNotRegisterTheCloseProtocol()),
            ),
          );
      });
    return () => {
      active = false;
      remove();
    };
  }, [services, setError]);
  return { closeDrafts, setCloseDrafts };
}
