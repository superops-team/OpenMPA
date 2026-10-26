import { act, create, type ReactTestRenderer } from 'react-test-renderer';
import { expect, it, vi } from 'vitest';
const knowledge = vi.hoisted(() => ({
  list: vi.fn(),
  importText: vi.fn(),
  importStatus: vi.fn(),
}));
vi.mock('../src/client/desktop/generated/mygo', () => ({
  Knowledge: knowledge,
}));
import { DesktopKnowledge } from '../src/client/desktop/Knowledge';
it('shows accepted imports separately and only exposes READY after authoritative status read', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  knowledge.list.mockResolvedValue({ ok: true, value: { items: [] } });
  knowledge.importText.mockResolvedValue({
    ok: true,
    value: {
      importId: 'imp',
      knowledgeId: 'k',
      version: 1,
      status: 'RECEIVED',
    },
    idempotencyKey: 'key',
  });
  knowledge.importStatus.mockResolvedValue({
    ok: true,
    value: {
      importId: 'imp',
      knowledgeId: 'k',
      version: 1,
      displayName: 'Doc',
      status: 'READY',
      chunkCount: 1,
    },
  });
  let renderer!: ReactTestRenderer;
  await act(async () => {
    renderer = create(
      <DesktopKnowledge
        connection={{
          epoch: 9,
          scope: 'scope',
          runtimeId: 'r',
          status: 'connected',
        }}
        sessions={[{ id: 's', title: 'Session' }]}
      />,
    );
  });
  await act(async () =>
    renderer.root
      .findByType('select')
      .props.onChange({ target: { value: 's' } }),
  );
  const label = (name: string) =>
    renderer.root
      .findAllByType('label')
      .find((label) => label.children.includes(name))!;
  await act(async () => {
    label('Display name')
      .findByType('input')
      .props.onChange({ target: { value: 'Doc' } });
    label('Text material')
      .findByType('textarea')
      .props.onChange({ target: { value: 'Reference' } });
  });
  const button = (text: string) =>
    renderer.root
      .findAllByType('button')
      .find((button) =>
        button.children.some(
          (child) => typeof child === 'string' && child.trim() === text,
        ),
      )!;
  await act(async () => button('Import text').props.onClick());
  expect(JSON.stringify(renderer.toJSON())).toContain('RECEIVED');
  expect(JSON.stringify(renderer.toJSON())).not.toContain('READY');
  await act(async () =>
    button('Read authoritative import status').props.onClick(),
  );
  expect(JSON.stringify(renderer.toJSON())).toContain('READY');
  expect(knowledge.importStatus).toHaveBeenCalledWith({
    connectionEpoch: 9,
    sessionId: 's',
    importId: 'imp',
  });
  await act(async () => renderer.unmount());
});
it('releases the rejected import while keeping text available after an explicit validation failure', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  knowledge.list.mockResolvedValue({ ok: true, value: { items: [] } });
  knowledge.importText.mockResolvedValue({
    ok: false,
    error: {
      code: 'invalidInput',
      message: 'Invalid text',
      httpStatus: 422,
      retryDisposition: 'never',
    },
  });
  let renderer!: ReactTestRenderer;
  await act(async () => {
    renderer = create(
      <DesktopKnowledge
        connection={{
          epoch: 9,
          scope: 'scope',
          runtimeId: 'r',
          status: 'connected',
        }}
        sessions={[{ id: 's', title: 'Session' }]}
      />,
    );
  });
  await act(async () =>
    renderer.root
      .findByType('select')
      .props.onChange({ target: { value: 's' } }),
  );
  const label = (name: string) =>
    renderer.root
      .findAllByType('label')
      .find((label) => label.children.includes(name))!;
  await act(async () => {
    label('Display name')
      .findByType('input')
      .props.onChange({ target: { value: 'Doc' } });
    label('Text material')
      .findByType('textarea')
      .props.onChange({ target: { value: 'Retained text' } });
  });
  const button = () =>
    renderer.root
      .findAllByType('button')
      .find((button) =>
        button.children.some(
          (child) =>
            typeof child === 'string' && child.trim() === 'Import text',
        ),
      )!;
  await act(async () => button().props.onClick());
  expect(button().props.disabled).toBe(false);
  expect(label('Text material').findByType('textarea').props.value).toBe(
    'Retained text',
  );
  await act(async () => renderer.unmount());
});
