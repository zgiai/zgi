'use client';

import React from 'react';
import { AlertTriangle, RotateCcw } from 'lucide-react';
import { useT } from '@/i18n';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';

interface IndexFailedBannerProps {
  count: number;
  onRetry?: () => void;
  retrying?: boolean;
  className?: string;
}

export function IndexFailedBanner({ count, onRetry, retrying, className }: IndexFailedBannerProps) {
  const t = useT('datasets');

  if (!count || count <= 0) return null;

  return (
    <Alert className={cn('border-destructive/30 bg-destructive/10 p-2', className)}>
      <div className="flex items-center justify-between gap-4">
        <div className="flex items-center gap-2">
          <AlertTriangle className="h-5 w-5 text-destructive mt-0.5" />
          <div>
            <div className="text-sm font-medium text-destructive">
              {t('documents.errorBanner.failedDocsTitle', { count })}
            </div>
            <AlertDescription className="text-xs mt-1">
              {t('documents.errorBanner.failedDocsDesc')}
            </AlertDescription>
          </div>
        </div>
        {onRetry && (
          <Button onClick={onRetry} disabled={retrying} variant="outline" size="sm">
            <RotateCcw className="h-4 w-4 mr-1" />
            {retrying ? t('documents.errorBanner.retrying') : t('documents.errorBanner.retry')}
          </Button>
        )}
      </div>
    </Alert>
  );
}

export default IndexFailedBanner;
