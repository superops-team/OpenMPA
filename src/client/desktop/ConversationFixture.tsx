import { useState, type ComponentType } from 'react';
import {
  BookOpen,
  CheckCircle2,
  FileText,
  MessageSquare,
  Moon,
  Search,
  Settings,
  Sun,
} from 'lucide-react';
import { setThemePreference, type ThemePreference } from '../theme';
import { AssistantMarkdown } from './AssistantMarkdown';
import { TurnProcess } from './TurnProcess';
import type { ConversationTurn } from './turn-model';
import { Button, ComposerFooter, ComposerShell, StatusRow } from '../ui';

type FixtureView = 'conversation' | 'settings' | 'knowledge';

const fixtureViews = [
  { id: 'conversation', label: 'Conversation', icon: MessageSquare },
  { id: 'settings', label: 'Settings', icon: Settings },
  { id: 'knowledge', label: 'Knowledge', icon: BookOpen },
] satisfies ReadonlyArray<{
  id: FixtureView;
  label: string;
  icon: ComponentType<{ size?: number; 'aria-hidden'?: boolean }>;
}>;

const runningTurn: ConversationTurn = {
  invocationId: 'fixture-running',
  status: 'running',
  segments: [
    {
      type: 'reasoning',
      id: 'reasoning',
      visibility: 'hidden',
      content: '',
      partial: true,
    },
    {
      type: 'text',
      id: 'narration',
      role: 'assistant',
      content: 'Reading the conversation renderer and its event projection.',
      partial: false,
    },
    {
      type: 'tool',
      id: 'read-tool',
      toolId: 'tool-read-chat',
      name: 'read',
      status: 'success',
      subject: 'src/client/desktop/Chat.tsx',
    },
    {
      type: 'tool',
      id: 'test-tool',
      toolId: 'tool-run-tests',
      name: 'shell',
      status: 'running',
      subject: 'Run focused desktop tests',
    },
    {
      type: 'text',
      id: 'streaming-answer',
      role: 'assistant',
      content: 'The final answer is streaming below the process.',
      partial: true,
    },
  ],
};

const completedTurn: ConversationTurn = {
  invocationId: 'fixture-completed',
  status: 'completed',
  segments: [
    {
      type: 'tool',
      id: 'completed-read',
      toolId: 'tool-completed-read',
      name: 'read',
      status: 'success',
      subject: 'desktop/internal/session/events.go',
    },
    {
      type: 'status',
      id: 'completed-status',
      status: 'completed',
      content: '',
    },
  ],
};

const answer = [
  '## Conversation rendering review',
  '',
  'The current implementation now keeps the answer as the primary reading surface.',
  '',
  '| Area | Result |',
  '| --- | --- |',
  '| Event order | Preserved |',
  '| Tool state | Correlated by ID |',
  '| Hidden reasoning | Not exposed |',
  '',
  '```ts',
  "const status = tool.done ? 'Completed' : 'Running'",
  '```',
].join('\n');

export function ConversationFixture() {
  const [view, setView] = useState<FixtureView>('conversation');
  const [theme, setTheme] = useState<Exclude<ThemePreference, 'system'>>(() =>
    typeof document !== 'undefined' &&
    document.documentElement.dataset.theme === 'dark'
      ? 'dark'
      : 'light',
  );
  const [inspected, setInspected] = useState('');

  const updateTheme = (next: Exclude<ThemePreference, 'system'>) => {
    setTheme(next);
    setThemePreference(next);
  };

  return (
    <div className="app template-app desktop-app project-fixture">
      <header className="project-fixture__toolbar">
        <div className="project-fixture__brand">
          <strong>OpenMPA</strong>
          <span>Design system preview</span>
        </div>
        <nav aria-label="Preview page" className="project-fixture__nav">
          {fixtureViews.map((option) => {
            const Icon = option.icon;
            return (
              <Button
                key={option.id}
                variant={view === option.id ? 'secondary' : 'ghost'}
                size="sm"
                aria-current={view === option.id ? 'page' : undefined}
                onClick={() => setView(option.id)}
                leading={<Icon size={14} aria-hidden />}
              >
                {option.label}
              </Button>
            );
          })}
        </nav>
        <div
          className="project-fixture__themes"
          role="group"
          aria-label="Preview theme"
        >
          <Button
            variant={theme === 'light' ? 'secondary' : 'ghost'}
            size="icon-sm"
            aria-label="Use light theme"
            aria-pressed={theme === 'light'}
            onClick={() => updateTheme('light')}
          >
            <Sun aria-hidden />
          </Button>
          <Button
            variant={theme === 'dark' ? 'secondary' : 'ghost'}
            size="icon-sm"
            aria-label="Use dark theme"
            aria-pressed={theme === 'dark'}
            onClick={() => updateTheme('dark')}
          >
            <Moon aria-hidden />
          </Button>
        </div>
      </header>
      <div className="workspace project-fixture__workspace">
        {view === 'conversation' ? (
          <ConversationPreview onInspectTool={setInspected} />
        ) : view === 'settings' ? (
          <SettingsPreview />
        ) : (
          <KnowledgePreview />
        )}
      </div>
      <span className="sr-only" role="status" aria-live="polite">
        {inspected ? `Trace focus requested for ${inspected}` : ''}
      </span>
    </div>
  );
}

function ConversationPreview({
  onInspectTool,
}: {
  onInspectTool: (toolId: string) => void;
}) {
  return (
    <main className="conversation-fixture" aria-labelledby="fixture-chat-title">
      <section className="persistent-chat live-chat">
        <header className="chat-persona">
          <div>
            <strong id="fixture-chat-title">Turn process preview</strong>
            <span>OpenMPA Dev · visual fixture</span>
          </div>
        </header>
        <div className="chat-transcript-wrap">
          <div className="chat-transcript">
            <section className="conversation-turn" data-turn-status="running">
              <div className="chat-bubble user">
                Review the conversation renderer and show the execution process.
              </div>
              <TurnProcess turn={runningTurn} onInspectTool={onInspectTool} />
              <div className="chat-bubble assistant">
                <AssistantMarkdown text={answer} />
              </div>
            </section>
            <section className="conversation-turn" data-turn-status="completed">
              <div className="chat-bubble user">
                Keep completed work compact.
              </div>
              <TurnProcess turn={completedTurn} />
              <div className="chat-bubble assistant">
                <AssistantMarkdown text="Completed turns collapse to a one-line summary." />
              </div>
            </section>
          </div>
        </div>
        <form
          className="chat-composer"
          onSubmit={(event) => event.preventDefault()}
        >
          <ComposerShell className="composer-field">
            <textarea
              aria-label="Message"
              rows={1}
              placeholder="Message your assistant…"
            />
            <ComposerFooter className="composer-bar">
              <span className="composer-hint">Enter to send</span>
              <Button
                className="composer-action send"
                type="submit"
                variant="primary"
                size="icon-lg"
                aria-label="Send message"
              >
                ↑
              </Button>
            </ComposerFooter>
          </ComposerShell>
        </form>
      </section>
    </main>
  );
}

function SettingsPreview() {
  return (
    <main
      className="main-content desktop-settings fixture-page"
      aria-labelledby="fixture-settings-title"
    >
      <div className="page-heading">
        <div>
          <span className="eyebrow">SETTINGS</span>
          <h1 id="fixture-settings-title">Make the workspace yours.</h1>
          <p>
            Account, Runtime, assistant, theme and language controls share one
            compact desktop hierarchy.
          </p>
        </div>
        <Button variant="primary" size="md">
          Save changes
        </Button>
      </div>
      <div className="notice" role="status">
        Display preferences are stored locally and apply across every page.
      </div>
      <section className="task-detail-card desktop-form">
        <div className="desktop-section-head">
          <div>
            <h2>Display and behavior</h2>
            <p>Use one density and theme contract throughout OpenMPA.</p>
          </div>
        </div>
        <fieldset className="desktop-segmented">
          <legend>Theme</legend>
          <label>
            <input type="radio" name="fixture-theme" defaultChecked />
            <span>Follow system</span>
          </label>
          <label>
            <input type="radio" name="fixture-theme" />
            <span>Light</span>
          </label>
          <label>
            <input type="radio" name="fixture-theme" />
            <span>Dark</span>
          </label>
        </fieldset>
        <div className="desktop-inline">
          <label className="desktop-field fixture-field-grow">
            <span>Runtime region</span>
            <select defaultValue="cn-beijing">
              <option value="cn-beijing">China North · Beijing</option>
              <option value="ap-southeast-1">Asia Pacific · Singapore</option>
            </select>
          </label>
          <label className="desktop-field fixture-field-grow">
            <span>Workspace language</span>
            <select defaultValue="system">
              <option value="system">Follow system</option>
              <option value="zh-Hans">中文</option>
              <option value="en">English</option>
            </select>
          </label>
        </div>
        <label className="desktop-check">
          <input type="checkbox" defaultChecked />
          Keep completed execution details collapsed
        </label>
      </section>
      <section className="task-detail-card desktop-form">
        <div className="desktop-section-head">
          <div>
            <h2>Runtime connection</h2>
            <p>Connection state uses the same semantic feedback as Chat.</p>
          </div>
          <Button size="sm" variant="outline">
            Recheck
          </Button>
        </div>
        <div className="fixture-status-list">
          <StatusRow
            icon={<CheckCircle2 aria-hidden />}
            title="AgentKit Runtime"
            description="runtime-openmpa-dev · cn-beijing"
            status="Connected"
            tone="success"
          />
          <StatusRow
            icon={<CheckCircle2 aria-hidden />}
            title="Credential vault"
            description="Secrets stay in the native host"
            status="Protected"
            tone="success"
          />
        </div>
      </section>
    </main>
  );
}

function KnowledgePreview() {
  return (
    <main
      className="main-content fixture-page"
      aria-labelledby="fixture-knowledge-title"
    >
      <div className="page-heading">
        <div>
          <span className="eyebrow">SESSION KNOWLEDGE</span>
          <h1 id="fixture-knowledge-title">
            Reference material, with provenance.
          </h1>
          <p>
            Imported knowledge stays scoped to the selected conversation and
            keeps its source and version visible.
          </p>
        </div>
        <Button variant="primary" size="md" leading={<FileText aria-hidden />}>
          Import material
        </Button>
      </div>
      <label className="desktop-field project-fixture__session">
        <span>Conversation</span>
        <select defaultValue="design-review">
          <option value="design-review">Desktop design review</option>
          <option value="runtime-hardening">Runtime hardening</option>
        </select>
      </label>
      <section className="task-detail-card desktop-form">
        <div className="desktop-section-head">
          <div>
            <h2>Knowledge library</h2>
            <p>Two ready sources · one pending review</p>
          </div>
          <label className="search-box">
            <Search size={14} aria-hidden />
            <span className="sr-only">Search knowledge</span>
            <input placeholder="Search knowledge" />
          </label>
        </div>
        <div className="task-list">
          <button className="task-row" type="button" data-selected="true">
            <FileText size={18} aria-hidden />
            <div>
              <strong>OpenMPA design system</strong>
              <span>Version 3 · Ready · Markdown · 18 chunks</span>
            </div>
            <span className="status completed">Ready</span>
          </button>
          <button className="task-row" type="button">
            <FileText size={18} aria-hidden />
            <div>
              <strong>Runtime integration notes</strong>
              <span>Version 5 · Processing · PDF · 42 chunks</span>
            </div>
            <span className="status running">Processing</span>
          </button>
        </div>
      </section>
      <section className="task-detail-card desktop-form">
        <div className="desktop-section-head">
          <div>
            <h2>OpenMPA design system</h2>
            <p>Knowledge kn-84bf · Version 3 · Ready</p>
          </div>
          <Button size="sm" variant="outline">
            Read content
          </Button>
        </div>
        <div className="document-save-notice">
          Provenance verified. Imported from DESIGN.md on 27 October 2026.
        </div>
      </section>
    </main>
  );
}
