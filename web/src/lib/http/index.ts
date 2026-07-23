// HTTP module main entry point - exports all public APIs

// Configuration
export {
  type ApiEndpoint,
  type HttpConfig,
  getCurrentEnvironment,
  getHttpConfig,
  getEndpointConfig,
  buildApiUrl,
} from './config';

// Types (centralized)
export type {
  ExtendedRequestConfig,
  RetryableConfig,
  ApiResponse,
  SseMessage,
  SseOptions,
  SseEventCallbacks,
  SsePostOptions,
} from './types';

// Token management
export { TokenManager } from './token-manager';

// SSE client
export { SSE_IDLE_TIMEOUT_MS, SseClient } from './sse-client';

// HTTP Client
export { HttpClient, httpClientFactory, http } from './client';

// Webapp-specific client (separate token logic)
export { webappHttp, ensureWebAppToken, getWebAppToken, WEBAPP_TOKEN_KEY } from './webapp';

// Services
export { BaseService, type ServiceConfig, type PaginationParams } from './services';

// Backward compatibility - expose the default http client
export { http as default } from './client';
