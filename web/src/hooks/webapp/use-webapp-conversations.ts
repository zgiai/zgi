import { useEffect } from 'react';
import { useQuery } from '@tanstack/react-query';
import { toast } from 'sonner';
import { WebAppService } from '@/services/webapp.service';
import type { WebAppApiResponseData, WebAppConversationList } from '@/services/types/webapp';
import { emitWebAppOffline, isWebAppOfflineError } from '@/utils/webapp/errors';

export interface UseWebAppConversationsParams {
  page?: number;
  limit?: number;
}

import { WEBAPP_KEYS } from '@/hooks/query-keys';

export function useWebAppConversations(
  versionUuid: string | null,
  params: UseWebAppConversationsParams
) {
  const query = useQuery<WebAppApiResponseData<WebAppConversationList>>({
    queryKey: WEBAPP_KEYS.conversationList(versionUuid || 'none', {
      page: params.page ?? 1,
      limit: params.limit ?? 20,
    }),
    queryFn: () => WebAppService.getConversations(versionUuid ?? '', params),
    enabled: Boolean(versionUuid),
    staleTime: 30 * 1000,
    gcTime: 5 * 60 * 1000,
    retry: false,
    placeholderData: previous => previous, // keep previous page data during pagination
  });

  useEffect(() => {
    if (query.isError) {
      if (isWebAppOfflineError(query.error)) {
        emitWebAppOffline();
        return;
      }
      const err = query.error as unknown as { message?: string };
      toast.error(err?.message || 'Failed to load conversations');
    }
  }, [query.isError, query.error]);

  return query;
}
