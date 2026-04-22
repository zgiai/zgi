'use client';

import { useEffect, useState } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import Link from 'next/link';
import { zodResolver } from '@hookform/resolvers/zod';
import { useForm } from 'react-hook-form';
import { useT } from '@/i18n';
import { useLocale } from '@/hooks/use-locale';
import * as z from 'zod';

import { cn } from '@/lib/utils';
import { isValidPhoneNumber } from '@/utils/validation';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Info, Loader2 } from 'lucide-react';
import { useStartRegister } from '@/hooks/auth/use-start-register';
import { useSystemFeatures } from '@/hooks/auth/use-system-features';
import { usePhoneCheck, usePhoneCode } from '@/hooks/auth/use-phone-auth';

interface RegisterFormProps {
  className?: string;
}

type RegisterMethod = 'email' | 'phone';

const DEFAULT_PHONE_COUNTRY_CODE = 'CN';

export function RegisterForm({ className }: RegisterFormProps) {
  const router = useRouter();
  const searchParams = useSearchParams();
  const redirect = searchParams.get('redirect');
  const loginHref = redirect ? `/login?redirect=${encodeURIComponent(redirect)}` : '/login';
  const t = useT().auth;
  const tCommon = useT('common');
  const { locale } = useLocale();

  const [mounted, setMounted] = useState(false);
  const [registerMethod, setRegisterMethod] = useState<RegisterMethod>('email');
  const [emailRegisterStep, setEmailRegisterStep] = useState<'email' | 'verifying'>('email');
  const [refreshing, setRefreshing] = useState<boolean>(false);

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

  const startRegisterMutation = useStartRegister();
  const phoneCheckMutation = usePhoneCheck({
    silentSuccess: true,
    errorMessageKey: 'userAlreadyExists',
  });
  const phoneCodeMutation = usePhoneCode();
  const { data: systemFeatures, refetch } = useSystemFeatures();
  const canRegister = Boolean(systemFeatures?.is_allow_register);

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
        setEmailRegisterStep('verifying');
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
      const checkResponse = await phoneCheckMutation.mutateAsync({
        country_code: DEFAULT_PHONE_COUNTRY_CODE,
        phone: data.phone,
      });

      if (checkResponse.is_registered) {
        phoneForm.setError('phone', {
          message: t('userAlreadyExists'),
        });
        return;
      }

      const response = await phoneCodeMutation.mutateAsync({
        country_code: DEFAULT_PHONE_COUNTRY_CODE,
        phone: data.phone,
        scene: 'register',
      });

      const verifyUrl = appendRedirect(
        `/verify?method=phone&type=register&phone=${encodeURIComponent(data.phone)}` +
          `&country_code=${encodeURIComponent(DEFAULT_PHONE_COUNTRY_CODE)}` +
          `&token=${encodeURIComponent(response.token)}`
      );
      router.push(verifyUrl);
    } catch (err) {
      console.error('Phone registration start failed:', err);
    }
  };

  if (!canRegister) {
    return (
      <Card>
        <CardContent className="p-6">
          <Alert>
            <Info className="h-4 w-4" />
            <AlertDescription>{t('registrationDisabled')}</AlertDescription>
          </Alert>
          <div className="mt-4 flex justify-end">
            <Button onClick={onRefresh} disabled={refreshing} className="min-w-28">
              {refreshing && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              {tCommon('refresh')}
            </Button>
          </div>
        </CardContent>
      </Card>
    );
  }

  const emailFormLoading = startRegisterMutation.isPending;
  const phoneFormLoading =
    phoneCheckMutation.isPending || phoneCodeMutation.isPending || phoneForm.formState.isSubmitting;

  return (
    <div className={cn('flex flex-col gap-8', className)}>
      <Card className="glass-panel border-none shadow-premium overflow-hidden">
        <div
          className={cn(
            'p-8 pt-10 text-center space-y-2',
            mounted ? 'animate-in fade-in slide-in-from-top-4 duration-700' : 'opacity-0'
          )}
        >
          <CardTitle className="text-3xl font-bold tracking-tight">{t('createAccount')}</CardTitle>
          <p className="text-muted-foreground/80">
            {registerMethod === 'phone' ? t('phoneRegisterDesc') : t('enterEmailToStart')}
          </p>
        </div>

        <CardContent className="px-8 pb-10">
          <Tabs
            value={registerMethod}
            onValueChange={value => setRegisterMethod(value as RegisterMethod)}
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
              <form onSubmit={emailForm.handleSubmit(onEmailSubmit)} className="space-y-6">
                <div className="space-y-2">
                  <Label
                    htmlFor="email"
                    className="text-xs font-semibold uppercase tracking-wider opacity-60 ml-1"
                  >
                    {t('emailAddress')}
                  </Label>
                  <Input
                    id="email"
                    type="email"
                    placeholder={t('emailPlaceholder')}
                    disabled={emailFormLoading || emailRegisterStep === 'verifying'}
                    autoComplete="email"
                    {...emailForm.register('email')}
                    aria-invalid={emailForm.formState.errors.email ? 'true' : 'false'}
                    errorText={emailForm.formState.errors.email?.message}
                    className="h-11 px-4 text-base"
                  />
                </div>

                <Button
                  type="submit"
                  size="xl"
                  className="w-full font-bold tracking-wide"
                  loading={emailFormLoading}
                  disabled={emailRegisterStep === 'verifying' || !emailForm.watch('email')}
                  interactive
                >
                  {emailRegisterStep === 'verifying' ? t('verificationCodeSent') : t('continue')}
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
              <form onSubmit={phoneForm.handleSubmit(onPhoneSubmit)} className="space-y-6">
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

                <Button
                  type="submit"
                  size="xl"
                  className="w-full font-bold tracking-wide"
                  loading={phoneFormLoading}
                  disabled={!phoneForm.watch('phone')}
                  interactive
                >
                  {t('continue')}
                </Button>
              </form>
            </TabsContent>
          </Tabs>

          <div className="mt-8 text-center animate-in fade-in duration-700 delay-300">
            <p className="text-sm text-muted-foreground">
              {t('alreadyHaveAccount')}{' '}
              <Link
                href={loginHref}
                className="font-bold text-primary hover:text-primary-hover transition-colors"
              >
                {t('signInLink')}
              </Link>
            </p>
          </div>
        </CardContent>
      </Card>

      <p className="px-8 text-center text-[10px] text-muted-foreground/50 leading-relaxed max-w-[320px] mx-auto animate-in fade-in duration-700 delay-500">
        {t.rich('byCreatingAccount', {
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
      </p>
    </div>
  );
}
