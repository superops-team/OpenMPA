import { act, create, type ReactTestRenderer } from 'react-test-renderer';
import { expect, it, vi } from 'vitest';
import { ConversationFixture } from '../src/client/desktop/ConversationFixture';
import { messages } from '../src/client/desktop/generated/locales';

it('renders running and completed turn-process states without private reasoning', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  let renderer!: ReactTestRenderer;
  await act(async () => {
    renderer = create(<ConversationFixture />);
  });
  const output = JSON.stringify(renderer.toJSON());
  expect(output).toContain(messages.chatProcessRunning(4));
  expect(output).toContain(messages.chatProcessCompleted(1));
  expect(output).toContain('Conversation rendering review');
  expect(output).not.toContain('hidden reasoning');
  await act(async () => renderer.unmount());
});

it('previews conversation, settings and knowledge with one accessible page switcher', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  let renderer!: ReactTestRenderer;
  await act(async () => {
    renderer = create(<ConversationFixture />);
  });

  const pageButton = (label: string) =>
    renderer.root
      .findAllByType('button')
      .find((node) => node.children.includes(label));

  expect(pageButton('Conversation')?.props['aria-current']).toBe('page');
  await act(async () => pageButton('Settings')?.props.onClick());
  expect(JSON.stringify(renderer.toJSON())).toContain('Display and behavior');
  expect(pageButton('Settings')?.props['aria-current']).toBe('page');

  await act(async () => pageButton('Knowledge')?.props.onClick());
  const output = JSON.stringify(renderer.toJSON());
  expect(output).toContain('Knowledge library');
  expect(output).toContain('OpenMPA design system');
  expect(pageButton('Knowledge')?.props['aria-current']).toBe('page');

  expect(
    renderer.root.findByProps({ 'aria-label': 'Use light theme' }).props[
      'aria-pressed'
    ],
  ).toBe(true);
  await act(async () => renderer.unmount());
});
