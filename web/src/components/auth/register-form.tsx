'use client';

import { useEffect, useState, type CSSProperties, type ReactNode } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import Link from 'next/link';
import { zodResolver } from '@hookform/resolvers/zod';
import { Mail, Smartphone } from 'lucide-react';
import { useForm } from 'react-hook-form';
import * as z from 'zod';

import { useT } from '@/i18n';
import { useLocale } from '@/hooks/use-locale';
import { usePhoneCheck, usePhoneCode, useStartRegister, useSystemFeatures } from '@/hooks';
import { cn } from '@/lib/utils';
import { isValidPhoneNumber } from '@/utils/validation';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { Icons } from '@/components/ui/icons';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';

interface RegisterFormProps {
  className?: string;
}

type RegisterMethod = 'email' | 'phone';

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

const registerInputClassName =
  'h-11 rounded-xl border-[var(--border-default)] px-4 text-base text-[var(--text-primary)] placeholder:text-[var(--placeholder)] focus-visible:border-[var(--brand-primary)]';

const registerPrimaryButtonClassName =
  'h-11 w-full rounded-xl bg-[var(--button-primary)] text-base font-bold tracking-normal text-white shadow-[0_12px_28px_-18px_rgba(17,24,39,0.7)] hover:bg-[var(--button-primary-hover)] hover:brightness-100';

const registerTabTriggerClassName =
  'h-9 gap-2 rounded-xl border border-transparent text-base font-semibold text-[var(--text-primary)] shadow-none data-[state=active]:border-[var(--brand-primary-border)] data-[state=active]:bg-white data-[state=active]:text-[var(--brand-primary)] data-[state=active]:shadow-[0_4px_12px_rgba(37,99,235,0.08)]';

export function RegisterForm({ className }: RegisterFormProps) {
  const router = useRouter();
  const searchParams = useSearchParams();
  const redirect = searchParams.get('redirect');
  const loginHref = redirect ? `/login?redirect=${encodeURIComponent(redirect)}` : '/login';
  const t = useT('auth');
  const tCommon = useT('common');
  const { locale } = useLocale();

  const [mounted, setMounted] = useState(false);
  const [registerMethod, setRegisterMethod] = useState<RegisterMethod>('email');
  const [registerStep, setRegisterStep] = useState<'email' | 'verifying'>('email');
  const [refreshing, setRefreshing] = useState(false);

  const startRegisterMutation = useStartRegister();
  const phoneCheckMutation = usePhoneCheck({ silentSuccess: true });
  const phoneCodeMutation = usePhoneCode();
  const { data: systemFeatures, refetch } = useSystemFeatures();

  const canRegister = Boolean(systemFeatures?.is_allow_register);

  const emailRegisterSchema = z.object({
    email: z.string().min(1, t('emailRequired')).email(t('invalidEmail')),
  });

  const phoneRegisterSchema = z.object({
    phone: z
      .string()
      .min(1, t('phoneRequired'))
      .refine(value => isValidPhoneNumber(value, 'INTL'), t('invalidPhoneNumber')),
  });

  type EmailRegisterFormData = z.infer<typeof emailRegisterSchema>;
  type PhoneRegisterFormData = z.infer<typeof phoneRegisterSchema>;

  const emailForm = useForm<EmailRegisterFormData>({
    resolver: zodResolver(emailRegisterSchema),
    defaultValues: {
      email: '',
    },
  });

  const phoneForm = useForm<PhoneRegisterFormData>({
    resolver: zodResolver(phoneRegisterSchema),
    defaultValues: {
      phone: '',
    },
  });

  useEffect(() => {
    setMounted(true);
  }, []);

  const onRefresh = async (): Promise<void> => {
    setRefreshing(true);
    try {
      await refetch();
    } finally {
      setRefreshing(false);
    }
  };

  const appendRedirect = (url: string): string => {
    if (!redirect) {
      return url;
    }

    return `${url}&redirect=${encodeURIComponent(redirect)}`;
  };

  const onEmailSubmit = async (data: EmailRegisterFormData) => {
    try {
      const response = await startRegisterMutation.mutateAsync({
        email: data.email,
        language: locale,
      });

      if (response.result === 'success') {
        setRegisterStep('verifying');
        const verifyUrl = appendRedirect(
          `/verify?email=${encodeURIComponent(data.email)}&token=${response.token}&type=register`
        );
        router.push(verifyUrl);
      }
    } catch (err: unknown) {
      console.error('Start registration failed:', err);
    }
  };

  const onPhoneSubmit = async (data: PhoneRegisterFormData) => {
    try {
      const countryCode = DEFAULT_PHONE_COUNTRY_CODE;

      const checkResponse = await phoneCheckMutation.mutateAsync({
        country_code: countryCode,
        phone: data.phone,
      });

      if (checkResponse.is_registered) {
        phoneForm.setError('phone', {
          message: t('userAlreadyExists'),
        });
        return;
      }

      const response = await phoneCodeMutation.mutateAsync({
        country_code: countryCode,
        phone: data.phone,
        scene: 'register',
      });

      const verifyUrl = appendRedirect(
        `/verify?method=phone&type=register&phone=${encodeURIComponent(data.phone)}` +
          `&country_code=${encodeURIComponent(countryCode)}&token=${encodeURIComponent(response.token)}`
      );
      router.push(verifyUrl);
    } catch (err) {
      console.error('Phone registration start failed:', err);
    }
  };

  if (!canRegister) {
    return (
      <div className={cn('flex flex-col gap-6', className)} style={authThemeVars}>
        <Card className="overflow-hidden rounded-[28px] border border-[var(--border-default)] bg-white/95 shadow-[0_18px_48px_-28px_rgba(15,23,42,0.35),0_8px_24px_-18px_rgba(15,23,42,0.18)] backdrop-blur-xl">
          <CardContent className="p-8">
            <Alert className="border-[var(--brand-primary-border)] bg-[var(--bg-soft)] text-[var(--text-primary)]">
              <Icons.Info className="h-4 w-4" />
              <AlertDescription>{t('registrationDisabled')}</AlertDescription>
            </Alert>
            <div className="mt-4 flex justify-end">
              <Button
                onClick={onRefresh}
                disabled={refreshing}
                className="h-11 min-w-28 rounded-xl bg-[var(--button-primary)] text-white hover:bg-[var(--button-primary-hover)] hover:brightness-100"
              >
                {refreshing ? <Icons.Spinner className="mr-2 h-4 w-4 animate-spin" /> : null}
                {tCommon('refresh')}
              </Button>
            </div>
          </CardContent>
        </Card>
      </div>
    );
  }

  const emailFormLoading = startRegisterMutation.isPending;
  const phoneFormLoading =
    phoneCheckMutation.isPending || phoneCodeMutation.isPending || phoneForm.formState.isSubmitting;
  const authRichT = t as typeof t & {
    rich: (
      key: 'byCreatingAccount',
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
            {t('createAccount')}
          </CardTitle>
          <p className="text-base text-[var(--text-secondary)]">
            {registerMethod === 'phone' ? t('phoneRegisterDesc') : t('enterEmailToStart')}
          </p>
        </div>

        <CardContent className="px-8 pb-9">
          <Tabs
            value={registerMethod}
            onValueChange={value => setRegisterMethod(value as RegisterMethod)}
            className="w-full"
          >
            <TabsList className="grid h-11 w-full grid-cols-2 rounded-2xl border border-[var(--border-default)] bg-[var(--bg-soft)] p-1 text-[var(--text-primary)] shadow-none">
              <TabsTrigger value="email" className={registerTabTriggerClassName}>
                <Mail className="size-5" />
                {t('authMethodEmail')}
              </TabsTrigger>
              <TabsTrigger value="phone" className={registerTabTriggerClassName}>
                <Smartphone className="size-5" />
                {t('authMethodPhone')}
              </TabsTrigger>
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
                    {t('emailAddress')}
                  </Label>
                  <Input
                    id="email"
                    type="email"
                    leftIcon={<Mail />}
                    placeholder={t('emailPlaceholder')}
                    disabled={emailFormLoading || registerStep === 'verifying'}
                    autoComplete="email"
                    {...emailForm.register('email')}
                    aria-invalid={emailForm.formState.errors.email ? 'true' : 'false'}
                    errorText={emailForm.formState.errors.email?.message}
                    className={registerInputClassName}
                  />
                </div>

                <Button
                  type="submit"
                  size="xl"
                  className={registerPrimaryButtonClassName}
                  loading={emailFormLoading}
                  disabled={registerStep === 'verifying' || !emailForm.watch('email')}
                  interactive
                >
                  {registerStep === 'verifying' ? t('verificationCodeSent') : t('continue')}
                </Button>
              </form>
            </TabsContent>

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
                    className={registerInputClassName}
                  />
                </div>

                <Button
                  type="submit"
                  size="xl"
                  className={registerPrimaryButtonClassName}
                  loading={phoneFormLoading}
                  disabled={!phoneForm.watch('phone')}
                  interactive
                >
                  {t('continue')}
                </Button>
              </form>
            </TabsContent>
          </Tabs>

          <div className="mt-8 animate-in fade-in text-center duration-700 delay-300">
            <p className="text-sm text-[var(--text-secondary)]">
              {t('alreadyHaveAccount')}{' '}
              <Link
                href={loginHref}
                className="font-bold text-[var(--brand-primary)] transition-colors hover:text-[var(--brand-primary-hover)]"
              >
                {t('signInLink')}
              </Link>
            </p>
          </div>
        </CardContent>
      </Card>

      <p className="mx-auto max-w-[320px] px-8 text-center text-[10px] leading-relaxed text-[var(--text-muted)] animate-in fade-in duration-700 delay-500">
        {authRichT.rich('byCreatingAccount', {
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
      </p>
    </div>
  );
}
