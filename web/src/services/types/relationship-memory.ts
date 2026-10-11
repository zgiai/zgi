import type { ApiResponseData } from './common';

export type UserMemoryConsumerType = 'agent' | 'digital_human';
export type UserMemoryRelationshipStatus = 'active' | 'archived';
export type UserMemoryCategory = 'profile' | 'preference' | 'constraint' | 'relationship';

export interface UserRelationshipMemoryItem {
  id: string;
  relationship_id: string;
  key: string;
  display_name: string;
  category?: UserMemoryCategory | '';
  content: string;
  content_encoding: 'plain' | string;
  revision: number;
  source_kind: 'legacy' | 'explicit' | 'automatic' | 'manager';
  source_conversation_id?: string;
  source_message_id?: string;
  extractor_version?: string;
  last_operation_id?: string;
  undoable_until?: string;
  created_at: string;
  updated_at: string;
}

export interface UserMemoryRelationship {
  id: string;
  organization_id: string;
  workspace_id: string;
  subject_id: string;
  consumer_type: UserMemoryConsumerType;
  consumer_id: string;
  consumer_name_snapshot: string;
  status: UserMemoryRelationshipStatus;
  memory_enabled: boolean;
  auto_capture_enabled: boolean;
  settings_revision: number;
  memory_epoch: number;
  created_at: string;
  updated_at: string;
  archived_at?: string;
  workspace_name: string;
  can_manage: boolean;
  item_count: number;
}

export interface UserMemoryRelationshipDetail extends UserMemoryRelationship {
  items: UserRelationshipMemoryItem[];
}

export interface UserMemoryRelationshipPage {
  items: UserMemoryRelationship[];
  total: number;
  page: number;
  page_size: number;
}

export type UserMemoryRelationshipPageResponse = ApiResponseData<UserMemoryRelationshipPage>;
export type UserMemoryRelationshipDetailResponse = ApiResponseData<UserMemoryRelationshipDetail>;
export type UserMemoryRelationshipResponse = ApiResponseData<UserMemoryRelationship>;
export type UserRelationshipMemoryItemResponse = ApiResponseData<UserRelationshipMemoryItem>;

export interface UserMemoryDeleteResult {
  operation_id: string;
  key: string;
  deleted: boolean;
  undoable_until?: string;
}

export type UserMemoryDeleteResponse = ApiResponseData<UserMemoryDeleteResult>;

export interface UserMemoryRelationshipFilters {
  workspace_id?: string;
  consumer_type?: UserMemoryConsumerType;
  status?: UserMemoryRelationshipStatus;
  page?: number;
  page_size?: number;
}
