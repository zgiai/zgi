// Webapp-specific HTTP helpers with independent token logic
// - Uses localStorage key 'zgi-web-app-token'
// - If missing, generate a token on the client and persist it
// - Attach token as Authorization: Bearer <token>

import { http, type ExtendedRequestConfig, type SseOptions, type SsePostOptions } from './client';
import { sessionManager } from '@/lib/auth/session-manager';
import { generateClientId } from '@/utils/client-id';

export const WEBAPP_TOKEN_KEY = 'zgi-web-app-token';

// Read token from localStorage (client-only)
export function getWebAppToken(): string | null {
  if (typeof window === 'undefined') return null;
  return window.localStorage.getItem(WEBAPP_TOKEN_KEY);
}

// Generate a stable random token and persist to localStorage
export function ensureWebAppToken(): string {
  if (typeof window === 'undefined') {
    // In SSR, just return an empty string to avoid throwing; callers should prefer client-side usage
    return '';
  }
  const existing = window.localStorage.getItem(WEBAPP_TOKEN_KEY);
  if (existing && existing.length > 0) return existing;

  const token = generateClientId();

  window.localStorage.setItem(WEBAPP_TOKEN_KEY, token);
  return token;
}

function buildWebAppAuthHeader(): Record<string, string> {
  const token = ensureWebAppToken();
  // If token is empty (SSR), return empty header to avoid invalid value
  return token ? { Authorization: `Bearer ${token}` } : {};
}

function isMainSiteLoggedIn(): boolean {
  return sessionManager.hasSession();
}

export const webappHttp = {
  get: async <T = unknown>(url: string, config?: ExtendedRequestConfig): Promise<T> => {
    if (isMainSiteLoggedIn()) {
      return http.get<T>(url, config);
    }
    const headers = { ...config?.headers, ...buildWebAppAuthHeader() } as Record<string, string>;
    return http.get<T>(url, { ...config, headers, skipAuth: true, skipErrorHandling: true });
  },
  post: async <T = unknown>(
    url: string,
    data?: unknown,
    config?: ExtendedRequestConfig
  ): Promise<T> => {
    if (isMainSiteLoggedIn()) {
      return http.post<T>(url, data, config);
    }
    const headers = { ...config?.headers, ...buildWebAppAuthHeader() } as Record<string, string>;
    return http.post<T>(url, data, { ...config, headers, skipAuth: true, skipErrorHandling: true });
  },
  put: async <T = unknown>(url: string, data?: unknown, config?: ExtendedRequestConfig): Promise<T> => {
    if (isMainSiteLoggedIn()) {
      return http.put<T>(url, data, config);
    }
    const headers = { ...config?.headers, ...buildWebAppAuthHeader() } as Record<string, string>;
    return http.put<T>(url, data, { ...config, headers, skipAuth: true, skipErrorHandling: true });
  },
  patch: async <T = unknown>(
    url: string,
    data?: unknown,
    config?: ExtendedRequestConfig
  ): Promise<T> => {
    if (isMainSiteLoggedIn()) {
      return http.patch<T>(url, data, config);
    }
    const headers = { ...config?.headers, ...buildWebAppAuthHeader() } as Record<string, string>;
    return http.patch<T>(url, data, { ...config, headers, skipAuth: true, skipErrorHandling: true });
  },
  delete: async <T = unknown>(url: string, config?: ExtendedRequestConfig): Promise<T> => {
    if (isMainSiteLoggedIn()) {
      return http.delete<T>(url, config);
    }
    const headers = { ...config?.headers, ...buildWebAppAuthHeader() } as Record<string, string>;
    return http.delete<T>(url, { ...config, headers, skipAuth: true, skipErrorHandling: true });
  },
  upload: async <T = unknown>(
    url: string,
    formData: FormData,
    onProgress?: (progress: number) => void,
    config?: ExtendedRequestConfig
  ): Promise<T> => {
    if (isMainSiteLoggedIn()) {
      return http.upload<T>(url, formData, onProgress, config);
    }
    const headers = { ...config?.headers, ...buildWebAppAuthHeader() } as Record<string, string>;
    return http.upload<T>(url, formData, onProgress, { ...config, headers, skipAuth: true, skipErrorHandling: true });
  },
  sse: async <TOut = unknown, TBody = unknown>(
    path: string,
    options: SseOptions<TBody, TOut>
  ): Promise<{ close: () => void }> => {
    if (isMainSiteLoggedIn()) {
      return http.sse<TOut, TBody>(path, options);
    }
    const headers = { ...(options.headers || {}), ...buildWebAppAuthHeader() } as Record<string, string>;
    return http.sse<TOut, TBody>(path, { ...options, headers, skipAuth: true, skipErrorHandling: true });
  },
  ssePost: async <TBody = unknown>(
    path: string,
    options: SsePostOptions<TBody>
  ): Promise<{ close: () => void }> => {
    if (isMainSiteLoggedIn()) {
      return http.ssePost<TBody>(path, options);
    }
    const { callbacks, ...rest } = options;
    const headers = { ...(rest.headers || {}), ...buildWebAppAuthHeader() } as Record<string, string>;
    return http.ssePost<TBody>(path, { ...rest, headers, skipAuth: true, skipErrorHandling: true, callbacks });
  },
};
