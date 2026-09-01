export interface InviteRegistrationContext {
  redirect?: string | null;
  inviteToken?: string | null;
}

export type RegistrationStatusHint = 'pending';
export type InviteAcceptanceStatus = 'pending' | 'approved' | 'rejected' | 'expired';
export type RegistrationHintResolution = 'pending' | 'approved' | 'recover';

export function readInviteRegistrationContext(
  searchParams: Pick<URLSearchParams, 'get'>
): InviteRegistrationContext {
  return {
    redirect: searchParams.get('redirect'),
    inviteToken: searchParams.get('invite_token'),
  };
}

export function appendInviteRegistrationContext(
  href: string,
  context: InviteRegistrationContext
): string {
  const params = new URLSearchParams();
  if (context.redirect) params.set('redirect', context.redirect);
  if (context.inviteToken) params.set('invite_token', context.inviteToken);
  if (params.size === 0) return href;

  const separator = href.includes('?') ? '&' : '?';
  return `${href}${separator}${params.toString()}`;
}

export function buildInviteRegistrationHref(token: string): string {
  return appendInviteRegistrationContext('/register', {
    inviteToken: token,
    redirect: `/invite/${token}`,
  });
}

export function readRegistrationStatusHint(
  searchParams: Pick<URLSearchParams, 'get'>
): RegistrationStatusHint | null {
  return searchParams.get('registration_status') === 'pending' ? 'pending' : null;
}

export function shouldVerifyRegistrationStatusHint({
  hint,
  isAuthenticated,
  hasInviteInfo,
  alreadyVerified,
}: {
  hint: RegistrationStatusHint | null;
  isAuthenticated: boolean;
  hasInviteInfo: boolean;
  alreadyVerified: boolean;
}): boolean {
  return hint === 'pending' && isAuthenticated && hasInviteInfo && !alreadyVerified;
}

export function resolveRegistrationHintStatus(
  status: InviteAcceptanceStatus
): RegistrationHintResolution {
  if (status === 'pending' || status === 'approved') return status;
  return 'recover';
}

export function removeRegistrationStatusHint(
  pathname: string,
  searchParams: Pick<URLSearchParams, 'toString'>
): string {
  const params = new URLSearchParams(searchParams.toString());
  params.delete('registration_status');
  const query = params.toString();
  return query ? `${pathname}?${query}` : pathname;
}
