'use client';

import { useState, useEffect } from 'react';
import { useT } from '@/i18n';
import Link from 'next/link';
import { useSearchParams } from 'next/navigation';
import { zodResolver } from '@hookform/resolvers/zod';
import { useForm } from 'react-hook-form';
import * as z from 'zod';

import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardTitle } from '@/components/ui/card';
import { Input, PasswordInput } from '@/components/ui/input';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { Info, Shield } from 'lucide-react';
import { useLogin } from '@/hooks/auth/use-login';
import { useSystemFeatures } from '@/hooks/auth/use-system-features';
import { Label } from '../ui/label';
import { withBasePathIfInternal } from '@/lib/config';
import { buildSsoStartUrl } from '@/utils/auth-sso';
import { getAuthBusinessErrorCode, getAuthBusinessErrorData } from '@/utils/auth-errors';

interface LoginFormProps {
  className?: string;
}

export function LoginForm({ className }: LoginFormProps) {
  const t = useT().auth;
  const searchParams = useSearchParams();

  // Get email from URL params
  const emailFromParams = decodeURIComponent(searchParams.get('email') || '');

  // Form validation schema with translated messages
  const loginSchema = z.object({
    email: z.string().min(1, t('emailRequired')).email(t('invalidEmail')),
    password: z.string().min(8, t('passwordTooShort')).max(100, t('passwordTooLong')),
    invite_token: z.string().optional(),
  });

  type LoginFormData = z.infer<typeof loginSchema>;

  const loginMutation = useLogin();
  const isLoading = loginMutation.isPending;
  const { data: systemFeatures } = useSystemFeatures();

  // Derived state
  const canRegister = Boolean(systemFeatures?.is_allow_register);
  const hasSocialLogin = Boolean(systemFeatures?.enable_social_oauth_login);

  // Form setup
  const {
    register,
    handleSubmit,
    formState: { errors, isSubmitting },
  } = useForm<LoginFormData>({
    resolver: zodResolver(loginSchema),
    defaultValues: {
      email: emailFromParams,
      password: '',
    },
  });
  const inviteToken = searchParams.get('invite_token');
  const [mounted, setMounted] = useState(false);

  useEffect(() => {
    setMounted(true);
  }, []);

  // Form submission
  const onSubmit = async (data: LoginFormData) => {
    try {
      const formData = data;
      if (inviteToken) {
        formData.invite_token = inviteToken;
      }
      await loginMutation.mutateAsync(formData);
      // Determine redirect target (default to the console homepage)
      const urlParams = new URLSearchParams(window.location.search);
      const redirectUrl = withBasePathIfInternal(urlParams.get('redirect') || '/console');

      // Use hard navigation to fully reload the page and trigger new
      // server-side rendering & data fetching.
      window.location.href = redirectUrl;
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

      // Error is handled by the store
      console.error('Login failed:', err);
    }
  };

  const onSsoLogin = () => {
    const redirect = withBasePathIfInternal(searchParams.get('redirect') || '/console');
    window.location.href = buildSsoStartUrl('casdoor', redirect);
  };

  const isFormLoading = isLoading || isSubmitting;

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
          <p className="text-muted-foreground/80">{t('signInToAccount')}</p>
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

          {/* Login Form */}
          <form
            onSubmit={handleSubmit(onSubmit)}
            className={cn(
              'space-y-5',
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
                {t('email')}
              </Label>
              <Input
                id="email"
                type="email"
                placeholder={t('enterEmail')}
                autoComplete="email"
                disabled={isFormLoading || !!inviteToken}
                {...register('email')}
                aria-invalid={errors.email ? 'true' : 'false'}
                errorText={errors.email?.message}
                className="h-11 px-4 text-base"
              />
            </div>

            {/* Password Field */}
            <div className="space-y-2">
              <div className="flex items-center justify-between ml-1">
                <Label
                  htmlFor="password"
                  className="text-xs font-semibold uppercase tracking-wider opacity-60"
                >
                  {t('password')}
                </Label>
                <Link
                  href="/forgot-password"
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
                disabled={isFormLoading}
                {...register('password')}
                errorText={errors.password?.message}
                className="h-11 px-4 text-base"
              />
            </div>

            {/* Submit Button */}
            <Button
              type="submit"
              size="xl"
              className="w-full font-bold tracking-wide mt-2"
              loading={isFormLoading}
              interactive
            >
              {t('signIn')}
            </Button>
          </form>

          {/* Social Login */}
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
                  disabled={isFormLoading}
                  className="glass-panel border-border/30 hover:bg-muted/50 transition-all gap-2"
                  onClick={onSsoLogin}
                >
                  <Shield className="h-5 w-5" />
                  <span>{t('signInWithSSO')}</span>
                </Button>
              </div>
            </div>
          )}

          {/* Registration Link */}
          {canRegister && (
            <div className="text-center text-sm pt-2 animate-in fade-in duration-700 delay-500">
              <span className="text-muted-foreground">{t('dontHaveAccount')}</span>{' '}
              <Link
                href="/register"
                className="font-bold text-primary hover:text-primary-hover transition-colors"
              >
                {t('createAccount')}
              </Link>
            </div>
          )}
        </CardContent>
      </Card>

      {/* Terms and Privacy */}
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
