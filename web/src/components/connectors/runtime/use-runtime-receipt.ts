'use client';

import { useCallback, useSyncExternalStore } from 'react';
import { useAuthStore } from '@/store/auth-store';
import { useOrganizationStore } from '@/store/organization-store';
import { connectorService } from '@/services/connector.service';
import { connectorRuntimeDecisionTransport } from './connector-runtime-transport';

/** Observable state is scoped to authenticated owner + organization + opaque receipt. */
export function useConnectorRuntimeReceipt(receipt: string) {
  const actor = useAuthStore.use.user()?.id ?? '';
  const organization = useOrganizationStore.use.currentOrganization()?.id ?? '';
  const scope = JSON.stringify([organization, actor]);
  const phase = useSyncExternalStore(
    connectorRuntimeDecisionTransport.subscribe,
    () => connectorRuntimeDecisionTransport.getPhase(receipt, scope),
    () => undefined
  );
  const check = useCallback(async () => {
    if (!receipt || !actor || !organization) throw new Error('Connector scope is unavailable');
    try {
      const response = await connectorService.getRuntimeConfirmationStatus(receipt);
      if (
        useAuthStore.getState().user?.id !== actor ||
        useOrganizationStore.getState().currentOrganization?.id !== organization
      ) {
        return;
      }
      if (response.data?.resolution === 'unresolved' && Object.keys(response.data).length === 1) {
        return;
      }
      if (!connectorRuntimeDecisionTransport.acceptStatus(receipt, response.data, scope)) {
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
  }, [receipt, actor, organization, scope]);
  return {
    scope,
    phase,
    result: connectorRuntimeDecisionTransport.getResult(receipt, scope),
    check,
  };
}
