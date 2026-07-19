'use client';

import { useEffect, useState, type CSSProperties, type ReactNode } from 'react';
import Link from 'next/link';
import { useSearchParams } from 'next/navigation';
import { zodResolver } from '@hookform/resolvers/zod';
import { KeyRound, Mail, Smartphone } from 'lucide-react';
import { useForm } from 'react-hook-form';
import * as z from 'zod';

import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import { withBasePathIfInternal } from '@/lib/config';
import {
  hasNotificationSMSTemplate,
  NOTIFICATION_SMS_AUTH_PHONE_RESET_PASSWORD_TEMPLATE,
} from '@/lib/features/notification-sms';
import { buildSsoStartUrl } from '@/utils/auth-sso';
import { getAuthBusinessErrorCode, getAuthBusinessErrorData } from '@/utils/auth-errors';
import { isValidPhoneNumber } from '@/utils/validation';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardTitle } from '@/components/ui/card';
import { Input, PasswordInput } from '@/components/ui/input';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { Icons } from '@/components/ui/icons';
import { Label } from '@/components/ui/label';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import {
  useLogin,
  usePhonePasswordLogin,
  useSystemFeatures,
} from '@/hooks';

interface LoginFormProps {
  className?: string;
}

type LoginMethod = 'email' | 'phone';

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

const loginTabTriggerClassName =
  'h-9 gap-2 rounded-xl border border-transparent text-base font-semibold text-[var(--text-primary)] shadow-none data-[state=active]:border-[var(--brand-primary-border)] data-[state=active]:bg-white data-[state=active]:text-[var(--brand-primary)] data-[state=active]:shadow-[0_4px_12px_rgba(37,99,235,0.08)]';

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
  const [loginMethod, setLoginMethod] = useState<LoginMethod>('email');

  const loginMutation = useLogin();
  const phonePasswordLoginMutation = usePhonePasswordLogin();
  const { data: systemFeatures } = useSystemFeatures();

  const canRegister = Boolean(systemFeatures?.is_allow_register);
  const hasSocialLogin = Boolean(systemFeatures?.enable_social_oauth_login);
  const hasPhoneLogin = !inviteToken;
  const hasPhonePasswordReset = hasNotificationSMSTemplate(
    systemFeatures,
    NOTIFICATION_SMS_AUTH_PHONE_RESET_PASSWORD_TEMPLATE
  );

  const emailLoginSchema = z.object({
    email: z.string().min(1, t('emailRequired')).email(t('invalidEmail')),
    password: z.string().min(8, t('passwordTooShort')).max(100, t('passwordTooLong')),
    invite_token: z.string().optional(),
  });

  const phoneLoginSchema = z.object({
    phone: z
      .string()
      .min(1, t('phoneRequired'))
      .refine(value => isValidPhoneNumber(value, 'INTL'), t('invalidPhoneNumber')),
    password: z.string().min(8, t('passwordTooShort')).max(100, t('passwordTooLong')),
  });

  type EmailLoginFormData = z.infer<typeof emailLoginSchema>;
  type PhoneLoginFormData = z.infer<typeof phoneLoginSchema>;

  const emailForm = useForm<EmailLoginFormData>({
    resolver: zodResolver(emailLoginSchema),
    defaultValues: {
      email: emailFromParams,
      password: '',
    },
  });

  const phoneForm = useForm<PhoneLoginFormData>({
    resolver: zodResolver(phoneLoginSchema),
    defaultValues: {
      phone: '',
      password: '',
    },
  });

  const forgotPasswordEmail = emailForm.watch('email').trim();
  const forgotPasswordHref = forgotPasswordEmail
    ? `/forgot-password?email=${encodeURIComponent(forgotPasswordEmail)}`
    : '/forgot-password';
  const phoneValue = phoneForm.watch('phone');
  const forgotPasswordPhoneHref = phoneValue.trim()
    ? `/forgot-password?phone=${encodeURIComponent(phoneValue.trim())}`
    : '/forgot-password?method=phone';

  useEffect(() => {
    setMounted(true);
  }, []);

  useEffect(() => {
    if (!hasPhoneLogin && loginMethod === 'phone') {
      setLoginMethod('email');
    }
  }, [hasPhoneLogin, loginMethod]);

  const navigateAfterLogin = () => {
    const urlParams = new URLSearchParams(window.location.search);
    const redirectUrl = withBasePathIfInternal(urlParams.get('redirect') || '/console');
    window.location.href = redirectUrl;
  };

  const onEmailSubmit = async (data: EmailLoginFormData) => {
    try {
      const formData = { ...data };
      if (inviteToken) {
        formData.invite_token = inviteToken;
      }
      await loginMutation.mutateAsync(formData);
      navigateAfterLogin();
    } catch (err) {
      if (getAuthBusinessErrorCode(err) === 'account_not_found') {
        const token = getAuthBusinessErrorData(err);
        if (typeof token === 'string' && token.length > 0) {
          const urlParams = new URLSearchParams(window.location.search);
          const completeUrl = new URL('/register/complete', window.location.origin);
          completeUrl.searchParams.set('email', data.email);
          completeUrl.searchParams.set('token', token);
          const redirect = urlParams.get('redirect');
          if (redirect) {
            completeUrl.searchParams.set('redirect', redirect);
          }
          window.location.href = withBasePathIfInternal(completeUrl.pathname + completeUrl.search);
          return;
        }
      }

      console.error('Login failed:', err);
    }
  };

  const onPhoneSubmit = async (data: PhoneLoginFormData) => {
    if (!hasPhoneLogin) {
      phoneForm.setError('phone', {
        message: t('invalidPhoneNumber'),
      });
      return;
    }

    try {
      await phonePasswordLoginMutation.mutateAsync({
        country_code: DEFAULT_PHONE_COUNTRY_CODE,
        phone: data.phone,
        password: data.password,
      });

      navigateAfterLogin();
    } catch (err) {
      console.error('Phone login failed:', err);
    }
  };

  const onSsoLogin = () => {
    const redirectTarget = withBasePathIfInternal(redirect || '/console');
    window.location.href = buildSsoStartUrl('casdoor', redirectTarget);
  };

  const emailFormLoading = loginMutation.isPending || emailForm.formState.isSubmitting;
  const phoneFormLoading =
    phonePasswordLoginMutation.isPending || phoneForm.formState.isSubmitting;
  const isAnyFormLoading = emailFormLoading || phoneFormLoading;
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
          <p className="text-base text-[var(--text-secondary)]">
            {loginMethod === 'phone' ? t('signInWithPhoneDesc') : t('signInToAccount')}
          </p>
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

          <Tabs
            value={loginMethod}
            onValueChange={value => setLoginMethod(value as LoginMethod)}
            className="w-full"
          >
            <TabsList
              className={cn(
                'grid h-11 w-full rounded-2xl border border-[var(--border-default)] bg-[var(--bg-soft)] p-1 text-[var(--text-primary)] shadow-none',
                hasPhoneLogin ? 'grid-cols-2' : 'grid-cols-1'
              )}
            >
              <TabsTrigger value="email" className={loginTabTriggerClassName}>
                <Mail className="size-5" />
                {t('authMethodEmail')}
              </TabsTrigger>
              {hasPhoneLogin ? (
                <TabsTrigger value="phone" className={loginTabTriggerClassName}>
                  <Smartphone className="size-5" />
                  {t('authMethodPhone')}
                </TabsTrigger>
              ) : null}
            </TabsList>

            <TabsContent
              value="email"
              className={cn(
                'mt-6',
                mounted
                  ? 'animate-in fade-in slide-in-from-bottom-4 duration-700 delay-100'
                  : 'opacity-0'
              )}
            >
              <form onSubmit={emailForm.handleSubmit(onEmailSubmit)} className="space-y-6">
                <div className="space-y-2">
                  <Label
                    htmlFor="email"
                    className="ml-1 text-sm font-semibold text-[var(--text-primary)]"
                  >
                    {t('email')}
                  </Label>
                  <Input
                    id="email"
                    type="email"
                    leftIcon={<Mail />}
                    placeholder={t('enterEmail')}
                    autoComplete="email"
                    disabled={emailFormLoading || Boolean(inviteToken)}
                    {...emailForm.register('email')}
                    aria-invalid={emailForm.formState.errors.email ? 'true' : 'false'}
                    errorText={emailForm.formState.errors.email?.message}
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
                    disabled={emailFormLoading}
                    {...emailForm.register('password')}
                    errorText={emailForm.formState.errors.password?.message}
                    className={loginInputClassName}
                  />
                </div>

                <Button
                  type="submit"
                  size="xl"
                  className={loginPrimaryButtonClassName}
                  loading={emailFormLoading}
                  interactive
                >
                  {t('signIn')}
                </Button>
              </form>
            </TabsContent>

            {hasPhoneLogin ? (
              <TabsContent
                value="phone"
                className={cn(
                  'mt-6',
                  mounted
                    ? 'animate-in fade-in slide-in-from-bottom-4 duration-700 delay-100'
                    : 'opacity-0'
                )}
              >
                <form onSubmit={phoneForm.handleSubmit(onPhoneSubmit)} className="space-y-6">
                  <div className="space-y-2">
                    <Label
                      htmlFor="phone"
                      className="ml-1 text-sm font-semibold text-[var(--text-primary)]"
                    >
                      {t('phone')}
                    </Label>
                    <Input
                      id="phone"
                      type="tel"
                      leftIcon={<Smartphone />}
                      placeholder={t('phonePlaceholder')}
                      autoComplete="tel"
                      disabled={phoneFormLoading}
                      {...phoneForm.register('phone')}
                      aria-invalid={phoneForm.formState.errors.phone ? 'true' : 'false'}
                      errorText={phoneForm.formState.errors.phone?.message}
                      className={loginInputClassName}
                    />
                  </div>

                  <div className="space-y-2">
                    <div className="ml-1 flex items-center justify-between">
                      <Label
                        htmlFor="phonePassword"
                        className="text-sm font-semibold text-[var(--text-primary)]"
                      >
                        {t('password')}
                      </Label>
                      {hasPhonePasswordReset ? (
                        <Link
                          href={forgotPasswordPhoneHref}
                          className="text-sm font-medium text-[var(--brand-primary)] transition-colors hover:text-[var(--brand-primary-hover)]"
                          tabIndex={-1}
                        >
                          {t('forgotPasswordLink')}
                        </Link>
                      ) : null}
                    </div>
                    <PasswordInput
                      id="phonePassword"
                      leftIcon={<KeyRound />}
                      placeholder={t('enterPassword')}
                      autoComplete="current-password"
                      disabled={phoneFormLoading}
                      {...phoneForm.register('password')}
                      errorText={phoneForm.formState.errors.password?.message}
                      className={loginInputClassName}
                    />
                  </div>

                  <Button
                    type="submit"
                    size="xl"
                    className={loginPrimaryButtonClassName}
                    loading={phoneFormLoading}
                    interactive
                  >
                    {t('signIn')}
                  </Button>
                </form>
              </TabsContent>
            ) : null}
          </Tabs>

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
                  disabled={isAnyFormLoading}
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
