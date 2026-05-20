'use client';

import { WifiOff } from 'lucide-react';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';

interface WebAppOfflineStateProps {
  className?: string;
}

/**
 * @component WebAppOfflineState
 * @category Feature
 * @status Stable
 * @description Dedicated unavailable state for public WebApp pages when the app is offline.
 * @usage Render for public WebApp config, chat, run, and conversation errors with code 204008.
 * @example
 * <WebAppOfflineState />
 */
export function WebAppOfflineState({ className }: WebAppOfflineStateProps) {
  const t = useT('webapp');

  return (
    <div
      className={cn(
        'flex h-full min-h-80 w-full flex-col items-center justify-center px-6 text-center',
        className
      )}
    >
      <div className="flex size-14 items-center justify-center rounded-full bg-destructive/10 text-destructive ring-1 ring-destructive/20">
        <WifiOff className="size-7" />
      </div>
      <h1 className="mt-4 text-lg font-semibold text-foreground">{t('offline.title')}</h1>
      <p className="mt-2 max-w-md text-sm leading-6 text-muted-foreground">
        {t('offline.description')}
      </p>
    </div>
  );
}
