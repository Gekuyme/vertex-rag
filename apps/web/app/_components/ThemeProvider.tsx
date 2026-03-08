"use client";

import type { ReactNode } from "react";
import { createContext, useContext, useEffect, useMemo, useState } from "react";
import { resolveThemePreference, themeCookieName, type ThemePreference } from "../../lib/theme";

type ThemeContextValue = {
  themePreference: ThemePreference;
  setThemePreference: (theme: ThemePreference) => void;
  resolvedTheme: "light" | "dark";
};

const ThemeContext = createContext<ThemeContextValue | null>(null);

type ThemeProviderProps = {
  children: ReactNode;
  initialTheme: ThemePreference;
};

function systemTheme(): "light" | "dark" {
  if (typeof window === "undefined") {
    return "light";
  }

  return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
}

export function ThemeProvider({ children, initialTheme }: ThemeProviderProps) {
  const [themePreference, setThemePreferenceState] = useState<ThemePreference>(initialTheme);
  const [resolvedTheme, setResolvedTheme] = useState<"light" | "dark">(
    initialTheme === "system" ? systemTheme() : initialTheme
  );

  useEffect(() => {
    const nextResolvedTheme = themePreference === "system" ? systemTheme() : themePreference;
    setResolvedTheme(nextResolvedTheme);

    const root = document.documentElement;
    root.dataset.theme = themePreference;
    root.style.colorScheme = nextResolvedTheme;
    window.localStorage.setItem(themeCookieName, themePreference);
    document.cookie = `${themeCookieName}=${themePreference}; path=/; max-age=31536000; samesite=lax`;
  }, [themePreference]);

  useEffect(() => {
    if (typeof window === "undefined") {
      return;
    }

    const mediaQuery = window.matchMedia("(prefers-color-scheme: dark)");
    const onChange = () => {
      if (themePreference === "system") {
        setResolvedTheme(mediaQuery.matches ? "dark" : "light");
        document.documentElement.style.colorScheme = mediaQuery.matches ? "dark" : "light";
      }
    };

    onChange();
    mediaQuery.addEventListener("change", onChange);
    return () => mediaQuery.removeEventListener("change", onChange);
  }, [themePreference]);

  useEffect(() => {
    const stored = resolveThemePreference(window.localStorage.getItem(themeCookieName));
    if (stored !== initialTheme) {
      setThemePreferenceState(stored);
    }
  }, [initialTheme]);

  const value = useMemo<ThemeContextValue>(
    () => ({
      themePreference,
      setThemePreference: setThemePreferenceState,
      resolvedTheme
    }),
    [resolvedTheme, themePreference]
  );

  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>;
}

export function useTheme() {
  const value = useContext(ThemeContext);
  if (!value) {
    throw new Error("useTheme must be used within ThemeProvider");
  }

  return value;
}
