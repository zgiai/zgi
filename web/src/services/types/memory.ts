import type { ApiResponseData, SuccessResponse } from './common';

export type AccountMemoryCategory = 'preference' | 'profile' | 'instruction' | 'fact' | 'other';
export type AccountMemoryType = 'long_term' | 'temporary';
export type AccountMemoryStatus = 'active' | 'expired';

export interface AccountMemoryEntry {
  id: string;
  content: string;
  category: AccountMemoryCategory;
  memory_type: AccountMemoryType;
  expires_at?: number | null;
  status: AccountMemoryStatus;
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
  memory_type?: AccountMemoryType;
  expires_at?: string;
}

export interface UpdateAccountMemoryEntryRequest {
  content?: string;
  category?: AccountMemoryCategory;
  memory_type?: AccountMemoryType;
  expires_at?: string;
  enabled?: boolean;
}

export type AccountMemoryStateResponse = ApiResponseData<AccountMemoryState>;
export type AccountMemoryEntryResponse = ApiResponseData<AccountMemoryEntry>;
export type AccountMemoryDeleteResponse = ApiResponseData<SuccessResponse>;
