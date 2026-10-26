import React from 'react';
import { createRoot } from 'react-dom/client';
import { isMyGo } from 'mygo-runtime';
import { DesktopApp } from './DesktopApp';
import { createDesktopServices } from './services';
import '../style.css';
import '../editor.css';
import './style.css';
import { watchSystemTheme } from '../theme';
watchSystemTheme();
createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    {isMyGo() ? (
      <DesktopApp services={createDesktopServices()} />
    ) : (
      <main className="main-content">
        <div className="page-heading">
          <div>
            <h1>OpenMPA Desktop</h1>
            <p>
              Open this frontend through the MyGo desktop application to connect
              real platform and workspace services.
            </p>
          </div>
        </div>
      </main>
    )}
  </React.StrictMode>,
);
