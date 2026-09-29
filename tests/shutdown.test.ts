import { afterEach, expect, it, vi } from 'vitest';
import { createShutdown } from '../src/server/shutdown.js';
afterEach(() => vi.useRealTimers());
it('waits for Channels and HTTP shutdown and handles repeated signals once', async () => {
  let finish!: () => void;
  const wait = new Promise<void>((resolve) => {
    finish = resolve;
  });
  const stopRunner = vi.fn(),
    stopPlatform = vi.fn(() => wait),
    closeServer = vi.fn(async () => {}),
    exit = vi.fn(),
    report = vi.fn();
  const shutdown = createShutdown({
    stopRunner,
    stopPlatform,
    closeServer,
    exit,
    report,
  });
  const first = shutdown();
  const second = shutdown();
  expect(first).toBe(second);
  await Promise.resolve();
  expect(exit).not.toHaveBeenCalled();
  finish();
  await first;
  expect(stopRunner).toHaveBeenCalledTimes(1);
  expect(stopPlatform).toHaveBeenCalledTimes(1);
  expect(closeServer).toHaveBeenCalledTimes(1);
  expect(exit).toHaveBeenCalledWith(0);
});
it('bounds a stuck shutdown and reports operation failures without stopping other cleanup', async () => {
  vi.useFakeTimers();
  const exit = vi.fn(),
    report = vi.fn(),
    closeServer = vi.fn(async () => {});
  const shutdown = createShutdown({
    stopRunner: () => {
      throw new Error('secret');
    },
    stopPlatform: () => new Promise(() => {}),
    closeServer,
    exit,
    report,
    timeoutMs: 100,
  });
  const pending = shutdown();
  await vi.advanceTimersByTimeAsync(100);
  await pending;
  expect(closeServer).toHaveBeenCalledTimes(1);
  expect(exit).toHaveBeenCalledWith(1);
  expect(report).toHaveBeenCalledTimes(2);
});
