import { afterEach, expect, it, vi } from 'vitest';
import { createHash } from 'node:crypto';
import { readFileSync } from 'node:fs';
import {
  CopilotKitIntelligence,
  LearnedSkillsError,
} from '@copilotkit/runtime/v2';
import type { RunAgentInput } from '@ag-ui/core';
import { lastValueFrom, toArray } from 'rxjs';
import { DotAgent } from '../src/server/dot-agent.js';
import { Store } from '../src/server/store.js';
import { WorkspaceStore } from '../src/server/workspace.js';

afterEach(() => vi.restoreAllMocks());

it('native delivery adds a verified published catalog and skill tools to the existing model request', async () => {
  const store = new Store(':memory:');
  const workspace = new WorkspaceStore(':memory:', 'owner');
  try {
    const dot = workspace.dots()[0];
    workspace.updateDot(dot.id, {
      ...dot,
      learningContainerId: 'research',
      skillDeliveryEnabled: true,
    });
    workspace.bindThread('thread', dot.id, 'Learning');
    const bytes = readFileSync(
      new URL('./fixtures/learning-skills.zip', import.meta.url),
    );
    vi.spyOn(
      CopilotKitIntelligence.prototype,
      'getLearnedSkillsSnapshots',
    ).mockResolvedValue([
      {
        containerId: 'research',
        status: 'snapshot',
        bytes,
        revision: 'fixture-v1',
        etag: `"${createHash('sha256').update(bytes).digest('hex')}"`,
        contentType: 'application/zip',
      },
    ]);
    const network = vi
      .spyOn(globalThis, 'fetch')
      .mockResolvedValue(
        new Response(
          `data: ${JSON.stringify({ id: 'fixture', object: 'chat.completion.chunk', created: 1, model: 'fixture', choices: [{ index: 0, delta: { role: 'assistant', content: 'Ready to review evidence.' }, finish_reason: null }] })}\n\n` +
            `data: ${JSON.stringify({ id: 'fixture', object: 'chat.completion.chunk', created: 1, model: 'fixture', choices: [{ index: 0, delta: {}, finish_reason: 'stop' }] })}\n\ndata: [DONE]\n\n`,
          { headers: { 'Content-Type': 'text/event-stream' } },
        ),
      );
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
    );
    const events = await lastValueFrom(
      agent
        .run({
          threadId: 'thread',
          runId: 'run',
          messages: [
            { id: 'message', role: 'user', content: 'Review the evidence.' },
          ],
          state: {},
          tools: [],
          context: [],
          forwardedProps: {},
        })
        .pipe(toArray()),
    );
    expect(JSON.stringify(events)).toContain('Ready to review evidence.');
    expect(network).toHaveBeenCalledTimes(1);
    const request = String(network.mock.calls[0][1]?.body);
    expect(request).toContain('evidence-review');
    expect(request).toContain('copilotkit_load_skill');
    expect(request).toContain('copilotkit_read_skill_file');
    expect(request).toContain('read_space_page');
    expect(request).toContain('Use only the authorized server tools');
  } finally {
    workspace.close();
    store.close();
  }
});

it('native skill delivery fails the invocation before contacting the model when delivery is denied', async () => {
  const store = new Store(':memory:');
  const workspace = new WorkspaceStore(':memory:', 'owner');
  try {
    const dot = workspace.dots()[0];
    workspace.updateDot(dot.id, {
      ...dot,
      learningContainerId: 'research',
      skillDeliveryEnabled: true,
    });
    workspace.bindThread('thread', dot.id, 'Learning');
    const delivery = vi
      .spyOn(CopilotKitIntelligence.prototype, 'getLearnedSkillsSnapshots')
      .mockRejectedValue(new LearnedSkillsError('DELIVERY_DISABLED', false));
    const network = vi
      .spyOn(globalThis, 'fetch')
      .mockRejectedValue(new Error('Unexpected network request'));
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
    );
    const input: RunAgentInput = {
      threadId: 'thread',
      runId: 'run',
      messages: [],
      tools: [],
      context: [],
      state: {},
      forwardedProps: {},
    };
    await expect(
      lastValueFrom(agent.run(input).pipe(toArray())),
    ).rejects.toMatchObject({ code: 'DELIVERY_DISABLED' });
    expect(delivery).toHaveBeenCalledWith(
      expect.objectContaining({ containers: [{ containerId: 'research' }] }),
    );
    expect(network).not.toHaveBeenCalled();
  } finally {
    workspace.close();
    store.close();
  }
});
