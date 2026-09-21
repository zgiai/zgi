'use client';

import { CircleX, TriangleAlert } from 'lucide-react';
import { OperationConfirmationCard } from '@/components/aichat/operation-cards';
import { useT } from '@/i18n/translations';

interface ConnectorRuntimeTerminalStateProps {
  state: 'cancelled' | 'completed' | 'failed' | 'unknown';
  compact?: boolean;
  className?: string;
}

/** Durable terminal projection restored after a one-shot confirmation. */
export function ConnectorRuntimeTerminalState({
  state,
  compact = false,
  className,
}: ConnectorRuntimeTerminalStateProps) {
  const t = useT('connectors');
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
    <div role="status" aria-live="polite">
      <OperationConfirmationCard
        className={className}
        compact={compact}
        title={title}
        description={description}
        status={completed ? 'confirmed' : 'rejected'}
        statusLabel={statusLabel}
        warnings={
          completed || cancelled
            ? []
            : [
                {
                  id: state,
                  icon: unknown ? (
                    <TriangleAlert className="size-3.5" />
                  ) : (
                    <CircleX className="size-3.5" />
                  ),
                  description,
                  tone: unknown ? ('warning' as const) : ('destructive' as const),
                },
              ]
        }
      />
    </div>
  );
}
