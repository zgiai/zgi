'use client';

import { useState } from 'react';
import { AlertTriangle, ChevronLeft, ChevronRight, RefreshCw } from 'lucide-react';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import {
  integrationConnectionHealthEventItems,
  useIntegrationConnectionHealthEvents,
} from '@/hooks';
import { useT } from '@/i18n';
import type { IntegrationCatalogItem, IntegrationConnection } from '@/services/types/integration';
import { IntegrationConnectionHealthBadge } from './health-badge';
import { safeOptionalIntegrationDisplayText } from './display-utils';
import { useIntegrationMetadata } from './metadata-i18n';
import { ProviderDiagnosticsDetails } from './provider-diagnostics-details';

const PAGE_SIZE = 10;

function HealthValue({ value }: { value: string }) {
  return <Badge variant="outline">{value}</Badge>;
}

export function IntegrationConnectionHealthPanel({
  connection,
  provider,
  enabled = true,
  showHistory = true,
}: {
  connection: IntegrationConnection;
  provider?: IntegrationCatalogItem;
  enabled?: boolean;
  showHistory?: boolean;
}) {
  const t = useT('integrations');
  const metadata = useIntegrationMetadata();
  const [page, setPage] = useState(1);
  const eventsQuery = useIntegrationConnectionHealthEvents(
    connection.id,
    { page, limit: PAGE_SIZE },
    enabled && showHistory
  );
  const response = eventsQuery.data?.data;
  const events = integrationConnectionHealthEventItems(response);
  const total = response?.total ?? events.length;
  const hasMore = response?.has_more ?? false;

  return (
    <section className="space-y-3">
      <div>
        <h3 className="text-sm font-semibold">{t('connectionHealth.title')}</h3>
        <p className="mt-1 text-xs text-muted-foreground">{t('connectionHealth.description')}</p>
      </div>

      {connection.attention_code ? (
        <Alert className="border-warning/40 bg-warning/5">
          <AlertTriangle className="size-4" />
          <AlertDescription>
            <span className="font-medium">
              {t(`connectionHealth.attention.${connection.attention_code}`)}
            </span>
            {connection.missing_required_scopes?.length ? (
              <span className="mt-1 block text-xs">
                {t('connectionHealth.missingScopes', {
                  scopes: connection.missing_required_scopes
                    .map(scope => safeOptionalIntegrationDisplayText(scope))
                    .filter((scope): scope is string => Boolean(scope))
                    .map(scope => metadata.scope(scope, provider))
                    .join(', '),
                })}
              </span>
            ) : null}
          </AlertDescription>
        </Alert>
      ) : null}

      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <div className="rounded-lg border bg-muted/20 p-3">
          <p className="text-xs text-muted-foreground">{t('connectionHealth.overall')}</p>
          <div className="mt-2">
            <IntegrationConnectionHealthBadge connection={connection} />
          </div>
        </div>
        <div className="rounded-lg border bg-muted/20 p-3">
          <p className="text-xs text-muted-foreground">{t('connectionHealth.authentication')}</p>
          <div className="mt-2">
            <HealthValue value={t(`connectionHealth.authStatus.${connection.auth_status}`)} />
          </div>
        </div>
        <div className="rounded-lg border bg-muted/20 p-3">
          <p className="text-xs text-muted-foreground">{t('connectionHealth.scope')}</p>
          <div className="mt-2">
            <HealthValue value={t(`connectionHealth.scopeStatus.${connection.scope_status}`)} />
          </div>
        </div>
        <div className="rounded-lg border bg-muted/20 p-3">
          <p className="text-xs text-muted-foreground">{t('connectionHealth.failures')}</p>
          <p className="mt-2 text-sm font-semibold">
            {metadata.number(connection.consecutive_failures ?? 0)}
          </p>
        </div>
      </div>

      <dl className="grid gap-x-5 rounded-lg border px-3 sm:grid-cols-2">
        <HealthDetailRow label={t('connectionHealth.lastChecked')}>
          {metadata.date(connection.last_health_checked_at, t('executions.noValue'))}
        </HealthDetailRow>
        <HealthDetailRow label={t('connectionHealth.lastHealthy')}>
          {metadata.date(connection.last_healthy_at, t('executions.noValue'))}
        </HealthDetailRow>
        <HealthDetailRow label={t('connectionHealth.lastRuntimeSuccess')}>
          {metadata.date(connection.last_runtime_success_at, t('executions.noValue'))}
        </HealthDetailRow>
        <HealthDetailRow label={t('connectionHealth.lastRuntimeFailure')}>
          {metadata.date(connection.last_runtime_failure_at, t('executions.noValue'))}
        </HealthDetailRow>
        <HealthDetailRow label={t('connectionHealth.scopeChecked')}>
          {metadata.date(connection.scope_checked_at, t('executions.noValue'))}
        </HealthDetailRow>
      </dl>

      {showHistory ? (
        <div className="rounded-lg border">
          <div className="flex flex-wrap items-center justify-between gap-2 border-b px-3 py-2.5">
            <div>
              <h4 className="text-sm font-medium">{t('connectionHealth.history')}</h4>
              <p className="text-xs text-muted-foreground">
                {t('connectionHealth.historyCount', { count: total })}
              </p>
            </div>
            <Button
              isIcon
              size="sm"
              variant="ghost"
              aria-label={t('connectionHealth.refresh')}
              title={t('connectionHealth.refresh')}
              disabled={eventsQuery.isFetching}
              onClick={() => void eventsQuery.refetch()}
            >
              <RefreshCw className="size-4" />
            </Button>
          </div>

          {eventsQuery.isLoading ? (
            <div className="space-y-2 p-3">
              <Skeleton className="h-20 rounded-md" />
              <Skeleton className="h-20 rounded-md" />
            </div>
          ) : eventsQuery.isError ? (
            <p className="p-5 text-center text-sm text-destructive">
              {t('connectionHealth.loadFailed')}
            </p>
          ) : events.length === 0 ? (
            <p className="p-5 text-center text-sm text-muted-foreground">
              {t('connectionHealth.empty')}
            </p>
          ) : (
            <div className="divide-y">
              {events.map(event => (
                <div key={event.id} className="space-y-2 p-3">
                  <div className="flex flex-wrap items-start justify-between gap-2">
                    <div className="flex flex-wrap items-center gap-1.5">
                      <Badge
                        variant={
                          event.classification === 'success'
                            ? 'success'
                            : event.classification === 'ignored'
                              ? 'outline'
                              : 'warning'
                        }
                      >
                        {t(`connectionHealth.classification.${event.classification}`)}
                      </Badge>
                      <Badge variant="outline">
                        {t(`connectionHealth.source.${event.source}`)}
                      </Badge>
                      <Badge variant="subtle">
                        {t(`connectionHealth.checkKind.${event.check_kind}`)}
                      </Badge>
                      {!event.applied ? (
                        <Badge variant="outline">{t('connectionHealth.notApplied')}</Badge>
                      ) : null}
                    </div>
                    <span className="text-xs text-muted-foreground">
                      {metadata.date(event.observed_at, t('executions.noValue'))}
                    </span>
                  </div>
                  <div className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
                    <span>
                      {t('connectionHealth.resultingHealth')}:{' '}
                      {t(`connectionHealth.healthStatus.${event.health_status_after}`)}
                    </span>
                    <span>{t('connectionHealth.latency', { value: event.latency_ms })}</span>
                    {safeOptionalIntegrationDisplayText(event.reason_code) ? (
                      <span>{metadata.healthReason(event.reason_code)}</span>
                    ) : null}
                  </div>
                  <ProviderDiagnosticsDetails
                    className="rounded-md bg-muted/30 px-2.5 py-2"
                    providerErrorCode={event.provider_error_code}
                    providerRequestId={event.provider_request_id}
                    providerHTTPStatus={event.provider_http_status}
                    retryAfterAt={event.retry_after_at}
                  />
                  {event.missing_scopes?.length ? (
                    <p className="text-xs text-warning">
                      {t('connectionHealth.missingScopes', {
                        scopes: event.missing_scopes
                          .map(scope => safeOptionalIntegrationDisplayText(scope))
                          .filter((scope): scope is string => Boolean(scope))
                          .map(scope => metadata.scope(scope, provider))
                          .join(', '),
                      })}
                    </p>
                  ) : null}
                </div>
              ))}
            </div>
          )}

          {page > 1 || hasMore ? (
            <div className="flex items-center justify-between border-t px-3 py-2">
              <Button
                size="sm"
                variant="ghost"
                disabled={page <= 1}
                onClick={() => setPage(current => Math.max(1, current - 1))}
              >
                <ChevronLeft className="size-4" />
                {t('connectionHealth.previous')}
              </Button>
              <span className="text-xs text-muted-foreground">
                {t('connectionHealth.page', { page })}
              </span>
              <Button
                size="sm"
                variant="ghost"
                disabled={!hasMore}
                onClick={() => setPage(current => current + 1)}
              >
                {t('connectionHealth.next')}
                <ChevronRight className="size-4" />
              </Button>
            </div>
          ) : null}
        </div>
      ) : null}
    </section>
  );
}

function HealthDetailRow({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex items-start justify-between gap-3 border-b py-2.5 text-sm last:border-b-0">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="text-right">{children}</dd>
    </div>
  );
}
