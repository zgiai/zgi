'use client';

import { useCallback, useMemo } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { promptService } from '@/services/prompt.service';
import type { ApiResponseData } from '@/services/types/common';
import type {
  AdoptPromptOptimizationRunRequest,
  CreatePromptRequest,
  PromptDetail,
  PromptListParams,
  PromptOptimizeRequest,
  PromptSummary,
  PromptOptimizationRun,
  PromptOptimizationRunListParams,
  PromptUsageSummary,
  PromptVersionPayload,
  SetPromptLabelsRequest,
  UpdatePromptRequest,
} from '@/services/types/prompt';
import { PROMPT_KEYS } from '@/hooks/query-keys';
import { getErrorMessage } from '@/utils/error-notifications';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import { useLocale } from '@/hooks/use-locale';
import { localizePromptDetail, localizePromptSummary } from './prompt-localization';

export function usePrompts(params: PromptListParams = {}, enabled = true) {
  const { locale } = useLocale();
  const query = useQuery({
    queryKey: PROMPT_KEYS.list(params),
    queryFn: () => promptService.listPrompts(params),
    enabled,
    staleTime: 60 * 1000,
  });

  const prompts = useMemo<PromptSummary[]>(
    () => (query.data?.data?.data ?? []).map(prompt => localizePromptSummary(prompt, locale)),
    [locale, query.data]
  );
  const pageData = useMemo(() => {
    if (!query.data?.data) {
      return undefined;
    }
    return {
      ...query.data.data,
      data: prompts,
    };
  }, [prompts, query.data]);

  return {
    prompts,
    pageData,
    isLoading: query.isLoading,
    isFetching: query.isFetching,
    error: query.error ? getErrorMessage(query.error) : null,
    refetch: query.refetch,
  };
}

export function usePrompt(promptId?: string, enabled = true) {
  const { locale } = useLocale();
  const query = useQuery({
    queryKey: PROMPT_KEYS.detail(promptId || ''),
    queryFn: () => promptService.getPrompt(promptId || ''),
    enabled: enabled && !!promptId,
    staleTime: 60 * 1000,
  });

  const prompt = useMemo(
    () => (query.data?.data ? localizePromptDetail(query.data.data as PromptDetail, locale) : undefined),
    [locale, query.data]
  );

  return {
    prompt,
    isLoading: query.isLoading,
    isFetching: query.isFetching,
    error: query.error ? getErrorMessage(query.error) : null,
    refetch: query.refetch,
  };
}

export function usePromptUsage(promptId?: string, enabled = true) {
  const query = useQuery({
    queryKey: PROMPT_KEYS.usage(promptId || ''),
    queryFn: () => promptService.getPromptUsage(promptId || ''),
    enabled: enabled && !!promptId,
    staleTime: 30 * 1000,
  });

  return {
    usage: query.data?.data as PromptUsageSummary | undefined,
    isLoading: query.isLoading,
    isFetching: query.isFetching,
    error: query.error ? getErrorMessage(query.error) : null,
    refetch: query.refetch,
  };
}

export function useCreatePrompt() {
  const queryClient = useQueryClient();
  const t = useT('prompts');

  return useMutation({
    mutationFn: (data: CreatePromptRequest) => promptService.createPrompt(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: PROMPT_KEYS.all });
      toast.success(t('messages.createSuccess'));
    },
    onError: err => {
      toast.error(getErrorMessage(err) || t('messages.createFailed'));
    },
  });
}

export function useUpdatePrompt(promptId: string) {
  const queryClient = useQueryClient();
  const t = useT('prompts');

  return useMutation({
    mutationFn: (data: UpdatePromptRequest) => promptService.updatePrompt(promptId, data),
    onSuccess: data => {
      queryClient.invalidateQueries({ queryKey: PROMPT_KEYS.all });
      queryClient.setQueryData<ApiResponseData<PromptDetail>>(PROMPT_KEYS.detail(promptId), data);
      toast.success(t('messages.updateSuccess'));
    },
    onError: err => {
      toast.error(getErrorMessage(err) || t('messages.updateFailed'));
    },
  });
}

export function useCreatePromptVersion(promptId: string) {
  const queryClient = useQueryClient();
  const t = useT('prompts');

  return useMutation({
    mutationFn: (data: PromptVersionPayload) => promptService.createPromptVersion(promptId, data),
    onSuccess: data => {
      queryClient.invalidateQueries({ queryKey: PROMPT_KEYS.all });
      queryClient.setQueryData<ApiResponseData<PromptDetail>>(PROMPT_KEYS.detail(promptId), data);
      toast.success(t('messages.versionCreateSuccess'));
    },
    onError: err => {
      toast.error(getErrorMessage(err) || t('messages.versionCreateFailed'));
    },
  });
}

export function useSetPromptLabels(promptId: string) {
  const queryClient = useQueryClient();
  const t = useT('prompts');

  return useMutation({
    mutationFn: (data: SetPromptLabelsRequest) => promptService.setPromptLabels(promptId, data),
    onSuccess: data => {
      queryClient.invalidateQueries({ queryKey: PROMPT_KEYS.all });
      queryClient.setQueryData<ApiResponseData<PromptDetail>>(PROMPT_KEYS.detail(promptId), data);
      toast.success(t('messages.labelUpdateSuccess'));
    },
    onError: err => {
      toast.error(getErrorMessage(err) || t('messages.labelUpdateFailed'));
    },
  });
}

export function usePromptContentHelpers() {
  const parsePromptContent = useCallback((content: PromptDetail['versions'][number]['content']) => {
    if (typeof content === 'string') {
      return { type: 'text' as const, text: content };
    }
    return { type: 'chat' as const, messages: content };
  }, []);

  return { parsePromptContent };
}

export function useOptimizePrompt() {
  const queryClient = useQueryClient();
  const t = useT('prompts');

  return useMutation({
    mutationFn: (data: PromptOptimizeRequest) => promptService.optimizePrompt(data),
    onSuccess: (_data, variables) => {
      if (variables.prompt_id) {
        queryClient.invalidateQueries({
          queryKey: PROMPT_KEYS.optimizationRuns(variables.prompt_id),
        });
      }
    },
    onError: err => {
      toast.error(getErrorMessage(err) || t('messages.optimizerRunFailed'));
    },
  });
}

export function usePromptOptimizationRuns(
  promptId?: string,
  params: PromptOptimizationRunListParams = {},
  enabled: boolean = true
) {
  const query = useQuery({
    queryKey: PROMPT_KEYS.optimizationRuns(promptId || '', params),
    queryFn: () => promptService.listOptimizationRuns(promptId || '', params),
    enabled: enabled && !!promptId,
    staleTime: 30 * 1000,
  });

  return {
    runs: (query.data?.data?.data ?? []) as PromptOptimizationRun[],
    pageData: query.data?.data,
    isLoading: query.isLoading,
    isFetching: query.isFetching,
    error: query.error ? getErrorMessage(query.error) : null,
    refetch: query.refetch,
  };
}

export function useAdoptPromptOptimizationRun(promptId: string) {
  const queryClient = useQueryClient();
  const t = useT('prompts');

  return useMutation({
    mutationFn: ({ runId, payload }: { runId: string; payload: AdoptPromptOptimizationRunRequest }) =>
      promptService.adoptOptimizationRun(promptId, runId, payload),
    onSuccess: data => {
      queryClient.invalidateQueries({ queryKey: PROMPT_KEYS.optimizationRuns(promptId) });
      queryClient.setQueryData<ApiResponseData<PromptDetail>>(PROMPT_KEYS.detail(promptId), data);
      toast.success(t('messages.optimizerAdoptSuccess'));
    },
    onError: err => {
      toast.error(getErrorMessage(err) || t('messages.optimizerAdoptFailed'));
    },
  });
}
