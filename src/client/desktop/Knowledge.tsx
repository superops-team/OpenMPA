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
export function DesktopKnowledge({
  connection,
  sessions,
}: {
  connection?: ConnectionVM;
  sessions: SessionVM[];
}) {
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
  const [error, setError] = useState('');
  const generation = useRef(0);
  const pendingImports = useRef(new Map<string, PendingImport>());
  const scopeKey = `${connection?.epoch}:${sessionId}`;
  const scope = () => {
    if (!connection || !sessionId)
      throw new Error('Choose a Runtime conversation.');
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
            : 'Knowledge operation failed.',
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
      if (pending)
        throw new Error(
          `Import ${pending.input.idempotencyKey} has an unknown outcome. Verify and recover that exact submission before creating a new import.`,
        );
      if (version && !selected)
        throw new Error('Choose an item for the new version.');
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
          <span className="eyebrow">SESSION KNOWLEDGE</span>
          <h1>Reference material, with provenance.</h1>
          <p>
            Imported knowledge belongs to the selected conversation. It does not
            create executable sandbox attachments.
          </p>
        </div>
      </div>
      <label>
        Conversation
        <select
          value={sessionId}
          onChange={(event) => setSessionId(event.target.value)}
        >
          <option value="">Choose a conversation</option>
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
          Connect a Runtime to access session knowledge.
        </div>
      )}
      {connection && sessionId && (
        <>
          <section className="task-detail-card desktop-form">
            <div className="desktop-inline">
              <h2>Knowledge library</h2>
              <button
                className="text-button"
                disabled={busy}
                onClick={() => void run(load)}
              >
                Refresh library
              </button>
            </div>
            <div className="task-list">
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
                      Version {item.version} · {item.status} · {item.sourceType}
                    </span>
                  </div>
                </button>
              ))}
            </div>
          </section>
          <section className="task-detail-card desktop-form">
            <h2>Import material</h2>
            <label>
              Display name
              <input
                value={name}
                onChange={(event) => setName(event.target.value)}
              />
            </label>
            <label>
              Text material
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
                Import text
              </button>
              <button
                className="text-button"
                disabled={busy || !!pending}
                onClick={() => importKnowledge('file', false)}
              >
                <Upload size={14} />
                Choose file & import
              </button>
              <button
                className="text-button"
                disabled={busy || !selected || !text.trim() || !!pending}
                onClick={() => importKnowledge('text', true)}
              >
                Add text version to selected item
              </button>
              <button
                className="text-button"
                disabled={busy || !selected || !!pending}
                onClick={() => importKnowledge('file', true)}
              >
                Choose file for a new version
              </button>
            </div>
            <p>
              File selection is provided by the native host. An accepted import
              must be checked before treating it as ready.
            </p>
            {pending && (
              <div className="document-save-notice">
                <p>
                  Pending import key {pending.input.idempotencyKey}. The
                  original {pending.mode} submission is retained. Verify the
                  library and import status before recovery.
                </p>
                <button
                  disabled={busy}
                  onClick={() => {
                    if (
                      window.confirm(
                        'Recover this exact import using its original idempotency key? For files, select the same original file.',
                      )
                    )
                      void run((matches) => submit(pending, matches));
                  }}
                >
                  Recover original import
                </button>
                <button
                  disabled={busy}
                  onClick={() => {
                    if (
                      window.confirm(
                        'Forget this unresolved import locally? This does not cancel a server import. Verify its outcome before importing the same material again.',
                      )
                    ) {
                      pendingImports.current.delete(scopeKey);
                      setPending(undefined);
                    }
                  }}
                >
                  Discard local recovery after review
                </button>
              </div>
            )}
            {receipt && (
              <div role="status">
                <p>
                  Import {receipt.importId} · Knowledge {receipt.knowledgeId} ·
                  Version {receipt.version} · {receipt.status}
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
                  Read authoritative import status
                </button>
              </div>
            )}
          </section>
          {selected && (
            <section className="task-detail-card desktop-form">
              <h2>{selected.displayName}</h2>
              <p>
                Knowledge {selected.knowledgeId} · Version {selected.version} ·{' '}
                {selected.status} · {selected.chunkCount} chunks
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
                  Read selected content
                </button>
                <button
                  className="text-button"
                  disabled={busy}
                  onClick={() => {
                    if (
                      window.confirm(
                        `Delete knowledge ${selected.knowledgeId}?`,
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
                  Delete knowledge
                </button>
              </div>
              {content && (
                <>
                  <pre className="desktop-content">{content.content}</pre>
                  <p>
                    {content.completeness} · Citation {content.citation.ref} ·{' '}
                    {content.visibleBytes} bytes visible
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
                      Read more content
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
            <h2>Search reference material</h2>
            <label>
              Search query
              <input
                required
                value={query}
                onChange={(event) => setQuery(event.target.value)}
              />
            </label>
            <label>
              Source types (optional, comma separated)
              <input
                value={sources}
                onChange={(event) => setSources(event.target.value)}
              />
            </label>
            <label>
              Updated after (optional RFC3339)
              <input
                value={updatedAfter}
                onChange={(event) => setUpdatedAfter(event.target.value)}
              />
            </label>
            <p>
              {selected
                ? `Searching selected knowledge ${selected.knowledgeId}`
                : 'Searching this conversation knowledge'}
            </p>
            <button className="primary" disabled={busy}>
              <Search size={14} />
              Search
            </button>
            {search && (
              <>
                <p>
                  {search.completeness}
                  {search.boundedReason ? ` · ${search.boundedReason}` : ''}
                </p>
                {search.items.map((item) => (
                  <article key={item.ref}>
                    <strong>{item.displayName}</strong>
                    <p>{item.content}</p>
                    <small>
                      Citation {item.citation.ref} · Version {item.version}
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
                    Load more matches
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
