'use client';

import { useQuery } from '@tanstack/react-query';
import { useAuthStore } from '@/store/auth-store';
import { useOrganizationStore } from '@/store/organization-store';
import { connectorService } from '@/services/connector.service';

export const pendingSetupKeys = ['connectors', 'pending-setups'] as const;
export function usePendingSetups(connectorId?: string, enabled = true) {
  const owner = useAuthStore.use.user()?.id;
  const org = useOrganizationStore.use.currentOrganization()?.id;
  return useQuery({
    queryKey: [...pendingSetupKeys, org, owner, 'list', connectorId],
    queryFn: () => connectorService.getPendingSetups(connectorId),
    enabled: enabled && Boolean(owner && org) && (!connectorId || connectorId === 'feishu'),
    staleTime: 0,
    retry: false,
  });
}
export function usePendingSetup(id?: string) {
  const owner = useAuthStore.use.user()?.id;
  const org = useOrganizationStore.use.currentOrganization()?.id;
  return useQuery({
    queryKey: [...pendingSetupKeys, org, owner, 'detail', id],
    queryFn: () => connectorService.getPendingSetup(id ?? ''),
    enabled: Boolean(id && owner && org),
    staleTime: 0,
    retry: false,
  });
}
