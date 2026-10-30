import { expect, it, vi } from 'vitest';
import { createStudioServices } from '../src/client/desktop/studio-services';
import { LocalizedError, renderMessage } from '../src/client/desktop/locale';
import { messagesIn } from '../src/client/desktop/generated/locales';
import type { Usage } from '../src/client/desktop/generated/mygo';

it('maps the concrete Usage transport without a second binding', async () => {
  const collaborationBinding = vi.fn(async () => ({
    ok: true,
    value: {
      space_id: 's',
      space_name: 'Space',
      region: 'cn-beijing',
      revision: 2,
    },
  }));
  const collaborationAgents = vi.fn(async () => ({
    ok: true,
    value: {
      items: [
        {
          Id: 'a',
          Name: 'Alice',
          Description: 'description',
          Status: 'available',
          Alias: 'alice',
          AliasRevision: 3,
          AliasAvailable: true,
          runtimeId: 'r-peer',
        },
      ],
      total: 1,
      page: 1,
      page_size: 20,
    },
  }));
  const saveMainProfileInstructions = vi.fn(async () => ({
    ok: false,
    status: 'unsupportedOrUnconfigured',
    error: { code: 'capabilityUnavailable', message: 'safe', httpStatus: 404 },
  }));
  const bridge = {
    collaborationBinding,
    collaborationAgents,
    saveMainProfileInstructions,
  } as unknown as typeof Usage;
  const services = createStudioServices(bridge);
  expect(await services.collaboration.binding(2)).toEqual({
    spaceId: 's',
    spaceName: 'Space',
    region: 'cn-beijing',
    revision: 2,
  });
  expect((await services.collaboration.agents(2, 's', 1)).items[0]).toEqual({
    id: 'a',
    name: 'Alice',
    description: 'description',
    status: 'available',
    alias: 'alice',
    aliasRevision: 3,
    aliasAvailable: true,
    runtimeId: 'r-peer',
    card: undefined,
  });
  const outcome = await services.resources.saveMainProfileInstructions(
    2,
    'one-use',
    'instructions',
  );
  expect(outcome).toEqual({
    status: 'unsupportedOrUnconfigured',
    error: { code: 'capabilityUnavailable', message: 'safe', httpStatus: 404 },
  });
  expect(saveMainProfileInstructions).toHaveBeenCalledWith({
    connectionEpoch: 2,
    snapshotId: 'one-use',
    instructions: 'instructions',
  });
});

it('preserves machine fields and localizes bridge failures, not provider text', async () => {
  const bridge = {
    readAgentsMD: async () => ({
      ok: false,
      error: {
        code: 'businessIdentityRequired',
        message: 'safe provider text',
        httpStatus: 401,
      },
    }),
  } as unknown as typeof Usage;
  const services = createStudioServices(bridge);
  let caught: unknown;
  try {
    await services.resources.readAgentsMD(2);
  } catch (error) {
    caught = error;
  }
  expect(caught).toBeInstanceOf(LocalizedError);
  expect(caught).toMatchObject({
    code: 'businessIdentityRequired',
    httpStatus: 401,
  });
  expect(
    renderMessage((caught as LocalizedError).text, messagesIn('zh-Hans')),
  ).toBe(messagesIn('zh-Hans').businessIdentityRequired());
});
it('forwards the complete Main profile and keeps creation separate from snapshot updates', async () => {
  const failure = {
    ok: false,
    status: 'unavailable',
    error: { code: 'conflict', message: 'safe' },
  };
  const saveMainProfile = vi.fn(async () => failure);
  const createMainProfile = vi.fn(async () => failure);
  const bridge = {
    saveMainProfile,
    createMainProfile,
  } as unknown as typeof Usage;
  const api = createStudioServices(bridge).resources;
  const value = {
    agentsMd: 'Main only',
    model: { id: 'model-id' },
    skills: [],
    mcpSources: [{ resourceId: 'agentkit:lookup', enabledTools: [] }],
    mcpServers: {
      private: {
        url: 'https://example.test/mcp',
        headers: {},
        enabledTools: null,
      },
    },
  };
  expect(
    (await api.saveMainProfile!(3, 'snapshot-authority', value)).error?.code,
  ).toBe('conflict');
  expect(saveMainProfile).toHaveBeenCalledExactlyOnceWith({
    connectionEpoch: 3,
    snapshotId: 'snapshot-authority',
    profile: value,
  });
  expect(createMainProfile).not.toHaveBeenCalled();
  await api.createMainProfile!(3, value);
  expect(createMainProfile).toHaveBeenCalledExactlyOnceWith({
    connectionEpoch: 3,
    profile: value,
  });
});
