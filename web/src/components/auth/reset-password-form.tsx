'use client';

import { useEffect } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import Link from 'next/link';
import { zodResolver } from '@hookform/resolvers/zod';
import { useForm } from 'react-hook-form';
import { useT } from '@/i18n';
import { useLocale } from '@/hooks/use-locale';
import * as z from 'zod';
import { validatePassword, mapPasswordErrorsToI18nKeys } from '@/utils/validation';

import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { PasswordInput } from '@/components/ui/input';
import { Loader2 } from 'lucide-react';
import { useResetPassword } from '@/hooks/auth/use-reset-password';
import { useSystemFeatures } from '@/hooks/auth/use-system-features';
import { usePhoneResetPassword } from '@/hooks/auth/use-phone-auth';
import { isPhonePasswordResetEnabled } from '@/lib/features/notification-sms';
import { toast } from 'sonner';

interface ResetPasswordFormProps {
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

export function ResetPasswordForm({ className }: ResetPasswordFormProps) {
  const router = useRouter();
  const searchParams = useSearchParams();
  const email = searchParams.get('email');
  const token = searchParams.get('token');
  const phone = searchParams.get('phone');
  const countryCode = searchParams.get('country_code') || DEFAULT_PHONE_COUNTRY_CODE;
  const verifiedToken = searchParams.get('verified_token');
  const isPhoneResetFlow = searchParams.get('method') === 'phone';
  const t = useT('auth');
  const { locale } = useLocale();
  const { data: systemFeatures } = useSystemFeatures();
  const systemFeaturesLoaded = systemFeatures !== undefined;
  const phoneResetEnabled = isPhonePasswordResetEnabled(systemFeatures);
  const phoneResetUnavailable = isPhoneResetFlow && systemFeaturesLoaded && !phoneResetEnabled;

  // Form validation schema with translated messages
  const resetPasswordSchema = z
    .object({
      password: z.string(),
      confirmPassword: z.string(),
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
            message: t(key as unknown as Parameters<typeof t>[0]),
          });
        });
      }

      if (data.password !== data.confirmPassword) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          path: ['confirmPassword'],
          message: t('passwordsNotMatch'),
        });
      }
    });

  type ResetPasswordFormData = z.infer<typeof resetPasswordSchema>;

  // Auth state
  const resetPasswordMutation = useResetPassword();
  const phoneResetPasswordMutation = usePhoneResetPassword();
  const isLoading = resetPasswordMutation.isPending || phoneResetPasswordMutation.isPending;

  // Form setup
  const {
    register: registerField,
    handleSubmit,
    formState: { errors },
  } = useForm<ResetPasswordFormData>({
    resolver: zodResolver(resetPasswordSchema),
    defaultValues: {
      password: '',
      confirmPassword: '',
    },
  });

  // Form submission
  const onSubmit = async (data: ResetPasswordFormData) => {
    if (isPhoneResetFlow && (!phone || !verifiedToken || phoneResetUnavailable)) {
      toast.error(t('missingTokenError'));
      return;
    }

    if (!isPhoneResetFlow && (!email || !token)) {
      toast.error(t('missingTokenError'));
      return;
    }

    try {
      if (isPhoneResetFlow) {
        await phoneResetPasswordMutation.mutateAsync({
          phone: phone || '',
          country_code: normalizeCountryCode(countryCode),
          verified_token: verifiedToken || '',
          new_password: data.password,
        });
      } else {
        const result = await resetPasswordMutation.mutateAsync({
          email: email || '',
          password: data.password,
          password_confirm: data.confirmPassword,
          token: token || '',
          language: locale,
        });

        if (!result) {
          return;
        }
      }

      router.push('/login');
    } catch (_err) {
      // Error is handled by the store
    }
  };

  useEffect(() => {
    if (phoneResetUnavailable) {
      router.replace('/forgot-password');
    }
  }, [phoneResetUnavailable, router]);

  if (phoneResetUnavailable) {
    return null;
  }

  return (
    <div className={cn('flex flex-col gap-6', className)}>
      <Card>
        <CardHeader className="text-center">
          <CardTitle className="text-2xl font-bold">{t('resetPasswordTitle2')}</CardTitle>
          <p className="text-muted-foreground">{t('createNewPassword')}</p>
        </CardHeader>

        <CardContent>
          {/* Errors are shown via toast; field-level errors remain */}

          {/* Reset Password Form */}
          <form onSubmit={handleSubmit(onSubmit)} className="space-y-6">
            {/* Password Field */}
            <div className="space-y-2">
              <PasswordInput
                id="password"
                placeholder={t('passwordPlaceholder')}
                disabled={isLoading}
                {...registerField('password')}
                aria-invalid={errors.password ? 'true' : 'false'}
                errorText={errors.password?.message}
              />
            </div>

            {/* Confirm Password Field */}
            <div className="space-y-2">
              <PasswordInput
                id="confirmPassword"
                placeholder={t('passwordPlaceholder')}
                disabled={isLoading}
                {...registerField('confirmPassword')}
                aria-invalid={errors.confirmPassword ? 'true' : 'false'}
                errorText={errors.confirmPassword?.message}
              />
            </div>

            {/* Submit Button */}
            <Button type="submit" className="w-full" disabled={isLoading}>
              {isLoading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              {t('resetPasswordBtn')}
            </Button>
          </form>

          {/* Back Link */}
          <div className="text-center mt-6">
            <Link href="/login" className="text-sm font-medium text-primary hover:underline">
              {t('backToLogin')}
            </Link>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
