/**
 * Stores a one-time redirect target for the logout flow.
 * This prevents auth guards from racing the intended post-logout redirect.
 */

const LOGOUT_REDIRECT_STORAGE_KEY = 'zgi:logout-redirect';

export function setPendingLogoutRedirect(url: string): void {
  if (typeof window === 'undefined' || !url) return;
  sessionStorage.setItem(LOGOUT_REDIRECT_STORAGE_KEY, url);
}

export function peekPendingLogoutRedirect(): string | null {
  if (typeof window === 'undefined') return null;
  return sessionStorage.getItem(LOGOUT_REDIRECT_STORAGE_KEY);
}

export function consumePendingLogoutRedirect(): string | null {
  if (typeof window === 'undefined') return null;
  const url = sessionStorage.getItem(LOGOUT_REDIRECT_STORAGE_KEY);
  if (url) {
    sessionStorage.removeItem(LOGOUT_REDIRECT_STORAGE_KEY);
  }
  return url;
}

