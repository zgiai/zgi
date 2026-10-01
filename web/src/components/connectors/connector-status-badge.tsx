'use client';

import { AlertTriangle, CheckCircle2, CircleSlash2 } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import type { ConnectorReadiness } from '@/services/types/connector';

interface ConnectorStatusBadgeProps {
  status: ConnectorReadiness;
  className?: string;
}

export function ConnectorStatusBadge({ status, className }: ConnectorStatusBadgeProps) {
  const t = useT('connectors');
  if (status === 'ready') {
    return (
      <Badge variant="success" className={cn('gap-1.5', className)}>
        <CheckCircle2 aria-hidden="true" />
        {t('status.ready')}
      </Badge>
    );
  }
  if (status === 'reconnect_required') {
    return (
      <Badge variant="warning" className={cn('gap-1.5', className)}>
        <AlertTriangle aria-hidden="true" />
        {t('status.reconnectRequired')}
      </Badge>
    );
  }
  return (
    <Badge variant="subtle" className={cn('gap-1.5', className)}>
      <CircleSlash2 aria-hidden="true" />
      {t('status.unavailable')}
    </Badge>
  );
}
