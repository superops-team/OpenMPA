import { useEffect, useRef, useState } from 'react';
import type { ConnectionVM } from './models';
import type {
  CollaborationServices,
  CollaborationBindingVM,
  CollaborationSpaceVM,
  CollaborationAgentVM,
  RegistryPageVM,
} from './studio-models';
import { localized, useMessages, useMessageState } from './locale';
import { studioFailure } from './studio-services';
import { CollaborationManagement } from './CollaborationManagement';
import { Users, Bot, RefreshCw, Link2, Unlink, Search } from 'lucide-react';
import './collaboration-page.css';

interface MemberDraft {
  alias: string;
}
export function DesktopCollaboration({
  services,
  connection,
  hidden,
}: {
  services?: CollaborationServices;
  connection?: ConnectionVM;
  hidden: boolean;
}) {
  const m = useMessages();
  const [binding, setBinding] = useState<CollaborationBindingVM>();
  const [spaces, setSpaces] = useState<RegistryPageVM<CollaborationSpaceVM>>();
  const [members, setMembers] =
    useState<RegistryPageVM<CollaborationAgentVM>>();
  const [space, setSpace] = useState('');
  const [memberId, setMemberId] = useState('');
  const [query, setQuery] = useState('');
  const [drafts, setDrafts] = useState<Record<string, MemberDraft>>({});
  const [busy, setBusy] = useState(false);
  const loaded = useRef(false);
  const [fresh, setFresh] = useState(false);
  const [bindingFresh, setBindingFresh] = useState(false);
  const [notice, setNotice] = useMessageState();
  const request = useRef(0);
  const operation = useRef(false);
  const identity = JSON.stringify([connection?.epoch, connection?.scope]);
  const current = useRef(identity);
  current.current = identity;
  const selectedSpace = useRef(space);
  selectedSpace.current = space;
  const selectedMember = useRef(memberId);
  selectedMember.current = memberId;
  useEffect(() => {
    current.current = identity;
    return () => {
      current.current = '';
      loaded.current = false;
      request.current++;
    };
  }, [identity]);
  const refresh = async (
    spaceId = space,
    spacesPage = spaces?.page ?? 1,
    membersPage = 1,
  ) => {
    if (!services || !connection || operation.current) return;
    const id = ++request.current;
    const scope = identity;
    const active = () => current.current === scope && request.current === id;
    setBusy(true);
    setFresh(false);
    setBindingFresh(false);
    setNotice('');
    setMembers(undefined);
    try {
      const before = await services.binding(connection.epoch);
      if (!active()) return;
      setBinding(before);
      setBindingFresh(true);
      const directory = await services.spaces(connection.epoch, spacesPage);
      if (!active()) return;
      const chosen = spaceId || before.spaceId || directory.items[0]?.id || '';
      const agents = chosen
        ? await services.agents(connection.epoch, chosen, membersPage)
        : undefined;
      if (!active()) return;
      const after = await services.binding(connection.epoch);
      if (!active()) return;
      setBinding(after);
      setBindingFresh(true);
      if (
        before.revision !== after.revision ||
        before.spaceId !== after.spaceId ||
        before.region !== after.region
      ) {
        setNotice(localized((m) => m.collaborationRefreshRequired()));
        return;
      }
      setSpaces(directory);
      setSpace(chosen);
      selectedSpace.current = chosen;
      setMembers(agents);
      setFresh(true);
      const selected = agents?.items.some((item) => item.id === memberId)
        ? memberId
        : agents?.items[0]?.id || '';
      setMemberId(selected);
      selectedMember.current = selected;
      if (agents)
        setDrafts((previous) => {
          const next = { ...previous };
          for (const item of agents.items) {
            const key = JSON.stringify([chosen, item.id]);
            if (!next[key]) next[key] = { alias: item.alias ?? '' };
          }
          return next;
        });
    } catch (error) {
      if (active()) setNotice(studioFailure(error));
    } finally {
      if (active()) setBusy(false);
    }
  };
  const member = members?.items.find((item) => item.id === memberId);
  const draftKey = JSON.stringify([space, memberId]);
  const draft = drafts[draftKey] ?? { alias: member?.alias ?? '' };
  const edit = (field: keyof MemberDraft, text: string) =>
    setDrafts((previous) => ({
      ...previous,
      [draftKey]: { ...(previous[draftKey] ?? draft), [field]: text },
    }));
  const writeBinding = async (spaceId: string | null) => {
    if (
      !services ||
      !connection ||
      !binding ||
      !bindingFresh ||
      busy ||
      operation.current
    )
      return;
    const scope = identity,
      revision = binding.revision;
    if (
      !window.confirm(m.confirmSpaceBinding()) ||
      current.current !== scope ||
      !bindingFresh
    )
      return;
    operation.current = true;
    setBusy(true);
    setFresh(false);
    setBindingFresh(false);
    request.current++;
    try {
      const result = await services.setBinding(
        connection.epoch,
        spaceId,
        revision,
      );
      if (current.current !== scope) return;
      setBinding(result);
      setBindingFresh(true);
      setFresh(true);
      setNotice(localized((m) => m.spaceBindingSaved()));
    } catch (error) {
      if (current.current === scope) setNotice(studioFailure(error));
    } finally {
      operation.current = false;
      if (current.current === scope) setBusy(false);
    }
  };
  const saveAlias = async () => {
    if (
      !services ||
      !connection ||
      !member ||
      !fresh ||
      busy ||
      operation.current ||
      !member.aliasAvailable
    )
      return;
    const scope = identity,
      chosen = space,
      agent = member.id;
    const revision = member.aliasRevision;
    const alias = draft.alias.trim() || null;
    if (
      !window.confirm(m.confirmMemberAlias()) ||
      current.current !== scope ||
      selectedSpace.current !== chosen ||
      selectedMember.current !== agent
    )
      return;
    operation.current = true;
    setBusy(true);
    setFresh(false);
    request.current++;
    try {
      const saved = await services.setAlias(
        connection.epoch,
        chosen,
        agent,
        alias,
        revision,
      );
      if (current.current === scope && selectedSpace.current === chosen) {
        setMembers(
          (previous) =>
            previous && {
              ...previous,
              items: previous.items.map((item) =>
                item.id === agent
                  ? {
                      ...item,
                      alias: saved.alias,
                      aliasRevision: saved.revision,
                    }
                  : item,
              ),
            },
        );
        setDrafts((previous) => ({
          ...previous,
          [JSON.stringify([chosen, agent])]: {
            ...draft,
            alias: saved.alias ?? '',
          },
        }));
        setFresh(true);
        setNotice(localized((m) => m.memberAliasSaved()));
      }
    } catch (error) {
      if (current.current === scope) setNotice(studioFailure(error));
    } finally {
      operation.current = false;
      if (current.current === scope) setBusy(false);
    }
  };
  useEffect(() => {
    if (!hidden && !loaded.current && services && connection) {
      loaded.current = true;
      void refresh();
    }
  }, [hidden, services, connection?.epoch]);
  const chooseMember = (id: string) => {
    selectedMember.current = id;
    setMemberId(id);
  };
  const memberState = (status: string) =>
    status === 'running'
      ? m.a2aMemberRunning()
      : status === 'stopped'
        ? m.a2aMemberStopped()
        : status || m.desktopUnknown();
  const filteredMembers = members?.items.filter((item) =>
    `${item.name} ${item.alias || ''} ${item.description}`
      .toLowerCase()
      .includes(query.trim().toLowerCase()),
  );
  const management = (mode: 'toolbar' | 'member') =>
    services &&
    connection && (
      <CollaborationManagement
        key={`${identity}:${mode}`}
        mode={mode}
        services={services}
        epoch={connection.epoch}
        spaceId={space}
        projectName={
          spaces?.items.find((item) => item.id === space)?.projectName ||
          'default'
        }
        boundSpaceId={binding?.spaceId ?? null}
        member={mode === 'member' ? member : undefined}
        membersTotal={members?.total}
        disabled={busy || !fresh || !spaces}
        onBusy={(value) => {
          operation.current = value;
          setBusy(value);
        }}
        onUncertain={() => {
          setFresh(false);
          setBindingFresh(false);
        }}
        onChanged={async (selected = space) => {
          operation.current = false;
          setBusy(false);
          await refresh(selected);
        }}
      />
    );
  return (
    <main
      className="main-content desktop-studio-page collaboration-page"
      hidden={hidden}
      style={hidden ? { display: 'none' } : undefined}
      data-testid="studio-collaboration"
    >
      <header className="page-heading">
        <div>
          <h1>{m.a2aCollaboration()}</h1>
          <p>{m.a2aPageDescription()}</p>
        </div>
        <button
          type="button"
          className="collaboration-button"
          disabled={busy || !services || !connection}
          onClick={() => void refresh()}
        >
          <RefreshCw
            size={16}
            className={busy ? 'collaboration-spinning' : ''}
            aria-hidden="true"
          />
          {m.refreshCollaboration()}
        </button>
      </header>
      {!connection ? (
        <p>{m.connectRuntimeCollaboration()}</p>
      ) : !services ? (
        <p>{m.capabilityUnavailable()}</p>
      ) : (
        <>
          {notice && (
            <p className="collaboration-notice" role="status">
              {notice}
            </p>
          )}
          {!fresh && !busy && (
            <p className="collaboration-hint">
              {m.collaborationRefreshRequired()}
            </p>
          )}
          <section
            className="collaboration-binding"
            aria-label={m.currentBinding()}
            aria-busy={busy}
          >
            <div className="collaboration-binding-current">
              <span className="collaboration-binding-icon">
                <Link2 size={20} aria-hidden="true" />
              </span>
              <div>
                <span className="collaboration-label">
                  {m.currentBinding()}
                </span>
                <strong>
                  {!binding
                    ? m.bindingNotLoaded()
                    : binding.spaceName ||
                      binding.spaceId ||
                      m.noBoundA2aSpace()}
                </strong>
                {binding?.spaceId && <code>{binding.spaceId}</code>}
              </div>
              <button
                type="button"
                className="collaboration-button is-quiet"
                disabled={busy || !bindingFresh || !binding?.spaceId}
                onClick={() => void writeBinding(null)}
              >
                <Unlink size={14} aria-hidden="true" />
                {m.unbindSpace()}
              </button>
            </div>
            {spaces ? (
              <div className="collaboration-space-picker">
                <label>
                  <span>{m.collaborationSpace()}</span>
                  <select
                    data-testid="collaboration-space"
                    value={space}
                    disabled={busy || (!spaces.items.length && !space)}
                    onChange={(event) => {
                      selectedSpace.current = event.target.value;
                      setSpace(event.target.value);
                      chooseMember('');
                      setQuery('');
                      setFresh(false);
                      void refresh(event.target.value, spaces.page);
                    }}
                  >
                    {!space && (
                      <option value="">{m.noVisibleA2aSpaces()}</option>
                    )}
                    {space &&
                      !spaces.items.some((item) => item.id === space) && (
                        <option value={space}>
                          {binding?.spaceId === space
                            ? binding.spaceName || space
                            : space}
                        </option>
                      )}
                    {spaces.items.map((item) => (
                      <option key={item.id} value={item.id}>
                        {item.name || item.id}
                      </option>
                    ))}
                  </select>
                </label>
                <button
                  type="button"
                  className="collaboration-button is-primary"
                  disabled={
                    busy ||
                    !fresh ||
                    !space ||
                    binding?.spaceId === space ||
                    !spaces.items.some((item) => item.id === space)
                  }
                  onClick={() => void writeBinding(space)}
                >
                  <Link2 size={15} aria-hidden="true" />
                  {binding?.spaceId === space
                    ? m.a2aAlreadyBound()
                    : m.bindSelectedSpace()}
                </button>
                {spaces.total > spaces.pageSize && (
                  <div className="collaboration-pagination">
                    <span>
                      {m.usagePage()} {spaces.page}
                    </span>
                    <button
                      type="button"
                      className="collaboration-button"
                      disabled={busy || spaces.page <= 1}
                      onClick={() => void refresh('', spaces.page - 1)}
                    >
                      {m.previousSpacesPage()}
                    </button>
                    <button
                      type="button"
                      className="collaboration-button"
                      disabled={
                        busy || spaces.page * spaces.pageSize >= spaces.total
                      }
                      onClick={() => void refresh('', spaces.page + 1)}
                    >
                      {m.nextSpacesPage()}
                    </button>
                  </div>
                )}
              </div>
            ) : (
              <p className="collaboration-hint">
                {m.refreshCollaborationSpaces()}
              </p>
            )}
          </section>
          {management('toolbar')}
          <div className="collaboration-workspace" aria-busy={busy}>
            <section
              className="collaboration-members"
              aria-label={m.spaceMembers()}
            >
              <div className="collaboration-section-heading">
                <h2>
                  <Users size={17} aria-hidden="true" />
                  {m.spaceMembers()}
                </h2>
                <span className="collaboration-count">
                  {members?.total ?? 0}
                </span>
              </div>
              <label className="collaboration-search">
                <Search size={15} aria-hidden="true" />
                <input
                  placeholder={m.a2aSearchMembers()}
                  aria-label={m.a2aSearchMembers()}
                  value={query}
                  onChange={(event) => setQuery(event.target.value)}
                />
              </label>
              <div className="collaboration-member-list">
                {busy ? (
                  <p className="collaboration-empty" role="status">
                    {m.a2aLoadingMembers()}
                  </p>
                ) : !members ? (
                  <p className="collaboration-empty">
                    {m.selectSpaceMembers()}
                  </p>
                ) : !members.items.length ? (
                  <div className="collaboration-empty">
                    <Bot size={30} aria-hidden="true" />
                    <p>{m.noVisibleSpaceMembers()}</p>
                  </div>
                ) : !filteredMembers?.length ? (
                  <p className="collaboration-empty">
                    {m.a2aNoMatchingMembers()}
                  </p>
                ) : (
                  filteredMembers.map((item) => (
                    <button
                      type="button"
                      key={item.id}
                      data-testid={`collaboration-member-${item.id}`}
                      className={`collaboration-member ${item.id === memberId ? 'is-selected' : ''}`}
                      aria-pressed={item.id === memberId}
                      disabled={busy}
                      onClick={() => chooseMember(item.id)}
                    >
                      <span className="collaboration-avatar">
                        <Bot size={20} aria-hidden="true" />
                      </span>
                      <span className="collaboration-member-copy">
                        <strong>{item.name || item.id}</strong>
                        <span>{item.alias ? `@${item.alias}` : item.id}</span>
                        <span
                          className={`collaboration-status ${item.status === 'running' ? 'is-running' : ''}`}
                        >
                          {memberState(item.status)}
                        </span>
                      </span>
                    </button>
                  ))
                )}
              </div>
              {members && members.total > members.pageSize && (
                <div className="collaboration-pagination">
                  <button
                    type="button"
                    className="collaboration-button"
                    disabled={busy || members.page <= 1}
                    onClick={() =>
                      void refresh(space, spaces?.page, members.page - 1)
                    }
                  >
                    {m.previousMembersPage()}
                  </button>
                  <span>{members.page}</span>
                  <button
                    type="button"
                    className="collaboration-button"
                    disabled={
                      busy || members.page * members.pageSize >= members.total
                    }
                    onClick={() =>
                      void refresh(space, spaces?.page, members.page + 1)
                    }
                  >
                    {m.nextMembersPage()}
                  </button>
                </div>
              )}
            </section>
            <section
              className="collaboration-details"
              aria-label={m.memberDetails()}
            >
              {member ? (
                <article data-testid="collaboration-member-details">
                  <header className="collaboration-member-header">
                    <span className="collaboration-avatar is-large">
                      <Bot size={25} aria-hidden="true" />
                    </span>
                    <div>
                      <h2>{member.name || member.id}</h2>
                      <code>{member.id}</code>
                    </div>
                    <span
                      className={`collaboration-status ${member.status === 'running' ? 'is-running' : ''}`}
                    >
                      {memberState(member.status)}
                    </span>
                  </header>
                  {member.description && (
                    <p className="collaboration-description">
                      {member.description}
                    </p>
                  )}
                  {member.card && (
                    <div className="collaboration-card-info">
                      <strong>{member.card.name}</strong>
                      {member.card.description &&
                        member.card.description !== member.description && (
                          <p>{member.card.description}</p>
                        )}
                      <span>
                        {member.card.version} {member.card.protocolVersion}
                      </span>
                    </div>
                  )}
                  {management('member')}
                  <div className="collaboration-alias-form">
                    <div>
                      <h3>{m.memberAlias()}</h3>
                      <p>{m.removeAliasHint()}</p>
                    </div>
                    {member.aliasAvailable ? (
                      <div className="collaboration-inline-field">
                        <input
                          data-testid="collaboration-alias"
                          aria-label={m.memberAlias()}
                          value={draft.alias}
                          onChange={(event) =>
                            edit('alias', event.target.value)
                          }
                        />
                        <button
                          type="button"
                          className="collaboration-button"
                          disabled={busy || !fresh}
                          onClick={() => void saveAlias()}
                        >
                          {m.saveMemberAlias()}
                        </button>
                      </div>
                    ) : (
                      <p>{m.aliasesUnavailable()}</p>
                    )}
                  </div>
                </article>
              ) : (
                <div className="collaboration-empty is-detail">
                  <Users size={34} aria-hidden="true" />
                  <h2>{m.a2aChooseMemberTitle()}</h2>
                  <p>{m.selectSpaceMembers()}</p>
                </div>
              )}
            </section>
          </div>
          <p className="collaboration-footer">{m.a2aManageResources()}</p>
        </>
      )}
    </main>
  );
}
