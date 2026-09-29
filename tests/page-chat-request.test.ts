import { expect, it, vi } from 'vitest';
import { PageChatRequests } from '../src/client/page-chat-requests';
function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<T>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
}
it.each(['page-b:dot-a', 'page-a:dot-b'])(
  'ignores an old page-chat response after selecting %s',
  async (nextScope) => {
    const requests = new PageChatRequests();
    requests.select('page-a:dot-a');
    const old = deferred<string>();
    const success = vi.fn(),
      failure = vi.fn(),
      settled = vi.fn();
    const pending = requests.run('page-a:dot-a', () => old.promise, {
      success,
      failure,
      settled,
    });
    requests.select(nextScope);
    old.resolve('thread-a');
    await pending;
    expect(success).not.toHaveBeenCalled();
    expect(failure).not.toHaveBeenCalled();
    expect(settled).not.toHaveBeenCalled();
  },
);
it('ignores stale errors and completion without clearing a newer request busy state', async () => {
  const requests = new PageChatRequests();
  requests.select('page-a:dot-a');
  const old = deferred<string>();
  const latest = deferred<string>();
  const success = vi.fn(),
    failure = vi.fn(),
    settled = vi.fn();
  const first = requests.run('page-a:dot-a', () => old.promise, {
    success,
    failure,
    settled,
  });
  requests.select('page-b:dot-a');
  const second = requests.run('page-b:dot-a', () => latest.promise, {
    success,
    failure,
    settled,
  });
  old.reject(new Error('Old request failed'));
  await first;
  expect(failure).not.toHaveBeenCalled();
  expect(settled).not.toHaveBeenCalled();
  latest.resolve('thread-b');
  await second;
  expect(success).toHaveBeenCalledWith('thread-b');
  expect(settled).toHaveBeenCalledTimes(1);
});
