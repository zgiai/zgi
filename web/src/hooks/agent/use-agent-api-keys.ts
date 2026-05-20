'use client';

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { agentService } from '@/services';
import type {
  AgentApiKey,
  CreateAgentApiKeyRequest,
  UpdateAgentApiKeyRequest,
} from '@/services/types/agent';
import { toast } from 'sonner';
import { getErrorMessage } from '@/utils/error-notifications';
import { useT } from '@/i18n';

import { AGENT_KEYS } from '@/hooks/query-keys';

const getApiKeysKey = (agentId: string) => [...AGENT_KEYS.all, 'api-keys', agentId] as const;

export interface UseAgentApiKeysOptions {
  enabled?: boolean;
  staleTime?: number;
  gcTime?: number;
  refetchOnWindowFocus?: boolean;
  refetchInterval?: number | false;
}

export function useAgentApiKeys(
  agentId: string | null,
  {
    enabled = true,
    staleTime = 5 * 60 * 1000,
    gcTime = 10 * 60 * 1000,
    refetchOnWindowFocus = false,
    refetchInterval = false,
  }: UseAgentApiKeysOptions = {}
) {
  const query = useQuery({
    queryKey: getApiKeysKey(agentId || ''),
    queryFn: () => (agentId ? agentService.getAgentApiKeys(agentId) : Promise.resolve(null)),
    enabled: enabled && !!agentId,
    staleTime,
    gcTime,
    refetchOnWindowFocus,
    refetchInterval,
  });
  const keys = (query.data?.data?.api_keys ?? []) as AgentApiKey[];

  return {
    keys,
    isLoading: query.isLoading,
    isFetching: query.isFetching,
    error: query.error,
    refetch: query.refetch,
  };
}

export function useCreateAgentApiKey(agentId: string) {
  const queryClient = useQueryClient();
  const t = useT('agents');

  return useMutation({
    mutationFn: (data: CreateAgentApiKeyRequest) => agentService.createAgentApiKey(agentId, data),
    onSuccess: () => {
      toast.success(t('apiKeys.toasts.createSuccess'));
      queryClient.invalidateQueries({ queryKey: getApiKeysKey(agentId) });
    },
    onError: error => {
      toast.error(getErrorMessage(error) || t('apiKeys.toasts.createFailed'));
    },
  });
}

export function useUpdateAgentApiKey(agentId: string, keyId: string) {
  const queryClient = useQueryClient();
  const t = useT('agents');

  return useMutation({
    mutationFn: (data: UpdateAgentApiKeyRequest) =>
      agentService.updateAgentApiKey(agentId, keyId, data),
    onSuccess: () => {
      toast.success(t('apiKeys.toasts.updateSuccess'));
      queryClient.invalidateQueries({ queryKey: getApiKeysKey(agentId) });
    },
    onError: error => {
      toast.error(getErrorMessage(error) || t('apiKeys.toasts.updateFailed'));
    },
  });
}

export function useDeleteAgentApiKey(agentId: string) {
  const queryClient = useQueryClient();
  const t = useT('agents');

  return useMutation({
    mutationFn: (keyId: string) => agentService.deleteAgentApiKey(agentId, keyId),
    onSuccess: () => {
      toast.success(t('apiKeys.toasts.deleteSuccess'));
      queryClient.invalidateQueries({ queryKey: getApiKeysKey(agentId) });
    },
    onError: error => {
      toast.error(getErrorMessage(error) || t('apiKeys.toasts.deleteFailed'));
    },
  });
}
