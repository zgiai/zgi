import { BaseService } from '@/lib/http/services';
import { webappHttp } from '@/lib/http';
import type { ApiResponseData } from './types/common';
import type { FileParseProviderKey, FileUploadProcessingMode } from './types/file';

// File upload response types - updated to match API response format
export interface UploadResponse {
  id: string;
  name: string;
  size: number;
  extension: string;
  mime_type: string;
  hash?: string;
  created_by: string;
  created_at: string;
  url?: string;
}

export interface UploadConfig {
  file_size_limit: number;
  batch_count_limit: number;
  image_file_size_limit: number;
  video_file_size_limit: number;
  audio_file_size_limit: number;
  workflow_file_upload_limit: number;
}

export interface MultipleUploadResponse {
  files: UploadResponse[];
  success: number;
  failed: number;
}

// Upload service using the new BaseService architecture
export class UploadService extends BaseService {
  constructor() {
    super({
      endpoint: 'upload',
      basePath: '/console/api/files',
    });
  }

  // Single file upload with progress tracking
  async uploadSingle(
    file: File,
    options: {
      folder?: string;
      public?: boolean;
      folder_id?: string;
      workspace_id?: string;
      is_temporary?: boolean;
      is_icon?: boolean;
      processing_mode?: FileUploadProcessingMode;
      parse_provider?: FileParseProviderKey;
      onProgress?: (progress: number) => void;
    } = {}
  ): Promise<UploadResponse> {
    try {
      const workspaceId = options.workspace_id;
      const response = await this.uploadFile('/upload', file, {
        onProgress: options.onProgress,
        additionalData: {
          ...(options.folder && { folder: options.folder }),
          ...(options.folder_id && { folder_id: options.folder_id }),
          ...(workspaceId && { workspace_id: workspaceId }),
          ...(options.public !== undefined && { public: String(options.public) }),
          ...(options.is_temporary !== undefined && { is_temporary: String(options.is_temporary) }),
          ...(options.is_icon !== undefined && { is_icon: String(options.is_icon) }),
          ...(options.processing_mode && { processing_mode: options.processing_mode }),
          ...(options.parse_provider && { parse_provider: options.parse_provider }),
        },
      });

      // Handle the new API response format: { code, message, data }
      const apiResponse = response as unknown as ApiResponseData<UploadResponse>;
      if (apiResponse.code === '0' && apiResponse.data) {
        return apiResponse.data as unknown as UploadResponse;
      }

      // If response doesn't match expected format, try to use it directly
      return response as unknown as UploadResponse;
    } catch (error) {
      console.error('File upload failed:', error);
      throw error;
    }
  }

  async uploadWebAppSingle(
    webAppId: string,
    file: File,
    options: {
      onProgress?: (progress: number) => void;
    } = {}
  ): Promise<UploadResponse> {
    const formData = new FormData();
    formData.append('file', file);
    formData.append('is_temporary', 'true');

    const response = await webappHttp.post<ApiResponseData<UploadResponse>>(
      `/console/api/webapps/${webAppId}/files/upload`,
      formData,
      {
        headers: {
          'Content-Type': 'multipart/form-data',
        },
        onUploadProgress: options.onProgress
          ? progressEvent => {
              const progress = Math.round((progressEvent.loaded * 98) / (progressEvent.total || 1));
              options.onProgress?.(progress);
            }
          : undefined,
      }
    );
    if (response.code === '0' && response.data) {
      return response.data;
    }
    throw new Error(response.message || 'Failed to upload file');
  }

  // Get upload configuration
  async getConfig(): Promise<UploadConfig> {
    try {
      const response = await this.request<ApiResponseData<UploadConfig>>('get', '/upload');
      if (response.code === '0' && response.data) {
        return response.data;
      }
      throw new Error(response.message || 'Failed to get upload config');
    } catch (error) {
      console.error('Get upload config failed:', error);
      throw error;
    }
  }

  async getWebAppConfig(webAppId: string): Promise<UploadConfig> {
    const response = await webappHttp.get<ApiResponseData<UploadConfig>>(
      `/console/api/webapps/${webAppId}/files/upload`
    );
    if (response.code === '0' && response.data) {
      return response.data;
    }
    throw new Error(response.message || 'Failed to get upload config');
  }

  // Multiple file upload
  async uploadMultiple(
    files: File[],
    options: {
      folder?: string;
      public?: boolean;
      workspace_id?: string;
      is_temporary?: boolean;
      processing_mode?: FileUploadProcessingMode;
      parse_provider?: FileParseProviderKey;
      onProgress?: (progress: number) => void;
    } = {}
  ): Promise<MultipleUploadResponse> {
    try {
      // For now, upload files one by one
      const uploadPromises = files.map(async file => {
        return this.uploadSingle(file, {
          folder: options.folder,
          public: options.public,
          workspace_id: options.workspace_id,
          is_temporary: options.is_temporary,
          processing_mode: options.processing_mode,
          parse_provider: options.parse_provider,
        });
      });

      const results = await Promise.all(uploadPromises);

      return {
        files: results,
        success: results.length,
        failed: 0,
      };
    } catch (error) {
      console.error('Multiple file upload failed:', error);
      throw error;
    }
  }
}

// Export singleton instance
export const uploadService = new UploadService();

// Export as default
export default uploadService;
