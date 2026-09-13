import { timezones } from '@/lib/constants';

function timezoneLocale(locale?: string): 'zh-Hans' | 'en-US' {
  return locale?.toLowerCase().startsWith('zh') ? 'zh-Hans' : 'en-US';
}

function formatOffset(timezone: string, locale: 'zh-Hans' | 'en-US'): string | null {
  try {
    const value = new Intl.DateTimeFormat(locale, {
      timeZone: timezone,
      timeZoneName: 'longOffset',
    })
      .formatToParts(new Date())
      .find(part => part.type === 'timeZoneName')?.value;
    if (!value) return null;
    return value.replace(/^GMT/, 'UTC');
  } catch {
    return null;
  }
}

/**
 * Returns a localized, human-readable timezone label while keeping the IANA
 * identifier out of the primary UI. Unknown but valid IANA zones fall back to
 * the localized Intl name and offset; invalid values are returned unchanged so
 * operators can still diagnose malformed data.
 */
export function translateTimezone(timezone: string | null | undefined, locale?: string): string {
  const normalized = timezone?.trim();
  if (!normalized) return '';

  const resolvedLocale = timezoneLocale(locale);
  const known = timezones.find(item => item.value === normalized);
  if (known) {
    return `${known.label[resolvedLocale]} (${known.offset})`;
  }

  try {
    const name = new Intl.DateTimeFormat(resolvedLocale, {
      timeZone: normalized,
      timeZoneName: 'long',
    })
      .formatToParts(new Date())
      .find(part => part.type === 'timeZoneName')?.value;
    const offset = formatOffset(normalized, resolvedLocale);
    if (name && name !== normalized) return offset ? `${name} (${offset})` : name;
  } catch {
    // Keep the raw value as a safe fallback for invalid or unsupported zones.
  }

  return normalized;
}
