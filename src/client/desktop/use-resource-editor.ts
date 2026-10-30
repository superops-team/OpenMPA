import { useEffect, useRef, useState } from 'react';
import {
  localized,
  LocalizedError,
  useMessageState,
  type MessageText,
} from './locale';
import { studioFailure } from './studio-services';

export function useResourceEditor<R, D>({
  hidden,
  read,
  write,
  value,
  merge,
  savedMessage,
}: {
  hidden: boolean;
  savedMessage?: MessageText;
  read(): Promise<R>;
  write(record: R, draft: D): Promise<R>;
  value(record: R): D;
  merge?(base: D, draft: D, incoming: D): D;
}) {
  const [record, setRecord] = useState<R>();
  const [draft, setDraft] = useState<D>();
  const [editing, setEditing] = useState(false);
  const [busy, setBusy] = useState(false);
  const [fresh, setFresh] = useState(false);
  const [notice, setNotice] = useMessageState();
  const [error, setError] = useMessageState();
  const pending = useRef(false);
  const alive = useRef(true);
  const attempted = useRef(false);
  const editRevision = useRef(0);
  useEffect(() => {
    alive.current = true;
    return () => {
      alive.current = false;
    };
  }, []);
  const perform = async (saving: boolean) => {
    if (
      pending.current ||
      (saving && (!fresh || record === undefined || draft === undefined))
    )
      return;
    pending.current = true;
    attempted.current = true;
    const revision = editRevision.current;
    const previous = record === undefined ? undefined : value(record);
    setBusy(true);
    setError('');
    setNotice('');
    if (saving) setFresh(false);
    try {
      const result = saving ? await write(record!, draft!) : await read();
      if (!alive.current) return;
      const incoming = value(result);
      setRecord(result);
      setFresh(true);
      if (saving && revision === editRevision.current) {
        setDraft(undefined);
        setEditing(false);
      } else if (editing) {
        setDraft((current) =>
          current === undefined
            ? incoming
            : previous !== undefined && merge
              ? merge(previous, current, incoming)
              : current,
        );
      }
      if (saving)
        setNotice(savedMessage ?? localized((m) => m.instructionsSaved()));
      else if (editing) setNotice(localized((m) => m.profileDraftRetained()));
    } catch (reason) {
      if (alive.current) {
        setError(
          reason instanceof LocalizedError
            ? reason.text
            : studioFailure(reason),
        );
        if (saving) setFresh(false);
      }
    } finally {
      pending.current = false;
      if (alive.current) setBusy(false);
    }
  };
  useEffect(() => {
    if (!hidden && !attempted.current) void perform(false);
  }, [hidden]);
  const baseline = record === undefined ? undefined : value(record);
  return {
    record,
    draft,
    editing,
    busy,
    fresh,
    notice,
    error,
    dirty: editing && JSON.stringify(draft) !== JSON.stringify(baseline),
    initialize: (initial: R) => {
      if (!busy) {
        setRecord(initial);
        setDraft(structuredClone(value(initial)));
        setEditing(true);
        setFresh(true);
        setError('');
        setNotice('');
      }
    },
    refresh: () => perform(false),
    save: () => perform(true),
    begin: () => {
      if (baseline !== undefined && !busy) {
        setDraft(structuredClone(baseline));
        setEditing(true);
        setNotice('');
      }
    },
    cancel: () => {
      if (!busy) {
        editRevision.current++;
        setDraft(undefined);
        setEditing(false);
        setError('');
        setNotice('');
      }
    },
    update: (next: D) => {
      editRevision.current++;
      setDraft(next);
    },
  };
}
