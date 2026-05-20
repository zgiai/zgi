import type { SearchMethod } from '@/services/types/dataset';

/**
 * @util normalizeDatasetSearchMethod
 * Normalize dataset search method so graph search is only used when graph flow is enabled.
 */
export function normalizeDatasetSearchMethod(
  searchMethod: SearchMethod | undefined,
  isGraphEnabled: boolean
): SearchMethod {
  if (!isGraphEnabled) {
    return 'semantic_search';
  }

  return searchMethod === 'graph_search' ? 'graph_search' : 'semantic_search';
}
