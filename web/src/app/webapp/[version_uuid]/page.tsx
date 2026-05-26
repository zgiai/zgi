'use client';

import React, { useEffect } from 'react';
import { Skeleton } from '@/components/ui/skeleton';
import { useWebAppConfig } from '@/hooks/webapp/use-webapp';
import { useParams, useRouter } from 'next/navigation';
import { detectWebappMode } from '@/utils/webapp/helpers';
import { useT } from '@/i18n';
import { WebAppOfflineState } from '@/components/webapp/offline-state';
import { WebAppNotPublishedState } from '@/components/webapp/not-published-state';
import { isWebAppNotPublishedError, isWebAppOfflineError } from '@/utils/webapp/errors';

/**
 * Redirect shim: detects webapp mode and redirects to /chat or /run
 */
export default function WebappPage(): JSX.Element {
  const { version_uuid } = useParams<{ version_uuid: string }>();
  const { data, error, isLoading, isError } = useWebAppConfig(version_uuid);
  const router = useRouter();
  const t = useT('webapp');

  useEffect(() => {
    if (!isLoading && data?.data) {
      const mode = detectWebappMode(data.data);
      router.replace(`/webapp/${version_uuid}/${mode}`);
    }
  }, [isLoading, data, version_uuid, router]);

  return (
    <div className="h-full w-full flex items-stretch justify-center p-4">
      <div className="w-full h-full border rounded-lg bg-background shadow-sm overflow-hidden">
        {isLoading ? (
          <div className="h-full p-4 flex flex-col gap-3">
            <Skeleton className="h-8 w-48" />
            <div className="flex-1 min-h-0 border rounded-md p-4">
              <Skeleton className="h-full w-full" />
            </div>
            <Skeleton className="h-10 w-full" />
          </div>
        ) : isWebAppOfflineError(error) ? (
          <WebAppOfflineState />
        ) : isWebAppNotPublishedError(error) ? (
          <WebAppNotPublishedState />
        ) : isError ? (
          <div className="h-full flex items-center justify-center text-sm text-muted-foreground">
            {t('run.configError')}
          </div>
        ) : (
          <div className="h-full flex items-center justify-center text-sm text-muted-foreground">
            {t('run.loadingConfig')}
          </div>
        )}
      </div>
    </div>
  );
}
