import { localized, useMessages, useMessageState } from './locale';
import { useEffect, useRef, useState } from 'react';
import { FileText, Plus, Search, Upload } from 'lucide-react';
import { Knowledge } from './generated/mygo';
import { unwrap } from './result';
import type { ConnectionVM, SessionVM } from './models';
type Item = NonNullable<Awaited<ReturnType<typeof Knowledge.read>>['value']>;
type SearchPage = NonNullable<
  Awaited<ReturnType<typeof Knowledge.search>>['value']
>;
type ContentPage = NonNullable<
  Awaited<ReturnType<typeof Knowledge.content>>['value']
>;
type Receipt = NonNullable<
  Awaited<ReturnType<typeof Knowledge.importText>>['value']
>;
type PendingImport = {
  mode: 'text' | 'file';
  version: boolean;
  input: {
    connectionEpoch: number;
    sessionId: string;
    knowledgeId?: string;
    idempotencyKey: string;
    displayName: string;
    content?: string;
  };
};
const scopeRequired = localized((m) => m.desktopChooseARuntimeConversation());
const itemRequired = localized((m) => m.desktopChooseAnItemForTheNewVersion());
export function DesktopKnowledge({
  connection,
  sessions,
}: {
  connection?: ConnectionVM;
  sessions: SessionVM[];
}) {
  const m = useMessages();
  const [sessionId, setSessionId] = useState('');
  const [items, setItems] = useState<Item[]>([]);
  const [selected, setSelected] = useState<Item>();
  const [name, setName] = useState('');
  const [text, setText] = useState('');
  const [query, setQuery] = useState('');
  const [sources, setSources] = useState('');
  const [updatedAfter, setUpdatedAfter] = useState('');
  const [search, setSearch] = useState<SearchPage>();
  const [content, setContent] = useState<ContentPage>();
  const [receipt, setReceipt] = useState<Receipt>();
  const [pending, setPending] = useState<PendingImport>();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useMessageState();
  const statusLabel = (status: string) => {
    const labels: Record<string, string> = {
      received: m.knowledgeReceived(),
      pending: m.pagePending(),
      processing: m.knowledgeProcessing(),
      ready: m.knowledgeReady(),
      failed: m.desktopFailed(),
      deleted: m.knowledgeDeleted(),
      complete: m.pageCompleted(),
      bounded: m.knowledgeBounded(),
      partial: m.knowledgePartial(),
    };
    const key = status.toLowerCase();
    return Object.hasOwn(labels, key) ? labels[key] : status;
  };
  const generation = useRef(0);
  const pendingImports = useRef(new Map<string, PendingImport>());
  const scopeKey = `${connection?.epoch}:${sessionId}`;
  const scope = () => {
    if (!connection || !sessionId) throw scopeRequired;
    return { connectionEpoch: connection.epoch, sessionId };
  };
  const run = async (action: (current: () => boolean) => Promise<void>) => {
    const current = generation.current;
    const matches = () => generation.current === current;
    setError('');
    setBusy(true);
    try {
      await action(matches);
    } catch (error) {
      if (matches())
        setError(
          error instanceof Error
            ? error.message
            : error === scopeRequired
              ? scopeRequired
              : error === itemRequired
                ? itemRequired
                : localized((m) => m.desktopKnowledgeOperationFailed()),
        );
    } finally {
      if (matches()) setBusy(false);
    }
  };
  const load = async (matches: () => boolean) => {
    const value = unwrap(await Knowledge.list(scope()));
    if (matches()) setItems(value.items);
  };
  useEffect(() => {
    const current = ++generation.current;
    setItems([]);
    setSelected(undefined);
    setContent(undefined);
    setSearch(undefined);
    setReceipt(undefined);
    setPending(pendingImports.current.get(scopeKey));
    setBusy(false);
    if (connection && sessionId)
      void run(async () => load(() => generation.current === current));
    return () => {
      generation.current++;
    };
  }, [connection?.epoch, sessionId]);
  const submit = async (input: PendingImport, matches: () => boolean) => {
    let value: Receipt;
    try {
      value = unwrap(
        await (input.mode === 'text'
          ? input.version
            ? Knowledge.addTextVersion({
                ...input.input,
                content: input.input.content || '',
              })
            : Knowledge.importText({
                ...input.input,
                content: input.input.content || '',
              })
          : input.version
            ? Knowledge.addFileVersion(input.input)
            : Knowledge.importFile(input.input)),
      );
    } catch (error) {
      const safe =
        error &&
        typeof error === 'object' &&
        (('httpStatus' in error &&
          [400, 401, 403, 404, 409, 412, 422].includes(
            Number(error.httpStatus),
          )) ||
          ('code' in error &&
            ['cancelled', 'invalidInput', 'fileTooLarge'].includes(
              String(error.code),
            ))) &&
        !(
          'retryDisposition' in error && error.retryDisposition === 'verifyOnly'
        );
      if (safe) {
        pendingImports.current.delete(scopeKey);
        if (matches()) setPending(undefined);
      }
      throw error;
    }
    if (matches()) {
      setReceipt(value);
      pendingImports.current.delete(scopeKey);
      setPending(undefined);
      setName('');
      setText('');
    }
    await load(matches);
  };
  const importKnowledge = (mode: 'text' | 'file', version: boolean) =>
    void run(async (matches) => {
      if (pending) {
        const key = pending.input.idempotencyKey;
        setError(localized((m) => m.knowledgeImportUnknown(key)));
        return;
      }
      if (version && !selected) throw itemRequired;
      const input: PendingImport = {
        mode,
        version,
        input: {
          ...scope(),
          knowledgeId: version ? selected?.knowledgeId : undefined,
          idempotencyKey: crypto.randomUUID(),
          displayName: name.trim(),
          ...(mode === 'text' ? { content: text } : {}),
        },
      };
      pendingImports.current.set(scopeKey, input);
      setPending(input);
      await submit(input, matches);
    });
  return (
    <main className="main-content">
      <div className="page-heading">
        <div>
          <span className="eyebrow">{m.desktopSESSIONKNOWLEDGE()}</span>
          <h1>{m.desktopReferenceMaterialWithProvenance()}</h1>
          <p>
            {m.desktopImportedKnowledgeBelongsToTheSelectedConversationItDoes()}
          </p>
        </div>
      </div>
      <label>
        {m.conversation()}
        <select
          value={sessionId}
          onChange={(event) => setSessionId(event.target.value)}
        >
          <option value="">{m.chooseConversation()}</option>
          {sessions.map((session) => (
            <option key={session.id} value={session.id}>
              {session.title}
            </option>
          ))}
        </select>
      </label>
      {error && (
        <div className="error-banner" role="alert">
          {error}
        </div>
      )}
      {!connection && (
        <div className="setup-card">
          {m.desktopConnectARuntimeToAccessSessionKnowledge()}
        </div>
      )}
      {connection && !sessionId && (
        <div className="setup-card">
          {m.desktopChooseARuntimeConversation()}
        </div>
      )}
      {connection && sessionId && (
        <>
          <section className="task-detail-card desktop-form">
            <div className="desktop-inline">
              <h2>{m.desktopKnowledgeLibrary()}</h2>
              <button
                className="text-button"
                disabled={busy}
                onClick={() => void run(load)}
              >
                {m.desktopRefreshLibrary()}
              </button>
            </div>
            <div className="task-list">
              {!items.length && !busy && <p>{m.knowledgeNoItems()}</p>}
              {items.map((item) => (
                <button
                  className="task-row"
                  key={`${item.knowledgeId}:${item.version}`}
                  onClick={() =>
                    void run(async (matches) => {
                      const value = unwrap(
                        await Knowledge.read({
                          ...scope(),
                          knowledgeId: item.knowledgeId,
                          version: item.version,
                        }),
                      );
                      if (matches()) {
                        setSelected(value);
                        setContent(undefined);
                      }
                    })
                  }
                >
                  <FileText size={18} />
                  <div>
                    <strong>{item.displayName}</strong>
                    <span>
                      {m.desktopVersionInline()} {item.version} ·{' '}
                      {statusLabel(item.status)} · {item.sourceType}
                    </span>
                  </div>
                </button>
              ))}
            </div>
          </section>
          <section className="task-detail-card desktop-form">
            <h2>{m.desktopImportMaterial()}</h2>
            <label>
              {m.desktopDisplayName()}
              <input
                value={name}
                onChange={(event) => setName(event.target.value)}
              />
            </label>
            <label>
              {m.desktopTextMaterial()}
              <textarea
                value={text}
                onChange={(event) => setText(event.target.value)}
              />
            </label>
            <div className="desktop-inline">
              <button
                className="primary"
                disabled={busy || !name.trim() || !text.trim() || !!pending}
                onClick={() => importKnowledge('text', false)}
              >
                <Plus size={14} />
                {m.desktopImportText()}
              </button>
              <button
                className="text-button"
                disabled={busy || !!pending}
                onClick={() => importKnowledge('file', false)}
              >
                <Upload size={14} />
                {m.desktopChooseFileImport()}
              </button>
              <button
                className="text-button"
                disabled={busy || !selected || !text.trim() || !!pending}
                onClick={() => importKnowledge('text', true)}
              >
                {m.desktopAddTextVersionToSelectedItem()}
              </button>
              <button
                className="text-button"
                disabled={busy || !selected || !!pending}
                onClick={() => importKnowledge('file', true)}
              >
                {m.desktopChooseFileForANewVersion()}
              </button>
            </div>
            <p>{m.desktopFileSelectionIsProvidedByTheNativeHostAn()}</p>
            {pending && (
              <div className="document-save-notice">
                <p>
                  {m.desktopPendingImportKey()} {pending.input.idempotencyKey}
                  {m.desktopTheOriginal()}{' '}
                  {pending.mode === 'text'
                    ? m.knowledgeText()
                    : m.knowledgeFile()}{' '}
                  {m.desktopSubmissionIsRetainedVerifyTheLibraryAndImportStatus()}
                </p>
                <button
                  disabled={busy}
                  onClick={() => {
                    if (window.confirm(m.knowledgeRecoverConfirm()))
                      void run((matches) => submit(pending, matches));
                  }}
                >
                  {m.desktopRecoverOriginalImport()}
                </button>
                <button
                  disabled={busy}
                  onClick={() => {
                    if (window.confirm(m.knowledgeDiscardConfirm())) {
                      pendingImports.current.delete(scopeKey);
                      setPending(undefined);
                    }
                  }}
                >
                  {m.desktopDiscardLocalRecoveryAfterReview()}
                </button>
              </div>
            )}
            {receipt && (
              <div role="status">
                <p>
                  {m.desktopImport()} {receipt.importId} {m.desktopKnowledge()}{' '}
                  {receipt.knowledgeId} {m.desktopVersionInlineLabel()}{' '}
                  {receipt.version} · {statusLabel(receipt.status)}
                </p>
                <button
                  className="text-button"
                  disabled={busy}
                  onClick={() =>
                    void run(async (matches) => {
                      const value = unwrap(
                        await Knowledge.importStatus({
                          ...scope(),
                          importId: receipt.importId,
                        }),
                      );
                      if (matches()) setSelected(value);
                      await load(matches);
                    })
                  }
                >
                  {m.desktopReadAuthoritativeImportStatus()}
                </button>
              </div>
            )}
          </section>
          {selected && (
            <section className="task-detail-card desktop-form">
              <h2>{selected.displayName}</h2>
              <p>
                {m.knowledge()} {selected.knowledgeId}{' '}
                {m.desktopVersionInlineLabel()} {selected.version} ·{' '}
                {statusLabel(selected.status)} · {selected.chunkCount}{' '}
                {m.desktopChunks()}
              </p>
              {selected.safeMessage && (
                <p role="status">{selected.safeMessage}</p>
              )}
              <div className="desktop-inline">
                <button
                  className="text-button"
                  disabled={busy}
                  onClick={() =>
                    void run(async (matches) => {
                      const value = unwrap(
                        await Knowledge.content({
                          ...scope(),
                          knowledgeId: selected.knowledgeId,
                          version: selected.version,
                          limit: 65536,
                        }),
                      );
                      if (matches()) setContent(value);
                    })
                  }
                >
                  {m.desktopReadSelectedContent()}
                </button>
                <button
                  className="text-button"
                  disabled={busy}
                  onClick={() => {
                    if (
                      window.confirm(
                        m.knowledgeDeleteConfirm(selected.knowledgeId),
                      )
                    )
                      void run(async (matches) => {
                        unwrap(
                          await Knowledge.delete({
                            ...scope(),
                            knowledgeId: selected.knowledgeId,
                            version: selected.version,
                          }),
                        );
                        if (matches()) {
                          setSelected(undefined);
                          setContent(undefined);
                        }
                        await load(matches);
                      });
                  }}
                >
                  {m.desktopDeleteKnowledge()}
                </button>
              </div>
              {content && (
                <>
                  <pre className="desktop-content">{content.content}</pre>
                  <p>
                    {statusLabel(content.completeness)} {m.desktopCitation()}{' '}
                    {content.citation.ref} · {content.visibleBytes}{' '}
                    {m.desktopBytesVisible()}
                  </p>
                  {content.nextCursor && (
                    <button
                      className="text-button"
                      disabled={busy}
                      onClick={() =>
                        void run(async (matches) => {
                          const next = unwrap(
                            await Knowledge.content({
                              ...scope(),
                              knowledgeId: content.knowledgeId,
                              version: content.version,
                              cursor: content.nextCursor || undefined,
                              limit: 65536,
                            }),
                          );
                          if (matches())
                            setContent({
                              ...next,
                              content: content.content + next.content,
                            });
                        })
                      }
                    >
                      {m.desktopReadMoreContent()}
                    </button>
                  )}
                </>
              )}
            </section>
          )}
          <form
            className="task-detail-card desktop-form"
            onSubmit={(event) => {
              event.preventDefault();
              void run(async (matches) => {
                const result = unwrap(
                  await Knowledge.search({
                    ...scope(),
                    query,
                    knowledgeIds: selected ? [selected.knowledgeId] : [],
                    sourceTypes: sources
                      .split(',')
                      .map((source) => source.trim())
                      .filter(Boolean),
                    updatedAfter: updatedAfter || undefined,
                    limit: 20,
                  }),
                );
                if (matches()) setSearch(result);
              });
            }}
          >
            <h2>{m.desktopSearchReferenceMaterial()}</h2>
            <label>
              {m.desktopSearchQuery()}
              <input
                required
                value={query}
                onChange={(event) => setQuery(event.target.value)}
              />
            </label>
            <label>
              {m.desktopSourceTypesOptionalCommaSeparated()}
              <input
                value={sources}
                onChange={(event) => setSources(event.target.value)}
              />
            </label>
            <label>
              {m.desktopUpdatedAfterOptionalRFC3339()}
              <input
                value={updatedAfter}
                onChange={(event) => setUpdatedAfter(event.target.value)}
              />
            </label>
            <p>
              {selected
                ? m.knowledgeSearchingItem(selected.knowledgeId)
                : m.knowledgeSearchingConversation()}
            </p>
            <button className="primary" disabled={busy}>
              <Search size={14} />
              {m.desktopSearch()}
            </button>
            {search && (
              <>
                <p>
                  {statusLabel(search.completeness)}
                  {search.boundedReason ? ` · ${search.boundedReason}` : ''}
                </p>
                {!search.items.length && <p>{m.knowledgeNoMatches()}</p>}
                {search.items.map((item) => (
                  <article key={item.ref}>
                    <strong>{item.displayName}</strong>
                    <p>{item.content}</p>
                    <small>
                      {m.desktopCitationInline()} {item.citation.ref}{' '}
                      {m.desktopVersionInlineLabel()} {item.version}
                    </small>
                  </article>
                ))}
                {search.nextCursor && (
                  <button
                    type="button"
                    className="text-button"
                    disabled={busy}
                    onClick={() =>
                      void run(async (matches) => {
                        const next = unwrap(
                          await Knowledge.search({
                            ...scope(),
                            query,
                            knowledgeIds: selected
                              ? [selected.knowledgeId]
                              : [],
                            sourceTypes: sources
                              .split(',')
                              .map((source) => source.trim())
                              .filter(Boolean),
                            updatedAfter: updatedAfter || undefined,
                            limit: 20,
                            cursor: search.nextCursor || undefined,
                          }),
                        );
                        if (matches())
                          setSearch({
                            ...next,
                            items: [...search.items, ...next.items],
                          });
                      })
                    }
                  >
                    {m.desktopLoadMoreMatches()}
                  </button>
                )}
              </>
            )}
          </form>
        </>
      )}
    </main>
  );
}
