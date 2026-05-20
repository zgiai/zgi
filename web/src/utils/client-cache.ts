import { deleteCookie, getCookie } from '@/utils/cookie';

export const CLIENT_CACHE_KEYS = {
  profile: 'zgi:cache:profile',
  systemFeatures: 'zgi:cache:system-features',
  builtInWorkflows: 'zgi:cache:built-in-workflows',
} as const;

export const LEGACY_CLIENT_CACHE_COOKIE_KEYS = {
  profile: 'zgi_profile_cache',
  systemFeatures: 'zgi_system_features_cache',
  builtInWorkflows: 'zgi:built-in-workflows',
} as const;

export const PROFILE_CLIENT_CACHE_TTL_MS = 3 * 24 * 60 * 60 * 1000;
export const SYSTEM_FEATURES_CLIENT_CACHE_TTL_MS = 3 * 24 * 60 * 60 * 1000;
export const BUILT_IN_WORKFLOWS_CLIENT_CACHE_TTL_MS = 7 * 24 * 60 * 60 * 1000;

interface ClientCacheEnvelope<T> {
  value: T;
  expiresAt: number | null;
}

interface ReadClientCacheOptions<T> {
  validate?: (value: unknown) => value is T;
}

interface ReadClientCacheWithLegacyCookieOptions<T> extends ReadClientCacheOptions<T> {
  key: string;
  legacyCookieKey: string;
  ttlMs?: number;
}

function isBrowser(): boolean {
  return typeof window !== 'undefined';
}

function parseEnvelope<T>(
  raw: string | null,
  validate?: (value: unknown) => value is T
): ClientCacheEnvelope<T> | null {
  if (!raw) {
    return null;
  }

  try {
    const parsed = JSON.parse(raw) as Partial<ClientCacheEnvelope<unknown>>;
    const expiresAt =
      typeof parsed.expiresAt === 'number' || parsed.expiresAt === null ? parsed.expiresAt : null;

    if (!('value' in parsed)) {
      return null;
    }

    if (validate && !validate(parsed.value)) {
      return null;
    }

    return {
      value: parsed.value as T,
      expiresAt,
    };
  } catch {
    return null;
  }
}

/**
 * @util Read a cached value from localStorage and discard it when expired.
 */
export function readClientCache<T>(
  key: string,
  options?: ReadClientCacheOptions<T>
): T | null {
  if (!isBrowser()) {
    return null;
  }

  try {
    const envelope = parseEnvelope<T>(window.localStorage.getItem(key), options?.validate);
    if (!envelope) {
      return null;
    }

    if (typeof envelope.expiresAt === 'number' && envelope.expiresAt <= Date.now()) {
      removeClientCache(key);
      return null;
    }

    return envelope.value;
  } catch {
    return null;
  }
}

/**
 * @util Persist a cached value into localStorage with an optional TTL.
 */
export function writeClientCache<T>(key: string, value: T, ttlMs?: number): void {
  if (!isBrowser()) {
    return;
  }

  try {
    const envelope: ClientCacheEnvelope<T> = {
      value,
      expiresAt: typeof ttlMs === 'number' ? Date.now() + ttlMs : null,
    };
    window.localStorage.setItem(key, JSON.stringify(envelope));
  } catch {
    // Ignore storage write failures.
  }
}

/**
 * @util Remove a cached value from localStorage.
 */
export function removeClientCache(key: string): void {
  if (!isBrowser()) {
    return;
  }

  try {
    window.localStorage.removeItem(key);
  } catch {
    // Ignore storage removal failures.
  }
}

/**
 * @util Read a local cache first and migrate a legacy cookie value into localStorage when needed.
 */
export function readClientCacheWithLegacyCookie<T>({
  key,
  legacyCookieKey,
  ttlMs,
  validate,
}: ReadClientCacheWithLegacyCookieOptions<T>): T | null {
  const cached = readClientCache<T>(key, { validate });
  if (cached !== null) {
    return cached;
  }

  if (!isBrowser()) {
    return null;
  }

  try {
    const legacy = getCookie<unknown>(legacyCookieKey);
    if (legacy === null || (validate && !validate(legacy))) {
      return null;
    }

    writeClientCache(key, legacy as T, ttlMs);
    deleteCookie(legacyCookieKey);
    return legacy as T;
  } catch {
    return null;
  }
}

/**
 * @util Clear the profile client cache from localStorage and remove the legacy cookie.
 */
export function clearProfileClientCache(): void {
  removeClientCache(CLIENT_CACHE_KEYS.profile);
  deleteCookie(LEGACY_CLIENT_CACHE_COOKIE_KEYS.profile);
}

/**
 * @util Clear the system-features client cache from localStorage and remove the legacy cookie.
 */
export function clearSystemFeaturesClientCache(): void {
  removeClientCache(CLIENT_CACHE_KEYS.systemFeatures);
  deleteCookie(LEGACY_CLIENT_CACHE_COOKIE_KEYS.systemFeatures);
}

/**
 * @util Clear the built-in-workflows client cache from localStorage and remove the legacy cookie.
 */
export function clearBuiltInWorkflowsClientCache(): void {
  removeClientCache(CLIENT_CACHE_KEYS.builtInWorkflows);
  deleteCookie(LEGACY_CLIENT_CACHE_COOKIE_KEYS.builtInWorkflows);
}

/**
 * @util Clear auth-related client caches that should be refreshed after auth/context changes.
 */
export function clearAuthClientCaches(): void {
  clearProfileClientCache();
  clearSystemFeaturesClientCache();
}
