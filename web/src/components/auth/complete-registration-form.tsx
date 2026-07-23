'use client';

import { useEffect, useState } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import Link from 'next/link';
import { zodResolver } from '@hookform/resolvers/zod';
import { Controller, useForm } from 'react-hook-form';
import { useT } from '@/i18n';
import * as z from 'zod';
import { mapPasswordErrorsToI18nKeys, validatePassword } from '@/utils/validation';

import { cn } from '@/lib/utils';
import { withBasePathIfInternal } from '@/lib/config';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Input, PasswordInput } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { Checkbox } from '@/components/ui/checkbox';
import { CheckCircle, Loader2 } from 'lucide-react';
import { useFinishRegister } from '@/hooks/auth/use-finish-register';
import { useSystemFeatures } from '@/hooks/auth/use-system-features';
import { usePhoneRegister } from '@/hooks/auth/use-phone-auth';
import { isPhoneRegisterEnabled } from '@/lib/features/notification-sms';

interface CompleteRegistrationFormProps {
  className?: string;
}

const DEFAULT_PHONE_COUNTRY_CODE = 'CN';

function normalizeCountryCode(value: string): string {
  const normalized = value.trim().toUpperCase();

  if (normalized === '+86' || normalized === '86') {
    return 'CN';
  }

  return normalized.startsWith('+') ? normalized.slice(1) : normalized;
}

export function CompleteRegistrationForm({ className }: CompleteRegistrationFormProps) {
  const router = useRouter();
  const searchParams = useSearchParams();
  const email = searchParams.get('email');
  const token = searchParams.get('token');
  const phone = searchParams.get('phone');
  const countryCode = searchParams.get('country_code') || DEFAULT_PHONE_COUNTRY_CODE;
  const verifiedToken = searchParams.get('verified_token');
  const isPhoneRegisterFlow = searchParams.get('method') === 'phone';
  const tAuth = useT().auth;

  const [isSuccess, setIsSuccess] = useState(false);

  const completeRegistrationSchema = z
    .object({
      name: z
        .string()
        .min(1, tAuth('nameRequired'))
        .min(2, tAuth('nameMinLength'))
        .max(50, tAuth('nameTooLong')),
      password: z.string(),
      confirmPassword: z.string().min(1, tAuth('confirmPasswordRequired')),
      terms: z.boolean().refine(val => val === true, tAuth('termsRequired')),
    })
    .superRefine((data, ctx) => {
      const result = validatePassword(data.password, {
        min: 8,
        max: 64,
        requireUpper: true,
        requireLower: true,
        requireNumber: true,
        requireSpecial: false,
        forbidWhitespace: true,
      });
      if (!result.valid) {
        const keys = mapPasswordErrorsToI18nKeys(result.errors);
        keys.forEach((key: string) => {
          ctx.addIssue({
            code: z.ZodIssueCode.custom,
            path: ['password'],
            message: tAuth(key as unknown as Parameters<typeof tAuth>[0]),
          });
        });
      }

      if (data.password !== data.confirmPassword) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          path: ['confirmPassword'],
          message: tAuth('passwordsDoNotMatch'),
        });
      }
    });

  type CompleteRegistrationFormData = z.infer<typeof completeRegistrationSchema>;

  const finishRegisterMutation = useFinishRegister();
  const phoneRegisterMutation = usePhoneRegister();
  const { data: systemFeatures } = useSystemFeatures();
  const systemFeaturesLoaded = systemFeatures !== undefined;
  const phoneRegisterEnabled = isPhoneRegisterEnabled(systemFeatures);
  const phoneRegisterUnavailable =
    isPhoneRegisterFlow && systemFeaturesLoaded && !phoneRegisterEnabled;

  const {
    register: registerField,
    handleSubmit,
    control,
    formState: { errors, isSubmitting },
  } = useForm<CompleteRegistrationFormData>({
    resolver: zodResolver(completeRegistrationSchema),
    defaultValues: {
      name: '',
      password: '',
      confirmPassword: '',
      terms: false,
    },
  });

  const onSubmit = async (data: CompleteRegistrationFormData) => {
    try {
      if (isPhoneRegisterFlow) {
        if (!phone || !verifiedToken || phoneRegisterUnavailable) {
          return;
        }

        await phoneRegisterMutation.mutateAsync({
          phone,
          country_code: normalizeCountryCode(countryCode),
          verified_token: verifiedToken,
          name: data.name,
          password: data.password,
        });
      } else {
        if (!email || !token) {
          return;
        }

        await finishRegisterMutation.mutateAsync({
          email,
          name: data.name,
          password: data.password,
          password_confirm: data.confirmPassword,
          token,
        });
      }

      setIsSuccess(true);
      setTimeout(() => {
        const redirectUrl = withBasePathIfInternal(searchParams.get('redirect') || '/console');
        window.location.href = redirectUrl;
      }, 2000);
    } catch (err) {
      console.error('Registration completion failed:', err);
    }
  };

  const isFormLoading =
    finishRegisterMutation.isPending || phoneRegisterMutation.isPending || isSubmitting;

  useEffect(() => {
    if (phoneRegisterUnavailable) {
      router.replace('/register');
    }
  }, [phoneRegisterUnavailable, router]);

  if (phoneRegisterUnavailable) {
    return null;
  }

  if (!isPhoneRegisterFlow && (!email || !token)) {
    if (typeof window !== 'undefined') {
      router.push('/register');
    }
    return null;
  }

  if (isPhoneRegisterFlow && (!phone || !verifiedToken)) {
    if (typeof window !== 'undefined') {
      router.push('/register');
    }
    return null;
  }

  const identifierLabel = isPhoneRegisterFlow ? tAuth('phone') : tAuth('email');
  const identifierValue = isPhoneRegisterFlow ? phone || '' : email || '';

  return (
    <div className={cn('flex flex-col gap-6', className)}>
      <Card>
        <CardHeader className="text-center">
          <CardTitle className="text-2xl font-bold">{tAuth('completeRegistrationTitle')}</CardTitle>
          <p className="text-muted-foreground">{tAuth('completeRegistrationDesc')}</p>
        </CardHeader>

        <CardContent className="space-y-6">
          {isSuccess ? (
            <Alert>
              <CheckCircle className="h-4 w-4" />
              <AlertDescription>{tAuth('accountCreatedSuccess')}</AlertDescription>
            </Alert>
          ) : (
            <form onSubmit={handleSubmit(onSubmit)} className="space-y-4">
              <div className="rounded-md border border-primary/20 bg-primary/10 text-primary p-3">
                <div className="text-sm font-semibold">{tAuth('passwordRequirements')}</div>
                <ul className="mt-1 space-y-1 text-sm">
                  <li>- {tAuth('passwordReq1')}</li>
                  <li>- {tAuth('passwordReq2')}</li>
                  <li>- {tAuth('passwordReq3')}</li>
                  <li>- {tAuth('passwordReq4')}</li>
                </ul>
              </div>

              <div className="space-y-2">
                <Label htmlFor="identifier">{identifierLabel}</Label>
                <Input id="identifier" value={identifierValue} disabled className="bg-muted" />
              </div>

              <div className="space-y-2">
                <Label htmlFor="name">{tAuth('fullName')}</Label>
                <Input
                  id="name"
                  type="text"
                  placeholder={tAuth('enterFullName')}
                  autoComplete="name"
                  disabled={isFormLoading}
                  {...registerField('name')}
                  aria-invalid={errors.name ? 'true' : 'false'}
                  errorText={errors.name?.message}
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="password">{tAuth('password')}</Label>
                <PasswordInput
                  id="password"
                  placeholder={tAuth('createStrongPassword')}
                  autoComplete="new-password"
                  disabled={isFormLoading}
                  {...registerField('password')}
                  aria-invalid={errors.password ? 'true' : 'false'}
                  errorText={errors.password?.message}
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="confirmPassword">{tAuth('confirmPassword')}</Label>
                <PasswordInput
                  id="confirmPassword"
                  placeholder={tAuth('confirmYourPassword')}
                  autoComplete="new-password"
                  disabled={isFormLoading}
                  {...registerField('confirmPassword')}
                  aria-invalid={errors.confirmPassword ? 'true' : 'false'}
                  errorText={errors.confirmPassword?.message}
                />
              </div>

              <div className="flex items-start space-x-2">
                <Controller
                  name="terms"
                  control={control}
                  render={({ field }) => (
                    <Checkbox
                      id="terms"
                      disabled={isFormLoading}
                      checked={field.value}
                      onCheckedChange={field.onChange}
                      aria-invalid={errors.terms ? 'true' : 'false'}
                    />
                  )}
                />
                <div className="grid gap-1.5 leading-none">
                  <Label htmlFor="terms" className="text-sm font-normal">
                    {tAuth.rich('agreeToTerms', {
                      termsLink: (chunks: React.ReactNode) => (
                        <Link
                          href="/terms"
                          className="underline hover:text-foreground"
                          target="_blank"
                        >
                          {chunks}
                        </Link>
                      ),
                      privacyLink: (chunks: React.ReactNode) => (
                        <Link
                          href="/privacy"
                          className="underline hover:text-foreground"
                          target="_blank"
                        >
                          {chunks}
                        </Link>
                      ),
                    })}
                  </Label>
                  {errors.terms && (
                    <p className="text-sm text-destructive">{errors.terms.message}</p>
                  )}
                </div>
              </div>

              <Button type="submit" className="w-full" disabled={isFormLoading}>
                {isFormLoading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                {tAuth('completeRegistrationBtn')}
              </Button>
            </form>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
