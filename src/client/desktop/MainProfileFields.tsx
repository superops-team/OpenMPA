import { Plus, Trash2 } from 'lucide-react';
import { useState } from 'react';
import type { MainProfileVM } from './studio-models';
import { useMessages } from './locale';
import { maskedHeader, toolNames } from './main-profile-form';
import { AssistantMarkdown } from './AssistantMarkdown';

type MCPServer = NonNullable<MainProfileVM['mcpServers']>[string];
export function MainSkillFields({
  skills,
  editing,
  onChange,
}: {
  skills: MainProfileVM['skills'];
  editing: boolean;
  onChange(value: MainProfileVM['skills']): void;
}) {
  const m = useMessages();
  return (
    <>
      <p className="resource-editor-note">{m.mainSkillsHint()}</p>
      {!skills.length && (
        <p className="resource-editor-note">{m.mainNoSkills()}</p>
      )}
      {skills.map((skill, index) => (
        <details
          className="profile-item"
          key={index}
          open={editing || undefined}
        >
          <summary>
            {skill.name || m.mainNewSkill()}{' '}
            <span className="profile-version">{skill.version}</span>
          </summary>
          {editing ? (
            <div className="profile-item-body">
              <div className="profile-field-row">
                <label>
                  {m.profileName()}
                  <input
                    value={skill.name}
                    maxLength={128}
                    onChange={(e) =>
                      onChange(
                        skills.map((s, i) =>
                          i === index ? { ...s, name: e.target.value } : s,
                        ),
                      )
                    }
                  />
                </label>
                <label>
                  {m.profileVersion()}
                  <input
                    value={skill.version}
                    maxLength={512}
                    onChange={(e) =>
                      onChange(
                        skills.map((s, i) =>
                          i === index ? { ...s, version: e.target.value } : s,
                        ),
                      )
                    }
                  />
                </label>
                <button
                  type="button"
                  className="resource-button"
                  aria-label={m.profileRemoveItem(
                    skill.name || m.mainNewSkill(),
                  )}
                  onClick={() => onChange(skills.filter((_, i) => i !== index))}
                >
                  <Trash2 size={16} aria-hidden="true" />
                </button>
              </div>
              <label>
                {m.mainSkillContent()}
                <textarea
                  rows={7}
                  value={skill.content}
                  maxLength={40000}
                  onChange={(e) =>
                    onChange(
                      skills.map((s, i) =>
                        i === index ? { ...s, content: e.target.value } : s,
                      ),
                    )
                  }
                />
              </label>
            </div>
          ) : (
            <div className="profile-item-body">
              <AssistantMarkdown text={skill.content} />
            </div>
          )}
        </details>
      ))}
      {editing && (
        <button
          type="button"
          className="resource-button"
          disabled={skills.length >= 32}
          onClick={() =>
            onChange([...skills, { name: '', version: '1.0.0', content: '' }])
          }
        >
          <Plus size={16} aria-hidden="true" />
          {m.mainAddSkill()}
        </button>
      )}
    </>
  );
}
function ToolSelection({
  tools,
  editing,
  onChange,
}: {
  tools: string[] | null;
  editing: boolean;
  onChange(value: string[] | null): void;
}) {
  const m = useMessages();
  const mode = tools === null ? 'all' : tools.length ? 'selected' : 'disabled';
  return editing ? (
    <div className="profile-tool-selection">
      <label>
        {m.mainToolAccess()}
        <select
          value={mode}
          onChange={(e) =>
            onChange(
              e.target.value === 'all'
                ? null
                : e.target.value === 'disabled'
                  ? []
                  : [''],
            )
          }
        >
          <option value="all">{m.mainAllTools()}</option>
          <option value="selected">{m.mainSelectedTools()}</option>
          <option value="disabled">{m.mainSourceDisabled()}</option>
        </select>
      </label>
      {mode === 'selected' && (
        <label>
          {m.mainToolNames()}
          <textarea
            key={JSON.stringify(tools)}
            rows={3}
            defaultValue={tools?.join('\n')}
            onBlur={(e) => onChange(toolNames(e.target.value))}
          />
        </label>
      )}
    </div>
  ) : (
    <p className="resource-editor-note">
      {mode === 'all'
        ? m.mainAllTools()
        : mode === 'disabled'
          ? m.mainSourceDisabled()
          : tools?.join(' · ')}
    </p>
  );
}
function HeaderFields({
  headers,
  onChange,
}: {
  headers: Record<string, string>;
  onChange(value: Record<string, string>): void;
}) {
  const m = useMessages();
  const [error, setError] = useState(false);
  return (
    <div className="profile-headers">
      {Object.entries(headers).map(([name, value]) => (
        <div className="profile-field-row" key={name}>
          <label>
            {m.mainHeaderName()}
            <input
              autoComplete="off"
              defaultValue={name}
              onBlur={(e) => {
                const key = e.target.value.trim();
                if (key === name) return;
                if (
                  !key ||
                  (key !== name &&
                    Object.keys(headers).some(
                      (k) => k.toLowerCase() === key.toLowerCase(),
                    ))
                ) {
                  setError(true);
                  e.target.value = name;
                  return;
                }
                setError(false);
                const next = { ...headers };
                delete next[name];
                next[key] = value;
                onChange(next);
              }}
            />
          </label>
          <label>
            {m.mainHeaderValue()}
            <input
              type="password"
              autoComplete="new-password"
              value={value === maskedHeader ? '' : value}
              placeholder={
                value === maskedHeader ? m.mainHeaderReenter() : undefined
              }
              maxLength={8192}
              onChange={(e) => onChange({ ...headers, [name]: e.target.value })}
            />
          </label>
          <button
            type="button"
            className="resource-button"
            aria-label={m.profileRemoveItem(name)}
            onClick={() =>
              onChange(
                Object.fromEntries(
                  Object.entries(headers).filter(([key]) => key !== name),
                ),
              )
            }
          >
            <Trash2 size={16} aria-hidden="true" />
          </button>
        </div>
      ))}
      {error && (
        <p role="alert" className="resource-editor-error">
          {m.profileDuplicateName()}
        </p>
      )}
      <button
        type="button"
        className="resource-button"
        disabled={Object.keys(headers).length >= 32}
        onClick={() => {
          let name = 'Authorization';
          let number = 1;
          while (
            Object.keys(headers).some(
              (k) => k.toLowerCase() === name.toLowerCase(),
            )
          )
            name = `X-Header-${number++}`;
          onChange({ ...headers, [name]: '' });
        }}
      >
        <Plus size={16} aria-hidden="true" />
        {m.mainAddHeader()}
      </button>
    </div>
  );
}
export function MainMCPFields({
  profile,
  editing,
  onChange,
}: {
  profile: MainProfileVM;
  editing: boolean;
  onChange(value: MainProfileVM): void;
}) {
  const m = useMessages();
  const [duplicate, setDuplicate] = useState(false);
  const sources = profile.mcpSources ?? [];
  const servers = profile.mcpServers ?? {};
  const changeServer = (name: string, value: MCPServer) =>
    onChange({ ...profile, mcpServers: { ...servers, [name]: value } });
  return (
    <>
      <p className="resource-editor-note">{m.mainMcpHint()}</p>
      {!sources.length && !Object.keys(servers).length && (
        <p className="resource-editor-note">{m.mainNoMcp()}</p>
      )}
      {sources.map((source, index) => (
        <div className="profile-item profile-item-body" key={`source-${index}`}>
          <div className="profile-field-row">
            {editing ? (
              <label>
                {m.mainResourceId()}
                <input
                  value={source.resourceId}
                  maxLength={512}
                  onChange={(e) =>
                    onChange({
                      ...profile,
                      mcpSources: sources.map((s, i) =>
                        i === index ? { ...s, resourceId: e.target.value } : s,
                      ),
                    })
                  }
                />
              </label>
            ) : (
              <strong>{source.resourceId}</strong>
            )}
            <span className="profile-kind">{m.mainCatalogSource()}</span>
            {editing && (
              <button
                type="button"
                className="resource-button"
                aria-label={m.profileRemoveItem(
                  source.resourceId || m.mainCatalogSource(),
                )}
                onClick={() =>
                  onChange({
                    ...profile,
                    mcpSources: sources.filter((_, i) => i !== index),
                  })
                }
              >
                <Trash2 size={16} aria-hidden="true" />
              </button>
            )}
          </div>
          <ToolSelection
            tools={source.enabledTools}
            editing={editing}
            onChange={(enabledTools) =>
              onChange({
                ...profile,
                mcpSources: sources.map((s, i) =>
                  i === index ? { ...s, enabledTools } : s,
                ),
              })
            }
          />
        </div>
      ))}
      {Object.entries(servers).map(([name, server]) => (
        <details
          className="profile-item"
          key={name}
          open={editing || undefined}
        >
          <summary>
            {name} <span className="profile-kind">{m.mainDirectSource()}</span>
          </summary>
          <div className="profile-item-body">
            {editing ? (
              <>
                <div className="profile-field-row">
                  <label>
                    {m.profileName()}
                    <input
                      defaultValue={name}
                      maxLength={128}
                      onBlur={(e) => {
                        const nextName = e.target.value.trim();
                        if (nextName === name) return;
                        if (
                          !nextName ||
                          (nextName !== name && nextName in servers)
                        ) {
                          setDuplicate(true);
                          e.target.value = name;
                          return;
                        }
                        setDuplicate(false);
                        const next = { ...servers };
                        delete next[name];
                        next[nextName] = server;
                        onChange({ ...profile, mcpServers: next });
                      }}
                    />
                  </label>
                  <button
                    type="button"
                    className="resource-button"
                    aria-label={m.profileRemoveItem(name)}
                    onClick={() =>
                      onChange({
                        ...profile,
                        mcpServers: Object.fromEntries(
                          Object.entries(servers).filter(
                            ([key]) => key !== name,
                          ),
                        ),
                      })
                    }
                  >
                    <Trash2 size={16} aria-hidden="true" />
                  </button>
                </div>
                <label>
                  {m.mainMcpUrl()}
                  <input
                    type="url"
                    value={server.url}
                    maxLength={2048}
                    onChange={(e) =>
                      changeServer(name, { ...server, url: e.target.value })
                    }
                  />
                </label>
                <HeaderFields
                  headers={server.headers}
                  onChange={(headers) =>
                    changeServer(name, { ...server, headers })
                  }
                />
              </>
            ) : (
              <>
                <p className="profile-address">{server.url}</p>
                {Object.keys(server.headers).length > 0 && (
                  <p className="resource-editor-note">
                    {m.mainSavedHeaders(Object.keys(server.headers).join(', '))}
                  </p>
                )}
              </>
            )}
            <ToolSelection
              tools={server.enabledTools}
              editing={editing}
              onChange={(enabledTools) =>
                changeServer(name, { ...server, enabledTools })
              }
            />
          </div>
        </details>
      ))}
      {duplicate && (
        <p role="alert" className="resource-editor-error">
          {m.profileDuplicateName()}
        </p>
      )}
      {editing && (
        <div className="resource-editor-actions">
          <button
            type="button"
            className="resource-button"
            disabled={sources.length >= 32}
            onClick={() =>
              onChange({
                ...profile,
                mcpSources: [
                  ...sources,
                  { resourceId: '', enabledTools: null },
                ],
              })
            }
          >
            <Plus size={16} aria-hidden="true" />
            {m.mainAddCatalog()}
          </button>
          <button
            type="button"
            className="resource-button"
            disabled={Object.keys(servers).length >= 32}
            onClick={() => {
              let n = 1;
              while (`server-${n}` in servers) n++;
              onChange({
                ...profile,
                mcpServers: {
                  ...servers,
                  [`server-${n}`]: { url: '', headers: {}, enabledTools: null },
                },
              });
            }}
          >
            <Plus size={16} aria-hidden="true" />
            {m.mainAddDirect()}
          </button>
        </div>
      )}
    </>
  );
}
