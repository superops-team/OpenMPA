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
