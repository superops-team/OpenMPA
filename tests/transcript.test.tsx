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

it('renders tool-only assistant messages inline between chat turns without printing tool JSON', () => {
  const html = renderToStaticMarkup(
    <ChatTranscript
      messages={[
        { id: 'request', role: 'user', content: 'Open the website' },
        {
          id: 'tool-call',
          role: 'assistant',
          toolCalls: [
            {
              id: 'navigate',
              type: 'function',
              function: {
                name: 'computer_navigate',
                arguments: '{"url":"https://example.com"}',
              },
            },
          ],
        },
        { id: 'reply', role: 'assistant', content: 'Here is the summary' },
      ]}
      calls={[]}
      renderTools={(message) =>
        message.toolCalls?.length ? (
          <section>Inline computer view</section>
        ) : null
      }
    />,
  );
  expect(html.indexOf('Open the website')).toBeLessThan(
    html.indexOf('Inline computer view'),
  );
  expect(html.indexOf('Inline computer view')).toBeLessThan(
    html.indexOf('Here is the summary'),
  );
  expect(html).not.toContain('undefined');
  expect(html).not.toContain('computer_navigate');
});
