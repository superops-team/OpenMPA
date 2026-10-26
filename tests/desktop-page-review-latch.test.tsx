import { act, create, type ReactTestRenderer } from 'react-test-renderer';
import { expect, it, vi } from 'vitest';
import type { DesktopServices } from '../src/client/desktop/models';
vi.mock('../src/client/SpaceLibrary', () => ({
  SpaceLibrary: ({ onPage }: { onPage: (id: string) => void }) => (
    <button onClick={() => onPage('p')}>Open page</button>
  ),
}));
vi.mock('../src/client/PageDocumentView', () => ({
  PageDocumentView: ({
    renderConversation,
  }: {
    renderConversation: (
      save: () => Promise<boolean>,
      open: (value: boolean) => void,
    ) => unknown;
  }) =>
    renderConversation(
      async () => true,
      () => {},
    ),
}));
vi.mock('../src/client/desktop/Chat', () => ({
  DesktopChat: ({
    onSaveReply,
  }: {
    onSaveReply: (message: {
      id: string;
      role: string;
      content: string;
      invocationId: string;
      partial: boolean;
      kind: string;
    }) => Promise<void>;
  }) => (
    <button
      onClick={() =>
        onSaveReply({
          id: 'e',
          role: 'assistant',
          content: 'Reply',
          invocationId: 'i',
          partial: false,
          kind: 'text',
        })
      }
    >
      Review reply
    </button>
  ),
}));
import { DesktopPages } from '../src/client/desktop/Pages';
it('latches review confirmation so two immediate clicks submit only one fixed receipt', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  vi.stubGlobal('window', { confirm: () => true });
  const page = {
    id: 'p',
    spaceId: 'space',
    parentId: null,
    title: 'Page',
    content: 'Base',
    revision: 3,
    createdAt: 1,
    updatedAt: 1,
    sourceThreadId: null,
  };
  let finish!: (value: typeof page) => void;
  const saved = new Promise<typeof page>((resolve) => {
    finish = resolve;
  });
  const saveReply = vi.fn(
    (
      _page: unknown,
      _scope: string,
      _session: string,
      _message: unknown,
      _receipt: string,
    ) => saved,
  );
  const services = {
    workspace: {
      state: async () => ({ readOnly: false, recoveryCode: '' }),
      pages: async () => [page],
      read: async () => page,
      linked: async () => 's',
      link: async () => {},
      saveReply,
    },
  } as unknown as DesktopServices;
  let renderer!: ReactTestRenderer;
  await act(async () => {
    renderer = create(
      <DesktopPages
        services={services}
        space={{ id: 'space', name: 'Space', description: '', createdAt: 1 }}
        connection={{
          epoch: 2,
          runtimeId: 'r',
          scope: 'scope',
          status: 'ready',
        }}
        sessions={[{ id: 's', title: 'Bot' }]}
        onDirty={() => {}}
        onSchedule={() => {}}
        onOpenSession={() => {}}
      />,
    );
  });
  const button = (text: string) =>
    renderer.root
      .findAllByType('button')
      .find((button) => button.children.includes(text))!;
  await act(async () => button('Open page').props.onClick());
  await act(async () => button('Open page chat').props.onClick());
  await act(async () => button('Review reply').props.onClick());
  await act(async () => {
    const form = renderer.root.findByType('form');
    form.props.onSubmit({ preventDefault() {} });
    form.props.onSubmit({ preventDefault() {} });
  });
  expect(saveReply).toHaveBeenCalledTimes(1);
  expect(saveReply.mock.calls[0][4]).toEqual(expect.any(String));
  await act(async () => finish({ ...page, revision: 4 }));
  await act(async () => renderer.unmount());
});
