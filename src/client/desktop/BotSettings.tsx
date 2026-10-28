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
  const [config, setConfig] = useState<AssistantConfigVM>();
  const [catalog, setCatalog] = useState<AssistantSkillVM[]>([]);
  const [query, setQuery] = useState('');
  const [skillDraft, setSkillDraft] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
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
            failure instanceof Error
              ? failure.message
              : 'Could not load assistant settings.',
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
      title="Assistant settings"
      description="Configure this assistant locally. New turns send the saved AGENTS.md and selected skills with the request."
      onClose={onClose}
      footer={
        <>
          <Button type="button" variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button
            type="submit"
            form="assistant-settings-form"
            variant="primary"
            disabled={busy || !config}
          >
            Save assistant settings
          </Button>
        </>
      }
    >
      <div className="desktop-form assistant-settings">
        {error && <p role="alert">{error}</p>}
        {!config ? (
          <p role="status">Loading assistant settings…</p>
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
                      failure instanceof Error
                        ? failure.message
                        : 'Could not save assistant settings.',
                    );
                })
                .finally(() => {
                  if (generation.current === current) setBusy(false);
                });
            }}
          >
            <Tabs
              group="assistant-settings"
              label="Assistant settings sections"
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
                    placeholder="Write persistent instructions for this assistant."
                    onChange={(event) =>
                      setConfig({ ...config, agentsMd: event.target.value })
                    }
                  />
                </label>
                <p>
                  These persistent instructions are attached to future turns.
                  Existing conversation history is unchanged.
                </p>
              </div>
            </TabPanel>
            <TabPanel
              group="assistant-settings"
              id="skills"
              active={activeTab === 'skills'}
            >
              <div className="assistant-settings-grid">
                <section>
                  <h3>Selected skills</h3>
                  {config.skills.length === 0 ? (
                    <p>No skills selected.</p>
                  ) : (
                    <div className="assistant-skill-list">
                      {config.skills.map((skill) => (
                        <div className="assistant-skill-row" key={skill.id}>
                          <div>
                            <strong>{skillLabel(skill)}</strong>
                            <p>{skill.description || skill.id}</p>
                          </div>
                          <IconButton
                            label={`Remove ${skillLabel(skill)}`}
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
                    Import skill text
                    <textarea
                      value={skillDraft}
                      placeholder="Paste a skill name or short skill instruction."
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
                    Import skill
                  </Button>
                </section>
                <section>
                  <h3>Skill space</h3>
                  <label>
                    Search skills
                    <input
                      value={query}
                      placeholder="Search by name or description"
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
