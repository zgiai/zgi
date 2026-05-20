import { BaseService } from '@/lib/http/services';
import type {
  DatasetFolder,
  DatasetFolderList,
  CreateDatasetFolderRequest,
  UpdateDatasetFolderRequest,
  FolderDatasetsResponse,
  MoveDatasetRequest,
  MoveDatasetResponse,
  CreateDatasetFolderResponse,
} from './types/dataset-folder';
import type { ApiResponseData } from './types/common';

/**
 * DatasetFolderService
 * ---------------------------------------------------------------------------
 * Handles all dataset folder related APIs.
 * All methods return the unified `ApiResponseData<T>` structure **without**
 * stripping the `data` wrapper so that callers can decide how to consume it.
 * ---------------------------------------------------------------------------
 */
class DatasetFolderService extends BaseService {
  constructor() {
    super({
      endpoint: 'main',
      basePath: '/console/api',
    });
  }

  /* ------------------------------------------------------------------------ */
  /* Dataset folder operations                                                */
  /* ------------------------------------------------------------------------ */

  /**
   * Get dataset folder list
   * GET /console/api/dataset-folders
   */
  getDatasetFolders(
    params: { keyword?: string; workspace_id?: string } = {}
  ): Promise<ApiResponseData<DatasetFolderList>> {
    const query: Record<string, unknown> = {};
    if (params.keyword && params.keyword.trim().length > 0) query.keyword = params.keyword;
    const workspaceId = params.workspace_id;
    if (workspaceId) query.workspace_id = workspaceId;
    return this.request('get', '/dataset-folders', undefined, { params: query });
  }

  /**
   * Create dataset folder
   * POST /console/api/dataset-folders
   */
  createDatasetFolder(
    data: CreateDatasetFolderRequest
  ): Promise<ApiResponseData<CreateDatasetFolderResponse>> {
    const body = { ...data };

    return this.request('post', '/dataset-folders', body);
  }

  /**
   * Get dataset folder detail
   * GET /console/api/dataset-folders/{folder_id}
   */
  getDatasetFolder(folderId: string): Promise<ApiResponseData<DatasetFolder>> {
    return this.request('get', `/dataset-folders/${folderId}`);
  }

  /**
   * Update dataset folder
   * PATCH /console/api/dataset-folders/{folder_id}
   */
  updateDatasetFolder(
    folderId: string,
    data: UpdateDatasetFolderRequest
  ): Promise<ApiResponseData<Record<string, unknown>>> {
    const body = { ...data };

    return this.request('patch', `/dataset-folders/${folderId}`, body);
  }

  /**
   * Delete dataset folder
   * DELETE /console/api/dataset-folders/{folder_id}
   */
  deleteDatasetFolder(folderId: string): Promise<ApiResponseData<DatasetFolder>> {
    return this.request('delete', `/dataset-folders/${folderId}`);
  }

  /**
   * Get folder datasets (paginated datasets within a specific folder)
   * GET /console/api/dataset-folders/datasets
   */
  getFolderDatasets(
    params: {
      folder_id?: string;
      page?: number;
      limit?: number;
      keyword?: string;
      workspace_id?: string;
    } = {}
  ): Promise<ApiResponseData<FolderDatasetsResponse>> {
    const query: Record<string, unknown> = {};
    if (params.folder_id) query.folder_id = params.folder_id;
    if (params.page !== undefined) query.page = params.page;
    if (params.limit !== undefined) query.limit = params.limit;
    if (params.keyword && params.keyword.trim().length > 0) query.keyword = params.keyword;
    const workspaceId = params.workspace_id;
    if (workspaceId) query.workspace_id = workspaceId;
    return this.request('get', '/dataset-folders/datasets', undefined, { params: query });
  }

  /**
   * Move dataset to folder
   * POST /console/api/dataset-folders/move-dataset
   */
  moveDatasetToFolder(data: MoveDatasetRequest): Promise<ApiResponseData<MoveDatasetResponse>> {
    return this.request('post', '/dataset-folders/move-dataset', data);
  }
}

// Export singleton instance
export const datasetFolderService = new DatasetFolderService();
export default datasetFolderService;
