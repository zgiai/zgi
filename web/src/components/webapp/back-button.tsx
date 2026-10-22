'use client';

import React from 'react';
import { ArrowLeft } from 'lucide-react';
import { useRouter } from 'next/navigation';
import { Button } from '@/components/ui/button';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import { WEBAPP_BACK_TARGET } from '@/lib/config';

const historyBack = 'back';

/**
 * @component WebAppBackButton
 * @category Feature
 * @status Stable
 * @description Back button in the agent landing page header, configured by
 * NEXT_PUBLIC_WEBAPP_BACK_URL: empty hides it, "back" returns to the previous
 * page, and any other value is the link it opens.
 * @usage Rendered by the WebApp version layout header.
 * @example
 * <WebAppBackButton />
 */
export function WebAppBackButton({ className }: { className?: string }) {
  const t = useT('webapp');
  const router = useRouter();
  const target = WEBAPP_BACK_TARGET;
  const goesBack = target.toLowerCase() === historyBack;
  // A page opened in a new tab has nothing to go back to.
  const [canGoBack, setCanGoBack] = React.useState(false);
  React.useEffect(() => {
    setCanGoBack(window.history.length > 1);
  }, []);

  const handleClick = React.useCallback(() => {
    if (goesBack) {
      router.back();
      return;
    }
    // The router adds the base path to internal paths; other links leave the app.
    if (target.startsWith('/') && !target.startsWith('//')) {
      router.push(target);
      return;
    }
    window.location.assign(target);
  }, [goesBack, router, target]);

  if (!target || (goesBack && !canGoBack)) return null;
  return (
    <Button
      type="button"
      size="sm"
      variant="ghost"
      className={cn('h-8 shrink-0 gap-1 px-2 text-xs', className)}
      onClick={handleClick}
      title={t('header.back')}
      aria-label={t('header.back')}
    >
      <ArrowLeft className="size-4" />
      <span className="hidden sm:inline">{t('header.back')}</span>
    </Button>
  );
}
