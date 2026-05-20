// Dataset folder related type definitions

import type { Dataset } from '..';

/**
 * Dataset folder interface
 */
export interface DatasetFolder {
  id: string;
  workspace_id?: string;
  name: string;
  description: string;
  parent_id: string | null;
  created_by: string;
  created_at: string;
  updated_by: string | null;
  updated_at: string;
  position: number;
  workspace?: {
    id: string;
    name: string;
  };
  can_edit: boolean;
}

/**
 * Dataset folder list response
 */
export interface DatasetFolderList {
  data: DatasetFolder[];
  has_more: boolean;
  limit: number;
  page: number;
  total: number;
}

/**
 * Create dataset folder request
 */
export interface CreateDatasetFolderRequest {
  name: string;
  workspace_id?: string;
  description?: string;
  // icon fields removed
  position?: string;
  parent_id?: string | null;
}

/**
 * Update dataset folder request
 */
export interface UpdateDatasetFolderRequest {
  name?: string;
  workspace_id?: string;
  description?: string;
  // icon fields removed
  position?: string;
}

/**
 * Folder datasets response - paginated datasets inside a folder.
 * Folders cannot contain subfolders by design.
 */
export interface FolderDatasetsResponse {
  page: number;
  limit: number;
  total: number;
  has_more: boolean;
  data: Dataset[];
}

/**
 * Move dataset to folder request
 */
export interface MoveDatasetRequest {
  dataset_id: string;
  folder_id: string; // Empty string means move to root directory
}

/**
 * Move dataset response
 */
export interface MoveDatasetResponse {
  id: string;
  workspace_id: string;
  name: string;
  description: string;
  parent_id: string | null;
  created_by: string;
  created_at: string;
  updated_by: string | null;
  updated_at: string;
  // icon fields removed
  position: number;
  workspace?: {
    id: string;
    name: string;
  };
}

export interface CreateDatasetFolderResponse {
  name: string;
  workspace_id?: string;
  description: string;
}
