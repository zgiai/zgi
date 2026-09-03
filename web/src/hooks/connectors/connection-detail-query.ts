import type { QueryClient } from '@tanstack/react-query';
import { CONNECTOR_KEYS } from '@/hooks/query-keys';
import type { ApiResponseData } from '@/services/types/common';
import type { PersonalConnectionDetail, RemovalImpact } from '@/services/types/connector';

export function isPersonalConnectionDetailResponse(
  response: unknown,
  connectionId: string
): response is ApiResponseData<PersonalConnectionDetail> {
  if (!response || typeof response !== 'object' || !('data' in response)) return false;
  const connection = response.data as Partial<PersonalConnectionDetail> | null | undefined;
  return Boolean(
    connection &&
      connection.id === connectionId &&
      Array.isArray(connection.capabilities) &&
      Array.isArray(connection.bindings)
  );
}

export function requirePersonalConnectionDetailResponse(
  response: unknown,
  connectionId: string
): ApiResponseData<PersonalConnectionDetail> {
  if (!isPersonalConnectionDetailResponse(response, connectionId)) {
    throw new Error('Connector connection detail is incomplete');
  }
  return response;
}

export function personalConnectionDetailQueryOptions(
  connectionId: string,
  fetchDetail: () => Promise<ApiResponseData<PersonalConnectionDetail>>,
  enabled = true
) {
  return {
    queryKey: CONNECTOR_KEYS.connection(connectionId),
    queryFn: async () => requirePersonalConnectionDetailResponse(await fetchDetail(), connectionId),
    enabled: enabled && Boolean(connectionId),
    // Re-fetch summaries left by older clients even when their cache timestamp is fresh.
    staleTime: (query: { state: { data?: unknown } }) =>
      isPersonalConnectionDetailResponse(query.state.data, connectionId) ? 15_000 : 0,
    // Never let an incomplete cached value reach usage counts or removal controls.
    select: (response: ApiResponseData<PersonalConnectionDetail>) =>
      requirePersonalConnectionDetailResponse(response, connectionId),
    retry: false as const,
  };
}

export function invalidatePersonalConnectionQueries(queryClient: QueryClient): void {
  // Setup/reconnect/rename return summaries. Only a detail endpoint can populate a detail key.
  void queryClient.invalidateQueries({ queryKey: CONNECTOR_KEYS.catalog() });
  void queryClient.invalidateQueries({ queryKey: CONNECTOR_KEYS.connections() });
  void queryClient.invalidateQueries({ queryKey: [...CONNECTOR_KEYS.all, 'chat-preferences'] });
}

export function requirePersonalConnectionRemovalImpactResponse(
  response: unknown,
  connectionId: string
): ApiResponseData<RemovalImpact> {
  if (!response || typeof response !== 'object' || !('data' in response)) {
    throw new Error('Connector removal impact is incomplete');
  }
  const impact = response.data as Partial<RemovalImpact> | null | undefined;
  if (
    !impact ||
    impact.connection_id !== connectionId ||
    typeof impact.can_remove !== 'boolean' ||
    !Array.isArray(impact.affected_targets)
  ) {
    throw new Error('Connector removal impact is incomplete');
  }
  return response as ApiResponseData<RemovalImpact>;
}
