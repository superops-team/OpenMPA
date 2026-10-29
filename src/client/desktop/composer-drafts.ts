import type { ConversationTarget, DraftSnapshot } from './studio-models';

interface SessionDraft {
  value: string | undefined;
  editRevision: number;
  choices: string[];
}

export function composeChoice(body: string, text: string): string {
  if (text.startsWith('@')) return `${text}${body ? `\n${body}` : ''}`;
  return `${body}${body && !/\s$/.test(body) ? '\n' : ''}${text}`;
}

export function validChoice(text: string): boolean {
  return (
    typeof text === 'string' &&
    !!text.trim() &&
    !text.includes('\0') &&
    new TextEncoder().encode(text).length <= 64 * 1024
  );
}

export function skillChoice(name: string): string | undefined {
  return /^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$/.test(name)
    ? `$${name}`
    : undefined;
}

// App-lifetime memory only. A receipt can update its original session after unmount.
export class ComposerDraftStore {
  private identity = '';
  private drafts = new Map<string, SessionDraft>();

  reset(epoch: number | undefined, scope: string | undefined) {
    const identity = JSON.stringify([epoch, scope]);
    if (identity !== this.identity) {
      this.identity = identity;
      this.drafts.clear();
    }
  }

  private current(target: ConversationTarget) {
    return (
      this.identity === JSON.stringify([target.connectionEpoch, target.scope])
    );
  }

  private entry(target: ConversationTarget): SessionDraft | undefined {
    if (!this.current(target)) return undefined;
    let entry = this.drafts.get(target.sessionId);
    if (!entry) {
      entry = { value: undefined, editRevision: 0, choices: [] };
      this.drafts.set(target.sessionId, entry);
    }
    return entry;
  }

  value(target: ConversationTarget) {
    return this.entry(target)?.value;
  }

  set(
    target: ConversationTarget,
    update:
      | string
      | undefined
      | ((previous: string | undefined) => string | undefined),
  ) {
    const entry = this.entry(target);
    if (!entry) return;
    const next = typeof update === 'function' ? update(entry.value) : update;
    if (typeof update === 'function' && next === entry.value) return;
    entry.value = next;
    entry.editRevision++;
    entry.choices = [];
  }

  insert(target: ConversationTarget, text: string): boolean {
    if (!validChoice(text)) return false;
    const entry = this.entry(target);
    if (!entry) return false;
    const value = composeChoice(entry.value ?? '', text);
    if (
      value.length > 100000 ||
      new TextEncoder().encode(value).length > 100000
    )
      return false;
    entry.value = value;
    entry.choices.push(text);
    return true;
  }

  snapshot(target: ConversationTarget): DraftSnapshot {
    const entry = this.entry(target);
    return {
      ...target,
      value: entry?.value ?? '',
      editRevision: entry?.editRevision ?? -1,
      choiceCount: entry?.choices.length ?? 0,
    };
  }

  accept(snapshot: DraftSnapshot) {
    const entry = this.entry(snapshot);
    if (!entry || entry.editRevision !== snapshot.editRevision) return;
    const choices = entry.choices.slice(snapshot.choiceCount);
    entry.value = choices.reduce(composeChoice, '');
    entry.choices = choices;
  }

  restore(snapshot: DraftSnapshot, cleared: DraftSnapshot) {
    if (!this.current(snapshot)) return;
    const entry = this.drafts.get(snapshot.sessionId);
    if (!entry || entry.editRevision !== cleared.editRevision) return;
    const choices = entry.choices.slice(cleared.choiceCount);
    entry.value = choices.reduce(composeChoice, snapshot.value);
    entry.choices = [];
    entry.editRevision++;
  }

  remove(sessionId: string) {
    this.drafts.delete(sessionId);
  }
}
