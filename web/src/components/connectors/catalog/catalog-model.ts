import type { ConnectorCatalogItem, ConnectorCatalogStatus } from '@/services/types/connector';
import { connectorCatalogSearchValues } from '@/components/connectors/connector-metadata';
import { supportsConnectorSetup } from '../setup/app-setup-state';

export type ConnectorCatalogRowState =
  | 'not_connected'
  | 'connected'
  | 'reconnect_required'
  | 'unavailable';

export interface ConnectorCatalogFilters {
  query: string;
  status: ConnectorCatalogStatus;
}

export interface PartitionedConnectorCatalog {
  connectors: ConnectorCatalogItem[];
  dataSources: ConnectorCatalogItem[];
}

export function getConnectorCatalogRowState(item: ConnectorCatalogItem): ConnectorCatalogRowState {
  const connectionState = item.connection_summary.state;
  if (item.connection_summary.count === 0 && supportsConnectorSetup(item) &&
    item.capabilities.every(capability => capability.usable || !capability.unavailable_reason || capability.unavailable_reason === 'connector_not_verified')) {
    return 'not_connected';
  }
  if (item.connection_summary.count > 0 && connectionState === 'ready') return 'connected';
  if (item.connection_summary.count > 0 && connectionState === 'reconnect_required') {
    return 'reconnect_required';
  }
  if (
    connectionState === 'unavailable' ||
    !item.capabilities.some(capability => capability.usable)
  ) {
    return 'unavailable';
  }
  return 'not_connected';
}

function searchableText(item: ConnectorCatalogItem): string {
  return connectorCatalogSearchValues(item).join(' ').toLocaleLowerCase();
}

function matchesQuery(item: ConnectorCatalogItem, query: string): boolean {
  const terms = query.trim().toLocaleLowerCase().split(/\s+/).filter(Boolean);
  if (terms.length === 0) return true;
  const haystack = searchableText(item);
  return terms.every(term => haystack.includes(term));
}

function matchesStatus(item: ConnectorCatalogItem, status: ConnectorCatalogStatus): boolean {
  if (status === 'all') return true;
  const rowState = getConnectorCatalogRowState(item);
  if (status === 'connected') return rowState === 'connected';
  return (
    (item.connection_summary.needs_attention_count ?? 0) > 0 ||
    rowState === 'reconnect_required' ||
    rowState === 'unavailable'
  );
}

export function filterConnectorCatalog(
  items: readonly ConnectorCatalogItem[],
  filters: ConnectorCatalogFilters
): ConnectorCatalogItem[] {
  return items.filter(
    item => matchesQuery(item, filters.query) && matchesStatus(item, filters.status)
  );
}

export function partitionConnectorCatalog(
  items: readonly ConnectorCatalogItem[]
): PartitionedConnectorCatalog {
  const connectors: ConnectorCatalogItem[] = [];
  const dataSources: ConnectorCatalogItem[] = [];
  for (const item of items) {
    if (item.classification === 'connector') connectors.push(item);
    else dataSources.push(item);
  }
  return { connectors, dataSources };
}
