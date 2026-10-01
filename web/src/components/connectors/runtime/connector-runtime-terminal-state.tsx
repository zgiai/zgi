'use client';

import { CircleX, TriangleAlert } from 'lucide-react';
import { OperationResultCard } from '@/components/aichat/operation-cards';
import { useT } from '@/i18n/translations';
import type { ConnectorRuntimeOperationPresentation } from '@/services/types/connector';

interface ConnectorRuntimeTerminalStateProps {
  state: 'cancelled' | 'completed' | 'failed' | 'unknown';
  compact?: boolean;
  className?: string;
  presentation?: ConnectorRuntimeOperationPresentation;
}

function presentationDescription(
  presentation: ConnectorRuntimeOperationPresentation,
  t: ReturnType<typeof useT<'connectors'>>
) {
  if (presentation.summary) return presentation.summary;
  switch (presentation.error_code) {
    case 'connector_mail_submission_rejected':
      return t('runtime.presentation.errors.mailSubmissionRejected');
    case 'connector_mail_recipient_rejected':
      return t('runtime.presentation.errors.mailRecipientRejected');
    case 'connector_mail_authentication_rejected':
      return t('runtime.presentation.errors.mailAuthenticationRejected');
    case 'connector_operation_outcome_unknown':
      return t('runtime.presentation.errors.unknown');
    default:
      return t(`runtime.presentation.status.${presentation.display_status}`);
  }
}

/** Durable terminal projection restored after a one-shot confirmation. */
export function ConnectorRuntimeTerminalState({
  state,
  compact = false,
  className,
  presentation,
}: ConnectorRuntimeTerminalStateProps) {
  const t = useT('connectors');
  if (presentation) {
    const status =
      presentation.display_status === 'successful'
        ? 'success'
        : presentation.display_status === 'failed' || presentation.display_status === 'unavailable'
          ? 'error'
          : presentation.display_status === 'partial'
            ? 'warning'
            : 'info';
    return (
      <OperationResultCard
        className={className}
        compact={compact}
        eyebrow={t('runtime.presentation.title')}
        title={presentation.title ?? t('runtime.presentation.title')}
        description={presentationDescription(presentation, t)}
        status={status}
        statusLabel={t(`runtime.presentation.status.${presentation.display_status}`)}
        details={[
          ...(presentation.redacted
            ? [{ id: 'redacted', description: t('runtime.presentation.redacted'), tone: 'warning' as const }]
            : []),
          ...(presentation.truncated
            ? [{ id: 'truncated', description: t('runtime.presentation.truncated'), tone: 'warning' as const }]
            : []),
        ]}
      />
    );
  }
  const unknown = state === 'unknown';
  const completed = state === 'completed';
  const cancelled = state === 'cancelled';
  const title = completed
    ? t('runtime.confirmation.completedTitle')
    : cancelled
      ? t('runtime.confirmation.cancelledTitle')
      : unknown
        ? t('runtime.confirmation.unknownTitle')
        : t('runtime.confirmation.status.failed');
  const description = completed
    ? t('runtime.confirmation.completedDescription')
    : cancelled
      ? t('runtime.confirmation.cancelledDescription')
      : unknown
        ? t('runtime.confirmation.unknownDescription')
        : t('runtime.confirmation.operationFailed');
  const statusLabel = completed
    ? t('runtime.confirmation.status.completed')
    : cancelled
      ? t('runtime.confirmation.status.cancelled')
      : unknown
        ? t('runtime.confirmation.status.unknown')
        : t('runtime.confirmation.status.failed');

  return (
    <OperationResultCard
      className={className}
      compact={compact}
      eyebrow={t('runtime.presentation.title')}
      title={title}
      description={description}
      status={completed ? 'success' : cancelled ? 'info' : unknown ? 'warning' : 'error'}
      statusLabel={statusLabel}
      details={
        completed || cancelled
          ? []
          : [
              {
                id: state,
                icon: unknown ? <TriangleAlert className="size-3.5" /> : <CircleX className="size-3.5" />,
                description,
                tone: unknown ? ('warning' as const) : ('destructive' as const),
              },
            ]
      }
    />
  );
}
