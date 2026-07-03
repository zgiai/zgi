'use client';

import { useEffect } from 'react';
import { useParams, useRouter } from 'next/navigation';
import { Skeleton } from '@/components/ui/skeleton';
import { WebAppNotPublishedState } from '@/components/webapp/not-published-state';
import { WebAppOfflineState } from '@/components/webapp/offline-state';
import { useWebAppConfig } from '@/hooks/webapp/use-webapp';
import { useT } from '@/i18n';
import { AlertCircle } from 'lucide-react';
import { isWebAppNotPublishedError, isWebAppOfflineError } from '@/utils/webapp/errors';
import { detectWebappMode } from '@/utils/webapp/helpers';

export default function WebappConversationRedirectPage(): JSX.Element {
  const { version_uuid, conversation_id } = useParams<{
    version_uuid: string;
    conversation_id: string;
  }>();
  const router = useRouter();
  const t = useT('webapp');
  const { data, error, isError, isLoading } = useWebAppConfig(version_uuid);

  useEffect(() => {
    if (isLoading || !data?.data) return;
    const mode = detectWebappMode(data.data);
    const conv = encodeURIComponent(conversation_id);
    router.replace(`/webapp/${version_uuid}/${mode}?convId=${conv}`);
  }, [conversation_id, data, isLoading, router, version_uuid]);

  if (isWebAppOfflineError(error)) {
    return <WebAppOfflineState />;
  }

  if (isWebAppNotPublishedError(error)) {
    return <WebAppNotPublishedState />;
  }

  if (isError) {
    return (
      <div className="flex h-full w-full flex-col items-center justify-center gap-2 text-sm text-muted-foreground">
        <AlertCircle className="size-8 text-destructive/60" />
        <span>{t('run.configError')}</span>
      </div>
    );
  }

  return (
    <div className="h-full w-full p-4">
      <Skeleton className="h-full w-full" />
    </div>
  );
}
