'use client';

import { useState, useEffect, useMemo } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import Link from 'next/link';
import { zodResolver } from '@hookform/resolvers/zod';
import { useForm } from 'react-hook-form';
import * as z from 'zod';
import { useT } from '@/i18n';
import { useLocale } from '@/hooks/use-locale';
import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Label } from '@/components/ui/label';
import { Icons } from '@/components/ui/icons';
import {
  useVerifyRegister,
  useVerifyForgotPassword,
  useForgotPassword,
  useStartRegister,
} from '@/hooks';
import { VerificationCodeInput } from '@/components/ui/verification-code-input';
import type { ForgotPasswordInitResponse, RegisterInitResponse } from '@/services/types/auth';

// Validation schema
const verificationSchema = z.object({
  code: z
    .string()
    .min(1, 'Verification code is required')
    .min(6, 'Code must be 6 digits')
    .max(6, 'Code must be 6 digits')
    .regex(/^\d+$/, 'Code must contain only digits'),
});

type VerificationFormData = z.infer<typeof verificationSchema>;

const RESEND_COOLDOWN_SECONDS = 60;
const RESEND_COOLDOWN_MS = RESEND_COOLDOWN_SECONDS * 1000;

interface ResendCooldownSnapshot {
  serverTime: number;
  resendAvailableAt: number;
  clientReceivedAt: number;
}

function buildResendCooldownKey(email: string, token: string, type: string): string {
  return `auth:verify:resend_cooldown:${type}:${email.toLowerCase()}:${token}`;
}

function parseTimestamp(value: unknown): number | null {
  const timestamp = Number(value);
  return Number.isFinite(timestamp) && timestamp > 0 ? timestamp : null;
}

function readResendCooldownSnapshot(key: string): ResendCooldownSnapshot | null {
  if (typeof window === 'undefined') {
    return null;
  }

  const value = window.localStorage.getItem(key);
  if (!value) {
    return null;
  }

  const fallbackTimestamp = parseTimestamp(value);
  if (fallbackTimestamp) {
    return createCooldownSnapshot({
      serverTime: Date.now(),
      resendAvailableAt: fallbackTimestamp,
    });
  }

  try {
    const parsed = JSON.parse(value) as Partial<ResendCooldownSnapshot>;
    const serverTime = parseTimestamp(parsed.serverTime);
    const resendAvailableAt = parseTimestamp(parsed.resendAvailableAt);
    const clientReceivedAt = parseTimestamp(parsed.clientReceivedAt);

    if (!serverTime || !resendAvailableAt || !clientReceivedAt) {
      return null;
    }

    return {
      serverTime,
      resendAvailableAt,
      clientReceivedAt,
    };
  } catch {
    return null;
  }
}

function writeResendCooldownSnapshot(key: string, snapshot: ResendCooldownSnapshot): void {
  if (typeof window === 'undefined') {
    return;
  }

  window.localStorage.setItem(key, JSON.stringify(snapshot));
}

function createCooldownSnapshot(args?: {
  serverTime?: number;
  resendAvailableAt?: number;
  resendAfterSeconds?: number;
}): ResendCooldownSnapshot {
  const now = Date.now();
  const serverTime = parseTimestamp(args?.serverTime) ?? now;
  const resendAvailableAt =
    parseTimestamp(args?.resendAvailableAt) ??
    serverTime + (parseTimestamp(args?.resendAfterSeconds) ?? RESEND_COOLDOWN_SECONDS) * 1000;

  return {
    serverTime,
    resendAvailableAt,
    clientReceivedAt: now,
  };
}

function createCooldownSnapshotFromResponse(
  response?: RegisterInitResponse | ForgotPasswordInitResponse
): ResendCooldownSnapshot {
  return createCooldownSnapshot({
    serverTime: response?.server_time,
    resendAvailableAt: response?.resend_available_at,
    resendAfterSeconds: response?.resend_after_seconds,
  });
}

function secondsUntil(snapshot: ResendCooldownSnapshot): number {
  const elapsedOnClient = Date.now() - snapshot.clientReceivedAt;
  const remainingMs = snapshot.resendAvailableAt - snapshot.serverTime - elapsedOnClient;
  return Math.max(0, Math.ceil(remainingMs / 1000));
}

interface VerificationFormProps {
  className?: string;
}

export function VerificationForm({ className }: VerificationFormProps) {
  const t = useT('auth');
  const router = useRouter();
  const searchParams = useSearchParams();

  // URL parameters
  const email = searchParams.get('email');
  const token = searchParams.get('token');
  const type = searchParams.get('type') || 'reset'; // 'reset' or 'register'

  // Component state
  const [isResending, setIsResending] = useState(false);
  const [canResend, setCanResend] = useState(false);
  const [countdown, setCountdown] = useState(RESEND_COOLDOWN_SECONDS);
  const [resendCooldown, setResendCooldown] = useState<ResendCooldownSnapshot | null>(null);
  const [verificationCode, setVerificationCode] = useState('');
  const [isSubmitting, setIsSubmitting] = useState(false);

  // Auth state
  const verifyRegisterMutation = useVerifyRegister();
  const verifyForgotPasswordMutation = useVerifyForgotPassword();
  const forgotPasswordMutation = useForgotPassword();
  const startRegisterMutation = useStartRegister();
  const isLoading = verifyRegisterMutation.isPending || verifyForgotPasswordMutation.isPending;
  const { locale } = useLocale();
  const resendCooldownKey = useMemo(
    () => (email && token ? buildResendCooldownKey(email, token, type) : null),
    [email, token, type]
  );

  useEffect(() => {
    if (!email || !token) {
      let redirectPath = type === 'reset' ? '/forgot-password' : '/register';
      const redirect = searchParams.get('redirect');
      if (redirect && type !== 'reset') {
        redirectPath += `?redirect=${encodeURIComponent(redirect)}`;
      }
      router.push(redirectPath);
      return;
    }
  }, [email, router, searchParams, token, type]);

  useEffect(() => {
    if (!resendCooldownKey) {
      return;
    }

    let nextCooldown = readResendCooldownSnapshot(resendCooldownKey);
    if (!nextCooldown) {
      nextCooldown = createCooldownSnapshot();
      writeResendCooldownSnapshot(resendCooldownKey, nextCooldown);
    }

    setResendCooldown(nextCooldown);
  }, [resendCooldownKey]);

  useEffect(() => {
    if (!resendCooldown) {
      return;
    }

    const syncCountdown = () => {
      const remainingSeconds = secondsUntil(resendCooldown);
      setCountdown(remainingSeconds);
      setCanResend(remainingSeconds === 0);
    };

    syncCountdown();
    const timer = window.setInterval(syncCountdown, 1000);

    return () => window.clearInterval(timer);
  }, [resendCooldown]);

  // Form setup
  const {
    setValue,
    handleSubmit,
    formState: { errors },
  } = useForm<VerificationFormData>({
    resolver: zodResolver(verificationSchema),
    defaultValues: {
      code: '',
    },
  });

  // Handle verification code change
  const handleCodeChange = (code: string) => {
    setVerificationCode(code);
    setValue('code', code);
  };

  // Handle code completion
  const handleCodeComplete = async (code: string) => {
    if (code.length === 6 && !isSubmitting) {
      await submitCode(code);
    }
  };

  // Form submission
  const submitCode = async (code: string) => {
    if (!email || !token || isSubmitting) {
      return;
    }

    setIsSubmitting(true);
    try {
      let result = false;

      if (type === 'register') {
        const res = await verifyRegisterMutation.mutateAsync({ email, code, token });
        result = res?.is_valid ?? false;
      } else {
        const res = await verifyForgotPasswordMutation.mutateAsync({ email, code, token });
        result = res?.data?.is_valid ?? false;
      }

      if (result) {
        if (type === 'reset') {
          router.push(`/reset-password?token=${token}&email=${encodeURIComponent(email)}`);
        } else {
          let completeUrl = `/register/complete?token=${token}&email=${encodeURIComponent(email)}`;
          const redirect = searchParams.get('redirect');
          if (redirect) {
            completeUrl += `&redirect=${encodeURIComponent(redirect)}`;
          }
          router.push(completeUrl);
        }
      }
    } catch (err) {
      // Error is handled by the store
      console.error('Verification failed:', err);
    } finally {
      setIsSubmitting(false);
    }
  };

  const onSubmit = async (data: VerificationFormData) => {
    await submitCode(data.code);
  };

  const handleResend = async () => {
    if (!email || !token) {
      return;
    }

    setIsResending(true);
    try {
      let response: RegisterInitResponse | ForgotPasswordInitResponse | undefined;
      if (type === 'reset') {
        response = await forgotPasswordMutation.mutateAsync({ email, language: locale });
      } else {
        response = await startRegisterMutation.mutateAsync({ email, language: locale });
      }
      const nextCooldown = createCooldownSnapshotFromResponse(response);
      const cooldownKey = buildResendCooldownKey(email, token, type);
      writeResendCooldownSnapshot(cooldownKey, nextCooldown);
      setResendCooldown(nextCooldown);
      setCountdown(RESEND_COOLDOWN_SECONDS);
      setCanResend(false);
    } catch (err) {
      console.error('Resend failed:', err);
    } finally {
      setIsResending(false);
    }
  };

  const formLoading = isLoading || isSubmitting;
  const title = type === 'reset' ? t('resetPasswordTitle') : t('completeRegistrationTitle');

  return (
    <div className={cn('flex flex-col gap-6', className)}>
      <Card>
        <CardHeader className="text-center">
          <CardTitle className="text-2xl font-bold">{title}</CardTitle>
          <p className="text-muted-foreground">
            {t('verificationSentTo')} <span className="font-medium">{email || t('yourEmail')}</span>
          </p>
        </CardHeader>

        <CardContent className="space-y-6">
          {/* Errors are shown via toast; field-level errors remain */}

          {/* Verification Form */}
          <form onSubmit={handleSubmit(onSubmit)} className="space-y-6">
            {/* Code Field */}
            <div className="space-y-4">
              <Label htmlFor="code" className="text-center block">
                {t('verificationCode')}
              </Label>
              <VerificationCodeInput
                onChange={handleCodeChange}
                onComplete={handleCodeComplete}
                disabled={formLoading}
                errorText={errors.code?.message}
                autoFocus
              />
            </div>

            {/* Submit Button */}
            <Button
              type="submit"
              className="w-full"
              disabled={formLoading || verificationCode.length !== 6}
            >
              {formLoading && <Icons.Spinner className="mr-2 h-4 w-4 animate-spin" />}
              {t('verifyCodeButtonText')}
            </Button>
          </form>

          {/* Resend Code */}
          <div className="text-center text-sm">
            {t('didntReceiveCode')}{' '}
            {canResend ? (
              <Button
                variant="link"
                className="p-0 font-medium text-primary hover:underline h-auto"
                onClick={handleResend}
                disabled={isResending}
              >
                {isResending ? (
                  <>
                    <Icons.Spinner className="mr-1 h-3 w-3 animate-spin" />
                    {t('resending')}
                  </>
                ) : (
                  t('resendCode')
                )}
              </Button>
            ) : (
              <span className="text-muted-foreground">{t('resendCodeIn', { countdown })}</span>
            )}
          </div>

          {/* Back Link */}
          <div className="text-center text-sm">
            <Link
              href={type === 'reset' ? '/forgot-password' : '/register'}
              className="font-medium text-primary hover:underline"
            >
              {type === 'reset' ? t('backToReset') : t('backToRegistration')}
            </Link>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
