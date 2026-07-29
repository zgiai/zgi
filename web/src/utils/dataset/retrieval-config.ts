import type { SearchMethod } from '@/services/types/dataset';

/**
 * @util normalizeDatasetSearchMethod
 * Normalize legacy dataset search methods to the two supported retrieval modes.
 * New or missing configurations default to hybrid + graph retrieval.
 */
export function normalizeDatasetSearchMethod(
  searchMethod: SearchMethod | undefined,
  isGraphEnabled: boolean
): SearchMethod {
  if (!searchMethod) {
    return 'graph_search';
  }

  if (searchMethod === 'graph_search') {
    return isGraphEnabled ? 'graph_search' : 'hybrid_search';
  }

  return 'hybrid_search';
}
