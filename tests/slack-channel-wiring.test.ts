import { expect, it, vi } from 'vitest';
import { createChannel } from '@copilotkit/channels';
import { HttpAgent } from '@ag-ui/client';
import { createSlackChannel } from '../src/server/slack-channel.js';
vi.mock('@copilotkit/channels', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@copilotkit/channels')>();
  return {
    ...actual,
    createChannel: vi.fn(
      (options: Parameters<typeof actual.createChannel>[0]) => {
        const channel = actual.createChannel(options);
        vi.spyOn(channel, 'onMention');
        vi.spyOn(channel, 'onMessage');
        return channel;
      },
    ),
  };
});
it('registers distinct mention and subscribed-message handlers on a serial SDK channel', () => {
  const channel = createSlackChannel({
    name: 'test-channel',
    agent: () => new HttpAgent({ url: 'http://unused.invalid' }),
    config: { slackTeam: 'team', slackUsers: ['person'] },
    ownerId: 'owner',
    paused: () => false,
  });
  expect(channel.name).toBe('test-channel');
  expect(createChannel).toHaveBeenCalledWith(
    expect.objectContaining({
      store: { concurrency: 'serial' },
      identifyUser: expect.any(Function),
    }),
  );
  expect(channel.onMention).toHaveBeenCalledExactlyOnceWith(
    expect.any(Function),
  );
  expect(channel.onMessage).toHaveBeenCalledExactlyOnceWith(
    expect.any(Function),
  );
  expect(vi.mocked(channel.onMention).mock.calls[0][0]).not.toBe(
    vi.mocked(channel.onMessage).mock.calls[0][0],
  );
});
