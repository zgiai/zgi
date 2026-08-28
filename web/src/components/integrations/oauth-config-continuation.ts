import type {
  IntegrationAuthDefinition,
  IntegrationCatalogItem,
} from '@/services/types/integration';
import { integrationCatalogID, resolveIntegrationAuthDefinitions } from './integration-utils';
import { isOAuthAuthMethod } from './auth-method-selection';

export interface ConfiguredOAuthContinuation {
  provider: IntegrationCatalogItem;
  auth: IntegrationAuthDefinition;
}

/**
 * Resolves the OAuth method from a freshly fetched catalog response.
 *
 * OAuth client configuration changes the server-derived auth metadata. The
 * continuation must therefore use the refreshed provider rather than patching
 * the pre-save object held by the catalog card.
 */
export function resolveConfiguredOAuthContinuation(
  catalog: IntegrationCatalogItem[],
  integrationId: string,
  authMethodId: string
): ConfiguredOAuthContinuation | null {
  const provider = catalog.find(
    item => item.enabled && integrationCatalogID(item) === integrationId
  );
  if (!provider) return null;

  const auth = resolveIntegrationAuthDefinitions(provider).find(
    method => method.id === authMethodId
  );
  if (
    !auth ||
    !auth.available ||
    !isOAuthAuthMethod(auth) ||
    !auth.oauth?.connect_enabled ||
    !auth.oauth.client_configured
  ) {
    return null;
  }

  return { provider, auth };
}
