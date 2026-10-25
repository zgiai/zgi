import { http } from '@/lib/http';
import type { ApiResponseData } from './types/common';
import type { MemoryArchiveEntry, MemoryHistoryPage } from './types/memory-history';
import type {
  UserMemoryDeleteResponse,
  UserMemoryRelationshipDetailResponse,
  UserMemoryRelationshipFilters,
  UserMemoryRelationshipPageResponse,
  UserMemoryRelationshipResponse,
  UserRelationshipMemoryItemResponse,
} from './types/relationship-memory';

const basePath = '/console/api/user-memories/relationships';

function queryString(values: object) {
  const params = new URLSearchParams();
  Object.entries(values).forEach(([key, value]) => {
    if (value !== undefined && value !== '') params.set(key, String(value));
  });
  const encoded = params.toString();
  return encoded ? `?${encoded}` : '';
}

export const relationshipMemoryService = {
  list(filters: UserMemoryRelationshipFilters = {}) {
    return http.get<UserMemoryRelationshipPageResponse>(`${basePath}${queryString(filters)}`);
  },

  get(id: string) {
    return http.get<UserMemoryRelationshipDetailResponse>(`${basePath}/${encodeURIComponent(id)}`);
  },

  updateSettings(
    id: string,
    payload: {
      memory_enabled?: boolean;
      auto_capture_enabled?: boolean;
      expected_revision: number;
    }
  ) {
    return http.patch<UserMemoryRelationshipResponse>(
      `${basePath}/${encodeURIComponent(id)}/settings`,
      payload
    );
  },

  putItem(
    id: string,
    key: string,
    payload: {
      display_name?: string;
      category?: string;
      content: string;
      expected_revision: number;
    }
  ) {
    return http.put<UserRelationshipMemoryItemResponse>(
      `${basePath}/${encodeURIComponent(id)}/items/${encodeURIComponent(key)}`,
      payload
    );
  },

  deleteItem(id: string, key: string, expectedRevision: number) {
    return http.delete<UserMemoryDeleteResponse>(
      `${basePath}/${encodeURIComponent(id)}/items/${encodeURIComponent(key)}?expected_revision=${expectedRevision}`
    );
  },

  clear(id: string) {
    return http.delete<ApiResponseData<{ result: 'success' }>>(
      `${basePath}/${encodeURIComponent(id)}/items`
    );
  },

  permanentlyDelete(id: string) {
    return http.delete<ApiResponseData<{ result: 'success' }>>(
      `${basePath}/${encodeURIComponent(id)}`
    );
  },

  export(id: string) {
    return http.get<ApiResponseData<unknown>>(`${basePath}/${encodeURIComponent(id)}/export`);
  },

  undo(id: string, operationId: string) {
    return http.post<ApiResponseData<unknown>>(
      `${basePath}/${encodeURIComponent(id)}/operations/${encodeURIComponent(operationId)}/undo`
    );
  },

  history(id: string, params: { key?: string; before?: string; limit?: number } = {}) {
    return http.get<ApiResponseData<MemoryHistoryPage>>(
      `${basePath}/${encodeURIComponent(id)}/history${queryString(params)}`
    );
  },

  revert(id: string, versionId: string) {
    return http.post<ApiResponseData<unknown>>(
      `${basePath}/${encodeURIComponent(id)}/history/${encodeURIComponent(versionId)}/revert`
    );
  },

  archive(id: string) {
    return http.get<ApiResponseData<{ entries: MemoryArchiveEntry[] }>>(
      `${basePath}/${encodeURIComponent(id)}/archive`
    );
  },

  updateArchiveEntry(id: string, entryId: string, content: string) {
    return http.patch<ApiResponseData<MemoryArchiveEntry>>(
      `${basePath}/${encodeURIComponent(id)}/archive/${encodeURIComponent(entryId)}`,
      { content }
    );
  },

  deleteArchiveEntry(id: string, entryId: string) {
    return http.delete<ApiResponseData<{ entry_id: string }>>(
      `${basePath}/${encodeURIComponent(id)}/archive/${encodeURIComponent(entryId)}`
    );
  },
};
