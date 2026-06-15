'use client';

import { Badge } from '@/components/ui/badge';
import { useT } from '@/i18n/translations';

interface LogStatusBadgeProps {
  status: string;
}

export function LogStatusBadge({ status }: LogStatusBadgeProps) {
  const t = useT('agents');
  const normalizedStatus = status.trim().toLowerCase();

  if (normalizedStatus === 'running' || normalizedStatus === 'in_progress') {
    return (
      <Badge variant="default" className="flex items-center gap-1">
        <span className="inline-block h-2 w-2 rounded-full bg-blue-500" />
        {t('workflow.running')}
      </Badge>
    );
  }

  if (
    normalizedStatus === 'succeeded' ||
    normalizedStatus === 'success' ||
    normalizedStatus === 'completed' ||
    normalizedStatus === 'partial-succeeded'
  ) {
    return (
      <Badge variant="secondary" className="flex items-center gap-1">
        <span className="inline-block h-2 w-2 rounded-full bg-green-500" />
        {t('workflow.succeeded')}
      </Badge>
    );
  }

  if (normalizedStatus === 'stopped') {
    return (
      <Badge variant="warning" className="flex items-center gap-1">
        <span className="inline-block h-2 w-2 rounded-full bg-warning" />
        {t('workflow.stopped')}
      </Badge>
    );
  }

  if (normalizedStatus === 'paused') {
    return (
      <Badge variant="info" className="flex items-center gap-1">
        <span className="inline-block h-2 w-2 rounded-full bg-info" />
        {t('workflow.paused')}
      </Badge>
    );
  }

  if (normalizedStatus === 'failed' || normalizedStatus === 'error') {
    return (
      <Badge variant="destructive" className="flex items-center gap-1">
        <span className="inline-block h-2 w-2 rounded-full bg-red-500" />
        {t('workflow.failed')}
      </Badge>
    );
  }

  return (
    <Badge variant="subtle" className="flex items-center gap-1">
      <span className="inline-block h-2 w-2 rounded-full bg-muted-foreground" />
      {status || '-'}
    </Badge>
  );
}
