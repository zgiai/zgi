import type { ModuleName } from './loader';

const PUBLIC_MODULES: ModuleName[] = ['common', 'navigation'];
const AUTH_MODULES: ModuleName[] = ['common', 'auth', 'ui', 'navigation'];
const WEBAPP_MODULES: ModuleName[] = ['common', 'ui', 'webapp', 'files', 'agents', 'nodes'];
const APP_TOKEN_MODULES: ModuleName[] = ['common', 'ui', 'webapp', 'files', 'agents', 'nodes'];
const ANNOUNCEMENT_TOKEN_MODULES: ModuleName[] = ['common', 'ui', 'nodes'];
const PROFILE_MODULES: ModuleName[] = ['common', 'ui', 'navigation', 'profile'];

const CONSOLE_MODULES: ModuleName[] = [
  'common',
  'auth',
  'ui',
  'navigation',
  'workspace',
  'dashboard',
  'models',
  'aiProviders',
  'agents',
  'nodes',
  'datasets',
  'dbs',
  'files',
  'webapp',
  'automation',
  'contentParse',
  'prompts',
  'channels',
  'apikeys',
  'settings',
];

const DASHBOARD_MODULES: ModuleName[] = [
  'common',
  'auth',
  'ui',
  'navigation',
  'dashboard',
  'workspace',
  'aiProviders',
  'models',
  'channels',
  'apikeys',
  'market',
  'settings',
];

const AUTH_SEGMENTS = new Set([
  'activate',
  'forgot-password',
  'init',
  'invite',
  'login',
  'register',
  'reset-password',
  'sso',
  'verify',
]);

function uniqueModules(modules: ModuleName[]): ModuleName[] {
  return Array.from(new Set(modules));
}

function normalizePathname(pathname: string): string {
  const pathnameOnly = (pathname || '/').split('?')[0].split('#')[0] || '/';
  const segments = pathnameOnly.split('/').filter(Boolean);
  const knownRootIndex = segments.findIndex(segment =>
    [
      'a',
      'activate',
      'console',
      'dashboard',
      'forgot-password',
      'init',
      'invite',
      'login',
      'n',
      'privacy',
      'profile',
      'register',
      'reset-password',
      'sso',
      'terms',
      'verify',
      'webapp',
    ].includes(segment)
  );
  const normalizedSegments = knownRootIndex >= 0 ? segments.slice(knownRootIndex) : segments;

  return `/${normalizedSegments.join('/')}`;
}

export function getModulesForPathname(pathname: string): ModuleName[] {
  const normalizedPathname = normalizePathname(pathname);
  const firstSegment = normalizedPathname.split('/').filter(Boolean)[0];

  if (!firstSegment) return PUBLIC_MODULES;
  if (AUTH_SEGMENTS.has(firstSegment)) return AUTH_MODULES;
  if (firstSegment === 'console') return uniqueModules(CONSOLE_MODULES);
  if (firstSegment === 'dashboard') return uniqueModules(DASHBOARD_MODULES);
  if (firstSegment === 'webapp') return uniqueModules(WEBAPP_MODULES);
  if (firstSegment === 'a') return uniqueModules(APP_TOKEN_MODULES);
  if (firstSegment === 'n') return uniqueModules(ANNOUNCEMENT_TOKEN_MODULES);
  if (firstSegment === 'profile') return uniqueModules(PROFILE_MODULES);

  return PUBLIC_MODULES;
}
