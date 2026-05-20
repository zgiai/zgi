import { useEffect, useMemo } from 'react';
import { useQuery } from '@tanstack/react-query';
import { toast } from 'sonner';
import { modelService } from '@/services/model.service';
import type { ParameterRuleItem } from '@/services/types/model';

export interface UseModelParameterRulesOptions {
  provider?: string;
  model?: string;
  enabled?: boolean;
  staleTime?: number;
}

export interface UseModelParameterRulesReturn {
  data: ParameterRuleItem[];
  isLoading: boolean;
  isFetching: boolean;
  error: string | null;
  isNotFound: boolean;
  hasLoaded: boolean;
  refetch: () => Promise<void>;
}

export function useModelParameterRules({
  provider,
  model,
  enabled = true,
  staleTime = 24 * 60 * 60 * 1000,
}: UseModelParameterRulesOptions = {}): UseModelParameterRulesReturn {
  const queryKey = useMemo(() => ['model-parameter-rules', provider, model], [provider, model]);

  const {
    data = [],
    isLoading,
    isFetching,
    error,
    refetch,
  } = useQuery({
    queryKey,
    queryFn: async (): Promise<ParameterRuleItem[]> => {
      if (!provider || !model) return [];
      const res = await modelService.getModelParameters({ provider, model });
      return res?.data ?? [];
    },
    enabled: enabled && Boolean(provider) && Boolean(model),
    staleTime,
    gcTime: 24 * 60 * 60 * 1000,
    refetchOnWindowFocus: false,
    retry: false,
    retryDelay: attempt => Math.min(1000 * 2 ** attempt, 30000),
    meta: { errorMessage: 'Failed to load model parameter rules' },
  });

  useEffect(() => {
    const status = (error as { response?: { status?: number } } | null)?.response?.status;
    if (error && status !== 404) {
      toast.error((error as Error).message || 'Failed to load model parameter rules');
    }
  }, [error]);

  const isNotFound = (error as { response?: { status?: number } } | null)?.response?.status === 404;
  const hasLoaded = useMemo(
    () => data.length > 0 || (!isLoading && (!error || isNotFound)),
    [data.length, isLoading, error, isNotFound]
  );

  return {
    data,
    isLoading,
    isFetching,
    error: error && !isNotFound ? (error as Error).message : null,
    isNotFound,
    hasLoaded,
    refetch: async () => {
      await refetch({ cancelRefetch: false });
    },
  };
}
