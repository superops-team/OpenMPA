import { expect, it } from 'vitest';
import { parseProfile } from '../src/client/desktop/profile';
it('keeps null inheritance distinct from all-resource lists and rejects unknown profile keys', () => {
  expect(
    parseProfile('{"agentsMd":null,"model":null,"skills":[],"mcp":null}'),
  ).toEqual({ agentsMd: null, model: null, skills: [], mcp: null });
  expect(() =>
    parseProfile('{"agentsMd":null,"model":null,"skills":[4],"mcp":null}'),
  ).toThrow();
  expect(() =>
    parseProfile(
      '{"agentsMd":null,"model":null,"skills":null,"mcp":null,"token":"secret"}',
    ),
  ).toThrow();
});
it('validates profile limits by Unicode characters and rejects unsupported model IDs', () => {
  const profile = {
    agentsMd: '😀'.repeat(32000),
    model: null,
    skills: null,
    mcp: [],
  };
  expect(parseProfile(JSON.stringify(profile)).agentsMd).toBe(profile.agentsMd);
  expect(() =>
    parseProfile(
      JSON.stringify({ ...profile, agentsMd: profile.agentsMd + 'x' }),
    ),
  ).toThrow();
  expect(() =>
    parseProfile(JSON.stringify({ ...profile, model: 'provider/model' })),
  ).toThrow();
  expect(() =>
    parseProfile(
      JSON.stringify({ ...profile, skills: Array(201).fill('skill') }),
    ),
  ).toThrow();
});
