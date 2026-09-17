// Service abstraction layer for type-safe API interactions

import { httpClientFactory, type ExtendedRequestConfig } from './client';
import { buildApiUrl, UPLOAD_TIMEOUT_MS } from './config';

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

// Multipart bodies stream from the browser and can take minutes, while the
// per-endpoint timeout is sized for JSON calls. Give a FormData request the upload
// budget unless the caller already asked for a specific timeout.
function withUploadTimeout(
  data: unknown,
  options: ExtendedRequestConfig
): ExtendedRequestConfig {
  if (options.timeout !== undefined || typeof FormData === 'undefined') {
    return options;
  }
  if (!(data instanceof FormData)) {
    return options;
  }
  return { ...options, timeout: UPLOAD_TIMEOUT_MS };
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
    const requestOptions = withUploadTimeout(data, options);

    switch (method) {
      case 'get':
        return await client.get<T>(url, requestOptions);
      case 'post':
        return await client.post<T>(url, data, requestOptions);
      case 'put':
        return await client.put<T>(url, data, requestOptions);
      case 'patch':
        return await client.patch<T>(url, data, requestOptions);
      case 'delete':
        return await client.delete<T>(url, requestOptions);
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
      signal?: AbortSignal;
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
            // `total` is 0 when the browser cannot determine the body size. Dividing
            // by it produced runaway percentages, so report 0 until the size is known.
            const total = progressEvent.total || 0;
            const progress = total > 0 ? Math.round((progressEvent.loaded * 98) / total) : 0;
            if (options.onProgress) {
              options.onProgress(progress);
            }
          }
        : undefined,
      signal: options.signal,
    });
  }
}
