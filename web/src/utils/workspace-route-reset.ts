'use client';

type SearchParamsLike = Pick<URLSearchParams, 'has' | 'toString'>;

const RESOURCE_DETAIL_ROUTES: Array<{ pattern: RegExp; target: string }> = [
  { pattern: /^\/console\/workflows\/[^/]+(?:\/.*)?$/, target: '/console/workflows' },
  { pattern: /^\/console\/agents\/[^/]+(?:\/.*)?$/, target: '/console/agents' },
  { pattern: /^\/console\/dataset\/[^/]+(?:\/.*)?$/, target: '/console/dataset' },
  { pattern: /^\/console\/db\/[^/]+(?:\/.*)?$/, target: '/console/db' },
  { pattern: /^\/console\/prompts\/[^/]+(?:\/.*)?$/, target: '/console/prompts' },
  { pattern: /^\/console\/work\/app\/[^/]+(?:\/.*)?$/, target: '/console/work/app' },
];

function buildPath(pathname: string, searchParams: URLSearchParams) {
  const query = searchParams.toString();
  return query ? `${pathname}?${query}` : pathname;
}

function toSearchParams(searchParams?: SearchParamsLike | string | null) {
  if (!searchParams) {
    return new URLSearchParams();
  }

  return new URLSearchParams(
    typeof searchParams === 'string' ? searchParams : searchParams.toString()
  );
}

/**
 * Returns a safe route for workspace switching when the current URL points to
 * workspace-scoped detail state that should not survive across workspaces.
 */
export function getWorkspaceSwitchRedirect(
  pathname: string,
  searchParams?: SearchParamsLike | string | null
): string | null {
  const detailRoute = RESOURCE_DETAIL_ROUTES.find(route => route.pattern.test(pathname));
  if (detailRoute) {
    return detailRoute.target;
  }

  const nextParams = toSearchParams(searchParams);

  if (
    (pathname === '/console/work/chat' || pathname === '/console/work/image') &&
    nextParams.has('convId')
  ) {
    nextParams.delete('convId');
    return buildPath(pathname, nextParams);
  }

  if (pathname === '/console/work/task') {
    const shouldResetPanel =
      nextParams.has('taskId') || nextParams.has('mode') || nextParams.has('tab');
    if (shouldResetPanel) {
      nextParams.delete('taskId');
      nextParams.delete('mode');
      nextParams.delete('tab');
      return buildPath(pathname, nextParams);
    }
  }

  if (pathname === '/console/prompts' && nextParams.has('promptId')) {
    nextParams.delete('promptId');
    return buildPath(pathname, nextParams);
  }

  return null;
}
