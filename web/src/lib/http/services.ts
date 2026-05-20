// Service abstraction layer for type-safe API interactions

import { httpClientFactory, type ExtendedRequestConfig } from './client';
import { buildApiUrl } from './config';

// Generic service configuration
export interface ServiceConfig {
  endpoint?: string;
  basePath?: string;
  version?: string;
}

// Upload response type
export interface UploadResponse {
  url: string;
  filename: string;
  [key: string]: unknown;
}

// Progress event type
interface ProgressEvent {
  loaded: number;
  total?: number;
}

// Common pagination parameters
export interface PaginationParams {
  page?: number;
  limit?: number;
  sort?: string;
  order?: 'asc' | 'desc';
}

// Base service class with common functionality and error handling
export abstract class BaseService {
  protected client: ReturnType<typeof httpClientFactory.getClient>;
  protected config: ServiceConfig;

  constructor(config: ServiceConfig = {}) {
    this.config = config;
    this.client = httpClientFactory.getClient(config.endpoint);
  }

  // Build service-specific URL, honoring endpoint overrides
  protected buildUrl(path: string, endpointName?: string): string {
    const sameEndpoint = !endpointName || endpointName === this.config.endpoint;
    const basePath = sameEndpoint ? this.config.basePath || '' : '';
    const fullPath = basePath ? `${basePath}${path}` : path;
    return buildApiUrl(fullPath, endpointName || this.config.endpoint);
  }

  // Handle request with common options - error handling is done in client
  protected async request<T>(
    method: 'get' | 'post' | 'put' | 'patch' | 'delete',
    path: string,
    data?: unknown,
    options: ExtendedRequestConfig = {}
  ): Promise<T> {
    const endpointName = options.endpoint || this.config.endpoint;

    // Choose client based on endpoint override
    const client =
      endpointName === this.config.endpoint
        ? this.client
        : httpClientFactory.getClient(endpointName);

    const url = this.buildUrl(path, endpointName);

    switch (method) {
      case 'get':
        return await client.get<T>(url, options);
      case 'post':
        return await client.post<T>(url, data, options);
      case 'put':
        return await client.put<T>(url, data, options);
      case 'patch':
        return await client.patch<T>(url, data, options);
      case 'delete':
        return await client.delete<T>(url, options);
      default:
        throw new Error(`Unsupported HTTP method: ${method}`);
    }
  }

  // Utility method for handling file uploads
  protected async uploadFile(
    path: string,
    file: File,
    options: {
      onProgress?: (progress: number) => void;
      additionalData?: Record<string, string>;
    } = {}
  ): Promise<UploadResponse> {
    const formData = new FormData();
    formData.append('file', file);

    // Add additional form data
    if (options.additionalData) {
      Object.entries(options.additionalData).forEach(([key, value]) => {
        formData.append(key, value);
      });
    }

    return this.request('post', path, formData, {
      headers: {
        'Content-Type': 'multipart/form-data',
      },
      onUploadProgress: options.onProgress
        ? (progressEvent: ProgressEvent) => {
            const progress = Math.round((progressEvent.loaded * 98) / (progressEvent.total || 1));
            if (options.onProgress) {
              options.onProgress(progress);
            }
          }
        : undefined,
    });
  }
}
