'use client';

import { useMemo } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import { MODEL_KEYS } from '@/hooks/query-keys';
import { modelService } from '@/services/model.service';
import {
  type DefaultModelSource,
  type DefaultModelUseCase,
  type DefaultModelValue,
  type ResolvedDefaultModelItem,
} from '@/services/types/model';

export const DEFAULT_MODEL_USE_CASES = [
  'text-chat',
  'vision',
  'image-gen',
  'embedding',
  'rerank',
  'speech-to-text',
  'text-to-speech',
  'realtime-audio',
  'video-gen',
  'moderation',
  'reasoning',
  'function-calling',
] as const satisfies readonly DefaultModelUseCase[];

export type ManagedDefaultModelValue = DefaultModelValue;
export type DefaultModelSettings = Record<DefaultModelUseCase, ManagedDefaultModelValue>;

export interface ResolvedDefaultModel extends ManagedDefaultModelValue {
  use_case: DefaultModelUseCase;
  source: DefaultModelSource;
}

export type ResolvedDefaultModelSettings = Record<DefaultModelUseCase, ResolvedDefaultModel>;

export const EMPTY_DEFAULT_MODEL_VALUE: ManagedDefaultModelValue = {
  provider: '',
  model: '',
  params: {},
};

export function createEmptyDefaultModelSettings(): DefaultModelSettings {
  return DEFAULT_MODEL_USE_CASES.reduce(
    (acc, useCase) => {
      acc[useCase] = { ...EMPTY_DEFAULT_MODEL_VALUE };
      return acc;
    },
    {} as DefaultModelSettings
  );
}

function createEmptyResolvedDefaultModel(useCase: DefaultModelUseCase): ResolvedDefaultModel {
  return {
    ...EMPTY_DEFAULT_MODEL_VALUE,
    use_case: useCase,
    source: 'none',
  };
}

function normalizeResolvedDefaultModel(
  useCase: DefaultModelUseCase,
  item?: ResolvedDefaultModelItem
): ResolvedDefaultModel {
  if (!item) {
    return createEmptyResolvedDefaultModel(useCase);
  }

  return {
    use_case: useCase,
    provider: item.provider || '',
    model: item.model || '',
    params: item.params ?? {},
    source: item.source,
  };
}

function toResolvedSettings(items: ResolvedDefaultModelItem[] | undefined): ResolvedDefaultModelSettings {
  const itemMap = new Map(items?.map(item => [item.use_case, item]));

  return DEFAULT_MODEL_USE_CASES.reduce(
    (acc, useCase) => {
      acc[useCase] = normalizeResolvedDefaultModel(useCase, itemMap.get(useCase));
      return acc;
    },
    {} as ResolvedDefaultModelSettings
  );
}

function toEditableSettings(resolvedSettings: ResolvedDefaultModelSettings): DefaultModelSettings {
  return DEFAULT_MODEL_USE_CASES.reduce(
    (acc, useCase) => {
      const resolved = resolvedSettings[useCase];
      acc[useCase] = {
        provider: resolved.provider,
        model: resolved.model,
        params: resolved.params,
      };
      return acc;
    },
    {} as DefaultModelSettings
  );
}

function shallowEqualParams(
  a: Record<string, number | string | boolean>,
  b: Record<string, number | string | boolean>
): boolean {
  const aKeys = Object.keys(a);
  const bKeys = Object.keys(b);

  if (aKeys.length !== bKeys.length) {
    return false;
  }

  return aKeys.every(key => a[key] === b[key]);
}

function isSameDefaultModel(a: ManagedDefaultModelValue, b: ManagedDefaultModelValue): boolean {
  return (
    a.provider === b.provider &&
    a.model === b.model &&
    shallowEqualParams(a.params ?? {}, b.params ?? {})
  );
}

export function useDefaultModels() {
  const t = useT('common');
  const queryClient = useQueryClient();

  const {
    data,
    isLoading,
    isError,
    error,
    refetch,
  } = useQuery({
    queryKey: MODEL_KEYS.defaultModels(),
    queryFn: () => modelService.getDefaultModels(),
    staleTime: 5 * 60 * 1000,
    gcTime: 10 * 60 * 1000,
    refetchOnWindowFocus: false,
  });

  const resolvedSettings = useMemo(() => toResolvedSettings(data?.data?.items), [data?.data?.items]);
  const settings = useMemo(() => toEditableSettings(resolvedSettings), [resolvedSettings]);

  const updateMutation = useMutation({
    mutationFn: async (nextSettings: DefaultModelSettings) => {
      const operations: Promise<unknown>[] = [];

      for (const useCase of DEFAULT_MODEL_USE_CASES) {
        const nextValue = nextSettings[useCase];
        const currentResolved = resolvedSettings[useCase];
        const hasNextValue = Boolean(nextValue.provider && nextValue.model);

        if (!hasNextValue) {
          if (currentResolved.source === 'explicit') {
            operations.push(modelService.deleteDefaultModel(useCase));
          }
          continue;
        }

        const normalizedNextValue: ManagedDefaultModelValue = {
          provider: nextValue.provider,
          model: nextValue.model,
          params: nextValue.params ?? {},
        };

        if (isSameDefaultModel(normalizedNextValue, currentResolved)) {
          continue;
        }

        operations.push(
          modelService.upsertDefaultModel(useCase, {
            provider: normalizedNextValue.provider,
            model: normalizedNextValue.model,
            params: normalizedNextValue.params,
          })
        );
      }

      await Promise.all(operations);
    },
    onSuccess: async () => {
      toast.success(t('saveSuccess'));
      await queryClient.invalidateQueries({ queryKey: MODEL_KEYS.defaultModels() });
    },
    onError: () => {
      toast.error(t('saveFailed'));
    },
  });

  return {
    settings,
    resolvedSettings,
    isLoading,
    isError,
    error,
    refetch,
    updateDefaultModels: updateMutation.mutate,
    isUpdating: updateMutation.isPending,
  };
}
