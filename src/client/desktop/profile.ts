import { localized, LocalizedError } from './locale';
export function parseProfile(text: string): {
  agentsMd: string | null;
  model: string | null;
  skills: string[] | null;
  mcp: string[] | null;
} {
  let input: unknown;
  try {
    input = JSON.parse(text);
  } catch {
    throw new LocalizedError(localized((m) => m.profileInvalidJson()));
  }
  if (!input || typeof input !== 'object' || Array.isArray(input))
    throw new LocalizedError(
      localized((m) => m.desktopProfileMustBeAJSONObject()),
    );
  const record = input as Record<string, unknown>;
  const keys = ['agentsMd', 'model', 'skills', 'mcp'];
  if (
    Object.keys(record).some((key) => !keys.includes(key)) ||
    keys.some((key) => !(key in record))
  )
    throw new LocalizedError(localized((m) => m.profileFieldsRequired()));
  for (const key of ['agentsMd', 'model'])
    if (record[key] !== null && typeof record[key] !== 'string')
      throw new LocalizedError(localized((m) => m.profileStringRequired(key)));
  for (const key of ['skills', 'mcp'])
    if (
      record[key] !== null &&
      (!Array.isArray(record[key]) ||
        !record[key].every((value: unknown) => typeof value === 'string'))
    )
      throw new LocalizedError(localized((m) => m.profileListRequired(key)));
  if (
    typeof record.agentsMd === 'string' &&
    Array.from(record.agentsMd).length > 32000
  )
    throw new LocalizedError(localized((m) => m.profileInstructionsTooLong()));
  if (
    typeof record.model === 'string' &&
    (Array.from(record.model).length > 256 || /[/:]/.test(record.model))
  )
    throw new LocalizedError(
      localized((m) => m.desktopModelIDsMustBeAtMost256CharactersAnd()),
    );
  for (const key of ['skills', 'mcp'])
    if (Array.isArray(record[key]) && record[key].length > 200)
      throw new LocalizedError(localized((m) => m.profileListTooLong(key)));
  return record as {
    agentsMd: string | null;
    model: string | null;
    skills: string[] | null;
    mcp: string[] | null;
  };
}
