'use client';

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { CONNECTOR_KEYS } from '@/hooks/query-keys';
import { connectorService } from '@/services/connector.service';
import {
  invalidatePersonalConnectionQueries,
  personalConnectionDetailQueryOptions,
  requirePersonalConnectionDetailResponse,
  requirePersonalConnectionRemovalImpactResponse,
} from './connection-detail-query';
import type {
  ConnectorCatalogItem,
  ConnectorCatalogResponse,
  ConnectorTargetBinding,
  ConnectorTargetBindingListResponse,
  ConnectorTargetRef,
  GetConnectorCatalogParams,
  GetPersonalConnectionsParams,
  PersonalConnection,
  PersonalConnectionListResponse,
  RemovePersonalConnectionOptions,
  RenamePersonalConnectionRequest,
} from '@/services/types/connector';

export function connectorCatalogItems(
  value: ConnectorCatalogResponse | null | undefined
): ConnectorCatalogItem[] {
  return value?.items ?? [];
}

export function personalConnectionItems(
  value: PersonalConnectionListResponse | null | undefined
): PersonalConnection[] {
  return value?.items ?? [];
}

export function connectorTargetBindingItems(
  value: ConnectorTargetBindingListResponse | null | undefined
): ConnectorTargetBinding[] {
  return value?.items ?? [];
}

export function useConnectorCatalog(params?: GetConnectorCatalogParams, enabled = true) {
  return useQuery({
    queryKey: CONNECTOR_KEYS.catalogList(params),
    queryFn: () => connectorService.getCatalog(params),
    enabled,
    staleTime: 60_000,
    retry: false,
  });
}

export function useConnector(connectorId: string, enabled = true) {
  return useQuery({
    queryKey: CONNECTOR_KEYS.definition(connectorId),
    queryFn: () => connectorService.getConnector(connectorId),
    enabled: enabled && Boolean(connectorId),
    staleTime: 60_000,
    retry: false,
  });
}

export function usePersonalConnections(params?: GetPersonalConnectionsParams, enabled = true) {
  return useQuery({
    queryKey: CONNECTOR_KEYS.connectionList(params),
    queryFn: () => connectorService.getConnections(params),
    enabled,
    staleTime: 30_000,
    retry: false,
  });
}

export function usePersonalConnection(connectionId: string, enabled = true) {
  return useQuery(
    personalConnectionDetailQueryOptions(
      connectionId,
      () => connectorService.getConnection(connectionId),
      enabled
    )
  );
}

export function useDiagnosePersonalConnection(connectionId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async () =>
      requirePersonalConnectionDetailResponse(
        await connectorService.diagnoseConnection(connectionId),
        connectionId
      ),
    onSuccess: response => {
      queryClient.setQueryData(CONNECTOR_KEYS.connection(connectionId), response);
      void queryClient.invalidateQueries({ queryKey: CONNECTOR_KEYS.catalog() });
      void queryClient.invalidateQueries({ queryKey: CONNECTOR_KEYS.connectionList() });
      void queryClient.invalidateQueries({ queryKey: [...CONNECTOR_KEYS.all, 'chat-preferences'] });
    },
  });
}

export function useRenamePersonalConnection(connectionId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (data: RenamePersonalConnectionRequest) =>
      connectorService.renameConnection(connectionId, data),
    onSuccess: () => {
      invalidatePersonalConnectionQueries(queryClient);
    },
  });
}

export function usePersonalConnectionRemovalImpact(connectionId: string, enabled = true) {
  return useQuery({
    queryKey: CONNECTOR_KEYS.removalImpact(connectionId),
    queryFn: async () =>
      requirePersonalConnectionRemovalImpactResponse(
        await connectorService.getRemovalImpact(connectionId),
        connectionId
      ),
    select: response => requirePersonalConnectionRemovalImpactResponse(response, connectionId),
    enabled: enabled && Boolean(connectionId),
    staleTime: 0,
    retry: false,
  });
}

export function useRemovePersonalConnection(connectionId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (options?: RemovePersonalConnectionOptions) =>
      connectorService.removeConnection(connectionId, options),
    onSuccess: () => {
      queryClient.removeQueries({ queryKey: CONNECTOR_KEYS.connection(connectionId) });
      void queryClient.invalidateQueries({ queryKey: [...CONNECTOR_KEYS.all, 'chat-preferences'] });
      void queryClient.invalidateQueries({ queryKey: CONNECTOR_KEYS.catalog() });
      void queryClient.invalidateQueries({ queryKey: CONNECTOR_KEYS.connectionList() });
      void queryClient.invalidateQueries({
        queryKey: [...CONNECTOR_KEYS.all, 'target-bindings'],
      });
    },
  });
}

export function useConnectorTargetBindings(target: ConnectorTargetRef | undefined, enabled = true) {
  return useQuery({
    queryKey: CONNECTOR_KEYS.targetBindings(target),
    queryFn: () => connectorService.getTargetBindings(target as ConnectorTargetRef),
    enabled: enabled && Boolean(target?.id),
    staleTime: 10_000,
    retry: false,
  });
}

export function useUpdateConnectorTargetBindingHostDelegation() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ bindingId, revision, audiences }: {
      bindingId: string;
      revision: number;
      audiences: Array<'workspace' | 'api' | 'channel'>;
    }) => connectorService.updateTargetBindingHostDelegation(bindingId, { revision, audiences }),
    onSuccess: response => {
      const binding = response.data;
      if (binding?.target) {
        void queryClient.invalidateQueries({
          queryKey: CONNECTOR_KEYS.targetBindings(binding.target),
        });
      } else {
        void queryClient.invalidateQueries({ queryKey: [...CONNECTOR_KEYS.all, 'target-bindings'] });
      }
    },
  });
}
