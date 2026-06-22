import { useQuery } from '@tanstack/react-query';
import { AICHAT_KEYS } from '@/hooks/query-keys';
import { aichatService } from '@/services/aichat.service';

export function useAIChatSearch(query: string, limit = 20, options?: { enabled?: boolean }) {
  const normalizedQuery = query.trim();

  return useQuery({
    queryKey: AICHAT_KEYS.search(normalizedQuery, limit),
    queryFn: async () => {
      const response = await aichatService.search(normalizedQuery, limit);
      return response.data ?? [];
    },
    enabled: (options?.enabled ?? true) && normalizedQuery.length > 0,
    retry: false,
    staleTime: 60_000,
    refetchOnMount: false,
    refetchOnReconnect: false,
    refetchOnWindowFocus: false,
  });
}
