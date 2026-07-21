import { useQuery } from '@tanstack/react-query';
import { ImageRuntimeService } from '@/services/image-runtime.service';

export const IMAGE_RUNTIME_KEYS = {
  models: ['image-runtime', 'models'] as const,
  conversations: ['image-runtime', 'conversations'] as const,
  search: ['image-runtime', 'search'] as const,
};

export function useImageRuntimeModels() {
  const query = useQuery({
    queryKey: IMAGE_RUNTIME_KEYS.models,
    queryFn: () => ImageRuntimeService.listModels(),
    staleTime: 60 * 1000,
    retry: false,
  });

  return {
    ...query,
    models: query.data?.data ?? [],
  };
}
