'use client';

import { useCallback, useEffect, useRef, useState } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import { AlertCircle, CheckCircle2, Loader2 } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { useT } from '@/i18n';
import { integrationService } from '@/services/integration.service';
import type { IntegrationOAuthPopupStatusMessage } from '@/hooks/integrations/use-oauth-flow';
import type { IntegrationOAuthFlowStatus } from '@/services/types/integration';
import { isIntegrationOAuthFlowTerminal } from './oauth-flow-state';
import { useIntegrationMetadata } from './metadata-i18n';

const DEFAULT_POLL_INTERVAL = 1_500;
const OPAQUE_FLOW_REFERENCE = /^[A-Za-z0-9_-]{24,256}$/;
const SAFE_ERROR_CODE = /^[a-z][a-z0-9_]{0,63}$/;

function notifyOpener(flowId: string, status: IntegrationOAuthFlowStatus) {
  const message: IntegrationOAuthPopupStatusMessage = {
    type: 'zgi:integration-oauth-status',
    status,
  };
  if (typeof BroadcastChannel !== 'undefined') {
    const channel = new BroadcastChannel(`zgi:integration-oauth:${flowId}`);
    channel.postMessage(message);
    channel.close();
  }
  if (window.opener && !window.opener.closed) {
    window.opener.postMessage(message, window.location.origin);
  }
}

export function IntegrationOAuthResultClient() {
  const t = useT('integrations');
  const metadata = useIntegrationMetadata();
  const router = useRouter();
  const searchParams = useSearchParams();
  const flowCandidate = searchParams.get('flow')?.trim() ?? '';
  const flowId = OPAQUE_FLOW_REFERENCE.test(flowCandidate) ? flowCandidate : '';
  const [status, setStatus] = useState<IntegrationOAuthFlowStatus | 'invalid' | 'unreachable'>(
    flowId ? 'pending' : 'invalid'
  );
  const initialErrorCode = searchParams.get('error_code')?.trim() ?? '';
  const [errorCode, setErrorCode] = useState(
    SAFE_ERROR_CODE.test(initialErrorCode) ? initialErrorCode : ''
  );
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const mountedRef = useRef(true);
  const pollingRef = useRef(false);

  const poll = useCallback(async () => {
    if (!flowId || pollingRef.current) return;
    pollingRef.current = true;
    if (timerRef.current) clearTimeout(timerRef.current);
    timerRef.current = null;
    try {
      const response = await integrationService.getOAuthFlow(flowId);
      if (!mountedRef.current) return;
      const nextStatus = response.data.status;
      setStatus(nextStatus);
      const nextErrorCode = response.data.error_code?.trim() ?? '';
      if (SAFE_ERROR_CODE.test(nextErrorCode)) setErrorCode(nextErrorCode);
      notifyOpener(flowId, nextStatus);
      if (isIntegrationOAuthFlowTerminal(nextStatus)) {
        if (window.opener && !window.opener.closed) {
          timerRef.current = setTimeout(() => window.close(), 1_200);
        }
        return;
      }
      timerRef.current = setTimeout(
        () => void poll(),
        response.data.next_poll_after_ms ?? DEFAULT_POLL_INTERVAL
      );
    } catch {
      if (!mountedRef.current) return;
      setStatus('unreachable');
      timerRef.current = setTimeout(() => void poll(), 2_500);
    } finally {
      pollingRef.current = false;
    }
  }, [flowId]);

  useEffect(() => {
    mountedRef.current = true;
    void poll();
    return () => {
      mountedRef.current = false;
      if (timerRef.current) clearTimeout(timerRef.current);
    };
  }, [poll]);

  const succeeded = status === 'succeeded';
  const waiting = ['pending', 'authorizing', 'exchanging', 'unreachable'].includes(status);

  return (
    <main className="flex min-h-dvh items-center justify-center bg-bg-canvas/60 p-5">
      <section className="w-full max-w-md rounded-2xl border bg-background p-6 text-center shadow-sm">
        <div role="status" aria-live="polite" aria-busy={waiting}>
          {waiting ? (
            <Loader2 aria-hidden="true" className="mx-auto size-9 animate-spin text-primary" />
          ) : null}
          {succeeded ? (
            <CheckCircle2 aria-hidden="true" className="mx-auto size-10 text-success" />
          ) : null}
          {!waiting && !succeeded ? (
            <AlertCircle aria-hidden="true" className="mx-auto size-10 text-destructive" />
          ) : null}

          <h1 className="mt-4 text-lg font-semibold">{t(`oauth.result.status.${status}.title`)}</h1>
          <p className="mt-2 text-sm leading-6 text-muted-foreground">
            {!waiting && !succeeded && errorCode
              ? metadata.error(errorCode)
              : t(`oauth.result.status.${status}.description`)}
          </p>
        </div>

        <div className="mt-6 flex flex-col gap-2 sm:flex-row sm:justify-center">
          {status === 'unreachable' ? (
            <Button
              variant="outline"
              onClick={() => {
                if (timerRef.current) clearTimeout(timerRef.current);
                timerRef.current = null;
                void poll();
              }}
            >
              {t('oauth.result.retry')}
            </Button>
          ) : null}
          <Button onClick={() => router.replace('/console/integrations?view=connected')}>
            {t('oauth.result.returnToConnections')}
          </Button>
        </div>

        <noscript>
          <p className="mt-4 text-sm text-muted-foreground">{t('oauth.result.noScript')}</p>
        </noscript>
      </section>
    </main>
  );
}
