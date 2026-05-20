// Simple cookie utilities with JSON support and expiration handling
// All comments in English for clarity

interface CookieOptions {
  domain?: string;
  path?: string;
  sameSite?: 'Lax' | 'Strict' | 'None';
  secure?: boolean;
}

function buildCookieString(name: string, value: string, days: number, options?: CookieOptions): string {
  const expires = new Date();
  expires.setTime(expires.getTime() + days * 24 * 60 * 60 * 1000);
  const secure =
    options?.secure ?? (typeof window !== 'undefined' && window.location.protocol === 'https:');
  const path = options?.path || '/';
  const sameSite = options?.sameSite || 'Lax';
  const domain = options?.domain ? `; domain=${options.domain}` : '';

  return `${name}=${value}; expires=${expires.toUTCString()}; path=${path}; SameSite=${sameSite}${domain}${
    secure ? '; Secure' : ''
  }`;
}

export function setCookie(name: string, value: unknown, days: number, options?: CookieOptions): void {
  if (typeof document === 'undefined') return;
  const encoded = encodeURIComponent(JSON.stringify(value));
  document.cookie = buildCookieString(name, encoded, days, options);
}

export function setRawCookie(
  name: string,
  value: string,
  days: number,
  options?: CookieOptions
): void {
  if (typeof document === 'undefined') return;
  document.cookie = buildCookieString(name, value, days, options);
}

export function getCookie<T = unknown>(name: string): T | null {
  if (typeof document === 'undefined') return null;
  const nameEQ = `${name}=`;
  const parts = document.cookie.split(';');
  for (let i = 0; i < parts.length; i++) {
    let c = parts[i];
    while (c.charAt(0) === ' ') c = c.substring(1, c.length);
    if (c.indexOf(nameEQ) === 0) {
      const raw = c.substring(nameEQ.length, c.length);
      try {
        return JSON.parse(decodeURIComponent(raw)) as T;
      } catch {
        return null;
      }
    }
  }
  return null;
}

export function deleteCookie(name: string, options?: Pick<CookieOptions, 'domain' | 'path'>): void {
  if (typeof document === 'undefined') return;
  const path = options?.path || '/';
  const domain = options?.domain ? `; domain=${options.domain}` : '';
  document.cookie = `${name}=; expires=Thu, 01 Jan 1970 00:00:00 GMT; path=${path}; SameSite=Lax${domain}`;
}
