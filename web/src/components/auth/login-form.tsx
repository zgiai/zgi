'use client';

import { useEffect, useState } from 'react';
import Link from 'next/link';
import { useSearchParams } from 'next/navigation';
import { zodResolver } from '@hookform/resolvers/zod';
import { useForm } from 'react-hook-form';
import * as z from 'zod';

import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import { withBasePathIfInternal } from '@/lib/config';
import { buildSsoStartUrl } from '@/utils/auth-sso';
import { getAuthBusinessErrorCode, getAuthBusinessErrorData } from '@/utils/auth-errors';
import { isValidPhoneNumber } from '@/utils/validation';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardTitle } from '@/components/ui/card';
import { Input, PasswordInput } from '@/components/ui/input';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Info, Shield } from 'lucide-react';
import { useLogin } from '@/hooks/auth/use-login';
import { useSystemFeatures } from '@/hooks/auth/use-system-features';
import {
  usePhoneCheck,
  usePhoneCode,
  usePhoneLogin,
  usePhoneVerify,
} from '@/hooks/auth/use-phone-auth';
import { Label } from '../ui/label';

interface LoginFormProps {
  className?: string;
}

type LoginMethod = 'email' | 'phone';

const DEFAULT_PHONE_COUNTRY_CODE = 'CN';

export function LoginForm({ className }: LoginFormProps) {
  const t = useT().auth;
  const searchParams = useSearchParams();

  const emailFromParams = decodeURIComponent(searchParams.get('email') || '');
  const inviteToken = searchParams.get('invite_token');
  const redirect = searchParams.get('redirect');
  const registerHref = redirect
    ? `/register?redirect=${encodeURIComponent(redirect)}`
    : '/register';

  const [mounted, setMounted] = useState(false);
  const [loginMethod, setLoginMethod] = useState<LoginMethod>('email');
  const [phoneToken, setPhoneToken] = useState('');
  const [phoneCountdown, setPhoneCountdown] = useState(0);

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
    code: z
      .string()
      .min(1, t('codeRequired'))
      .length(6, t('codeLength'))
      .regex(/^\d+$/, t('codeLength')),
  });

  type EmailLoginFormData = z.infer<typeof emailLoginSchema>;
  type PhoneLoginFormData = z.infer<typeof phoneLoginSchema>;

  const loginMutation = useLogin();
  const phoneCheckMutation = usePhoneCheck({ silentSuccess: true });
  const phoneCodeMutation = usePhoneCode();
  const phoneVerifyMutation = usePhoneVerify();
  const phoneLoginMutation = usePhoneLogin();
  const { data: systemFeatures } = useSystemFeatures();

  const canRegister = Boolean(systemFeatures?.is_allow_register);
  const hasSocialLogin = Boolean(systemFeatures?.enable_social_oauth_login);

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
      code: '',
    },
  });

  const forgotPasswordEmail = emailForm.watch('email').trim();
  const forgotPasswordHref = forgotPasswordEmail
    ? `/forgot-password?email=${encodeURIComponent(forgotPasswordEmail)}`
    : '/forgot-password';

  useEffect(() => {
    setMounted(true);
  }, []);

  useEffect(() => {
    if (phoneCountdown <= 0) {
      return;
    }

    const timer = window.setTimeout(() => {
      setPhoneCountdown(phoneCountdown - 1);
    }, 1000);

    return () => window.clearTimeout(timer);
  }, [phoneCountdown]);

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

  const onSendPhoneCode = async () => {
    const isValid = await phoneForm.trigger(['phone']);
    if (!isValid) {
      return;
    }

    try {
      const values = phoneForm.getValues();
      const checkResponse = await phoneCheckMutation.mutateAsync({
        country_code: DEFAULT_PHONE_COUNTRY_CODE,
        phone: values.phone,
      });

      if (!checkResponse.is_registered) {
        phoneForm.setError('phone', {
          message: t('phoneNotRegistered'),
        });
        return;
      }

      const codeResponse = await phoneCodeMutation.mutateAsync({
        country_code: DEFAULT_PHONE_COUNTRY_CODE,
        phone: values.phone,
        scene: 'login',
      });

      setPhoneToken(codeResponse.token);
      setPhoneCountdown(60);
    } catch (err) {
      console.error('Failed to send phone code:', err);
    }
  };

  const onPhoneSubmit = async (data: PhoneLoginFormData) => {
    if (!phoneToken) {
      phoneForm.setError('code', {
        message: t('sendCodeFirst'),
      });
      return;
    }

    try {
      const verifyResult = await phoneVerifyMutation.mutateAsync({
        country_code: DEFAULT_PHONE_COUNTRY_CODE,
        phone: data.phone,
        scene: 'login',
        token: phoneToken,
        code: data.code,
      });

      await phoneLoginMutation.mutateAsync({
        country_code: DEFAULT_PHONE_COUNTRY_CODE,
        phone: data.phone,
        verified_token: verifyResult.verified_token,
      });

      navigateAfterLogin();
    } catch (err) {
      console.error('Phone login failed:', err);
    }
  };

  const onSsoLogin = () => {
    const redirect = withBasePathIfInternal(searchParams.get('redirect') || '/console');
    window.location.href = buildSsoStartUrl('casdoor', redirect);
  };

  const emailFormLoading = loginMutation.isPending || emailForm.formState.isSubmitting;
  const phoneFormLoading =
    phoneCheckMutation.isPending ||
    phoneCodeMutation.isPending ||
    phoneVerifyMutation.isPending ||
    phoneLoginMutation.isPending ||
    phoneForm.formState.isSubmitting;
  const isAnyFormLoading = emailFormLoading || phoneFormLoading;

  return (
    <div className={cn('flex flex-col gap-8', className)}>
      <Card className="glass-panel border-none shadow-premium overflow-hidden">
        <div
          className={cn(
            'p-8 pt-10 text-center space-y-2',
            mounted ? 'animate-in fade-in slide-in-from-top-4 duration-700' : 'opacity-0'
          )}
        >
          <CardTitle className="text-3xl font-bold tracking-tight">{t('welcomeBack')}</CardTitle>
          <p className="text-muted-foreground/80">
            {loginMethod === 'phone' ? t('signInWithPhoneDesc') : t('signInToAccount')}
          </p>
        </div>

        <CardContent className="px-8 pb-10 space-y-6">
          {!!inviteToken && (
            <Alert
              className={cn(
                'bg-primary/5 border-primary/20 text-primary',
                mounted ? 'animate-in fade-in zoom-in-95 duration-500' : 'opacity-0'
              )}
            >
              <Info className="h-4 w-4" />
              <AlertDescription>{t('inviteLoginHint')}</AlertDescription>
            </Alert>
          )}

          <Tabs
            value={loginMethod}
            onValueChange={value => setLoginMethod(value as LoginMethod)}
            className="w-full"
          >
            <TabsList className="grid w-full grid-cols-2">
              <TabsTrigger value="email">{t('authMethodEmail')}</TabsTrigger>
              <TabsTrigger value="phone">{t('authMethodPhone')}</TabsTrigger>
            </TabsList>

            <TabsContent
              value="email"
              className={cn(
                mounted
                  ? 'animate-in fade-in slide-in-from-bottom-4 duration-700 delay-100'
                  : 'opacity-0'
              )}
            >
              <form onSubmit={emailForm.handleSubmit(onEmailSubmit)} className="space-y-5">
                <div className="space-y-2">
                  <Label
                    htmlFor="email"
                    className="text-xs font-semibold uppercase tracking-wider opacity-60 ml-1"
                  >
                    {t('email')}
                  </Label>
                  <Input
                    id="email"
                    type="email"
                    placeholder={t('enterEmail')}
                    autoComplete="email"
                    disabled={emailFormLoading || !!inviteToken}
                    {...emailForm.register('email')}
                    aria-invalid={emailForm.formState.errors.email ? 'true' : 'false'}
                    errorText={emailForm.formState.errors.email?.message}
                    className="h-11 px-4 text-base"
                  />
                </div>

                <div className="space-y-2">
                  <div className="flex items-center justify-between ml-1">
                    <Label
                      htmlFor="password"
                      className="text-xs font-semibold uppercase tracking-wider opacity-60"
                    >
                      {t('password')}
                    </Label>
                    <Link
                      href={forgotPasswordHref}
                      className="text-xs font-medium text-primary/70 hover:text-primary transition-colors"
                      tabIndex={-1}
                    >
                      {t('forgotPasswordLink')}
                    </Link>
                  </div>
                  <PasswordInput
                    id="password"
                    placeholder={t('enterPassword')}
                    autoComplete="current-password"
                    disabled={emailFormLoading}
                    {...emailForm.register('password')}
                    errorText={emailForm.formState.errors.password?.message}
                    className="h-11 px-4 text-base"
                  />
                </div>

                <Button
                  type="submit"
                  size="xl"
                  className="w-full font-bold tracking-wide mt-2"
                  loading={emailFormLoading}
                  interactive
                >
                  {t('signIn')}
                </Button>
              </form>
            </TabsContent>

            <TabsContent
              value="phone"
              className={cn(
                mounted
                  ? 'animate-in fade-in slide-in-from-bottom-4 duration-700 delay-100'
                  : 'opacity-0'
              )}
            >
              <form onSubmit={phoneForm.handleSubmit(onPhoneSubmit)} className="space-y-5">
                <div className="space-y-2">
                  <Label
                    htmlFor="phone"
                    className="text-xs font-semibold uppercase tracking-wider opacity-60 ml-1"
                  >
                    {t('phone')}
                  </Label>
                  <Input
                    id="phone"
                    type="tel"
                    placeholder={t('phonePlaceholder')}
                    autoComplete="tel"
                    disabled={phoneFormLoading}
                    {...phoneForm.register('phone')}
                    aria-invalid={phoneForm.formState.errors.phone ? 'true' : 'false'}
                    errorText={phoneForm.formState.errors.phone?.message}
                    className="h-11 px-4 text-base"
                  />
                </div>

                <div className="space-y-2">
                  <Label
                    htmlFor="phoneCode"
                    className="text-xs font-semibold uppercase tracking-wider opacity-60 ml-1"
                  >
                    {t('verificationCode')}
                  </Label>
                  <div className="flex items-start gap-3">
                    <Input
                      id="phoneCode"
                      inputMode="numeric"
                      placeholder={t('enterCode')}
                      disabled={phoneFormLoading}
                      {...phoneForm.register('code')}
                      aria-invalid={phoneForm.formState.errors.code ? 'true' : 'false'}
                      errorText={phoneForm.formState.errors.code?.message}
                      className="h-11 px-4 text-base"
                    />
                    <Button
                      type="button"
                      variant="outline"
                      className="h-11 min-w-28"
                      disabled={phoneFormLoading || phoneCountdown > 0}
                      onClick={onSendPhoneCode}
                    >
                      {phoneCountdown > 0
                        ? t('resendCodeIn', { countdown: phoneCountdown })
                        : t('sendCode')}
                    </Button>
                  </div>
                </div>

                <Button
                  type="submit"
                  size="xl"
                  className="w-full font-bold tracking-wide mt-2"
                  loading={phoneFormLoading}
                  interactive
                >
                  {t('signIn')}
                </Button>
              </form>
            </TabsContent>
          </Tabs>

          {hasSocialLogin && (
            <div className="animate-in fade-in duration-700 delay-300">
              <div className="relative mb-6">
                <div className="absolute inset-0 flex items-center">
                  <span className="w-full border-t border-border/50" />
                </div>
                <div className="relative flex justify-center text-[10px] uppercase tracking-widest font-bold">
                  <span className="bg-transparent px-4 text-muted-foreground/60 backdrop-blur-sm">
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
                  className="glass-panel border-border/30 hover:bg-muted/50 transition-all gap-2"
                  onClick={onSsoLogin}
                >
                  <Shield className="h-5 w-5" />
                  <span>{t('signInWithSSO')}</span>
                </Button>
              </div>
            </div>
          )}

          {canRegister && (
            <div className="text-center text-sm pt-2 animate-in fade-in duration-700 delay-500">
              <span className="text-muted-foreground">{t('dontHaveAccount')}</span>{' '}
              <Link
                href={registerHref}
                className="font-bold text-primary hover:text-primary-hover transition-colors"
              >
                {t('createAccount')}
              </Link>
            </div>
          )}
        </CardContent>
      </Card>

      <div className="text-center text-[10px] text-muted-foreground/50 leading-relaxed max-w-[320px] mx-auto animate-in fade-in duration-700 delay-700">
        {t.rich('termsPrivacyText', {
          termsLink: chunks => (
            <Link
              href="/terms"
              target="_blank"
              rel="noopener noreferrer"
              className="text-muted-foreground/80 hover:text-primary underline transition-colors"
            >
              {chunks}
            </Link>
          ),
          privacyLink: chunks => (
            <Link
              href="/privacy"
              target="_blank"
              rel="noopener noreferrer"
              className="text-muted-foreground/80 hover:text-primary underline transition-colors"
            >
              {chunks}
            </Link>
          ),
        })}
      </div>
    </div>
  );
}
