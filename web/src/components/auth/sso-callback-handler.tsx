'use client';

import { useEffect, useMemo, useRef, useState } from 'react';
import { useSearchParams } from 'next/navigation';
import { useT } from '@/i18n';
import { useConsumeCasdoorTicket } from '@/hooks';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardTitle } from '@/components/ui/card';
import { Icons } from '@/components/ui/icons';
import { withBasePath } from '@/lib/config';
import { buildSsoStartUrl } from '@/utils/auth-sso';

type CallbackStatus = 'processing' | 'success' | 'error';

export function SSOCallbackHandler() {
  const t = useT('auth');
  const searchParams = useSearchParams();
  const consumeTicketMutation = useConsumeCasdoorTicket();
  const { mutateAsync } = consumeTicketMutation;
  const consumedRef = useRef(false);
  const [status, setStatus] = useState<CallbackStatus>('processing');
  const [message, setMessage] = useState<string>(t('ssoProcessing'));

  const ticket = (searchParams.get('ticket') || '').trim();
  const error = (searchParams.get('error') || '').trim();
  const consoleUrl = withBasePath('/console');
  const retryRedirectTarget = searchParams.get('redirect') || consoleUrl;
  const retryUrl = buildSsoStartUrl('casdoor', retryRedirectTarget);
  const logPrefix = '[SSO Callback]';
  const ticketSuccessKey = ticket ? `zgi:sso-ticket-success:${ticket}` : '';

  const ssoErrorMessageMap = useMemo<Record<string, string>>(
    () => ({
      disabled: t('ssoErrorDisabled'),
      not_configured: t('ssoErrorNotConfigured'),
      missing_code_or_state: t('ssoErrorMissingCodeOrState'),
      invalid_state: t('ssoErrorInvalidState'),
      exchange_failed: t('ssoErrorExchangeFailed'),
      account_resolution_failed: t('ssoErrorAccountResolutionFailed'),
      ticket_issue_failed: t('ssoErrorTicketIssueFailed'),
    }),
    [t]
  );

  useEffect(() => {
    let cancelled = false;

    console.info(logPrefix, 'effect start', {
      href: window.location.href,
      ticketPresent: Boolean(ticket),
      error,
      consoleUrl,
      mutationStatus: consumeTicketMutation.status,
    });

    if (consumedRef.current) {
      console.info(logPrefix, 'skip consume because ticket already handled');
      return () => {
        cancelled = true;
      };
    }

    if (
      typeof window !== 'undefined' &&
      ticketSuccessKey &&
      window.sessionStorage.getItem(ticketSuccessKey) === 'success'
    ) {
      console.info(logPrefix, 'ticket already consumed successfully, redirect immediately', {
        ticketSuccessKey,
        redirectTarget: consoleUrl,
      });
      setStatus('success');
      setMessage(t('ssoRedirectingToConsole'));
      window.location.replace(consoleUrl);
      return () => {
        cancelled = true;
      };
    }

    if (error) {
      console.warn(logPrefix, 'callback contains error query param', { error });
      setStatus('error');
      setMessage(ssoErrorMessageMap[error] ?? t('ssoLoginFailed'));
      return () => {
        cancelled = true;
      };
    }

    if (!ticket) {
      console.warn(logPrefix, 'missing ticket in callback url');
      setStatus('error');
      setMessage(t('ssoTicketMissing'));
      return () => {
        cancelled = true;
      };
    }

    consumedRef.current = true;
    console.info(logPrefix, 'start consume-ticket mutation', {
      ticketPreview: `${ticket.slice(0, 8)}...`,
    });

    mutateAsync(ticket)
      .then(() => {
        if (cancelled) {
          console.warn(logPrefix, 'mutation resolved after effect cleanup, skip redirect');
          return;
        }

        if (typeof window !== 'undefined' && ticketSuccessKey) {
          window.sessionStorage.setItem(ticketSuccessKey, 'success');
          console.info(logPrefix, 'stored ticket success marker', { ticketSuccessKey });
        }

        console.info(logPrefix, 'mutation resolved, switching to success state');
        setStatus('success');
        setMessage(t('ssoRedirectingToConsole'));
        console.info(logPrefix, 'executing immediate window.location.replace', {
          from: window.location.href,
          to: consoleUrl,
        });
        window.location.replace(consoleUrl);
      })
      .catch(caughtError => {
        if (cancelled) {
          console.warn(logPrefix, 'mutation rejected after effect cleanup', caughtError);
          return;
        }
        const errorMessage =
          caughtError instanceof Error && caughtError.message
            ? caughtError.message
            : t('ssoLoginFailed');
        if (typeof window !== 'undefined' && ticketSuccessKey) {
          window.sessionStorage.removeItem(ticketSuccessKey);
          console.info(logPrefix, 'cleared ticket success marker after error', {
            ticketSuccessKey,
          });
        }
        console.error(logPrefix, 'mutation rejected', caughtError);
        setStatus('error');
        setMessage(errorMessage);
      });

    return () => {
      cancelled = true;
      console.info(logPrefix, 'effect cleanup');
    };
  }, [consoleUrl, error, mutateAsync, ssoErrorMessageMap, t, ticket, ticketSuccessKey]);

  const isProcessing = status === 'processing';
  const isSuccess = status === 'success';

  return (
    <Card className="glass-panel border-none shadow-premium overflow-hidden">
      <CardContent className="px-8 py-10 space-y-6">
        <div className="space-y-2 text-center">
          <CardTitle className="text-2xl font-bold tracking-tight">
            {t('ssoCallbackTitle')}
          </CardTitle>
          <p className="text-muted-foreground/80">{t('ssoCallbackDesc')}</p>
        </div>

        {isProcessing ? (
          <div className="space-y-6">
            <div className="flex justify-center">
              <div className="flex size-16 items-center justify-center rounded-full bg-primary/10 text-primary">
                <Icons.Loader className="size-7 animate-spin" />
              </div>
            </div>
            <div className="space-y-2 text-center">
              <p className="text-base font-medium">{message}</p>
              <p className="text-sm text-muted-foreground">{t('ssoProcessingHint')}</p>
            </div>
            <div className="space-y-3 rounded-xl border border-border/60 bg-muted/30 p-4">
              <div className="flex items-center gap-3 text-sm">
                <Icons.CheckCircle className="size-4 text-primary" />
                <span>{t('ssoProcessingStepAuthorize')}</span>
              </div>
              <div className="flex items-center gap-3 text-sm">
                <Icons.Loader className="size-4 animate-spin text-primary" />
                <span>{t('ssoProcessingStepExchange')}</span>
              </div>
              <div className="flex items-center gap-3 text-sm text-muted-foreground">
                <Icons.ArrowRight className="size-4" />
                <span>{t('ssoProcessingStepRedirect')}</span>
              </div>
            </div>
          </div>
        ) : isSuccess ? (
          <div className="space-y-5">
            <div className="flex items-start gap-3 rounded-lg border border-primary/20 bg-primary/5 p-4 text-primary">
              <Icons.CheckCircle className="mt-0.5 size-4 shrink-0" />
              <div className="space-y-1">
                <p className="text-sm font-medium leading-6">{t('ssoSuccess')}</p>
                <p className="text-sm leading-6">{message}</p>
              </div>
            </div>
            <Button
              type="button"
              className="w-full"
              size="xl"
              onClick={() => {
                console.info(logPrefix, 'manual go-to-console click', { to: consoleUrl });
                window.location.replace(consoleUrl);
              }}
            >
              {t('goToConsole')}
            </Button>
          </div>
        ) : (
          <div className="space-y-5">
            <div className="flex items-start gap-3 rounded-lg border border-destructive/20 bg-destructive/5 p-4 text-destructive">
              <Icons.AlertCircle className="mt-0.5 size-4 shrink-0" />
              <p className="text-sm leading-6">{message}</p>
            </div>
            <Button
              type="button"
              className="w-full"
              size="xl"
              onClick={() => {
                console.info(logPrefix, 'retry sso click', { to: retryUrl });
                window.location.href = retryUrl;
              }}
            >
              {t('retrySsoLogin')}
            </Button>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
