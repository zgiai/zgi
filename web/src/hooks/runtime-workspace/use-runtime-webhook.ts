'use client';

import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import { runtimeWorkspaceService } from '@/services/runtime-workspace.service';
import type { RuntimeWebhookInput } from '@/services/types/runtime-workspace';
import { RUNTIME_WORKSPACE_KEYS } from '@/hooks/query-keys';
import { getErrorMessage } from '@/utils/error-notifications';

/** A Runtime Workspace's webhook endpoint and its recent deliveries (ADR 0011). */
export function useRuntimeWebhook(runtimeWorkspaceId: string | null) {
  const queryClient = useQueryClient();
  const invalidate = () =>
    queryClient.invalidateQueries({ queryKey: RUNTIME_WORKSPACE_KEYS.webhook(runtimeWorkspaceId) });
  const onError = (error: unknown) => {
    toast.error(getErrorMessage(error));
  };
  const webhook = useQuery({
    queryKey: RUNTIME_WORKSPACE_KEYS.webhook(runtimeWorkspaceId),
    queryFn: () => runtimeWorkspaceService.getWebhook(runtimeWorkspaceId as string),
    enabled: Boolean(runtimeWorkspaceId),
    retry: false,
  });
  const deliveryPages = useInfiniteQuery({
    queryKey: RUNTIME_WORKSPACE_KEYS.webhookDeliveries(runtimeWorkspaceId),
    queryFn: ({ pageParam }) =>
      runtimeWorkspaceService.listWebhookDeliveries(runtimeWorkspaceId as string, pageParam),
    initialPageParam: 1,
    getNextPageParam: last => (last.has_more ? last.page + 1 : undefined),
    enabled: Boolean(runtimeWorkspaceId && webhook.data?.configured),
    refetchInterval: 10_000,
    retry: false,
  });
  const deliveries = {
    data: deliveryPages.data?.pages.flatMap(page => page.items),
    hasMore: Boolean(deliveryPages.hasNextPage),
    loadingMore: deliveryPages.isFetchingNextPage,
    loadMore: () => void deliveryPages.fetchNextPage(),
  };
  const save = useMutation({
    mutationFn: (input: RuntimeWebhookInput) =>
      runtimeWorkspaceService.saveWebhook(runtimeWorkspaceId as string, input),
    onSuccess: () => void invalidate(),
    onError,
  });
  const remove = useMutation({
    mutationFn: () => runtimeWorkspaceService.deleteWebhook(runtimeWorkspaceId as string),
    onSuccess: () => void invalidate(),
    onError,
  });
  const rotate = useMutation({
    mutationFn: () => runtimeWorkspaceService.rotateWebhookSecret(runtimeWorkspaceId as string),
    onError,
  });
  const test = useMutation({
    mutationFn: () => runtimeWorkspaceService.testWebhook(runtimeWorkspaceId as string),
    onSuccess: () => void invalidate(),
    onError,
  });
  const retry = useMutation({
    mutationFn: (deliveryId: string) =>
      runtimeWorkspaceService.retryWebhookDelivery(runtimeWorkspaceId as string, deliveryId),
    onSuccess: () => void invalidate(),
    onError,
  });
  return { webhook, deliveries, save, remove, rotate, test, retry };
}
