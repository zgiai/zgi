'use client';

import { useEffect, useMemo } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { MODEL_KEYS } from '@/hooks/query-keys';
import { modelService } from '@/services/model.service';
import type {
  DefaultModelUseCase,
  DefaultModelValue,
  ResolvedDefaultModelItem,
} from '@/services/types/model';

export interface UseDefaultModelByUseCaseReturn {
  value: DefaultModelValue | null;
  source: 'explicit' | 'auto' | 'none';
  isLoading: boolean;
  error: string | null;
}

const DEFAULT_MODEL_QUERY_OPTIONS = {
  staleTime: 5 * 60 * 1000,
  gcTime: 30 * 60 * 1000,
} as const;

function toDefaultModelValue(item: ResolvedDefaultModelItem | undefined): DefaultModelValue | null {
  if (!item || item.source === 'none' || !item.provider || !item.model) {
    return null;
  }

  return {
    provider: item.provider,
    model: item.model,
    params: item.params ?? {},
  };
}

export function useDefaultModelByUseCase(
  useCase: DefaultModelUseCase
): UseDefaultModelByUseCaseReturn {
  const queryClient = useQueryClient();

  const {
    data,
    isLoading,
    error,
  } = useQuery({
    queryKey: MODEL_KEYS.defaultModel(useCase),
    queryFn: async () => {
      const response = await queryClient.ensureQueryData({
        queryKey: MODEL_KEYS.defaultModels(),
        queryFn: () => modelService.getDefaultModels(),
        ...DEFAULT_MODEL_QUERY_OPTIONS,
      });

      return response.data.items.find(item => item.use_case === useCase);
    },
    ...DEFAULT_MODEL_QUERY_OPTIONS,
  });

  const value = useMemo(() => toDefaultModelValue(data), [data]);

  return {
    value,
    source: data?.source ?? 'none',
    isLoading,
    error: error ? (error as Error).message : null,
  };
}

export function useInitializeDefaultModelByUseCase({
  useCase,
  currentModel,
  onInitialize,
  shouldOverwrite = false,
  enabled = true,
}: {
  useCase: DefaultModelUseCase;
  currentModel: { provider?: string; model?: string; name?: string };
  onInitialize: (val: DefaultModelValue) => void;
  shouldOverwrite?: boolean;
  enabled?: boolean;
}) {
  const { value: defaultModel, isLoading } = useDefaultModelByUseCase(useCase);

  const modelName = currentModel.model || currentModel.name || '';
  const provider = currentModel.provider || '';

  useEffect(() => {
    if (!enabled || isLoading || !defaultModel) {
      return;
    }

    const isEmpty = !modelName || !provider;
    if (!isEmpty && !shouldOverwrite) {
      return;
    }

    if (modelName !== defaultModel.model || provider !== defaultModel.provider) {
      onInitialize(defaultModel);
    }
  }, [enabled, isLoading, defaultModel, shouldOverwrite, modelName, provider, onInitialize]);
}
