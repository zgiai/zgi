import assert from 'node:assert/strict';
import { URL, URLSearchParams } from 'node:url';
import {
  appendInviteRegistrationContext,
  buildInviteRegistrationHref,
  getLegacyLoginInviteToken,
  readInviteRegistrationContext,
  readRegistrationStatusHint,
  removeRegistrationStatusHint,
  resolveRegistrationHintStatus,
  shouldLockLegacyInviteAccount,
  shouldVerifyRegistrationStatusHint,
} from '../src/utils/invite-registration.ts';

const token = 'invite/token with spaces';
const registrationHref = buildInviteRegistrationHref(token);
const registrationUrl = new URL(registrationHref, 'https://zgi.test');
assert.equal(registrationUrl.pathname, '/register');
assert.equal(registrationUrl.searchParams.get('invite_token'), token);
assert.equal(registrationUrl.searchParams.get('redirect'), `/invite/${token}`);
assert.equal(registrationUrl.searchParams.get('invite_contract'), 'organization');

const contextParams = new URLSearchParams({
  invite_token: token,
  redirect: '/invite/source?from=email',
  invite_contract: 'organization',
});
const context = readInviteRegistrationContext(contextParams);
const verificationHref = appendInviteRegistrationContext(
  '/verify?email=user%40example.com&type=register',
  context
);
const verificationUrl = new URL(verificationHref, 'https://zgi.test');
assert.equal(verificationUrl.searchParams.get('email'), 'user@example.com');
assert.equal(verificationUrl.searchParams.get('type'), 'register');
assert.equal(verificationUrl.searchParams.get('invite_token'), token);
assert.equal(verificationUrl.searchParams.get('redirect'), '/invite/source?from=email');
assert.equal(verificationUrl.searchParams.get('invite_contract'), 'organization');
assert.equal(getLegacyLoginInviteToken(context), undefined);
assert.equal(
  getLegacyLoginInviteToken({ inviteToken: 'legacy-invitation-code' }),
  'legacy-invitation-code'
);
assert.equal(shouldLockLegacyInviteAccount(context, ''), false);
assert.equal(shouldLockLegacyInviteAccount(context, 'invited@example.com'), false);
assert.equal(
  shouldLockLegacyInviteAccount({ inviteToken: 'legacy-invitation-code' }, ''),
  false
);
assert.equal(
  shouldLockLegacyInviteAccount(
    { inviteToken: 'legacy-invitation-code' },
    'invited@example.com'
  ),
  true
);
assert.equal(shouldLockLegacyInviteAccount({}, 'invited@example.com'), false);
assert.equal(appendInviteRegistrationContext('/login', {}), '/login');

assert.equal(readRegistrationStatusHint(new URLSearchParams('registration_status=pending')), 'pending');
assert.equal(readRegistrationStatusHint(new URLSearchParams('registration_status=approved')), null);
assert.equal(
  shouldVerifyRegistrationStatusHint({
    hint: 'pending',
    isAuthenticated: true,
    hasInviteInfo: true,
    alreadyVerified: false,
  }),
  true
);
for (const scenario of [
  { hint: null, isAuthenticated: true, hasInviteInfo: true, alreadyVerified: false },
  { hint: 'pending', isAuthenticated: false, hasInviteInfo: true, alreadyVerified: false },
  { hint: 'pending', isAuthenticated: true, hasInviteInfo: false, alreadyVerified: false },
  { hint: 'pending', isAuthenticated: true, hasInviteInfo: true, alreadyVerified: true },
]) {
  assert.equal(shouldVerifyRegistrationStatusHint(scenario), false);
}
assert.equal(resolveRegistrationHintStatus('pending'), 'pending');
assert.equal(resolveRegistrationHintStatus('approved'), 'approved');
assert.equal(resolveRegistrationHintStatus('rejected'), 'recover');
assert.equal(resolveRegistrationHintStatus('expired'), 'recover');
assert.equal(
  removeRegistrationStatusHint(
    '/invite/token',
    new URLSearchParams('registration_status=pending&source=registration')
  ),
  '/invite/token?source=registration'
);

console.log('Registration invitation context and server-verification policy checks passed.');
