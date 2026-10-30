import type { MainProfileVM } from './studio-models';

export const maskedHeader = '**********';
export const profileName = /^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$/;
export const toolNames = (text: string) =>
  text
    .split(/[\n,]/)
    .map((s) => s.trim())
    .filter(Boolean);
export function mainProfileIssue(
  profile: MainProfileVM,
): 'fields' | 'credentials' | undefined {
  if (
    profile.model &&
    (!profile.model.id.trim() || profile.model.id.length > 512)
  )
    return 'fields';
  const names = new Set<string>();
  let length = [...profile.agentsMd].length;
  if (profile.skills.length > 32) return 'fields';
  for (const skill of profile.skills) {
    const name = skill.name.toLowerCase();
    if (
      !profileName.test(skill.name) ||
      names.has(name) ||
      !skill.version.trim() ||
      skill.version.length > 512 ||
      skill.version.toLowerCase() === 'latest' ||
      /[*^~<>=\s]/.test(skill.version) ||
      !skill.content
    )
      return 'fields';
    names.add(name);
    length += [...skill.content].length;
  }
  if (length > 40000) return 'fields';
  const ids = new Set<string>();
  const validTools = (tools: string[] | null) =>
    tools === null ||
    (tools.length <= 128 &&
      new Set(tools).size === tools.length &&
      tools.every((t) => t.trim() === t && t.length > 0 && t.length <= 512));
  if (
    (profile.mcpSources?.length ?? 0) > 32 ||
    Object.keys(profile.mcpServers ?? {}).length > 32
  )
    return 'fields';
  for (const source of profile.mcpSources ?? []) {
    if (
      !source.resourceId ||
      source.resourceId.trim() !== source.resourceId ||
      source.resourceId.length > 512 ||
      source.resourceId.startsWith('profile:') ||
      ids.has(source.resourceId) ||
      !validTools(source.enabledTools)
    )
      return 'fields';
    ids.add(source.resourceId);
  }
  for (const [name, server] of Object.entries(profile.mcpServers ?? {})) {
    if (
      !profileName.test(name) ||
      !validTools(server.enabledTools) ||
      server.url.length > 2048 ||
      /\s/.test(server.url)
    )
      return 'fields';
    try {
      const url = new URL(server.url);
      if (
        url.protocol !== 'https:' ||
        !url.hostname ||
        url.username ||
        url.password ||
        server.url.includes('?') ||
        server.url.includes('#')
      )
        return 'fields';
    } catch {
      return 'fields';
    }
    const headers = Object.entries(server.headers);
    const keys = new Set<string>();
    if (headers.length > 32) return 'fields';
    for (const [key, value] of headers) {
      if (
        !/^[!#$%&'*+.^_`|~0-9A-Za-z-]+$/.test(key) ||
        keys.has(key.toLowerCase()) ||
        ['host', 'x-ve-tip-token', 'x-app-resourceid'].includes(
          key.toLowerCase(),
        ) ||
        value.length > 8192 ||
        /[\r\n]/.test(value)
      )
        return 'fields';
      keys.add(key.toLowerCase());
      if (value === maskedHeader || value.startsWith('fernet:v1:'))
        return 'credentials';
    }
  }
}

export function mergeMainProfile(
  base: MainProfileVM,
  draft: MainProfileVM,
  incoming: MainProfileVM,
): MainProfileVM {
  const result = { ...incoming };
  for (const key of [
    'agentsMd',
    'model',
    'skills',
    'mcpSources',
    'mcpServers',
  ] as const) {
    if (JSON.stringify(base[key]) !== JSON.stringify(draft[key]))
      Object.assign(result, { [key]: draft[key] });
  }
  return result;
}
