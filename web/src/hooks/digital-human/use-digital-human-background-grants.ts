'use client';

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  backgroundGrantQueryKey,
  type BackgroundGrantScope,
} from '@/components/digital-human/digital-human-background-grants-model';
import { connectorService } from '@/services/connector.service';
import type { IssueConnectorBackgroundGrantRequest } from '@/services/types/connector';

const disabledBackgroundGrantKey = ['connector-background-grants', 'disabled'] as const;

export function useDigitalHumanBackgroundGrants(
  scope: BackgroundGrantScope | undefined,
  enabled: boolean
) {
  const queryClient = useQueryClient();
  const queryKey = scope ? backgroundGrantQueryKey(scope) : disabledBackgroundGrantKey;
  const query = useQuery({
    queryKey,
    queryFn: () => connectorService.getBackgroundGrants(),
    enabled: enabled && Boolean(scope),
    staleTime: 10_000,
    retry: false,
  });
  const issue = useMutation({
    mutationFn: ({
      bindingId,
      request,
    }: {
      bindingId: string;
      request: IssueConnectorBackgroundGrantRequest;
    }) => connectorService.issueBackgroundGrant(bindingId, request),
    retry: false,
    onSuccess: () => queryClient.invalidateQueries({ queryKey }),
  });
  const revoke = useMutation({
    mutationFn: (grantId: string) => connectorService.revokeBackgroundGrant(grantId),
    retry: false,
    onSuccess: () => queryClient.invalidateQueries({ queryKey }),
  });
  return { query, issue, revoke, queryKey };
}
