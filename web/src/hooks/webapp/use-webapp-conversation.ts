import { useEffect } from 'react';
import { useQuery } from '@tanstack/react-query';
import { toast } from 'sonner';
import { WebAppService } from '@/services/webapp.service';
import type { WebAppApiResponseData, WebAppConversationDetail } from '@/services/types/webapp';
import { emitWebAppOffline, isWebAppOfflineError } from '@/utils/webapp/errors';

import { WEBAPP_KEYS } from '@/hooks/query-keys';

export function useWebAppConversation(versionUuid: string | null, conversationId: string | null) {
  const query = useQuery<WebAppApiResponseData<WebAppConversationDetail>>({
    queryKey: WEBAPP_KEYS.conversation(versionUuid || 'none', conversationId || 'none'),
    queryFn: () => WebAppService.getConversation(versionUuid ?? '', conversationId ?? ''),
    enabled: Boolean(versionUuid && conversationId),
    staleTime: 30 * 1000,
    gcTime: 5 * 60 * 1000,
    retry: false,
  });

  useEffect(() => {
    if (query.isError) {
      if (isWebAppOfflineError(query.error)) {
        emitWebAppOffline();
        return;
      }
      const err = query.error as unknown as { message?: string };
      toast.error(err?.message || 'Failed to load conversation');
    }
  }, [query.isError, query.error]);

  return query;
}
