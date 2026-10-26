import { expect, it, vi } from 'vitest';
const workspace = vi.hoisted(() => ({
  init: vi.fn(),
  listSpaces: vi.fn(),
  saveDraft: vi.fn(),
  flush: vi.fn(),
}));
vi.mock('../src/client/desktop/generated/mygo', () => ({
  Artifact: {},
  Host: {},
  Platform: {},
  Connection: {},
  Deployment: {},
  Schedule: {},
  Channels: {},
  Session: {},
  SessionBot: {},
  events: {},
  Workspace: workspace,
}));
import { createDesktopServices } from '../src/client/desktop/services';
it('opens the safe recovered read-only snapshot without pretending it is writable', async () => {
  workspace.init.mockResolvedValue({
    ok: true,
    value: {
      readOnly: true,
      recoveryCode: 'RECOVERED_BACKUP',
      recoveryAvailable: true,
      snapshot: { spaces: [] },
    },
  });
  workspace.listSpaces.mockResolvedValue({
    ok: true,
    value: {
      spaces: [
        { id: 's', name: 'Recovered', revision: 1, createdAt: 1, updatedAt: 1 },
      ],
      storeRevision: 1,
    },
  });
  expect(await createDesktopServices().workspace.spaces()).toEqual([
    {
      id: 's',
      name: 'Recovered',
      revision: 1,
      createdAt: 1,
      updatedAt: 1,
      description: '',
    },
  ]);
});
it('asks the Go durability owner whether failed drafts still block closing', async () => {
  const services = createDesktopServices();
  workspace.saveDraft.mockResolvedValue({
    ok: false,
    error: { code: 'READ_ONLY', message: 'Unable to save draft', status: 503 },
  });
  const page = {
    id: 'p',
    spaceId: 's',
    title: 'Title',
    content: 'Base',
    revision: 3,
    parentId: null,
    createdAt: 1,
    updatedAt: 1,
    sourceThreadId: null,
  };
  await expect(
    services.workspace.preserveDraft(page, {
      title: 'Title',
      content: 'Unsaved',
      parentId: null,
    }),
  ).rejects.toThrow();
  workspace.flush
    .mockResolvedValueOnce({
      ok: false,
      error: {
        code: 'UNSAVED_DRAFTS',
        message: 'Draft remains',
        status: 409,
        closeState: { safeClose: false, unsavedDrafts: [{ pageId: 'p' }] },
      },
    })
    .mockResolvedValueOnce({
      ok: true,
      value: {
        changed: false,
        storeRevision: 1,
        closeState: { safeClose: true, readOnly: true, unsavedDrafts: [] },
      },
    });
  await expect(services.workspace.flush()).rejects.toThrow('Draft remains');
  await expect(services.workspace.flush()).resolves.toBeUndefined();
  expect(workspace.flush).toHaveBeenCalledTimes(2);
});
