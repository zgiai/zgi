import { useQuery } from '@tanstack/react-query';
import { AICHAT_KEYS } from '@/hooks/query-keys';
import { aichatService } from '@/services/aichat.service';

export interface UseAIChatAssetOperationAuditsParams {
  page?: number;
  limit?: number;
}

/**
 * @hook useAIChatAssetOperationAudits
 * @description Load replayable asset operation audit records for one AIChat conversation.
 */
export function useAIChatAssetOperationAudits(
  conversationId: string | null | undefined,
  params: UseAIChatAssetOperationAuditsParams = {},
  options?: { enabled?: boolean }
) {
  const queryParams = {
    page: params.page ?? 1,
    limit: params.limit ?? 50,
  };

  return useQuery({
    queryKey: AICHAT_KEYS.assetOperationAudits(conversationId ?? '', queryParams),
    queryFn: async () => {
      const response = await aichatService.listAssetOperationAudits(
        conversationId ?? '',
        queryParams
      );
      return response.data;
    },
    enabled: Boolean(conversationId) && (options?.enabled ?? true),
    retry: false,
  });
}
