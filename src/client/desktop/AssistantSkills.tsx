import { useMemo, useState } from 'react';
import { Plus, Trash2, Upload } from 'lucide-react';
import { Button, IconButton } from '../ui';
import type { AssistantSkillVM } from './models';
import { useMessages } from './locale';

export function uniqueSkills(skills: AssistantSkillVM[]) {
  const seen = new Set<string>();
  return skills
    .map((skill) => ({
      ...skill,
      id: skill.id.trim(),
      name: skill.name.trim() || skill.id.trim(),
    }))
    .filter((skill) => {
      if (!skill.id || seen.has(skill.id)) return false;
      seen.add(skill.id);
      return true;
    });
}
const skillLabel = (skill: AssistantSkillVM) => skill.name || skill.id;
export function AssistantSkills({
  skills,
  catalog,
  onChange,
}: {
  skills: AssistantSkillVM[];
  catalog: AssistantSkillVM[];
  onChange: (skills: AssistantSkillVM[]) => void;
}) {
  const m = useMessages();
  const [query, setQuery] = useState('');
  const [skillDraft, setSkillDraft] = useState('');
  const filteredCatalog = useMemo(
    () =>
      catalog.filter((skill) =>
        `${skill.id} ${skill.name} ${skill.description || ''}`
          .toLowerCase()
          .includes(query.trim().toLowerCase()),
      ),
    [catalog, query],
  );
  const addSkill = (skill: AssistantSkillVM) =>
    onChange(uniqueSkills([...skills, skill]));
  const removeSkill = (id: string) =>
    onChange(skills.filter((skill) => skill.id !== id));
  const importSkillText = () => {
    const content = skillDraft.trim();
    if (!content) return;
    const id = content.split(/\s+/)[0];
    addSkill({
      id,
      name: id,
      description: content,
      source: 'imported',
      content,
    });
    setSkillDraft('');
  };
  return (
    <div className="assistant-settings-grid">
      <section>
        <h3>{m.selectedSkills()}</h3>
        {skills.length === 0 ? (
          <p>{m.noSkillsSelected()}</p>
        ) : (
          <div className="assistant-skill-list">
            {skills.map((skill) => (
              <div className="assistant-skill-row" key={skill.id}>
                <div>
                  <strong>{skillLabel(skill)}</strong>
                  <p>{skill.description || skill.id}</p>
                </div>
                <IconButton
                  label={m.removeSkill(skillLabel(skill))}
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
          {m.importSkillText()}
          <textarea
            value={skillDraft}
            placeholder={m.importSkillPlaceholder()}
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
          {m.importSkill()}
        </Button>
      </section>
      <section>
        <h3>{m.skillSpace()}</h3>
        <label>
          {m.searchSkills()}
          <input
            value={query}
            placeholder={m.searchSkillPlaceholder()}
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
              disabled={skills.some((item) => item.id === skill.id)}
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
  );
}
