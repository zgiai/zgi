'use client';

import { useEffect, useState, type CSSProperties, type ReactNode } from 'react';
import Link from 'next/link';
import { useSearchParams } from 'next/navigation';
import { zodResolver } from '@hookform/resolvers/zod';
import { KeyRound, Mail } from 'lucide-react';
import { useForm } from 'react-hook-form';
import * as z from 'zod';

import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import { withBasePathIfInternal } from '@/lib/config';
import { buildSsoStartUrl } from '@/utils/auth-sso';
import { getAuthBusinessErrorCode, getAuthBusinessErrorData } from '@/utils/auth-errors';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardTitle } from '@/components/ui/card';
import { Input, PasswordInput } from '@/components/ui/input';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { Icons } from '@/components/ui/icons';
import { Label } from '@/components/ui/label';
import { useLogin, usePhonePasswordLogin, useSystemFeatures } from '@/hooks';

interface LoginFormProps {
  className?: string;
}

const DEFAULT_PHONE_COUNTRY_CODE = 'CN';
const authThemeVars = {
  '--brand-primary': '#2563EB',
  '--brand-primary-hover': '#1D4ED8',
  '--brand-primary-border': '#BFDBFE',
  '--text-primary': '#111827',
  '--text-secondary': '#6B7280',
  '--text-muted': '#9CA3AF',
  '--placeholder': '#A0A7B2',
  '--border-default': '#E5E7EB',
  '--border-strong': '#D1D5DB',
  '--bg-soft': '#F8FAFC',
  '--button-primary': '#111827',
  '--button-primary-hover': '#0B1020',
} as CSSProperties;

const loginInputClassName =
  'h-11 rounded-xl border-[var(--border-default)] px-4 text-base text-[var(--text-primary)] placeholder:text-[var(--placeholder)] focus-visible:border-[var(--brand-primary)]';

const loginPrimaryButtonClassName =
  'mt-2 h-11 w-full rounded-xl bg-[var(--button-primary)] text-base font-bold tracking-normal text-white shadow-[0_12px_28px_-18px_rgba(17,24,39,0.7)] hover:bg-[var(--button-primary-hover)] hover:brightness-100';

function isEmailLike(value: string): boolean {
  return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(value.trim());
}

function isE164PhoneLike(value: string): boolean {
  return /^\+[1-9]\d{7,14}$/.test(value.replace(/[\s()-]/g, ''));
}

function normalizePhoneAccount(value: string): string | null {
  const trimmed = value.trim();
  if (!/^[+\d\s()-]+$/.test(trimmed)) {
    return null;
  }

  const compact = trimmed.replace(/[\s()-]/g, '');
  if (isE164PhoneLike(compact)) {
    return compact;
  }

  const digits = trimmed.replace(/\D/g, '');
  if (/^1[3-9]\d{9}$/.test(digits)) {
    return digits;
  }
  if (/^86(1[3-9]\d{9})$/.test(digits)) {
    return `+${digits}`;
  }

  return null;
}

export function LoginForm({ className }: LoginFormProps) {
  const t = useT('auth');
  const searchParams = useSearchParams();
  const inviteToken = searchParams.get('invite_token');
  const redirect = searchParams.get('redirect');
  const emailFromParams = decodeURIComponent(searchParams.get('email') || '');
  const registerHref = redirect
    ? `/register?redirect=${encodeURIComponent(redirect)}`
    : '/register';

  const [mounted, setMounted] = useState(false);

  const loginMutation = useLogin();
  const phonePasswordLoginMutation = usePhonePasswordLogin();
  const { data: systemFeatures } = useSystemFeatures();

  const canRegister = Boolean(systemFeatures?.is_allow_register);
  const hasSocialLogin = Boolean(systemFeatures?.enable_social_oauth_login);

  const loginSchema = z
    .object({
      account: z.string().min(1, t('accountRequired')),
      password: z.string().min(8, t('passwordTooShort')).max(100, t('passwordTooLong')),
      invite_token: z.string().optional(),
    })
    .superRefine((data, ctx) => {
      if (inviteToken) {
        if (!isEmailLike(data.account)) {
          ctx.addIssue({
            code: z.ZodIssueCode.custom,
            path: ['account'],
            message: t('invalidEmail'),
          });
        }
        return;
      }

      if (!isEmailLike(data.account) && !normalizePhoneAccount(data.account)) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          path: ['account'],
          message: t('invalidEmailOrPhone'),
        });
      }
    });

  type LoginFormData = z.infer<typeof loginSchema>;

  const loginForm = useForm<LoginFormData>({
    resolver: zodResolver(loginSchema),
    defaultValues: {
      account: emailFromParams,
      password: '',
    },
  });

  const accountValue = loginForm.watch('account').trim();
  const phoneAccountValue = normalizePhoneAccount(accountValue);
  const forgotPasswordHref = isEmailLike(accountValue)
    ? `/forgot-password?email=${encodeURIComponent(accountValue)}`
    : phoneAccountValue
      ? `/forgot-password?phone=${encodeURIComponent(phoneAccountValue)}`
      : '/forgot-password';

  useEffect(() => {
    setMounted(true);
  }, []);

  const navigateAfterLogin = () => {
    const urlParams = new URLSearchParams(window.location.search);
    const redirectUrl = withBasePathIfInternal(urlParams.get('redirect') || '/console');
    window.location.href = redirectUrl;
  };

  const onSubmit = async (data: LoginFormData) => {
    const account = data.account.trim();
    const phoneAccount = normalizePhoneAccount(account);
    try {
      if (!inviteToken && !isEmailLike(account) && phoneAccount) {
        await phonePasswordLoginMutation.mutateAsync({
          country_code: DEFAULT_PHONE_COUNTRY_CODE,
          phone: phoneAccount,
          password: data.password,
        });
        navigateAfterLogin();
        return;
      }

      const formData = {
        email: account,
        password: data.password,
        invite_token: inviteToken || undefined,
      };
      await loginMutation.mutateAsync(formData);
      navigateAfterLogin();
    } catch (err) {
      if (!isEmailLike(account) || getAuthBusinessErrorCode(err) !== 'account_not_found') {
        console.error('Login failed:', err);
        return;
      }

      const token = getAuthBusinessErrorData(err);
      if (typeof token === 'string' && token.length > 0) {
        const urlParams = new URLSearchParams(window.location.search);
        const completeUrl = new URL('/register/complete', window.location.origin);
        completeUrl.searchParams.set('email', account);
        completeUrl.searchParams.set('token', token);
        const redirect = urlParams.get('redirect');
        if (redirect) {
          completeUrl.searchParams.set('redirect', redirect);
        }
        window.location.href = withBasePathIfInternal(completeUrl.pathname + completeUrl.search);
        return;
      }

      console.error('Login failed:', err);
    }
  };

  const onSsoLogin = () => {
    const redirectTarget = withBasePathIfInternal(redirect || '/console');
    window.location.href = buildSsoStartUrl('casdoor', redirectTarget);
  };

  const formLoading =
    loginMutation.isPending ||
    phonePasswordLoginMutation.isPending ||
    loginForm.formState.isSubmitting;
  const authRichT = t as typeof t & {
    rich: (
      key: 'termsPrivacyText',
      values: {
        termsLink: (chunks: ReactNode) => ReactNode;
        privacyLink: (chunks: ReactNode) => ReactNode;
      }
    ) => ReactNode;
  };

  return (
    <div className={cn('flex flex-col gap-6', className)} style={authThemeVars}>
      <Card className="overflow-hidden rounded-[28px] border border-[var(--border-default)] bg-white/95 shadow-[0_18px_48px_-28px_rgba(15,23,42,0.35),0_8px_24px_-18px_rgba(15,23,42,0.18)] backdrop-blur-xl">
        <div
          className={cn(
            'space-y-2 px-8 pb-6 pt-9 text-center',
            mounted ? 'animate-in fade-in slide-in-from-top-4 duration-700' : 'opacity-0'
          )}
        >
          <CardTitle className="text-[32px] font-bold leading-tight tracking-normal text-[var(--text-primary)]">
            {t('welcomeBack')}
          </CardTitle>
          <p className="text-base text-[var(--text-secondary)]">{t('signInToAccount')}</p>
        </div>

        <CardContent className="space-y-6 px-8 pb-9">
          {inviteToken ? (
            <Alert
              className={cn(
                'border-primary/20 bg-primary/5 text-primary',
                mounted ? 'animate-in fade-in zoom-in-95 duration-500' : 'opacity-0'
              )}
            >
              <Icons.Info className="h-4 w-4" />
              <AlertDescription>{t('inviteLoginHint')}</AlertDescription>
            </Alert>
          ) : null}

          <form
            onSubmit={loginForm.handleSubmit(onSubmit)}
            className={cn(
              'space-y-6',
              mounted
                ? 'animate-in fade-in slide-in-from-bottom-4 duration-700 delay-100'
                : 'opacity-0'
            )}
          >
            <div className="space-y-2">
              <Label
                htmlFor="account"
                className="ml-1 text-sm font-semibold text-[var(--text-primary)]"
              >
                {t('account')}
              </Label>
              <Input
                id="account"
                type="text"
                leftIcon={<Mail />}
                placeholder={inviteToken ? t('enterEmail') : t('enterEmailOrPhone')}
                autoComplete="username"
                disabled={formLoading || Boolean(inviteToken)}
                {...loginForm.register('account')}
                aria-invalid={loginForm.formState.errors.account ? 'true' : 'false'}
                errorText={loginForm.formState.errors.account?.message}
                className={loginInputClassName}
              />
            </div>

            <div className="space-y-2">
              <div className="ml-1 flex items-center justify-between">
                <Label
                  htmlFor="password"
                  className="text-sm font-semibold text-[var(--text-primary)]"
                >
                  {t('password')}
                </Label>
                <Link
                  href={forgotPasswordHref}
                  className="text-sm font-medium text-[var(--brand-primary)] transition-colors hover:text-[var(--brand-primary-hover)]"
                  tabIndex={-1}
                >
                  {t('forgotPasswordLink')}
                </Link>
              </div>
              <PasswordInput
                id="password"
                leftIcon={<KeyRound />}
                placeholder={t('enterPassword')}
                autoComplete="current-password"
                disabled={formLoading}
                {...loginForm.register('password')}
                errorText={loginForm.formState.errors.password?.message}
                className={loginInputClassName}
              />
            </div>

            <Button
              type="submit"
              size="xl"
              className={loginPrimaryButtonClassName}
              loading={formLoading}
              interactive
            >
              {t('signIn')}
            </Button>
          </form>

          {hasSocialLogin ? (
            <div className="animate-in fade-in duration-700 delay-300">
              <div className="relative mb-6">
                <div className="absolute inset-0 flex items-center">
                  <span className="w-full border-t border-[var(--border-default)]" />
                </div>
                <div className="relative flex justify-center text-xs font-medium">
                  <span className="bg-white px-4 text-[var(--text-muted)]">
                    {t('orContinueWith')}
                  </span>
                </div>
              </div>

              <div className="grid grid-cols-1 gap-4">
                <Button
                  variant="outline"
                  type="button"
                  size="lg"
                  disabled={formLoading}
                  className="h-11 gap-2 rounded-xl border-[var(--border-default)] bg-white text-base font-semibold text-[var(--text-primary)] shadow-sm transition-all hover:bg-[var(--bg-soft)]"
                  onClick={onSsoLogin}
                >
                  <Icons.Shield className="h-5 w-5" />
                  <span>{t('signInWithSSO')}</span>
                </Button>
              </div>
            </div>
          ) : null}

          {canRegister ? (
            <div className="animate-in fade-in pt-2 text-center text-sm duration-700 delay-500">
              <span className="text-[var(--text-secondary)]">{t('dontHaveAccount')}</span>{' '}
              <Link
                href={registerHref}
                className="font-bold text-[var(--brand-primary)] transition-colors hover:text-[var(--brand-primary-hover)]"
              >
                {t('createAccount')}
              </Link>
            </div>
          ) : null}
        </CardContent>
      </Card>

      <div className="mx-auto max-w-[320px] text-center text-[10px] leading-relaxed text-[var(--text-muted)] animate-in fade-in duration-700 delay-700">
        {authRichT.rich('termsPrivacyText', {
          termsLink: chunks => (
            <Link
              href="/terms"
              target="_blank"
              rel="noopener noreferrer"
              className="text-[var(--text-secondary)] underline transition-colors hover:text-[var(--brand-primary)]"
            >
              {chunks}
            </Link>
          ),
          privacyLink: chunks => (
            <Link
              href="/privacy"
              target="_blank"
              rel="noopener noreferrer"
              className="text-[var(--text-secondary)] underline transition-colors hover:text-[var(--brand-primary)]"
            >
              {chunks}
            </Link>
          ),
        })}
      </div>
    </div>
  );
}
