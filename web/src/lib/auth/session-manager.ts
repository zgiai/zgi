import { ENABLE_ROOT_COOKIE_TOKEN_SYNC, ROOT_COOKIE_DOMAIN } from '@/lib/config';
import { deleteCookie, setRawCookie } from '@/utils/cookie';
import { generateClientId } from '@/utils/client-id';

export const AUTH_SESSION_STORAGE_KEY = 'auth_session_v1';
export const AUTH_SYNC_CHANNEL = 'zgi-auth';

const AUTH_TOKEN_STORAGE_KEY = 'auth_token';
const REFRESH_TOKEN_STORAGE_KEY = 'refresh_token';
const AUTH_TAB_ID_STORAGE_KEY = 'zgi-auth-tab-id';
const TOKEN_COOKIE_DAYS = 30;
const REFRESH_TOKEN_COOKIE_DAYS = 30;

export type AuthSyncEventType =
  | 'SIGNED_IN'
  | 'SIGNED_OUT'
  | 'TOKEN_REFRESHED'
  | 'PROFILE_UPDATED'
  | 'CONTEXT_CHANGED';

export interface AuthSessionEnvelope {
  accessToken: string;
  refreshToken: string | null;
  updatedAt: number;
  sourceTabId: string;
}

export interface AuthSyncEvent {
  type: AuthSyncEventType;
  sourceTabId: string;
  updatedAt: number;
  session?: AuthSessionEnvelope | null;
  payload?: Record<string, string | null | undefined>;
}

interface SetSessionOptions {
  type?: Extract<AuthSyncEventType, 'SIGNED_IN' | 'TOKEN_REFRESHED'>;
  broadcast?: boolean;
}

interface ClearSessionOptions {
  type?: Extract<AuthSyncEventType, 'SIGNED_OUT'>;
  broadcast?: boolean;
}

type AuthSyncListener = (event: AuthSyncEvent) => void;

function isBrowser(): boolean {
  return typeof window !== 'undefined';
}

function createId(): string {
  if (!isBrowser()) {
    return 'server';
  }

  return generateClientId();
}

function readStorageValue(key: string): string | null {
  if (!isBrowser()) {
    return null;
  }

  try {
    return window.localStorage.getItem(key);
  } catch {
    return null;
  }
}

function writeStorageValue(key: string, value: string): void {
  if (!isBrowser()) {
    return;
  }

  try {
    window.localStorage.setItem(key, value);
  } catch {
    // Ignore storage write failures and keep the in-memory flow alive.
  }
}

function removeStorageValue(key: string): void {
  if (!isBrowser()) {
    return;
  }

  try {
    window.localStorage.removeItem(key);
  } catch {
    // Ignore storage removal failures and keep the in-memory flow alive.
  }
}

function parseSession(raw: string | null): AuthSessionEnvelope | null {
  if (!raw) {
    return null;
  }

  try {
    const parsed = JSON.parse(raw) as Partial<AuthSessionEnvelope>;
    if (!parsed?.accessToken || typeof parsed.accessToken !== 'string') {
      return null;
    }

    return {
      accessToken: parsed.accessToken,
      refreshToken:
        typeof parsed.refreshToken === 'string' && parsed.refreshToken.length > 0
          ? parsed.refreshToken
          : null,
      updatedAt: typeof parsed.updatedAt === 'number' ? parsed.updatedAt : Date.now(),
      sourceTabId:
        typeof parsed.sourceTabId === 'string' && parsed.sourceTabId.length > 0
          ? parsed.sourceTabId
          : 'legacy',
    };
  } catch {
    return null;
  }
}

function serializeSession(session: AuthSessionEnvelope): string {
  return JSON.stringify(session);
}

function syncShadowTokenStorage(session: AuthSessionEnvelope | null): void {
  if (!isBrowser()) {
    return;
  }

  if (!session) {
    removeStorageValue(AUTH_TOKEN_STORAGE_KEY);
    removeStorageValue(REFRESH_TOKEN_STORAGE_KEY);
    return;
  }

  writeStorageValue(AUTH_TOKEN_STORAGE_KEY, session.accessToken);

  if (session.refreshToken) {
    writeStorageValue(REFRESH_TOKEN_STORAGE_KEY, session.refreshToken);
  } else {
    removeStorageValue(REFRESH_TOKEN_STORAGE_KEY);
  }
}

function syncRootTokenCookies(session: AuthSessionEnvelope | null): void {
  if (!isBrowser() || !ENABLE_ROOT_COOKIE_TOKEN_SYNC || ROOT_COOKIE_DOMAIN.length === 0) {
    return;
  }

  if (!session) {
    deleteCookie('auth_token', { domain: ROOT_COOKIE_DOMAIN });
    deleteCookie('refresh_token', { domain: ROOT_COOKIE_DOMAIN });
    return;
  }

  setRawCookie('auth_token', session.accessToken, TOKEN_COOKIE_DAYS, {
    domain: ROOT_COOKIE_DOMAIN,
    sameSite: 'Lax',
  });

  if (session.refreshToken) {
    setRawCookie('refresh_token', session.refreshToken, REFRESH_TOKEN_COOKIE_DAYS, {
      domain: ROOT_COOKIE_DOMAIN,
      sameSite: 'Lax',
    });
  } else {
    deleteCookie('refresh_token', { domain: ROOT_COOKIE_DOMAIN });
  }
}

class SessionManager {
  private listeners = new Set<AuthSyncListener>();
  private channel: BroadcastChannel | null = null;

  getCurrentTabId(): string {
    if (!isBrowser()) {
      return 'server';
    }

    try {
      const existing = window.sessionStorage.getItem(AUTH_TAB_ID_STORAGE_KEY);
      if (existing) {
        return existing;
      }

      const created = createId();
      window.sessionStorage.setItem(AUTH_TAB_ID_STORAGE_KEY, created);
      return created;
    } catch {
      return createId();
    }
  }

  private getBroadcastChannel(): BroadcastChannel | null {
    if (!isBrowser() || typeof BroadcastChannel === 'undefined') {
      return null;
    }

    if (!this.channel) {
      this.channel = new BroadcastChannel(AUTH_SYNC_CHANNEL);
    }

    return this.channel;
  }

  private emit(event: AuthSyncEvent): void {
    this.listeners.forEach(listener => listener(event));
  }

  private post(event: AuthSyncEvent): void {
    this.emit(event);

    const channel = this.getBroadcastChannel();
    if (!channel) {
      return;
    }

    channel.postMessage(event);
  }

  private getLegacySession(): AuthSessionEnvelope | null {
    const accessToken = readStorageValue(AUTH_TOKEN_STORAGE_KEY);
    if (!accessToken) {
      return null;
    }

    return {
      accessToken,
      refreshToken: readStorageValue(REFRESH_TOKEN_STORAGE_KEY),
      updatedAt: Date.now(),
      sourceTabId: this.getCurrentTabId(),
    };
  }

  private persistLegacySession(session: AuthSessionEnvelope): void {
    writeStorageValue(AUTH_SESSION_STORAGE_KEY, serializeSession(session));
    syncShadowTokenStorage(session);
    syncRootTokenCookies(session);
  }

  getSession(): AuthSessionEnvelope | null {
    const current = parseSession(readStorageValue(AUTH_SESSION_STORAGE_KEY));
    if (current) {
      return current;
    }

    const legacy = this.getLegacySession();
    if (!legacy) {
      return null;
    }

    this.persistLegacySession(legacy);
    return legacy;
  }

  getAccessToken(): string | null {
    return this.getSession()?.accessToken ?? null;
  }

  getRefreshToken(): string | null {
    return this.getSession()?.refreshToken ?? null;
  }

  hasSession(): boolean {
    return Boolean(this.getAccessToken());
  }

  syncRootCookiesForCurrentSession(): void {
    syncRootTokenCookies(this.getSession());
  }

  setSession(
    session: { accessToken: string; refreshToken?: string | null },
    options?: SetSessionOptions
  ): AuthSessionEnvelope {
    const envelope: AuthSessionEnvelope = {
      accessToken: session.accessToken,
      refreshToken: session.refreshToken ?? null,
      updatedAt: Date.now(),
      sourceTabId: this.getCurrentTabId(),
    };

    writeStorageValue(AUTH_SESSION_STORAGE_KEY, serializeSession(envelope));
    syncShadowTokenStorage(envelope);
    syncRootTokenCookies(envelope);

    const event: AuthSyncEvent = {
      type: options?.type ?? 'SIGNED_IN',
      sourceTabId: envelope.sourceTabId,
      updatedAt: envelope.updatedAt,
      session: envelope,
    };

    if (options?.broadcast === false) {
      this.emit(event);
    } else {
      this.post(event);
    }

    return envelope;
  }

  clearSession(options?: ClearSessionOptions): void {
    const event: AuthSyncEvent = {
      type: options?.type ?? 'SIGNED_OUT',
      sourceTabId: this.getCurrentTabId(),
      updatedAt: Date.now(),
      session: null,
    };

    removeStorageValue(AUTH_SESSION_STORAGE_KEY);
    syncShadowTokenStorage(null);
    syncRootTokenCookies(null);

    if (options?.broadcast === false) {
      this.emit(event);
    } else {
      this.post(event);
    }
  }

  broadcast(event: Omit<AuthSyncEvent, 'sourceTabId' | 'updatedAt'>): void {
    const nextEvent: AuthSyncEvent = {
      ...event,
      sourceTabId: this.getCurrentTabId(),
      updatedAt: Date.now(),
    };

    this.post(nextEvent);
  }

  broadcastProfileUpdated(): void {
    this.broadcast({ type: 'PROFILE_UPDATED' });
  }

  broadcastContextChanged(payload?: Record<string, string | null | undefined>): void {
    this.broadcast({ type: 'CONTEXT_CHANGED', payload });
  }

  subscribe(listener: AuthSyncListener): () => void {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
    };
  }

  subscribeToCrossTabEvents(listener: AuthSyncListener): () => void {
    if (!isBrowser()) {
      return () => undefined;
    }

    const handleEvent = (event: AuthSyncEvent) => {
      if (event.sourceTabId === this.getCurrentTabId()) {
        return;
      }

      listener(event);
    };

    const handleBroadcast = (message: MessageEvent<AuthSyncEvent>) => {
      if (!message.data) {
        return;
      }

      handleEvent(message.data);
    };

    const handleStorage = (event: StorageEvent) => {
      if (event.storageArea !== window.localStorage) {
        return;
      }

      if (event.key !== AUTH_SESSION_STORAGE_KEY) {
        return;
      }

      const previousSession = parseSession(event.oldValue);
      const nextSession = parseSession(event.newValue);

      const type: AuthSyncEventType = nextSession
        ? previousSession
          ? 'TOKEN_REFRESHED'
          : 'SIGNED_IN'
        : 'SIGNED_OUT';

      handleEvent({
        type,
        sourceTabId: nextSession?.sourceTabId ?? previousSession?.sourceTabId ?? 'storage-fallback',
        updatedAt: nextSession?.updatedAt ?? Date.now(),
        session: nextSession,
      });
    };

    const channel = this.getBroadcastChannel();
    if (channel) {
      channel.addEventListener('message', handleBroadcast);
    } else {
      window.addEventListener('storage', handleStorage);
    }

    return () => {
      if (channel) {
        channel.removeEventListener('message', handleBroadcast);
      } else {
        window.removeEventListener('storage', handleStorage);
      }
    };
  }
}

export const sessionManager = new SessionManager();
