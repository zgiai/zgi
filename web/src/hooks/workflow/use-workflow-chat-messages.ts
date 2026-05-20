'use client';

import { useMemo, useEffect, useCallback } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import { workflowService } from '@/services/workflow.service';
import type { ApiResponseData } from '@/services/types/common';
import type {
  WorkflowChatMessageItem,
  WorkflowChatMessagesList,
  WorkflowChatMessagesQuery,
} from '@/services/types/workflow';
import { WORKFLOW_KEYS } from '@/hooks/query-keys';
import { normalizeToastDescription } from '@/utils/error-notifications';

export interface UseWorkflowChatMessagesParams {
  agentId: string | null | undefined;
  conversationId: string | null | undefined;
  page?: number;
  limit?: number;
}

export interface UseWorkflowChatMessagesOptions {
  enabled?: boolean;
  staleTime?: number;
  gcTime?: number;
  refetchOnWindowFocus?: boolean;
}

export interface UseWorkflowChatMessagesReturn {
  messages: WorkflowChatMessageItem[];
  pagination: Omit<WorkflowChatMessagesList, 'data'> | null;
  isLoading: boolean;
  isFetching: boolean;
  error: string | null;
  reload: () => Promise<void>;
}

export function useWorkflowChatMessages(
  { agentId, conversationId, page = 1, limit = 20 }: UseWorkflowChatMessagesParams,
  {
    enabled = true,
    staleTime = 60_000,
    gcTime = 10 * 60_000,
    refetchOnWindowFocus = false,
  }: UseWorkflowChatMessagesOptions = {}
): UseWorkflowChatMessagesReturn {
  const t = useT('agents');
  const queryClient = useQueryClient();

  const params = useMemo<WorkflowChatMessagesQuery>(
    () => ({
      conversation_id: conversationId ?? '',
      page,
      limit,
    }),
    [conversationId, page, limit]
  );

  const queryKey = useMemo(
    () => WORKFLOW_KEYS.chatMessages(agentId ?? 'none', conversationId ?? 'none', params),
    [agentId, conversationId, params]
  );

  const isEnabled = Boolean(agentId && conversationId) && enabled;

  const { data, isLoading, isFetching, error } = useQuery<
    ApiResponseData<WorkflowChatMessagesList>
  >({
    queryKey,
    queryFn: () => workflowService.getWorkflowChatMessages(agentId as string, params),
    enabled: isEnabled,
    staleTime,
    gcTime,
    refetchOnWindowFocus,
    retry: false,
  });

  useEffect(() => {
    if (error) {
      const title = t('workflow.errors.loadChatMessagesFailed');
      toast.error(title, {
        description: normalizeToastDescription(title, (error as Error)?.message),
      });
    }
  }, [error, t]);

  const reload = useCallback(async () => {
    await queryClient.invalidateQueries({ queryKey });
  }, [queryClient, queryKey]);

  return {
    messages: data?.data?.data ?? [],
    pagination: data?.data
      ? {
          page: data.data.page,
          limit: data.data.limit,
          total: data.data.total,
          has_more: data.data.has_more,
        }
      : null,
    isLoading,
    isFetching,
    error: (error as Error | null)?.message ?? null,
    reload,
  };
}
