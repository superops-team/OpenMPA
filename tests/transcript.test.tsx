import { expect, it } from 'vitest';
import { renderToStaticMarkup } from 'react-dom/server';
import { ChatTranscript } from '../src/client/ChatTranscript';
it('keeps call receipts between the anchored message and later conversation turns', () => {
  const html = renderToStaticMarkup(
    <ChatTranscript
      messages={[
        { id: 'before', role: 'user', content: 'Before the call' },
        { id: 'after', role: 'assistant', content: 'Later message' },
      ]}
      calls={[
        {
          id: 'call',
          threadId: 'thread',
          status: 'ended',
          startedAt: 1000,
          endedAt: 6000,
          transcript: '',
          error: null,
          anchorMessageId: 'before',
        },
      ]}
    />,
  );
  expect(html.indexOf('Before the call')).toBeLessThan(
    html.indexOf('Call ended'),
  );
  expect(html.indexOf('Call ended')).toBeLessThan(
    html.indexOf('Later message'),
  );
});
