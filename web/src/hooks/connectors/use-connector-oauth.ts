'use client';

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useEffect } from 'react';
import { CONNECTOR_KEYS } from '@/hooks/query-keys';
import { connectorService } from '@/services/connector.service';
import { isOAuthFlowActive } from '@/components/connectors/setup/social-oauth-state';
import { invalidatePersonalConnectionQueries } from './connection-detail-query';

export function useConnectorOAuthFlow(flowId?: string) {
  const queryClient = useQueryClient();
  const query = useQuery({
    queryKey: [...CONNECTOR_KEYS.all, 'oauth-flow', flowId],
    queryFn: () => connectorService.getOAuthFlow(flowId ?? ''),
    enabled: Boolean(flowId),
    retry: false,
    refetchInterval: query =>
      !query.state.error && isOAuthFlowActive(query.state.data?.data) ? 1500 : false,
  });
  const flow = query.data?.data;
  useEffect(() => {
    if (flow?.status === 'ready') invalidatePersonalConnectionQueries(queryClient);
  }, [flow?.id, flow?.status, queryClient]);
  const selectPage = useMutation({
    mutationFn: (pageId: string) => connectorService.selectOAuthPage(flowId ?? '', pageId),
    // The opener and callback window can submit the same Page concurrently.
    // Re-read the shared result before presenting a selection failure.
    onError: async () => {
      await query.refetch();
    },
    onSuccess: response =>
      queryClient.setQueryData([...CONNECTOR_KEYS.all, 'oauth-flow', flowId], response),
  });
  return { query, flow, selectPage };
}
