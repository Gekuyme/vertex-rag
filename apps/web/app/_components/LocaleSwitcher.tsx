"use client";

import { usePathname, useRouter } from "next/navigation";
import { locales, replacePathLocale, type Locale } from "../../lib/i18n/config";
import { useI18n } from "./I18nProvider";

export default function LocaleSwitcher() {
  const router = useRouter();
  const pathname = usePathname();
  const { locale, messages } = useI18n();

  return (
    <label className="localeSwitcher">
      <span className="srOnly">{messages.localeSwitcherLabel}</span>
      <select
        aria-label={messages.localeSwitcherLabel}
        value={locale}
        onChange={(event) => {
          const nextLocale = event.target.value as Locale;
          router.replace(replacePathLocale(pathname, nextLocale));
        }}
      >
        {locales.map((entry) => (
          <option key={entry} value={entry}>
            {entry.toUpperCase()}
          </option>
        ))}
      </select>
    </label>
  );
}
