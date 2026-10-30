import { useEffect, useRef, useState } from 'react';
import type { ConnectionVM, SessionVM, PlatformVM } from './models';
import type {
  ResourcesServices,
  SkillPageVM,
  SkillSpaceVM,
  MCPDirectoryVM,
  ConversationTarget,
} from './studio-models';
import { localized, useMessages, useMessageState } from './locale';
import { studioFailure } from './studio-services';
import { skillChoice } from './composer-drafts';
import {
  BookOpen,
  Server,
  FileText,
  Settings2,
  Search,
  RefreshCw,
  FolderOpen,
  ChevronLeft,
  ChevronRight,
  Trash2,
} from 'lucide-react';
import { AgentsMDEditor } from './AgentsMDEditor';
import { MainProfileEditor } from './MainProfileEditor';
import { SkillUploadButton } from './SkillUpload';
import './resource-page.css';

type Section = 'skills' | 'mcp' | 'agentsMd' | 'mainProfile';
export function DesktopResources({
  services,
  connection,
  platform,
  target,
  onUse,
  hidden,
}: {
  services?: ResourcesServices;
  connection?: ConnectionVM;
  platform?: PlatformVM;
  session?: SessionVM;
  target?: ConversationTarget;
  onUse(target: ConversationTarget, text: string): boolean;
  onConfigureMCP(): void;
  hidden: boolean;
}) {
  const m = useMessages();
  const [section, setSection] = useState<Section>('skills');
  const [query, setQuery] = useState('');
  const [spaces, setSpaces] = useState<SkillSpaceVM[]>();
  const [spaceId, setSpaceId] = useState('');
  const [page, setPage] = useState<SkillPageVM>();
  const [mcp, setMCP] = useState<MCPDirectoryVM>();
  const [busy, setBusy] = useState(false);
  const [deleting, setDeleting] = useState('');
  const deletingRef = useRef(false);
  const [error, setError] = useMessageState();
  const request = useRef(0);
  const mcpRequest = useRef(0);
  const identity = JSON.stringify([
    connection?.epoch,
    connection?.scope,
    platform?.epoch,
    platform?.region,
  ]);
  const current = useRef(identity);
  current.current = identity;
  useEffect(() => {
    current.current = identity;
    return () => {
      current.current = '';
      request.current++;
      mcpRequest.current++;
    };
  }, [identity]);
  const skills = async (selected = spaceId, number = 1) => {
    if (!services || !connection || busy) return;
    const id = ++request.current;
    const scope = identity;
    const active = () => current.current === scope && request.current === id;
    setBusy(true);
    setError('');
    setPage(undefined);
    try {
      const mounts = await services.discoverSkillSpaces(connection.epoch);
      if (!active()) return;
      setSpaces(mounts);
      const selectedId = mounts.some((item) => item.id === selected)
        ? selected
        : mounts[0]?.id || '';
      setSpaceId(selectedId);
      if (selectedId) {
        const result = await services.listSkills(
          connection.epoch,
          mounts.find((item) => item.id === selectedId)?.region ||
            platform?.region ||
            'cn-beijing',
          selectedId,
          number,
          20,
        );
        if (
          active() &&
          result.connectionEpoch === connection.epoch &&
          (platform?.epoch === undefined ||
            result.platformEpoch === platform.epoch)
        )
          setPage(result);
        else if (active()) setError(localized((m) => m.staleConnection()));
      }
    } catch (error) {
      if (active()) setError(studioFailure(error));
    } finally {
      if (active()) setBusy(false);
    }
  };
  const loadMCP = async () => {
    if (!services || !connection) return;
    const id = ++mcpRequest.current;
    const scope = identity;
    setError('');
    setMCP(undefined);
    try {
      const result = await services.listWorkerMCPServers(connection.epoch);
      if (current.current === scope && id === mcpRequest.current)
        setMCP(result);
    } catch (error) {
      if (current.current === scope && id === mcpRequest.current)
        setError(studioFailure(error));
    }
  };
  const removeSkill = async (skill: SkillPageVM['items'][number]) => {
    if (
      !services?.deleteSkill ||
      !connection ||
      !skill.id ||
      busy ||
      deletingRef.current ||
      !window.confirm(m.deleteBoundSkillConfirm(skill.name))
    )
      return;
    const scope = identity;
    deletingRef.current = true;
    setDeleting(skill.id);
    setError('');
    try {
      await services.deleteSkill(connection.epoch, spaceId, skill.id);
      if (current.current === scope) {
        const number =
          page?.items.length === 1
            ? Math.max(1, page.pageNumber - 1)
            : page?.pageNumber || 1;
        await skills(spaceId, number);
      }
    } catch (reason) {
      if (current.current === scope) setError(studioFailure(reason));
    } finally {
      deletingRef.current = false;
      if (current.current === scope) setDeleting('');
    }
  };
  useEffect(() => {
    if (hidden || !connection || !services) return;
    if (section === 'skills' && spaces === undefined) void skills();
    if (section === 'mcp' && mcp === undefined) void loadMCP();
  }, [hidden, section]);
  return (
    <main
      className="main-content desktop-studio-page resource-page"
      hidden={hidden}
      style={hidden ? { display: 'none' } : undefined}
      data-testid="studio-resources"
    >
      <div className="page-heading">
        <div>
          <h1>{m.resources()}</h1>
          <p>{m.resourcePageSubtitle()}</p>
        </div>
      </div>
      <nav className="resource-tabs" aria-label={m.resourceSections()}>
        {(['skills', 'mcp', 'agentsMd', 'mainProfile'] as const).map((item) => (
          <button
            className={`resource-tab ${section === item ? 'is-active' : ''}`}
            type="button"
            key={item}
            data-testid={`resources-tab-${item}`}
            aria-pressed={section === item}
            onClick={() => setSection(item)}
          >
            {(() => {
              const Icon = {
                skills: BookOpen,
                mcp: Server,
                agentsMd: FileText,
                mainProfile: Settings2,
              }[item];
              return <Icon size={16} aria-hidden="true" />;
            })()}
            {m[item]()}
          </button>
        ))}
      </nav>
      {!connection ? (
        <p>{m.connectRuntimeResources()}</p>
      ) : !services ? (
        <p>{m.capabilityUnavailable()}</p>
      ) : (
        <>
          {error && (
            <p role="alert" aria-label={m.resourceError()}>
              {error}
            </p>
          )}
          <section
            className="resource-skills-panel"
            hidden={section !== 'skills'}
            style={section !== 'skills' ? { display: 'none' } : undefined}
            aria-busy={busy || !!deleting}
          >
            <div className="resource-toolbar">
              <label className="resource-space-field">
                <span>{m.mountedSkillSpace()}</span>
                {spaces?.length ? (
                  <select
                    data-testid="resources-skill-space"
                    value={spaceId}
                    disabled={busy || !!deleting}
                    onChange={(event) => {
                      setSpaceId(event.target.value);
                      setQuery('');
                      void skills(event.target.value);
                    }}
                  >
                    {spaces.map((space) => (
                      <option key={space.id} value={space.id}>
                        {space.name && space.name !== 'Skill Space'
                          ? space.name
                          : space.id}
                      </option>
                    ))}
                  </select>
                ) : (
                  <span className="resource-space-placeholder">
                    {spaces === undefined
                      ? m.skillSpacesNotLoaded()
                      : m.noMountedSkillSpaces()}
                  </span>
                )}
              </label>
              <div className="resource-toolbar-actions">
                <label className="resource-search">
                  <Search size={16} aria-hidden="true" />
                  <input
                    value={query}
                    onChange={(event) => setQuery(event.target.value)}
                    placeholder={m.searchBoundSkills()}
                    aria-label={m.searchBoundSkills()}
                  />
                </label>
                <button
                  type="button"
                  className="resource-icon-button"
                  title={m.refreshSkills()}
                  aria-label={m.refreshSkills()}
                  disabled={busy || !!deleting}
                  onClick={() => void skills()}
                >
                  <RefreshCw
                    size={16}
                    className={busy ? 'resource-spinning' : ''}
                    aria-hidden="true"
                  />
                </button>
                <SkillUploadButton
                  services={services}
                  disabled={busy || !!deleting}
                  epoch={connection.epoch}
                  space={spaces?.find((space) => space.id === spaceId)}
                  onUploaded={async (uploadedSpaceId) => {
                    if (uploadedSpaceId === spaceId) await skills(spaceId, 1);
                  }}
                />
              </div>
            </div>
            {spaces?.find((space) => space.id === spaceId) && (
              <div className="resource-space-summary">
                <FolderOpen size={19} aria-hidden="true" />
                <div>
                  <strong>
                    {spaces.find((space) => space.id === spaceId)!.name}
                  </strong>
                  <code>{spaceId}</code>
                </div>
                <span className="resource-count">
                  {m.boundSkillsCount(String(page?.totalCount ?? 0))}
                </span>
              </div>
            )}
            <div className="resource-skill-list">
              {busy ? (
                <div className="resource-empty" role="status">
                  <RefreshCw
                    size={24}
                    className="resource-spinning"
                    aria-hidden="true"
                  />
                  <p>{m.loadingBoundSkills()}</p>
                </div>
              ) : page?.items.length ? (
                <>
                  {page.items
                    .filter(
                      (skill) =>
                        !query.trim() ||
                        `${skill.name} ${skill.description}`
                          .toLowerCase()
                          .includes(query.trim().toLowerCase()),
                    )
                    .map((skill) => (
                      <article
                        className="resource-skill-row"
                        key={skill.id || skill.name}
                      >
                        <div className="resource-skill-icon">
                          <BookOpen size={19} aria-hidden="true" />
                        </div>
                        <div className="resource-skill-copy">
                          <h3>{skill.name}</h3>
                          <p>
                            {skill.description || m.skillDescriptionEmpty()}
                          </p>
                        </div>
                        {skill.version && (
                          <span className="resource-version">
                            {skill.version}
                          </span>
                        )}
                        {target && (
                          <button
                            type="button"
                            className="resource-use-button"
                            disabled={!skillChoice(skill.name)}
                            onClick={() => {
                              const text = skillChoice(skill.name);
                              if (!text || !onUse(target, text))
                                setError(
                                  localized((m) => m.conversationRequired()),
                                );
                            }}
                          >
                            {m.useInConversation()}
                          </button>
                        )}
                        <button
                          type="button"
                          className="resource-page-button is-danger"
                          aria-label={m.deleteBoundSkill(skill.name)}
                          disabled={
                            busy ||
                            !!deleting ||
                            !skill.id ||
                            !services.deleteSkill
                          }
                          onClick={() => void removeSkill(skill)}
                        >
                          <Trash2 size={15} aria-hidden="true" />
                          {deleting === skill.id
                            ? m.deletingBoundSkill()
                            : m.delete()}
                        </button>
                      </article>
                    ))}
                  {query.trim() &&
                    !page.items.some((skill) =>
                      `${skill.name} ${skill.description}`
                        .toLowerCase()
                        .includes(query.trim().toLowerCase()),
                    ) && (
                      <div className="resource-empty">
                        <Search size={26} aria-hidden="true" />
                        <h3>{m.noSkillSearchResults()}</h3>
                        <p>{m.tryAnotherSkillSearch()}</p>
                      </div>
                    )}
                </>
              ) : (
                !error && (
                  <div className="resource-empty">
                    <div className="resource-empty-icon">
                      <BookOpen size={30} aria-hidden="true" />
                    </div>
                    <h3>
                      {spaceId ? m.boundSpaceEmpty() : m.noMountedSkillSpaces()}
                    </h3>
                    <p>
                      {spaceId
                        ? m.boundSpaceEmptyHint()
                        : m.bindSkillSpaceHint()}
                    </p>
                  </div>
                )
              )}
            </div>
            {page && (page.pageNumber > 1 || page.hasMore) && (
              <footer className="resource-pagination">
                <span>
                  {m.usagePage()} {page.pageNumber} · {m.usageTotal()}{' '}
                  {page.totalCount}
                </span>
                <div>
                  <button
                    type="button"
                    className="resource-page-button"
                    disabled={busy || !!deleting || page.pageNumber <= 1}
                    onClick={() => void skills(spaceId, page.pageNumber - 1)}
                  >
                    <ChevronLeft size={15} aria-hidden="true" />
                    {m.previousSkillsPage()}
                  </button>
                  <button
                    type="button"
                    className="resource-page-button"
                    disabled={busy || !!deleting || !page.hasMore}
                    onClick={() => void skills(spaceId, page.pageNumber + 1)}
                  >
                    {m.nextSkillsPage()}
                    <ChevronRight size={15} aria-hidden="true" />
                  </button>
                </div>
              </footer>
            )}
            <p className="resource-scope-note">{m.workerSkillSpaceScope()}</p>
          </section>
          <section
            className="task-detail-card desktop-form resource-mcp-panel"
            hidden={section !== 'mcp'}
            style={section !== 'mcp' ? { display: 'none' } : undefined}
          >
            <p>{m.workerMcpScope()}</p>
            <button
              type="button"
              className="text-button"
              onClick={() => void loadMCP()}
            >
              {m.refreshMcpServers()}
            </button>
            {!mcp ? (
              <p>{m.mcpServersNotLoaded()}</p>
            ) : mcp.servers.length === 0 ? (
              <p>{m.noConfiguredMcpServers()}</p>
            ) : (
              mcp.servers.map((server) => (
                <article className="resource-mcp-server" key={server.id}>
                  <h3>{server.name || server.alias || server.id}</h3>
                  {server.address && (
                    <p className="studio-resource-address">{server.address}</p>
                  )}
                  <p>{server.description}</p>
                  {!server.toolsKnown ? (
                    <p>{m.toolDetailsUnavailable()}</p>
                  ) : !server.tools?.length ? (
                    <p>{m.noServerTools()}</p>
                  ) : (
                    server.tools.map((tool) => (
                      <p key={tool.name}>
                        <strong>{tool.name}</strong> {tool.description}
                      </p>
                    ))
                  )}
                </article>
              ))
            )}
          </section>
          <AgentsMDEditor
            key={`agents:${connection.epoch}:${connection.scope}`}
            services={services}
            connection={connection}
            hidden={hidden || section !== 'agentsMd'}
          />
          <MainProfileEditor
            key={`main:${connection.epoch}:${connection.scope}`}
            services={services}
            connection={connection}
            hidden={hidden || section !== 'mainProfile'}
          />
        </>
      )}
    </main>
  );
}
