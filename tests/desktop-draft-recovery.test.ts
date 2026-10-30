import { expect, it, vi } from 'vitest';
import { PageAutosave } from '../src/client/editor/autosave';
it('preserves an older recovered draft as a conflict without writing over a newer saved revision', async () => {
  const save = vi.fn();
  const controller = new PageAutosave(save);
  controller.receive({
    id: 'p',
    spaceId: 's',
    title: 'Remote',
    content: 'new',
    revision: 3,
    parentId: null,
    sourceThreadId: null,
    createdAt: 1,
    updatedAt: 3,
  });
  controller.restoreDraft(
    { title: 'Draft', content: 'old', parentId: null },
    1,
  );
  expect(controller.getSnapshot()).toMatchObject({
    status: 'conflict',
    draft: { content: 'old' },
    page: { revision: 3 },
  });
  expect(await controller.flush(true)).toBe(false);
  expect(save).not.toHaveBeenCalled();
  controller.dispose();
});
