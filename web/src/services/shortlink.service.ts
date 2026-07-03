import { buildApiUrl } from '@/lib/http/config';
import type { ApiResponseData } from './types/common';

const SHORT_TOKEN_ALPHABET = '23456789abcdefghkmnpqrstuvwxyz';
const SHORT_TOKEN_PATTERN = new RegExp(`^[${SHORT_TOKEN_ALPHABET}]{8}$`);

interface ShortLinkResolution {
  target_path: string;
}

export function normalizeShortToken(token: string): string {
  return token.trim().toLowerCase();
}

export function isValidShortToken(token: string): boolean {
  return SHORT_TOKEN_PATTERN.test(normalizeShortToken(token));
}

export async function resolveShortLinkTargetPath(token: string): Promise<string | null> {
  const shortToken = normalizeShortToken(token);
  if (!isValidShortToken(shortToken)) {
    return null;
  }

  const path = `/console/api/short-link-resolutions/${encodeURIComponent(shortToken)}`;
  const response = await fetch(
    buildShortLinkResolutionUrl(path),
    {
      method: 'GET',
      headers: {
        Accept: 'application/json',
      },
      cache: 'no-store',
    }
  );

  if (!response.ok) {
    return null;
  }

  const payload = (await response.json()) as ApiResponseData<ShortLinkResolution>;
  if (payload.code !== '0' || !isSafeTargetPath(payload.data?.target_path)) {
    return null;
  }
  return payload.data.target_path;
}

function buildShortLinkResolutionUrl(path: string): string {
  const cleanPath = path.startsWith('/') ? path : `/${path}`;
  if (typeof window === 'undefined') {
    const serverApiUrl = process.env.API_URL?.trim();
    if (serverApiUrl) {
      return `${serverApiUrl.replace(/\/+$/, '')}${cleanPath}`;
    }
  }
  return buildApiUrl(cleanPath, 'main');
}

function isSafeTargetPath(path: unknown): path is string {
  if (typeof path !== 'string') {
    return false;
  }
  if (path.startsWith('//') || path.includes('?') || path.includes('#')) {
    return false;
  }
  return /^\/a\/[^/]+$/.test(path) || /^\/n\/[^/]+$/.test(path);
}
