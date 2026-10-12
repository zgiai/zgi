import { useCallback, useMemo } from 'react';
import {
  useBindZgiMarketSubjectTools,
  useZgiMarketSubjectTools,
  useZgiMarketToolCandidates,
} from '@/hooks/use-zgi-market';
import type {
  MarketConnection,
  MarketToolBindingItem,
  MarketToolCandidate,
  MarketToolSubjectType,
} from '@/services/types/zgi-market';

/** The connected connections of the organization that a listing can use. */
export function usableMarketConnections(
  listing: Pick<MarketToolCandidate, 'required_connectors'>,
  connections: MarketConnection[]
): MarketConnection[] {
  const needed = listing.required_connectors.map(connector => connector.slug);
  return connections.filter(
    connection => needed.includes(connection.connector_slug) && connection.status === 'connected'
  );
}

/**
 * The ZGI Market servers of one agent or digital human, and the changes an administrator can make
 * to them. A change replaces the binding at once: it does not wait for a publish.
 */
export function useZgiMarketSubjectSelection(
  subjectType: MarketToolSubjectType,
  subjectId: string,
  enabled = true
) {
  const active = enabled && Boolean(subjectId);
  const candidates = useZgiMarketToolCandidates(active);
  const binding = useZgiMarketSubjectTools(subjectType, active ? subjectId : '');
  const bind = useBindZgiMarketSubjectTools(subjectType, subjectId);

  const items = useMemo<MarketToolBindingItem[]>(() => binding.data?.items ?? [], [binding.data]);
  const listings = candidates.data?.listings ?? [];
  const connections = candidates.data?.connections ?? [];

  const replace = useCallback(
    (next: Array<{ listing_id: number; connection_id?: number | null }>) =>
      bind.mutateAsync(
        next.map(item => ({
          listing_id: item.listing_id,
          ...(item.connection_id ? { connection_id: item.connection_id } : {}),
        }))
      ),
    [bind]
  );

  const add = useCallback(
    (listing: MarketToolCandidate, connectionId?: number) =>
      replace([
        ...items.filter(item => item.listing_id !== listing.listing_id),
        { listing_id: listing.listing_id, connection_id: connectionId ?? null },
      ]),
    [items, replace]
  );

  const remove = useCallback(
    (listingId: number) => replace(items.filter(item => item.listing_id !== listingId)),
    [items, replace]
  );

  const setConnection = useCallback(
    (listingId: number, connectionId: number) =>
      replace(
        items.map(item =>
          item.listing_id === listingId ? { ...item, connection_id: connectionId } : item
        )
      ),
    [items, replace]
  );

  return {
    items,
    listings,
    connections,
    gatewayConfigured: binding.data?.gateway_configured ?? true,
    isLoading: active && (candidates.isPending || binding.isPending),
    isError: candidates.isError || binding.isError,
    isSaving: bind.isPending,
    add,
    remove,
    setConnection,
  };
}
