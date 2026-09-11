'use client';

import { useCallback, useState, useSyncExternalStore } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { parseConnectorConfirmationPresentation } from './connector-confirmation-presentation';
import type { ConnectorConfirmationPreview } from '@/services/types/connector';
import { useAuthStore } from '@/store/auth-store';
import { useOrganizationStore } from '@/store/organization-store';
import { connectorService } from '@/services/connector.service';
import { connectorRuntimeDecisionTransport } from './connector-runtime-transport';

/** Observable state is scoped to authenticated owner + organization + opaque receipt. */
export function useConnectorRuntimeReceipt(receipt: string) {
  const actor = useAuthStore.use.user()?.id ?? '';
  const organization = useOrganizationStore.use.currentOrganization()?.id ?? '';
  const scope = JSON.stringify([organization, actor]);
  const identity = JSON.stringify([scope, receipt]);
  const queryClient = useQueryClient();
  const [presentation, setPresentation] = useState<{
    identity: string;
    preview: ConnectorConfirmationPreview | null;
  }>();
  const phase = useSyncExternalStore(
    connectorRuntimeDecisionTransport.subscribe,
    () => connectorRuntimeDecisionTransport.getPhase(receipt, scope),
    () => undefined
  );
  const check = useCallback(async () => {
    if (!receipt || !actor || !organization) throw new Error('Connector scope is unavailable');
    try {
      const response = await queryClient.fetchQuery({
        queryKey: ['connector-confirmation-presentation', scope, receipt],
        queryFn: () => connectorService.getRuntimeConfirmationStatus(receipt, 1),
        staleTime: 0,
        gcTime: 0,
        retry: false,
      });
      if (
        useAuthStore.getState().user?.id !== actor ||
        useOrganizationStore.getState().currentOrganization?.id !== organization
      ) {
        return;
      }
      const { presentation: payload, ...status } = response.data;
      setPresentation({ identity, preview: parseConnectorConfirmationPresentation(payload) });
      if (status.resolution === 'unresolved' && Object.keys(status).length === 1) {
        return;
      }
      if (!connectorRuntimeDecisionTransport.acceptStatus(receipt, status, scope)) {
        throw new Error('Connector status could not be verified');
      }
    } catch (error) {
      if (
        useAuthStore.getState().user?.id === actor &&
        useOrganizationStore.getState().currentOrganization?.id === organization
      ) {
        connectorRuntimeDecisionTransport.markUnconfirmed(receipt, scope);
      }
      throw error;
    }
  }, [receipt, actor, organization, scope, identity, queryClient]);
  return {
    scope,
    preview: presentation?.identity === identity ? presentation.preview : null,
    phase,
    result: connectorRuntimeDecisionTransport.getResult(receipt, scope),
    check,
  };
}
