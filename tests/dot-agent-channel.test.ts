import { afterEach, expect, it, vi } from 'vitest';
import { EventType, type BaseEvent, type RunAgentInput } from '@ag-ui/core';
import { Observable, lastValueFrom, of, throwError, toArray } from 'rxjs';
import { DotAgent } from '../src/server/dot-agent.js';
import { Store } from '../src/server/store.js';
import { WorkspaceStore } from '../src/server/workspace.js';
const inner = vi.hoisted(() => ({
  run: vi.fn<() => Observable<BaseEvent>>(),
  abortRun: vi.fn(),
}));
vi.mock('@copilotkit/runtime/v2', async (importOriginal) => {
  const original =
    await importOriginal<typeof import('@copilotkit/runtime/v2')>();
  return {
    ...original,
    BuiltInAgent: class {
      run = inner.run;
      abortRun = inner.abortRun;
    },
  };
});
const databases: Array<{ close(): void }> = [];
afterEach(() => {
  databases.splice(0).forEach((db) => db.close());
  vi.restoreAllMocks();
});
function fixture(channel = true) {
  const store = new Store(':memory:');
  const workspace = new WorkspaceStore(':memory:', 'owner');
  databases.push(store, workspace);
  const dot = workspace.dots()[0];
  workspace.bindThread('thread', dot.id, 'Test');
  const agent = new DotAgent(
    store,
    workspace,
    {
      intelligenceKey: 'fixture',
      apiKey: 'fixture',
      model: 'fixture',
      baseUrl: 'https://unused.invalid',
      runtimeUrl: '',
      voiceName: 'marin',
      slackUsers: [],
    },
    dot.id,
    channel,
  );
  const input: RunAgentInput = {
    threadId: 'thread',
    runId: 'run',
    state: {},
    messages: [],
    tools: [],
    context: [],
    forwardedProps: {},
  };
  return { agent, input, workspace };
}
it('replaces channel RUN_ERROR payload entirely before the SDK renderer sees it', async () => {
  const f = fixture();
  inner.run.mockReturnValue(
    of({
      type: EventType.RUN_ERROR,
      message: 'SECRET token',
      code: 'SECRET code',
      rawEvent: { credential: 'SECRET' },
    }),
  );
  const events = await lastValueFrom(f.agent.run(f.input).pipe(toArray()));
  expect(events).toEqual([
    {
      type: EventType.RUN_ERROR,
      message:
        'OpenDots could not complete this request. Please check the app and try again.',
    },
  ]);
});
it('sanitizes observable errors and startup exceptions without retaining causes', async () => {
  const f = fixture();
  inner.run.mockReturnValue(throwError(() => new Error('SECRET transport')));
  const events = await lastValueFrom(f.agent.run(f.input).pipe(toArray()));
  expect(events[0].type).toBe(EventType.RUN_ERROR);
  expect(JSON.stringify(events)).not.toContain('SECRET');
  vi.spyOn(f.workspace, 'dot').mockImplementation(() => {
    throw new Error('SECRET startup');
  });
  const startup = await lastValueFrom(f.agent.run(f.input).pipe(toArray()));
  expect(startup).toEqual(events);
});
it('preserves normal channel text and existing web error behavior', async () => {
  const f = fixture();
  const text = {
    type: EventType.TEXT_MESSAGE_CONTENT,
    messageId: 'msg',
    delta: 'Normal user-facing text',
  };
  inner.run.mockReturnValue(of(text));
  expect(await lastValueFrom(f.agent.run(f.input).pipe(toArray()))).toEqual([
    text,
  ]);
  const web = fixture(false);
  const error = { type: EventType.RUN_ERROR, message: 'Provider details' };
  inner.run.mockReturnValue(of(error));
  expect(await lastValueFrom(web.agent.run(web.input).pipe(toArray()))).toEqual(
    [error],
  );
});
