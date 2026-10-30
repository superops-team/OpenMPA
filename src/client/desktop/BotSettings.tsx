import { useId, useState } from 'react';
import { FileText, Puzzle } from 'lucide-react';
import { Button, ModalDialog, TabPanel, Tabs } from '../ui';
import { useMessages } from './locale';
import { useAssistantSettings } from './use-assistant-settings';
import { useModelCatalog } from './use-model-catalog';
import { ComposerModelPicker } from './ComposerModelPicker';
import { AssistantSkills } from './AssistantSkills';
import type {
  AssistantSetupInput,
  ConnectionVM,
  DesktopServices,
  SessionVM,
} from './models';

export function DesktopBotSettings(props: {
  services: DesktopServices;
  connection: ConnectionVM;
  session?: SessionVM;
  onSaved?: (session: SessionVM) => void;
  onCreate?: (input: AssistantSetupInput) => Promise<void>;
  onClose: () => void;
  mode?: 'create' | 'edit';
}) {
  const m = useMessages();
  const editor = useAssistantSettings(props);
  const catalog = useModelCatalog(
    props.services,
    props.connection,
    props.session?.id,
  );
  const [activeTab, setActiveTab] = useState<'instructions' | 'skills'>(
    'instructions',
  );
  const formId = useId();
  const creating = !props.session;
  const close = () => {
    if (!editor.busy) props.onClose();
  };
  const errorMessage =
    editor.error instanceof Error && !/^[a-zA-Z]+$/.test(editor.error.message)
      ? editor.error.message
      : creating
        ? m.createAssistantFailed()
        : m.saveAssistantSettingsFailed();
  return (
    <ModalDialog
      className="assistant-dialog"
      title={creating ? m.newAssistant() : m.assistantSettingsTitle()}
      description={
        creating
          ? m.newAssistantDescription()
          : m.assistantSettingsDescription()
      }
      closeLabel={m.dismiss()}
      onClose={close}
      footer={
        <>
          <Button variant="ghost" onClick={close} disabled={editor.busy}>
            {m.cancel()}
          </Button>
          <Button
            type="submit"
            form={formId}
            variant="primary"
            disabled={editor.busy || editor.loading || !editor.name.trim()}
          >
            {editor.busy
              ? m.working()
              : creating
                ? m.createAssistant()
                : m.saveAssistantSettings()}
          </Button>
        </>
      }
    >
      <form
        id={formId}
        className="desktop-form assistant-settings"
        onSubmit={(event) => {
          event.preventDefault();
          void editor.save();
        }}
      >
        {editor.error != null && (
          <div className="assistant-settings-error" role="alert">
            <span>
              {editor.loading ? m.loadAssistantSettingsFailed() : errorMessage}
            </span>
            {editor.loading && (
              <Button size="sm" onClick={editor.retry}>
                {m.retryLoading()}
              </Button>
            )}
          </div>
        )}
        <fieldset
          disabled={editor.busy || editor.loading}
          className="assistant-settings-fields"
        >
          <div className="assistant-basics">
            <label>
              {m.assistantName()}
              <input
                data-autofocus="true"
                aria-label={m.assistantName()}
                value={editor.name}
                maxLength={256}
                placeholder={m.assistantNamePlaceholder()}
                onChange={(event) => editor.setName(event.target.value)}
              />
            </label>
            <div className="assistant-model-picker">
              <span>{m.model()}</span>
              <ComposerModelPicker
                config={editor.model}
                busy={editor.busy || editor.loading}
                unavailable={editor.modelUnavailable}
                modelCatalog={catalog.models}
                catalog={catalog}
                catalogLoading={catalog.loading}
                catalogFailed={catalog.failed}
                onRetryCatalog={catalog.retry}
                placement="below"
                onApply={async (mode, model) => {
                  editor.setModel({
                    ...editor.model,
                    modelMode: mode,
                    model: mode === 'inherit' ? undefined : model,
                  });
                  return true;
                }}
              />
              {editor.modelUnavailable && (
                <p className="composer-model-note">
                  {m.diagnosticModelUnavailable()}
                </p>
              )}
            </div>
          </div>
          <Tabs
            group={formId}
            label={m.assistantSettingsTitle()}
            value={activeTab}
            onChange={setActiveTab}
            options={[
              {
                id: 'instructions',
                label: 'AGENTS.md',
                icon: <FileText aria-hidden="true" />,
              },
              {
                id: 'skills',
                label: 'Skills',
                icon: <Puzzle aria-hidden="true" />,
              },
            ]}
          />
          <TabPanel
            group={formId}
            id="instructions"
            active={activeTab === 'instructions'}
          >
            <div className="assistant-instructions-panel">
              <label>
                AGENTS.md
                <textarea
                  className="assistant-agents-editor"
                  value={editor.config.agentsMd}
                  maxLength={32000}
                  placeholder={m.agentsMdPlaceholder()}
                  onChange={(event) =>
                    editor.setConfig({
                      ...editor.config,
                      agentsMd: event.target.value,
                    })
                  }
                />
              </label>
              <p>{m.agentsMdHelp()}</p>
            </div>
          </TabPanel>
          <TabPanel group={formId} id="skills" active={activeTab === 'skills'}>
            {editor.skillsFailed && <p role="status">{m.skillsLoadFailed()}</p>}
            <AssistantSkills
              skills={editor.config.skills}
              catalog={editor.catalog}
              onChange={(skills) =>
                editor.setConfig({ ...editor.config, skills })
              }
            />
          </TabPanel>
        </fieldset>
        {editor.loading && editor.error == null && (
          <span className="sr-only" role="status">
            {m.loadingAssistantSettings()}
          </span>
        )}
      </form>
    </ModalDialog>
  );
}
