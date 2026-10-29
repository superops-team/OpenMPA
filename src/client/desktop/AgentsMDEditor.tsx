import { Edit3, RefreshCw, Save } from 'lucide-react';
import { AssistantMarkdown } from './AssistantMarkdown';
import type { ConnectionVM } from './models';
import type { ResourcesServices } from './studio-models';
import { useMessages } from './locale';
import { useResourceEditor } from './use-resource-editor';

export function AgentsMDEditor({
  services,
  connection,
  hidden,
}: {
  services: ResourcesServices;
  connection: ConnectionVM;
  hidden: boolean;
}) {
  const m = useMessages();
  const editor = useResourceEditor({
    hidden,
    read: () => services.readAgentsMD(connection.epoch),
    write: (record, content: string) =>
      services.saveAgentsMD(connection.epoch, {
        revision: record.revision,
        content,
      }),
    value: (record) => record.content,
  });
  return (
    <section
      className="resource-document"
      hidden={hidden}
      style={hidden ? { display: 'none' } : undefined}
      data-testid="resources-agentsMd"
    >
      <header className="resource-editor-toolbar">
        <div>
          <h2>AGENTS.md</h2>
          <p>{m.agentsDocumentScope()}</p>
        </div>
        <div className="resource-editor-actions">
          <button
            type="button"
            className="resource-button"
            aria-label={m.reloadAgentsMd()}
            title={m.reloadAgentsMd()}
            disabled={editor.busy}
            onClick={() => void editor.refresh()}
          >
            <RefreshCw size={16} aria-hidden="true" />
          </button>
          {editor.editing ? (
            <>
              <button
                type="button"
                className="resource-button"
                disabled={editor.busy}
                onClick={editor.cancel}
              >
                {m.cancel()}
              </button>
              <button
                type="button"
                className="resource-button is-primary"
                disabled={editor.busy || !editor.fresh || !editor.dirty}
                onClick={() => void editor.save()}
              >
                <Save size={16} aria-hidden="true" />
                {m.saveAgentsMd()}
              </button>
            </>
          ) : (
            <button
              type="button"
              className="resource-button"
              disabled={editor.busy || !editor.record}
              onClick={editor.begin}
            >
              <Edit3 size={16} aria-hidden="true" />
              {m.profileEdit()}
            </button>
          )}
        </div>
      </header>
      {editor.error && (
        <p role="alert" className="resource-editor-error">
          {editor.error}
        </p>
      )}
      {editor.notice && (
        <p role="status" className="resource-editor-note">
          {editor.notice}
        </p>
      )}
      {editor.busy && !editor.record ? (
        <p role="status">{m.working()}</p>
      ) : !editor.record ? (
        <p>{m.profileReadFailed()}</p>
      ) : editor.editing ? (
        <>
          <textarea
            aria-label={m.agentsMdInstructions()}
            className="resource-document-editor"
            data-testid="resources-agentsMd-draft"
            value={editor.draft ?? ''}
            maxLength={40000}
            spellCheck={false}
            onChange={(event) => editor.update(event.target.value)}
          />
          {(!editor.fresh || editor.dirty) && (
            <p className="resource-editor-note">
              {editor.fresh
                ? m.profileUnsaved()
                : m.readInstructionsBeforeSaving()}
            </p>
          )}
        </>
      ) : (
        <article
          className="resource-document-body"
          data-testid="resources-agentsMd-content"
        >
          {editor.record.content ? (
            <AssistantMarkdown text={editor.record.content} />
          ) : (
            <p className="resource-editor-note">{m.profileNoInstructions()}</p>
          )}
        </article>
      )}
    </section>
  );
}
