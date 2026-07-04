import type { SearchMethod } from '@/services/types/dataset';

/**
 * @util normalizeDatasetSearchMethod
 * Normalize dataset search method so graph search is only used when graph flow is enabled.
 */
export function normalizeDatasetSearchMethod(
  searchMethod: SearchMethod | undefined,
  isGraphEnabled: boolean
): SearchMethod {
  if (!searchMethod) {
    return 'hybrid_search';
  }

  if (searchMethod === 'graph_search') {
    return isGraphEnabled ? 'graph_search' : 'hybrid_search';
  }

  return searchMethod;
}
