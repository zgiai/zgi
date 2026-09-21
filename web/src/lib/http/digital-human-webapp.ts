import { sessionManager } from '@/lib/auth/session-manager';
import type { ApiResponseData } from '@/services/types/common';
import { http, type ExtendedRequestConfig, type SseOptions } from './client';

interface DigitalHumanWebAppSession {
  session_token?: string;
  expires_at?: number;
  anonymous_enabled: boolean;
  login_required: boolean;
}

interface StoredDigitalHumanWebAppSession {
  token: string;
  expiresAt: number;
}

const SESSION_KEY_PREFIX = 'zgi-digital-human-webapp-session:';
const SESSION_EXPIRY_SKEW_SECONDS = 60;
const pendingSessions = new Map<string, Promise<string>>();

function storageKey(webAppId: string): string {
  return `${SESSION_KEY_PREFIX}${webAppId}`;
}

function readStoredSession(webAppId: string): string | null {
  if (typeof window === 'undefined') return null;
  const raw = window.localStorage.getItem(storageKey(webAppId));
  if (!raw) return null;
  try {
    const stored = JSON.parse(raw) as StoredDigitalHumanWebAppSession;
    if (
      typeof stored.token === 'string' &&
      stored.token.length > 0 &&
      Number.isFinite(stored.expiresAt) &&
      stored.expiresAt > Date.now() / 1000 + SESSION_EXPIRY_SKEW_SECONDS
    ) {
      return stored.token;
    }
  } catch {
    // Invalid local state is replaced by a server-issued session below.
  }
  window.localStorage.removeItem(storageKey(webAppId));
  return null;
}

async function createAnonymousSession(webAppId: string): Promise<string> {
  const response = await http.post<ApiResponseData<DigitalHumanWebAppSession>>(
    `/api/v1/digital-human-webapps/${encodeURIComponent(webAppId)}/session`,
    {},
    { skipAuth: true, skipErrorHandling: true }
  );
  const session = response.data;
  if (session.login_required || !session.anonymous_enabled || !session.session_token) {
    throw new Error('Authentication is required for this digital human.');
  }
  if (typeof window !== 'undefined') {
    window.localStorage.setItem(
      storageKey(webAppId),
      JSON.stringify({
        token: session.session_token,
        expiresAt: session.expires_at ?? Date.now() / 1000,
      } satisfies StoredDigitalHumanWebAppSession)
    );
  }
  return session.session_token;
}

export async function ensureDigitalHumanWebAppSession(webAppId: string): Promise<string | null> {
  if (sessionManager.hasSession()) return null;
  const stored = readStoredSession(webAppId);
  if (stored) return stored;
  const pending = pendingSessions.get(webAppId);
  if (pending) return pending;
  const created = createAnonymousSession(webAppId).finally(() => pendingSessions.delete(webAppId));
  pendingSessions.set(webAppId, created);
  return created;
}

async function anonymousRequestConfig(
  webAppId: string,
  config?: ExtendedRequestConfig
): Promise<ExtendedRequestConfig> {
  if (sessionManager.hasSession()) return config ?? {};
  const token = await ensureDigitalHumanWebAppSession(webAppId);
  return {
    ...config,
    headers: {
      ...config?.headers,
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    skipAuth: true,
    skipErrorHandling: true,
  };
}

export function createDigitalHumanWebAppHttp(webAppId: string) {
  return {
    get: async <T = unknown>(url: string, config?: ExtendedRequestConfig): Promise<T> =>
      http.get<T>(url, await anonymousRequestConfig(webAppId, config)),
    post: async <T = unknown>(
      url: string,
      data?: unknown,
      config?: ExtendedRequestConfig
    ): Promise<T> => http.post<T>(url, data, await anonymousRequestConfig(webAppId, config)),
    patch: async <T = unknown>(
      url: string,
      data?: unknown,
      config?: ExtendedRequestConfig
    ): Promise<T> => http.patch<T>(url, data, await anonymousRequestConfig(webAppId, config)),
    delete: async <T = unknown>(url: string, config?: ExtendedRequestConfig): Promise<T> =>
      http.delete<T>(url, await anonymousRequestConfig(webAppId, config)),
    sse: async <TOut = unknown, TBody = unknown>(
      path: string,
      options: SseOptions<TBody, TOut>
    ): Promise<{ close: () => void }> => {
      if (sessionManager.hasSession()) return http.sse<TOut, TBody>(path, options);
      const token = await ensureDigitalHumanWebAppSession(webAppId);
      return http.sse<TOut, TBody>(path, {
        ...options,
        headers: {
          ...options.headers,
          ...(token ? { Authorization: `Bearer ${token}` } : {}),
        },
        skipAuth: true,
        skipErrorHandling: true,
      });
    },
  };
}

export function clearDigitalHumanWebAppSession(webAppId: string): void {
  if (typeof window !== 'undefined') {
    window.localStorage.removeItem(storageKey(webAppId));
  }
}
