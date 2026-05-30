/**
 * File management service
 * Handles all file-related API requests
 */

import type { ApiResponseData } from './types/common';
import type {
  AllFilesResponse,
  GetAllFilesRequest,
  RelatedResourcesResponse,
  StorageUsage,
  FileFolder,
  FileFoldersResponse,
  FileMetadataResponse,
  UploadFileRequest,
  UploadFileResponse,
  CreateFolderRequest,
  CreateFolderResponse,
  UpdateFolderRequest,
  UpdateFolderResponse,
  CreateTextFileRequest,
  CreateTextFileResponse,
  FileOriginalPreviewUrlResponse,
  CreateFileProcessingRequest,
  CreateFileProcessingResponse,
  FileDetailResponse,
  FileParsePreviewResponse,
  FileParseConfirmationListResponse,
  ResolveFileParseConfirmationRequest,
  ResolveFileParseConfirmationResponse,
  BatchIgnoreFileParseConfirmationsRequest,
  BatchIgnoreFileParseConfirmationsResponse,
  ListFileChunksRequest,
  ListFileChunksResponse,
  UpdateFileChunkRequest,
  UpdateFileChunkResponse,
} from './types/file';
import { BaseService } from '@/lib/http/services';

class FileManageService extends BaseService {
  /**
   * Get all files with pagination and optional filtering
   * @param params - Request parameters including page, limit, keyword, sort
   */
  async getAllFiles(params: GetAllFilesRequest): Promise<ApiResponseData<AllFilesResponse>> {
    return this.request('get', '/console/api/file-folders/all-files', undefined, {
      params,
    });
  }

  /**
   * Get recent uploaded files with pagination and optional filtering
   */
  async getRecentFiles(params: GetAllFilesRequest): Promise<ApiResponseData<AllFilesResponse>> {
    return this.request('get', '/console/api/file-folders/recent-files', undefined, {
      params,
    });
  }

  /**
   * Get favorite files with pagination and optional filtering
   */
  async getFavoriteFiles(params: GetAllFilesRequest): Promise<ApiResponseData<AllFilesResponse>> {
    return this.request('get', '/console/api/file-folders/favorite-files', undefined, {
      params,
    });
  }

  /**
   * Get files in folder with pagination and optional filtering
   */
  async getFolderFiles(params: GetAllFilesRequest): Promise<ApiResponseData<AllFilesResponse>> {
    return this.request('get', '/console/api/file-folders/files', undefined, { params });
  }

  /**
   * Get related resources for a specific file
   */
  async getRelatedResources(fileId: string): Promise<ApiResponseData<RelatedResourcesResponse>> {
    return this.request('get', `/console/api/files/${fileId}/related-resources`);
  }

  /**
   * Get storage usage information
   */
  async getStorageUsage(): Promise<ApiResponseData<StorageUsage>> {
    return this.request('get', '/console/api/files/storage-usage');
  }

  /**
   * Download a file by ID
   */
  async downloadFile(fileId: string): Promise<Blob> {
    const response = await this.request('get', `/console/api/files/${fileId}/download`, undefined, {
      responseType: 'blob',
    });
    return response as Blob;
  }

  /**
   * Get signed original file preview URL.
   */
  async getOriginalPreviewUrl(
    fileId: string
  ): Promise<ApiResponseData<FileOriginalPreviewUrlResponse>> {
    return this.request('get', `/console/api/files/${fileId}/preview-url`);
  }

  async getFileDetail(fileId: string): Promise<ApiResponseData<FileDetailResponse>> {
    return this.request('get', `/console/api/files/${fileId}/detail`);
  }

  async createProcessingRequest(
    fileId: string,
    data: CreateFileProcessingRequest
  ): Promise<ApiResponseData<CreateFileProcessingResponse>> {
    return this.request('post', `/console/api/files/${fileId}/processing-requests`, data);
  }

  async getParsePreview(fileId: string): Promise<ApiResponseData<FileParsePreviewResponse>> {
    return this.request('get', `/console/api/files/${fileId}/parse-preview`);
  }

  async getParseConfirmationItems(
    fileId: string,
    params?: { status?: string; limit?: number; offset?: number }
  ): Promise<ApiResponseData<FileParseConfirmationListResponse>> {
    return this.request('get', `/console/api/files/${fileId}/parse-confirmation-items`, undefined, {
      params,
    });
  }

  async resolveParseConfirmationItem(
    fileId: string,
    itemId: string,
    data: ResolveFileParseConfirmationRequest
  ): Promise<ApiResponseData<ResolveFileParseConfirmationResponse>> {
    return this.request(
      'post',
      `/console/api/files/${fileId}/parse-confirmation-items/${itemId}/resolve`,
      data
    );
  }

  async batchIgnoreParseConfirmationItems(
    fileId: string,
    data: BatchIgnoreFileParseConfirmationsRequest = {}
  ): Promise<ApiResponseData<BatchIgnoreFileParseConfirmationsResponse>> {
    return this.request(
      'post',
      `/console/api/files/${fileId}/parse-confirmation-items/batch-ignore`,
      data
    );
  }

  async getFileChunks(
    fileId: string,
    params?: ListFileChunksRequest
  ): Promise<ApiResponseData<ListFileChunksResponse>> {
    return this.request('get', `/console/api/files/${fileId}/chunks`, undefined, {
      params,
    });
  }

  async updateFileChunk(
    fileId: string,
    chunkId: string,
    data: UpdateFileChunkRequest
  ): Promise<ApiResponseData<UpdateFileChunkResponse>> {
    return this.request('patch', `/console/api/files/${fileId}/chunks/${chunkId}`, data);
  }

  async getFilesMetadata(fileIds: string[]): Promise<ApiResponseData<FileMetadataResponse>> {
    const params = fileIds.map(id => `file_ids=${encodeURIComponent(id)}`).join('&');
    return this.request('get', `/console/api/files/metadata?${params}`);
  }

  /**
   * Delete files by IDs
   */
  async deleteFiles(fileIds: string[]): Promise<ApiResponseData<{ success: boolean }>> {
    const params = fileIds.map(id => `file_ids=${id}`).join('&');
    return this.request('delete', `/console/api/files?${params}`);
  }

  /**
   * Get all file folders
   */
  async getFileFolders(workspaceId?: string): Promise<ApiResponseData<FileFoldersResponse>> {
    const params = workspaceId ? { workspace_id: workspaceId } : {};
    return this.request('get', '/console/api/file-folders', undefined, { params });
  }

  /**
   * Get a file folder by ID
   */
  async getFileFolder(folderId: string): Promise<ApiResponseData<FileFolder>> {
    return this.request('get', `/console/api/file-folders/${folderId}`);
  }

  /**
   * Get child folders of a specific folder
   */
  async getChildFolders(
    parentId?: string,
    workspaceId?: string
  ): Promise<ApiResponseData<FileFoldersResponse>> {
    const params: Record<string, string> = {};
    if (parentId) params.parent_id = parentId;
    if (workspaceId) params.workspace_id = workspaceId;
    return this.request('get', '/console/api/file-folders', undefined, { params });
  }

  /**
   * Upload file to a folder
   */
  async uploadFileToFolder(data: UploadFileRequest): Promise<ApiResponseData<UploadFileResponse>> {
    const formData = new FormData();
    formData.append('file', data.file);
    if (data.folder_id) {
      formData.append('folder_id', data.folder_id);
    }
    if (data.workspace_id) {
      formData.append('workspace_id', data.workspace_id);
    }
    if (data.processing_mode) {
      formData.append('processing_mode', data.processing_mode);
    }

    return this.request('post', '/console/api/files/upload', formData, {
      headers: {
        'Content-Type': 'multipart/form-data',
      },
    });
  }

  /**
   * Create a new folder
   */
  async createFolder(data: CreateFolderRequest): Promise<ApiResponseData<CreateFolderResponse>> {
    const body: Record<string, string> = {
      name: data.name,
      parent_id: data.parent_id || '',
      ...(data.workspace_id && { workspace_id: data.workspace_id }),
    };
    return this.request('post', '/console/api/file-folders', body);
  }

  /**
   * Update folder
   */
  async updateFolder(
    folderId: string,
    data: UpdateFolderRequest
  ): Promise<ApiResponseData<UpdateFolderResponse>> {
    const body: Record<string, string> = {
      name: data.name,
      parent_id: data.parent_id,
    };
    return this.request('patch', `/console/api/file-folders/${folderId}`, body);
  }

  /**
   * Delete folder
   */
  async deleteFolder(folderId: string): Promise<ApiResponseData<{ success: boolean }>> {
    return this.request('delete', `/console/api/file-folders/${folderId}`);
  }

  /**
   * Create a text file
   */
  async createTextFile(
    data: CreateTextFileRequest
  ): Promise<ApiResponseData<CreateTextFileResponse>> {
    return this.request('post', '/console/api/files/text', {
      filename: data.filename,
      content: data.content,
      ...(data.folder_id && { folder_id: data.folder_id }),
      ...(data.workspace_id && { workspace_id: data.workspace_id }),
    });
  }

  /**
   * Add a file to favorites
   */
  async addFileFavorite(fileId: string): Promise<ApiResponseData<{ success: boolean }>> {
    return this.request('post', '/console/api/file-favorites', { file_id: fileId });
  }

  /**
   * Remove a file from favorites
   */
  async removeFileFavorite(fileId: string): Promise<ApiResponseData<{ success: boolean }>> {
    return this.request('delete', `/console/api/file-favorites/${fileId}`);
  }
}

export const fileManageService = new FileManageService();
