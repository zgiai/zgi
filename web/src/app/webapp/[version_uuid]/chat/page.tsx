'use client';

import { Skeleton } from '@/components/ui/skeleton';
import AgentWebappChat from '@/components/webapp/agent-chat';
import WebappChat from '@/components/webapp/chat';
import { useWebAppConfig } from '@/hooks/webapp/use-webapp';
import { useParams } from 'next/navigation';
import { useT } from '@/i18n';
import { AlertCircle } from 'lucide-react';
import { WebAppOfflineState } from '@/components/webapp/offline-state';
import { isWebAppOfflineError } from '@/utils/webapp/errors';

export default function WebappChatPage(): JSX.Element {
  const t = useT();
  const { version_uuid } = useParams<{ version_uuid: string }>();
  const { data, error, isLoading } = useWebAppConfig(version_uuid);

  return (
    <div className="box-border h-full min-h-0 w-full overflow-hidden md:px-4 md:pb-2">
      <div className="h-full w-full min-h-0 bg-background overflow-hidden md:rounded-lg md:border md:shadow-sm">
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
        ) : data?.data?.config?.type?.toUpperCase?.() === 'AGENT' ? (
          <AgentWebappChat webAppId={version_uuid} config={data.data} />
        ) : data?.data ? (
          <WebappChat
            versionUuid={version_uuid}
            config={data.data}
            agentId={data.data?.config?.agent_id}
          />
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
