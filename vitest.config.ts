import { configDefaults, defineConfig } from 'vitest/config';

export default defineConfig({
  test: {
    // Isolated worktrees and the Python engine’s bundled Studio sources have
    // their own test runners and dependencies.
    exclude: [...configDefaults.exclude, '.openmpa/**', '**/.venv/**'],
  },
});
