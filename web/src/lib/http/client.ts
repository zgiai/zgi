// HTTP client factory with multi-endpoint support and intelligent retry

import type { AxiosInstance, AxiosResponse } from 'axios';
import { AxiosError } from 'axios';
import axios from 'axios';
import * as Sentry from '@sentry/nextjs';
import { ErrorNotificationService } from '@/utils/error-notifications';
import { isAuthRedirectInProgress, isLogoutInProgress } from '@/lib/auth/logout-state';
import {
  getCurrentEnvironment,
  getEndpointConfig,
  getHttpConfig,
  type ApiEndpoint,
} from './config';
import {
  isStaleTokenRefreshError,
  StaleTokenRefreshError,
  TokenManager,
} from './token-manager';
import { SseClient } from './sse-client';
import type { ExtendedRequestConfig, RetryableConfig, SseOptions, SsePostOptions } from './types';

// Re-export types for backward compatibility
export type {
  ExtendedRequestConfig,
  ApiResponse,
  SseMessage,
  SseOptions,
  SseEventCallbacks,
  SsePostOptions,
} from './types';

/**
 * HTTP client class for managing individual endpoint instances.
 * Uses TokenManager for auth and SseClient for streaming.
 */
export class HttpClient {
  private instance: AxiosInstance;
  private endpoint: ApiEndpoint;
  private config: ReturnType<typeof getHttpConfig>;
  private tokenManager: TokenManager;
  private sseClient: SseClient;

  constructor(endpointName?: string) {
    this.endpoint = getEndpointConfig(endpointName);
    this.config = getHttpConfig();
    this.instance = this.createInstance();
    this.tokenManager = new TokenManager(this.instance, this.endpoint.name);
    this.sseClient = new SseClient(this.endpoint, () => this.tokenManager.ensureValidToken());
    this.setupInterceptors();
  }

  private createInstance(): AxiosInstance {
    return axios.create({
      baseURL: this.endpoint.baseURL,
      timeout: this.endpoint.timeout || this.config.globalTimeout,
      headers: {
        'Content-Type': 'application/json',
        Accept: 'application/json',
      },
      withCredentials: false,
      validateStatus: status => status >= 200 && status < 400,
    });
  }

  private setupInterceptors(): void {
    // Request interceptor - add auth token
    this.instance.interceptors.request.use(
      async config => {
        config.headers = config.headers || {};
        if (!(config as ExtendedRequestConfig).skipAuth) {
          try {
            const token = await this.tokenManager.ensureValidToken();
            if (!token) {
              return Promise.reject(
                new AxiosError(
                  'Authentication session is not available',
                  'ERR_AUTH_SESSION_MISSING',
                  config
                )
              );
            }
            config.headers.Authorization = `Bearer ${token}`;
          } catch (error) {
            if (isStaleTokenRefreshError(error)) {
              const token = this.getTokenAfterStaleRefresh();
              if (token) {
                config.headers.Authorization = `Bearer ${token}`;
                return config;
              }
            }

            return Promise.reject(error);
          }
        }
        return config;
      },
      error => Promise.reject(error)
    );

    // Response interceptor - handle errors
    this.instance.interceptors.response.use(
      response => {
        // Check for business error codes in successful HTTP responses
        const data = response.data;
        if (data && typeof data === 'object' && 'code' in data) {
          const code = String(data.code);
          if (code !== '0') {
            const message = data.message || 'Business Error';
            // Create an AxiosError-like structure to trigger the error interceptor
            const businessError = new AxiosError(
              message,
              'ERR_BUSINESS',
              response.config,
              response.request,
              response
            );
            return Promise.reject(businessError);
          }
        }

        return response;
      },
      async (error: AxiosError) => {
        const config = error.config as ExtendedRequestConfig;

        if (isStaleTokenRefreshError(error)) {
          const token = this.getTokenAfterStaleRefresh();
          if (token && config && !config.skipAuth && !config.isRetryRequest) {
            return this.retryWithAuthToken(config, token);
          }

          return Promise.reject(error);
        }

        // Network errors
        if (
          error.code === 'NETWORK_ERROR' ||
          error.message === 'Network Error' ||
          !error.response
        ) {
          const errorCode = (error as { code?: string }).code;
          const errorName = (error as { name?: string }).name;
          const isCanceled =
            axios.isCancel(error) ||
            errorName === 'CanceledError' ||
            errorName === 'AbortError' ||
            errorCode === 'ERR_CANCELED';
          const isMissingAuthSession = errorCode === 'ERR_AUTH_SESSION_MISSING';
          const isReadRequest = this.isReadRequest(config);
          const shouldSilence =
            isCanceled ||
            isMissingAuthSession ||
            isReadRequest ||
            isLogoutInProgress() ||
            isAuthRedirectInProgress();

          if (!config?.skipErrorHandling && !shouldSilence) {
            ErrorNotificationService.showNetworkError();
          }
          if (!shouldSilence) {
            this.logErrorToSentry(error, config, 'network_error');
          }
          return Promise.reject(error);
        }

        if (config?.skipErrorHandling) {
          return Promise.reject(error);
        }

        // Extract backend error info
        const responseData = error.response?.data as
          | { code?: string; errorCode?: string; message?: string; errorMessage?: string }
          | undefined;
        const backendCode = responseData?.code || responseData?.errorCode;
        const backendMessage = responseData?.message?.trim() || responseData?.errorMessage?.trim();

        if (backendMessage) {
          error.message = backendMessage;
        }
        if (backendCode || backendMessage) {
          (
            error as unknown as { businessError?: { code?: string; message?: string } }
          ).businessError = {
            code: backendCode ?? '',
            message: backendMessage ?? error.message,
          };
        }

        // Token refresh logic
        const shouldRefreshToken =
          (error.response?.status === 401 ||
            (error.response?.status === 400 && backendCode === '212012')) &&
          !config?.isRetryRequest &&
          !config?.isRefreshingToken &&
          !config?.skipAuth &&
          !isLogoutInProgress() &&
          Boolean(this.tokenManager.getRefreshToken());

        if (shouldRefreshToken) {
          try {
            return await this.handleTokenRefresh(config);
          } catch (refreshError) {
            if (!isStaleTokenRefreshError(refreshError)) {
              this.logErrorToSentry(refreshError, config, 'token_refresh_failed');
            }
            return Promise.reject(refreshError);
          }
        }

        // Retry logic for server errors
        if (this.shouldRetry(error, config)) {
          return this.retryRequest(config);
        }

        // Handle auth errors
        const isAuthError =
          error.response?.status === 401 ||
          (error.response?.status === 400 && backendCode === '212012');
        if (!isAuthError) {
          this.logErrorToSentry(error, config, 'api_error');
        } else if (!config?.skipAuth && !this.tokenManager.getRefreshToken()) {
          this.tokenManager.clearAuthData();
          this.tokenManager.redirectToLogin();
        }

        return Promise.reject(error);
      }
    );
  }

  private isReadRequest(config?: ExtendedRequestConfig): boolean {
    const method = config?.method?.toUpperCase();
    return !method || method === 'GET' || method === 'HEAD' || method === 'OPTIONS';
  }

  private logErrorToSentry(
    error: unknown,
    config: ExtendedRequestConfig | undefined,
    reason: string
  ): void {
    Sentry.withScope(scope => {
      scope.setContext('http', {
        url: config?.url || '',
        baseURL: config?.baseURL || '',
        method: config?.method || '',
        status: error instanceof AxiosError ? error.response?.status || 0 : 0,
      });
      scope.setContext('auth', { reason });
      scope.setTag('endpoint', this.endpoint.name);
      const errorData =
        error instanceof AxiosError
          ? (error.response?.data as { code?: string } | undefined)
          : undefined;
      if (errorData?.code) scope.setTag('business_code', errorData.code);
    });
    Sentry.captureException(error);
  }

  private shouldRetry(error: AxiosError, config?: ExtendedRequestConfig): boolean {
    const retryCount = (config as RetryableConfig)?._retryCount || 0;
    const maxRetries = config?.retryAttemptsOverride ?? this.config.retryAttempts;

    if (maxRetries <= 0 || retryCount >= maxRetries) return false;

    const code = (error.response?.data as { code?: string })?.code;
    if (
      error.code === 'NETWORK_ERROR' ||
      error.response?.status === 401 ||
      (error.response?.status === 400 && code === '212012')
    ) {
      return false;
    }

    return !error.response || error.response.status >= 500 || error.response.status === 429;
  }

  private async retryRequest(config: ExtendedRequestConfig): Promise<AxiosResponse> {
    const retryConfig = config as RetryableConfig;
    const retryCount = (retryConfig._retryCount || 0) + 1;
    const delay = this.config.retryDelay * Math.pow(2, retryCount - 1);

    await new Promise(resolve => setTimeout(resolve, delay));
    retryConfig._retryCount = retryCount;

    return this.instance.request(config);
  }

  private getTokenAfterStaleRefresh(): string | null {
    if (isLogoutInProgress()) {
      return null;
    }

    const token = this.tokenManager.getAuthToken();
    return token && this.tokenManager.canUseToken(token) ? token : null;
  }

  private retryWithAuthToken(
    config: ExtendedRequestConfig,
    token: string
  ): Promise<AxiosResponse> {
    const retryConfig = { ...config, isRetryRequest: true };
    retryConfig.headers = { ...retryConfig.headers, Authorization: `Bearer ${token}` };
    return this.instance.request(retryConfig);
  }

  private async handleTokenRefresh(config: ExtendedRequestConfig): Promise<AxiosResponse> {
    if (isLogoutInProgress()) {
      throw new StaleTokenRefreshError('Token refresh skipped because logout is in progress');
    }

    try {
      const newToken = await this.tokenManager.getOrRefreshToken();
      if (!newToken) throw new Error('Failed to obtain new access token');

      if (!this.tokenManager.canUseToken(newToken)) {
        const currentToken = this.getTokenAfterStaleRefresh();
        if (currentToken) {
          return this.retryWithAuthToken(config, currentToken);
        }

        throw new StaleTokenRefreshError('Token refresh discarded because the auth session changed');
      }

      return this.retryWithAuthToken(config, newToken);
    } catch (error) {
      if (isStaleTokenRefreshError(error)) {
        const token = this.getTokenAfterStaleRefresh();
        if (token) {
          return this.retryWithAuthToken(config, token);
        }
      }

      throw error;
    }
  }

  // Public API methods
  public async ensureValidToken(): Promise<string | null> {
    return this.tokenManager.ensureValidToken();
  }

  async get<T = unknown>(url: string, config?: ExtendedRequestConfig): Promise<T> {
    const response = await this.instance.get<T>(url, config);
    return response.data;
  }

  async post<T = unknown>(url: string, data?: unknown, config?: ExtendedRequestConfig): Promise<T> {
    const response = await this.instance.post<T>(url, data, config);
    return response.data;
  }

  async put<T = unknown>(url: string, data?: unknown, config?: ExtendedRequestConfig): Promise<T> {
    const response = await this.instance.put<T>(url, data, config);
    return response.data;
  }

  async patch<T = unknown>(
    url: string,
    data?: unknown,
    config?: ExtendedRequestConfig
  ): Promise<T> {
    const response = await this.instance.patch<T>(url, data, config);
    return response.data;
  }

  async delete<T = unknown>(url: string, config?: ExtendedRequestConfig): Promise<T> {
    const response = await this.instance.delete<T>(url, config);
    return response.data;
  }

  async upload<T = unknown>(
    url: string,
    formData: FormData,
    onProgress?: (progress: number) => void,
    config?: ExtendedRequestConfig
  ): Promise<T> {
    const uploadConfig: ExtendedRequestConfig = {
      ...config,
      headers: { ...config?.headers, 'Content-Type': 'multipart/form-data' },
      onUploadProgress: e => {
        if (onProgress && e.total) {
          onProgress(Math.round((e.loaded * 100) / e.total));
        }
      },
    };
    const response = await this.instance.post<T>(url, formData, uploadConfig);
    return response.data;
  }

  // SSE methods - delegated to SseClient
  async sse<TOut = unknown, TBody = unknown>(path: string, options: SseOptions<TBody, TOut>) {
    return this.sseClient.sse<TOut, TBody>(path, options);
  }

  async ssePost<TBody = unknown>(path: string, options: SsePostOptions<TBody>) {
    return this.sseClient.ssePost<TBody>(path, options);
  }

  getInstance(): AxiosInstance {
    return this.instance;
  }
}

// Client factory for managing multiple endpoint instances
class HttpClientFactory {
  private clients: Map<string, HttpClient> = new Map();

  getClient(endpointName?: string): HttpClient {
    const key = endpointName || 'default';
    if (!this.clients.has(key)) {
      this.clients.set(key, new HttpClient(endpointName));
    }
    return this.clients.get(key)!;
  }

  createClient(endpointName?: string): HttpClient {
    return new HttpClient(endpointName);
  }

  clearClients(): void {
    this.clients.clear();
  }
}

// Export singleton factory and default client
export const httpClientFactory = new HttpClientFactory();
const defaultClient = httpClientFactory.getClient();

export const http = {
  get: <T = unknown>(url: string, config?: ExtendedRequestConfig) =>
    defaultClient.get<T>(url, config),
  post: <T = unknown>(url: string, data?: unknown, config?: ExtendedRequestConfig) =>
    defaultClient.post<T>(url, data, config),
  put: <T = unknown>(url: string, data?: unknown, config?: ExtendedRequestConfig) =>
    defaultClient.put<T>(url, data, config),
  patch: <T = unknown>(url: string, data?: unknown, config?: ExtendedRequestConfig) =>
    defaultClient.patch<T>(url, data, config),
  delete: <T = unknown>(url: string, config?: ExtendedRequestConfig) =>
    defaultClient.delete<T>(url, config),
  upload: <T = unknown>(
    url: string,
    formData: FormData,
    onProgress?: (progress: number) => void,
    config?: ExtendedRequestConfig
  ) => defaultClient.upload<T>(url, formData, onProgress, config),
  sse: <TOut = unknown, TBody = unknown>(path: string, options: SseOptions<TBody, TOut>) =>
    defaultClient.sse<TOut, TBody>(path, options),
  ssePost: <TBody = unknown>(path: string, options: SsePostOptions<TBody>) =>
    defaultClient.ssePost<TBody>(path, options),
  ensureValidToken: () => defaultClient.ensureValidToken(),
};

export default http;
