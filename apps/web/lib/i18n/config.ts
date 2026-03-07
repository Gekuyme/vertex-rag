export const locales = ["ru", "en"] as const;

export type Locale = (typeof locales)[number];

export const defaultLocale: Locale = "ru";
export const localeCookieName = "vertex_locale";

export function isLocale(value: string): value is Locale {
  return locales.includes(value as Locale);
}

export function resolveLocale(value?: string | null): Locale {
  if (value && isLocale(value)) {
    return value;
  }

  return defaultLocale;
}

export function getPreferredLocale(headerValue?: string | null): Locale {
  if (!headerValue) {
    return defaultLocale;
  }

  const requested = headerValue
    .split(",")
    .map((part) => part.trim().split(";")[0]?.toLowerCase())
    .filter(Boolean);

  for (const locale of requested) {
    if (!locale) {
      continue;
    }
    if (isLocale(locale)) {
      return locale;
    }
    const language = locale.split("-")[0];
    if (language && isLocale(language)) {
      return language;
    }
  }

  return defaultLocale;
}

export function getLocalizedPath(locale: Locale, path = "/"): string {
  const normalizedPath = path === "/" ? "" : path.startsWith("/") ? path : `/${path}`;
  return `/${locale}${normalizedPath}`;
}

export function replacePathLocale(pathname: string, locale: Locale): string {
  const segments = pathname.split("/");
  if (segments.length > 1 && isLocale(segments[1] || "")) {
    segments[1] = locale;
    return segments.join("/") || "/";
  }

  return getLocalizedPath(locale, pathname);
}
