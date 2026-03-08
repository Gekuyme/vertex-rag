export const themeCookieName = "vertex_theme";

export const themeOptions = ["system", "light", "dark"] as const;

export type ThemePreference = (typeof themeOptions)[number];

export function isThemePreference(value: string | null | undefined): value is ThemePreference {
  return Boolean(value && themeOptions.includes(value as ThemePreference));
}

export function resolveThemePreference(value?: string | null): ThemePreference {
  return isThemePreference(value) ? value : "system";
}
