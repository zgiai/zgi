'use client';

import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import { containsOpaqueUUID, safeOptionalIntegrationDisplayText } from './display-utils';
import { useIntegrationMetadata } from './metadata-i18n';

export interface ProviderDiagnosticsDetailsProps {
  providerErrorCode?: string | null;
  providerRequestId?: string | null;
  providerHTTPStatus?: number | null;
  retryAfterAt?: string | null;
  showRequestId?: boolean;
  className?: string;
}

export function ProviderDiagnosticsDetails({
  providerErrorCode,
  providerRequestId,
  providerHTTPStatus,
  retryAfterAt,
  showRequestId = true,
  className,
}: ProviderDiagnosticsDetailsProps) {
  const t = useT('integrations');
  const metadata = useIntegrationMetadata();
  const safeErrorCode = safeOptionalIntegrationDisplayText(providerErrorCode);
  const safeRequestId = safeOptionalIntegrationDisplayText(providerRequestId);
  const requestIdHidden = containsOpaqueUUID(providerRequestId);
  const safeHTTPStatus =
    Number.isInteger(providerHTTPStatus) &&
    Number(providerHTTPStatus) >= 100 &&
    Number(providerHTTPStatus) <= 599
      ? Number(providerHTTPStatus)
      : null;
  const safeRetryAfter = retryAfterAt ? metadata.date(retryAfterAt, '') : '';

  const items = [
    safeErrorCode
      ? {
          label: t('providerDiagnostics.errorCode'),
          value: safeErrorCode,
          code: true,
        }
      : null,
    safeHTTPStatus
      ? {
          label: t('providerDiagnostics.httpStatus'),
          value: String(safeHTTPStatus),
          code: false,
        }
      : null,
    safeRetryAfter
      ? {
          label: t('providerDiagnostics.retryAfter'),
          value: safeRetryAfter,
          code: false,
        }
      : null,
    showRequestId && (safeRequestId || requestIdHidden)
      ? {
          label: t('providerDiagnostics.requestId'),
          value: safeRequestId ?? t('providerDiagnostics.hiddenRequestId'),
          code: Boolean(safeRequestId),
        }
      : null,
  ].filter((item): item is { label: string; value: string; code: boolean } => Boolean(item));

  if (items.length === 0) return null;

  return (
    <dl className={cn('grid gap-1 text-xs', className)}>
      {items.map(item => (
        <div key={item.label} className="grid min-w-0 grid-cols-[auto_minmax(0,1fr)] gap-1.5">
          <dt className="text-muted-foreground">{item.label}:</dt>
          <dd className="min-w-0 break-all">
            {item.code ? <code>{item.value}</code> : item.value}
          </dd>
        </div>
      ))}
    </dl>
  );
}
