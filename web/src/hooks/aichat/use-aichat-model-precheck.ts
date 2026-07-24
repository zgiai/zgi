import { useQuery } from '@tanstack/react-query';
import { AICHAT_KEYS } from '@/hooks/query-keys';
import { aichatService } from '@/services/aichat.service';

export function useAIChatModelPrecheck(
  provider: string,
  model: string,
  options?: { enabled?: boolean }
) {
  const normalizedProvider = provider.trim();
  const normalizedModel = model.trim();

  return useQuery({
    queryKey: AICHAT_KEYS.workChatModelPrecheck(normalizedProvider, normalizedModel),
    queryFn: async () => {
      const response = await aichatService.precheckWorkChatModel({
        provider: normalizedProvider,
        model: normalizedModel,
      });
      return response.data;
    },
    enabled:
      (options?.enabled ?? true) && normalizedProvider.length > 0 && normalizedModel.length > 0,
    retry: false,
    staleTime: 0,
    refetchOnMount: true,
    refetchOnReconnect: true,
    refetchOnWindowFocus: true,
  });
}
