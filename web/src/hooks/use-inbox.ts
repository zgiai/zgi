'use client';

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { INBOX_KEYS } from '@/hooks/query-keys';
import { inboxService } from '@/services/inbox.service';

const UNREAD_POLL_MS = 60_000;

export function useInboxUnreadCount() {
  return useQuery({
    queryKey: INBOX_KEYS.unread(),
    queryFn: () => inboxService.unreadCount(),
    refetchInterval: UNREAD_POLL_MS,
    refetchOnWindowFocus: true,
  });
}

export function useInboxList(params: { unread_only?: boolean; page?: number } = {}, enabled = true) {
  return useQuery({
    queryKey: INBOX_KEYS.list(params),
    queryFn: () => inboxService.list({ ...params, page_size: 20 }),
    enabled,
  });
}

export function useInboxMutations() {
  const client = useQueryClient();
  const refresh = () => client.invalidateQueries({ queryKey: INBOX_KEYS.all });
  const markRead = useMutation({
    mutationFn: (request: { ids: string[] } | { source_type: string; source_id: string }) => inboxService.markRead(request),
    onSuccess: refresh,
  });
  const markAllRead = useMutation({ mutationFn: () => inboxService.markAllRead(), onSuccess: refresh });
  return { markRead, markAllRead };
}
