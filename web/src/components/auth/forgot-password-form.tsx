'use client';

import { useState, useEffect } from 'react';
import { useRouter } from 'next/navigation';
import Link from 'next/link';
import { zodResolver } from '@hookform/resolvers/zod';
import { useForm } from 'react-hook-form';
import { useLocale } from '@/hooks/use-locale';
import * as z from 'zod';

import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { CheckCircle } from 'lucide-react';
import { useForgotPassword } from '@/hooks/auth/use-forgot-password';
import { authenticationService } from '@/services/auth.service';
import { useT } from '@/i18n';
import { toast } from 'sonner';

interface ForgotPasswordFormProps {
  className?: string;
}

export function ForgotPasswordForm({ className }: ForgotPasswordFormProps) {
  const router = useRouter();
  const [isCodeSent, setIsCodeSent] = useState(false);
  const [isCheckingEmail, setIsCheckingEmail] = useState(false);
  const t = useT().auth;
  const { locale } = useLocale();

  // Form validation schema with translated messages
  const forgotPasswordSchema = z.object({
    email: z.string().min(1, t('emailRequired')).email(t('invalidEmail')),
  });

  type ForgotPasswordFormData = z.infer<typeof forgotPasswordSchema>;

  // Auth state
  const forgotPasswordMutation = useForgotPassword();
  const isLoading = isCheckingEmail || forgotPasswordMutation.isPending;

  // Form setup
  const {
    register,
    handleSubmit,
    watch,
    formState: { errors },
  } = useForm<ForgotPasswordFormData>({
    resolver: zodResolver(forgotPasswordSchema),
    defaultValues: {
      email: '',
    },
  });

  const emailValue = watch('email');

  const [mounted, setMounted] = useState(false);
  useEffect(() => {
    setMounted(true);
  }, []);

  // Form submission
  const onSubmit = async (data: ForgotPasswordFormData) => {
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
              <AlertDescription>{t('codeSentToEmail')}</AlertDescription>
            </Alert>
          )}

          {/* Forgot Password Form */}
          <form
            onSubmit={handleSubmit(onSubmit)}
            className={cn(
              'space-y-6',
              mounted
                ? 'animate-in fade-in slide-in-from-bottom-4 duration-700 delay-100'
                : 'opacity-0'
            )}
          >
            {/* Email Field */}
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

            {/* Submit Button */}
            <Button
              type="submit"
              size="xl"
              className="w-full font-bold tracking-wide"
              loading={isLoading}
              disabled={isCodeSent || !emailValue}
              interactive
            >
              {isCodeSent ? t('codeSent') : t('sendCode')}
            </Button>
          </form>

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
