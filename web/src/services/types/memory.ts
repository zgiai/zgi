import type { ApiResponseData, SuccessResponse } from './common';

export type AccountMemoryCategory = 'preference' | 'profile' | 'instruction' | 'fact' | 'other';

export interface AccountMemoryEntry {
  id: string;
  content: string;
  category: AccountMemoryCategory;
  enabled: boolean;
  created_at: number;
  updated_at: number;
}

export interface AccountMemoryState {
  enabled: boolean;
  entries: AccountMemoryEntry[];
  updated_at: number;
}

export interface UpdateAccountMemorySettingRequest {
  enabled: boolean;
}

export interface CreateAccountMemoryEntryRequest {
  content: string;
  category?: AccountMemoryCategory;
}

export interface UpdateAccountMemoryEntryRequest {
  content?: string;
  category?: AccountMemoryCategory;
  enabled?: boolean;
}

export type AccountMemoryStateResponse = ApiResponseData<AccountMemoryState>;
export type AccountMemoryEntryResponse = ApiResponseData<AccountMemoryEntry>;
export type AccountMemoryDeleteResponse = ApiResponseData<SuccessResponse>;
