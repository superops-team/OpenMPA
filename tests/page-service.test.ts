import { expect, it, vi } from 'vitest';
import { WorkspaceStore } from '../src/server/workspace.js';
import { PageService } from '../src/server/page-service.js';
import { pageAccess } from '../src/server/page-tools.js';
it('reuses one actual Intelligence thread per page and Dot under concurrent requests', async () => {
  const ws = new WorkspaceStore(':memory:', 'owner');
  const dot = ws.dots()[0];
  const page = ws.pages.create(dot.spaceId, { title: 'Design' });
  const getOrCreateThread = vi.fn(async () => {});
  const sdk = {
    getOrCreateThread,
    getThreadMessages: async () => ({ messages: [] }),
  };
  const service = new PageService(ws, () => sdk);
  const [a, b] = await Promise.all([
    service.conversation(dot.spaceId, page.id, dot.id),
    service.conversation(dot.spaceId, page.id, dot.id),
  ]);
  expect(a.id).toBe(b.id);
  expect(getOrCreateThread).toHaveBeenCalledTimes(1);
  expect((await service.conversation(dot.spaceId, page.id, dot.id)).id).toBe(
    a.id,
  );
  expect(ws.pages.forThread(a.id, dot.spaceId)?.id).toBe(page.id);
  ws.close();
});
it('exports canonical user/assistant text and rejects failed or oversized history without creating a page', async () => {
  const ws = new WorkspaceStore(':memory:', 'owner');
  const dot = ws.dots()[0];
  ws.bindThread('thread', dot.id, 'Thread');
  const getThreadMessages = vi.fn(async () => ({
    messages: [
      { role: 'user', content: [{ type: 'text', text: 'Question' }] },
      { role: 'assistant', content: 'Answer' },
      { role: 'tool', content: 'Secret tool response' },
    ],
  }));
  const service = new PageService(ws, () => ({
    getOrCreateThread: async () => {},
    getThreadMessages,
  }));
  const page = await service.saveConversation('thread', 'Saved', null);
  expect(page.content).toBe('## You\n\nQuestion\n\n## Dot\n\nAnswer');
  expect(page.sourceThreadId).toBe('thread');
  getThreadMessages.mockRejectedValueOnce(new Error('Offline'));
  await expect(
    service.saveConversation('thread', 'Failure', null),
  ).rejects.toThrow();
  getThreadMessages.mockResolvedValueOnce({
    messages: [{ role: 'assistant', content: 'x'.repeat(100001) }],
  });
  await expect(
    service.saveConversation('thread', 'Too long', null),
  ).rejects.toThrow(/exceeds/);
  expect(ws.pages.list(dot.spaceId)).toHaveLength(1);
  ws.close();
});
it('scopes agent tools to the Dot Space and re-reads current context with CAS and pause enforcement', () => {
  const ws = new WorkspaceStore(':memory:', 'owner');
  const dot = ws.dots()[0];
  const other = ws.createSpace('Other', '');
  const foreign = ws.pages.create(other.id, { title: 'Private' });
  const page = ws.pages.create(dot.spaceId, { title: 'Here' });
  ws.bindThread('thread', dot.id, 'Page');
  ws.pages.reserveThread(page.id, dot.id, 'thread');
  ws.pages.finishThread(page.id, dot.id);
  let paused = false;
  const access = pageAccess(ws, dot.spaceId, 'thread', () => {
    if (paused) throw new Error('Paused');
  });
  expect(() => access.read(foreign.id)).toThrow();
  access.edit(page.id, { expectedRevision: 1, content: 'Fresh' });
  expect(access.context()?.content).toBe('Fresh');
  expect(() =>
    access.edit(page.id, { expectedRevision: 1, content: 'Stale' }),
  ).toThrow();
  paused = true;
  expect(() => access.create({ title: 'No write' })).toThrow('Paused');
  expect(ws.pages.list(dot.spaceId)).toHaveLength(1);
  ws.close();
});
it('recovers the same reserved thread after a restart lease and a remote-success retry', async () => {
  const ws = new WorkspaceStore(':memory:', 'owner');
  const dot = ws.dots()[0];
  const page = ws.pages.create(dot.spaceId, { title: 'Recover' });
  ws.pages.reserveThread(page.id, dot.id, 'stable-thread');
  const sdk = {
    getOrCreateThread: vi.fn(async () => {}),
    getThreadMessages: async () => ({ messages: [] }),
  };
  const service = new PageService(ws, () => sdk);
  await expect(
    service.conversation(dot.spaceId, page.id, dot.id),
  ).rejects.toThrow(/being created/);
  vi.spyOn(Date, 'now').mockReturnValue(Date.now() + 61000);
  const result = await service.conversation(dot.spaceId, page.id, dot.id);
  expect(result.id).toBe('stable-thread');
  expect(sdk.getOrCreateThread).toHaveBeenCalledWith(
    expect.objectContaining({
      threadId: 'stable-thread',
      userId: 'owner',
      agentId: dot.id,
    }),
  );
  vi.restoreAllMocks();
  ws.close();
});
it('rejects a specialist in another Space before Intelligence is accessed', async () => {
  const ws = new WorkspaceStore(':memory:', 'owner');
  const dot = ws.dots()[0];
  const space = ws.createSpace('Other', '');
  const page = ws.pages.create(space.id, { title: 'Other' });
  const getSdk = vi.fn(() => {
    throw new Error('Should not contact provider');
  });
  const service = new PageService(ws, getSdk);
  await expect(service.conversation(space.id, page.id, dot.id)).rejects.toThrow(
    /specialist in this Space/,
  );
  expect(getSdk).not.toHaveBeenCalled();
  ws.close();
});
it('retries a failed provider creation with the same canonical reserved ID', async () => {
  const ws = new WorkspaceStore(':memory:', 'owner');
  const dot = ws.dots()[0];
  const page = ws.pages.create(dot.spaceId, { title: 'Retry' });
  const getOrCreateThread = vi
    .fn(async () => {})
    .mockRejectedValueOnce(new Error('Remote response lost'));
  const service = new PageService(ws, () => ({
    getOrCreateThread,
    getThreadMessages: async () => ({ messages: [] }),
  }));
  await expect(
    service.conversation(dot.spaceId, page.id, dot.id),
  ).rejects.toThrow('Remote response lost');
  const reserved = ws.pages.thread(page.id, dot.id)!.threadId;
  const thread = await service.conversation(dot.spaceId, page.id, dot.id);
  expect(thread.id).toBe(reserved);
  expect(getOrCreateThread).toHaveBeenCalledTimes(2);
  ws.close();
});

it('grants multiple Spaces without changing thread identity and enforces revocation on existing tools', async () => {
  const ws = new WorkspaceStore(':memory:', 'owner');
  const dot = ws.dots()[0];
  const other = ws.createSpace('Launch', '');
  const page = ws.pages.create(other.id, { title: 'Brief' });
  ws.updateDot(dot.id, { ...dot, spaceIds: [dot.spaceId, other.id] });
  const service = new PageService(ws, () => ({
    getOrCreateThread: async () => {},
    getThreadMessages: async () => ({
      messages: [{ role: 'assistant', content: 'Saved text' }],
    }),
  }));
  const thread = await service.conversation(other.id, page.id, dot.id);
  const access = pageAccess(ws, dot.spaceId, thread.id, () => {});
  expect(access.context()?.id).toBe(page.id);
  expect(access.read(page.id).title).toBe('Brief');
  expect(access.spaces()).toHaveLength(2);
  expect(
    (await service.saveConversation(thread.id, 'Copy', null)).spaceId,
  ).toBe(other.id);
  ws.updateDot(dot.id, { ...dot, spaceIds: [dot.spaceId] });
  expect(() => access.read(page.id, other.id)).toThrow(/access/);
  expect(() =>
    access.edit(page.id, { expectedRevision: 1, content: 'No' }, other.id),
  ).toThrow(/access/);
  await expect(
    service.conversation(other.id, page.id, dot.id),
  ).rejects.toThrow();
  await expect(service.saveConversation(thread.id, 'No', null)).rejects.toThrow(
    /revoked/,
  );
  ws.updateDot(dot.id, { ...dot, spaceIds: [dot.spaceId, other.id] });
  expect((await service.conversation(other.id, page.id, dot.id)).id).toBe(
    thread.id,
  );
  ws.close();
});
