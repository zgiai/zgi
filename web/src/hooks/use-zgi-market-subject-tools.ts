import { useCallback, useMemo } from 'react';
import {
  useBindZgiMarketSubjectTools,
  useZgiMarketSubjectTools,
  useZgiMarketToolCandidates,
} from '@/hooks/use-zgi-market';
import type {
  MarketConnection,
  MarketToolBindingInput,
  MarketToolBindingItem,
  MarketToolCandidate,
  MarketToolItemKind,
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

/** The kind of a bound item; one bound before API offerings existed is an MCP server. */
export function marketItemKind(item: Pick<MarketToolBindingItem, 'kind'>): MarketToolItemKind {
  return item.kind === 'api' ? 'api' : 'mcp';
}

/** Where the listing of a candidate or a bound item is shown. */
export function marketItemHref(item: { kind?: MarketToolItemKind; slug: string }): string {
  return `/dashboard/market/catalog/${item.kind === 'api' ? 'api' : 'mcp'}/${item.slug}`;
}

/** One tool of a market row as its chip shows it. */
export interface MarketToolChip {
  key: string;
  /** The tool name of an MCP server, or the request of an API operation. */
  label: string;
  risk: 'read' | 'write' | 'destructive';
  /** What the tool does, or its tool name for an API operation. */
  hint?: string;
}

/**
 * The tools of a candidate as chips. An MCP tool is named by the vendor; an API operation is
 * shown as its request, which reads better than the tool name derived from it.
 */
export function candidateToolChips(listing: MarketToolCandidate, locale: string): MarketToolChip[] {
  if (listing.kind === 'api') {
    return (listing.operations ?? [])
      .filter(operation => operation.bindable)
      .map(operation => ({
        key: operation.key,
        label: operation.key,
        risk: operation.risk_level,
        hint: operation.description || operation.name,
      }));
  }
  const chinese = locale.toLowerCase().startsWith('zh');
  return listing.tools.map(tool => ({
    key: tool.name,
    label: tool.name,
    risk: tool.risk_level,
    hint:
      (chinese
        ? tool.description['zh-CN'] || tool.description['en-US']
        : tool.description['en-US'] || tool.description['zh-CN']) || tool.name,
  }));
}

/** The tools of a bound item as chips; see candidateToolChips. */
export function boundToolChips(
  item: MarketToolBindingItem,
  candidate: MarketToolCandidate | undefined,
  locale: string
): MarketToolChip[] {
  if (marketItemKind(item) === 'api') {
    return (item.operations ?? []).map(operation => ({
      key: operation.key,
      label: operation.key,
      risk: operation.risk_level,
      hint:
        candidate?.operations?.find(known => known.key === operation.key)?.description ||
        operation.name,
    }));
  }
  const known = candidate ? candidateToolChips(candidate, locale) : [];
  return item.tools.map(tool => ({
    key: tool.name,
    label: tool.name,
    risk: tool.risk_level,
    hint: known.find(chip => chip.key === tool.name)?.hint ?? tool.name,
  }));
}

/** The operations of an API candidate that can be tools. */
export function bindableOperations(listing: Pick<MarketToolCandidate, 'operations'>) {
  return (listing.operations ?? []).filter(operation => operation.bindable);
}

/**
 * What a chosen set of operation keys means for a binding: undefined when it is every bindable
 * operation, so the item keeps following the listing, and the chosen keys otherwise.
 */
export function chosenOperations(
  listing: Pick<MarketToolCandidate, 'operations'>,
  keys: string[]
): string[] | undefined {
  const bindable = bindableOperations(listing).map(operation => operation.key);
  const chosen = bindable.filter(key => keys.includes(key));
  return chosen.length === bindable.length ? undefined : chosen;
}

/** What a filter of a connector picker asks of the market rows. */
export interface MarketRowFilter {
  query: string;
  status: 'all' | 'connected' | 'needs_attention';
}

/**
 * Whether a candidate passes a picker's search and status filter. The search reads the title in
 * both languages, the slugs, and the names and paths of its tools.
 */
export function matchesMarketRowFilter(
  listing: MarketToolCandidate,
  connections: MarketConnection[],
  filter: MarketRowFilter
): boolean {
  const needsConnection =
    listing.required_connectors.length > 0 &&
    usableMarketConnections(listing, connections).length === 0;
  if (filter.status === 'connected' && needsConnection) return false;
  if (filter.status === 'needs_attention' && !needsConnection) return false;
  const query = filter.query.trim().toLowerCase();
  if (!query) return true;
  const haystack = [
    listing.title['zh-CN'],
    listing.title['en-US'],
    listing.slug,
    listing.offering_slug,
    ...listing.tools.map(tool => tool.name),
    ...(listing.operations ?? []).flatMap(operation => [operation.path, operation.method]),
  ];
  return haystack.some(value => (value ?? '').toLowerCase().includes(query));
}

/** What the market is asked to keep for an item that is already bound. */
function keptItem(item: MarketToolBindingItem): MarketToolBindingInput {
  return {
    listing_id: item.listing_id,
    ...(item.connection_id ? { connection_id: item.connection_id } : {}),
    ...(marketItemKind(item) === 'api' && item.offering_slug
      ? {
          offering_slug: item.offering_slug,
          // An item that follows every operation is asked for without a list, so it keeps following.
          ...(item.all_operations
            ? {}
            : { operations: (item.operations ?? []).map(operation => operation.key) }),
        }
      : {}),
  };
}

/**
 * The ZGI Market servers and API offerings of one agent or digital human, and the changes an
 * administrator can make to them. A change replaces the binding at once: it does not wait for a
 * publish. An item is one activation, so it is told apart by its subscription.
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

  /**
   * Adds a candidate, or replaces it when it is already bound. `operations` are the operation
   * keys of an API offering to make tools of; without them every bindable operation is, also
   * ones the listing gains later.
   */
  const add = useCallback(
    (listing: MarketToolCandidate, connectionId?: number, operations?: string[]) =>
      bind.mutateAsync([
        ...items.filter(item => item.subscription_id !== listing.subscription_id).map(keptItem),
        {
          listing_id: listing.listing_id,
          ...(connectionId ? { connection_id: connectionId } : {}),
          ...(listing.kind === 'api'
            ? {
                offering_slug: listing.offering_slug,
                ...(operations && operations.length > 0 ? { operations } : {}),
              }
            : {}),
        },
      ]),
    [bind, items]
  );

  /** Changes which operations of a bound API offering are tools; undefined means all of them. */
  const setOperations = useCallback(
    (subscriptionId: number, operations: string[] | undefined) =>
      bind.mutateAsync(
        items.map(item => {
          if (item.subscription_id !== subscriptionId) {
            return keptItem(item);
          }
          const { operations: _dropped, ...kept } = keptItem(item);
          return operations && operations.length > 0 ? { ...kept, operations } : kept;
        })
      ),
    [bind, items]
  );

  const remove = useCallback(
    (subscriptionId: number) =>
      bind.mutateAsync(items.filter(item => item.subscription_id !== subscriptionId).map(keptItem)),
    [bind, items]
  );

  const setConnection = useCallback(
    (subscriptionId: number, connectionId: number) =>
      bind.mutateAsync(
        items.map(item =>
          keptItem(
            item.subscription_id === subscriptionId
              ? { ...item, connection_id: connectionId }
              : item
          )
        )
      ),
    [bind, items]
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
    setOperations,
  };
}
