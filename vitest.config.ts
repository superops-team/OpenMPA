import { configDefaults, defineConfig } from 'vitest/config';

export default defineConfig({
  test: {
    // Isolated Runtime/Worker worktrees have their own test runners and dependencies.
    exclude: [...configDefaults.exclude, '.openmpa/**'],
  },
});
