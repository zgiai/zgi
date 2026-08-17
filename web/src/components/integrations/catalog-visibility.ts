import type { IntegrationCatalogItem } from '@/services/types/integration';
import { integrationCatalogID } from './integration-utils';

// These adapters remain registered so existing connections keep working and
// can still be maintained. They are hidden only from new-connection discovery
// until their product surface is ready to be offered again.
const HIDDEN_CONNECTABLE_PROVIDER_IDS = new Set(['gmail', 'x']);

export function isConnectableProviderVisible(provider: IntegrationCatalogItem | string): boolean {
  const id = typeof provider === 'string' ? provider : integrationCatalogID(provider);
  return !HIDDEN_CONNECTABLE_PROVIDER_IDS.has(id.trim().toLowerCase());
}
