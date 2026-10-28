import { useEffect, useRef, useState } from 'react';
import { ModalDialog } from '../ui/ModalDialog';
import { Plus, UserPlus, Info, Trash2 } from 'lucide-react';
import type {
  CollaborationServices,
  CollaborationAgentVM,
  CollaborationDetailVM,
  CollaborationRegistration,
} from './studio-models';
import { useMessages, useMessageState, localized } from './locale';
import { studioFailure } from './studio-services';

export function CollaborationManagement({
  services,
  mode = 'all',
  epoch,
  spaceId,
  projectName,
  boundSpaceId,
  member,
  membersTotal,
  disabled,
  onBusy,
  onUncertain,
  onChanged,
}: {
  services: CollaborationServices;
  mode?: 'all' | 'toolbar' | 'member';
  epoch: number;
  spaceId: string;
  projectName: string;
  boundSpaceId: string | null;
  member?: CollaborationAgentVM;
  membersTotal?: number;
  disabled: boolean;
  onBusy(value: boolean): void;
  onUncertain(): void;
  onChanged(spaceId?: string): Promise<void>;
}) {
  const m = useMessages();
  const [form, setForm] = useState<'space' | 'agent'>();
  const [name, setName] = useState('');
  const [project, setProject] = useState(projectName);
  const [intent, setIntent] = useState(false);
  const [source, setSource] = useState<'Runtime' | 'url' | 'json'>('Runtime');
  const [runtimeId, setRuntimeId] = useState('');
  const [network, setNetwork] = useState('public');
  const [card, setCard] = useState('');
  const [runtimes, setRuntimes] = useState<Array<{ id: string; name: string }>>(
    [],
  );
  const [nextToken, setNextToken] = useState('');
  const [endpoints, setEndpoints] = useState<
    Array<{ networkType: string; url: string }>
  >([]);
  const [listing, setListing] = useState(false);
  const [detail, setDetail] = useState<CollaborationDetailVM>();
  const [notice, setNotice] = useMessageState();
  const [busy, setBusy] = useState(false);
  const [uncertain, setUncertain] = useState(false);
  const alive = useRef(true);
  const operation = useRef(false);
  const listingGeneration = useRef(0);
  const endpointGeneration = useRef(0);
  const detailGeneration = useRef(0);
  useEffect(() => {
    alive.current = true;
    return () => {
      alive.current = false;
      listingGeneration.current++;
      endpointGeneration.current++;
      detailGeneration.current++;
    };
  }, []);
  useEffect(() => {
    setDetail(undefined);
    detailGeneration.current++;
  }, [spaceId, member?.id]);
  useEffect(() => {
    setForm(undefined);
    setCard('');
    setRuntimeId('');
  }, [spaceId]);
  const perform = async (work: () => Promise<void>) => {
    if (operation.current || disabled || uncertain) return;
    operation.current = true;
    setBusy(true);
    onBusy(true);
    setNotice('');
    try {
      await work();
    } catch (error) {
      if (alive.current) {
        setNotice(studioFailure(error));
        setUncertain(true);
        onUncertain();
      }
    } finally {
      operation.current = false;
      if (alive.current) {
        setBusy(false);
        onBusy(false);
      }
    }
  };
  const loadRuntimes = async (token = '') => {
    if (!services.runtimes) return;
    if (!token) setRuntimeId('');
    const id = ++listingGeneration.current;
    setListing(true);
    setNotice('');
    try {
      const result = await services.runtimes(epoch, token);
      if (!alive.current || id !== listingGeneration.current) return;
      setRuntimes((previous) =>
        token
          ? [
              ...previous,
              ...result.items.filter(
                (item) => !previous.some((existing) => existing.id === item.id),
              ),
            ]
          : result.items,
      );
      setNextToken(result.nextToken);
    } catch (error) {
      if (alive.current && id === listingGeneration.current)
        setNotice(studioFailure(error));
    } finally {
      if (alive.current && id === listingGeneration.current) setListing(false);
    }
  };
  useEffect(() => {
    const id = ++endpointGeneration.current;
    setEndpoints([]);
    if (!runtimeId || !services.runtimeEndpoints) return;
    void services
      .runtimeEndpoints(epoch, runtimeId)
      .then((result) => {
        if (alive.current && id === endpointGeneration.current) {
          setEndpoints(result.endpoints);
          setNetwork(result.endpoints[0]?.networkType || 'public');
        }
      })
      .catch((error) => {
        if (alive.current && id === endpointGeneration.current)
          setNotice(studioFailure(error));
      });
  }, [runtimeId, epoch, services]);
  const open = (value: 'space' | 'agent') => {
    setForm(value);
    setNotice('');
    setProject(projectName);
    if (value === 'agent' && runtimes.length === 0) void loadRuntimes();
  };
  const blocked = disabled || busy || uncertain;
  return (
    <section
      className={`collaboration-management is-${mode}`}
      aria-label={
        mode === 'member' ? m.memberDetails() : m.a2aManageResources()
      }
    >
      {mode === 'all' && <h2>{m.a2aManageResources()}</h2>}
      {notice && <p role="status">{notice}</p>}
      {uncertain && (
        <div role="alert">
          <p>{m.collaborationRefreshRequired()}</p>
          <button
            type="button"
            className="text-button"
            disabled={busy}
            onClick={() => {
              setUncertain(false);
              setForm(undefined);
              void onChanged();
            }}
          >
            {m.refreshCollaboration()}
          </button>
        </div>
      )}
      {mode !== 'member' && (
        <div className="studio-actions">
          {services.createSpace && (
            <button
              type="button"
              className="text-button"
              disabled={blocked}
              onClick={() => open('space')}
            >
              <Plus size={15} aria-hidden="true" />
              {m.a2aCreateSpace()}
            </button>
          )}
          {services.registerAgent && (
            <button
              type="button"
              className="text-button"
              disabled={blocked || !spaceId}
              onClick={() => open('agent')}
            >
              <UserPlus size={15} aria-hidden="true" />
              {m.a2aRegisterAgent()}
            </button>
          )}
          {services.deleteSpace && (
            <button
              type="button"
              className="text-button"
              disabled={
                blocked ||
                !spaceId ||
                boundSpaceId === spaceId ||
                membersTotal !== 0
              }
              onClick={() => {
                if (window.confirm(m.a2aDeleteSpaceConfirm()))
                  void perform(async () => {
                    await services.deleteSpace!(epoch, spaceId);
                    if (alive.current) {
                      setNotice(localized((m) => m.a2aResourceDeleted()));
                      await onChanged('');
                    }
                  });
              }}
            >
              {m.a2aDeleteSpace()}
            </button>
          )}
        </div>
      )}
      {form && (
        <ModalDialog
          title={form === 'space' ? m.a2aCreateSpace() : m.a2aRegisterAgent()}
          closeLabel={m.closeDialog(
            form === 'space' ? m.a2aCreateSpace() : m.a2aRegisterAgent(),
          )}
          onClose={() => {
            if (!busy) setForm(undefined);
          }}
        >
          <form
            className="desktop-form collaboration-dialog-form"
            onSubmit={(event) => {
              event.preventDefault();
              void perform(async () => {
                if (form === 'space') {
                  const result = await services.createSpace!(
                    epoch,
                    name.trim(),
                    project.trim(),
                    intent,
                  );
                  if (alive.current) {
                    setForm(undefined);
                    setName('');
                    await onChanged(result.id);
                  }
                } else {
                  const registration: CollaborationRegistration =
                    source === 'Runtime'
                      ? { source: 'Runtime', runtimeId, networkType: network }
                      : source === 'url'
                        ? {
                            source: 'Standard',
                            importType: 'url',
                            cardUrl: card.trim(),
                          }
                        : {
                            source: 'Standard',
                            importType: 'json',
                            agentCard: card,
                          };
                  const result = await services.registerAgent!(
                    epoch,
                    spaceId,
                    project.trim(),
                    registration,
                  );
                  if (alive.current) {
                    setForm(undefined);
                    setCard('');
                    setNotice(localized((m) => m.a2aRegistered(result.id)));
                    await onChanged(spaceId);
                  }
                }
              });
            }}
          >
            {notice && <p role="alert">{notice}</p>}
            {uncertain && <p>{m.collaborationRefreshRequired()}</p>}
            {form === 'space' && (
              <>
                <label>
                  {m.a2aSpaceName()}
                  <input
                    autoFocus
                    required
                    maxLength={128}
                    value={name}
                    disabled={busy}
                    onChange={(e) => setName(e.target.value)}
                  />
                </label>
                <label className="studio-checkbox">
                  <input
                    type="checkbox"
                    checked={intent}
                    disabled={busy}
                    onChange={(e) => setIntent(e.target.checked)}
                  />
                  {m.a2aIntentEnabled()}
                </label>
              </>
            )}
            <label>
              {m.a2aProject()}
              <input
                required
                maxLength={128}
                value={project}
                disabled={busy}
                onChange={(e) => setProject(e.target.value)}
              />
            </label>
            {form === 'agent' && (
              <>
                <label>
                  {m.a2aRegistrationSource()}
                  <select
                    value={source}
                    disabled={busy}
                    onChange={(e) => {
                      setSource(e.target.value as typeof source);
                      setCard('');
                    }}
                  >
                    <option value="Runtime">Runtime</option>
                    <option value="url">{m.a2aCardUrl()}</option>
                    <option value="json">{m.a2aCardJson()}</option>
                  </select>
                </label>
                {source === 'Runtime' ? (
                  <>
                    <label>
                      {m.a2aSelectRuntime()}
                      <select
                        value={runtimeId}
                        required
                        disabled={busy || listing}
                        onChange={(e) => setRuntimeId(e.target.value)}
                      >
                        <option value="">{m.a2aRuntimePlaceholder()}</option>
                        {runtimes.map((item) => (
                          <option key={item.id} value={item.id}>
                            {item.name || item.id}
                          </option>
                        ))}
                      </select>
                    </label>
                    <button
                      type="button"
                      className="text-button"
                      disabled={busy || listing}
                      onClick={() => void loadRuntimes()}
                    >
                      {m.desktopRefresh()}
                    </button>
                    {nextToken && (
                      <button
                        type="button"
                        className="text-button"
                        disabled={busy || listing}
                        onClick={() => void loadRuntimes(nextToken)}
                      >
                        {m.a2aMoreRuntimes()}
                      </button>
                    )}
                    <label>
                      {m.a2aNetwork()}
                      <select
                        value={network}
                        required
                        disabled={busy || !endpoints.length}
                        onChange={(e) => setNetwork(e.target.value)}
                      >
                        {endpoints.map((ep) => (
                          <option key={ep.networkType} value={ep.networkType}>
                            {ep.networkType}
                          </option>
                        ))}
                      </select>
                    </label>
                    {runtimeId && !endpoints.length && (
                      <p>{m.a2aEndpointUnavailable()}</p>
                    )}
                    {endpoints.find((ep) => ep.networkType === network)
                      ?.url && (
                      <p className="studio-resource-address">
                        {
                          endpoints.find((ep) => ep.networkType === network)!
                            .url
                        }
                      </p>
                    )}
                  </>
                ) : source === 'url' ? (
                  <label>
                    {m.a2aCardUrl()}
                    <input
                      type="url"
                      required
                      placeholder="https://"
                      value={card}
                      disabled={busy}
                      onChange={(e) => setCard(e.target.value)}
                    />
                  </label>
                ) : (
                  <label>
                    {m.a2aCardJson()}
                    <textarea
                      required
                      maxLength={131072}
                      spellCheck={false}
                      value={card}
                      disabled={busy}
                      onChange={(e) => setCard(e.target.value)}
                    />
                  </label>
                )}
              </>
            )}
            <div className="studio-actions">
              <button
                type="submit"
                className="primary"
                disabled={
                  blocked ||
                  (form === 'agent' &&
                    source === 'Runtime' &&
                    !endpoints.some((ep) => ep.networkType === network))
                }
              >
                {m.a2aSubmit()}
              </button>
              <button
                type="button"
                className="text-button"
                disabled={busy}
                onClick={() => setForm(undefined)}
              >
                {m.cancel()}
              </button>
            </div>
          </form>
        </ModalDialog>
      )}
      {member && mode !== 'toolbar' && (
        <div className="desktop-form">
          {mode === 'all' && <h3>{member.name}</h3>}
          <div className="studio-actions">
            {services.agentDetail && (
              <button
                type="button"
                className="text-button"
                disabled={blocked}
                onClick={() => {
                  const id = ++detailGeneration.current;
                  void services.agentDetail!(epoch, spaceId, member.id)
                    .then((value) => {
                      if (alive.current && id === detailGeneration.current)
                        setDetail(value);
                    })
                    .catch((error) => {
                      if (alive.current && id === detailGeneration.current)
                        setNotice(studioFailure(error));
                    });
                }}
              >
                <Info size={14} aria-hidden="true" />
                {m.a2aViewDetails()}
              </button>
            )}
            {services.deleteAgent && (
              <button
                type="button"
                className="text-button"
                disabled={blocked}
                onClick={() => {
                  if (window.confirm(m.a2aUnregisterConfirm()))
                    void perform(async () => {
                      await services.deleteAgent!(epoch, spaceId, member.id);
                      if (alive.current) {
                        setDetail(undefined);
                        setNotice(localized((m) => m.a2aResourceDeleted()));
                        await onChanged(spaceId);
                      }
                    });
                }}
              >
                <Trash2 size={14} aria-hidden="true" />
                {m.a2aUnregister()}
              </button>
            )}
          </div>
          {detail && (
            <ModalDialog
              title={detail.name}
              closeLabel={m.closeDialog(detail.name)}
              onClose={() => setDetail(undefined)}
            >
              <article className="studio-member-detail">
                <h3>{detail.name}</h3>
                <p>
                  {detail.source} · {detail.status} · {detail.defaultVersion}
                </p>
                {detail.runtimeId && (
                  <p>
                    {detail.runtimeId} · {detail.networkType}
                  </p>
                )}
                {detail.serviceAddress && (
                  <p className="studio-resource-address">
                    {detail.serviceAddress}
                  </p>
                )}
                {detail.skills === null ? (
                  <p>{m.agentCardUnavailable()}</p>
                ) : (
                  detail.skills.map((skill) => (
                    <div key={skill.id || skill.name}>
                      <h4>{skill.name}</h4>
                      <p>{skill.description}</p>
                      {skill.tags?.length > 0 && (
                        <p>{skill.tags.join(' · ')}</p>
                      )}
                      {skill.examples?.map((example, index) => (
                        <p key={index}>{example}</p>
                      ))}
                    </div>
                  ))
                )}
                {detail.skillsTruncated && <p>{m.a2aSkillsTruncated()}</p>}
              </article>
            </ModalDialog>
          )}
        </div>
      )}
    </section>
  );
}
