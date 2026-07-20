'use client';

import { useEffect, useMemo, useState } from 'react';
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
import { Loader2 } from 'lucide-react';
import { useForgotPassword } from '@/hooks/auth/use-forgot-password';
import { useStartRegister } from '@/hooks/auth/use-start-register';
import { useSystemFeatures } from '@/hooks/auth/use-system-features';
import { useVerifyForgotPassword } from '@/hooks/auth/use-verify-forgot-password';
import { useVerifyRegister } from '@/hooks/auth/use-verify-register';
import { usePhoneCode, usePhoneVerify } from '@/hooks/auth/use-phone-auth';
import { VerificationCodeInput } from '@/components/ui/verification-code-input';
import type { ForgotPasswordInitResponse, RegisterInitResponse } from '@/services/types/auth';
import {
  isPhonePasswordResetEnabled,
  isPhoneRegisterEnabled,
} from '@/lib/features/notification-sms';

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
const DEFAULT_PHONE_COUNTRY_CODE = 'CN';

interface ResendCooldownSnapshot {
  serverTime: number;
  resendAvailableAt: number;
  clientReceivedAt: number;
}

function buildResendCooldownKey(identifier: string, token: string, type: string): string {
  return `auth:verify:resend_cooldown:${type}:${identifier.toLowerCase()}:${token}`;
}

function normalizeCountryCode(value: string): string {
  const normalized = value.trim().toUpperCase();

  if (normalized === '+86' || normalized === '86') {
    return 'CN';
  }

  return normalized.startsWith('+') ? normalized.slice(1) : normalized;
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

  const email = searchParams.get('email');
  const phone = searchParams.get('phone');
  const token = searchParams.get('token');
  const method = searchParams.get('method');
  const type = searchParams.get('type') || 'reset';
  const countryCode = searchParams.get('country_code') || DEFAULT_PHONE_COUNTRY_CODE;
  const isPhoneRegisterFlow = method === 'phone' && type === 'register';
  const isPhoneResetFlow = method === 'phone' && type === 'reset';
  const isPhoneFlow = isPhoneRegisterFlow || isPhoneResetFlow;
  const destination = isPhoneFlow ? phone : email;

  const [isResending, setIsResending] = useState(false);
  const [canResend, setCanResend] = useState(false);
  const [countdown, setCountdown] = useState(RESEND_COOLDOWN_SECONDS);
  const [resendCooldown, setResendCooldown] = useState<ResendCooldownSnapshot | null>(null);
  const [verificationCode, setVerificationCode] = useState('');
  const [isSubmitting, setIsSubmitting] = useState(false);

  const verifyRegisterMutation = useVerifyRegister();
  const verifyForgotPasswordMutation = useVerifyForgotPassword();
  const forgotPasswordMutation = useForgotPassword();
  const startRegisterMutation = useStartRegister();
  const phoneCodeMutation = usePhoneCode();
  const phoneVerifyMutation = usePhoneVerify();
  const { data: systemFeatures } = useSystemFeatures();
  const systemFeaturesLoaded = systemFeatures !== undefined;
  const phoneRegisterEnabled = isPhoneRegisterEnabled(systemFeatures);
  const phoneResetEnabled = isPhonePasswordResetEnabled(systemFeatures);
  const phoneFlowUnavailable =
    isPhoneFlow &&
    systemFeaturesLoaded &&
    ((isPhoneRegisterFlow && !phoneRegisterEnabled) || (isPhoneResetFlow && !phoneResetEnabled));
  const isLoading =
    verifyRegisterMutation.isPending ||
    verifyForgotPasswordMutation.isPending ||
    phoneVerifyMutation.isPending;
  const { locale } = useLocale();
  const resendCooldownKey = useMemo(() => {
    if (!destination || !token) {
      return null;
    }

    const cooldownType = isPhoneFlow ? `phone-${type}` : type;
    return buildResendCooldownKey(destination, token, cooldownType);
  }, [destination, isPhoneFlow, token, type]);

  useEffect(() => {
    const missingEmailParams = !isPhoneFlow && (!email || !token);
    const missingPhoneParams = isPhoneFlow && (!phone || !token);

    if (missingEmailParams || missingPhoneParams || phoneFlowUnavailable) {
      let redirectPath = type === 'reset' ? '/forgot-password' : '/register';
      const redirect = searchParams.get('redirect');
      if (redirect && type !== 'reset') {
        redirectPath += `?redirect=${encodeURIComponent(redirect)}`;
      }
      router.push(redirectPath);
    }
  }, [email, isPhoneFlow, phone, phoneFlowUnavailable, router, searchParams, token, type]);

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

  const handleCodeChange = (code: string) => {
    setVerificationCode(code);
    setValue('code', code);
  };

  const handleCodeComplete = async (code: string) => {
    if (code.length === 6 && !isSubmitting) {
      await submitCode(code);
    }
  };

  const submitCode = async (code: string) => {
    if (!token || isSubmitting) {
      return;
    }

    if (isPhoneFlow && (!phone || phoneFlowUnavailable)) {
      return;
    }

    if (!isPhoneFlow && !email) {
      return;
    }

    setIsSubmitting(true);
    try {
      if (isPhoneFlow) {
        const normalizedCountryCode = normalizeCountryCode(countryCode);
        const res = await phoneVerifyMutation.mutateAsync({
          phone: phone || '',
          country_code: normalizedCountryCode,
          scene: isPhoneRegisterFlow ? 'register' : 'reset_password',
          token,
          code,
        });

        if (res.verified_token) {
          if (isPhoneRegisterFlow) {
            let completeUrl =
              `/register/complete?method=phone&phone=${encodeURIComponent(phone || '')}` +
              `&country_code=${encodeURIComponent(normalizedCountryCode)}` +
              `&verified_token=${encodeURIComponent(res.verified_token)}`;
            const redirect = searchParams.get('redirect');
            if (redirect) {
              completeUrl += `&redirect=${encodeURIComponent(redirect)}`;
            }
            router.push(completeUrl);
          } else {
            router.push(
              `/reset-password?method=phone&phone=${encodeURIComponent(phone || '')}` +
                `&country_code=${encodeURIComponent(normalizedCountryCode)}` +
                `&verified_token=${encodeURIComponent(res.verified_token)}`
            );
          }
        }
        return;
      }

      let result = false;

      if (type === 'register') {
        const res = await verifyRegisterMutation.mutateAsync({ email: email || '', code, token });
        result = res?.is_valid ?? false;
      } else {
        const res = await verifyForgotPasswordMutation.mutateAsync({
          email: email || '',
          code,
          token,
        });
        result = res?.data?.is_valid ?? false;
      }

      if (result) {
        if (type === 'reset') {
          router.push(`/reset-password?token=${token}&email=${encodeURIComponent(email || '')}`);
        } else {
          let completeUrl = `/register/complete?token=${token}&email=${encodeURIComponent(email || '')}`;
          const redirect = searchParams.get('redirect');
          if (redirect) {
            completeUrl += `&redirect=${encodeURIComponent(redirect)}`;
          }
          router.push(completeUrl);
        }
      }
    } catch (err) {
      console.error('Verification failed:', err);
    } finally {
      setIsSubmitting(false);
    }
  };

  const onSubmit = async (data: VerificationFormData) => {
    await submitCode(data.code);
  };

  const handleResend = async () => {
    if (!resendCooldownKey) {
      return;
    }

    setIsResending(true);
    try {
      if (isPhoneFlow) {
        if (!phone || phoneFlowUnavailable) {
          return;
        }

        const normalizedCountryCode = normalizeCountryCode(countryCode);
        const response = await phoneCodeMutation.mutateAsync({
          phone,
          country_code: normalizedCountryCode,
          scene: isPhoneRegisterFlow ? 'register' : 'reset_password',
        });
        const nextCooldown = createCooldownSnapshot();
        const nextCooldownKey = buildResendCooldownKey(phone, response.token, `phone-${type}`);
        writeResendCooldownSnapshot(nextCooldownKey, nextCooldown);
        setResendCooldown(nextCooldown);

        const params = new URLSearchParams(searchParams.toString());
        params.set('country_code', normalizedCountryCode);
        params.set('token', response.token);
        router.replace(`/verify?${params.toString()}`);
      } else {
        if (!email || !token) {
          return;
        }

        let response: RegisterInitResponse | ForgotPasswordInitResponse | undefined;
        if (type === 'reset') {
          response = await forgotPasswordMutation.mutateAsync({ email, language: locale });
        } else {
          response = await startRegisterMutation.mutateAsync({ email, language: locale });
        }
        const nextCooldown = createCooldownSnapshotFromResponse(response);
        writeResendCooldownSnapshot(resendCooldownKey, nextCooldown);
        setResendCooldown(nextCooldown);
      }

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
  const destinationLabel = destination || (isPhoneFlow ? t('phone') : t('yourEmail'));

  if (phoneFlowUnavailable) {
    return null;
  }

  return (
    <div className={cn('flex flex-col gap-6', className)}>
      <Card>
        <CardHeader className="text-center">
          <CardTitle className="text-2xl font-bold">{title}</CardTitle>
          <p className="text-muted-foreground">
            {t('verificationSentTo')} <span className="font-medium">{destinationLabel}</span>
          </p>
        </CardHeader>

        <CardContent className="space-y-6">
          <form onSubmit={handleSubmit(onSubmit)} className="space-y-6">
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

            <Button
              type="submit"
              className="w-full"
              disabled={formLoading || verificationCode.length !== 6}
            >
              {formLoading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              {t('verifyCodeButtonText')}
            </Button>
          </form>

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
                    <Loader2 className="mr-1 h-3 w-3 animate-spin" />
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
