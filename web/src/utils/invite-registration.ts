export interface InviteRegistrationContext {
  redirect?: string | null;
  inviteToken?: string | null;
  inviteContract?: 'organization' | null;
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
    inviteContract:
      searchParams.get('invite_contract') === 'organization' ? 'organization' : null,
  };
}

export function appendInviteRegistrationContext(
  href: string,
  context: InviteRegistrationContext
): string {
  const params = new URLSearchParams();
  if (context.redirect) params.set('redirect', context.redirect);
  if (context.inviteToken) params.set('invite_token', context.inviteToken);
  if (context.inviteContract) params.set('invite_contract', context.inviteContract);
  if (params.size === 0) return href;

  const separator = href.includes('?') ? '&' : '?';
  return `${href}${separator}${params.toString()}`;
}

export function buildInviteRegistrationHref(token: string): string {
  return appendInviteRegistrationContext('/register', {
    inviteToken: token,
    redirect: `/invite/${token}`,
    inviteContract: 'organization',
  });
}

export function getLegacyLoginInviteToken(
  context: InviteRegistrationContext
): string | undefined {
  // Organization invite tokens are accepted after authentication by /invite/[token].
  // The legacy login API validates a different token store, so never mix contracts.
  if (context.inviteContract === 'organization') return undefined;
  return context.inviteToken || undefined;
}

export function shouldLockLegacyInviteAccount(
  context: InviteRegistrationContext,
  accountPrefill: string
): boolean {
  return Boolean(getLegacyLoginInviteToken(context) && accountPrefill.trim());
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
