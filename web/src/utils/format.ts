import type { User } from '@/services/types/auth';
import {
  CLIENT_CACHE_KEYS,
  LEGACY_CLIENT_CACHE_COOKIE_KEYS,
  PROFILE_CLIENT_CACHE_TTL_MS,
  readClientCacheWithLegacyCookie,
} from '@/utils/client-cache';
import { useAuthStore } from '@/store/auth-store';

/*
 * Common formatting utilities
 * --------------------------------------------------------
 * - formatFileSize: Convert bytes to human-readable string
 * - formatDate: Convert unix timestamp (seconds or milliseconds) to formatted date-time string
 */

// Format file size in human-readable units
export function formatFileSize(bytes?: number | null, decimals = 1): string {
  if (!bytes || bytes <= 0) return '-';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  const value = parseFloat((bytes / Math.pow(k, i)).toFixed(decimals));
  return `${value} ${sizes[i]}`;
}

export function formatNumber(num?: number | string | null, decimals = 1): string {
  if (num === undefined || num === null || num === '') return '-';
  const n = Number(num);
  if (isNaN(n)) return '-';
  if (n === 0) return '0';

  // Handle negative numbers
  if (n < 0) return `-${formatNumber(-n, decimals)}`;
  // Handle numbers less than 1
  if (n < 1) return n.toFixed(decimals);
  const k = 1000;
  const sizes = ['', 'K', 'M', 'B', 'T'];
  const i = Math.min(Math.floor(Math.log(n) / Math.log(k)), sizes.length - 1);
  const value = parseFloat((n / Math.pow(k, i)).toFixed(decimals));
  return `${value}${sizes[i]}`;
}

function getUserTimezone(): string {
  const fallbackTz = 'Asia/Shanghai';
  if (typeof window !== 'undefined') {
    try {
      const cached = readClientCacheWithLegacyCookie<User>({
        key: CLIENT_CACHE_KEYS.profile,
        legacyCookieKey: LEGACY_CLIENT_CACHE_COOKIE_KEYS.profile,
        ttlMs: PROFILE_CLIENT_CACHE_TTL_MS,
      });
      const tz = cached?.timezone;
      if (typeof tz === 'string' && tz.trim().length > 0) return tz;
    } catch {
      console.error('Failed to get user timezone from client cache');
    }
    try {
      const tz = useAuthStore.getState().user?.timezone;
      if (typeof tz === 'string' && tz.trim().length > 0) {
        return tz;
      }
    } catch {
      console.error('Failed to get user timezone from auth store');
    }
    try {
      const tz = Intl.DateTimeFormat().resolvedOptions().timeZone;
      if (typeof tz === 'string' && tz.trim().length > 0) return tz;
    } catch {
      console.error('Failed to get user timezone from Intl.DateTimeFormat');
    }
  }
  return fallbackTz;
}

function parseAsUTC(input: number | string | Date): Date {
  if (input instanceof Date) return input;
  if (typeof input === 'number') {
    const ms = input < 1e12 ? input * 1000 : input;
    return new Date(ms);
  }
  const s = String(input).trim();
  const asNum = Number(s);
  if (!Number.isNaN(asNum) && isFinite(asNum)) {
    const ms = asNum < 1e12 ? asNum * 1000 : asNum;
    return new Date(ms);
  }
  const hasExplicitTz = /Z$/i.test(s) || /[+-]\d{2}:?\d{2}$/.test(s) || /\bUTC\b|\bGMT\b/i.test(s);
  if (hasExplicitTz) {
    return new Date(s);
  }
  let iso = s.replace(' ', 'T');
  if (!/T/.test(iso)) iso = `${iso}T00:00:00`;
  if (!/Z$/i.test(iso)) iso = `${iso}Z`;
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? new Date(s) : d;
}

// Format timestamp to a formatted string using tokens like 'YYYY-MM-DD HH:mm:ss'
export function formatDate(
  timestamp: number | string | Date,
  format: string = 'YYYY-MM-DD HH:mm',
  formatOptions?: { locale?: string; timezone?: string }
): string {
  const locale = formatOptions?.locale ?? 'zh-CN';
  const timezone = formatOptions?.timezone ?? getUserTimezone();

  const date = parseAsUTC(timestamp);
  if (isNaN(date.getTime())) return 'Invalid Date';

  // Use Intl with target timezone, then map parts to tokens
  const dtf = new Intl.DateTimeFormat(locale, {
    timeZone: timezone,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  });
  const parts = dtf.formatToParts(date);
  const kv: Record<string, string> = {};
  for (const p of parts) {
    if (
      p.type === 'year' ||
      p.type === 'month' ||
      p.type === 'day' ||
      p.type === 'hour' ||
      p.type === 'minute' ||
      p.type === 'second'
    ) {
      kv[p.type] = p.value;
    }
  }

  let out = format;
  out = out.replace(/YYYY/g, kv.year ?? '');
  out = out.replace(/MM/g, kv.month ?? '');
  out = out.replace(/DD/g, kv.day ?? '');
  out = out.replace(/HH/g, kv.hour ?? '');
  out = out.replace(/mm/g, kv.minute ?? '');
  out = out.replace(/ss/g, kv.second ?? '');
  return out;
}

export function formatMs(ms: number): string {
  if (ms <= 0) return '0ms';
  const k = 1000;
  const sizes = ['ms', 's', 'min', 'h'];
  const thresholds = [1, k, k * 60, k * 3600];
  let sizeIndex = 0;

  while (sizeIndex < thresholds.length - 1 && ms >= thresholds[sizeIndex + 1]) {
    sizeIndex++;
  }

  const value = parseFloat((ms / thresholds[sizeIndex]).toFixed(1));
  return `${value}${sizes[sizeIndex]}`;
}

export function formatWorkflowElapsedMs(elapsedMs?: number | null): string {
  if (elapsedMs === undefined || elapsedMs === null || elapsedMs <= 0) return '0ms';
  return formatMs(elapsedMs);
}

export function formatDurationSeconds(seconds?: number | null): string {
  if (seconds === undefined || seconds === null || seconds <= 0) return '0s';

  if (seconds < 1) {
    return `${parseFloat(seconds.toFixed(3))}s`;
  }

  if (seconds < 60) {
    return `${parseFloat(seconds.toFixed(seconds < 10 ? 2 : 1))}s`;
  }

  const minutes = Math.floor(seconds / 60);
  const remainingSeconds = parseFloat((seconds % 60).toFixed(1));

  if (minutes < 60) {
    return remainingSeconds > 0 ? `${minutes}m ${remainingSeconds}s` : `${minutes}m`;
  }

  const hours = Math.floor(minutes / 60);
  const remainingMinutes = minutes % 60;
  return remainingMinutes > 0 ? `${hours}h ${remainingMinutes}m` : `${hours}h`;
}

// Format token count to human-readable string (e.g., 128000 -> '128K')
export function formatTokens(value?: number): string {
  if (!value || value <= 0) return '-';
  if (value >= 1_000_000) return `${Math.round(value / 1_000_000)}M`;
  if (value >= 1000) return `${Math.round(value / 1000)}K`;
  return String(value);
}
