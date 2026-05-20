'use client';

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
import { Icons } from '@/components/ui/icons';
import { useResetPassword } from '@/hooks';
import { toast } from 'sonner';

interface ResetPasswordFormProps {
  className?: string;
}

export function ResetPasswordForm({ className }: ResetPasswordFormProps) {
  const router = useRouter();
  const searchParams = useSearchParams();
  const email = searchParams.get('email');
  const token = searchParams.get('token');
  const t = useT('auth');
  const { locale } = useLocale();

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
  const isLoading = resetPasswordMutation.isPending;

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
    if (!email || !token) {
      toast.error('Error');
      return;
    }

    try {
      const result = await resetPasswordMutation.mutateAsync({
        email,
        password: data.password,
        password_confirm: data.confirmPassword,
        token,
        language: locale,
      });

      if (result) {
        router.push('/login');
      }
    } catch (_err) {
      // Error is handled by the store
    }
  };

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
              {isLoading && <Icons.Spinner className="mr-2 h-4 w-4 animate-spin" />}
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
