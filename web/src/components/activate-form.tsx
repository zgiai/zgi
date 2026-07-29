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

  return (
    <div className="flex w-full grow items-center justify-center px-6 md:px-[108px]">
      <Card className="w-full max-w-md">
        <CardHeader><CardTitle className="text-center text-2xl">{t('invitationActivation.title')}</CardTitle></CardHeader>
        <CardContent className="space-y-5">
          {loading && <div className="flex justify-center py-8"><Icons.Spinner className="h-6 w-6 animate-spin" /></div>}
          {!loading && !canContinue && (
            <Alert variant="destructive"><Icons.AlertCircle className="h-4 w-4" /><AlertDescription>{statusMessage}</AlertDescription></Alert>
          )}
          {!loading && canContinue && result?.data && (
            <form className="space-y-4" onSubmit={submit}>
              {result.data.workspace_name && (
                <Alert><AlertDescription>{t('invitationActivation.destination', { organization: result.data.organization_name ? t('invitationActivation.organizationPrefix', { name: result.data.organization_name }) : '', workspace: result.data.workspace_name })}</AlertDescription></Alert>
              )}
              {(result.data.inviter_name || result.data.role) && (
                <div className="rounded-lg border bg-muted/30 p-3 text-sm text-muted-foreground">
                  {result.data.inviter_name && <p>{t('invitationActivation.inviter', { name: result.data.inviter_name })}</p>}
                  {result.data.role && <p>{t('invitationActivation.role', { role: result.data.role })}</p>}
                </div>
              )}
              <div className="space-y-2"><Label htmlFor="invite-email">{t('invitationActivation.invitedEmail')}</Label><Input id="invite-email" value={result.data.email} disabled /></div>
              {!result.data.account_exists && <div className="space-y-2"><Label htmlFor="invite-name">{t('invitationActivation.name')}</Label><Input id="invite-name" value={name} onChange={e => setName(e.target.value)} maxLength={30} required /></div>}
              <div className="space-y-2"><Label htmlFor="invite-password">{t(result.data.account_exists ? 'invitationActivation.loginPassword' : 'invitationActivation.setPassword')}</Label><PasswordInput id="invite-password" value={password} onChange={e => setPassword(e.target.value)} minLength={8} required /></div>
              {!result.data.account_exists && <div className="space-y-2"><Label htmlFor="invite-password-confirm">{t('invitationActivation.confirmPassword')}</Label><PasswordInput id="invite-password-confirm" value={confirmPassword} onChange={e => setConfirmPassword(e.target.value)} minLength={8} required /></div>}
              {error && <Alert variant="destructive"><AlertDescription>{error}</AlertDescription></Alert>}
              <Button className="w-full" type="submit" disabled={submitting}>{submitting && <Icons.Spinner className="mr-2 h-4 w-4 animate-spin" />}{t(result.data.account_exists ? 'invitationActivation.loginAndJoin' : 'invitationActivation.createAndJoin')}</Button>
            </form>
          )}
        </CardContent>
      </Card>
    </div>
  );
};

export default ActivateForm;
