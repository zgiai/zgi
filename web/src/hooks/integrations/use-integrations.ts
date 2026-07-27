'use client';

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import { AICHAT_KEYS, INTEGRATION_KEYS } from '@/hooks/query-keys';
import { integrationService } from '@/services/integration.service';
import { integrationErrorTranslationKeyFromError } from '@/services/integration-error-i18n';
import type {
  AcknowledgeIntegrationOAuthRecoveryRequest,
  CreateIntegrationConnectionRequest,
  GetIntegrationExecutionsParams,
  IntegrationActionPolicy,
  IntegrationCatalogItem,
  IntegrationConnection,
  IntegrationConnectionGrant,
  IntegrationConnectionHealthEvent,
  IntegrationExecution,
  SaveIntegrationConnectionGrantRequest,
  UpdateIntegrationOAuthClientConfigRequest,
  UpdateIntegrationActionPoliciesRequest,
  UpdateIntegrationConnectionRequest,
} from '@/services/types/integration';

export function integrationCatalogItems(
  value:
    | {
        data?: IntegrationCatalogItem[];
        items?: IntegrationCatalogItem[];
      }
    | null
    | undefined
): IntegrationCatalogItem[] {
  return value?.items ?? value?.data ?? [];
}

export function integrationConnectionItems(
  value:
    | {
        data?: IntegrationConnection[];
        items?: IntegrationConnection[];
      }
    | null
    | undefined
): IntegrationConnection[] {
  return value?.items ?? value?.data ?? [];
}

export function integrationExecutionItems(
  value:
    | {
        data?: IntegrationExecution[];
        items?: IntegrationExecution[];
      }
    | null
    | undefined
): IntegrationExecution[] {
  return value?.items ?? value?.data ?? [];
}

export function integrationConnectionGrantItems(
  value: { items?: IntegrationConnectionGrant[] } | null | undefined
): IntegrationConnectionGrant[] {
  return value?.items ?? [];
}

export function integrationConnectionHealthEventItems(
  value: { items?: IntegrationConnectionHealthEvent[] } | null | undefined
): IntegrationConnectionHealthEvent[] {
  return value?.items ?? [];
}

export function useIntegrationCatalog(
  enabled = true,
  audience: 'account' | 'shared' | 'organization' = 'account'
) {
  return useQuery({
    queryKey:
      audience === 'account'
        ? INTEGRATION_KEYS.catalog()
        : [...INTEGRATION_KEYS.catalog(), audience],
    queryFn: () => integrationService.getCatalog(audience),
    staleTime: 60_000,
    retry: false,
    enabled,
  });
}

export function useIntegrationProviderCapabilities(
  integrationId: string,
  audience: 'account' | 'organization' = 'account',
  enabled = true
) {
  return useQuery({
    queryKey: INTEGRATION_KEYS.capabilities(integrationId, audience),
    queryFn: () => integrationService.getProviderCapabilities(integrationId, audience),
    staleTime: 30_000,
    retry: false,
    enabled: enabled && Boolean(integrationId),
  });
}

export function useIntegrationConnections(params?: {
  integration_id?: string;
  status?: string;
  page?: number;
  limit?: number;
}) {
  return useQuery({
    queryKey: INTEGRATION_KEYS.connectionList(params),
    queryFn: () => integrationService.getConnections(params),
    staleTime: 30_000,
    retry: false,
  });
}

export function useAllIntegrationConnections(
  params?: {
    integration_id?: string;
    status?: string;
  },
  enabled = true
) {
  return useQuery({
    queryKey: INTEGRATION_KEYS.connectionList({ ...params, all: true }),
    queryFn: () => integrationService.getAllConnections(params),
    staleTime: 30_000,
    retry: false,
    enabled,
  });
}

export function useMyIntegrationConnections(
  params?: {
    integration_id?: string;
    page?: number;
    limit?: number;
  },
  enabled = true
) {
  return useQuery({
    queryKey: INTEGRATION_KEYS.myConnections(params),
    queryFn: () => integrationService.getMyConnections(params),
    staleTime: 30_000,
    retry: false,
    enabled,
  });
}

export function useAllMyIntegrationConnections(
  params?: {
    integration_id?: string;
  },
  enabled = true
) {
  return useQuery({
    queryKey: INTEGRATION_KEYS.myConnections({ ...params, all: true }),
    queryFn: () => integrationService.getAllMyConnections(params),
    staleTime: 30_000,
    retry: false,
    enabled,
  });
}

export function useIntegrationConnection(id: string, enabled = true) {
  return useQuery({
    queryKey: INTEGRATION_KEYS.connection(id),
    queryFn: () => integrationService.getConnection(id),
    enabled: enabled && Boolean(id),
    staleTime: 15_000,
    retry: false,
  });
}

export function useIntegrationConnectionGrants(id: string, enabled = true) {
  return useQuery({
    queryKey: INTEGRATION_KEYS.grants(id),
    queryFn: () => integrationService.getConnectionGrants(id),
    enabled: enabled && Boolean(id),
    staleTime: 15_000,
    retry: false,
  });
}

export function useIntegrationConnectionHealthEvents(
  id: string,
  params?: { page?: number; limit?: number },
  enabled = true
) {
  return useQuery({
    queryKey: INTEGRATION_KEYS.healthEvents(id, params),
    queryFn: () => integrationService.getConnectionHealthEvents(id, params),
    enabled: enabled && Boolean(id),
    staleTime: 15_000,
    retry: false,
  });
}

export function useIntegrationExecutions(params?: GetIntegrationExecutionsParams) {
  return useQuery({
    queryKey: INTEGRATION_KEYS.executions(params),
    queryFn: () => integrationService.getExecutions(params),
    staleTime: 15_000,
    retry: false,
  });
}

export function useIntegrationActionPolicies(integrationId: string) {
  return useQuery({
    queryKey: INTEGRATION_KEYS.policies(integrationId),
    queryFn: () => integrationService.getActionPolicies(integrationId),
    enabled: Boolean(integrationId),
    staleTime: 30_000,
    retry: false,
  });
}

export function useIntegrationOAuthClientConfig(
  integrationId: string,
  authMethodId: string,
  enabled = true,
  clientConfigId = authMethodId
) {
  return useQuery({
    queryKey: INTEGRATION_KEYS.oauthClientConfig(integrationId, clientConfigId),
    queryFn: () => integrationService.getOAuthClientConfig(integrationId, authMethodId),
    enabled: enabled && Boolean(integrationId && authMethodId),
    staleTime: 30_000,
    retry: false,
  });
}

export function useIntegrationOAuthClientConfigImpact(
  integrationId: string,
  authMethodId: string,
  enabled = true,
  clientConfigId = authMethodId
) {
  return useQuery({
    queryKey: INTEGRATION_KEYS.oauthClientConfigImpact(integrationId, clientConfigId),
    queryFn: () => integrationService.getOAuthClientConfigImpact(integrationId, authMethodId),
    enabled: enabled && Boolean(integrationId && authMethodId),
    staleTime: 10_000,
    retry: false,
  });
}

export function useIntegrationOAuthRecovery(enabled = true) {
  return useQuery({
    queryKey: INTEGRATION_KEYS.oauthRecovery(),
    queryFn: () => integrationService.getOAuthRecovery(),
    enabled,
    staleTime: 0,
    gcTime: 0,
    refetchOnMount: 'always',
    retry: false,
  });
}

export function useAcknowledgeIntegrationOAuthRecovery() {
  const queryClient = useQueryClient();
  const t = useT('integrations');
  return useMutation({
    mutationFn: ({
      operationRef,
      data,
    }: {
      operationRef: string;
      data: AcknowledgeIntegrationOAuthRecoveryRequest;
    }) => integrationService.acknowledgeOAuthRecovery(operationRef, data),
    onSuccess: () => {
      toast.success(t('messages.oauthRecoveryAcknowledged'));
      void queryClient.invalidateQueries({ queryKey: INTEGRATION_KEYS.oauthRecovery() });
    },
    onError: error => {
      const key = integrationErrorTranslationKeyFromError(error, 'integrations');
      toast.error(key ? t(key) : t('messages.requestFailed'));
    },
  });
}

export function useUpdateIntegrationOAuthClientConfig(
  integrationId: string,
  authMethodId: string,
  clientConfigId = authMethodId
) {
  const queryClient = useQueryClient();
  const t = useT('integrations');
  return useMutation({
    mutationFn: (data: UpdateIntegrationOAuthClientConfigRequest) =>
      integrationService.updateOAuthClientConfig(integrationId, authMethodId, data),
    onSuccess: () => {
      toast.success(t('messages.oauthClientConfigured'));
      void queryClient.invalidateQueries({
        queryKey: INTEGRATION_KEYS.oauthClientConfig(integrationId, clientConfigId),
      });
      void queryClient.invalidateQueries({
        queryKey: INTEGRATION_KEYS.oauthClientConfigImpact(integrationId, clientConfigId),
      });
      void queryClient.invalidateQueries({ queryKey: INTEGRATION_KEYS.catalog() });
    },
    onError: error => {
      const key = integrationErrorTranslationKeyFromError(error, 'integrations');
      toast.error(key ? t(key) : t('messages.requestFailed'));
    },
  });
}

export function useDeleteIntegrationOAuthClientConfig(
  integrationId: string,
  authMethodId: string,
  clientConfigId = authMethodId
) {
  const queryClient = useQueryClient();
  const t = useT('integrations');
  return useMutation({
    mutationFn: () => integrationService.deleteOAuthClientConfig(integrationId, authMethodId),
    onSuccess: () => {
      toast.success(t('messages.oauthClientRemoved'));
      void queryClient.invalidateQueries({
        queryKey: INTEGRATION_KEYS.oauthClientConfig(integrationId, clientConfigId),
      });
      void queryClient.invalidateQueries({
        queryKey: INTEGRATION_KEYS.oauthClientConfigImpact(integrationId, clientConfigId),
      });
      void queryClient.invalidateQueries({ queryKey: INTEGRATION_KEYS.catalog() });
    },
    onError: error => {
      const key = integrationErrorTranslationKeyFromError(error, 'integrations');
      toast.error(key ? t(key) : t('messages.requestFailed'));
    },
  });
}

function useConnectionMutation<TVariables>(
  mutationFn: (variables: TVariables) => Promise<unknown>,
  successKey: 'created' | 'updated' | 'tested' | 'defaultSet' | 'deleted'
) {
  const queryClient = useQueryClient();
  const t = useT('integrations');
  return useMutation({
    mutationFn,
    onSuccess: () => {
      toast.success(t(`messages.${successKey}`));
      void queryClient.invalidateQueries({ queryKey: INTEGRATION_KEYS.connections() });
      void queryClient.invalidateQueries({ queryKey: INTEGRATION_KEYS.myConnections() });
      void queryClient.invalidateQueries({
        queryKey: [...INTEGRATION_KEYS.all, 'available-connections'],
      });
      void queryClient.invalidateQueries({
        queryKey: [...AICHAT_KEYS.all, 'integration-preferences'],
      });
      void queryClient.invalidateQueries({ queryKey: INTEGRATION_KEYS.catalog() });
      void queryClient.invalidateQueries({ queryKey: INTEGRATION_KEYS.executions() });
    },
    onError: error => {
      const key = integrationErrorTranslationKeyFromError(error, 'integrations');
      toast.error(key ? t(key) : t('messages.requestFailed'));
    },
  });
}

function useMyConnectionMutation<TVariables>(
  mutationFn: (variables: TVariables) => Promise<unknown>,
  successKey: 'created' | 'updated' | 'tested' | 'deleted'
) {
  const queryClient = useQueryClient();
  const t = useT('integrations');
  return useMutation({
    mutationFn,
    onSuccess: () => {
      toast.success(t(`messages.${successKey}`));
      void queryClient.invalidateQueries({
        queryKey: [...INTEGRATION_KEYS.all, 'my-connections'],
      });
      void queryClient.invalidateQueries({
        queryKey: [...INTEGRATION_KEYS.all, 'available-connections'],
      });
      void queryClient.invalidateQueries({
        queryKey: [...AICHAT_KEYS.all, 'integration-preferences'],
      });
      void queryClient.invalidateQueries({ queryKey: INTEGRATION_KEYS.catalog() });
    },
    onError: error => {
      const key = integrationErrorTranslationKeyFromError(error, 'integrations');
      toast.error(key ? t(key) : t('messages.requestFailed'));
    },
  });
}

export function useCreateIntegrationConnection() {
  return useConnectionMutation<CreateIntegrationConnectionRequest>(
    data => integrationService.createConnection(data),
    'created'
  );
}

export function useCreateMyIntegrationConnection() {
  return useMyConnectionMutation<CreateIntegrationConnectionRequest>(
    data => integrationService.createMyConnection(data),
    'created'
  );
}

export function useUpdateIntegrationConnection() {
  return useConnectionMutation<{ id: string; data: UpdateIntegrationConnectionRequest }>(
    ({ id, data }) => integrationService.updateConnection(id, data),
    'updated'
  );
}

export function useUpdateMyIntegrationConnection() {
  return useMyConnectionMutation<{ id: string; data: UpdateIntegrationConnectionRequest }>(
    ({ id, data }) => integrationService.updateMyConnection(id, data),
    'updated'
  );
}

export function useTestIntegrationConnection() {
  return useConnectionMutation<string>(id => integrationService.testConnection(id), 'tested');
}

export function useTestMyIntegrationConnection() {
  return useMyConnectionMutation<string>(id => integrationService.testMyConnection(id), 'tested');
}

export function useSetDefaultIntegrationConnection() {
  return useConnectionMutation<string>(
    id => integrationService.setDefaultConnection(id),
    'defaultSet'
  );
}

export function useDeleteIntegrationConnection() {
  return useConnectionMutation<string>(id => integrationService.deleteConnection(id), 'deleted');
}

export function useDeleteMyIntegrationConnection() {
  return useMyConnectionMutation<string>(
    id => integrationService.deleteMyConnection(id),
    'deleted'
  );
}

function useConnectionGrantMutation<TVariables>(
  connectionId: string,
  mutationFn: (variables: TVariables) => Promise<unknown>,
  successKey: 'grantSaved' | 'grantDeleted'
) {
  const queryClient = useQueryClient();
  const t = useT('integrations');
  return useMutation({
    mutationFn,
    onSuccess: () => {
      toast.success(t(`messages.${successKey}`));
      void queryClient.invalidateQueries({ queryKey: INTEGRATION_KEYS.grants(connectionId) });
      void queryClient.invalidateQueries({ queryKey: INTEGRATION_KEYS.myConnections() });
      void queryClient.invalidateQueries({ queryKey: INTEGRATION_KEYS.connections() });
      void queryClient.invalidateQueries({
        queryKey: [...INTEGRATION_KEYS.all, 'available-connections'],
      });
      void queryClient.invalidateQueries({
        queryKey: [...AICHAT_KEYS.all, 'integration-preferences'],
      });
    },
    onError: error => {
      const key = integrationErrorTranslationKeyFromError(error, 'integrations');
      toast.error(key ? t(key) : t('messages.requestFailed'));
    },
  });
}

export function useCreateIntegrationConnectionGrant(connectionId: string) {
  return useConnectionGrantMutation<SaveIntegrationConnectionGrantRequest>(
    connectionId,
    data => integrationService.createConnectionGrant(connectionId, data),
    'grantSaved'
  );
}

export function useUpdateIntegrationConnectionGrant(connectionId: string) {
  return useConnectionGrantMutation<{
    grantId: string;
    data: SaveIntegrationConnectionGrantRequest;
  }>(
    connectionId,
    ({ grantId, data }) => integrationService.updateConnectionGrant(connectionId, grantId, data),
    'grantSaved'
  );
}

export function useDeleteIntegrationConnectionGrant(connectionId: string) {
  return useConnectionGrantMutation<string>(
    connectionId,
    grantId => integrationService.deleteConnectionGrant(connectionId, grantId),
    'grantDeleted'
  );
}

export function useUpdateIntegrationActionPolicies(integrationId: string) {
  const queryClient = useQueryClient();
  const t = useT('integrations');
  return useMutation({
    mutationFn: ({
      revision,
      policies,
    }: {
      revision: string;
      policies: IntegrationActionPolicy[];
    }) => {
      const data: UpdateIntegrationActionPoliciesRequest = {
        revision,
        policies: policies.map(policy => ({
          action_id: policy.action_id,
          enabled: policy.enabled,
          approval_policy: policy.approval_policy,
          data_egress_allowed: policy.data_egress_allowed,
        })),
      };
      return integrationService.updateActionPolicies(integrationId, data);
    },
    onSuccess: () => {
      toast.success(t('messages.policiesSaved'));
      void queryClient.invalidateQueries({ queryKey: INTEGRATION_KEYS.policies(integrationId) });
    },
    onError: error => {
      const key = integrationErrorTranslationKeyFromError(error, 'integrations');
      toast.error(key ? t(key) : t('messages.requestFailed'));
    },
  });
}
