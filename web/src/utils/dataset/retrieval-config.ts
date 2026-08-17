import type { SearchMethod } from '@/services/types/dataset';

/**
 * @util normalizeDatasetSearchMethod
 * Normalize legacy dataset search methods to the two supported retrieval modes.
 * Missing configurations use graph retrieval only when the dataset has a graph.
 */
export function normalizeDatasetSearchMethod(
  searchMethod: SearchMethod | undefined,
  isGraphEnabled: boolean
): SearchMethod {
  if (!searchMethod) {
    return isGraphEnabled ? 'graph_search' : 'hybrid_search';
  }

  if (searchMethod === 'graph_search') {
    return isGraphEnabled ? 'graph_search' : 'hybrid_search';
  }

  return 'hybrid_search';
}
