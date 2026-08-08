'use client';

import { useEffect, useState, type FormEvent } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Icons } from '@/components/ui/icons';
import { Input, PasswordInput } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { authService } from '@/services/auth.service';
import { accountService } from '@/services/account.service';
import type { ActivationCheckResponse } from '@/services/types/auth';
import { useAuthStore } from '@/store/auth-store';
import { getErrorMessage } from '@/utils/error-notifications';
import { useT } from '@/i18n';

const ActivateForm = () => {
  const router = useRouter();
  const t = useT('auth');
  const searchParams = useSearchParams();
  const workspaceID = searchParams.get('workspace_id') || undefined;
  const email = searchParams.get('email') || '';
  const token = searchParams.get('token') || '';
  const currentUser = useAuthStore.use.user();
  const [result, setResult] = useState<ActivationCheckResponse | null>(null);
  const [name, setName] = useState('');
  const [password, setPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    if (!email || !token) {
      setResult({ is_valid: false, status: 'invalid' });
      setLoading(false);
      return;
    }
    void authService.checkActivate({ email, token, workspace_id: workspaceID }).then(res => {
      setResult(res);
      setName(res.data?.user_name || '');
    }).catch(() => setResult({ is_valid: false, status: 'invalid' })).finally(() => setLoading(false));
  }, [email, token, workspaceID]);

  const selectInvitationDestination = async () => {
    if (!result?.data?.organization_id) return true;
    try {
      await accountService.updateContext({
        mode: 'organization',
        current_organization_id: result.data.organization_id,
        current_workspace_id: result.data.workspace_id || null,
      });
      return true;
    } catch {
      return false;
    }
  };

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (!result?.data?.account_exists && password !== confirmPassword) {
      setError(t('invitationActivation.passwordMismatch'));
      return;
    }
    setSubmitting(true);
    setError('');
    try {
      if (result?.data?.account_exists) {
        await authService.login({ email, password, invite_token: token });
        const destinationSelected = await selectInvitationDestination();
        await useAuthStore.getState().initializeAuth({ force: true });
        router.replace(destinationSelected ? '/console' : '/onboarding/organization');
        return;
      }
      await authService.activate({
        token,
        email,
        workspace_id: result?.data?.workspace_id || workspaceID,
        name,
        password,
        interface_language: navigator.language.startsWith('zh') ? 'zh-Hans' : 'en-US',
        timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
      });
      const destinationSelected = await selectInvitationDestination();
      await useAuthStore.getState().initializeAuth({ force: true });
      router.replace(destinationSelected ? '/console' : '/onboarding/organization');
    } catch (cause) {
      setError(getErrorMessage(cause));
    } finally {
      setSubmitting(false);
    }
  };

  const effectiveStatus = result?.is_valid && currentUser?.email && currentUser.email !== result.data?.email
    ? 'email_mismatch'
    : result?.status;
  const statusMessages: Record<string, string> = {
    invalid: t('invitationActivation.invalid'),
    expired: t('invitationActivation.expired'),
    revoked: t('invitationActivation.revoked'),
    used: t('invitationActivation.used'),
    organization_unavailable: t('invitationActivation.organizationUnavailable'),
    membership_unavailable: t('invitationActivation.membershipUnavailable'),
    role_unavailable: t('invitationActivation.roleUnavailable'),
    account_unavailable: t('invitationActivation.accountUnavailable'),
    email_mismatch: t('invitationActivation.emailMismatch', { email: result?.data?.email || email }),
  };
  const statusMessage = statusMessages[effectiveStatus || 'invalid'];
  const canContinue = result?.is_valid && effectiveStatus !== 'email_mismatch';
  const invitation = result?.data;
  const hasInvitationDestination = Boolean(invitation?.organization_name || invitation?.workspace_name);

  return (
    <div className="flex w-full grow items-center justify-center px-6 md:px-[108px]">
      <Card className="w-full max-w-md">
        <CardHeader><CardTitle className="text-center text-2xl">{t('invitationActivation.title')}</CardTitle></CardHeader>
        <CardContent className="space-y-5">
          {loading && <div className="flex justify-center py-8"><Icons.Spinner className="h-6 w-6 animate-spin" /></div>}
          {!loading && !canContinue && (
            <Alert variant="destructive"><Icons.AlertCircle className="h-4 w-4" /><AlertDescription>{statusMessage}</AlertDescription></Alert>
          )}
          {!loading && canContinue && invitation && (
            <form className="space-y-4" onSubmit={submit}>
              {hasInvitationDestination && (
                <div className="space-y-3 rounded-lg border bg-muted/30 p-4">
                  <p className="text-sm font-medium">{t('invitationActivation.invitationDetails')}</p>
                  {invitation.organization_name && (
                    <div className="flex items-center gap-3">
                      <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-background text-muted-foreground">
                        <Icons.Building className="h-4 w-4" />
                      </div>
                      <div className="min-w-0">
                        <p className="text-xs text-muted-foreground">
                          {t('invitationActivation.organization')}
                        </p>
                        <p className="truncate text-sm font-medium">{invitation.organization_name}</p>
                      </div>
                    </div>
                  )}
                  {invitation.workspace_name && (
                    <div className="flex items-center gap-3">
                      <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-background text-muted-foreground">
                        <Icons.Users className="h-4 w-4" />
                      </div>
                      <div className="min-w-0">
                        <p className="text-xs text-muted-foreground">
                          {t('invitationActivation.workspace')}
                        </p>
                        <p className="truncate text-sm font-medium">{invitation.workspace_name}</p>
                      </div>
                    </div>
                  )}
                </div>
              )}
              {(invitation.inviter_name || invitation.role) && (
                <div className="rounded-lg border bg-muted/30 p-3 text-sm text-muted-foreground">
                  {invitation.inviter_name && <p>{t('invitationActivation.inviter', { name: invitation.inviter_name })}</p>}
                  {invitation.role && <p>{t('invitationActivation.role', { role: invitation.role })}</p>}
                </div>
              )}
              <div className="space-y-2"><Label htmlFor="invite-email">{t('invitationActivation.invitedEmail')}</Label><Input id="invite-email" value={invitation.email} disabled /></div>
              {!invitation.account_exists && <div className="space-y-2"><Label htmlFor="invite-name">{t('invitationActivation.name')}</Label><Input id="invite-name" value={name} onChange={e => setName(e.target.value)} maxLength={30} required /></div>}
              <div className="space-y-2"><Label htmlFor="invite-password">{t(invitation.account_exists ? 'invitationActivation.loginPassword' : 'invitationActivation.setPassword')}</Label><PasswordInput id="invite-password" value={password} onChange={e => setPassword(e.target.value)} minLength={8} required /></div>
              {!invitation.account_exists && <div className="space-y-2"><Label htmlFor="invite-password-confirm">{t('invitationActivation.confirmPassword')}</Label><PasswordInput id="invite-password-confirm" value={confirmPassword} onChange={e => setConfirmPassword(e.target.value)} minLength={8} required /></div>}
              {error && <Alert variant="destructive"><AlertDescription>{error}</AlertDescription></Alert>}
              <Button className="w-full" type="submit" disabled={submitting}>{submitting && <Icons.Spinner className="mr-2 h-4 w-4 animate-spin" />}{t(invitation.account_exists ? 'invitationActivation.loginAndJoin' : 'invitationActivation.createAndJoin')}</Button>
            </form>
          )}
        </CardContent>
      </Card>
    </div>
  );
};

export default ActivateForm;
