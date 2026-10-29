import { act, create, type ReactTestRenderer } from 'react-test-renderer';
import { readFileSync } from 'node:fs';
import { expect, it, vi } from 'vitest';
import { TurnProcess } from '../src/client/desktop/TurnProcess';
import type { ConversationTurn } from '../src/client/desktop/turn-model';

it.each(['completed', 'failed', 'stopped'] as const)(
  'settles stale partial rows when a %s process is expanded',
  async (status) => {
    vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
    let renderer!: ReactTestRenderer;
    await act(async () => {
      renderer = create(<TurnProcess turn={{ ...runningTurn, status }} />);
    });
    await act(async () =>
      renderer.root
        .findByProps({ 'aria-label': 'Expand execution process' })
        .props.onClick(),
    );
    const output = JSON.stringify(renderer.toJSON());
    expect(output).not.toContain('Thinking');
    expect(output).not.toContain('Running');
    expect(output).toContain(
      `${status === 'completed' ? 'Completed' : status === 'failed' ? 'Failed' : 'Stopped'} · 1.2s`,
    );
    expect(output).toContain('Completed · 423ms');
    await act(async () => renderer.unmount());
  },
);

it.each(['completed', 'failed', 'stopped'] as const)(
  'settles unfinished tools and narration in a %s process without inventing tool success',
  async (status) => {
    vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
    let renderer!: ReactTestRenderer;
    const turn: ConversationTurn = {
      ...runningTurn,
      status,
      segments: [
        {
          type: 'tool',
          id: 'pending',
          toolId: 'pending',
          name: 'read',
          status: 'running',
          durationMs: 200,
        },
        {
          type: 'text',
          id: 'progress',
          role: 'assistant',
          kind: 'text',
          content: 'Reading files',
          partial: true,
          durationMs: 300,
        },
        {
          type: 'text',
          id: 'answer',
          role: 'assistant',
          kind: 'text',
          content: 'Answer',
          partial: false,
        },
      ],
    };
    await act(async () => {
      renderer = create(<TurnProcess turn={turn} />);
    });
    await act(async () =>
      renderer.root
        .findByProps({ 'aria-label': 'Expand execution process' })
        .props.onClick(),
    );
    const output = JSON.stringify(renderer.toJSON());
    expect(output).not.toContain('Running');
    expect(output).toContain(
      `${status === 'failed' ? 'Failed' : 'Stopped'} · 200ms`,
    );
    expect(output).toContain(
      `${status === 'completed' ? 'Completed' : status === 'failed' ? 'Failed' : 'Stopped'} · 300ms`,
    );
    await act(async () => renderer.unmount());
  },
);

const runningTurn: ConversationTurn = {
  invocationId: 'inv-1',
  status: 'running',
  segments: [
    {
      type: 'reasoning',
      id: 'reasoning',
      visibility: 'summary',
      content:
        'Checking the event projection chain.\nCompare the terminal snapshot with live steps.',
      partial: true,
      durationMs: 1200,
    },
    {
      type: 'tool',
      id: 'tool',
      toolId: 'tool-1',
      name: 'commandExecution',
      status: 'success',
      subject: "/bin/bash -lc 'ps aux | wc -l'",
      durationMs: 423,
    },
    {
      type: 'text',
      id: 'usage',
      role: 'assistant',
      kind: 'text',
      content: '[usage] 42 tokens',
      partial: false,
      durationMs: 5400,
    },
  ],
};

it('shows readable running steps with reasoning, command details and durations', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  let renderer!: ReactTestRenderer;
  await act(async () => {
    renderer = create(<TurnProcess turn={runningTurn} />);
  });

  const toggle = renderer.root.findByProps({
    'aria-label': 'Collapse execution process',
  });
  expect(toggle.props['aria-expanded']).toBe(true);
  const output = JSON.stringify(renderer.toJSON());
  expect(output).toContain('Working · 3 steps');
  expect(output).toContain('Reasoning');
  expect(output).toContain('Checking the event projection chain.');
  expect(output).toContain('Compare the terminal snapshot with live steps.');
  expect(output).toContain('Thinking · 1.2s');
  expect(output).toContain('Run command');
  expect(output).toContain("/bin/bash -lc 'ps aux | wc -l'");
  expect(output).toContain('Completed · 423ms');
  expect(output).toContain('Usage');
  expect(output).toContain('42 tokens');
  expect(output).toContain('Completed · 5.4s');
  expect(output).not.toContain('Details hidden');
  expect(output).not.toContain('Inspect Run command');

  await act(async () => toggle.props.onClick());
  expect(JSON.stringify(renderer.toJSON())).not.toContain(
    "/bin/bash -lc 'ps aux | wc -l'",
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
    'aria-label': 'Expand execution process',
  });
  expect(toggle.props['aria-expanded']).toBe(false);
  expect(JSON.stringify(renderer.toJSON())).toContain('Worked · 3 steps');
  expect(JSON.stringify(renderer.toJSON())).not.toContain(
    "/bin/bash -lc 'ps aux | wc -l'",
  );
  await act(async () => renderer.unmount());
});

it('keeps live steps expanded when the turn settles and updates their status in place', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  let renderer!: ReactTestRenderer;
  await act(async () => {
    renderer = create(<TurnProcess turn={runningTurn} />);
  });
  await act(async () => {
    renderer.update(
      <TurnProcess turn={{ ...runningTurn, status: 'completed' }} />,
    );
  });
  expect(
    renderer.root.findByProps({ 'aria-label': 'Collapse execution process' })
      .props['aria-expanded'],
  ).toBe(true);
  const output = JSON.stringify(renderer.toJSON());
  expect(output).toContain('Worked · 3 steps');
  expect(output).toContain('Completed · 1.2s');
  expect(output).not.toContain('Thinking');
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
