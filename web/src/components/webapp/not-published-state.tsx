'use client';

import { FileWarning } from 'lucide-react';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';

interface WebAppNotPublishedStateProps {
  className?: string;
}

/**
 * @component WebAppNotPublishedState
 * @category Feature
 * @status Stable
 * @description Dedicated unavailable state for public Agent WebApps before a published version exists.
 */
export function WebAppNotPublishedState({ className }: WebAppNotPublishedStateProps) {
  const t = useT('webapp');

  return (
    <div
      className={cn(
        'flex h-full min-h-80 w-full flex-col items-center justify-center px-6 text-center',
        className
      )}
    >
      <div className="flex size-14 items-center justify-center rounded-full bg-muted text-muted-foreground ring-1 ring-border">
        <FileWarning className="size-7" />
      </div>
      <h1 className="mt-4 text-lg font-semibold text-foreground">{t('notPublished.title')}</h1>
      <p className="mt-2 max-w-md text-sm leading-6 text-muted-foreground">
        {t('notPublished.description')}
      </p>
    </div>
  );
}
