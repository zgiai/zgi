'use client';

import { use, useEffect, useMemo } from 'react';
import { AlertCircle } from 'lucide-react';
import AgentWebappChat from '@/components/webapp/agent-chat';
import WebappChat from '@/components/webapp/chat';
import { WebAppNotPublishedState } from '@/components/webapp/not-published-state';
import { WebappRun } from '@/components/webapp/run';
import { PermissionDeniedState } from '@/components/common/permission-gate-state';
import { Card, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';
import { useRunnableWebApps } from '@/hooks/agent/use-runnable-webapps';
import { useWebAppConfig } from '@/hooks/webapp/use-webapp';
import { useT } from '@/i18n/translations';
import { isWebAppNotPublishedError } from '@/utils/webapp/errors';
import { detectWebappMode } from '@/utils/webapp/helpers';

const RECENT_WEBAPP_STORAGE_KEY = 'zgi:webapp:recent';

interface ConsoleWorkAppDetailPageProps {
  params: Promise<{ web_app_id: string }>;
}

export default function ConsoleWorkAppDetailPage({ params }: ConsoleWorkAppDetailPageProps) {
  const t = useT('webapp');
  const resolvedParams = use(params);
  const webAppId = resolvedParams.web_app_id;
  const {
    items,
    isLoading: isListLoading,
    canUseResourceList,
  } = useRunnableWebApps({ workspaceId: null });

  const isRunnable = useMemo(
    () => items.some(item => item.web_app_id === webAppId),
    [items, webAppId]
  );
  const shouldLoadConfig = !isListLoading && canUseResourceList && isRunnable;
  const { data, error: configError, isLoading: isConfigLoading } = useWebAppConfig(webAppId, {
    enabled: shouldLoadConfig,
  });

  useEffect(() => {
    if (!isRunnable || typeof window === 'undefined') return;

    const current = window.localStorage.getItem(RECENT_WEBAPP_STORAGE_KEY);
    const ids = current ? (JSON.parse(current) as string[]) : [];
    const nextIds = [webAppId, ...ids.filter(id => id !== webAppId)].slice(0, 6);
    window.localStorage.setItem(RECENT_WEBAPP_STORAGE_KEY, JSON.stringify(nextIds));
  }, [isRunnable, webAppId]);

  if (isListLoading || isConfigLoading) {
    return (
      <div className="h-full w-full p-6">
        <div className="space-y-4">
          <Skeleton className="h-8 w-48" />
          <Skeleton className="h-[520px] w-full" />
        </div>
      </div>
    );
  }

  if (!canUseResourceList) {
    return <PermissionDeniedState />;
  }

  if (!isRunnable) {
    return (
      <div className="h-full w-full p-6">
        <Card className="max-w-xl border-dashed">
          <CardHeader>
            <div className="size-10 rounded-full bg-muted flex items-center justify-center mb-2">
              <AlertCircle className="size-5 text-muted-foreground" />
            </div>
            <CardTitle>{t('appCenter.appUnavailableTitle')}</CardTitle>
            <CardDescription>{t('appCenter.appUnavailableDescription')}</CardDescription>
          </CardHeader>
        </Card>
      </div>
    );
  }

  if (isWebAppNotPublishedError(configError)) {
    return (
      <div className="h-full w-full p-6">
        <Card className="h-full border-dashed">
          <WebAppNotPublishedState />
        </Card>
      </div>
    );
  }

  const config = data?.data;

  if (!config) {
    return (
      <div className="h-full w-full p-6">
        <Card className="max-w-xl border-dashed">
          <CardHeader>
            <div className="size-10 rounded-full bg-muted flex items-center justify-center mb-2">
              <AlertCircle className="size-5 text-muted-foreground" />
            </div>
            <CardTitle>{t('appCenter.loadAppFailed')}</CardTitle>
          </CardHeader>
        </Card>
      </div>
    );
  }

  const mode = detectWebappMode(config);

  if (config.config?.type?.toUpperCase?.() === 'AGENT') {
    return (
      <div className="h-full min-h-0 w-full overflow-hidden bg-background">
        <AgentWebappChat webAppId={webAppId} config={config} />
      </div>
    );
  }

  if (mode === 'run') {
    return (
      <div className="h-full w-full">
        <div className="w-full h-full bg-background overflow-hidden">
          <WebappRun versionUuid={webAppId} config={config} enablePrecheck />
        </div>
      </div>
    );
  }

  return (
    <div className="box-border h-full min-h-0 w-full overflow-hidden md:p-2">
      <div className="h-full w-full min-h-0 bg-background overflow-hidden md:rounded-lg md:border md:shadow-sm">
        <WebappChat
          versionUuid={webAppId}
          config={config}
          agentId={config.config.agent_id}
          enablePrecheck
        />
      </div>
    </div>
  );
}
