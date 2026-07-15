'use client';

import { useState, useEffect } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import Link from 'next/link';
import { zodResolver } from '@hookform/resolvers/zod';
import { useForm } from 'react-hook-form';
import { useT } from '@/i18n';
import { useLocale } from '@/hooks/use-locale';
import * as z from 'zod';

import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { Info, Loader2 } from 'lucide-react';
import { useStartRegister } from '@/hooks/auth/use-start-register';
import { useSystemFeatures } from '@/hooks/auth/use-system-features';

interface RegisterFormProps {
  className?: string;
}

export function RegisterForm({ className }: RegisterFormProps) {
  const router = useRouter();
  const searchParams = useSearchParams();
  const redirect = searchParams.get('redirect');
  const loginHref = redirect ? `/login?redirect=${encodeURIComponent(redirect)}` : '/login';
  const [registerStep, setRegisterStep] = useState<'email' | 'verifying'>('email');
  const t = useT().auth;
  const tCommon = useT('common');
  const { locale } = useLocale();

  // Form validation schema with translated messages
  const registerFormSchema = z.object({
    email: z.string().min(1, t('emailRequired')).email(t('invalidEmail')),
  });

  type RegisterFormData = z.infer<typeof registerFormSchema>;

  // Auth state
  const startRegisterMutation = useStartRegister();
  const isLoading = startRegisterMutation.isPending;
  const { data: systemFeatures, refetch } = useSystemFeatures();
  const canRegister = Boolean(systemFeatures?.is_allow_register);
  const [refreshing, setRefreshing] = useState<boolean>(false);
  const onRefresh = async (): Promise<void> => {
    setRefreshing(true);
    try {
      await refetch();
    } finally {
      setRefreshing(false);
    }
  };

  // Form setup
  const {
    register: registerField,
    handleSubmit,
    watch,
    formState: { errors },
  } = useForm<RegisterFormData>({
    resolver: zodResolver(registerFormSchema),
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
  const onSubmit = async (data: RegisterFormData) => {
    try {
      const response = await startRegisterMutation.mutateAsync({
        email: data.email,
        language: locale,
      });

      if (response.result === 'success') {
        setRegisterStep('verifying');

        // Redirect to verification page with token
        let verifyUrl = `/verify?email=${encodeURIComponent(data.email)}&token=${response.token}&type=register`;
        if (redirect) {
          verifyUrl += `&redirect=${encodeURIComponent(redirect)}`;
        }
        router.push(verifyUrl);
      }
    } catch (err: unknown) {
      // Errors are handled in hook with toast
      console.error('Start registration failed:', err);
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
          <p className="text-muted-foreground/80">{t('enterEmailToStart')}</p>
        </div>

        <CardContent className="px-8 pb-10">
          {/* Register Form */}
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
                disabled={isLoading || registerStep === 'verifying'}
                autoComplete="email"
                {...registerField('email')}
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
              disabled={registerStep === 'verifying' || !emailValue}
              interactive
            >
              {registerStep === 'verifying' ? t('verificationCodeSent') : t('continue')}
            </Button>
          </form>

          {/* Links */}
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

      {/* Terms and Privacy */}
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
