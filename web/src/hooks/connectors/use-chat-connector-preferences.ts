'use client';

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useAuthStore } from '@/store/auth-store';
import { useOrganizationStore } from '@/store/organization-store';
import { connectorService } from '@/services/connector.service';
import type { ApiResponseData } from '@/services/types/common';
import type {
  ConnectorChatPreferences,
  SetConnectorChatPreferenceRequest,
} from '@/services/types/connector';

export const CHAT_CONNECTOR_PREFERENCE_KEYS = {
  all: ['connectors', 'chat-preferences'] as const,
  scope: (organizationID?: string, accountID?: string) =>
    [...CHAT_CONNECTOR_PREFERENCE_KEYS.all, organizationID ?? '', accountID ?? ''] as const,
};

/** A missing or malformed payload must not be interpreted as no saved defaults. */
export function requireChatConnectorPreferences(
  response: ApiResponseData<ConnectorChatPreferences>
): ApiResponseData<ConnectorChatPreferences> {
  const data = response?.data;
  if (
    !data ||
    !Number.isSafeInteger(data.revision) ||
    data.revision < 0 ||
    !data.defaults ||
    typeof data.defaults !== 'object' ||
    Array.isArray(data.defaults) ||
    Object.keys(data.defaults).length > 64 ||
    Object.values(data.defaults).some(
      value =>
        !value ||
        (value.mode !== 'none' && value.mode !== 'selected') ||
        (value.mode === 'selected' &&
          (typeof value.connection_id !== 'string' ||
            !value.connection_id ||
            typeof value.available !== 'boolean'))
    )
  ) {
    throw new Error('Connector chat preferences are incomplete');
  }
  return response;
}

function usePreferenceScope() {
  const account = useAuthStore.use.user();
  const organization = useOrganizationStore.use.currentOrganization();
  return { accountID: account?.id, organizationID: organization?.id };
}

function requireCurrentScope(organizationID?: string, accountID?: string) {
  if (
    !organizationID ||
    !accountID ||
    useAuthStore.getState().user?.id !== accountID ||
    useOrganizationStore.getState().currentOrganization?.id !== organizationID
  ) {
    throw new Error('Connector preference scope changed');
  }
}

export function useChatConnectorPreferences(enabled = true) {
  const queryClient = useQueryClient();
  const { accountID, organizationID } = usePreferenceScope();
  const queryKey = CHAT_CONNECTOR_PREFERENCE_KEYS.scope(organizationID, accountID);
  const query = useQuery({
    queryKey,
    queryFn: async () => {
      requireCurrentScope(organizationID, accountID);
      const response = await connectorService.getChatPreferences();
      requireCurrentScope(organizationID, accountID);
      const validated = requireChatConnectorPreferences(response);
      // An earlier GET can finish after a successful PUT has already advanced the cache.
      const cached = queryClient.getQueryData<ApiResponseData<ConnectorChatPreferences>>(queryKey);
      return cached && cached.data.revision > validated.data.revision ? cached : validated;
    },
    enabled: enabled && Boolean(accountID && organizationID),
    staleTime: 0,
    retry: false,
  });
  return { ...query, scope: JSON.stringify([organizationID ?? '', accountID ?? '']) };
}

export function useSetChatConnectorPreference() {
  const queryClient = useQueryClient();
  const { accountID, organizationID } = usePreferenceScope();
  const queryKey = CHAT_CONNECTOR_PREFERENCE_KEYS.scope(organizationID, accountID);
  return useMutation({
    mutationFn: async ({
      connectorID,
      input,
    }: {
      connectorID: string;
      input: SetConnectorChatPreferenceRequest;
    }) => {
      requireCurrentScope(organizationID, accountID);
      const response = requireChatConnectorPreferences(
        await connectorService.setChatPreference(connectorID, input, crypto.randomUUID())
      );
      // Capture the originating scope. A late response must never populate another user's cache.
      queryClient.setQueryData<ApiResponseData<ConnectorChatPreferences>>(queryKey, previous =>
        previous && previous.data.revision > response.data.revision ? previous : response
      );
      return response;
    },
    onError: () => {
      void queryClient.invalidateQueries({ queryKey });
    },
  });
}
