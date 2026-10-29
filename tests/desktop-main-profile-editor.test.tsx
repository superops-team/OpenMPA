import { act, create, type ReactTestRenderer } from 'react-test-renderer';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { MainProfileEditor } from '../src/client/desktop/MainProfileEditor';
import { AgentsMDEditor } from '../src/client/desktop/AgentsMDEditor';
import { messagesIn } from '../src/client/desktop/generated/locales';
import type {
  ResourcesServices,
  MainProfileReadVM,
} from '../src/client/desktop/studio-models';
import {
  mainProfileIssue,
  mergeMainProfile,
} from '../src/client/desktop/main-profile-form';
const m = messagesIn('en');
const connection = {
  epoch: 4,
  scope: 'scope',
  runtimeId: 'r-current',
  status: 'ready',
};
let renderer: ReactTestRenderer;
const state: MainProfileReadVM = {
  status: 'configured',
  snapshotId: 'snapshot',
  connectionEpoch: 4,
  record: {
    mpaInstanceId: 'agent-current',
    revision: 7,
    digest: 'digest',
    etag: '"7"',
    profile: {
      agentsMd: '# Main instructions',
      model: { id: 'main-model' },
      skills: [
        { name: 'review', version: '1.2.0', content: 'Review the changes.' },
      ],
      mcpSources: [{ resourceId: 'agentkit:search', enabledTools: null }],
      mcpServers: {
        private: {
          url: 'https://example.test/mcp',
          headers: { Authorization: '**********' },
          enabledTools: [],
        },
      },
    },
    sourceProfileId: null,
    sourceProfileDigest: null,
    updatedBy: 'owner',
    createdAt: '',
  },
};
beforeEach(() => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  vi.stubGlobal('mygo', { locale: 'en' });
});
afterEach(async () => {
  if (renderer) await act(async () => renderer.unmount());
  vi.unstubAllGlobals();
});
function button(label: string) {
  return renderer.root
    .findAllByType('button')
    .find(
      (b) => b.children.includes(label) || b.props['aria-label'] === label,
    )!;
}
function field(label: string, type: 'input' | 'select' | 'textarea' = 'input') {
  return renderer.root
    .findAllByType('label')
    .find((l) => l.children[0] === label)!
    .findByType(type);
}
function services() {
  return {
    readMainProfile: vi.fn(async () => ({
      status: 'configured',
      value: structuredClone(state),
    })),
    saveMainProfile: vi.fn(async (_epoch, _snapshot, profile) => ({
      status: 'configured',
      value: {
        ...structuredClone(state),
        snapshotId: 'new',
        record: {
          ...state.record!,
          revision: 8,
          profile: {
            ...profile,
            mcpServers: {
              private: {
                ...profile.mcpServers.private,
                headers: { Authorization: '**********' },
              },
            },
          },
        },
      },
    })),
  } as unknown as ResourcesServices;
}
it('automatically reads the complete profile and saves model, skills, MCP selections and re-entered credentials together', async () => {
  const api = services();
  await act(async () => {
    renderer = create(
      <MainProfileEditor
        services={api}
        connection={connection}
        hidden={false}
      />,
    );
  });
  expect(api.readMainProfile).toHaveBeenCalledExactlyOnceWith(4);
  expect(renderer.root.findAllByType('textarea')).toHaveLength(0);
  expect(JSON.stringify(renderer.toJSON())).toContain('main-model');
  await act(async () => button(m.profileEdit()).props.onClick());
  expect(button(m.saveMainProfile()).props.disabled).toBe(true);
  await act(async () =>
    field(m.mainModelId()).props.onChange({ target: { value: 'new-model' } }),
  );
  expect(button(m.saveMainProfile()).props.disabled).toBe(true);
  expect(JSON.stringify(renderer.toJSON())).toContain(
    m.mainCredentialsRequired(),
  );
  expect(field(m.mainHeaderValue()).props.value).toBe('');
  await act(async () =>
    field(m.mainHeaderValue()).props.onChange({
      target: { value: 'Bearer test-only' },
    }),
  );
  expect(button(m.saveMainProfile()).props.disabled).toBe(false);
  await act(async () => button(m.saveMainProfile()).props.onClick());
  expect(api.saveMainProfile).toHaveBeenCalledWith(
    4,
    'snapshot',
    expect.objectContaining({
      model: { id: 'new-model' },
      skills: state.record!.profile.skills,
      mcpSources: [{ resourceId: 'agentkit:search', enabledTools: null }],
      mcpServers: {
        private: {
          url: 'https://example.test/mcp',
          headers: { Authorization: 'Bearer test-only' },
          enabledTools: [],
        },
      },
    }),
  );
  expect(renderer.root.findAllByType('textarea')).toHaveLength(0);
  expect(JSON.stringify(renderer.toJSON())).not.toContain('Bearer test-only');
});
it('cancels local profile edits without writing or retaining newly entered secrets', async () => {
  const api = services();
  await act(async () => {
    renderer = create(
      <MainProfileEditor
        services={api}
        connection={connection}
        hidden={false}
      />,
    );
  });
  await act(async () => button(m.profileEdit()).props.onClick());
  await act(async () =>
    field(m.mainHeaderValue()).props.onChange({
      target: { value: 'temporary-test-value' },
    }),
  );
  await act(async () => button(m.cancel()).props.onClick());
  expect(api.saveMainProfile).not.toHaveBeenCalled();
  await act(async () => button(m.profileEdit()).props.onClick());
  expect(field(m.mainHeaderValue()).props.value).toBe('');
  expect(JSON.stringify(renderer.toJSON())).not.toContain(
    'temporary-test-value',
  );
});
it('renders AGENTS.md immediately and returns to the saved document after cancelling edits', async () => {
  const api = {
    readAgentsMD: vi.fn(async () => ({
      content: '# Team instructions\n\nFollow the project rules.',
      revision: 3,
    })),
    saveAgentsMD: vi.fn(),
  } as unknown as ResourcesServices;
  await act(async () => {
    renderer = create(
      <AgentsMDEditor services={api} connection={connection} hidden={false} />,
    );
  });
  expect(api.readAgentsMD).toHaveBeenCalledOnce();
  expect(renderer.root.findAllByType('textarea')).toHaveLength(0);
  expect(
    renderer.root
      .findAllByType('h1')
      .some((h) => h.children.includes('Team instructions')),
  ).toBe(true);
  await act(async () => button(m.profileEdit()).props.onClick());
  await act(async () =>
    renderer.root
      .findByType('textarea')
      .props.onChange({ target: { value: 'local edit' } }),
  );
  await act(async () => button(m.cancel()).props.onClick());
  expect(api.saveAgentsMD).not.toHaveBeenCalled();
  expect(JSON.stringify(renderer.toJSON())).not.toContain('local edit');
  expect(renderer.root.findAllByType('textarea')).toHaveLength(0);
});
it('validates pinned skills and MCP boundaries and merges untouched settings from a fresh server revision', () => {
  const original = structuredClone(state.record!.profile);
  expect(mainProfileIssue(original)).toBe('credentials');
  original.mcpServers!.private.headers = {};
  expect(mainProfileIssue(original)).toBeUndefined();
  expect(
    mainProfileIssue({
      ...original,
      skills: [{ ...original.skills[0], version: 'latest' }],
    }),
  ).toBe('fields');
  expect(
    mainProfileIssue({
      ...original,
      mcpServers: {
        bad: {
          url: 'http://example.test/mcp',
          headers: {},
          enabledTools: null,
        },
      },
    }),
  ).toBe('fields');
  expect(
    mainProfileIssue({
      ...original,
      mcpSources: [{ resourceId: 'profile:private', enabledTools: null }],
    }),
  ).toBe('fields');
  const merged = mergeMainProfile(
    original,
    { ...original, agentsMd: 'my edit' },
    { ...original, model: { id: 'concurrent-model' } },
  );
  expect(merged.agentsMd).toBe('my edit');
  expect(merged.model?.id).toBe('concurrent-model');
  expect(merged.mcpServers!.private.enabledTools).toEqual([]);
});
it('never creates on a missing read and requires an explicit create and save', async () => {
  const createMainProfile = vi.fn(async () => ({
    status: 'configured',
    value: structuredClone(state),
  }));
  const api = {
    ...services(),
    readMainProfile: vi.fn(async () => ({
      status: 'unsupportedOrUnconfigured',
      error: { code: 'requestFailed', httpStatus: 404, message: 'not found' },
    })),
    createMainProfile,
  } as unknown as ResourcesServices;
  await act(async () => {
    renderer = create(
      <MainProfileEditor
        services={api}
        connection={connection}
        hidden={false}
      />,
    );
  });
  expect(createMainProfile).not.toHaveBeenCalled();
  await act(async () => button(m.mainCreateProfile()).props.onClick());
  expect(createMainProfile).not.toHaveBeenCalled();
  await act(async () =>
    renderer.root
      .findByProps({ 'data-testid': 'resources-mainProfile-draft' })
      .props.onChange({ target: { value: 'new Main instructions' } }),
  );
  await act(async () => button(m.saveMainProfile()).props.onClick());
  expect(createMainProfile).toHaveBeenCalledWith(4, {
    agentsMd: 'new Main instructions',
    model: null,
    skills: [],
  });
  expect(api.saveMainProfile).not.toHaveBeenCalled();
});
