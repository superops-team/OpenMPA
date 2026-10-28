import { z } from 'zod';

const hex = z.string().regex(/^#[0-9a-fA-F]{6}$/);
const pixels = z.string().regex(/^\d+(?:\.\d+)?px$/);
const duration = z.string().regex(/^\d+ms$/);
const colorRoles = z.enum([
  'background',
  'background-alt',
  'header',
  'panel',
  'sidebar',
  'surface',
  'surface-hover',
  'surface-pressed',
  'card',
  'card-selected',
  'card-border',
  'popover',
  'popover-border',
  'menu',
  'menu-hover',
  'input',
  'input-focused',
  'input-border',
  'input-border-hover',
  'input-border-focused',
  'foreground',
  'foreground-subtle',
  'foreground-subtlest',
  'foreground-inverse',
  'border',
  'border-hover',
  'brand',
  'primary',
  'primary-hover',
  'primary-pressed',
  'primary-foreground',
  'accent',
  'hover',
  'selected',
  'link',
  'focus',
  'success',
  'success-foreground',
  'success-surface',
  'success-surface-foreground',
  'warning',
  'warning-foreground',
  'warning-surface',
  'warning-surface-foreground',
  'destructive',
  'destructive-foreground',
  'destructive-surface',
  'destructive-surface-foreground',
  'progress',
  'progress-foreground',
  'progress-surface',
  'progress-surface-foreground',
  'code',
  'selection',
  'disabled',
  'disabled-foreground',
  'tooltip',
  'tooltip-foreground',
  'scrollbar',
  'scrim',
  'media-canvas',
]);
const palette = z.record(colorRoles, hex);
const pxRoles = <T extends string>(names: readonly T[]) =>
  z.record(z.enum(names), pixels);

export const designTokenSchema = z
  .object({
    $schema: z.literal('./tokens.schema.json'),
    name: z.string().min(1),
    version: z.string().regex(/^\d+\.\d+\.\d+$/),
    source: z
      .object({ design: z.string(), reference: z.url(), license: z.string() })
      .strict(),
    theme: z.object({ light: palette, dark: palette }).strict(),
    typography: z
      .object({
        'ui-font-size': pixels,
        'ui-xl': pixels,
        'ui-lg': pixels,
        'ui-base': pixels,
        'ui-caption': pixels,
        'ui-sm': pixels,
        'ui-xs': pixels,
        'mobile-input-safe': pixels,
        'line-heading': z.string(),
        'line-body': z.string(),
        'line-compact': z.string(),
      })
      .strict(),
    font: z.object({ ui: z.string().min(1), mono: z.string().min(1) }).strict(),
    space: pxRoles(['1', '2', '3', '4', '5', '6']),
    radius: pxRoles(['none', 'sm', 'md', 'lg', 'xl', '2xl', 'full']),
    control: pxRoles(['xs', 'sm', 'md', 'lg', 'input']),
    icon: pxRoles(['sm', 'md', 'lg', 'brand']),
    border: pxRoles(['default', 'focus']),
    layer: z.record(
      z.enum(['base', 'sticky', 'popover', 'overlay', 'toast']),
      z.number().int().nonnegative(),
    ),
    weight: z.record(
      z.enum(['regular', 'medium', 'semibold', 'bold']),
      z.number().int().min(100).max(900),
    ),
    motion: z
      .object({
        'press-duration': duration,
        'state-duration': duration,
        'overlay-duration': duration,
        'spinner-duration': duration,
        'ease-out': z.string().regex(/^cubic-bezier\([\d., ]+\)$/),
      })
      .strict(),
    elevation: z
      .object({ overlay: z.string(), attention: z.string() })
      .strict(),
    layout: pxRoles([
      'rail-width',
      'sidebar-width',
      'titlebar-height',
      'reading-column',
      'fixture-column',
      'settings-column',
      'assistant-skill-list-height',
    ]),
  })
  .strict();

export type DesignTokens = z.infer<typeof designTokenSchema>;

export function contrastRatio(a: string, b: string) {
  const luminance = (hex: string) => {
    const channels = [1, 3, 5].map((offset) => {
      const n = parseInt(hex.slice(offset, offset + 2), 16) / 255;
      return n <= 0.04045 ? n / 12.92 : ((n + 0.055) / 1.055) ** 2.4;
    });
    return channels[0] * 0.2126 + channels[1] * 0.7152 + channels[2] * 0.0722;
  };
  const x = luminance(a);
  const y = luminance(b);
  return (Math.max(x, y) + 0.05) / (Math.min(x, y) + 0.05);
}

export function parseDesignTokens(input: unknown): DesignTokens {
  const tokens = designTokenSchema.parse(input);
  const textSurfaces = [
    'background',
    'panel',
    'sidebar',
    'surface',
    'card',
    'input',
    'popover',
    'card-selected',
    'code',
  ] as const;
  for (const [theme, colors] of Object.entries(tokens.theme)) {
    const pairs: [string, string, number][] = [];
    for (const text of [
      'foreground',
      'foreground-subtle',
      'foreground-subtlest',
    ] as const)
      for (const surface of textSurfaces) pairs.push([text, surface, 4.5]);
    for (const status of ['success', 'warning', 'destructive', 'progress']) {
      pairs.push([`${status}-foreground`, status, 4.5]);
      pairs.push([`${status}-surface-foreground`, `${status}-surface`, 4.5]);
    }
    for (const surface of ['primary', 'primary-hover', 'primary-pressed'])
      pairs.push(['primary-foreground', surface, 4.5]);
    pairs.push(['tooltip-foreground', 'tooltip', 4.5]);
    for (const surface of textSurfaces) pairs.push(['focus', surface, 3]);
    for (const [fg, bg, minimum] of pairs) {
      const ratio = contrastRatio(
        colors[fg as keyof typeof colors],
        colors[bg as keyof typeof colors],
      );
      if (ratio < minimum)
        throw new Error(
          `${theme}: ${fg} on ${bg} has contrast ${ratio.toFixed(2)}, requires ${minimum}`,
        );
    }
  }
  for (const role of ['line-heading', 'line-body', 'line-compact'] as const) {
    const value = Number(tokens.typography[role]);
    if (!Number.isFinite(value) || value < 1 || value > 2)
      throw new Error(`Invalid typography ${role}`);
  }
  return tokens;
}
