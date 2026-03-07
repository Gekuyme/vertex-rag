"use client";

import type { ReactNode } from "react";
import { createContext, useContext, useEffect, useMemo } from "react";
import { getMessages, type Messages } from "../../lib/i18n/messages";
import { localeCookieName, type Locale } from "../../lib/i18n/config";

type I18nContextValue = {
  locale: Locale;
  messages: Messages;
};

const I18nContext = createContext<I18nContextValue | null>(null);

type I18nProviderProps = {
  children: ReactNode;
  locale: Locale;
};

export function I18nProvider({ children, locale }: I18nProviderProps) {
  const value = useMemo<I18nContextValue>(
    () => ({
      locale,
      messages: getMessages(locale)
    }),
    [locale]
  );

  useEffect(() => {
    document.documentElement.lang = locale;
    document.cookie = `${localeCookieName}=${locale}; path=/; max-age=31536000; samesite=lax`;
  }, [locale]);

  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>;
}

export function useI18n() {
  const value = useContext(I18nContext);
  if (!value) {
    throw new Error("useI18n must be used within I18nProvider");
  }

  return value;
}
