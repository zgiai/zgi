'use client';

import { ShieldX } from 'lucide-react';
import { OperationConfirmationCard } from '@/components/aichat/operation-cards';
import { useT } from '@/i18n/translations';

interface ConnectorRuntimeBlockedStateProps {
  compact?: boolean;
  className?: string;
}

/** Non-interactive state for unattended high-risk or unclassified execution. */
export function ConnectorRuntimeBlockedState({
  compact = false,
  className,
}: ConnectorRuntimeBlockedStateProps) {
  const t = useT('connectors');

  return (
    <div role="status" aria-live="polite">
      <OperationConfirmationCard
        className={className}
        compact={compact}
        title={t('runtime.blocked.title')}
        description={t('runtime.blocked.description')}
        status="rejected"
        statusLabel={t('runtime.blocked.status')}
        warnings={[
          {
            id: 'confirmation-required',
            icon: <ShieldX className="size-3.5" />,
            title: t('runtime.blocked.reasonTitle'),
            description: t('runtime.blocked.reasonDescription'),
            tone: 'destructive',
          },
        ]}
      />
    </div>
  );
}
