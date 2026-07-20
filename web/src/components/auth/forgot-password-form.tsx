'use client';

import { useState, useEffect } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import Link from 'next/link';
import { zodResolver } from '@hookform/resolvers/zod';
import { useForm } from 'react-hook-form';
import { useLocale } from '@/hooks/use-locale';
import * as z from 'zod';

import { isPhonePasswordResetEnabled } from '@/lib/features/notification-sms';
import { cn } from '@/lib/utils';
import { isValidPhoneNumber } from '@/utils/validation';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { CheckCircle, Mail, Smartphone } from 'lucide-react';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { useForgotPassword } from '@/hooks/auth/use-forgot-password';
import { usePhoneCheck, usePhoneCode, useSystemFeatures } from '@/hooks';
import { authenticationService } from '@/services/auth.service';
import { useT } from '@/i18n';
import { toast } from 'sonner';

interface ForgotPasswordFormProps {
  className?: string;
}

type ForgotPasswordMethod = 'email' | 'phone';

const DEFAULT_PHONE_COUNTRY_CODE = 'CN';
const forgotPasswordTabTriggerClassName =
  'h-9 gap-2 rounded-xl border border-transparent text-base font-semibold shadow-none data-[state=active]:bg-background';

export function ForgotPasswordForm({ className }: ForgotPasswordFormProps) {
  const router = useRouter();
  const searchParams = useSearchParams();
  const requestedMethod =
    searchParams.get('method') === 'phone' || searchParams.has('phone') ? 'phone' : 'email';
  const [isCodeSent, setIsCodeSent] = useState(false);
  const [isCheckingEmail, setIsCheckingEmail] = useState(false);
  const [forgotPasswordMethod, setForgotPasswordMethod] =
    useState<ForgotPasswordMethod>(requestedMethod);
  const t = useT().auth;
  const { locale } = useLocale();
  const emailFromParams = searchParams.get('email') ?? '';
  const phoneFromParams = searchParams.get('phone') ?? '';

  const forgotPasswordSchema = z.object({
    email: z.string().min(1, t('emailRequired')).email(t('invalidEmail')),
  });
  const phoneForgotPasswordSchema = z.object({
    phone: z
      .string()
      .min(1, t('phoneRequired'))
      .refine(value => isValidPhoneNumber(value, 'INTL'), t('invalidPhoneNumber')),
  });

  type ForgotPasswordFormData = z.infer<typeof forgotPasswordSchema>;
  type PhoneForgotPasswordFormData = z.infer<typeof phoneForgotPasswordSchema>;

  const forgotPasswordMutation = useForgotPassword();
  const phoneCheckMutation = usePhoneCheck({ silentSuccess: true });
  const phoneCodeMutation = usePhoneCode();
  const { data: systemFeatures } = useSystemFeatures();
  const hasPhonePasswordReset = isPhonePasswordResetEnabled(systemFeatures);
  const emailFormLoading = isCheckingEmail || forgotPasswordMutation.isPending;
  const phoneFormLoading =
    phoneCheckMutation.isPending || phoneCodeMutation.isPending;
  const isLoading = emailFormLoading || phoneFormLoading;

  const {
    register,
    handleSubmit: handleEmailSubmit,
    watch,
    formState: { errors },
  } = useForm<ForgotPasswordFormData>({
    resolver: zodResolver(forgotPasswordSchema),
    defaultValues: {
      email: emailFromParams,
    },
  });
  const {
    register: registerPhone,
    handleSubmit: handlePhoneSubmit,
    watch: watchPhone,
    formState: { errors: phoneErrors },
    setError: setPhoneError,
  } = useForm<PhoneForgotPasswordFormData>({
    resolver: zodResolver(phoneForgotPasswordSchema),
    defaultValues: {
      phone: phoneFromParams,
    },
  });

  const emailValue = watch('email');
  const phoneValue = watchPhone('phone');

  const [mounted, setMounted] = useState(false);
  useEffect(() => {
    setMounted(true);
  }, []);

  useEffect(() => {
    if (systemFeatures && !hasPhonePasswordReset && forgotPasswordMethod === 'phone') {
      setForgotPasswordMethod('email');
    }
  }, [forgotPasswordMethod, hasPhonePasswordReset, systemFeatures]);

  const onEmailSubmit = async (data: ForgotPasswordFormData) => {
    setIsCheckingEmail(true);
    try {
      const emailCheck = await authenticationService.checkEmail(data.email);
      if (!emailCheck.is_registered) {
        toast.error(t('errorSendingRecovery'), {
          description: t('businessErrors.accountNotFound'),
        });
        return;
      }
    } catch (_err) {
      toast.error(t('errorSendingRecovery'), {
        description: t('businessErrors.emailCheckFailed'),
      });
      return;
    } finally {
      setIsCheckingEmail(false);
    }

    try {
      const response = await forgotPasswordMutation.mutateAsync({
        email: data.email,
        language: locale,
      });
      if (response.result === 'success') {
        setIsCodeSent(true);
        router.push(
          `/verify?email=${encodeURIComponent(data.email)}&token=${response.token}&type=reset`
        );
      }
    } catch (_err) {
      // Error is handled by the mutation hook.
    }
  };

  const onPhoneSubmit = async (data: PhoneForgotPasswordFormData) => {
    if (!hasPhonePasswordReset) {
      setPhoneError('phone', {
        message: t('phoneAuthUnavailable'),
      });
      return;
    }

    try {
      const checkResponse = await phoneCheckMutation.mutateAsync({
        country_code: DEFAULT_PHONE_COUNTRY_CODE,
        phone: data.phone,
      });

      if (!checkResponse.is_registered) {
        setPhoneError('phone', { message: t('phoneNotRegistered') });
        return;
      }

      const response = await phoneCodeMutation.mutateAsync({
        country_code: DEFAULT_PHONE_COUNTRY_CODE,
        phone: data.phone,
        scene: 'reset_password',
      });

      setIsCodeSent(true);
      router.push(
        `/verify?method=phone&type=reset&phone=${encodeURIComponent(data.phone)}` +
          `&country_code=${encodeURIComponent(DEFAULT_PHONE_COUNTRY_CODE)}` +
          `&token=${encodeURIComponent(response.token)}`
      );
    } catch (_err) {
      // Error is handled by the mutation hook.
    }
  };

  const activeForgotPasswordMethod = hasPhonePasswordReset ? forgotPasswordMethod : 'email';

  return (
    <div className={cn('flex flex-col gap-8', className)}>
      <Card className="glass-panel border-none shadow-premium overflow-hidden">
        <div className="p-8 pt-10 text-center space-y-2 animate-in fade-in slide-in-from-top-4 duration-700">
          <CardTitle className="text-3xl font-bold tracking-tight">
            {t('resetPasswordTitle2')}
          </CardTitle>
          <p className="text-muted-foreground/80">{t('resetPasswordDesc')}</p>
        </div>

        <CardContent className="px-8 pb-10 space-y-6">
          {/* Success Alert */}
          {isCodeSent && (
            <Alert className="bg-success/5 border-success/20 text-success animate-in fade-in zoom-in-95 duration-500">
              <CheckCircle className="h-4 w-4" />
              <AlertDescription>
                {activeForgotPasswordMethod === 'phone' ? t('codeSentToPhone') : t('codeSentToEmail')}
              </AlertDescription>
            </Alert>
          )}

          <Tabs
            value={activeForgotPasswordMethod}
            onValueChange={value => setForgotPasswordMethod(value as ForgotPasswordMethod)}
            className="w-full"
          >
            <TabsList
              className={cn(
                'grid h-11 w-full rounded-2xl p-1',
                hasPhonePasswordReset ? 'grid-cols-2' : 'grid-cols-1'
              )}
            >
              <TabsTrigger value="email" className={forgotPasswordTabTriggerClassName}>
                <Mail className="size-5" />
                {t('authMethodEmail')}
              </TabsTrigger>
              {hasPhonePasswordReset ? (
                <TabsTrigger value="phone" className={forgotPasswordTabTriggerClassName}>
                  <Smartphone className="size-5" />
                  {t('authMethodPhone')}
                </TabsTrigger>
              ) : null}
            </TabsList>

            <TabsContent value="email" className="mt-6">
              <form
                onSubmit={handleEmailSubmit(onEmailSubmit)}
                className={cn(
                  'space-y-6',
                  mounted
                    ? 'animate-in fade-in slide-in-from-bottom-4 duration-700 delay-100'
                    : 'opacity-0'
                )}
              >
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
                    disabled={isLoading || isCodeSent}
                    {...register('email')}
                    aria-invalid={errors.email ? 'true' : 'false'}
                    errorText={errors.email?.message}
                    className="h-11 px-4 text-base"
                  />
                </div>

                <Button
                  type="submit"
                  size="xl"
                  className="w-full font-bold tracking-wide"
                  loading={emailFormLoading}
                  disabled={isCodeSent || !emailValue}
                  interactive
                >
                  {isCodeSent ? t('codeSent') : t('sendCode')}
                </Button>
              </form>
            </TabsContent>

            {hasPhonePasswordReset ? (
              <TabsContent value="phone" className="mt-6">
                <form
                  onSubmit={handlePhoneSubmit(onPhoneSubmit)}
                  className={cn(
                    'space-y-6',
                    mounted
                      ? 'animate-in fade-in slide-in-from-bottom-4 duration-700 delay-100'
                      : 'opacity-0'
                  )}
                >
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
                      disabled={isLoading || isCodeSent}
                      {...registerPhone('phone')}
                      aria-invalid={phoneErrors.phone ? 'true' : 'false'}
                      errorText={phoneErrors.phone?.message}
                      className="h-11 px-4 text-base"
                    />
                  </div>

                  <Button
                    type="submit"
                    size="xl"
                    className="w-full font-bold tracking-wide"
                    loading={phoneFormLoading}
                    disabled={isCodeSent || !phoneValue}
                    interactive
                  >
                    {isCodeSent ? t('codeSent') : t('sendCode')}
                  </Button>
                </form>
              </TabsContent>
            ) : null}
          </Tabs>

          {/* Links */}
          <div className="text-center space-y-4 animate-in fade-in duration-700 delay-300 pt-2">
            <Link
              href="/login"
              className="text-sm font-bold text-primary hover:text-primary-hover transition-colors block"
            >
              {t('backToLogin')}
            </Link>
            <p className="text-xs text-muted-foreground">
              {t.rich('dontHaveAccountRegister', {
                registerLink: chunks => (
                  <Link
                    href="/register"
                    className="font-bold text-primary hover:text-primary-hover transition-colors ml-1"
                  >
                    {chunks}
                  </Link>
                ),
              })}
            </p>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
