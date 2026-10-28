import postcss from 'postcss';

export function checkTokenUsage(css: string, declaredTokens: Set<string>) {
  const failures: string[] = [];
  const ast = postcss.parse(css);
  ast.walkDecls((declaration) => {
    const { prop, value } = declaration;
    const where = `line ${declaration.source?.start?.line ?? 0}: ${prop}`;
    if (
      /#(?:[\da-f]{3,8})\b|\b(?:rgba?|hsla?|oklch|oklab)\(|\b(?:white|black|red|blue|green|gray|grey)\b/i.test(
        value,
      )
    )
      failures.push(`${where} uses a literal color`);
    if (
      /^(?:font-size|line-height|border-radius|box-shadow)$/.test(prop) &&
      !/^(?:none|normal|inherit|initial|unset|0)$/.test(value) &&
      !value.includes('var(')
    )
      failures.push(`${where} does not use a semantic token`);
    for (const match of value.matchAll(
      /var\(\s*(--(?:color|text|font|line|radius|space|control|icon|border|layer|weight|motion|shadow|layout)-[\w-]+)/g,
    ))
      if (!declaredTokens.has(match[1]))
        failures.push(`${where} references unknown token ${match[1]}`);
  });
  return failures;
}

export function declaredDesignTokens(css: string) {
  const names = new Set<string>();
  postcss.parse(css).walkDecls((declaration) => {
    if (declaration.prop.startsWith('--')) names.add(declaration.prop);
  });
  return names;
}
