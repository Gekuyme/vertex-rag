import type { Metadata } from "next";
import type { ReactNode } from "react";
import { cookies } from "next/headers";
import { notFound } from "next/navigation";
import { Geist } from "next/font/google";
import { I18nProvider } from "../_components/I18nProvider";
import { ThemeProvider } from "../_components/ThemeProvider";
import { isLocale, locales, type Locale } from "../../lib/i18n/config";
import { getMessages } from "../../lib/i18n/messages";
import { resolveThemePreference, themeCookieName } from "../../lib/theme";
import "../globals.css";

const geistDisplay = Geist({
  subsets: ["latin"],
  variable: "--font-display"
});

type LocaleLayoutProps = Readonly<{
  children: ReactNode;
  params: Promise<{ locale: string }>;
}>;

export function generateStaticParams() {
  return locales.map((locale) => ({ locale }));
}

export async function generateMetadata({ params }: LocaleLayoutProps): Promise<Metadata> {
  const { locale } = await params;
  if (!isLocale(locale)) {
    return {};
  }

  const messages = getMessages(locale);
  return {
    title: messages.metadata.title,
    description: messages.metadata.description
  };
}

export default async function LocaleLayout({ children, params }: LocaleLayoutProps) {
  const { locale } = await params;
  if (!isLocale(locale)) {
    notFound();
  }
  const cookieStore = await cookies();
  const initialTheme = resolveThemePreference(cookieStore.get(themeCookieName)?.value);

  return (
    <html lang={locale} data-theme={initialTheme}>
      <body className={geistDisplay.variable}>
        <ThemeProvider initialTheme={initialTheme}>
          <I18nProvider locale={locale as Locale}>{children}</I18nProvider>
        </ThemeProvider>
      </body>
    </html>
  );
}
