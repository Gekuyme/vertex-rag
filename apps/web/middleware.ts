import type { NextRequest } from "next/server";
import { NextResponse } from "next/server";
import {
  defaultLocale,
  getLocalizedPath,
  getPreferredLocale,
  isLocale,
  localeCookieName,
  resolveLocale
} from "./lib/i18n/config";

function pathnameHasLocale(pathname: string) {
  const segment = pathname.split("/")[1];
  return Boolean(segment && isLocale(segment));
}

export function middleware(request: NextRequest) {
  const { pathname } = request.nextUrl;
  if (pathnameHasLocale(pathname)) {
    return NextResponse.next();
  }

  const cookieLocale = request.cookies.get(localeCookieName)?.value;
  const locale = cookieLocale
    ? resolveLocale(cookieLocale)
    : getPreferredLocale(request.headers.get("accept-language")) || defaultLocale;
  const redirectURL = request.nextUrl.clone();
  redirectURL.pathname = getLocalizedPath(locale, pathname);
  return NextResponse.redirect(redirectURL);
}

export const config = {
  matcher: ["/((?!api|_next/static|_next/image|favicon.ico|.*\\..*).*)"]
};
