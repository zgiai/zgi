/**
 * File management service types
 */
import type { Dataset } from './dataset';
export interface FileItem {
  content_text: null | string;
  created_at: string;
  created_by: string;
  created_by_role: string;
  extension: string;
  hash: string;
  id: string;
  is_favorite?: boolean;
  key: string;
  mime_type: string;
  name: string;
  related_count: number;
  related_dataset_count: number;
  size: number;
  source_url: string;
  storage_type: string;
  workspace_id: string;
  used: boolean;
  used_at: null;
  used_by: null;
}

export interface AllFilesResponse {
  data: FileItem[];
  page: number;
  limit: number;
  total: number;
  has_more: boolean;
}

export interface GetAllFilesRequest {
  page: string;
  limit: string;
  keyword?: string;
  sort?: string;
  folder_id?: string;
  extension?: string;
  workspace_id?: string;
}

export interface RelatedResource {
  datasets: {
    count: number;
    items: Dataset[];
  };
}

export interface RelatedResourceItem {
  id: string;
  name: string;
  type: 'dataset' | 'agent' | 'workflow';
}

export interface RelatedResourcesResponse {
  data: RelatedResource;
}

export interface FileOriginalPreviewUrlResponse {
  url: string;
  file_id: string;
  name: string;
  extension: string;
  mime_type: string;
}

export interface StorageUsage {
  used: number; // GB
  total: number; // GB
}

/**
 * File folder item
 */
export interface FileFolder {
  id: string;
  name: string;
  description: string | null;
  icon: string | null;
  icon_background: string | null;
  icon_type: string | null;
  parent_id: string | null;
  position: number;
  workspace_id: string;
  created_at: string;
  created_by: string;
  updated_at: string;
  updated_by: string | null;
}

/**
 * File folders response
 */
export interface FileFoldersResponse {
  data: FileFolder[];
}

/**
 * Upload file request
 */
export interface UploadFileRequest {
  file: File;
  folder_id?: string;
  workspace_id?: string;
}

/**
 * Upload file response
 */
export interface UploadFileResponse {
  id: string;
  name: string;
  size: number;
  extension: string;
  mime_type: string;
  created_at: string;
  created_by: string;
}

/**
 * Create folder request
 */
export interface CreateFolderRequest {
  name: string;
  parent_id?: string;
  workspace_id?: string;
}

/**
 * Create folder response
 */
export interface CreateFolderResponse {
  id: string;
  name: string;
  parent_id: string | null;
  created_at: string;
  created_by: string;
}

/**
 * Update folder request
 */
export interface UpdateFolderRequest {
  name: string;
  parent_id: string;
}

/**
 * Update folder response
 */
export interface UpdateFolderResponse {
  id: string;
  name: string;
  parent_id: string | null;
  updated_at: string;
  updated_by: string;
}

/**
 * Create text file request
 */
export interface CreateTextFileRequest {
  filename: string;
  content: string;
  folder_id?: string;
  workspace_id?: string;
}

/**
 * Create text file response
 */
export interface CreateTextFileResponse {
  id: string;
  name: string;
  size: number;
  extension: string;
  mime_type: string;
  created_at: string;
  created_by: string;
}
