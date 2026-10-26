import { expect, it, vi } from 'vitest';
const api = vi.hoisted(() => ({
  saveReviewedResult: vi.fn(),
  getPage: vi.fn(),
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
  Workspace: api,
}));
import { createDesktopServices } from '../src/client/desktop/services';
it('reuses the reviewed receipt and original page revision for repeated reviewed-output recovery', async () => {
  const page = {
    id: 'p',
    spaceId: 's',
    parentId: null,
    title: 'Title',
    content: 'Base',
    revision: 3,
    createdAt: 1,
    updatedAt: 1,
    sourceThreadId: null,
  };
  api.getPage.mockResolvedValue({ ok: true, value: page });
  api.saveReviewedResult.mockImplementation(async (r) => ({
    ok: true,
    value: { page: { ...page, content: r.content, revision: 4 }, receipt: {} },
  }));
  const services = createDesktopServices();
  const message = {
    id: 'e',
    role: 'assistant',
    content: 'Reply',
    invocationId: 'i',
    partial: false,
    kind: 'text',
  };
  await services.workspace.saveReply(
    page,
    'scope',
    'bot',
    message,
    'reviewed-receipt',
  );
  await services.workspace.saveReply(
    page,
    'scope',
    'bot',
    message,
    'reviewed-receipt',
  );
  expect(
    api.saveReviewedResult.mock.calls.map(([r]) => ({
      receiptId: r.receiptId,
      expectedRevision: r.expectedRevision,
      content: r.content,
    })),
  ).toEqual([
    {
      receiptId: 'reviewed-receipt',
      expectedRevision: 3,
      content: 'Base\n\nReply',
    },
    {
      receiptId: 'reviewed-receipt',
      expectedRevision: 3,
      content: 'Base\n\nReply',
    },
  ]);
});
