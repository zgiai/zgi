'use client';

import { Skeleton } from '@/components/ui/skeleton';
import { WebappRun } from '@/components/webapp/run';
import { useWebAppConfig } from '@/hooks/webapp/use-webapp';
import { useParams } from 'next/navigation';
import { useT } from '@/i18n';
import { AlertCircle } from 'lucide-react';
import { WebAppOfflineState } from '@/components/webapp/offline-state';
import { isWebAppOfflineError } from '@/utils/webapp/errors';

export default function WebappRunPage(): JSX.Element {
  const { version_uuid } = useParams<{ version_uuid: string }>();
  const { data, error, isLoading } = useWebAppConfig(version_uuid);
  const t = useT();

  return (
    <div className="h-full w-full">
      <div className="w-full h-full bg-background overflow-hidden">
        {isLoading ? (
          <div className="h-full grid grid-cols-1 md:grid-cols-[360px_1fr] gap-4 md:px-4 md:pb-2">
            <div className="md:border md:rounded-lg p-4">
              <Skeleton className="h-6 w-32 mb-4" />
              <div className="space-y-3">
                <Skeleton className="h-10 w-full" />
                <Skeleton className="h-10 w-full" />
                <Skeleton className="h-10 w-full" />
              </div>
            </div>
            <div className="md:border md:rounded-lg p-4">
              <Skeleton className="h-full w-full" />
            </div>
          </div>
        ) : isWebAppOfflineError(error) ? (
          <WebAppOfflineState />
        ) : data?.data ? (
          <WebappRun versionUuid={version_uuid} config={data.data} />
        ) : (
          <div className="h-full flex flex-col items-center justify-center gap-2 text-sm text-muted-foreground">
            <AlertCircle className="h-8 w-8 text-destructive/60" />
            <span>{t('webapp.run.configError')}</span>
          </div>
        )}
      </div>
    </div>
  );
}
