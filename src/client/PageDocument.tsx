import type { Page } from '../server/pages';
import type { WorkspaceState } from '../shared/types';
import { api } from './api';
import { PageConversation } from './PageConversation';
import { PageDocumentView } from './PageDocumentView';
export function PageDocument({
  page,
  pages,
  workspace,
  paused,
  onHome,
  onOutline,
  onSubpage,
  onDirty,
  onSaved,
  onRefresh,
  onDeleted,
  onSchedule,
  onThread,
  onSettings,
  onCreateDot,
}: {
  page: Page;
  pages: Page[];
  workspace: WorkspaceState;
  paused: boolean;
  onHome: () => void;
  onOutline: () => void;
  onSubpage: () => void;
  onDirty: (value: boolean) => void;
  onSaved: (page: Page) => void;
  onRefresh: () => void;
  onDeleted: (id: string) => void;
  onSchedule: (id: string) => void;
  onThread: (id: string) => void;
  onSettings: () => void;
  onCreateDot: () => void;
}) {
  return (
    <PageDocumentView
      page={page}
      pages={pages}
      onHome={onHome}
      onOutline={onOutline}
      onSubpage={onSubpage}
      onDirty={onDirty}
      onSaved={onSaved}
      onRefresh={onRefresh}
      onDeleted={onDeleted}
      onThread={onThread}
      services={{
        save: (id, patch, signal) =>
          api<Page>(
            `/spaces/${page.spaceId}/pages/${id}`,
            'PATCH',
            patch,
            signal,
          ),
        read: (id) => api<Page>(`/spaces/${page.spaceId}/pages/${id}`),
        remove: async (id) => {
          await api(`/spaces/${page.spaceId}/pages/${id}`, 'DELETE');
        },
      }}
      renderConversation={(beforeChat, onOpenChange) => (
        <PageConversation
          page={page}
          workspace={workspace}
          paused={paused}
          beforeChat={beforeChat}
          onRefresh={onRefresh}
          onSchedule={onSchedule}
          onSettings={onSettings}
          onCreateDot={onCreateDot}
          onOpenChange={onOpenChange}
        />
      )}
    />
  );
}
