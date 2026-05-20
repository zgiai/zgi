import type { ProviderMetadata } from '@/services/types/provider';

interface ProviderNameTranslator {
  (key: string, values?: Record<string, string | number | Date>): string;
  has?: (key: string) => boolean;
}

export interface TranslateProviderNameOptions {
  fallbackName?: string | null;
  t?: ProviderNameTranslator;
}

export interface ProviderDisplayInfo {
  name: string;
  description: string;
}

export interface ProviderDisplayTarget {
  provider?: string | null;
  provider_name?: string | null;
  description?: string | null;
  tagline?: string | null;
  metadata?: ProviderMetadata | null;
}

export interface ResolveProviderDisplayOptions {
  locale?: string | null;
  getProviderName?: (provider?: string | null, fallbackName?: string | null) => string;
}

const PROVIDER_CANONICAL_ALIASES: Record<string, string> = {
  '01ai': 'zeroone',
  '360ai': 'ai360',
  anthropic: 'anthropic',
  baichuan: 'baichuan',
  baidu: 'wenxin',
  cohere: 'cohere',
  deepseek: 'deepseek',
  doubao: 'doubao',
  gemini: 'google',
  glm: 'zhipu',
  google: 'google',
  iflytek: 'spark',
  'infini-ai': 'infiniai',
  infinigence: 'infiniai',
  minimax: 'minimax',
  mistral: 'mistral',
  moonshot: 'moonshot',
  moonshotai: 'moonshot',
  nvidia: 'nvidia',
  ollama: 'ollama',
  openai: 'openai',
  qianfan: 'wenxin',
  qwen: 'qwen',
  sensenova: 'sensenova',
  sensetime: 'sensenova',
  siliconcloud: 'siliconcloud',
  siliconflow: 'siliconcloud',
  spark: 'spark',
  tencent: 'tencentcloud',
  tencentcloud: 'tencentcloud',
  wenxin: 'wenxin',
  xai: 'xai',
  yi: 'zeroone',
  zeroone: 'zeroone',
  zhipu: 'zhipu',
  zhipuai: 'zhipu',
};

const PROVIDER_ICON_KEYS: Record<string, string> = {
  ai360: 'ai360',
  anthropic: 'anthropic',
  baichuan: 'baichuan',
  cohere: 'cohere',
  deepseek: 'deepseek',
  doubao: 'doubao',
  google: 'google',
  infinigence: 'infinigence',
  meta: 'meta',
  minimax: 'minimax',
  mistral: 'mistral',
  moonshot: 'moonshot',
  nvidia: 'nvidia',
  ollama: 'ollama',
  openai: 'openai',
  qwen: 'qwen',
  sensenova: 'sensenova',
  siliconcloud: 'siliconcloud',
  spark: 'spark',
  tencentcloud: 'tencentcloud',
  wenxin: 'wenxin',
  xai: 'xai',
  zeroone: 'zeroone',
  zhipu: 'zhipu',
};

const SPLITTER = /[/:|]/;

function normalizeProviderKey(input: string): string {
  return input.trim().toLowerCase().replace(/\s+/g, '');
}

/**
 * @util Resolve a provider identifier into a canonical brand key shared by
 * icon and i18n display helpers.
 */
export function resolveProviderCanonicalKey(provider?: string | null): string {
  const raw = provider?.trim();
  if (!raw) return 'unknown';

  const normalized = normalizeProviderKey(raw);
  const segments = normalized.split(SPLITTER).filter(Boolean);
  const lastSegment = segments.at(-1) || normalized;
  const candidates = [normalized, ...segments];

  const direct = candidates.find(candidate => PROVIDER_CANONICAL_ALIASES[candidate]);

  if (direct) {
    return PROVIDER_CANONICAL_ALIASES[direct];
  }

  if (normalized.includes('siliconflow')) return 'siliconcloud';
  if (normalized.includes('sensetime')) return 'sensenova';
  if (normalized.includes('iflytek') || normalized.includes('xinghuo')) return 'spark';
  if (normalized.includes('baidu') || normalized.includes('qianfan')) return 'wenxin';
  if (normalized.includes('tencent')) return 'tencentcloud';
  if (normalized.includes('moonshot') || normalized.includes('kimi')) return 'moonshot';
  if (normalized.includes('zhipu') || normalized.includes('glm')) return 'zhipu';
  if (normalized.includes('360')) return 'ai360';
  if (normalized.includes('yi') || normalized.includes('0lai') || normalized.includes('01ai')) {
    return 'zeroone';
  }
  if (normalized.includes('infini')) return 'infiniai';
  if (normalized.includes('meta')) return 'meta';
  if (normalized.includes('neta')) return 'nvidia';

  return lastSegment;
}

/**
 * @util Resolve a provider identifier into a @lobehub/icons provider key.
 */
export function resolveProviderIconKey(provider?: string | null): string {
  const canonicalKey = resolveProviderCanonicalKey(provider);
  return PROVIDER_ICON_KEYS[canonicalKey] || canonicalKey;
}

/**
 * @util Translate a provider identifier into a localized display name with a
 * graceful fallback to the API-provided or raw provider name.
 */
export function translateProviderName(
  provider?: string | null,
  options: TranslateProviderNameOptions = {}
): string {
  const fallbackName = options.fallbackName?.trim();
  const rawProvider = provider?.trim();
  const fallback = fallbackName || rawProvider || '';

  if (!fallback) {
    return '';
  }

  const canonicalKey = resolveProviderCanonicalKey(rawProvider || fallbackName);
  const messageKey = `providers.${canonicalKey}`;
  const translator = options.t;

  if (!translator) {
    return fallback;
  }

  if (typeof translator.has === 'function' && !translator.has(messageKey)) {
    return fallback;
  }

  try {
    const translated = translator(messageKey);
    if (!translated || translated === messageKey || translated.startsWith('providers.')) {
      return fallback;
    }
    return translated;
  } catch {
    return fallback;
  }
}

function resolveLocalizedProviderMetadata(
  metadata?: ProviderMetadata | null,
  locale?: string | null
): NonNullable<ProviderMetadata['i18n']>[string] | null {
  const i18n = metadata?.i18n;
  if (!i18n) return null;

  const normalizedLocale = locale?.trim();
  const potentialKeys = normalizedLocale ? [normalizedLocale] : [];

  if (normalizedLocale?.includes('-')) {
    potentialKeys.push(normalizedLocale.split('-')[0] || normalizedLocale);
  }

  for (const key of potentialKeys) {
    if (i18n[key]) {
      return i18n[key];
    }
  }

  return null;
}

/**
 * @util Resolve provider display info with the same priority order used by the provider cards.
 */
export function resolveProviderDisplayInfo(
  provider?: ProviderDisplayTarget | null,
  options: ResolveProviderDisplayOptions = {}
): ProviderDisplayInfo {
  if (!provider) {
    return { name: '', description: '' };
  }

  const localized = resolveLocalizedProviderMetadata(provider.metadata, options.locale);
  const fallbackName =
    options.getProviderName?.(provider.provider, provider.provider_name) ||
    provider.provider_name ||
    provider.provider ||
    '';

  return {
    name: localized?.provider_name || fallbackName,
    description: localized?.description || localized?.tagline || provider.description || provider.tagline || '',
  };
}
