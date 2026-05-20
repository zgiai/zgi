'use client';

import { Badge } from '@/components/ui/badge';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import type {
  DocumentStatus,
  DocumentDisplayStatus,
  DocumentIndexingStatus,
} from '@/services/types/dataset';

interface DocumentStatusBadgeProps {
  status: DocumentStatus | DocumentDisplayStatus | DocumentIndexingStatus;
  className?: string;
}

export function DocumentStatusBadge({ status, className }: DocumentStatusBadgeProps) {
  const t = useT('datasets');

  const statusConfig: Record<
    string,
    {
      label: string;
      variant: 'default' | 'secondary' | 'outline' | 'destructive';
      className: string;
    }
  > = {
    // Document Display Status (display_status)
    waiting: {
      label: t('status.waiting'),
      variant: 'outline',
      className: 'border-gray-200 text-gray-600 bg-gray-50',
    },
    queuing: {
      label: t('status.queuing'),
      variant: 'outline',
      className: 'border-blue-200 text-blue-700 bg-blue-50',
    },
    indexing: {
      label: t('status.indexing'),
      variant: 'secondary',
      className: 'bg-blue-100 text-blue-800 hover:bg-blue-200',
    },
    paused: {
      label: t('status.paused'),
      variant: 'outline',
      className: 'border-yellow-200 text-yellow-700 bg-yellow-50',
    },
    error: {
      label: t('status.error'),
      variant: 'destructive',
      className: 'bg-destructive text-destructive-foreground hover:bg-destructive/80',
    },
    available: {
      label: t('status.available'),
      variant: 'default',
      className: 'bg-green-100 text-green-800 hover:bg-green-200',
    },
    enabled: {
      label: t('status.enabled'),
      variant: 'default',
      className: 'bg-success text-success-foreground hover:bg-success/80',
    },
    disabled: {
      label: t('status.disabled'),
      variant: 'secondary',
      className: 'bg-gray-100 text-gray-600',
    },
    archived: {
      label: t('status.archived'),
      variant: 'outline',
      className: 'border-gray-200 text-gray-500 bg-gray-50',
    },

    // Document Indexing Status (indexing_status)
    parsing: {
      label: t('status.parsing'),
      variant: 'secondary',
      className: 'bg-blue-100 text-blue-800',
    },
    cleaning: {
      label: t('status.cleaning'),
      variant: 'secondary',
      className: 'bg-blue-100 text-blue-800',
    },
    splitting: {
      label: t('status.splitting'),
      variant: 'secondary',
      className: 'bg-blue-100 text-blue-800',
    },
    extracting: {
      label: t('status.extracting'),
      variant: 'secondary',
      className: 'bg-blue-100 text-blue-800',
    },
    alignment: {
      label: t('status.alignment'),
      variant: 'secondary',
      className: 'bg-blue-100 text-blue-800',
    },
    ingesting: {
      label: t('status.ingesting'),
      variant: 'secondary',
      className: 'bg-blue-100 text-blue-800',
    },
    completed: {
      label: t('status.completed'),
      variant: 'default',
      className: 'bg-success text-success-foreground hover:bg-success/80',
    },

    // Legacy Document Status (for backward compatibility)
    processing: {
      label: t('status.processing'),
      variant: 'secondary',
      className: 'bg-warning text-warning-foreground hover:bg-warning/80',
    },
    pending: {
      label: t('status.pending'),
      variant: 'outline',
      className: 'border-muted-foreground/30 text-muted-foreground',
    },
    failed: {
      label: t('status.failed'),
      variant: 'destructive',
      className: 'bg-destructive text-destructive-foreground hover:bg-destructive/80',
    },
    autoDisabled: {
      label: t('status.autoDisabled'),
      variant: 'secondary',
      className: 'bg-gray-100 text-gray-600',
    },
    auto_disabled: {
      label: t('status.auto_disabled'),
      variant: 'secondary',
      className: 'bg-gray-100 text-gray-600',
    },
  };

  const config = statusConfig[status] || {
    label: status,
    variant: 'secondary' as const,
    className: 'bg-muted text-muted-foreground',
  };

  return (
    <Badge variant={config.variant} className={cn(config.className, className)}>
      {config.label}
    </Badge>
  );
}
