import { act, create, type ReactTestRenderer } from 'react-test-renderer';
import { expect, it, vi } from 'vitest';
import { usePageAutosave } from '../src/client/editor/use-page-autosave';
import type { Page } from '../src/shared/page-types';
import type { PageAutosave } from '../src/client/editor/autosave';
const page: Page = {
  id: 'p',
  spaceId: 's',
  title: 'Saved',
  content: '',
  parentId: null,
  revision: 1,
  createdAt: 1,
  updatedAt: 1,
  sourceThreadId: null,
};
it('uses the injected document service and keeps a 409 draft for explicit review', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  const save = vi.fn(async () => {
    throw Object.assign(new Error('Changed'), {
      status: 409,
      currentPage: { ...page, revision: 2 },
    });
  });
  let controller: PageAutosave;
  function Harness() {
    controller = usePageAutosave(page, () => {}, save).controller;
    return null;
  }
  let renderer: ReactTestRenderer;
  await act(async () => {
    renderer = create(<Harness />);
  });
  await act(async () => {
    controller.edit({ content: 'My draft' });
    expect(await controller.flush(true)).toBe(false);
  });
  expect(save).toHaveBeenCalledOnce();
  expect(controller!.getSnapshot()).toMatchObject({
    status: 'conflict',
    draft: { content: 'My draft' },
  });
  await act(async () => renderer.unmount());
});
