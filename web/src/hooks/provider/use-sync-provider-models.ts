import { useState, useCallback } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import type { ScopedTranslations } from '@/i18n/translations';
import { modelMetaService } from '@/services/model-meta.service';
import { providerService } from '@/services/provider.service';
import { PROVIDER_KEYS, MODEL_KEYS, MODEL_META_KEYS } from '@/hooks/query-keys';
import type { ApiResponseData } from '@/services/types/common';
import type {
  ModelMetaDiffResponse,
  ModelMetaModelUpdateProvidersResponse,
  ModelMetaProviderDiffResponse,
  ModelMetaStatusResponse,
  ModelMetaSyncResult,
} from '@/services/types/provider';

type ModelMetaErrorLike = Error & {
  businessError?: { code?: string; message?: string };
  response?: { status?: number; data?: { code?: string; message?: string } };
};

function getModelMetaErrorCode(error: unknown): string | undefined {
  if (!error || typeof error !== 'object') return undefined;
  const err = error as ModelMetaErrorLike;
  return err.businessError?.code ?? err.response?.data?.code;
}

function getModelMetaErrorMessage(error: unknown, fallback: string): string {
  if (!error || typeof error !== 'object') return fallback;
  const err = error as ModelMetaErrorLike;
  return (
    err.businessError?.message ??
    err.response?.data?.message ??
    err.message ??
    fallback
  );
}

export function isModelMetaReadOnlyError(error: unknown): boolean {
  return getModelMetaErrorCode(error) === '403003';
}

export function isModelMetaForbiddenError(error: unknown): boolean {
  if (!error || typeof error !== 'object') return false;
  const err = error as ModelMetaErrorLike;
  return err.response?.status === 403 && !isModelMetaReadOnlyError(error);
}

function invalidateProviderModelQueries(
  queryClient: ReturnType<typeof useQueryClient>,
  provider?: string
) {
  queryClient.invalidateQueries({ queryKey: PROVIDER_KEYS.all });
  queryClient.invalidateQueries({ queryKey: MODEL_KEYS.allRoot });
  if (provider) {
    queryClient.invalidateQueries({ queryKey: PROVIDER_KEYS.detail(provider) });
    queryClient.invalidateQueries({ queryKey: MODEL_META_KEYS.modelDiff(provider) });
  }
}

function showSyncResultToast(
  result: ModelMetaSyncResult,
  t: ScopedTranslations<'aiProviders'>
): void {
  const description = result.errors?.length
    ? result.errors.slice(0, 3).join('\n')
    : undefined;

  if (result.status === 'success') {
    toast.success(
      t('sidebar.syncSuccessDetailed', {
        provider: result.provider,
        new: result.new_models,
        updated: result.updated_models,
        duration: (result.duration_ms / 1000).toFixed(1),
      }),
      { description }
    );
    return;
  }

  if (result.status === 'partial') {
    toast.warning(
      t('sidebar.syncPartialDetailed', {
        provider: result.provider,
        success: result.success_models,
        failed: result.failed_models,
        duration: (result.duration_ms / 1000).toFixed(1),
      }),
      { description }
    );
    return;
  }

  toast.error(
    t('sidebar.syncFailedDetailed', {
      provider: result.provider,
      failed: result.failed_models,
      duration: (result.duration_ms / 1000).toFixed(1),
    }),
    { description }
  );
}

/**
 * Hook to check for model updates for a specific provider
 */
export function useCheckModelUpdates(provider: string) {
  const t = useT('aiProviders');
  const [isCheckingUpdates, setIsCheckingUpdates] = useState(false);
  const [showDiffDialog, setShowDiffDialog] = useState(false);
  const [diffData, setDiffData] = useState<ModelMetaDiffResponse | null>(null);

  const { refetch: checkDiff } = useModelDiff(provider, { enabled: false });

  const onCheckUpdates = useCallback(async () => {
    setIsCheckingUpdates(true);
    try {
      const { data } = await checkDiff();
      if (!data) return;

      const { summary } = data.data;
      if (summary.new_models > 0 || summary.updated_models > 0) {
        setDiffData(data.data);
        setShowDiffDialog(true);
      } else {
        toast.success(t('sidebar.noUpdatesInfo'));
      }
    } catch (error: unknown) {
      toast.error(getModelMetaErrorMessage(error, t('sidebar.syncError')));
    } finally {
      setIsCheckingUpdates(false);
    }
  }, [checkDiff, t]);

  return {
    isCheckingUpdates,
    showDiffDialog,
    setShowDiffDialog,
    diffData,
    setDiffData,
    onCheckUpdates,
  };
}

/**
 * @hook useModelMetaStatus
 * @description Loads the high-level ModelMeta synchronization status.
 */
export function useModelMetaStatus(options?: { enabled?: boolean }) {
  return useQuery<ApiResponseData<ModelMetaStatusResponse>>({
    queryKey: MODEL_META_KEYS.status(),
    queryFn: () => modelMetaService.getSyncStatus(),
    enabled: options?.enabled ?? true,
    staleTime: 60 * 1000,
    retry: false,
  });
}

/**
 * @hook useModelMetaProviderDiff
 * @description Loads provider-level diff data for the ModelMeta catalog.
 */
export function useModelMetaProviderDiff(options?: { enabled?: boolean }) {
  return useQuery<ApiResponseData<ModelMetaProviderDiffResponse>>({
    queryKey: MODEL_META_KEYS.providerDiff(),
    queryFn: () => modelMetaService.getProviderDiff(),
    enabled: options?.enabled ?? true,
    staleTime: 60 * 1000,
    retry: false,
  });
}

/**
 * @hook useModelMetaModelUpdateProviders
 * @description Discovers providers that have model-level updates even when provider metadata is unchanged.
 */
export function useModelMetaModelUpdateProviders(options?: { enabled?: boolean }) {
  return useQuery<ModelMetaModelUpdateProvidersResponse>({
    queryKey: MODEL_META_KEYS.modelUpdateProviders(),
    queryFn: async () => {
      const providersResponse = await providerService.getProviders({ limit: 200, page: 1 });
      const providers = (providersResponse.data.items ?? []).filter(
        item => item.provider_type === 'global'
      );

      const checkedAt = new Date().toISOString();
      const providerErrors: Array<{ provider: string; error: string }> = [];

      const settled = await Promise.allSettled(
        providers.map(async provider => {
          const diffResponse = await modelMetaService.getModelDiff(provider.provider);
          const summary = diffResponse.data.summary;
          const changedCount = summary.new_models + summary.updated_models;

          if (changedCount === 0) {
            return null;
          }

          return {
            provider: provider.provider,
            name: provider.provider_name || provider.provider,
            new_models: summary.new_models,
            updated_models: summary.updated_models,
            total_remote: summary.total_remote,
            total_local: summary.total_local,
          };
        })
      );

      const items = settled
        .flatMap((result, index) => {
          if (result.status === 'fulfilled') {
            return result.value ? [result.value] : [];
          }

          providerErrors.push({
            provider: providers[index]?.provider ?? 'unknown',
            error: getModelMetaErrorMessage(result.reason, 'Failed to load model diff'),
          });
          return [];
        })
        .sort(
          (left, right) =>
            right.new_models +
            right.updated_models -
            (left.new_models + left.updated_models)
        );

      return {
        checked_at: checkedAt,
        items,
        provider_errors: providerErrors.length > 0 ? providerErrors : undefined,
      };
    },
    enabled: options?.enabled ?? true,
    staleTime: 60 * 1000,
    retry: false,
  });
}

/**
 * Hook to get model diff for a specific provider
 */
export function useModelDiff(provider: string, options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: MODEL_META_KEYS.modelDiff(provider),
    queryFn: () => modelMetaService.getModelDiff(provider),
    enabled: !!provider && (options?.enabled ?? false),
    staleTime: 5 * 60 * 1000, // 5 minutes
    retry: false,
  });
}

/**
 * @hook useSyncProviderFull
 * @description Synchronizes a provider definition and all of its models.
 */
export function useSyncProviderFull() {
  const queryClient = useQueryClient();
  const t = useT('aiProviders');

  return useMutation({
    mutationFn: (provider: string) => modelMetaService.syncProviderFull(provider),
    onSuccess: res => {
      showSyncResultToast(res.data, t);
      invalidateProviderModelQueries(queryClient, res.data.provider);
    },
    onError: (error: unknown) => {
      toast.error(getModelMetaErrorMessage(error, t('sidebar.syncError')));
    },
  });
}

/**
 * @hook useSyncProviderModelsAction
 * @description Synchronizes all or selected models for a dynamically chosen provider.
 */
export function useSyncProviderModelsAction() {
  const queryClient = useQueryClient();
  const t = useT('aiProviders');

  return useMutation({
    mutationFn: ({ provider, models }: { provider: string; models?: string[] }) =>
      modelMetaService.syncModels(provider, models ? { models } : undefined),
    onSuccess: res => {
      showSyncResultToast(res.data, t);
      invalidateProviderModelQueries(queryClient, res.data.provider);
    },
    onError: (error: unknown) => {
      toast.error(getModelMetaErrorMessage(error, t('sidebar.syncError')));
    },
  });
}

/**
 * @hook useSyncModels
 * @description Synchronizes a selected set of models for a single provider.
 */
export function useSyncModels(provider: string) {
  const queryClient = useQueryClient();
  const t = useT('aiProviders');

  return useMutation({
    mutationFn: (models: string[]) => modelMetaService.syncModels(provider, { models }),
    onSuccess: res => {
      showSyncResultToast(res.data, t);
      invalidateProviderModelQueries(queryClient, provider);
    },
    onError: (error: unknown) => {
      toast.error(getModelMetaErrorMessage(error, t('sidebar.syncError')));
    },
  });
}
