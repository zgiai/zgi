export type ConsoleRouteScope = 'organization' | 'workspace';

export interface ConsoleRouteAccess {
  scope: ConsoleRouteScope;
  requiresWorkspace: boolean;
}

export const ORGANIZATION_SCOPED_CONSOLE_ROUTES = [
  '/console/settings',
  '/console/work',
  '/console/work/chat',
  '/console/work/image',
  '/console/work/app',
] as const;

export const ORGANIZATION_SCOPED_CONSOLE_ROUTE_PREFIXES = ['/console/work/app/'] as const;

export const ORGANIZATION_SCOPED_WORK_ROUTES = [
  '/console/work',
  '/console/work/chat',
  '/console/work/image',
  '/console/work/app',
] as const;

export const ORGANIZATION_SCOPED_WORK_ROUTE_PREFIXES = ['/console/work/app/'] as const;

function matchesRoute(
  pathname: string,
  exactRoutes: readonly string[],
  routePrefixes: readonly string[]
) {
  return exactRoutes.includes(pathname) || routePrefixes.some(prefix => pathname.startsWith(prefix));
}

export function getConsoleRouteAccess(pathname: string): ConsoleRouteAccess {
  if (isOrganizationScopedConsoleRoute(pathname)) {
    return {
      scope: 'organization',
      requiresWorkspace: false,
    };
  }

  return {
    scope: 'workspace',
    requiresWorkspace: true,
  };
}

export function isOrganizationScopedConsoleRoute(pathname: string) {
  return matchesRoute(
    pathname,
    ORGANIZATION_SCOPED_CONSOLE_ROUTES,
    ORGANIZATION_SCOPED_CONSOLE_ROUTE_PREFIXES
  );
}

export function isOrganizationScopedWorkRoute(pathname: string) {
  return matchesRoute(
    pathname,
    ORGANIZATION_SCOPED_WORK_ROUTES,
    ORGANIZATION_SCOPED_WORK_ROUTE_PREFIXES
  );
}
