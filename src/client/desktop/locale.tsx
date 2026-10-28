import { createContext, useContext, useEffect, useState } from 'react';
import { isMyGo, onLocaleChange } from 'mygo-runtime';
import { currentLocale } from 'mygo-runtime/i18n';
import { messages, messagesIn, type Messages } from './generated/locales';

const LocaleContext = createContext<Messages>(messages);

export function LocaleProvider({ children }: { children: React.ReactNode }) {
  const [language, setLanguage] = useState(currentLocale);
  useEffect(
    () =>
      isMyGo() ? onLocaleChange((locale) => setLanguage(locale)) : undefined,
    [],
  );
  return (
    <LocaleContext.Provider value={messagesIn(language)}>
      {children}
    </LocaleContext.Provider>
  );
}

export function useMessages() {
  return useContext(LocaleContext);
}

export type MessageText = string | { message: (messages: Messages) => string };

export function localized(
  message: (messages: Messages) => string,
): MessageText {
  return { message };
}

export function renderMessage(text: MessageText, messages: Messages): string {
  return typeof text === 'string' ? text : text.message(messages);
}

export class LocalizedError extends Error {
  constructor(readonly text: MessageText) {
    super(renderMessage(text, messages));
  }
}

export function errorMessage(
  error: unknown,
  fallback: MessageText,
): MessageText {
  if (error instanceof LocalizedError) return error.text;
  return error instanceof Error ? error.message : fallback;
}

export function useMessageState() {
  const m = useMessages();
  const [text, setText] = useState<MessageText>('');
  return [renderMessage(text, m), setText] as const;
}
