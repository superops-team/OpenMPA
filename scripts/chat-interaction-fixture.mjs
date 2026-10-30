export function installChatInteractionFixture() {
  const call = globalThis.mygo.call.bind(globalThis.mygo);
  const history = [];
  let settled = false;
  let index = 0;
  globalThis.mygo.channel = () => {
    const queue = [];
    let waiting;
    let closed = false;
    return {
      push(value) {
        if (waiting) {
          const resolve = waiting;
          waiting = undefined;
          resolve({ value, done: false });
        } else queue.push(value);
      },
      close() {
        closed = true;
        waiting?.({ done: true });
      },
      [Symbol.asyncIterator]() {
        return this;
      },
      next() {
        if (queue.length)
          return Promise.resolve({ value: queue.shift(), done: false });
        if (closed) return Promise.resolve({ done: true });
        return new Promise((resolve) => {
          waiting = resolve;
        });
      },
    };
  };
  globalThis.mygo.call = async (method, arg, channel) => {
    if (method === 'Session.Submit') {
      globalThis.__chatSubmitted = arg;
      return new Promise((resolve) => {
        globalThis.__acceptChatSubmission = () =>
          resolve({
            ok: true,
            value: { invocationId: 'chat-flow', status: 'accepted' },
          });
      });
    }
    if (method === 'Session.Subscribe') {
      const emit = (event) => {
        const value = {
          schemaVersion: 2,
          connectionEpoch: arg.connectionEpoch,
          subscriptionEpoch: arg.subscriptionEpoch,
          sessionId: arg.sessionId,
          invocationId: 'chat-flow',
          eventId: `chat-event-${++index}`,
          author: 'assistant',
          ...event,
        };
        history.push(value);
        channel.push(value);
      };
      globalThis.__chatEmit = emit;
      emit({
        kind: 'reasoning',
        partial: true,
        reasoningVisibility: 'detail',
        text: '正在分析：介绍下你自己',
      });
      return new Promise((resolve) => {
        globalThis.__finishChatSubscription = () =>
          resolve({ ok: true, value: {} });
      });
    }
    if (method === 'Session.Read' && settled)
      return {
        ok: true,
        value: {
          summary: {
            id: arg.sessionId,
            sessionId: arg.sessionId,
            title: 'Chat interaction check',
            status: 'idle',
          },
          events: history,
          completedInvocations: ['chat-flow'],
        },
      };
    return call(method, arg);
  };
  globalThis.__settleChat = () => {
    settled = true;
    globalThis.__chatEmit({
      kind: 'text',
      text: 'This is the completed answer.',
      partial: false,
      terminal: true,
    });
    globalThis.__finishChatSubscription();
  };
}
