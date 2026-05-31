// src/lib/config.ts
// Centralized environment variable reader with fallback and type safety

/**
 * Current Node.js environment (development/production/test)
 */
export const NODE_ENV: string = process.env.NODE_ENV || 'development';

/**
 * Application name.
 * Built as a compile-time constant so prerendered metadata and runtime pages
 * always use the same brand name.
 */
export const APP_NAME: string = process.env.APP_NAME_STATIC || 'ZGI';

/**
 * Brand identity used for brand-specific UI affordances.
 * Keep this separate from display name so non-ZGI deployments can suppress
 * ZGI-only visual marks while still choosing their own app name/logo.
 */
export const BRAND_NAME: string = (
  readPublicEnvRaw('NEXT_PUBLIC_BRAND_NAME') ||
  process.env.APP_BRAND_STATIC ||
  APP_NAME
)
  .trim()
  .toLowerCase();

export const IS_ZGI_BRAND: boolean = BRAND_NAME === 'zgi';

/**
 * Read public env with runtime browser overrides for values that can safely
 * change after the image has been built.
 */
function readPublicEnvRaw(key: string): string | undefined {
  if (typeof window !== 'undefined' && typeof window.__ENV__ !== 'undefined' && window.__ENV__) {
    const v = window.__ENV__[key];
    if (typeof v === 'string') return v;
  }
  return process.env[key];
}

function normalizeBasePath(value: string | undefined): string {
  if (!value) return '';
  const trimmed = value.trim();
  if (!trimmed || trimmed === '/') return '';
  const prefixed = trimmed.startsWith('/') ? trimmed : `/${trimmed}`;
  return prefixed.replace(/\/+$/, '');
}

export const BASE_PATH: string = normalizeBasePath(
  readPublicEnvRaw('NEXT_PUBLIC_BASE_PATH') || readPublicEnvRaw('BASE_PATH')
);

export function withBasePath(path: string): string {
  if (!path.startsWith('/')) return path;
  if (!BASE_PATH) return path;
  if (path === BASE_PATH || path.startsWith(`${BASE_PATH}/`)) return path;
  return `${BASE_PATH}${path}`;
}

export function withBasePathIfInternal(url: string): string {
  if (!url) return url;
  if (/^[a-zA-Z][a-zA-Z\d+\-.]*:/.test(url) || url.startsWith('//')) return url;
  if (!url.startsWith('/')) return url;
  if (BASE_PATH && (url === BASE_PATH || url.startsWith(`${BASE_PATH}/`))) return url;
  return withBasePath(url);
}

function resolvePublicAssetUrl(value: string | undefined, fallbackPath: string): string {
  if (!value) return withBasePath(fallbackPath);
  return value.startsWith('/') ? withBasePath(value) : value;
}

function resolveOptionalPublicAssetUrl(value: string | undefined): string {
  if (!value) return '';
  const trimmed = value.trim();
  if (!trimmed) return '';
  return trimmed.startsWith('/') ? withBasePath(trimmed) : trimmed;
}

/**
 * Logo URL
 */
const rawLogoUrl = readPublicEnvRaw('NEXT_PUBLIC_LOGO_URL');
export const HAS_CUSTOM_LOGO_URL: boolean = Boolean(rawLogoUrl?.trim());
export const LOGO_URL: string = resolvePublicAssetUrl(
  rawLogoUrl,
  '/logo.png'
);

/**
 * Logo URL for dark theme
 */
const rawDarkLogoUrl = readPublicEnvRaw('NEXT_PUBLIC_DARK_LOGO_URL');
export const HAS_CUSTOM_DARK_LOGO_URL: boolean = Boolean(rawDarkLogoUrl?.trim());
export const DARK_LOGO_URL: string = resolvePublicAssetUrl(
  rawDarkLogoUrl,
  '/logo_dark.png'
);

/**
 * Custom favicon URL.
 * When empty, the app falls back to Next.js default favicon discovery.
 */
export const FAVICON_URL: string = resolveOptionalPublicAssetUrl(
  readPublicEnvRaw('NEXT_PUBLIC_FAVICON_URL')
);

export const LOGO_REDIRECT_URL: string = withBasePathIfInternal(
  readPublicEnvRaw('NEXT_PUBLIC_LOGO_REDIRECT_URL') || '/console'
);

/**
 * Redirect URL after logout completes.
 * Supports internal paths and full external URLs.
 */
export function getLogoutRedirectUrl(): string {
  return withBasePathIfInternal(readPublicEnvRaw('NEXT_PUBLIC_LOGOUT_REDIRECT_URL') || '/login');
}

const ICON_BG_BY_DEFAULT_THEME: Record<string, string> = {
  light: '#0847f7',
  dark: 'oklch(0.922 0 0)',
  violet: 'oklch(0.35 0.25 285)',
  'warm-orange': 'oklch(0.62 0.25 56)',
  'tech-blue': '#2450de',
  'graphite-cyan': '#0e7490',
  emerald: '#059669',
};

const CURRENT_DEFAULT_THEME_NAME: string = readPublicEnvRaw('NEXT_PUBLIC_DEFAULT_THEME') || 'light';
const DEFAULT_THEME_ICON_ALIASES: Record<string, string> = {
  'ai-young': 'violet',
  'whale-orange': 'warm-orange',
};
const CURRENT_DEFAULT_THEME_ICON_KEY: string =
  DEFAULT_THEME_ICON_ALIASES[CURRENT_DEFAULT_THEME_NAME] || CURRENT_DEFAULT_THEME_NAME;

export const ICON_BG: string =
  readPublicEnvRaw('NEXT_PUBLIC_ICON_BG') ||
  ICON_BG_BY_DEFAULT_THEME[CURRENT_DEFAULT_THEME_ICON_KEY] ||
  ICON_BG_BY_DEFAULT_THEME.light;
export const ICON_TEXT: string = readPublicEnvRaw('NEXT_PUBLIC_ICON_TEXT') || 'z';

/**
 * App version from environment
 */
export const APP_VERSION: string = readPublicEnvRaw('NEXT_PUBLIC_APP_VERSION') || '0.0.0';

/**
 * API base URL
 * In browser, try to use current host if NEXT_PUBLIC_API_URL is not set
 */
export const API_URL: string = (() => {
  // Server-side or build-time
  if (typeof window === 'undefined') {
    return readPublicEnvRaw('NEXT_PUBLIC_API_URL') || 'http://localhost:2679';
  }

  // Client-side: use NEXT_PUBLIC_API_URL if set, otherwise use current host with API port
  const runtimeApi = readPublicEnvRaw('NEXT_PUBLIC_API_URL');
  if (runtimeApi) return runtimeApi;

  // Auto-detect from current location
  const { protocol, hostname } = window.location;
  // If accessing via standard ports (80/443), assume API is on same domain
  if (
    window.location.port === '' ||
    window.location.port === '80' ||
    window.location.port === '443'
  ) {
    return `${protocol}//${hostname}`;
  }
  // Otherwise use the local public gateway port.
  return `${protocol}//${hostname}:2679`;
})();

/**
 * Auth API base URL (optional, will be derived from API_URL if not provided)
 */
export const AUTH_API_URL: string = readPublicEnvRaw('NEXT_PUBLIC_AUTH_API_URL') || '';

/**
 * Upload API base URL (optional, will be derived from API_URL if not provided)
 */
export const UPLOAD_API_URL: string = readPublicEnvRaw('NEXT_PUBLIC_UPLOAD_API_URL') || '';

function parsePublicEnvList(value: string | undefined): string[] {
  if (!value) return [];
  return value
    .split(',')
    .map(item => item.trim())
    .filter(Boolean);
}

export const FILE_PREVIEW_ALLOWED_ORIGINS: string[] = parsePublicEnvList(
  readPublicEnvRaw('NEXT_PUBLIC_FILE_PREVIEW_ALLOWED_ORIGINS')
);

/**
 * Market API base URL (for marketplace plugins, uses separate endpoint)
 */
export const MARKET_API_URL: string = readPublicEnvRaw('NEXT_PUBLIC_MARKET_API_URL') || '';

/**
 * Marketplace distribution channel. When set, plugin list requests explicitly
 * ask the market API for this channel instead of relying on host-based routing.
 */
export const MARKETPLACE_CHANNEL: string = (
  readPublicEnvRaw('NEXT_PUBLIC_MARKETPLACE_CHANNEL') || ''
).trim();

/**
 * Enable language switcher in UI
 */
export const ENABLE_LANG_SWITCH: boolean =
  readPublicEnvRaw('NEXT_PUBLIC_ENABLE_LANG_SWITCH') !== 'false';

/**
 * Enable theme switcher in UI
 * When false, theme switching is disabled and the app uses DEFAULT_THEME_NAME
 */
export const ENABLE_THEME_SWITCH: boolean =
  readPublicEnvRaw('NEXT_PUBLIC_ENABLE_THEME_SWITCH') !== 'false';

/**
 * Default theme name (light | dark | violet | warm-orange | tech-blue | graphite-cyan | emerald)
 * Legacy values ai-young and whale-orange are still accepted.
 * Used when theme switching is disabled or no user preference is stored
 */
export const DEFAULT_THEME_NAME: string = readPublicEnvRaw('NEXT_PUBLIC_DEFAULT_THEME') || 'light';

/**
 * Default locale for the app
 */
export const DEFAULT_LOCALE: string = readPublicEnvRaw('NEXT_PUBLIC_DEFAULT_LOCALE') || 'zh-Hans';

/**
 * Workflow editor autosave configuration
 */
function parsePositiveInt(value: string | undefined, fallback: number): number {
  if (typeof value !== 'string') return fallback;
  const parsed = Number.parseInt(value, 10);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : fallback;
}

// Interval between autosave ticks in milliseconds
export const WORKFLOW_AUTOSAVE_INTERVAL_MS: number = parsePositiveInt(
  readPublicEnvRaw('NEXT_PUBLIC_WORKFLOW_AUTOSAVE_INTERVAL_MS'),
  30_000
);

// UI stream rendering throttle interval in milliseconds
export const STREAM_RENDER_THROTTLE_MS: number = parsePositiveInt(
  readPublicEnvRaw('NEXT_PUBLIC_STREAM_RENDER_THROTTLE_MS'),
  120
);

/**
 * Auth page promotional text (left panel) - Bilingual support
 */
export const AUTH_TITLE_LINE1_EN: string =
  readPublicEnvRaw('NEXT_PUBLIC_AUTH_TITLE_LINE1_EN') || 'With AI Precision';

export const AUTH_TITLE_LINE1_ZH: string =
  readPublicEnvRaw('NEXT_PUBLIC_AUTH_TITLE_LINE1_ZH') || '以 AI 精准驱动';

export const AUTH_TITLE_LINE2_EN: string =
  readPublicEnvRaw('NEXT_PUBLIC_AUTH_TITLE_LINE2_EN') || 'Transform Your Workflow.';

export const AUTH_TITLE_LINE2_ZH: string =
  readPublicEnvRaw('NEXT_PUBLIC_AUTH_TITLE_LINE2_ZH') || '重塑您的工作流程。';

export const AUTH_DESCRIPTION_EN: string =
  readPublicEnvRaw('NEXT_PUBLIC_AUTH_DESCRIPTION_EN') ||
  'Powerful AI-driven platform designed for intelligent automation and deep insights. Step into the next generation of productivity.';

export const AUTH_DESCRIPTION_ZH: string =
  readPublicEnvRaw('NEXT_PUBLIC_AUTH_DESCRIPTION_ZH') ||
  '强大的 AI 驱动平台，专为智能自动化和深度洞察而设计。迈入下一代生产力时代。';

/**
 * Auth page sidebar background image URL
 * When set, replaces the default generative background with a custom image
 */
export const AUTH_BG_IMAGE: string = readPublicEnvRaw('NEXT_PUBLIC_AUTH_BG_IMAGE') || '';

/**
 * Console AI chat conversation sidebar background image URL.
 * Used for brand-specific deployments without forking chat UI.
 */
export const AICHAT_SIDEBAR_BG_IMAGE: string = resolveOptionalPublicAssetUrl(
  readPublicEnvRaw('NEXT_PUBLIC_AICHAT_SIDEBAR_BG_IMAGE')
);

/**
 * Web app chat conversation history sidebar background image URL.
 * Used for brand-specific deployments without forking chat UI.
 */
export const WEBAPP_CHAT_SIDEBAR_BG_IMAGE: string = resolveOptionalPublicAssetUrl(
  readPublicEnvRaw('NEXT_PUBLIC_WEBAPP_CHAT_SIDEBAR_BG_IMAGE')
);

/**
 * Hide the auth page left promotional panel
 * When true, auth pages render as a single-column layout
 */
export const HIDE_AUTH_LEFT_PANEL: boolean =
  readPublicEnvRaw('NEXT_PUBLIC_HIDE_AUTH_LEFT_PANEL') === 'true';

/**
 * Disable redirecting authenticated users away from auth pages.
 * When true, logged-in users may stay on /login, /register, and /forgot-password.
 */
export const DISABLE_AUTHENTICATED_REDIRECT_ON_AUTH_PAGES: boolean =
  readPublicEnvRaw('NEXT_PUBLIC_DISABLE_AUTHENTICATED_REDIRECT_ON_AUTH_PAGES') === 'true';

/**
 * Force auth pages to use a single SSO provider.
 * When set, auth pages redirect directly to the configured provider start endpoint.
 */
export const SINGLE_SSO_PROVIDER: string = (
  readPublicEnvRaw('NEXT_PUBLIC_SINGLE_SSO_PROVIDER') || ''
)
  .trim()
  .toLowerCase();

/**
 * Is this the official cloud version?
 * Controls visibility of official channels, cost center features, and sync buttons
 */
export const IS_CLOUD: boolean = readPublicEnvRaw('NEXT_PUBLIC_IS_CLOUD') === 'true';

export const ROOT_COOKIE_DOMAIN: string = (readPublicEnvRaw('NEXT_PUBLIC_ROOT_COOKIE_DOMAIN') || '')
  .trim()
  .toLowerCase();

export const ENABLE_ROOT_COOKIE_TOKEN_SYNC: boolean = ROOT_COOKIE_DOMAIN.length > 0;

/**
 * Agent detail features that are still being refined.
 * Keep them hidden by default while retaining the implementation behind
 * explicit public flags for local validation or customer-specific builds.
 */
export const ENABLE_AGENT_API_PAGE: boolean =
  readPublicEnvRaw('NEXT_PUBLIC_ENABLE_AGENT_API_PAGE') === 'true';

export const ENABLE_AGENT_BATCH_TEST_PAGE: boolean =
  readPublicEnvRaw('NEXT_PUBLIC_ENABLE_AGENT_BATCH_TEST_PAGE') === 'true';

// Add more env variables here as needed, always use this file for env access
