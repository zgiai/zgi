'use client';

import { useT } from '@/i18n/translations';
import { cn } from '@/lib/utils';
import {
  connectorOperationTone,
  parseConnectorOperationPresentation,
} from './connector-operation-presentation';

/** Read-only operation evidence. Confirmation controls live on their own
 * server-issued continuation surface and are never restored from this object. */
export function ConnectorOperationSummary({
  invocation,
  className,
  showHeading = true,
}: {
  invocation: unknown;
  className?: string;
  showHeading?: boolean;
}) {
  const t = useT('connectors');
  const presentation = parseConnectorOperationPresentation(invocation);
  const status = presentation?.execution_status ?? presentation?.display_status ?? 'unavailable';
  const tone = connectorOperationTone(invocation);
  if (
    !showHeading &&
    presentation &&
    !presentation.summary &&
    presentation.source_kind !== 'history' &&
    !presentation.truncated &&
    !presentation.redacted
  ) {
    return null;
  }
  return (
    <div
      role="status"
      className={cn(
        'space-y-1 rounded-md p-2 text-xs',
        tone === 'success'
          ? 'bg-emerald-500/5'
          : tone === 'error'
            ? 'bg-destructive/10'
            : 'bg-amber-500/10',
        className
      )}
    >
      {showHeading && (
        <p className="font-medium">
          {presentation?.title ?? t('runtime.presentation.title')} ·{' '}
          {t(`runtime.presentation.status.${status}`)}
        </p>
      )}
      {presentation?.summary && (
        <p className="whitespace-pre-wrap break-words">{presentation.summary}</p>
      )}
      {!presentation && <p>{t('runtime.presentation.unavailable')}</p>}
      {presentation?.source_kind === 'history' && (
        <p className="text-muted-foreground">{t('runtime.presentation.history')}</p>
      )}
      {presentation?.truncated && (
        <p className="text-muted-foreground">{t('runtime.presentation.truncated')}</p>
      )}
      {presentation?.redacted && (
        <p className="text-muted-foreground">{t('runtime.presentation.redacted')}</p>
      )}
    </div>
  );
}
