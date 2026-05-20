import { BaseService } from '@/lib/http/services';
import type { ApiResponseData } from './types/common';

export interface SupportedFileTypesResponse {
  allowed_extensions: string[];
}

export class FileService extends BaseService {
  constructor() {
    super({
      endpoint: 'main',
      basePath: '/console/api/files',
    });
  }

  // Download raw file content (Blob)
  async downloadFile(fileId: string): Promise<Blob> {
    try {
      // Use the underlying client directly to pass responseType without type cast warnings
      const url = this.buildUrl(`/${fileId}/download`, this.config.endpoint);
      const axios = this.client.getInstance();
      const res = await axios.get(url, { responseType: 'blob' });
      return res.data as Blob;
    } catch (error) {
      console.error('Download file failed:', error);
      throw error;
    }
  }

  // Get file preview URL
  async getFilePreview(fileId: string): Promise<string> {
    try {
      const response = await this.request<ApiResponseData<{ content: string }>>(
        'get',
        `/${fileId}/preview`
      );

      // Handle the new API response format
      if (response.code === '0' && response.data) {
        return response.data.content;
      }

      // Fallback to construct preview URL directly
      return '';
    } catch (error) {
      console.error('Get file preview failed:', error);
      // Fallback to construct preview URL directly
      return '';
    }
  }

  // Get supported file types
  async getSupportedFileTypes(): Promise<SupportedFileTypesResponse> {
    try {
      const response = await this.request<ApiResponseData<SupportedFileTypesResponse>>(
        'get',
        '/support-type'
      );

      // Handle the new API response format
      if (response.code === '0' && response.data) {
        return response.data;
      }

      // If response doesn't match expected format, try to use it directly
      return response as unknown as SupportedFileTypesResponse;
    } catch (error) {
      console.error('Get supported file types failed:', error);
      throw error;
    }
  }
}

export const fileService = new FileService();

export default fileService;
