import { API_URL, withBasePathIfInternal } from '@/lib/config';

interface SearchParamsLike {
  get(name: string): string | null;
  toString(): string;
}

export function buildSsoStartUrl(provider: string, redirect?: string | null): string {
  const normalizedProvider = provider.trim().toLowerCase();
  const normalizedRedirect = redirect ? withBasePathIfInternal(redirect) : '';
  const baseUrl = `${API_URL}/console/api/sso/${normalizedProvider}/start`;

  if (!normalizedRedirect) {
    return baseUrl;
  }

  return `${baseUrl}?redirect=${encodeURIComponent(normalizedRedirect)}`;
}

export function shouldAutoRedirectToSingleSso(pathname: string): boolean {
  if (!pathname) return false;
  return !pathname.endsWith('/init') && !pathname.endsWith('/sso/callback');
}

export function resolveSingleSsoRedirectTarget(
  pathname: string,
  searchParams: SearchParamsLike
): string {
  const redirect = searchParams.get('redirect');
  if (redirect) {
    return withBasePathIfInternal(redirect);
  }

  if (pathname.includes('/invite/')) {
    const query = searchParams.toString();
    return query ? `${pathname}?${query}` : pathname;
  }

  return '/console';
}
