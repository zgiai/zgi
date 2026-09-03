'use client';

import Link from 'next/link';
import { AlertCircle, CheckCircle2, Loader2 } from 'lucide-react';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Button } from '@/components/ui/button';
import { useT } from '@/i18n';
import type { KeySetupState } from './key-setup-state';

export function ConnectorStatusQueryError({
  onRetry,
  isRetrying,
}: {
  onRetry: () => void;
  isRetrying: boolean;
}) {
  const t = useT('connectors');
  return (
    <div className="space-y-3" role="status" aria-live="polite">
      <Alert>
        <AlertCircle aria-hidden="true" className="size-4" />
        <AlertTitle>{t('setup.statusReadFailedTitle')}</AlertTitle>
        <AlertDescription>{t('setup.statusReadFailedDescription')}</AlertDescription>
      </Alert>
      <Button variant="outline" className="w-full" disabled={isRetrying} onClick={onRetry}>
        {isRetrying ? <Loader2 aria-hidden="true" className="size-4 animate-spin" /> : null}
        {t('setup.retryStatus')}
      </Button>
    </div>
  );
}

export function ConnectorSetupStatus({
  state,
  showDefaultAction = true,
  statusError = false,
  statusRefreshing = false,
  onRetryStatus,
}: {
  state: KeySetupState;
  showDefaultAction?: boolean;
  statusError?: boolean;
  statusRefreshing?: boolean;
  onRetryStatus?: () => void;
}) {
  const t = useT('connectors');
  if (state.status === 'verifying') {
    if (statusError && state.attemptId && onRetryStatus) {
      return <ConnectorStatusQueryError onRetry={onRetryStatus} isRetrying={statusRefreshing} />;
    }
    return (
      <div
        className="rounded-lg border bg-muted/20 p-5 text-center"
        role="status"
        aria-live="polite"
      >
        <Loader2 aria-hidden="true" className="mx-auto size-6 animate-spin text-primary" />
        <h3 className="mt-3 text-sm font-semibold">{t('setup.verifyingTitle')}</h3>
        <p className="mt-1 text-sm leading-5 text-muted-foreground">
          {t('setup.verifyingDescription')}
        </p>
      </div>
    );
  }
  if (state.status !== 'ready') return null;
  return (
    <div className="space-y-4" role="status" aria-live="polite">
      <Alert className="border-success/40 bg-success/5">
        <CheckCircle2 aria-hidden="true" className="size-4 text-success" />
        <AlertTitle>{t('setup.readyTitle')}</AlertTitle>
        <AlertDescription>
          <p>{t('setup.readyDescription')}</p>
          {state.connection?.external_display_name ? (
            <p className="mt-1 font-medium text-foreground">
              {t('setup.readyAs', { name: state.connection.external_display_name })}
            </p>
          ) : null}
        </AlertDescription>
      </Alert>
      {showDefaultAction ? (
        <div className="space-y-2">
          {state.connection ? (
            <Button asChild className="w-full">
              <Link
                href={`/console/connectors/connections/${encodeURIComponent(state.connection.id)}?rename=1`}
              >
                {t('setup.nameConnection')}
              </Link>
            </Button>
          ) : null}
          <Button asChild variant="outline" className="w-full">
            <Link href="/console/connectors/connections">{t('manageConnections')}</Link>
          </Button>
        </div>
      ) : null}
    </div>
  );
}
