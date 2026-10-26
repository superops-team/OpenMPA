export function parseProfile(text: string): {
  agentsMd: string | null;
  model: string | null;
  skills: string[] | null;
  mcp: string[] | null;
} {
  const input: unknown = JSON.parse(text);
  if (!input || typeof input !== 'object' || Array.isArray(input))
    throw new Error('Profile must be a JSON object.');
  const record = input as Record<string, unknown>;
  const keys = ['agentsMd', 'model', 'skills', 'mcp'];
  if (
    Object.keys(record).some((key) => !keys.includes(key)) ||
    keys.some((key) => !(key in record))
  )
    throw new Error(
      'A complete profile requires agentsMd, model, skills and mcp only.',
    );
  for (const key of ['agentsMd', 'model'])
    if (record[key] !== null && typeof record[key] !== 'string')
      throw new Error(`${key} must be a string or null.`);
  for (const key of ['skills', 'mcp'])
    if (
      record[key] !== null &&
      (!Array.isArray(record[key]) ||
        !record[key].every((value: unknown) => typeof value === 'string'))
    )
      throw new Error(`${key} must be a list of strings or null.`);
  if (
    typeof record.agentsMd === 'string' &&
    Array.from(record.agentsMd).length > 32000
  )
    throw new Error('agentsMd exceeds 32,000 Unicode characters.');
  if (
    typeof record.model === 'string' &&
    (Array.from(record.model).length > 256 || /[/:]/.test(record.model))
  )
    throw new Error(
      'Model IDs must be at most 256 characters and cannot contain / or :.',
    );
  for (const key of ['skills', 'mcp'])
    if (Array.isArray(record[key]) && record[key].length > 200)
      throw new Error(`${key} exceeds 200 entries.`);
  return record as {
    agentsMd: string | null;
    model: string | null;
    skills: string[] | null;
    mcp: string[] | null;
  };
}
