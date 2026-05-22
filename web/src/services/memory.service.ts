import { http } from '@/lib/http';
import type {
  AccountMemoryDeleteResponse,
  AccountMemoryEntryResponse,
  AccountMemoryStateResponse,
  CreateAccountMemoryEntryRequest,
  UpdateAccountMemoryEntryRequest,
  UpdateAccountMemorySettingRequest,
} from '@/services/types/memory';

const MEMORY_BASE_PATH = '/console/api/memory';

export const memoryService = {
  getMe() {
    return http.get<AccountMemoryStateResponse>(`${MEMORY_BASE_PATH}/me`);
  },

  updateSettings(payload: UpdateAccountMemorySettingRequest) {
    return http.patch<AccountMemoryStateResponse>(`${MEMORY_BASE_PATH}/me/settings`, payload);
  },

  createEntry(payload: CreateAccountMemoryEntryRequest) {
    return http.post<AccountMemoryEntryResponse>(`${MEMORY_BASE_PATH}/me/entries`, payload);
  },

  updateEntry(id: string, payload: UpdateAccountMemoryEntryRequest) {
    return http.patch<AccountMemoryEntryResponse>(
      `${MEMORY_BASE_PATH}/me/entries/${encodeURIComponent(id)}`,
      payload
    );
  },

  deleteEntry(id: string) {
    return http.delete<AccountMemoryDeleteResponse>(
      `${MEMORY_BASE_PATH}/me/entries/${encodeURIComponent(id)}`
    );
  },
};
