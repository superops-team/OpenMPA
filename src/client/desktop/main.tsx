import React from 'react';
import { createRoot } from 'react-dom/client';
import { isMyGo } from 'mygo-runtime';
import { DesktopApp } from './DesktopApp';
import { createDesktopServices } from './services';
import { messages } from './generated/locales';
import '../style.css';
import '../editor.css';
import './style.css';
import '../styles/design-alignment.css';
import '../styles/desktop-design-alignment.css';
import './trace.css';
import './turn-process.css';
import './assistant-markdown.css';
import './conversation-fixture.css';
import { watchSystemTheme } from '../theme';
import { ConversationFixture } from './ConversationFixture';
watchSystemTheme();
const showConversationFixture =
  import.meta.env.DEV &&
  import.meta.env.VITE_OPENMPA_FIXTURE === 'conversation';
createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    {showConversationFixture ? (
      <ConversationFixture />
    ) : isMyGo() ? (
      <DesktopApp services={createDesktopServices()} />
    ) : (
      <main className="main-content">
        <div className="page-heading">
          <div>
            <h1>{messages.desktopOpenMPADesktop()}</h1>
            <p>
              {messages.desktopOpenThisFrontendThroughTheMyGoDesktopApplicationTo()}
            </p>
          </div>
        </div>
      </main>
    )}
  </React.StrictMode>,
);
