import { useQuery } from '@tanstack/react-query';
import { useEffect } from 'react';
import { toast } from 'sonner';
import { WebAppService } from '@/services/webapp.service';
import type { WebAppApiResponseData, WebAppWorkflowConfig } from '@/services/types/webapp';
import {
  emitWebAppOffline,
  isWebAppNotPublishedError,
  isWebAppOfflineError,
} from '@/utils/webapp/errors';

/**
 * Fetch webapp config by version UUID using webappHttp.
 * Keep consistent with existing hooks: minimal toasts for GET, error only.
 */
import { WEBAPP_KEYS } from '@/hooks/query-keys';

export function useWebAppConfig(versionUuid: string | null) {
  const query = useQuery<WebAppApiResponseData<WebAppWorkflowConfig>>({
    queryKey: WEBAPP_KEYS.config(versionUuid || 'none'),
    queryFn: () => WebAppService.getConfig(versionUuid ?? ''),
    enabled: Boolean(versionUuid),
    staleTime: 0,
    gcTime: 10 * 60 * 1000,
    refetchOnMount: 'always',
    retry: false,
  });
  useEffect(() => {
    if (query.isError) {
      if (isWebAppOfflineError(query.error)) {
        emitWebAppOffline();
        return;
      }
      if (isWebAppNotPublishedError(query.error)) {
        return;
      }
      const err = query.error as unknown as { message?: string };
      const message = err?.message || 'Failed to load webapp config';
      toast.error(message);
    }
  }, [query.isError, query.error]);
  return query;
}
