'use client';

import { toast } from 'sonner';

import { useState, useEffect, type FormEvent } from 'react';
import { useRouter, useParams } from 'next/navigation';
import { useT } from '@/i18n';
import Link from 'next/link';
import * as z from 'zod';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input, PasswordInput } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { AlertCircle, Info, Loader2, Users } from 'lucide-react';
import { useAcceptInvite, useInviteInfo } from '@/hooks/auth/use-invite';
import { authenticationService } from '@/services/auth.service';
import { useAuthStore } from '@/store/auth-store';
import { clearSessionBoundClientState } from '@/lib/auth/client-state';
import { getErrorMessage } from '@/utils/error-notifications';

const inviteAcceptErrorTranslationKeys = {
  alreadyMember: 'auth.inviteAcceptErrors.alreadyMember',
  pendingApproval: 'auth.inviteAcceptErrors.pendingApproval',
  invalidToken: 'auth.inviteAcceptErrors.invalidToken',
  inactive: 'auth.inviteAcceptErrors.inactive',
  expired: 'auth.inviteAcceptErrors.expired',
  memberNameExists: 'auth.inviteAcceptErrors.memberNameExists',
  loginRequired: 'auth.inviteAcceptErrors.loginRequired',
} as const;

type InviteAcceptErrorKey = keyof typeof inviteAcceptErrorTranslationKeys;

const inviteAcceptErrorByCode: Record<string, InviteAcceptErrorKey> = {
  '205020': 'alreadyMember',
  '205021': 'pendingApproval',
  '401001': 'loginRequired',
};

const inviteAcceptErrorPatterns: Array<{
  key: InviteAcceptErrorKey;
  includes: string;
}> = [
  { key: 'invalidToken', includes: 'invalid invite token' },
  { key: 'inactive', includes: 'invite link is not active' },
  { key: 'expired', includes: 'invite link expired' },
  { key: 'memberNameExists', includes: 'member name already exists' },
  { key: 'alreadyMember', includes: 'already a member of this organization' },
  { key: 'pendingApproval', includes: 'join request is pending approval' },
  { key: 'loginRequired', includes: 'please login first' },
];

function getBackendErrorDetails(error: unknown): { code?: string; message?: string } {
  if (!error || typeof error !== 'object') return {};

  const candidate = error as {
    businessError?: { code?: string | number; message?: string };
    response?: {
      data?: {
        code?: string | number;
        errorCode?: string | number;
        message?: string;
        errorMessage?: string;
      };
    };
    message?: string;
  };
  const data = candidate.response?.data;
  const code = candidate.businessError?.code ?? data?.code ?? data?.errorCode;
  const message =
    candidate.businessError?.message ?? data?.message ?? data?.errorMessage ?? candidate.message;

  return {
    code: code === undefined ? undefined : String(code),
    message,
  };
}

export default function InvitePage() {
  const params = useParams();
  const token = params.token as string;
  const router = useRouter();
  const t = useT();

  const { data: inviteInfo, isLoading: loading, error } = useInviteInfo(token);
  const { mutate: acceptInvite, isPending: isAccepting } = useAcceptInvite();
  const isAuthenticated = useAuthStore(state => state.isAuthenticated);

  const [isProcessing, setIsProcessing] = useState(false);
  const [needsAuth, setNeedsAuth] = useState(true);
  const [emailChecked, setEmailChecked] = useState(false);
  const [checkingEmail, setCheckingEmail] = useState(false);
  const [joinRequestPending, setJoinRequestPending] = useState(false);

  // Login form schema
  const loginSchema = z.object({
    email: z.string().min(1, t('auth.emailRequired')).email(t('auth.invalidEmail')),
    password: z.string().min(8, t('auth.passwordTooShort')).max(100, t('auth.passwordTooLong')),
    member_name: z.string().optional(),
  });

  type LoginFormData = z.infer<typeof loginSchema>;

  const loginForm = useForm<LoginFormData>({
    resolver: zodResolver(loginSchema),
    defaultValues: {
      email: '',
      password: '',
      member_name: '',
    },
  });

  // Check if user is already logged in
  useEffect(() => {
    if (isAuthenticated && inviteInfo) {
      setNeedsAuth(false);

      if (typeof window !== 'undefined') {
        const justRegistered = sessionStorage.getItem('invite_token') === token;
        if (justRegistered) {
          sessionStorage.removeItem('invite_token');
        }
      }
    }
  }, [isAuthenticated, inviteInfo, token]);

  // Common function to refresh user profile after login
  const refreshUserProfile = async () => {
    await clearSessionBoundClientState();
    try {
      await useAuthStore.getState().initializeAuth({ force: true });
    } catch {
      // Ignore bootstrap failures and continue with invite acceptance.
    }
  };

  const getInviteAcceptErrorMessage = (error: unknown) => {
    const { code, message } = getBackendErrorDetails(error);
    const keyFromCode = code ? inviteAcceptErrorByCode[code] : undefined;
    if (keyFromCode) {
      return t(inviteAcceptErrorTranslationKeys[keyFromCode]);
    }

    const normalizedMessage = message?.trim().toLowerCase();
    const keyFromMessage = normalizedMessage
      ? inviteAcceptErrorPatterns.find(item => normalizedMessage.includes(item.includes))?.key
      : undefined;

    if (keyFromMessage) {
      return t(inviteAcceptErrorTranslationKeys[keyFromMessage]);
    }

    return getErrorMessage(error) || t('auth.failedToJoin');
  };

  // Handle accepting invite
  const handleAcceptInvite = (memberName?: string) => {
    acceptInvite(
      { token, member_name: memberName },
      {
        onSuccess: result => {
          if (result.status === 'pending') {
            setJoinRequestPending(true);
            setIsProcessing(false);
            toast.info(t('auth.joinRequestSubmitted'));
            return;
          }

          toast.success(t('auth.joinedOrganization'));
          window.location.href = '/console';
        },
        onError: error => {
          toast.error(getInviteAcceptErrorMessage(error));
          setIsProcessing(false);
        },
      }
    );
  };

  // Handle direct accept if already logged in
  const handleDirectAccept = () => {
    setIsProcessing(true);
    handleAcceptInvite();
  };

  // Check if email is registered
  const handleCheckEmail = async (email: string) => {
    setCheckingEmail(true);
    try {
      const emailCheck = await authenticationService.checkEmail(email);

      if (emailCheck.is_registered) {
        setEmailChecked(true);
        loginForm.setValue('email', email);
      } else {
        if (typeof window !== 'undefined') {
          sessionStorage.setItem('invite_token', token);
        }
        router.push(`/register?redirect=${encodeURIComponent(`/invite/${token}`)}`);
      }
    } catch {
      toast.error(t('common.error'));
    } finally {
      setCheckingEmail(false);
    }
  };

  // Handle login
  const handleLogin = async (data: LoginFormData) => {
    setIsProcessing(true);
    try {
      await authenticationService.login({
        email: data.email,
        password: data.password,
      });

      await refreshUserProfile();

      toast.success(t('auth.loginSuccess'));

      handleAcceptInvite(data.member_name);
    } catch {
      setIsProcessing(false);
      toast.error(t('auth.loginFailed'));
    }
  };

  const isFormLoading = isProcessing || isAccepting || checkingEmail;

  // Loading state
  if (loading) {
    return (
      <div>
        <Loader2 className="h-8 w-8 animate-spin" />
      </div>
    );
  }

  // Error state
  if (error || !inviteInfo) {
    return (
      <div>
        <Card className="w-full max-w-md">
          <CardHeader>
            <CardTitle className="text-destructive">{t('auth.invalidInvite')}</CardTitle>
          </CardHeader>
          <CardContent>
            <Alert variant="destructive">
              <AlertCircle className="h-4 w-4" />
              <AlertTitle>{t('common.error')}</AlertTitle>
              <AlertDescription>{error?.message || t('auth.invalidInviteDesc')}</AlertDescription>
            </Alert>
            <div className="mt-6 text-center">
              <Link href="/login">
                <Button variant="outline">{t('auth.goToLogin')}</Button>
              </Link>
            </div>
          </CardContent>
        </Card>
      </div>
    );
  }

  // Main invite page
  return (
    <div>
      <Card className="w-full max-w-md">
        <CardHeader className="text-center space-y-2">
          <div className="mx-auto bg-primary/10 p-3 rounded-full w-fit mb-2">
            <Users className="h-8 w-8 text-primary" />
          </div>
          <CardTitle className="text-2xl">{t('auth.joinOrganization')}</CardTitle>
          <p className="text-muted-foreground">
            {t('auth.invitedToJoin')} <strong>{inviteInfo.group.name}</strong>
            {inviteInfo.department ? (
              <span>
                {` ${t('auth.inDepartment')} `}
                <strong>{inviteInfo.department.name}</strong>
              </span>
            ) : null}
            .
          </p>
        </CardHeader>

        <CardContent>
          {joinRequestPending ? (
            <div className="space-y-4">
              <Alert>
                <Info className="h-4 w-4" />
                <AlertTitle>{t('auth.joinRequestSubmitted')}</AlertTitle>
                <AlertDescription>{t('auth.joinRequestSubmittedDesc')}</AlertDescription>
              </Alert>
              <Link href="/console" className="block">
                <Button variant="outline" className="w-full">
                  {t('common.back')}
                </Button>
              </Link>
            </div>
          ) : !needsAuth && isAuthenticated ? (
            // User is already logged in, show direct accept button
            <div className="space-y-4">
              <Alert>
                <Info className="h-4 w-4" />
                <AlertTitle>{t('auth.alreadyLoggedIn')}</AlertTitle>
                <AlertDescription>{t('auth.clickToJoinOrganization')}</AlertDescription>
              </Alert>
              <Button onClick={handleDirectAccept} className="w-full" disabled={isFormLoading}>
                {isFormLoading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                {t('auth.joinOrganization')}
              </Button>
            </div>
          ) : !emailChecked ? (
            // Step 1: Check email
            <form
              onSubmit={(e: FormEvent<HTMLFormElement>) => {
                e.preventDefault();
                const input = e.currentTarget.elements.namedItem('email');
                if (input instanceof HTMLInputElement) {
                  void handleCheckEmail(input.value);
                }
              }}
              className="space-y-4"
            >
              <div className="space-y-2">
                <Label htmlFor="email">{t('auth.email')}</Label>
                <Input
                  id="email"
                  name="email"
                  type="email"
                  placeholder={t('auth.enterEmail')}
                  autoComplete="email"
                  disabled={isFormLoading}
                  required
                />
              </div>
              <Button type="submit" className="w-full" disabled={isFormLoading}>
                {isFormLoading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                {t('auth.continue')}
              </Button>
            </form>
          ) : (
            // Step 2: Login form for registered users
            <form onSubmit={loginForm.handleSubmit(handleLogin)} className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="login-email">{t('auth.email')}</Label>
                <Input
                  id="login-email"
                  type="email"
                  disabled
                  {...loginForm.register('email')}
                  className="bg-muted"
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="login-member-name">{t('common.name')}</Label>
                <Input
                  id="login-member-name"
                  placeholder={t('auth.enterName')}
                  disabled={isFormLoading}
                  {...loginForm.register('member_name')}
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="login-password">{t('auth.password')}</Label>
                <PasswordInput
                  id="login-password"
                  placeholder={t('auth.enterPassword')}
                  autoComplete="current-password"
                  disabled={isFormLoading}
                  {...loginForm.register('password')}
                  errorText={loginForm.formState.errors.password?.message}
                />
              </div>

              <Button type="submit" className="w-full" disabled={isFormLoading}>
                {isFormLoading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                {t('auth.signIn')}
              </Button>
            </form>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
