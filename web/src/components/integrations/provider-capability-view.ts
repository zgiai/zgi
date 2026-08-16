'use client';

import { useMemo } from 'react';
import { useIntegrationProviderCapabilities } from '@/hooks';
import type {
  IntegrationActionCapability,
  IntegrationActionDefinition,
  IntegrationCatalogItem,
} from '@/services/types/integration';
import { integrationCatalogID, resolveIntegrationAuthDefinitions } from './integration-utils';

export function isReadCapability(action: IntegrationActionDefinition): boolean {
  return action.effect === 'read' || action.effect === 'none';
}

export function hasActionScopes(action: IntegrationActionDefinition): boolean {
  return Boolean(action.required_scopes?.length || action.required_any_scopes?.length);
}

/**
 * User-facing capability surfaces always describe what the signed-in account
 * can use in the current workspace. Organization policy editing is loaded
 * separately and must not silently replace this availability scope.
 */
export function useCurrentAccountProviderCapabilityView(
  provider: IntegrationCatalogItem | null,
  enabled = true
) {
  const integrationId = provider ? integrationCatalogID(provider) : '';
  const capabilityQuery = useIntegrationProviderCapabilities(integrationId, 'account', enabled);
  const actions = useMemo(() => provider?.actions ?? [], [provider?.actions]);
  const liveCapabilities =
    capabilityQuery.isSuccess && !capabilityQuery.isFetching
      ? capabilityQuery.data?.data
      : undefined;
  const liveCapabilityByAction = useMemo(
    () =>
      new Map<string, IntegrationActionCapability>(
        (liveCapabilities?.actions ?? []).map(action => [action.id, action])
      ),
    [liveCapabilities?.actions]
  );
  const summary = useMemo(
    () => ({
      total: actions.length,
      read: actions.filter(isReadCapability).length,
      write: actions.filter(action => !isReadCapability(action)).length,
    }),
    [actions]
  );
  const supportedCallers = useMemo(
    () => new Set(actions.flatMap(action => action.supported_callers ?? [])),
    [actions]
  );
  const authenticationMethods = useMemo(
    () => (provider ? resolveIntegrationAuthDefinitions(provider) : []),
    [provider]
  );

  return {
    integrationId,
    capabilityQuery,
    actions,
    liveCapabilities,
    liveCapabilityByAction,
    summary,
    supportedCallers,
    authenticationMethods,
  };
}
