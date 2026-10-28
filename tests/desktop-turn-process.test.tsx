import { act, create, type ReactTestRenderer } from 'react-test-renderer';
import { readFileSync } from 'node:fs';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { TurnProcess } from '../src/client/desktop/TurnProcess';
import type { ConversationTurn } from '../src/client/desktop/turn-model';
import { messages } from '../src/client/desktop/generated/locales';

const m = messages;
beforeEach(() => vi.stubGlobal('mygo', { locale: 'zh-Hans' }));
afterEach(() => vi.unstubAllGlobals());

const runningTurn: ConversationTurn = {
  invocationId: 'inv-1',
  status: 'running',
  segments: [
    {
      type: 'reasoning',
      id: 'reasoning',
      visibility: 'hidden',
      content: 'must not render',
      partial: true,
    },
    {
      type: 'tool',
      id: 'tool',
      toolId: 'tool-1',
      name: 'read',
      status: 'running',
      subject: 'src/client/desktop/Chat.tsx',
    },
  ],
};

it('shows the running process without leaking hidden reasoning', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  const inspect = vi.fn();
  let renderer!: ReactTestRenderer;
  await act(async () => {
    renderer = create(
      <TurnProcess turn={runningTurn} onInspectTool={inspect} />,
    );
  });

  const toggle = renderer.root.findByProps({
    'aria-label': m.collapseSection(m.desktopExecutionProcess()),
  });
  expect(toggle.props['aria-expanded']).toBe(true);
  const output = JSON.stringify(renderer.toJSON());
  expect(output).toContain(m.chatProcessRunning(2));
  expect(output).toContain(m.desktopReasoning());
  expect(output).toContain(m.desktopDetailsHidden());
  expect(output).not.toContain('must not render');
  expect(output).toContain(m.desktopRead());
  expect(output).toContain('src/client/desktop/Chat.tsx');
  expect(output).toContain(m.desktopRunning());

  await act(async () => {
    renderer.root
      .findByProps({ 'aria-label': m.chatInspectTool(m.desktopRead()) })
      .props.onClick();
  });
  expect(inspect).toHaveBeenCalledWith('tool-1');

  await act(async () => toggle.props.onClick());
  expect(JSON.stringify(renderer.toJSON())).not.toContain(
    'src/client/desktop/Chat.tsx',
  );
  await act(async () => renderer.unmount());
});

it('collapses a completed process by default', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  let renderer!: ReactTestRenderer;
  await act(async () => {
    renderer = create(
      <TurnProcess turn={{ ...runningTurn, status: 'completed' }} />,
    );
  });
  const toggle = renderer.root.findByProps({
    'aria-label': m.expandSection(m.desktopExecutionProcess()),
  });
  expect(toggle.props['aria-expanded']).toBe(false);
  expect(JSON.stringify(renderer.toJSON())).toContain(
    m.chatProcessCompleted(2),
  );
  expect(JSON.stringify(renderer.toJSON())).not.toContain(
    'src/client/desktop/Chat.tsx',
  );
  await act(async () => renderer.unmount());
});

it('keeps disclosure feedback compact without animating layout', () => {
  const css = readFileSync(
    new URL('../src/client/ui/ui.css', import.meta.url),
    'utf8',
  );
  expect(css).toMatch(
    /\.ui-disclosure-trigger:active:not\(:disabled\)\s*\{[^}]*transform:\s*none;/s,
  );
  expect(css).toMatch(
    /\.ui-disclosure-trigger\s*>\s*svg\s*\{[^}]*transition:\s*transform\s+var\(--motion-state-duration\)\s+var\(--motion-ease-out\);/s,
  );
  expect(css).toMatch(
    /\.ui-disclosure-trigger:focus-visible\s*>\s*svg\s*\{[^}]*transition:\s*none;/s,
  );
  expect(css).not.toMatch(
    /\.ui-disclosure(?:-content)?[^}]*transition:[^;]*(?:height|width|padding|margin)/s,
  );
});
