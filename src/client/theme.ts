export type ThemePreference = 'system' | 'light' | 'dark';
const key = 'openmpa-theme';
// The choice for this page, even if storage refuses to keep it.
let chosen: ThemePreference | undefined;
const query = () => window.matchMedia('(prefers-color-scheme: dark)');
// Storage can be unavailable (private windows, blocked site data); the theme
// then follows the system for this visit.
export function themePreference(): ThemePreference {
  if (chosen) return chosen;
  try {
    const value = localStorage.getItem(key);
    if (value === 'light' || value === 'dark') return value;
  } catch {
    // Fall through to the system theme.
  }
  return 'system';
}
export function resolveTheme(preference: ThemePreference, systemDark: boolean) {
  return preference === 'system' ? (systemDark ? 'dark' : 'light') : preference;
}
function apply() {
  if (typeof document === 'undefined') return;
  const theme = resolveTheme(themePreference(), query().matches);
  const root = document.documentElement;
  root.dataset.themeChanging = '';
  root.dataset.theme = theme;
  const background =
    typeof getComputedStyle === 'function'
      ? getComputedStyle(root).getPropertyValue('--color-background').trim()
      : '';
  if (background)
    document
      .querySelector('meta[name="theme-color"]')
      ?.setAttribute('content', background);
  root.getBoundingClientRect?.();
  const nextFrame =
    typeof requestAnimationFrame === 'function'
      ? requestAnimationFrame
      : (callback: FrameRequestCallback) => {
          callback(0);
          return 0;
        };
  nextFrame(() => {
    delete root.dataset.themeChanging;
  });
}
export function setThemePreference(preference: ThemePreference) {
  chosen = preference;
  try {
    if (preference === 'system') localStorage.removeItem(key);
    else localStorage.setItem(key, preference);
  } catch {
    // Applies for this visit only.
  }
  apply();
}
export function watchSystemTheme() {
  apply();
  query().addEventListener('change', apply);
}
