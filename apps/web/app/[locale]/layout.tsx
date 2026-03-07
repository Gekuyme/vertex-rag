import type { Metadata } from "next";
import type { ReactNode } from "react";
import { notFound } from "next/navigation";
import { I18nProvider } from "../_components/I18nProvider";
import { isLocale, locales, type Locale } from "../../lib/i18n/config";
import { getMessages } from "../../lib/i18n/messages";
import "../globals.css";

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

  return (
    <html lang={locale}>
      <body>
        <I18nProvider locale={locale as Locale}>{children}</I18nProvider>
      </body>
    </html>
  );
}
