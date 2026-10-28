import {
  errorMessage,
  localized,
  useMessages,
  useMessageState,
} from './locale';
import { useEffect, useMemo, useRef, useState } from 'react';
import { FileText, Plus, Puzzle, Trash2, Upload } from 'lucide-react';
import { Button, IconButton, ModalDialog, TabPanel, Tabs } from '../ui';
import type {
  AssistantConfigVM,
  AssistantSkillVM,
  ConnectionVM,
  DesktopServices,
  SessionVM,
} from './models';

function skillLabel(skill: AssistantSkillVM) {
  return skill.name || skill.id;
}

function uniqueSkills(skills: AssistantSkillVM[]) {
  const seen = new Set<string>();
  const out: AssistantSkillVM[] = [];
  for (const skill of skills) {
    const id = skill.id.trim();
    if (!id || seen.has(id)) continue;
    seen.add(id);
    out.push({ ...skill, id, name: skill.name.trim() || id });
  }
  return out;
}

export function DesktopBotSettings({
  services,
  connection,
  session,
  onSaved,
  onClose,
}: {
  services: DesktopServices;
  connection: ConnectionVM;
  session: SessionVM;
  onSaved: (session: SessionVM) => void;
  onClose: () => void;
}) {
  const m = useMessages();
  const [config, setConfig] = useState<AssistantConfigVM>();
  const [catalog, setCatalog] = useState<AssistantSkillVM[]>([]);
  const [query, setQuery] = useState('');
  const [skillDraft, setSkillDraft] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useMessageState();
  const [activeTab, setActiveTab] = useState<'instructions' | 'skills'>(
    'instructions',
  );
  const generation = useRef(0);
  const assistantId = session.mpaInstanceId || connection.runtimeId;
  useEffect(() => {
    const current = ++generation.current;
    setConfig(undefined);
    setCatalog([]);
    setError('');
    void Promise.all([
      services.sessions.assistantConfig(
        connection.epoch,
        session.id,
        assistantId,
      ),
      services.sessions.skillCatalog(connection.epoch, ''),
    ])
      .then(([config, catalog]) => {
        if (generation.current === current) {
          setConfig(config);
          setCatalog(catalog);
        }
      })
      .catch((failure: unknown) => {
        if (generation.current === current)
          setError(
            errorMessage(
              failure,
              localized((m) => m.settingsAssistantLoadFailed()),
            ),
          );
      });
    return () => {
      generation.current++;
    };
  }, [services, connection.epoch, session.id, assistantId]);

  const filteredCatalog = useMemo(() => {
    const needle = query.trim().toLowerCase();
    if (!needle) return catalog;
    return catalog.filter((skill) =>
      `${skill.id} ${skill.name} ${skill.description || ''}`
        .toLowerCase()
        .includes(needle),
    );
  }, [catalog, query]);

  const addSkill = (skill: AssistantSkillVM) => {
    if (!config) return;
    setConfig({
      ...config,
      skills: uniqueSkills([...config.skills, skill]),
    });
  };

  const removeSkill = (id: string) => {
    if (!config) return;
    setConfig({
      ...config,
      skills: config.skills.filter((skill) => skill.id !== id),
    });
  };

  const importSkillText = () => {
    const value = skillDraft.trim();
    if (!value || !config) return;
    addSkill({
      id: value.split(/\s+/)[0],
      name: value.split(/\s+/)[0],
      description: value,
      source: 'imported',
      content: value,
    });
    setSkillDraft('');
  };

  return (
    <ModalDialog
      closeLabel={m.closeDialog(m.settingsAssistantTitle())}
      title={m.settingsAssistantTitle()}
      description={m.settingsAssistantDialogDescription()}
      onClose={onClose}
      footer={
        <>
          <Button type="button" variant="ghost" onClick={onClose}>
            {m.cancel()}
          </Button>
          <Button
            type="submit"
            form="assistant-settings-form"
            variant="primary"
            disabled={busy || !config}
          >
            {m.settingsSaveAssistantSettings()}
          </Button>
        </>
      }
    >
      <div className="desktop-form assistant-settings">
        {error && <p role="alert">{error}</p>}
        {!config ? (
          <p role="status">{m.settingsAssistantLoading()}</p>
        ) : (
          <form
            id="assistant-settings-form"
            className="desktop-form"
            onSubmit={(event) => {
              event.preventDefault();
              if (busy) return;
              const current = generation.current;
              setBusy(true);
              setError('');
              void services.sessions
                .saveAssistantConfig(
                  connection.epoch,
                  session.id,
                  assistantId,
                  {
                    agentsMd: config.agentsMd,
                    skills: uniqueSkills(config.skills),
                  },
                )
                .then((saved) => {
                  if (generation.current === current) {
                    setConfig(saved);
                    onSaved(session);
                  }
                })
                .catch((failure: unknown) => {
                  if (generation.current === current)
                    setError(
                      errorMessage(
                        failure,
                        localized((m) => m.settingsAssistantSaveFailed()),
                      ),
                    );
                })
                .finally(() => {
                  if (generation.current === current) setBusy(false);
                });
            }}
          >
            <Tabs
              group="assistant-settings"
              label={m.settingsAssistantSections()}
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
                  label: m.settingsSkills(),
                  icon: <Puzzle aria-hidden="true" />,
                },
              ]}
            />
            <TabPanel
              group="assistant-settings"
              id="instructions"
              active={activeTab === 'instructions'}
            >
              <div className="assistant-instructions-panel">
                <label>
                  AGENTS.md
                  <textarea
                    autoFocus
                    className="assistant-agents-editor"
                    value={config.agentsMd}
                    maxLength={32000}
                    placeholder={m.settingsAssistantInstructionsPlaceholder()}
                    onChange={(event) =>
                      setConfig({ ...config, agentsMd: event.target.value })
                    }
                  />
                </label>
                <p>{m.settingsAssistantInstructionsHint()}</p>
              </div>
            </TabPanel>
            <TabPanel
              group="assistant-settings"
              id="skills"
              active={activeTab === 'skills'}
            >
              <div className="assistant-settings-grid">
                <section>
                  <h3>{m.settingsSelectedSkills()}</h3>
                  {config.skills.length === 0 ? (
                    <p>{m.settingsNoSelectedSkills()}</p>
                  ) : (
                    <div className="assistant-skill-list">
                      {config.skills.map((skill) => (
                        <div className="assistant-skill-row" key={skill.id}>
                          <div>
                            <strong>{skillLabel(skill)}</strong>
                            <p>{skill.description || skill.id}</p>
                          </div>
                          <IconButton
                            label={m.settingsRemoveSkill(skillLabel(skill))}
                            variant="ghost"
                            size="icon-sm"
                            onClick={() => removeSkill(skill.id)}
                          >
                            <Trash2 size={14} />
                          </IconButton>
                        </div>
                      ))}
                    </div>
                  )}
                  <label>
                    {m.settingsImportSkillText()}
                    <textarea
                      value={skillDraft}
                      placeholder={m.settingsSkillImportPlaceholder()}
                      onChange={(event) => setSkillDraft(event.target.value)}
                    />
                  </label>
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    leading={<Upload size={14} />}
                    disabled={!skillDraft.trim()}
                    onClick={importSkillText}
                  >
                    {m.settingsImportSkill()}
                  </Button>
                </section>
                <section>
                  <h3>{m.settingsSkillSpace()}</h3>
                  <label>
                    {m.settingsSearchSkills()}
                    <input
                      value={query}
                      placeholder={m.settingsSearchSkillsPlaceholder()}
                      onChange={(event) => setQuery(event.target.value)}
                    />
                  </label>
                  <div className="assistant-skill-list assistant-skill-catalog">
                    {filteredCatalog.map((skill) => (
                      <Button
                        type="button"
                        className="assistant-skill-option"
                        variant="outline"
                        size="content"
                        key={skill.id}
                        onClick={() => addSkill(skill)}
                        disabled={config.skills.some(
                          (item) => item.id === skill.id,
                        )}
                      >
                        <Plus size={14} aria-hidden="true" />
                        <span>
                          <strong>{skillLabel(skill)}</strong>
                          <small>{skill.description || skill.id}</small>
                        </span>
                      </Button>
                    ))}
                  </div>
                </section>
              </div>
            </TabPanel>
          </form>
        )}
      </div>
    </ModalDialog>
  );
}
